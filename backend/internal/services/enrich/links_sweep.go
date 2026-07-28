package enrich

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/services/shared"
)

// Ongoing artist-LINKS sweep (PSY-1279) — Phase A of the links enrichment rollout.
// A slow background ticker runs MBID-keyed url-rel lookup (BackfillArtistLinks) over
// a bounded batch of artists missing spotify/bandcamp/website, auto-applying
// host-anchored fills through ArtistService.UpdateArtist.
//
// AUTO-APPLY (not artist_link_suggestions): the persisted MBID is the identity signal;
// the name-search discovery + admin review queue (PSY-1190–1208, sweep-link-suggestions
// cmd) remains for MBID-less artists. This sweep is the durable backstop once an MBID
// exists — same posture as location/discography Phase-A sweeps.
//
// SHARED MusicBrainz client (PSY-1208) + no-result memo (links_enrich_attempted_at).
// Default OFF (ENABLE_ARTIST_LINKS_SWEEP=1).
const (
	defaultArtistLinksSweepInterval  = 24 * time.Hour
	defaultArtistLinksSweepBatch     = 50
	defaultArtistLinksSweepReattempt = 90 * 24 * time.Hour
)

// ArtistLinksSweep is a background ticker service that fills missing music links.
type ArtistLinksSweep struct {
	db     *gorm.DB
	mb     MBURLRelLookup
	writer linksWriter

	interval  time.Duration
	batch     int
	reattempt time.Duration

	stopCh chan struct{}
	wg     sync.WaitGroup
	logger *slog.Logger
}

// NewArtistLinksSweep constructs the sweep. mb MUST be the shared process-wide
// MusicBrainz client (PSY-1208); writer is the catalog ArtistService (UpdateArtist).
func NewArtistLinksSweep(database *gorm.DB, mb MBURLRelLookup, writer linksWriter) *ArtistLinksSweep {
	if database == nil {
		database = db.GetDB()
	}
	return &ArtistLinksSweep{
		db:        database,
		mb:        mb,
		writer:    writer,
		interval:  shared.EnvPositiveDuration("ARTIST_LINKS_SWEEP_INTERVAL_HOURS", time.Hour, defaultArtistLinksSweepInterval),
		batch:     shared.EnvPositiveInt("ARTIST_LINKS_SWEEP_BATCH", defaultArtistLinksSweepBatch),
		reattempt: shared.EnvPositiveDuration("ARTIST_LINKS_SWEEP_REATTEMPT_DAYS", 24*time.Hour, defaultArtistLinksSweepReattempt),
		stopCh:    make(chan struct{}),
		logger:    slog.Default(),
	}
}

// Start begins the background sweep. No cycle on the boot path; the first sweep is
// scheduled from persisted run state rather than process start.
func (s *ArtistLinksSweep) Start(ctx context.Context) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		shared.RunTickerLoop(ctx, shared.LoopConfig{
			Name:     "artist_links_sweep",
			Interval: s.interval,
			StopCh:   s.stopCh,
		}, s.runCycle)
	}()
	s.logger.Info("artist links sweep started",
		"interval", s.interval, "batch", s.batch, "reattempt", s.reattempt)
}

// Stop gracefully stops the sweep.
func (s *ArtistLinksSweep) Stop() {
	close(s.stopCh)
	s.wg.Wait()
	s.logger.Info("artist links sweep stopped")
}

// RunSweepNow runs one cycle immediately (tests / manual trigger).
func (s *ArtistLinksSweep) RunSweepNow(ctx context.Context) { s.runCycle(ctx) }

func (s *ArtistLinksSweep) runCycle(ctx context.Context) {
	report, err := backfillArtistLinks(ctx, &gormArtistStore{db: s.db}, s.mb, s.writer, LinksOptions{
		Limit:           s.batch,
		ReattemptWindow: s.reattempt,
	})
	if err != nil {
		shared.ReportCycle(ctx, 0, err)
		s.logger.Error("artist links sweep: cycle failed", "error", err)
		return
	}
	shared.ReportCycle(ctx, report.ArtistsScanned, nil)
	if report.ArtistsScanned == 0 {
		return
	}
	s.logger.Info("artist links sweep batch done",
		"scanned", report.ArtistsScanned,
		"filled_spotify", report.FilledSpotify,
		"filled_bandcamp", report.FilledBandcamp,
		"filled_website", report.FilledWebsite,
		"no_links", report.ArtistsNoLinks,
		"errors", len(report.Errors),
	)
}
