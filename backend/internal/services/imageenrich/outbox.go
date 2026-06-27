package imageenrich

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"psychic-homily-backend/db"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/shared"
)

const (
	defaultOutboxInterval     = 60 * time.Second
	defaultOutboxBatch        = 20
	defaultOutboxStaleReclaim = 15 * time.Minute
)

// ImageEnrichOutboxPoller drains the image_enrich_queue transactional outbox
// (PSY-1247) — the PROMPT, on-create enrichment trigger (Phase B of PSY-1245).
// The catalog create funnel enqueues a job row in the same tx as a new
// artist/release; this poller claims pending rows and runs the same
// fill-when-empty enrichers the sweep uses, so a new entity gets its image within
// ~one poll interval instead of waiting for the slow daily Phase-A sweep.
//
// It shares the ImageEnrichmentSweep as its enrichment ENGINE: the sweep owns the
// per-entity enrichers (runPhotoEnricher / runCoverEnricher), the attempted_at
// memo stamping, and — critically — the ONE process-wide MusicBrainz client
// (PSY-1208). Reusing it keeps ALL MB traffic (sweep + outbox + discovery) under a
// single mutex-serialized ~1 req/s throttle; a second client would double the rate
// and trip MB's sticky 503 penalty. The sweep's ticker is the backfill trigger;
// this poller is the prompt trigger; both drive the same engine, and run safely
// concurrently because the shared MB mutex serializes their lookups.
//
// Claiming uses SELECT ... FOR UPDATE SKIP LOCKED so multiple server instances can
// poll the same queue without double-processing a row (unlike the older
// admin.EnrichmentQueueItem claim). Rows are flipped to `processing` under that
// lock, then enriched OUTSIDE the lock — the enrichers do slow network I/O, and
// holding row locks across MB calls would be a long-transaction antipattern. A
// crash mid-process strands a `processing` row; reclaimStale returns rows stuck
// past a window to `pending` so they retry (still bounded by max_attempts).
type ImageEnrichOutboxPoller struct {
	db     *gorm.DB
	engine *ImageEnrichmentSweep // shared enrichers + MB client (PSY-1208); see type doc

	interval     time.Duration
	batch        int
	staleReclaim time.Duration

	stopCh chan struct{}
	wg     sync.WaitGroup
	logger *slog.Logger
}

// NewImageEnrichOutboxPoller constructs the poller. engine MUST be the same
// ImageEnrichmentSweep wired in the container so the shared MB client is reused.
func NewImageEnrichOutboxPoller(database *gorm.DB, engine *ImageEnrichmentSweep) *ImageEnrichOutboxPoller {
	if database == nil {
		database = db.GetDB()
	}
	return &ImageEnrichOutboxPoller{
		db:           database,
		engine:       engine,
		interval:     sweepEnvDuration("IMAGE_ENRICH_OUTBOX_INTERVAL_SECONDS", time.Second, defaultOutboxInterval),
		batch:        sweepEnvInt("IMAGE_ENRICH_OUTBOX_BATCH", defaultOutboxBatch),
		staleReclaim: sweepEnvDuration("IMAGE_ENRICH_OUTBOX_STALE_RECLAIM_MINUTES", time.Minute, defaultOutboxStaleReclaim),
		stopCh:       make(chan struct{}),
		logger:       slog.Default(),
	}
}

// Start begins the background poller. No startup cycle (mirrors the sweep /
// EnrichmentWorker): the first tick fires one interval in.
func (p *ImageEnrichOutboxPoller) Start(ctx context.Context) {
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		shared.RunTickerLoop(ctx, "image_enrich_outbox", p.interval, p.stopCh, false, p.processTick)
	}()
	p.logger.Info("image enrichment outbox poller started",
		"interval", p.interval, "batch", p.batch, "stale_reclaim", p.staleReclaim)
}

// Stop gracefully stops the poller.
func (p *ImageEnrichOutboxPoller) Stop() {
	close(p.stopCh)
	p.wg.Wait()
	p.logger.Info("image enrichment outbox poller stopped")
}

