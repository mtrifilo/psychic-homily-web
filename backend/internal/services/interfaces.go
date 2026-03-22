package services

import (
	"psychic-homily-backend/internal/services/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// Compile-time interface satisfaction checks for services in the root package
// and the catalog sub-package. Other sub-packages have their own interfaces.go.
//
// Engagement services: internal/services/engagement/interfaces.go
// Pipeline services:   internal/services/pipeline/interfaces.go
// Auth services:       internal/services/auth/interfaces.go
// Notification services: internal/services/notification/interfaces.go
// User services:       internal/services/user/interfaces.go
// Admin services:      internal/services/admin/interfaces.go
var (
	_ contracts.ShowServiceInterface                = (*catalog.ShowService)(nil)
	_ contracts.VenueServiceInterface               = (*catalog.VenueService)(nil)
	_ contracts.ArtistServiceInterface              = (*catalog.ArtistService)(nil)
	_ contracts.FestivalServiceInterface            = (*catalog.FestivalService)(nil)
	_ contracts.LabelServiceInterface               = (*catalog.LabelService)(nil)
	_ contracts.ReleaseServiceInterface             = (*catalog.ReleaseService)(nil)
	_ contracts.CollectionServiceInterface          = (*CollectionService)(nil)
	_ contracts.RequestServiceInterface             = (*RequestService)(nil)
	_ contracts.TagServiceInterface                 = (*catalog.TagService)(nil)
	_ contracts.ArtistRelationshipServiceInterface  = (*catalog.ArtistRelationshipService)(nil)
	_ contracts.SceneServiceInterface               = (*catalog.SceneService)(nil)
	_ contracts.FestivalIntelligenceServiceInterface = (*catalog.FestivalIntelligenceService)(nil)
	_ contracts.ChartsServiceInterface              = (*catalog.ChartsService)(nil)
)
