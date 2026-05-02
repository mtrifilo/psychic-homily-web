package routes

import (
	"os"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httprate"

	catalogh "psychic-homily-backend/internal/api/handlers/catalog"
	"psychic-homily-backend/internal/api/middleware"
)

// setupShowRoutes configures all show-related endpoints
func setupShowRoutes(rc RouteContext) {
	showHandler := catalogh.NewShowHandler(rc.SC.Show, rc.SC.Show, rc.SC.Show, rc.SC.SavedShow, rc.SC.Discord, rc.SC.MusicDiscovery, rc.SC.Extraction)

	// Public show endpoints - registered on main API without middleware
	// Note: Static routes must come before parameterized routes
	huma.Get(rc.API, "/shows", showHandler.GetShowsHandler)
	huma.Get(rc.API, "/shows/cities", showHandler.GetShowCitiesHandler)
	huma.Get(rc.API, "/shows/upcoming", showHandler.GetUpcomingShowsHandler)
	huma.Get(rc.API, "/shows/search", showHandler.SearchShowsHandler)

	// Show detail with optional auth for access control on non-approved shows
	optionalAuthGroup := huma.NewGroup(rc.API, "")
	optionalAuthGroup.UseMiddleware(middleware.OptionalHumaJWTMiddleware(rc.SC.JWT))
	huma.Get(optionalAuthGroup, "/shows/{show_id}", showHandler.GetShowHandler)

	// Export endpoint - only register in development environment
	if os.Getenv("ENVIRONMENT") == "development" {
		huma.Get(rc.API, "/shows/{show_id}/export", showHandler.ExportShowHandler)
	}

	// Rate-limited show creation: 10 requests per hour per IP
	// Prevents flooding the admin approval queue
	// API token requests (phk_ prefix) bypass the rate limit — they're trusted admin clients
	rc.Router.Group(func(r chi.Router) {
		r.Use(rateLimitUnlessAPIToken(
			middleware.ShowCreateRequestsPerHour,
			time.Hour,
		))
		showCreateAPI := humachi.New(r, huma.DefaultConfig("Psychic Homily Show Create", "1.0.0"))
		showCreateAPI.UseMiddleware(middleware.HumaRequestIDMiddleware)
		showCreateAPI.UseMiddleware(middleware.HumaJWTMiddleware(rc.SC.JWT, rc.Cfg.Session))
		huma.Post(showCreateAPI, "/shows", showHandler.CreateShowHandler)
	})

	// Rate-limited AI processing: 5 requests per minute per IP
	// Calls external Anthropic API — expensive operation
	rc.Router.Group(func(r chi.Router) {
		r.Use(httprate.Limit(
			middleware.AIProcessRequestsPerMinute,
			time.Minute,
			httprate.WithKeyFuncs(httprate.KeyByIP),
			httprate.WithLimitHandler(rateLimitHandler),
		))
		aiProcessAPI := humachi.New(r, huma.DefaultConfig("Psychic Homily AI Process", "1.0.0"))
		aiProcessAPI.UseMiddleware(middleware.HumaRequestIDMiddleware)
		aiProcessAPI.UseMiddleware(middleware.HumaJWTMiddleware(rc.SC.JWT, rc.Cfg.Session))
		huma.Post(aiProcessAPI, "/shows/ai-process", showHandler.AIProcessShowHandler)
	})

	// Protected show endpoints (no additional rate limiting needed)
	huma.Put(rc.Protected, "/shows/{show_id}", showHandler.UpdateShowHandler)
	huma.Delete(rc.Protected, "/shows/{show_id}", showHandler.DeleteShowHandler)
	huma.Post(rc.Protected, "/shows/{show_id}/unpublish", showHandler.UnpublishShowHandler)
	huma.Post(rc.Protected, "/shows/{show_id}/make-private", showHandler.MakePrivateShowHandler)
	huma.Post(rc.Protected, "/shows/{show_id}/publish", showHandler.PublishShowHandler)
	huma.Post(rc.Protected, "/shows/{show_id}/sold-out", showHandler.SetShowSoldOutHandler)
	huma.Post(rc.Protected, "/shows/{show_id}/cancelled", showHandler.SetShowCancelledHandler)
	huma.Get(rc.Protected, "/shows/my-submissions", showHandler.GetMySubmissionsHandler)
}