// RunNow runs one cycle immediately (tests / manual trigger).
func (p *ImageEnrichOutboxPoller) RunNow(ctx context.Context) { p.processTick(ctx) }

// processTick reclaims stranded rows, claims a pending batch, and runs each
// entity type's enricher, finalizing the claimed job rows by outcome.
func (p *ImageEnrichOutboxPoller) processTick(ctx context.Context) {
	p.reclaimStale(ctx)

	items, err := p.claimBatch(ctx)
	if err != nil {
		p.logger.Error("image-enrich outbox: claim failed", "error", err)
		return
	}
	if len(items) == 0 {
		return
	}

	// Partition the claimed batch by entity type so each goes to its enricher.
	var artistIDs, releaseIDs []uint
	var artistItems, releaseItems []catalogm.ImageEnrichQueueItem
	for _, it := range items {
		switch it.EntityType {
		case catalogm.ImageEnrichEntityArtist:
			artistIDs = append(artistIDs, it.EntityID)
			artistItems = append(artistItems, it)
		case catalogm.ImageEnrichEntityRelease:
			releaseIDs = append(releaseIDs, it.EntityID)
			releaseItems = append(releaseItems, it)
		default:
			// Impossible given the CHECK constraint, but a stray type must not wedge
			// the queue forever — mark it done so it leaves the active set.
			p.logger.Warn("image-enrich outbox: unknown entity_type, marking done",
				"type", it.EntityType, "id", it.ID)
			p.markDone(ctx, []catalogm.ImageEnrichQueueItem{it})
		}
	}

	if len(artistIDs) > 0 {
		p.runBatch(ctx, "artists", artistIDs, artistItems, p.engine.enrichPhotos)
	}
	if ctx.Err() != nil {
		return
	}
	if len(releaseIDs) > 0 {
		p.runBatch(ctx, "releases", releaseIDs, releaseItems, p.engine.enrichCovers)
	}
}

// claimBatch atomically claims up to `batch` pending rows: it SELECTs them FOR
// UPDATE SKIP LOCKED (so concurrent pollers never grab the same row) and flips
// them to `processing`, incrementing attempts, in the same short transaction. The
// row locks release at commit; enrichment then runs outside the transaction.
//
// The returned items carry their PRE-increment attempts (scanned before the
// update), so finalize reasons about the post-increment count as item.Attempts+1.
func (p *ImageEnrichOutboxPoller) claimBatch(ctx context.Context) ([]catalogm.ImageEnrichQueueItem, error) {
	var items []catalogm.ImageEnrichQueueItem
	err := p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if ferr := tx.
			Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate, Options: clause.LockingOptionsSkipLocked}).
			Where("status = ? AND attempts < max_attempts", catalogm.ImageEnrichStatusPending).
			Order("created_at ASC").
			Limit(p.batch).
			Find(&items).Error; ferr != nil {
			return ferr
		}
		if len(items) == 0 {
			return nil
		}
		return tx.Model(&catalogm.ImageEnrichQueueItem{}).
			Where("id IN ?", itemIDs(items)).
			Updates(map[string]interface{}{
				"status":   catalogm.ImageEnrichStatusProcessing,
				"attempts": gorm.Expr("attempts + 1"),
			}).Error
	})
	return items, err
}

// runBatch stamps the entity-level attempt memo (sweep coexistence), runs the
// enricher over the ids, then finalizes the claimed job rows: done on success,
// requeue/fail on error.
func (p *ImageEnrichOutboxPoller) runBatch(
	ctx context.Context,
	table string,
	ids []uint,
	items []catalogm.ImageEnrichQueueItem,
	enrich func(context.Context, []uint) error,
) {
	// Stamp image_enrich_attempted_at so the Phase-A sweep won't re-attempt an
	// entity the outbox just handled (idempotent coexistence). Best-effort.
	if err := p.engine.stampAttempted(ctx, table, ids); err != nil {
		p.logger.Warn("image-enrich outbox: stamp attempted failed", "table", table, "error", err)
	}

	if err := enrich(ctx, ids); err != nil {
		p.markFailedOrRetry(ctx, items, err)
		p.logger.Warn("image-enrich outbox: enrich failed", "table", table, "count", len(ids), "error", err)
		return
	}
	p.markDone(ctx, items)
	p.logger.Info("image-enrich outbox batch done", "table", table, "count", len(ids))
}

