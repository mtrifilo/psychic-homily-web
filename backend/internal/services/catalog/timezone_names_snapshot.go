package catalog

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/getsentry/sentry-go"
	"gorm.io/gorm"
)

// Maintenance of timezone_names_snapshot, the table shared.VenueTZJoin tests a
// stored venues.timezone against before feeding it to AT TIME ZONE (PSY-1761).
//
// The table is CREATED AND SEEDED BY ITS MIGRATION, so a migrated database is
// already correct and nothing in this file is a prerequisite for the read path.
// What it adds is FRESHNESS, and freshness is what bounds the guard's residual
// risk: a zone that was valid when written and has since left the catalog stays
// trusted until the snapshot catches up.
//
// The catalog moves when the SERVER does — a Postgres upgrade, a tzdata
// refresh, a restore onto a differently-packaged image (measured: postgres:18
// Debian carries 487 zones and lacks EST and Asia/Calcutta because Debian
// splits tzdata's `backward` links into tzdata-legacy; postgres:16-alpine
// carries 599 and has both). That is why the refresh runs at BOOT and not only
// on the sweep's timer: the events that invalidate the snapshot are the same
// events that restart this process. VenueTimezoneSweep refreshes it again on
// its own cadence, for the case where the database is replaced underneath a
// server that keeps running.

// TimezoneNamesSnapshotReport is one refresh's outcome. Added and Removed are
// reported separately because they mean opposite things: Added is routine (a
// tzdata release gains zones and a venue can now be dated by its own clock),
// while Removed means the catalog LOST names, so values that were legal when
// written may now be stored in venues.timezone.
type TimezoneNamesSnapshotReport struct {
	Total   int
	Added   int
	Removed int
}

// errEmptyTimezoneCatalog means pg_timezone_names returned nothing, which no
// working Postgres does.
var errEmptyTimezoneCatalog = errors.New("pg_timezone_names returned no rows")

// RefreshTimezoneNamesSnapshot reconciles timezone_names_snapshot with the
// server's live pg_timezone_names.
//
// It REFUSES to empty the table. An empty snapshot would not fail loudly: every
// venue's zone would fail the read guard's membership test and every show would
// silently re-date onto the US state map, which is exactly the silent
// wrongness the guard exists to avoid. So a catalog read returning zero rows is
// treated as a broken read, not as "there are no zones".
//
// The whole reconcile runs in one transaction, so no concurrent listing query
// can observe a half-rebuilt snapshot and mis-date a page. The catalog is read
// ONCE into a MATERIALIZED CTE rather than three times: pg_timezone_names is a
// set-returning function that reopens the tzdata files per scan, measured at
// 3.5 ms idle and 43-63 ms under load, which is the same cost that kept it off
// the read path in the first place.
func RefreshTimezoneNamesSnapshot(ctx context.Context, database *gorm.DB) (*TimezoneNamesSnapshotReport, error) {
	if database == nil {
		return nil, errors.New("database not initialized")
	}

	report := &TimezoneNamesSnapshotReport{}
	err := database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		type reconcileRow struct {
			Total   int `gorm:"column:total"`
			Added   int `gorm:"column:added"`
			Removed int `gorm:"column:removed"`
		}
		var row reconcileRow

		// One statement, one catalog scan, and the counts fall out of it. The
		// emptiness guard is the WHERE on both writes: if `live` is empty,
		// `pruned` deletes nothing and `added` inserts nothing, and the total
		// of 0 below turns that into an error instead of an emptied table.
		const reconcile = `
			WITH live AS MATERIALIZED (
				SELECT name FROM pg_timezone_names
			),
			pruned AS (
				DELETE FROM timezone_names_snapshot s
				WHERE EXISTS (SELECT 1 FROM live)
				  AND NOT EXISTS (SELECT 1 FROM live l WHERE l.name = s.name)
				RETURNING 1
			),
			added AS (
				INSERT INTO timezone_names_snapshot (name)
				SELECT name FROM live
				ON CONFLICT (name) DO NOTHING
				RETURNING 1
			)
			SELECT (SELECT count(*) FROM live)   AS total,
			       (SELECT count(*) FROM added)  AS added,
			       (SELECT count(*) FROM pruned) AS removed`

		if err := tx.Raw(reconcile).Scan(&row).Error; err != nil {
			return fmt.Errorf("reconcile timezone_names_snapshot: %w", err)
		}
		if row.Total == 0 {
			return errEmptyTimezoneCatalog
		}
		report.Total, report.Added, report.Removed = row.Total, row.Added, row.Removed
		return nil
	})
	if err != nil {
		return nil, err
	}
	return report, nil
}

