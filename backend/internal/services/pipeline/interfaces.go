package pipeline

import "psychic-homily-backend/internal/services/contracts"

// Compile-time interface satisfaction checks for pipeline services.
var (
	_ contracts.ExtractionServiceInterface      = (*ExtractionService)(nil)
	_ contracts.MusicDiscoveryServiceInterface   = (*MusicDiscoveryService)(nil)
	_ contracts.DiscoveryServiceInterface        = (*DiscoveryService)(nil)
	_ contracts.VenueSourceConfigServiceInterface = (*VenueSourceConfigService)(nil)
	_ contracts.FetcherServiceInterface          = (*FetcherService)(nil)
	_ contracts.PipelineServiceInterface         = (*PipelineService)(nil)
)
