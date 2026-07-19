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

	// Scene follows (PSY-1339): scenes are slug-addressed (the registry row
	// materializes lazily on first follow), so they get dedicated routes
	// instead of joining the generic id-keyed shape — /scenes/{slug}/follow
	// would collide with /{entity_type}/{entity_id}/follow for entity_type
	// "scenes" (the static segment wins), and the FE never holds a scene id.
	sceneFollowHandler := engagementh.NewSceneFollowHandler(rc.SC.Follow, rc.SC.Scene)
	huma.Post(rc.Protected, "/scenes/{slug}/follow", sceneFollowHandler.SceneFollowHandler)
	huma.Delete(rc.Protected, "/scenes/{slug}/follow", sceneFollowHandler.SceneUnfollowHandler)
	huma.Get(optionalAuthGroup, "/scenes/{slug}/followers", sceneFollowHandler.SceneFollowersHandler)

	// User follows (PSY-1496): username-addressed — do NOT add "users" to the
	// generic entity_type map; numeric user ids are not the profile UX key.
	// Reuses FollowService with entity_type=user (FollowEntityUser / PSY-296).
	userFollowHandler := engagementh.NewUserFollowHandler(rc.SC.Follow, rc.SC.User)
	huma.Post(rc.Protected, "/users/{username}/follow", userFollowHandler.UserFollowHandler)
	huma.Delete(rc.Protected, "/users/{username}/follow", userFollowHandler.UserUnfollowHandler)
	huma.Get(optionalAuthGroup, "/users/{username}/followers", userFollowHandler.UserFollowersHandler)

	// Public with optional auth: follower count + user follow status
	huma.Get(optionalAuthGroup, "/{entity_type}/{entity_id}/followers", followHandler.GetFollowersHandler)

	// Public with optional auth: follower list
	huma.Get(optionalAuthGroup, "/{entity_type}/{entity_id}/followers/list", followHandler.GetFollowersListHandler)

	// Batch follow counts (optional auth)
	huma.Post(optionalAuthGroup, "/follows/batch", followHandler.BatchFollowHandler)

	// User's following list (protected)
	huma.Get(rc.Protected, "/me/following", followHandler.GetMyFollowingHandler)

	// Library-specific following read model (protected): one aggregate-count
	// query plus bounded, deterministic alphabetical pages by entity type.
	huma.Get(rc.Protected, "/me/library/following/counts", followHandler.GetLibraryFollowingCountsHandler)
	huma.Get(rc.Protected, "/me/library/following", followHandler.GetLibraryFollowingHandler)
}
