package catalog

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/services/geo"
	"psychic-homily-backend/internal/services/shared"
)

// Scheduled street-geocode reconciliation (PSY-1544). A daily background
// ticker runs BackfillVenueStreetGeocodes — the same reconciler the manual
// geocode-venue-addresses CLI drives — so the write paths that deliberately
// skip inline Nominatim lookups (FindOrCreateVenue on show submission,
// data-sync import) and the contribution-edit path (which clears the street
// fields on an address change) converge to street-level pins without an
// operator remembering to run the CLI.
//
// Design points:
//
//   - SHARED Nominatim client: the container injects geo.DefaultNominatim(),
//     the same instance behind the inline CreateVenue/UpdateVenue geocoding.
//     Its semaphore serializes every request — including the pre-request
//     spacing wait — so sweep + inline traffic can never combine past the
//     OSM policy's 1 req/s budget. This is what makes the scheduled run safe
//     alongside live venue writes (the CLI, being a separate process, still
//     carries its off-hours caveat).
//   - Cheap steady state: the reconciler skips any venue whose stored
//     geocoded_address already matches its current address key — hits AND
//     recorded miss memos — so a tick only spends network budget on venues
//     whose address changed or was never attempted. Errors stay unrecorded
//     and retry next tick.
//   - Default ON (DISABLE_STREET_GEOCODE_SWEEP=1 to opt out), unlike the
//     opt-in ENABLE_* enrichment sweeps: those auto-write name-matched
//     third-party data (homonym/flood risk); this sweep performs the exact
//     deterministic address→coords write the inline paths already make by
//     default, and default-on is what actually closes the NULL-pin /
//     silent-downgrade leak without per-environment action.
//
// The interval ticker is anchored at process start, so "off-peak" cannot be
// guaranteed — but it isn't load-bearing: a tick occupies Nominatim for at
// most ~Limit seconds (1.1s spacing each), serialized with inline traffic by
// the shared limiter. Steady-state venue creation is far below the default
// limit; the initial catalog-wide backfill remains the CLI's job.
//
// PSY-1603 — why the first cycle is delay-anchored, not interval-anchored:
// the original loop ran no startup cycle and waited a full 24h interval, but
// the ticker restarts from zero on every process start. Production redeploys
// well inside 24h (deploy history showed a maximum completed uptime of 13h23m
// in the days after this sweep shipped), so the first tick was never reached
// and the sweep wrote nothing at all — 75 addressed venues sat on city
// centroids with no error to alert on. The fix keeps third-party traffic off
// the boot path (the reason there is no startup cycle) but anchors the first
// cycle to a short delay that fits inside any plausible uptime.
const (
	defaultStreetGeocodeSweepInterval = 24 * time.Hour
	// 25 lookups ≈ half a minute of the shared budget per day. Large-backlog
	// scenarios (feature rollout, mass import) should use the CLI instead of
	// raising this: the sweep exists for the trickle, not the flood.
	defaultStreetGeocodeSweepLimit = 25
	// Long enough to keep Nominatim off the boot path and to let a burst of
	// rapid redeploys pass without any cycle running; short enough to fit
	// inside any uptime the platform realistically gives us. Bounding the
	// cost of the extra cycles a deploy-heavy day now produces is Limit's
	// job — and a converged catalog makes zero network calls per cycle,
	// because the reconciler skips every venue whose stored key still
	// matches its address.
	defaultStreetGeocodeSweepStartDelay = 15 * time.Minute
)

// StreetGeocodeSweep is a background ticker service (mirrors
// ArtistLocationSweep / ImageEnrichmentSweep) that reconciles venue
// street-level geocodes via the shared backfill reconciler.
type StreetGeocodeSweep struct {
	db       *gorm.DB
	geocoder geo.AddressGeocoder

	interval   time.Duration
	startDelay time.Duration
	limit      int

	stopCh chan struct{}
	wg     sync.WaitGroup
	logger *slog.Logger
}

