package enrich

import (
	"context"
	"log/slog"
	"os"
	"strconv"
	"sync"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/services/shared"
)

// Ongoing artist-LOCATION sweep (PSY-1250) — Phase A of the artist-enrichment
// rollout. A slow background ticker runs the fill-when-empty location resolver
// (BackfillArtistLocations: MusicBrainz origin + Bandcamp self-report) over a bounded
// batch of still-city-less artists, so location coverage stays current as the catalog
// grows regardless of how an artist was added.
//
// Two design points carry the weight (both mirror the image sweep, PSY-1246):
//
//   - SHARED MusicBrainz client (PSY-1208): the resolver's MB calls MUST go through
//     the one process-wide ~1 req/s throttle — a second client would double the rate
//     and trip MB's sticky 503. The container passes the shared *pipeline client.
//   - No-result memo: BackfillArtistLocations keys only on an empty city, so the
//     locationless long tail (no MB/Bandcamp match) would be re-queried every cycle.
//     ReattemptWindow filters on location_enrich_attempted_at and the resolver stamps
//     it per batch, so a bounded nightly batch converges instead of re-hammering MB.
//
// Default OFF (ENABLE_ARTIST_LOCATION_SWEEP=1), unlike the default-on background
// workers: the resolver AUTO-WRITES locations from a name match, and the manual cmd's
// dry-run review is the documented homonym backstop — so opt-in-per-environment
// (enable on stage first, watch the report, then prod) is deliberate, matching the
// sibling image sweep rather than the DISABLE_* workers. Links enrichment is a
// separate follow-up; this sweep is location-only.
const (
	defaultArtistLocationSweepInterval  = 24 * time.Hour
	defaultArtistLocationSweepBatch     = 50
	defaultArtistLocationSweepReattempt = 30 * 24 * time.Hour
)

// ArtistLocationSweep is a background ticker service (mirrors EnrichmentWorker /
// ImageEnrichmentSweep) that fills missing artist locations via the shared resolver.
type ArtistLocationSweep struct {
	db       *gorm.DB
	bandcamp BandcampLocationResolver
	mb       MBCandidateSearcher

	interval  time.Duration
	batch     int
	reattempt time.Duration

	stopCh chan struct{}
	wg     sync.WaitGroup
	logger *slog.Logger
}

// NewArtistLocationSweep constructs the sweep. mb MUST be the shared process-wide
// MusicBrainz client (PSY-1208); bandcamp is a profile-location resolver.
func NewArtistLocationSweep(database *gorm.DB, bandcamp BandcampLocationResolver, mb MBCandidateSearcher) *ArtistLocationSweep {
	if database == nil {
		database = db.GetDB()
	}
	return &ArtistLocationSweep{
		db:        database,
		bandcamp:  bandcamp,
		mb:        mb,
		interval:  sweepEnvDuration("ARTIST_LOCATION_SWEEP_INTERVAL_HOURS", time.Hour, defaultArtistLocationSweepInterval),
		batch:     sweepEnvInt("ARTIST_LOCATION_SWEEP_BATCH", defaultArtistLocationSweepBatch),
		reattempt: sweepEnvDuration("ARTIST_LOCATION_SWEEP_REATTEMPT_DAYS", 24*time.Hour, defaultArtistLocationSweepReattempt),
		stopCh:    make(chan struct{}),
		logger:    slog.Default(),
	}
}

// Start begins the background sweep. No startup cycle (runImmediately=false): a server
// restart shouldn't kick off provider traffic; the first sweep fires one interval in.
func (s *ArtistLocationSweep) Start(ctx context.Context) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		shared.RunTickerLoop(ctx, "artist_location_sweep", s.interval, s.stopCh, false, s.runCycle)
	}()
	s.logger.Info("artist location sweep started",
		"interval", s.interval, "batch", s.batch, "reattempt", s.reattempt)
}

// Stop gracefully stops the sweep.
func (s *ArtistLocationSweep) Stop() {
	close(s.stopCh)
	s.wg.Wait()
	s.logger.Info("artist location sweep stopped")
}

// RunSweepNow runs one cycle immediately (tests / manual trigger).
func (s *ArtistLocationSweep) RunSweepNow(ctx context.Context) { s.runCycle(ctx) }

// runCycle resolves one bounded, memo-filtered batch. It calls the unexported core
// directly (not the cmd's BackfillArtistLocations) so the sweep's ctx propagates —
// a shutdown cancels mid-batch instead of finishing ~1s/artist of MB calls.
func (s *ArtistLocationSweep) runCycle(ctx context.Context) {
	report, err := backfillArtistLocations(ctx, &gormArtistStore{db: s.db}, s.bandcamp, s.mb, Options{
		Limit:           s.batch,
		ReattemptWindow: s.reattempt,
	})
	if err != nil {
		s.logger.Error("artist location sweep: cycle failed", "error", err)
		return
	}
	if report.ArtistsScanned == 0 {
		return
	}
	s.logger.Info("artist location sweep batch done",
		"scanned", report.ArtistsScanned,
		"filled_musicbrainz", report.FilledMusicBrainz,
		"filled_bandcamp", report.FilledBandcamp,
		"resolved_no_fill", report.ResolvedNoFill,
		"stamped_mbid", report.StampedMBID,
		"conflicts", len(report.Conflicts),
		"missed", report.Missed,
		"errors", len(report.Errors),
	)
}

// --- env helpers ----------------------------------------------------------

func sweepEnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

func sweepEnvDuration(key string, unit, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * unit
		}
	}
	return def
}
