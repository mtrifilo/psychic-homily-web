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

// Ongoing RELEASE-links sweep (PSY-1316) — Phase A of the release-links rollout.
// A slow background ticker runs the RG-MBID-keyed url-rel backfill
// (BackfillReleaseLinks) over a bounded batch of releases missing a
// bandcamp/spotify link, auto-applying host-anchored fills through
// ReleaseService.AddExternalLinkWithSource (source=mb_backfill, so enrichment
// rows stay auditable and the partial unique index closes the concurrent-run
// duplicate race).
//
// AUTO-APPLY posture inherited from PSY-1307: the identity chain is MBID-keyed
// end-to-end (artist MBID → RG-MBID → release url-rels, no name search) and only
// Official/status-less MB releases may source a link.
//
// SHARED MusicBrainz client (PSY-1208) + no-result memo
// (releases.links_enrich_attempted_at). Default OFF (ENABLE_RELEASE_LINKS_SWEEP=1).
const (
	defaultReleaseLinksSweepInterval  = 24 * time.Hour
	defaultReleaseLinksSweepBatch     = 25
	defaultReleaseLinksSweepReattempt = 90 * 24 * time.Hour
)

// ReleaseLinksSweep is a background ticker service that fills missing release
// platform links.
type ReleaseLinksSweep struct {
	db     *gorm.DB
	mb     MBReleaseURLRelBrowse
	writer releaseLinkWriter

	interval  time.Duration
	batch     int
	reattempt time.Duration

	stopCh chan struct{}
	wg     sync.WaitGroup
	logger *slog.Logger
}

// NewReleaseLinksSweep constructs the sweep. mb MUST be the shared process-wide
// MusicBrainz client (PSY-1208); writer is the catalog ReleaseService. The batch
// default is smaller than the artist sweeps' 50: each candidate costs a browse of
// its whole release-group (up to 10 paginated MB calls), not one lookup.
func NewReleaseLinksSweep(database *gorm.DB, mb MBReleaseURLRelBrowse, writer releaseLinkWriter) *ReleaseLinksSweep {
	if database == nil {
		database = db.GetDB()
	}
	return &ReleaseLinksSweep{
		db:        database,
		mb:        mb,
		writer:    writer,
		interval:  shared.EnvPositiveDuration("RELEASE_LINKS_SWEEP_INTERVAL_HOURS", time.Hour, defaultReleaseLinksSweepInterval),
		batch:     shared.EnvPositiveInt("RELEASE_LINKS_SWEEP_BATCH", defaultReleaseLinksSweepBatch),
		reattempt: shared.EnvPositiveDuration("RELEASE_LINKS_SWEEP_REATTEMPT_DAYS", 24*time.Hour, defaultReleaseLinksSweepReattempt),
		stopCh:    make(chan struct{}),
		logger:    slog.Default(),
	}
}

// Start begins the background sweep. No startup cycle (runImmediately=false).
func (s *ReleaseLinksSweep) Start(ctx context.Context) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		shared.RunTickerLoop(ctx, "release_links_sweep", s.interval, s.stopCh, false, s.runCycle)
	}()
	s.logger.Info("release links sweep started",
		"interval", s.interval, "batch", s.batch, "reattempt", s.reattempt)
}

// Stop gracefully stops the sweep.
func (s *ReleaseLinksSweep) Stop() {
	close(s.stopCh)
	s.wg.Wait()
	s.logger.Info("release links sweep stopped")
}

// RunSweepNow runs one cycle immediately (tests / manual trigger).
func (s *ReleaseLinksSweep) RunSweepNow(ctx context.Context) { s.runCycle(ctx) }

func (s *ReleaseLinksSweep) runCycle(ctx context.Context) {
	report, err := backfillReleaseLinks(ctx, &gormReleaseLinkStore{db: s.db}, s.mb, s.writer, ReleaseLinksOptions{
		Limit:           s.batch,
		ReattemptWindow: s.reattempt,
	})
	if err != nil {
		s.logger.Error("release links sweep: cycle failed", "error", err)
		return
	}
	if report.ReleasesScanned == 0 {
		return
	}
	s.logger.Info("release links sweep batch done",
		"scanned", report.ReleasesScanned,
		"rgs_browsed", report.RGsBrowsed,
		"filled_bandcamp", report.FilledBandcamp,
		"filled_spotify", report.FilledSpotify,
		"no_links", report.ReleasesNoLinks,
		"raced", report.LinksRaced,
		"errors", len(report.Errors),
	)
}
