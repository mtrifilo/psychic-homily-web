package routes

import (
	"github.com/danielgtaylor/huma/v2"

	engagementh "psychic-homily-backend/internal/api/handlers/engagement"
	"psychic-homily-backend/internal/api/middleware"
)

// setupFollowRoutes configures follow/unfollow endpoints for entities.
// Follow/unfollow requires authentication. Follower counts use optional auth
// (counts always available; user follow status if authenticated).
func setupFollowRoutes(rc RouteContext) {
	followHandler := engagementh.NewFollowHandler(rc.SC.Follow)

	// Optional auth group for public follower counts/lists
	optionalAuthGroup := huma.NewGroup(rc.API, "")
	optionalAuthGroup.UseMiddleware(middleware.OptionalHumaJWTMiddleware(rc.SC.JWT))

	// Follow/unfollow (protected): entity_type is a path param (artists, venues, labels, festivals)
	huma.Post(rc.Protected, "/{entity_type}/{entity_id}/follow", followHandler.FollowEntityHandler)
	huma.Delete(rc.Protected, "/{entity_type}/{entity_id}/follow", followHandler.UnfollowEntityHandler)

	// Public with optional auth: follower count + user follow status
	huma.Get(optionalAuthGroup, "/{entity_type}/{entity_id}/followers", followHandler.GetFollowersHandler)

	// Public with optional auth: follower list
	huma.Get(optionalAuthGroup, "/{entity_type}/{entity_id}/followers/list", followHandler.GetFollowersListHandler)

	// Batch follow counts (optional auth)
	huma.Post(optionalAuthGroup, "/follows/batch", followHandler.BatchFollowHandler)

	// User's following list (protected)
	huma.Get(rc.Protected, "/me/following", followHandler.GetMyFollowingHandler)
}
