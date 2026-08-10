package catalog

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/services/shared"
)

// Scheduled venue-timezone integrity sweep (PSY-1695).
//
// The show-list partition tests venues.timezone against a snapshot of the
// server's zone catalog and falls back to the state map when it misses
// (PSY-1761, shared.VenueTZJoin). It no longer RAISES on an unknown name, which
// changes what this sweep is FOR: it is no longer the thing standing between a
// drifted zone and an outage, it is the thing standing between a drifted zone
// and a silently wrong date. Its log line is the operator's cue to re-geocode.
//
// Layer one is the WRITE GATE (PSY-1707): every path that writes the column
// validates against pg_timezone_names first and stores NULL rather than a name
// the server does not carry. That is a point-in-time check, and it is not
// enough on its own, because the catalog is not stable.
//
// What pg_timezone_names contains comes from the server's TZDATA PACKAGING, not
// from Postgres, so the accept-set changes under a Postgres upgrade, a
// base-image change, a tzdata refresh (US/Pacific-New really was deleted in
// 2020b), or a restore onto a differently-packaged build — and a value that was
// valid when written silently becomes one this server cannot resolve. The
// measured packaging difference behind that claim is recorded once, in
// timezone_names_snapshot.go.
//
// This sweep re-validates stored zones against the LIVE catalog and NULLs the
// casualties, which is a shape every reader already handles (it is what a
// geocode miss produces, and it falls back to the US state map). Since PSY-1761
// it also refreshes timezone_names_snapshot on the same cycle, so the read
// guard and this sweep are judging against one catalog read rather than two
// that can disagree.
//
// Design points:
//
//   - Cheap steady state. One indexed-ish scan comparing venues.timezone
//     against pg_timezone_names; in the converged case it matches nothing and
//     writes nothing. There is no network call and no third-party budget to
//     respect, which is why it can afford to check EVERY venue rather than page.
//   - NULL, never guess. A drifted zone is set to NULL rather than remapped to
//     a plausible neighbour: a wrong zone silently mis-dates every show at that
//     venue, while an absent one is visibly a gap and falls back. Each one is
//     logged with the venue id and the rejected value so an operator can
//     re-geocode deliberately.
//   - Bounded window, and since PSY-1761 a window on a WRONG DATE rather than
//     on an outage. A zone can go unknown between cycles; until the next cycle
//     the read guard dates that venue's shows by the state map. Shortening the
//     interval shrinks the window of wrongness; it cannot close it.
//
// Registered as opt-in via ENABLE_VENUE_TIMEZONE_SWEEP, matching the ENABLE_*
// sweeps rather than the DISABLE_* workers. It writes NULLs over operator-
// visible data on a schedule, so it should require a deliberate act to turn ON
// in a given environment rather than arriving silently with a deploy.
//
// Its 24h interval puts it inside the overdue-sweep health check's coverage
// (PSY-1612 watches loops with an interval of an hour or more), so a silently
// dead cycle is reported rather than discovered weeks later. Note the corollary
// for whoever turns it off: a sweep that has run in an environment keeps its
// registration row and will read as overdue for the 7-day retirement window --
// see backend/README.md, "Overdue-sweep alerting", for the retirement SQL. A
// sweep never enabled in an environment has no row and is never reported.
//
// It NULLs rather than re-deriving. Re-geocoding here would be a second guess
// layered on the first: the venue's city/state may also have moved on, and the
// backfill CLI already owns deliberate re-derivation. NULL puts the venue in the
// state map fallback immediately and leaves a logged trail for an operator to
// act on.
const (
	defaultVenueTimezoneSweepInterval = 24 * time.Hour
	// Deliberately long. The drift this catches arrives with a deploy or an
	// image bump, not continuously, so a daily pass is ample; a short interval
	// would only add scans that find nothing.
	defaultVenueTimezoneSweepStartDelay = 10 * time.Minute
)

