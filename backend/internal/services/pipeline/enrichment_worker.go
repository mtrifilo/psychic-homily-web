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
)

// EnrichmentWorker is a background service that processes the enrichment queue.
// It follows the same Start/Stop pattern as SchedulerService and CleanupService.
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
// No startup cycle — waits one interval before the first tick.
func (w *EnrichmentWorker) run(ctx context.Context) {
	defer w.wg.Done()
	shared.RunTickerLoop(ctx, "enrichment_worker", w.interval, w.stopCh, false, func(c context.Context) {
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
