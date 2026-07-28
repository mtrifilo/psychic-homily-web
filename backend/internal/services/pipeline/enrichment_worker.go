package pipeline

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"psychic-homily-backend/internal/services/shared"
)

const (
	// DefaultEnrichmentInterval is how often the worker processes the queue.
	DefaultEnrichmentInterval = 30 * time.Second
	// DefaultEnrichmentBatchSize is how many items to process per tick.
	DefaultEnrichmentBatchSize = 10

	// enrichmentStaleReclaim is how long a row may sit in `processing` before
	// ReclaimStale treats it as orphaned (PSY-1572).
	//
	// Deliberately generous, and NOT yet measured for this queue — say so rather
	// than imply otherwise. The error is asymmetric: set it too SHORT and reclaim
	// fires on work that is still running, burning an attempt on a row that is
	// succeeding (exactly the PSY-1569 failure mode), while set too LONG it only
	// delays recovery of genuinely crashed rows. So it errs long. For calibration,
	// the image-enrich outbox measured 9-14 minutes for a 20-item batch against a
	// 15-minute window; this queue's batch is 10 but EnrichShow's per-item cost is
	// unmeasured. Measure a real batch before tightening this.
	enrichmentStaleReclaim = 30 * time.Minute

	// enrichmentStrandedError marks rows reclaim gave up on, distinguishing "the
	// machinery lost this row" from a genuine enrichment failure. The image-enrich
	// queue's equivalent sentinel is what made the PSY-1569 recovery query possible.
	enrichmentStrandedError = "stranded in processing after max attempts"
)

// EnrichmentWorker is a background service that processes the enrichment queue.
// It follows the same Start/Stop pattern as CleanupService and the other
// background ticker services.
type EnrichmentWorker struct {
	enrichmentService *EnrichmentService
	interval          time.Duration
	batchSize         int
	stopCh            chan struct{}
	wg                sync.WaitGroup
	logger            *slog.Logger
}

// NewEnrichmentWorker creates a new enrichment background worker.
func NewEnrichmentWorker(enrichmentService *EnrichmentService) *EnrichmentWorker {
	return &EnrichmentWorker{
		enrichmentService: enrichmentService,
		interval:          DefaultEnrichmentInterval,
		batchSize:         DefaultEnrichmentBatchSize,
		stopCh:            make(chan struct{}),
		logger:            slog.Default(),
	}
}

// Start begins the background enrichment worker.
func (w *EnrichmentWorker) Start(ctx context.Context) {
	w.wg.Add(1)
	go w.run(ctx)
	w.logger.Info("enrichment worker started",
		"interval", w.interval,
		"batch_size", w.batchSize,
	)
}

// Stop gracefully stops the enrichment worker.
func (w *EnrichmentWorker) Stop() {
	close(w.stopCh)
	w.wg.Wait()
	w.logger.Info("enrichment worker stopped")
}

// run is the main loop for the enrichment worker.
// No boot cycle. The interval is well under an hour, so the first cycle is
// bounded by StartDelay's cap at the interval — one interval, same as before —
// and the loop is below the run-state persistence threshold: a row
// write every 30s would buy nothing a drained queue doesn't already prove.
func (w *EnrichmentWorker) run(ctx context.Context) {
	defer w.wg.Done()
	shared.RunTickerLoop(ctx, shared.LoopConfig{
		Name:     "enrichment_worker",
		Interval: w.interval,
		StopCh:   w.stopCh,
	}, func(c context.Context) {
		w.processTick(c)
	})
}

// processTick processes a batch of enrichment items.
func (w *EnrichmentWorker) processTick(ctx context.Context) {
	processed, err := w.enrichmentService.ProcessQueue(ctx, w.batchSize)
	if err != nil {
		w.logger.Error("enrichment queue processing failed",
			"error", err,
		)
		return
	}

	if processed > 0 {
		w.logger.Info("enrichment tick completed",
			"items_processed", processed,
		)
	}
}

// RunNow triggers an immediate processing cycle (useful for testing).
func (w *EnrichmentWorker) RunNow(ctx context.Context) {
	w.processTick(ctx)
}