// EnableVenueTimezoneSweepEnvVar gates startup. Named as a constant, like
// EnablePublicReadRateLimitsEnvVar, so the flag string has one definition
// shared by main.go and its tests.
const EnableVenueTimezoneSweepEnvVar = "ENABLE_VENUE_TIMEZONE_SWEEP"

// VenueTimezoneSweep is a background ticker service that re-validates stored
// venue timezones against the server's live zone catalog.
type VenueTimezoneSweep struct {
	db *gorm.DB

	interval   time.Duration
	startDelay time.Duration

	stopCh chan struct{}
	wg     sync.WaitGroup
	logger *slog.Logger
}

// NewVenueTimezoneSweep constructs the sweep.
func NewVenueTimezoneSweep(database *gorm.DB) *VenueTimezoneSweep {
	if database == nil {
		database = db.GetDB()
	}
	return &VenueTimezoneSweep{
		db:         database,
		interval:   shared.EnvPositiveDuration("VENUE_TIMEZONE_SWEEP_INTERVAL_HOURS", time.Hour, defaultVenueTimezoneSweepInterval),
		startDelay: shared.EnvPositiveDuration("VENUE_TIMEZONE_SWEEP_START_DELAY_MINUTES", time.Minute, defaultVenueTimezoneSweepStartDelay),
		stopCh:     make(chan struct{}),
		logger:     slog.Default(),
	}
}

// Start begins the background sweep. Like the sibling sweeps it runs no cycle
// on the boot path, and RunScheduledLoop schedules the first pass from persisted
// run state so a redeploy cannot reset its clock (the PSY-1606 failure mode).
func (s *VenueTimezoneSweep) Start(ctx context.Context) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		shared.RunScheduledLoop(ctx, shared.LoopConfig{
			Name:       "venue_timezone_sweep",
			Interval:   s.interval,
			StopCh:     s.stopCh,
			StartDelay: s.startDelay,
		}, s.runCycle)
	}()
	s.logger.Info("venue timezone sweep started", "interval", s.interval, "start_delay", s.startDelay)
}

// Stop gracefully stops the sweep.
func (s *VenueTimezoneSweep) Stop() {
	close(s.stopCh)
	s.wg.Wait()
	s.logger.Info("venue timezone sweep stopped")
}

// RunSweepNow runs one cycle immediately (tests / manual trigger).
func (s *VenueTimezoneSweep) RunSweepNow(ctx context.Context) { s.runCycle(ctx) }

// VenueTimezoneDrift is one venue whose stored zone the server no longer knows.
type VenueTimezoneDrift struct {
	VenueID  uint   `gorm:"column:id"`
	Name     string `gorm:"column:name"`
	Timezone string `gorm:"column:timezone"`
}

// VenueTimezoneSweepReport is one cycle's outcome.
type VenueTimezoneSweepReport struct {
	Scanned int
	Cleared int
	Drifted []VenueTimezoneDrift
}

