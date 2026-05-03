package community

import "psychic-homily-backend/internal/services/contracts"

// Compile-time interface satisfaction checks for community services.
var (
	_ contracts.CollectionServiceInterface = (*CollectionService)(nil)
	_ contracts.RequestServiceInterface    = (*RequestService)(nil)
)
