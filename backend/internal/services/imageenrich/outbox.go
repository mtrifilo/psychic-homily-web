package imageenrich

import (
	"context"
	"errors"
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
	defaultOutboxRetention    = 7 * 24 * time.Hour
)

// ImageEnrichOutboxPoller drains the image_enrich_queue transactional outbox
// (PSY-1247) — the PROMPT, on-create enrichment trigger (Phase B of PSY-1245).
// The catalog create funnel enqueues a job row in the same tx as a new
// artist/release; this poller claims pending rows and runs the same
// fill-when-empty enrichers the sweep uses, so a new entity gets its image within
// ~one poll interval instead of waiting for the slow daily Phase-A sweep.
//
// # One-shot prompt + sweep backstop (NOT per-entity retry)
//
// The shared enrichers (BackfillCommonsPhotos / BackfillCoverArt) swallow
// per-entity provider misses into their report and return an error ONLY when the
// up-front batch DB load fails. So enrich(ctx, ids) returning nil means "the batch
// ran" — whether or not every entity got an image — and the jobs are marked done
// after ONE attempt. That is intentional: the outbox is the prompt one-shot path;
// the Phase-A sweep owns re-attempting the imageless tail on its (long) re-attempt
// window. The attempts/max_attempts retry machinery here therefore bounds the
// INFRA-error case (enrich returning an error, e.g. a DB load failure), not
// per-entity provider misses. Known limitation: an entity whose lookup hit a
// transient blip is marked done and not re-attempted by the outbox — it waits for
// the sweep's re-attempt window. Tracked as a follow-up (thread per-id outcomes);
// fixing it here by re-querying + requeuing every miss would breach the
// no-MB-budget-blowup AC (rapid retries on genuine no-matches).
//
// # Shared enrichment engine (PSY-1208)
//
// It shares the ImageEnrichmentSweep as its enrichment ENGINE: the sweep owns the
// per-entity enrichers (enrichPhotos / enrichCovers), the attempted_at memo
// stamping, and — critically — the ONE process-wide MusicBrainz client. Reusing it
// keeps ALL MB traffic (sweep + outbox + discovery) under a single
// mutex-serialized ~1 req/s throttle; a second client would double the rate and
// trip MB's sticky 503 penalty. The sweep's ticker is the backfill trigger; this
// poller is the prompt trigger; both drive the same engine, and run safely
// concurrently because the shared MB mutex serializes their lookups. (This reaches
// the sweep's unexported fields from within the package — a deliberate intra-package
// share; extracting a standalone Enricher both hold is a tracked follow-up.)
//
// # Concurrency + recovery
//
// Claiming uses SELECT ... FOR UPDATE SKIP LOCKED so multiple server instances can
// poll the same queue without double-claiming a row. Rows are flipped to
// `processing` under that lock, then enriched OUTSIDE the lock — the enrichers do
// slow network I/O, and holding row locks across MB calls would be a
// long-transaction antipattern. Two safety nets cover a crash mid-process:
//   - reclaimStale returns rows stuck in `processing` past staleReclaim to
//     `pending` (or `failed` if they have exhausted max_attempts, so they can't
//     zombie-hold the active-job slot forever).
//   - finalize writes (markDone / markFailedOrRetry / requeue) are guarded by
//     `status = 'processing'`, so a late finalize from a worker that lost the row to
//     reclaim is a no-op WHEN the row has since moved to a terminal/pending state.
//     The guard does NOT catch the ABA case — reclaim re-pends the row and another
//     worker re-claims it back to `processing` before the late write lands — there
//     the late write can still hit the row; correctness then rests on the enrichers
//     being idempotent (fill-when-empty), so whichever worker wins the entity is
//     imaged once and the row ends terminal. staleReclaim must exceed the worst-case
//     batch wall-clock to keep even that ABA case rare (batch=20 at ~1 req/s is
//     ~1-2 min, well under the 15 min default).
type ImageEnrichOutboxPoller struct {
	db     *gorm.DB
	engine *ImageEnrichmentSweep // shared enrichers + MB client (PSY-1208); see type doc

	interval     time.Duration
	batch        int
	staleReclaim time.Duration
	retention    time.Duration

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
		retention:    sweepEnvDuration("IMAGE_ENRICH_OUTBOX_RETENTION_HOURS", time.Hour, defaultOutboxRetention),
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
		"interval", p.interval, "batch", p.batch, "stale_reclaim", p.staleReclaim, "retention", p.retention)
}

