package discography

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/services/shared"
)

// Ongoing artist-DISCOGRAPHY sweep (PSY-1291) — Phase A of the discography rollout.
// A slow background ticker runs the MBID-keyed importer (BackfillArtistDiscography:
// MusicBrainz release-group browse + Cover Art Archive) over a bounded batch of
// MBID-bearing artists, so discography coverage stays current as the catalog grows.
//
// Two design points carry the weight (both mirror the location sweep, PSY-1250):
//
//   - SHARED MusicBrainz client (PSY-1208): the importer's MB browse MUST go through
//     the one process-wide ~1 req/s throttle — a second client would double the rate
//     and trip MB's sticky 503. The container passes the shared *pipeline client.
//   - Sync memo: without discography_synced_at the whole MBID catalog would be re-browsed
//     every cycle. ReattemptWindow filters on discography_synced_at and the importer
//     stamps it per batch, so a bounded nightly batch converges instead of re-hammering MB.
//
// Default OFF (ENABLE_ARTIST_DISCOGRAPHY_SWEEP=1), unlike the default-on background
// workers: releases are the highest flood-risk enrichment, so opt-in-per-environment
// (enable on stage first, watch the report, then prod) is deliberate, matching the
// sibling location + image sweeps rather than the DISABLE_* workers. NB the same flag
// is the discography-enrichment FEATURE switch: =1 also turns on PSY-1292's eager
// on-MBID-stamp import (fire-and-forget), wired in the service container.
//
// Operator note: keep REATTEMPT_DAYS comfortably larger than INTERVAL_HOURS × the number
// of ticks it takes to walk the MBID tail (tail size / batch) — a re-attempt window
// shorter than that defeats the memo and re-browses the whole catalog each pass (the MB
// throttle still caps the absolute rate, but it wastes cycles).
const (
	defaultArtistDiscographySweepInterval  = 24 * time.Hour
	defaultArtistDiscographySweepBatch      = 25 // heavier per-artist than location (50): each artist is a browse + N cover-art fetches
	defaultArtistDiscographySweepReattempt  = 90 * 24 * time.Hour
)

// ArtistDiscographySweep is a background ticker service that imports primary discography
// for MBID-bearing artists via the shared importer.
type ArtistDiscographySweep struct {
	db       *gorm.DB
	browser  ReleaseGroupBrowser
	coverart CoverArtFetcher

	interval  time.Duration
	batch     int
	reattempt time.Duration

	stopCh chan struct{}
	wg     sync.WaitGroup
	logger *slog.Logger
}

// NewArtistDiscographySweep constructs the sweep. browser MUST be the shared process-wide
// MusicBrainz client (PSY-1208); coverart is the Cover Art Archive client.
func NewArtistDiscographySweep(database *gorm.DB, browser ReleaseGroupBrowser, coverart CoverArtFetcher) *ArtistDiscographySweep {
	if database == nil {
		database = db.GetDB()
	}
	return &ArtistDiscographySweep{
		db:        database,
		browser:   browser,
		coverart:  coverart,
		interval:  shared.EnvPositiveDuration("ARTIST_DISCOGRAPHY_SWEEP_INTERVAL_HOURS", time.Hour, defaultArtistDiscographySweepInterval),
		batch:     shared.EnvPositiveInt("ARTIST_DISCOGRAPHY_SWEEP_BATCH", defaultArtistDiscographySweepBatch),
		reattempt: shared.EnvPositiveDuration("ARTIST_DISCOGRAPHY_SWEEP_REATTEMPT_DAYS", 24*time.Hour, defaultArtistDiscographySweepReattempt),
		stopCh:    make(chan struct{}),
		logger:    slog.Default(),
	}
}

// Start begins the background sweep. No startup cycle (runImmediately=false): a server
// restart shouldn't kick off provider traffic; the first sweep fires one interval in.
func (s *ArtistDiscographySweep) Start(ctx context.Context) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		shared.RunTickerLoop(ctx, "artist_discography_sweep", s.interval, s.stopCh, false, s.runCycle)
	}()
	s.logger.Info("artist discography sweep started",
		"interval", s.interval, "batch", s.batch, "reattempt", s.reattempt)
}

// Stop gracefully stops the sweep.
func (s *ArtistDiscographySweep) Stop() {
	close(s.stopCh)
	s.wg.Wait()
	s.logger.Info("artist discography sweep stopped")
}

// RunSweepNow runs one cycle immediately (tests / manual trigger).
func (s *ArtistDiscographySweep) RunSweepNow(ctx context.Context) { s.runCycle(ctx) }

// runCycle imports one bounded, memo-filtered batch. It calls the ctx-aware core directly
// (not the cmd's BackfillArtistDiscography) so a shutdown cancels mid-batch instead of
// finishing ~1s/artist of MB calls.
func (s *ArtistDiscographySweep) runCycle(ctx context.Context) {
	report, err := backfillArtistDiscography(ctx, s.db, s.browser, s.coverart, Options{
		Limit:           s.batch,
		ReattemptWindow: s.reattempt,
	})
	if err != nil {
		s.logger.Error("artist discography sweep: cycle failed", "error", err)
		return
	}
	if report.ArtistsScanned == 0 {
		return
	}
	s.logger.Info("artist discography sweep batch done",
		"scanned", report.ArtistsScanned,
		"release_groups_seen", report.ReleaseGroupsSeen,
		"created", report.Created,
		"deduped", report.Deduped,
		"cover_art_set", report.CoverArtSet,
		"artists_no_releases", report.ArtistsNoReleases,
		"errors", len(report.Errors),
	)
}