// RefreshAndReportTimezoneNamesSnapshot refreshes the snapshot and reports both
// things an operator needs to know afterwards: whether the snapshot had drifted
// from the catalog, and which venues the refreshed snapshot strands. It
// swallows errors so a caller on the boot path can treat the whole thing as
// telemetry.
//
// Not fatal on purpose: the table is already correct from its migration, so a
// failed refresh leaves the read path working against a possibly stale
// allowlist — strictly better than refusing to boot, and the log line says so.
//
// It reports as well as refreshing because BOOT MAY BE THE ONLY PLACE IT GETS
// REPORTED. VenueTimezoneSweep names the same venues on its own cycle, but it
// is opt-in (ENABLE_VENUE_TIMEZONE_SWEEP) and daily, so in an environment where
// nobody set the flag this is the sole signal that a venue's shows are being
// dated by the state map instead of by its own clock. The sweep calls
// RefreshTimezoneNamesSnapshot directly rather than this, so the two never
// report the same row twice in one cycle.
func RefreshAndReportTimezoneNamesSnapshot(ctx context.Context, database *gorm.DB, logger *slog.Logger) {
	report, err := RefreshTimezoneNamesSnapshot(ctx, database)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			logger.Info("timezone snapshot refresh cancelled", "error", err)
			return
		}
		logger.Error("could not refresh the timezone name snapshot; the venue-local "+
			"zone guard is running against a possibly stale allowlist", "error", err)
		captureTimezoneSnapshotEvent(func(scope *sentry.Scope) {
			scope.SetFingerprint([]string{"timezone-names-snapshot", "refresh-failed"})
			sentry.CaptureException(fmt.Errorf("refresh timezone_names_snapshot: %w", err))
		})
		return
	}

	// A refresh that CHANGED anything is itself a finding, and it is the only
	// one available for the guard's own staleness. Until this pass ran, the
	// guard was judging against a set that disagreed with the catalog: every
	// name in Added was a real zone being sent to the state map, and every name
	// in Removed was a name the guard was still trusting that AT TIME ZONE
	// would now raise on. Nothing else reports that — the drift detector below
	// compares venues against the LIVE catalog, so it is blind to the snapshot
	// having been behind. Steady state is 0/0, because the migration seeds the
	// table from the same catalog, so this is quiet unless it matters.
	if report.Added > 0 || report.Removed > 0 {
		logger.Warn("the timezone allowlist had drifted from this server's catalog; "+
			"venue-local dates resolved before this refresh may have used the wrong zone",
			"added", report.Added, "removed", report.Removed, "total", report.Total)
		captureTimezoneSnapshotEvent(func(scope *sentry.Scope) {
			scope.SetLevel(sentry.LevelWarning)
			scope.SetFingerprint([]string{"timezone-names-snapshot", "allowlist-drifted"})
			scope.SetExtra("added", report.Added)
			scope.SetExtra("removed", report.Removed)
			sentry.CaptureMessage(fmt.Sprintf(
				"timezone allowlist drifted from the server catalog: +%d/-%d of %d zones",
				report.Added, report.Removed, report.Total))
		})
	}

	drifted, err := detectVenueTimezoneDrift(database.WithContext(ctx))
	if err != nil {
		logger.Error("could not check stored venue timezones against the refreshed catalog; "+
			"any venue being dated by the state-map fallback is UNREPORTED this pass", "error", err)
		captureTimezoneSnapshotEvent(func(scope *sentry.Scope) {
			scope.SetFingerprint([]string{"timezone-names-snapshot", "drift-check-failed"})
			sentry.CaptureException(fmt.Errorf("detect drifted venue timezones: %w", err))
		})
		return
	}

	// Each stranded venue gets a line naming the rejected value, because that
	// is what an operator needs to re-geocode it. Sentry gets ONE issue for all
	// of them, fingerprinted on the condition rather than the venue: this is a
	// catalog-drift event, and one issue per venue per spelling would fragment
	// it exactly the way radio_sync.go's fingerprint comment warns about.
	for _, d := range drifted {
		logger.Error("venue timezone is not in the server's zone catalog; its shows are "+
			"being dated by the state-map fallback instead",
			"venue_id", d.VenueID, "venue_name", d.Name, "rejected_timezone", d.Timezone)
	}
	if len(drifted) > 0 {
		// Rendered to ONE string rather than attached as a slice of structs.
		// observability.ScrubSentryEvent only reaches STRING Extra values and
		// says so as a constraint on call sites: a map or struct passes through
		// the scrub hook untouched. Formatting here keeps this payload inside
		// what the scrubber can see.
		stranded := make([]string, 0, len(drifted))
		for _, d := range drifted {
			stranded = append(stranded, fmt.Sprintf("%d %s -> %s", d.VenueID, d.Name, d.Timezone))
		}
		captureTimezoneSnapshotEvent(func(scope *sentry.Scope) {
			scope.SetLevel(sentry.LevelError)
			scope.SetFingerprint([]string{"timezone-names-snapshot", "venues-stranded"})
			scope.SetExtra("venue_count", strconv.Itoa(len(drifted)))
			scope.SetExtra("venues", strings.Join(stranded, "; "))
			sentry.CaptureMessage(fmt.Sprintf(
				"%d venue(s) hold a timezone this server's catalog does not carry; their shows "+
					"are being dated by the state-map fallback", len(drifted)))
		})
	}

	logger.Info("timezone name snapshot refreshed",
		"total", report.Total, "added", report.Added, "removed", report.Removed,
		"stranded_venues", len(drifted))
}

// captureTimezoneSnapshotEvent applies the tags every event from this file
// carries, matching the WithScope shape the notification and radio services
// use. Without the service tag these events cannot be filtered or alerted on.
func captureTimezoneSnapshotEvent(capture func(scope *sentry.Scope)) {
	sentry.WithScope(func(scope *sentry.Scope) {
		scope.SetTag("service", "timezone_names_snapshot")
		scope.SetTag("source", "venue_local_zone_guard")
		capture(scope)
	})
}
