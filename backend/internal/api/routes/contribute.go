package routes

import (
	"github.com/danielgtaylor/huma/v2"

	communityh "psychic-homily-backend/internal/api/handlers/community"
)

// setupContributeRoutes configures public contribution opportunity endpoints.
func setupContributeRoutes(rc RouteContext) {
	contributeHandler := communityh.NewContributeHandler(rc.SC.DataQuality)
	huma.Get(rc.API, "/contribute/opportunities", contributeHandler.GetOpportunitiesHandler)
	huma.Get(rc.API, "/contribute/opportunities/{category}", contributeHandler.GetOpportunityCategoryHandler)
}
