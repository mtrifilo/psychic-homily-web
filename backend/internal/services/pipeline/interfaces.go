package pipeline

import "psychic-homily-backend/internal/services/contracts"

// Compile-time interface satisfaction checks for pipeline services.
var (
	_ contracts.ExtractionServiceInterface        = (*ExtractionService)(nil)
	_ contracts.DiscoveryServiceInterface         = (*DiscoveryService)(nil)
	_ contracts.EnrichmentServiceInterface        = (*EnrichmentService)(nil)
	_ contracts.EnrichmentWorkerInterface         = (*EnrichmentWorker)(nil)
	_ contracts.StreamingWorklistServiceInterface = (*StreamingWorklistService)(nil)
)
