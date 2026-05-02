// Package routes wires every HTTP/Huma endpoint into the chi mux.
//
// Each domain owns one file (e.g. shows.go, comments.go) that exposes a
// setupXxxRoutes(RouteContext) function. SetupRoutes here builds the shared
// RouteContext once and calls each domain setup in order. Shared types and
// rate-limit helpers live in shared.go; auth-specific helpers live in
// auth_rate_limit.go.
//
// PSY-422: this file used to be 1.1k lines containing every route definition.
// It was split per-domain so adding a new endpoint no longer requires
// editing a monolith. See routes/<domain>.go for each domain's routes.
package routes

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/services"
)

// SetupRoutes configures all API routes
func SetupRoutes(router *chi.Mux, sc *services.ServiceContainer, cfg *config.Config) huma.API {
	api := humachi.New(router, huma.DefaultConfig("Psychic Homily", "1.0.0"))

	// Add request ID middleware to all Huma routes
	api.UseMiddleware(middleware.HumaRequestIDMiddleware)

	// Enrich Sentry scope with request ID and HTTP metadata on all routes
	api.UseMiddleware(middleware.HumaSentryContextMiddleware)

	// Create a protected group that will require authentication
	protectedGroup := huma.NewGroup(api, "")
	protectedGroup.UseMiddleware(middleware.HumaJWTMiddleware(sc.JWT, cfg.Session))
	// Enrich Sentry scope with authenticated user context (runs after JWT middleware)
	protectedGroup.UseMiddleware(middleware.HumaSentryContextMiddleware)

	// PSY-423: admin group. JWT first (populates user), then Admin middleware
	// (requires IsAdmin=true). Endpoints registered on this group are gated at
	// the route level — handlers do not need to call shared.RequireAdmin(ctx).
	// Conditional-admin endpoints (owner-or-admin) stay on protectedGroup.
	adminGroup := huma.NewGroup(api, "")
	adminGroup.UseMiddleware(middleware.HumaJWTMiddleware(sc.JWT, cfg.Session))
	adminGroup.UseMiddleware(middleware.HumaAdminMiddleware())
	adminGroup.UseMiddleware(middleware.HumaSentryContextMiddleware)

	// Build the shared RouteContext once, pass to all setup functions
	rc := RouteContext{
		Router:    router,
		API:       api,
		Protected: protectedGroup,
		Admin:     adminGroup,
		SC:        sc,
		Cfg:       cfg,
	}

	// Setup domain-specific routes. Order is preserved from the original
	// monolithic routes.go to keep registration order identical (Huma/chi
	// route resolution is order-sensitive — literal paths must register
	// before parameterized siblings).
	setupSystemRoutes(rc)
	setupAuthRoutes(rc)
	setupProtectedAuthRoutes(rc)
	setupPasskeyRoutes(rc)

	setupShowRoutes(rc)
	setupArtistRoutes(rc)
	setupReleaseRoutes(rc)
	setupLabelRoutes(rc)
	setupFestivalRoutes(rc)
	setupVenueRoutes(rc)
	setupCalendarRoutes(rc)
	setupSavedShowRoutes(rc)
	setupFavoriteVenueRoutes(rc)
	setupShowReportRoutes(rc)
	setupArtistReportRoutes(rc)
	setupAdminRoutes(rc)
	setupPipelineRoutes(rc)
	setupContributorProfileRoutes(rc)
	setupCollectionRoutes(rc)
	setupRequestRoutes(rc)
	setupRevisionRoutes(rc)
	setupTagRoutes(rc)
	setupArtistRelationshipRoutes(rc)
	setupSceneRoutes(rc)
	setupAttendanceRoutes(rc)
	setupFollowRoutes(rc)
	setupNotificationFilterRoutes(rc)
	setupChartsRoutes(rc)
	setupPendingEditRoutes(rc)
	setupEntityReportRoutes(rc)
	setupContributeRoutes(rc)
	setupLeaderboardRoutes(rc)
	setupDataGapsRoutes(rc)
	setupRadioRoutes(rc)
	setupCommentRoutes(rc)
	setupCommentVoteRoutes(rc)
	setupCommentSubscriptionRoutes(rc)
	setupFieldNoteRoutes(rc)

	// PSY-432: test-fixtures reset endpoint — only registered when the env
	// flag is set. cmd/server/main.go refuses to boot if the flag is on and
	// ENVIRONMENT is not one of {test, ci, development}, so reaching this
	// branch in a non-test env isn't possible.
	setupTestFixtureRoutes(rc)

	return api
}