// markDone marks claimed rows done and stamps processed_at. A no-provider-match is
// still "done" from the outbox's view — the Phase-A sweep's re-attempt window owns
// eventual retry of the imageless tail.
func (p *ImageEnrichOutboxPoller) markDone(ctx context.Context, items []catalogm.ImageEnrichQueueItem) {
	if len(items) == 0 {
		return
	}
	now := time.Now()
	if err := p.db.WithContext(ctx).Model(&catalogm.ImageEnrichQueueItem{}).
		Where("id IN ?", itemIDs(items)).
		Updates(map[string]interface{}{
			"status":       catalogm.ImageEnrichStatusDone,
			"processed_at": &now,
			"last_error":   nil,
		}).Error; err != nil {
		p.logger.Error("image-enrich outbox: mark done failed", "error", err)
	}
}

// markFailedOrRetry returns rows that still have attempts left to `pending` for a
// later tick, and marks the exhausted ones `failed`. items carry pre-increment
// attempts (claimBatch already +1'd the row), so post-increment is Attempts+1.
func (p *ImageEnrichOutboxPoller) markFailedOrRetry(ctx context.Context, items []catalogm.ImageEnrichQueueItem, cause error) {
	var retryIDs, failIDs []uint
	for _, it := range items {
		if it.Attempts+1 >= it.MaxAttempts {
			failIDs = append(failIDs, it.ID)
		} else {
			retryIDs = append(retryIDs, it.ID)
		}
	}
	errStr := cause.Error()
	if len(retryIDs) > 0 {
		if err := p.db.WithContext(ctx).Model(&catalogm.ImageEnrichQueueItem{}).
			Where("id IN ?", retryIDs).
			Updates(map[string]interface{}{
				"status":     catalogm.ImageEnrichStatusPending,
				"last_error": errStr,
			}).Error; err != nil {
			p.logger.Error("image-enrich outbox: requeue failed", "error", err)
		}
	}
	if len(failIDs) > 0 {
		if err := p.db.WithContext(ctx).Model(&catalogm.ImageEnrichQueueItem{}).
			Where("id IN ?", failIDs).
			Updates(map[string]interface{}{
				"status":     catalogm.ImageEnrichStatusFailed,
				"last_error": errStr,
			}).Error; err != nil {
			p.logger.Error("image-enrich outbox: mark failed failed", "error", err)
		}
	}
}

// reclaimStale returns rows stuck in `processing` longer than staleReclaim back to
// `pending`, recovering jobs orphaned by a worker crash. attempts is NOT reset, so
// a genuinely poison row still converges to `failed` after max_attempts.
func (p *ImageEnrichOutboxPoller) reclaimStale(ctx context.Context) {
	cutoff := time.Now().Add(-p.staleReclaim)
	res := p.db.WithContext(ctx).Model(&catalogm.ImageEnrichQueueItem{}).
		Where("status = ? AND updated_at < ?", catalogm.ImageEnrichStatusProcessing, cutoff).
		Update("status", catalogm.ImageEnrichStatusPending)
	if res.Error != nil {
		p.logger.Error("image-enrich outbox: reclaim failed", "error", res.Error)
		return
	}
	if res.RowsAffected > 0 {
		p.logger.Warn("image-enrich outbox: reclaimed stale processing rows", "count", res.RowsAffected)
	}
}

func itemIDs(items []catalogm.ImageEnrichQueueItem) []uint {
	ids := make([]uint, len(items))
	for i, it := range items {
		ids[i] = it.ID
	}
	return ids
}
