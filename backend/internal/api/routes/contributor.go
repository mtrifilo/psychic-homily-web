package routes

import (
	"github.com/danielgtaylor/huma/v2"

	communityh "psychic-homily-backend/internal/api/handlers/community"
	"psychic-homily-backend/internal/api/middleware"
)

// setupContributorProfileRoutes configures contributor profile endpoints
func setupContributorProfileRoutes(rc RouteContext) {
	profileHandler := communityh.NewContributorProfileHandler(rc.SC.ContributorProfile, rc.SC.User)

	// Public profile endpoints with optional auth (so profile owner can see their own private profile)
	optionalAuthGroup := huma.NewGroup(rc.API, "")
	optionalAuthGroup.UseMiddleware(middleware.OptionalHumaJWTMiddleware(rc.SC.JWT))
	huma.Get(optionalAuthGroup, "/users/{username}", profileHandler.GetPublicProfileHandler)
	huma.Get(optionalAuthGroup, "/users/{username}/contributions", profileHandler.GetContributionHistoryHandler)
	huma.Get(optionalAuthGroup, "/users/{username}/sections", profileHandler.GetUserSectionsHandler)
	huma.Get(optionalAuthGroup, "/users/{username}/activity-heatmap", profileHandler.GetActivityHeatmapHandler)
	huma.Get(optionalAuthGroup, "/users/{username}/rankings", profileHandler.GetPercentileRankingsHandler)

	// Protected endpoints for authenticated user's own profile
	huma.Get(rc.Protected, "/auth/profile/contributor", profileHandler.GetOwnProfileHandler)
	huma.Get(rc.Protected, "/auth/profile/contributions", profileHandler.GetOwnContributionsHandler)
	huma.Patch(rc.Protected, "/auth/profile/visibility", profileHandler.UpdateProfileVisibilityHandler)
	huma.Patch(rc.Protected, "/auth/profile/privacy", profileHandler.UpdatePrivacySettingsHandler)
	huma.Get(rc.Protected, "/auth/profile/sections", profileHandler.GetOwnSectionsHandler)
	huma.Post(rc.Protected, "/auth/profile/sections", profileHandler.CreateSectionHandler)
	huma.Put(rc.Protected, "/auth/profile/sections/{section_id}", profileHandler.UpdateSectionHandler)
	huma.Delete(rc.Protected, "/auth/profile/sections/{section_id}", profileHandler.DeleteSectionHandler)
}

// setupLeaderboardRoutes configures public contributor leaderboard endpoints.
// Uses optional auth to include the requesting user's rank when authenticated.
func setupLeaderboardRoutes(rc RouteContext) {
	leaderboardHandler := communityh.NewLeaderboardHandler(rc.SC.Leaderboard)

	optionalAuthGroup := huma.NewGroup(rc.API, "")
	optionalAuthGroup.UseMiddleware(middleware.OptionalHumaJWTMiddleware(rc.SC.JWT))

	huma.Get(optionalAuthGroup, "/community/leaderboard", leaderboardHandler.GetLeaderboardHandler)
}
