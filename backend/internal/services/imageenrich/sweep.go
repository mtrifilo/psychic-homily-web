// Package imageenrich hosts the ongoing image-enrichment subsystem. It sits above
// both catalog (the shipped fill-when-empty enrichers + provider clients) and
// pipeline (the shared MusicBrainz client), so it depends on both without making
// either depend on the other — keeping catalog free of a pipeline import (which
// would cycle with pipeline's catalog-importing tests). Only the service container
// depends on this package.
//
// Three pieces, two triggers, one engine:
//   - Enricher (enricher.go) — the shared engine: runs the provider lookups + owns
//     the ONE MusicBrainz client (PSY-1208). Both triggers hold the same instance.
//   - ImageEnrichmentSweep (this file) — Phase-A trigger: a slow background ticker
//     that sweeps still-imageless entities (backfill + safety net, PSY-1246).
//   - ImageEnrichOutboxPoller (outbox.go) — Phase-B trigger: drains the on-create
//     transactional outbox for prompt enrichment (PSY-1247).
package imageenrich

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

// Ongoing image-enrichment sweep (PSY-1246) — the Phase-A "safety net" of the
// hybrid trigger model decided in PSY-1245. A slow background ticker runs the shared
// Enricher over entities that still have no image, so coverage stays current as the
// catalog grows regardless of how an entity was added. (Phase B — prompt on-create
// enqueue — is PSY-1247, the outbox poller.)
//
// It does NOT change what users see: enrichment only populates data; prod display
// stays gated on PSY-1242.
//
// Two design points carry the weight:
//
//   - SHARED MusicBrainz client (PSY-1208), owned by the injected Enricher. MB
//     blocks for exceeding ~1 req/s/IP, so all MB traffic (sweep + outbox +
//     discovery) MUST go through one mutex-serialized throttle — a second client
//     would double the rate and trip MB's sticky 503 penalty.
//   - No-result memo. Fill-when-empty keys only on an empty image column, so the
//     large imageless long tail (no provider match) would be re-queried every cycle.
//     selectBatch filters on image_enrich_attempted_at and the sweep stamps it (via
//     the Enricher) per batch, so a bounded batch converges instead of re-hammering
//     the providers.
const (
	defaultImageEnrichSweepInterval = 24 * time.Hour
	defaultImageEnrichSweepBatch    = 50
	defaultImageEnrichReattempt     = 90 * 24 * time.Hour
)

// ImageEnrichmentSweep is a background ticker service (mirrors CleanupService /
// EnrichmentWorker) that fills missing artist photos + release covers via the
// shared Enricher.
type ImageEnrichmentSweep struct {
	enricher *Enricher
	db       *gorm.DB // for selectBatch (the Enricher owns the writes)

	interval  time.Duration
	batch     int
	reattempt time.Duration

	stopCh chan struct{}
	wg     sync.WaitGroup
	logger *slog.Logger
}

// NewImageEnrichmentSweep constructs the sweep around the shared Enricher (so its
// MB traffic stays under the one PSY-1208 throttle).
func NewImageEnrichmentSweep(database *gorm.DB, enricher *Enricher) *ImageEnrichmentSweep {
	if database == nil {
		database = db.GetDB()
	}
	return &ImageEnrichmentSweep{
		enricher:  enricher,
		db:        database,
		interval:  sweepEnvDuration("IMAGE_ENRICH_SWEEP_INTERVAL_HOURS", time.Hour, defaultImageEnrichSweepInterval),
		batch:     sweepEnvInt("IMAGE_ENRICH_SWEEP_BATCH", defaultImageEnrichSweepBatch),
		reattempt: sweepEnvDuration("IMAGE_ENRICH_SWEEP_REATTEMPT_DAYS", 24*time.Hour, defaultImageEnrichReattempt),
		stopCh:    make(chan struct{}),
		logger:    slog.Default(),
	}
}

// Start begins the background sweep. No startup cycle (runImmediately=false): a
// server restart shouldn't kick off provider traffic; the first sweep fires one
// interval in. Mirrors EnrichmentWorker.
func (s *ImageEnrichmentSweep) Start(ctx context.Context) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		shared.RunTickerLoop(ctx, "image_enrich_sweep", s.interval, s.stopCh, false, s.runCycle)
	}()
	s.logger.Info("image enrichment sweep started",
		"interval", s.interval, "batch", s.batch, "reattempt", s.reattempt)
}

// Stop gracefully stops the sweep.
func (s *ImageEnrichmentSweep) Stop() {
	close(s.stopCh)
	s.wg.Wait()
	s.logger.Info("image enrichment sweep stopped")
}

// RunSweepNow runs one cycle immediately (tests / manual trigger).
func (s *ImageEnrichmentSweep) RunSweepNow(ctx context.Context) {
	s.runCycle(ctx)
}

// runCycle sweeps photos then covers sequentially, so the two share the MB throttle
// without overlapping.
func (s *ImageEnrichmentSweep) runCycle(ctx context.Context) {
	s.sweep(ctx, "artists", s.enricher.EnrichPhotos)
	if ctx.Err() != nil {
		return
	}
	s.sweep(ctx, "releases", s.enricher.EnrichCovers)
}

// sweep selects a bounded, memo-filtered batch of image-less entities from `table`,
// stamps their attempt timestamp, then runs `enrich` over those ids.
//
// Stamp-before-enrich is deliberate: a row is marked attempted even if the enrich
// step errors, so a poison row can't wedge the sweep (it waits one re-attempt window
// before a retry). Hard infra failures are rare and the window retries them.
func (s *ImageEnrichmentSweep) sweep(ctx context.Context, table string, enrich func(context.Context, []uint) error) {
	ids, err := s.selectBatch(ctx, table)
	if err != nil {
		s.logger.Error("image-enrich sweep: select failed", "table", table, "error", err)
		return
	}
	if len(ids) == 0 {
		return
	}

	if err := s.enricher.stampAttempted(ctx, table, ids); err != nil {
		s.logger.Error("image-enrich sweep: stamp failed", "table", table, "error", err)
		return
	}

	if err := enrich(ctx, ids); err != nil {
		s.logger.Error("image-enrich sweep: enrich failed", "table", table, "count", len(ids), "error", err)
		return
	}
	s.logger.Info("image-enrich sweep batch done", "table", table, "count", len(ids))
}

// imageColumn is the reference column whose emptiness marks an entity as needing
// enrichment, per table.
func imageColumn(table string) string {
	if table == "releases" {
		return "cover_art_url"
	}
	return "image_url"
}

// selectBatch returns up to `batch` entity ids that have no image and were not
// attempted within the re-attempt window, oldest-attempt first (NULLs — never tried
// — first) so brand-new and stalest rows are picked before recently re-checked ones.
func (s *ImageEnrichmentSweep) selectBatch(ctx context.Context, table string) ([]uint, error) {
	cutoff := time.Now().Add(-s.reattempt)
	col := imageColumn(table)
	var ids []uint
	err := s.db.WithContext(ctx).
		Table(table).
		// Explicit parens keep the OR-group local: this is the one query where an
		// OR clause is AND-combined with a second OR clause, so the grouping is
		// load-bearing (don't rely on GORM's per-Where auto-parenthesization alone).
		Where("("+col+" IS NULL OR "+col+" = '')").
		Where("image_enrich_attempted_at IS NULL OR image_enrich_attempted_at < ?", cutoff).
		Order("image_enrich_attempted_at ASC NULLS FIRST").
		Order("id ASC").
		Limit(s.batch).
		Pluck("id", &ids).Error
	return ids, err
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