// Stop gracefully stops the poller.
func (p *ImageEnrichOutboxPoller) Stop() {
	close(p.stopCh)
	p.wg.Wait()
	p.logger.Info("image enrichment outbox poller stopped")
}

// RunNow runs one cycle immediately (tests / manual trigger).
func (p *ImageEnrichOutboxPoller) RunNow(ctx context.Context) { p.processTick(ctx) }

// processTick reclaims stranded rows, prunes aged terminal rows, claims a pending
// batch, and runs each entity type's enricher, finalizing the claimed job rows.
func (p *ImageEnrichOutboxPoller) processTick(ctx context.Context) {
	p.reclaimStale(ctx)
	p.pruneTerminal(ctx)

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
			// Unreachable today: the entity_type CHECK constraint admits only
			// artist/release. Defense-in-depth for a future CHECK widening that
			// forgets to add a case here — mark FAILED (visibly stuck), never done
			// (silently discarded), so the divergence is observable.
			p.logger.Warn("image-enrich outbox: unhandled entity_type, marking failed",
				"type", it.EntityType, "id", it.ID)
			unhandled := "unhandled entity_type: " + it.EntityType
			p.finalize(ctx, []uint{it.ID}, map[string]interface{}{
				"status": catalogm.ImageEnrichStatusFailed, "last_error": unhandled,
			})
		}
	}

	// Both batches are attempted even if ctx is canceled mid-tick; runBatch detects
	// the cancellation (an enrich error, OR a swallowed mid-loop cancel via
	// ctx.Err()) and requeues those rows rather than stranding or wrongly marking
	// them done.
	if len(artistIDs) > 0 {
		p.runBatch(ctx, "artists", artistIDs, artistItems, p.engine.enrichPhotos)
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
// requeue/fail on infra error, requeue-without-burning-an-attempt on cancellation.
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

	// Finalize writes must survive a shutdown-canceled tick ctx (otherwise a claimed
	// row would be left `processing` until the next reclaim), but stay bounded so a
	// hung DB write during shutdown can't wedge Stop()/wg.Wait() — WithoutCancel
	// detaches from the tick cancel, the timeout caps it.
	fctx, cancelFinalize := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancelFinalize()

	err := enrich(ctx, ids)
	// The shared enrichers don't check ctx inside their per-entity loop: a mid-loop
	// cancellation (the common shutdown case — the lookup loop is slow, the up-front
	// DB load is fast) is swallowed into their report and they return nil. Promote a
	// nil return under a canceled ctx to a cancellation, so those rows are requeued
	// rather than marked done with no image.
	if err == nil && ctx.Err() != nil {
		err = ctx.Err()
	}
	switch {
	case err == nil:
		p.markDone(fctx, items)
		p.logger.Info("image-enrich outbox batch done", "table", table, "count", len(ids))
	case errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded):
		// Shutdown/timeout, not a provider failure — requeue WITHOUT counting it as
		// an attempt, so a deploy mid-job can't burn the row toward `failed`.
		p.requeueCanceled(fctx, items)
		p.logger.Info("image-enrich outbox batch canceled, requeued", "table", table, "count", len(ids))
	default:
		p.markFailedOrRetry(fctx, items, err)
		p.logger.Warn("image-enrich outbox: enrich failed", "table", table, "count", len(ids), "error", err)
	}
}

// markDone marks claimed rows done and stamps processed_at. A no-provider-match is
// still "done" from the outbox's view — the Phase-A sweep's re-attempt window owns
// eventual retry of the imageless tail. Writes through finalize (status='processing'
// guard; see the type doc for what that guard does and does not cover).
func (p *ImageEnrichOutboxPoller) markDone(ctx context.Context, items []catalogm.ImageEnrichQueueItem) {
	if len(items) == 0 {
		return
	}
	now := time.Now()
	p.finalize(ctx, itemIDs(items), map[string]interface{}{
		"status":       catalogm.ImageEnrichStatusDone,
		"processed_at": &now,
		"last_error":   nil,
	})
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
		p.finalize(ctx, retryIDs, map[string]interface{}{
			"status": catalogm.ImageEnrichStatusPending, "last_error": errStr,
		})
	}
	if len(failIDs) > 0 {
		p.finalize(ctx, failIDs, map[string]interface{}{
			"status": catalogm.ImageEnrichStatusFailed, "last_error": errStr,
		})
	}
}