// SweepVenueTimezones re-validates every stored venue timezone against the live
// pg_timezone_names and NULLs the ones the server no longer carries.
//
// Exported and DB-only (no service state) so the integration tests and any
// future CLI can drive the same reconciler the ticker does — the sweep body and
// the thing under test must not be allowed to diverge.
func SweepVenueTimezones(ctx context.Context, database *gorm.DB) (*VenueTimezoneSweepReport, error) {
	if database == nil {
		return nil, errors.New("database not initialized")
	}
	report := &VenueTimezoneSweepReport{}

	var scanned int64
	if err := database.WithContext(ctx).
		Table("venues").Where("timezone IS NOT NULL").Count(&scanned).Error; err != nil {
		return nil, err
	}
	report.Scanned = int(scanned)

	// Identify first, then clear, so the log names exactly what changed.
	drifted, err := detectVenueTimezoneDrift(database.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	report.Drifted = drifted
	if len(report.Drifted) == 0 {
		return report, nil
	}

	res := database.WithContext(ctx).Table("venues").
		Where(venueTimezoneDriftPredicate).
		Update("timezone", nil)
	if res.Error != nil {
		return nil, res.Error
	}
	report.Cleared = int(res.RowsAffected)
	return report, nil
}

// venueTimezoneDriftPredicate matches every venue whose stored zone this server
// can no longer resolve. The blank arm is belt-and-braces: the write gate stores
// NULL for blank input, but a blank reaching AT TIME ZONE is as unusable as an
// unknown name.
//
// Compared against the LIVE catalog rather than timezone_names_snapshot, which
// is the whole point of running it: it is the thing that says whether the
// snapshot the read guard trusts has fallen behind. Testing against the
// snapshot would make the check agree with itself by construction.
//
// Trimmed and lowercased exactly as shared.VenueTZJoin's read guard does, and
// built from the same shared.VenueTimezoneWhitespaceSQL so the two cannot
// drift. A detector stricter than the guard reports venues that are being dated
// correctly; a detector looser than the guard leaves a mis-dated venue with no
// signal at all, which is the failure this pairing exists to prevent.
var venueTimezoneDriftPredicate = `timezone IS NOT NULL AND (
	btrim(timezone, ` + shared.VenueTimezoneWhitespaceSQL + `) = ''
	OR NOT EXISTS (
		SELECT 1 FROM pg_timezone_names t
		WHERE lower(t.name) = lower(btrim(venues.timezone, ` + shared.VenueTimezoneWhitespaceSQL + `))
	)
)`

// detectVenueTimezoneDrift lists the venues the predicate above matches.
//
// Shared by the sweep (which then NULLs them) and by the snapshot refresh
// (which only reports them), so the set an operator is told about and the set
// that gets cleared cannot drift apart. Takes the scope rather than a *gorm.DB
// so a caller inside a transaction sees its own uncommitted snapshot.
func detectVenueTimezoneDrift(scope *gorm.DB) ([]VenueTimezoneDrift, error) {
	var drifted []VenueTimezoneDrift
	if err := scope.Table("venues").
		Select("id, name, timezone").
		Where(venueTimezoneDriftPredicate).
		Scan(&drifted).Error; err != nil {
		return nil, err
	}
	return drifted, nil
}

// runCycle runs one pass and logs its outcome.
func (s *VenueTimezoneSweep) runCycle(ctx context.Context) {
	// Refresh the read guard's allowlist first. This is the cadence that covers
	// the database being replaced underneath a server that keeps running — the
	// boot-time reconcile in cmd/server covers the ordinary upgrade, where the
	// process restarts anyway. Doing it before the sweep also means the two
	// agree: both are then judging against the same catalog.
	//
	// The refresh reports nothing here on purpose. Its drifted-venue signal and
	// the per-venue lines below name the SAME rows, and firing both would tell
	// an operator a venue is "falling back to the state map" and then, a
	// millisecond later, that it was "cleared to NULL" — two Sentry issues and
	// two contradictory log lines for one fact. The sweep owns the signal
	// because it is the half that also fixes the row.
	if _, err := RefreshTimezoneNamesSnapshot(ctx, s.db); err != nil {
		s.logger.Error("could not refresh the timezone name snapshot; the venue-local "+
			"zone guard is running against a possibly stale allowlist", "error", err)
	}

	report, err := SweepVenueTimezones(ctx, s.db)
	if err != nil {
		logFn := s.logger.Error
		if errors.Is(err, context.Canceled) {
			logFn = s.logger.Info
		}
		logFn("venue timezone sweep: cycle failed", "error", err)
		shared.ReportCycle(ctx, 0, err)
		return
	}

	// Each casualty gets its own line: this is the operator's only signal that a
	// venue silently lost its zone, and a count alone would not say which.
	for _, d := range report.Drifted {
		s.logger.Error("venue timezone is no longer in the server's zone catalog; cleared to NULL",
			"venue_id", d.VenueID, "venue_name", d.Name, "rejected_timezone", d.Timezone)
	}

	shared.ReportCycle(ctx, report.Scanned, nil)
	s.logger.Info("venue timezone sweep: cycle complete",
		"scanned", report.Scanned, "cleared", report.Cleared)
}
