package routes

import (
	"github.com/danielgtaylor/huma/v2"

	exploreh "psychic-homily-backend/internal/api/handlers/explore"
)

// setupExploreRoutes registers the three public read endpoints that
// back the /explore landing page. Anonymous and authenticated callers
// both see identical content (locked decision), so all routes register
// directly on the public API group with no JWT middleware.
//
//   - GET /explore/upcoming-shows — chronological, paginated.
//   - GET /explore/featured       — admin-curated bill + collection.
//   - GET /explore/shuffle-target — random artist from the
//     currently-relevant pool.
func setupExploreRoutes(rc RouteContext) {
	handler := exploreh.NewExploreHandler(rc.SC.Explore)

	huma.Get(rc.API, "/explore/upcoming-shows", handler.GetUpcomingShowsHandler)
	huma.Get(rc.API, "/explore/featured", handler.GetFeaturedHandler)
	huma.Get(rc.API, "/explore/shuffle-target", handler.GetShuffleTargetHandler)
}
