package routes

import (
	"github.com/danielgtaylor/huma/v2"

	adminh "psychic-homily-backend/internal/api/handlers/admin"
	communityh "psychic-homily-backend/internal/api/handlers/community"
)

// setupContributeRoutes configures public contribution opportunity endpoints.
func setupContributeRoutes(rc RouteContext) {
	contributeHandler := communityh.NewContributeHandler(rc.SC.DataQuality)
	huma.Get(rc.API, "/contribute/opportunities", contributeHandler.GetOpportunitiesHandler)
	huma.Get(rc.API, "/contribute/opportunities/{category}", contributeHandler.GetOpportunityCategoryHandler)
}

// setupDataGapsRoutes configures entity data-gap detection endpoints (protected).
func setupDataGapsRoutes(rc RouteContext) {
	dataGapsHandler := adminh.NewDataGapsHandler(rc.SC.Artist, rc.SC.Venue, rc.SC.Festival, rc.SC.Release, rc.SC.Label)
	huma.Get(rc.Protected, "/entities/{entity_type}/{id_or_slug}/data-gaps", dataGapsHandler.GetDataGapsHandler)
}