// NewStreetGeocodeSweep constructs the sweep. geocoder MUST be the
// process-wide shared client (geo.DefaultNominatim()) so the 1 req/s budget
// holds across sweep + inline write-path traffic.
func NewStreetGeocodeSweep(database *gorm.DB, geocoder geo.AddressGeocoder) *StreetGeocodeSweep {
	if database == nil {
		database = db.GetDB()
	}
	return &StreetGeocodeSweep{
		db:       database,
		geocoder: geocoder,
		interval:   shared.EnvPositiveDuration("STREET_GEOCODE_SWEEP_INTERVAL_HOURS", time.Hour, defaultStreetGeocodeSweepInterval),
		startDelay: shared.EnvPositiveDuration("STREET_GEOCODE_SWEEP_START_DELAY_MINUTES", time.Minute, defaultStreetGeocodeSweepStartDelay),
		limit:      shared.EnvPositiveInt("STREET_GEOCODE_SWEEP_LIMIT", defaultStreetGeocodeSweepLimit),
		stopCh:     make(chan struct{}),
		logger:     slog.Default(),
	}
}

// Start begins the background sweep. Still no cycle ON the boot path — a
// server restart shouldn't kick off third-party traffic — but the first cycle
// is anchored to startDelay rather than a full interval, because an
// interval-anchored first tick never arrives on a platform that redeploys more
// often than the interval (PSY-1603).
func (s *StreetGeocodeSweep) Start(ctx context.Context) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		shared.RunTickerLoopWithStartDelay(ctx, "street_geocode_sweep", s.startDelay, s.interval, s.stopCh, s.runCycle)
	}()
	s.logger.Info("street geocode sweep started", "interval", s.interval, "start_delay", s.startDelay, "limit", s.limit)
}

// Stop gracefully stops the sweep. The reconciler is context-aware down to
// the Nominatim limiter wait, so canceling the Start ctx before Stop (the
// main.go shutdown order) aborts an in-flight cycle promptly and this Wait
// never rides out a slow round trip.
func (s *StreetGeocodeSweep) Stop() {
	close(s.stopCh)
	s.wg.Wait()
	s.logger.Info("street geocode sweep stopped")
}

// RunSweepNow runs one cycle immediately (tests / manual trigger).
func (s *StreetGeocodeSweep) RunSweepNow(ctx context.Context) { s.runCycle(ctx) }

// runCycle runs one bounded reconciler pass and logs its outcome counts.
func (s *StreetGeocodeSweep) runCycle(ctx context.Context) {
	report, err := BackfillVenueStreetGeocodes(ctx, s.db, s.geocoder, StreetGeocodeOptions{
		Limit: s.limit,
	})
	if err != nil {
		// Load failure (report == nil) or ctx cancellation mid-run (partial
		// report). Log partial progress when there is any — a shutdown that
		// interrupts a cycle shouldn't hide what it already wrote. A canceled
		// ctx is the NORMAL shutdown path, not a fault — log it at Info so a
		// deploy that happens to land mid-cycle doesn't read as an error.
		logFn := s.logger.Error
		if errors.Is(err, context.Canceled) {
			logFn = s.logger.Info
		}
		if report != nil {
			logFn("street geocode sweep: cycle interrupted",
				"error", err,
				"scanned", report.Scanned,
				"set", report.Set,
				"updated", report.Updated,
				"missed", report.Missed,
				"cleared", report.Cleared,
			)
		} else {
			logFn("street geocode sweep: cycle failed", "error", err)
		}
		return
	}
	s.logger.Info("street geocode sweep run done",
		"scanned", report.Scanned,
		"set", report.Set,
		"updated", report.Updated,
		"unchanged", report.Unchanged,
		"missed", report.Missed,
		"cleared", report.Cleared,
		"no_address", report.NoAddress,
		"errors", len(report.Errors),
		"limit_hit", report.LimitHit,
	)
}
