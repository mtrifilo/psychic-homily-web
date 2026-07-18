package routes

import (
	"github.com/danielgtaylor/huma/v2"

	communityh "psychic-homily-backend/internal/api/handlers/community"
	"psychic-homily-backend/internal/api/middleware"
)

// setupContributeRoutes configures public contribution opportunity endpoints.
// Registered under optional auth: the endpoints are public, but an
// authenticated viewer additionally sees the personal "Loose Ends" categories
// (followed artists missing links, PSY-1483).
func setupContributeRoutes(rc RouteContext) {
	contributeHandler := communityh.NewContributeHandler(rc.SC.DataQuality)

	optionalAuthGroup := huma.NewGroup(rc.API, "")
	optionalAuthGroup.UseMiddleware(middleware.OptionalHumaJWTMiddleware(rc.SC.JWT))

	huma.Get(optionalAuthGroup, "/contribute/opportunities", contributeHandler.GetOpportunitiesHandler)
	huma.Get(optionalAuthGroup, "/contribute/opportunities/{category}", contributeHandler.GetOpportunityCategoryHandler)
}
