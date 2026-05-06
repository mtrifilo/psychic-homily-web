package catalog

import (
	"context"
	"log/slog"
	"os"
	"strconv"
	"sync"
	"time"

	"psychic-homily-backend/internal/services/shared"
)

// DefaultDerivationInterval is the default interval for relationship derivation (24 hours).
const DefaultDerivationInterval = 24 * time.Hour

// RelationshipDerivationService is a background service that periodically derives
// artist relationships from show co-occurrences (shared_bills) and label
// co-occurrences (shared_label).
//
// It follows the same Start/Stop pattern as RadioFetchService and other background services.
type RelationshipDerivationService struct {
	relService *ArtistRelationshipService
	interval   time.Duration

	stopCh chan struct{}
	wg     sync.WaitGroup
	logger *slog.Logger
}

// NewRelationshipDerivationService creates a new relationship derivation background service.
// Env vars:
//   - RELATIONSHIP_DERIVATION_INTERVAL_HOURS (default 24)
func NewRelationshipDerivationService(relService *ArtistRelationshipService) *RelationshipDerivationService {
	interval := DefaultDerivationInterval
	if envVal := os.Getenv("RELATIONSHIP_DERIVATION_INTERVAL_HOURS"); envVal != "" {
		if hours, err := strconv.Atoi(envVal); err == nil && hours > 0 {
			interval = time.Duration(hours) * time.Hour
		}
	}

	return &RelationshipDerivationService{
		relService: relService,
		interval:   interval,
		stopCh:     make(chan struct{}),
		logger:     slog.Default(),
	}
}

// Start begins the background derivation service.
func (s *RelationshipDerivationService) Start(ctx context.Context) {
	s.wg.Add(1)
	go s.runLoop(ctx)

	s.logger.Info("relationship derivation service started",
		"interval_hours", s.interval.Hours(),
	)
}

// Stop gracefully stops the derivation service.
func (s *RelationshipDerivationService) Stop() {
	close(s.stopCh)
	s.wg.Wait()
	s.logger.Info("relationship derivation service stopped")
}

// runLoop runs the periodic derivation cycle.
// No startup cycle — the admin endpoint is used for immediate triggering.
func (s *RelationshipDerivationService) runLoop(ctx context.Context) {
	defer s.wg.Done()
	shared.RunTickerLoop(ctx, "relationship_derivation", s.interval, s.stopCh, false, func(_ context.Context) {
		s.RunDerivationCycle()
	})
}

// RunDerivationCycle runs both shared_bills and shared_label derivation.
// Exported for use by the admin trigger endpoint.
func (s *RelationshipDerivationService) RunDerivationCycle() {
	start := time.Now()
	s.logger.Info("starting relationship derivation cycle")

	// Derive shared bills (artists who share 2+ approved shows)
	billsCount, err := s.relService.DeriveSharedBills(2)
	if err != nil {
		s.logger.Error("shared bills derivation failed", "error", err)
	} else {
		s.logger.Info("shared bills derivation complete", "upserted", billsCount)
	}

	// Derive shared labels (artists who share 1+ labels)
	labelsCount, err := s.relService.DeriveSharedLabels(1)
	if err != nil {
		s.logger.Error("shared labels derivation failed", "error", err)
	} else {
		s.logger.Info("shared labels derivation complete", "upserted", labelsCount)
	}

	s.logger.Info("relationship derivation cycle complete",
		"shared_bills_upserted", billsCount,
		"shared_labels_upserted", labelsCount,
		"duration", time.Since(start),
	)
}
