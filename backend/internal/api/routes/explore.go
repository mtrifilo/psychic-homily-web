package routes

import (
	"github.com/danielgtaylor/huma/v2"

	exploreh "psychic-homily-backend/internal/api/handlers/explore"
)

// setupExploreRoutes registers the public read endpoints that
// back leftover /explore surfaces and discovery shuffle. Anonymous and
// authenticated callers both see identical content (locked decision), so
// all routes register directly on the public API group with no JWT
// middleware.
//
//   - GET /explore/upcoming-shows — chronological, paginated.
//   - GET /explore/shuffle-target — random artist from the
//     currently-relevant pool.
//
// Featured Bill/Collection (GET /explore/featured) was retired in PSY-1480.
func setupExploreRoutes(rc RouteContext) {
	handler := exploreh.NewExploreHandler(rc.SC.Explore)

	huma.Get(rc.API, "/explore/upcoming-shows", handler.GetUpcomingShowsHandler)
	huma.Get(rc.API, "/explore/shuffle-target", handler.GetShuffleTargetHandler)
}
