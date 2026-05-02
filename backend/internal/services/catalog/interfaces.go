package catalog

import "psychic-homily-backend/internal/services/contracts"

// Compile-time interface satisfaction checks for catalog services.
var (
	_ contracts.ShowServiceInterface                 = (*ShowService)(nil)
	_ contracts.ShowAdminServiceInterface            = (*ShowService)(nil)
	_ contracts.ShowImportServiceInterface           = (*ShowService)(nil)
	_ contracts.ShowStateServiceInterface            = (*ShowService)(nil)
	_ contracts.ShowFullServiceInterface             = (*ShowService)(nil)
	_ contracts.VenueServiceInterface                = (*VenueService)(nil)
	_ contracts.ArtistServiceInterface               = (*ArtistService)(nil)
	_ contracts.FestivalServiceInterface             = (*FestivalService)(nil)
	_ contracts.LabelServiceInterface                = (*LabelService)(nil)
	_ contracts.ReleaseServiceInterface              = (*ReleaseService)(nil)
	_ contracts.TagServiceInterface                  = (*TagService)(nil)
	_ contracts.ArtistRelationshipServiceInterface   = (*ArtistRelationshipService)(nil)
	_ contracts.SceneServiceInterface                = (*SceneService)(nil)
	_ contracts.FestivalIntelligenceServiceInterface = (*FestivalIntelligenceService)(nil)
	_ contracts.ChartsServiceInterface               = (*ChartsService)(nil)
	_ contracts.RadioServiceInterface                = (*RadioService)(nil)
)
