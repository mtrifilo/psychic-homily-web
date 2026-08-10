package catalog

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/getsentry/sentry-go"
	"gorm.io/gorm"
)

// Maintenance of timezone_names_snapshot, the table shared.VenueTZJoin tests a
// stored venues.timezone against before feeding it to AT TIME ZONE (PSY-1761).
//
// The table is CREATED AND SEEDED BY ITS MIGRATION, so a migrated database is
// already correct and nothing here is a prerequisite for the read path. What
// this file adds is FRESHNESS, and freshness is what bounds the guard's
// residual risk: a zone that was valid when written and has since left the
// catalog stays trusted until the snapshot next catches up.
//
// The catalog moves when the SERVER does — a Postgres upgrade, a tzdata
// refresh, a restore onto a differently-packaged image (measured: postgres:18
// Debian carries 487 zones and lacks EST and Asia/Calcutta because Debian
// splits tzdata's `backward` links into tzdata-legacy; postgres:16-alpine
// carries 599 and has both). That is why the refresh runs at BOOT rather than
// only on a timer: the events that invalidate the snapshot are the same events
// that restart this process, so boot is the moment the answer is most likely to
// have changed. The venue timezone sweep refreshes it again on its own cadence
// for the case where the database is replaced underneath a server that keeps
// running.

// TimezoneNamesSnapshotReport is one refresh's outcome. Added and Removed are
// reported separately because they mean opposite things: Added is routine
// (a tzdata release gains zones), while Removed means values that were legal
// when written may now be in the database, which is what DriftedVenues names.
type TimezoneNamesSnapshotReport struct {
	Total   int
	Added   int
	Removed int
	// DriftedVenues are the venues whose stored zone the refreshed snapshot no
	// longer carries — i.e. the rows the read guard is now silently routing to
	// the state-map fallback. Reported so a wrong date gets fixed rather than
	// living forever behind a query that no longer complains.
	DriftedVenues []VenueTimezoneDrift
}

// ErrEmptyTimezoneCatalog means pg_timezone_names returned nothing, which no
// working Postgres does. Distinguished from a query failure because the
// response is the same either way — leave the existing snapshot alone — but the
// diagnosis is not.
var ErrEmptyTimezoneCatalog = errors.New("pg_timezone_names returned no rows")

// RefreshTimezoneNamesSnapshot reconciles timezone_names_snapshot with the
// server's live pg_timezone_names and reports the venues left stranded by the
// difference.
//
// It REFUSES to empty the table. An empty snapshot would not fail loudly: every
// venue's zone would fail the membership test and every show would silently
// re-date onto the US state map, which is exactly the silent-wrongness the
// guard exists to avoid. So a catalog read returning zero rows is treated as a
// broken read, not as "there are no zones".
//
// The whole reconcile runs in one transaction, so no concurrent listing query
// can observe a half-rebuilt snapshot and mis-date a page.
func RefreshTimezoneNamesSnapshot(ctx context.Context, database *gorm.DB) (*TimezoneNamesSnapshotReport, error) {
	if database == nil {
		return nil, errors.New("database not initialized")
	}

	report := &TimezoneNamesSnapshotReport{}
	err := database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var live int64
		if err := tx.Raw("SELECT count(*) FROM pg_timezone_names").Scan(&live).Error; err != nil {
			return fmt.Errorf("read pg_timezone_names: %w", err)
		}
		if live == 0 {
			return ErrEmptyTimezoneCatalog
		}
		report.Total = int(live)

		removed := tx.Exec(`DELETE FROM timezone_names_snapshot s
			WHERE NOT EXISTS (SELECT 1 FROM pg_timezone_names t WHERE t.name = s.name)`)
		if removed.Error != nil {
			return fmt.Errorf("prune timezone_names_snapshot: %w", removed.Error)
		}
		report.Removed = int(removed.RowsAffected)

		added := tx.Exec(`INSERT INTO timezone_names_snapshot (name)
			SELECT name FROM pg_timezone_names ON CONFLICT (name) DO NOTHING`)
		if added.Error != nil {
			return fmt.Errorf("populate timezone_names_snapshot: %w", added.Error)
		}
		report.Added = int(added.RowsAffected)

		// Read the stranded venues inside the SAME transaction, so the list
		// describes the snapshot this call just wrote rather than one a
		// concurrent refresh may have moved on from.
		drifted, err := detectVenueTimezoneDrift(tx)
		if err != nil {
			return fmt.Errorf("detect drifted venue timezones: %w", err)
		}
		report.DriftedVenues = drifted
		return nil
	})
	if err != nil {
		return nil, err
	}
	return report, nil
}

// ReconcileTimezoneNamesSnapshot refreshes the snapshot and emits the operator
// signal, swallowing the error so a caller on the boot path can treat it as
// telemetry rather than a reason not to serve traffic.
//
// Not fatal on purpose. The table is already correct from its migration, so a
// failed refresh leaves the read path working against a possibly-stale
// allowlist — strictly better than refusing to boot, and the log line says so.
func ReconcileTimezoneNamesSnapshot(ctx context.Context, database *gorm.DB, logger *slog.Logger) {
	if logger == nil {
		logger = slog.Default()
	}
	report, err := RefreshTimezoneNamesSnapshot(ctx, database)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			logger.Info("timezone snapshot refresh cancelled", "error", err)
			return
		}
		logger.Error("could not refresh the timezone name snapshot; the venue-local "+
			"zone guard is running against a possibly stale allowlist", "error", err)
		sentry.CaptureException(fmt.Errorf("refresh timezone_names_snapshot: %w", err))
		return
	}

	// Each stranded venue gets its own line AND its own Sentry event. This is
	// the only signal that a venue is being silently re-dated onto the state
	// map: the query that used to raise now succeeds, so nothing else will
	// complain, and a count alone would not say which venue to re-geocode.
	for _, d := range report.DriftedVenues {
		logger.Error("venue timezone is not in the server's zone catalog; its shows are "+
			"being dated by the state-map fallback instead",
			"venue_id", d.VenueID, "venue_name", d.Name, "rejected_timezone", d.Timezone)
		sentry.CaptureMessage(fmt.Sprintf(
			"venue %d (%s) has an unknown timezone %q; shows are falling back to the state map",
			d.VenueID, d.Name, d.Timezone))
	}

	logger.Info("timezone name snapshot refreshed",
		"total", report.Total, "added", report.Added, "removed", report.Removed,
		"drifted_venues", len(report.DriftedVenues))
}
