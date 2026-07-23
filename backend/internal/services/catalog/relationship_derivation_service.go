package catalog

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
)

// DefaultDerivationInterval is the default interval for relationship derivation (24 hours).
const DefaultDerivationInterval = 24 * time.Hour

// RelationshipDerivationService is a background service that periodically derives
// artist relationships from show co-occurrences (shared_bills), label
// co-occurrences (shared_label), and MusicBrainz artist-rels (member_of /
// side_project — PSY-1382).
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
	// PSY-1270: shared env helper (see env.go).
	interval := envPositiveHours("RELATIONSHIP_DERIVATION_INTERVAL_HOURS", DefaultDerivationInterval)

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

// RunDerivationCycle runs shared_bills, shared_label, and MusicBrainz
// member_of / side_project derivation (PSY-1382).
// Exported for use by the admin trigger endpoint.
func (s *RelationshipDerivationService) RunDerivationCycle() {
	start := time.Now()
	s.logger.Info("starting relationship derivation cycle")

	// Derive shared bills (artists who share minShows+ approved shows)
	billsCount, err := s.relService.DeriveSharedBills(contracts.DefaultSharedBillsMinShows)
	if err != nil {
		s.logger.Error("shared bills derivation failed", "error", err)
	} else {
		s.logger.Info("shared bills derivation complete", "upserted", billsCount)
	}

	// Derive shared labels (artists who share minLabels+ labels)
	labelsCount, err := s.relService.DeriveSharedLabels(contracts.DefaultSharedLabelsMinLabels)
	if err != nil {
		s.logger.Error("shared labels derivation failed", "error", err)
	} else {
		s.logger.Info("shared labels derivation complete", "upserted", labelsCount)
	}

	// PSY-1382: member_of / side_project from MusicBrainz artist-rels
	mbResult, err := s.relService.DeriveMusicBrainzArtistRels(context.Background())
	if err != nil {
		s.logger.Error("musicbrainz artist-rels derivation failed", "error", err)
	} else {
		s.logger.Info("musicbrainz artist-rels derivation complete",
			"member_of_upserted", mbResult.MemberOfUpserted,
			"side_project_upserted", mbResult.SideProjectUpserted,
			"artists_scanned", mbResult.ArtistsScanned,
			"lookups_failed", mbResult.LookupsFailed,
			"peers_skipped", mbResult.PeersSkipped,
		)
	}

	s.logger.Info("relationship derivation cycle complete",
		"shared_bills_upserted", billsCount,
		"shared_labels_upserted", labelsCount,
		"member_of_upserted", mbResult.MemberOfUpserted,
		"side_project_upserted", mbResult.SideProjectUpserted,
		"duration", time.Since(start),
	)
}