// requeueCanceled returns canceled-mid-flight rows to `pending` and DECREMENTS
// attempts (undoing the claim-time increment) so a shutdown does not count as a
// provider attempt. Guarded by status='processing'.
func (p *ImageEnrichOutboxPoller) requeueCanceled(ctx context.Context, items []catalogm.ImageEnrichQueueItem) {
	if len(items) == 0 {
		return
	}
	p.finalize(ctx, itemIDs(items), map[string]interface{}{
		"status":   catalogm.ImageEnrichStatusPending,
		"attempts": gorm.Expr("GREATEST(attempts - 1, 0)"),
	})
}

// finalize applies a terminal/requeue update to the given job ids, guarded by
// status='processing' so a row reclaimed/finalized by another worker since this
// worker claimed it is left untouched (no lost-update / state clobber).
func (p *ImageEnrichOutboxPoller) finalize(ctx context.Context, ids []uint, updates map[string]interface{}) {
	if len(ids) == 0 {
		return
	}
	if err := p.db.WithContext(ctx).Model(&catalogm.ImageEnrichQueueItem{}).
		Where("id IN ? AND status = ?", ids, catalogm.ImageEnrichStatusProcessing).
		Updates(updates).Error; err != nil {
		p.logger.Error("image-enrich outbox: finalize failed", "error", err)
	}
}

// reclaimStale recovers rows orphaned by a worker crash. A row stuck in
// `processing` past staleReclaim is returned to `pending` if it still has attempts
// left, or marked `failed` if it has exhausted max_attempts — the latter is
// essential: leaving an exhausted row `pending` would zombie it (the claim filter
// is attempts < max_attempts, so it never re-claims, never finalizes, and forever
// holds the entity's slot in the one-active-job-per-entity unique index).
func (p *ImageEnrichOutboxPoller) reclaimStale(ctx context.Context) {
	cutoff := time.Now().Add(-p.staleReclaim)

	// Two statements, not one: their attempts predicates are disjoint AND the first
	// moves its matched rows out of `processing`, so no row is hit by both and none
	// escapes both. Do NOT merge them into one UPDATE — the split is what keeps the
	// retry/fail partition clean.
	retry := p.db.WithContext(ctx).Model(&catalogm.ImageEnrichQueueItem{}).
		Where("status = ? AND updated_at < ? AND attempts < max_attempts", catalogm.ImageEnrichStatusProcessing, cutoff).
		Update("status", catalogm.ImageEnrichStatusPending)
	if retry.Error != nil {
		p.logger.Error("image-enrich outbox: reclaim (retry) failed", "error", retry.Error)
	}

	failed := p.db.WithContext(ctx).Model(&catalogm.ImageEnrichQueueItem{}).
		Where("status = ? AND updated_at < ? AND attempts >= max_attempts", catalogm.ImageEnrichStatusProcessing, cutoff).
		Updates(map[string]interface{}{
			"status":     catalogm.ImageEnrichStatusFailed,
			"last_error": "stranded in processing after max attempts",
		})
	if failed.Error != nil {
		p.logger.Error("image-enrich outbox: reclaim (fail) failed", "error", failed.Error)
	}

	if n := retry.RowsAffected + failed.RowsAffected; n > 0 {
		p.logger.Warn("image-enrich outbox: reclaimed stale processing rows",
			"requeued", retry.RowsAffected, "failed", failed.RowsAffected)
	}
}

// pruneTerminal deletes done/failed rows older than the retention window, keeping a
// short audit trail while bounding table growth (resolves the mark-done + periodic
// prune decision in PSY-1247). The active-state partial indexes don't cover this
// scan, but the terminal-row set stays small precisely because this prunes it.
func (p *ImageEnrichOutboxPoller) pruneTerminal(ctx context.Context) {
	cutoff := time.Now().Add(-p.retention)
	res := p.db.WithContext(ctx).
		Where("status IN ? AND updated_at < ?",
			[]string{catalogm.ImageEnrichStatusDone, catalogm.ImageEnrichStatusFailed}, cutoff).
		Delete(&catalogm.ImageEnrichQueueItem{})
	if res.Error != nil {
		p.logger.Error("image-enrich outbox: prune failed", "error", res.Error)
		return
	}
	if res.RowsAffected > 0 {
		p.logger.Info("image-enrich outbox: pruned terminal rows", "count", res.RowsAffected)
	}
}

func itemIDs(items []catalogm.ImageEnrichQueueItem) []uint {
	ids := make([]uint, len(items))
	for i, it := range items {
		ids[i] = it.ID
	}
	return ids
}
