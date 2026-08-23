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
	// The ONE Huma instance. Nothing in this package may call humachi.New again
	// — TestNoSubAPIInstancesRemain enforces that, and its message carries the
	// reasoning; the short version is that each instance owns a SEPARATE OpenAPI
	// document, so operations registered on a second one are reachable but
	// undocumented, and its /openapi.json registration silently shadows this one
	// (chi replaces a duplicate method+path rather than erroring).
	//
	// That is not hypothetical: production served a spec titled "Psychic Homily
	// Entity Reports" describing 8 report routes instead of the whole surface
	// (PSY-1554), and 27 operations — the entire auth surface among them — were
	// missing from the published document until PSY-1598 moved them here.
	//
	// The replacement for a sub-API is a Huma group carrying its own middleware:
	// huma.NewGroup(api, "") plus humaFromHTTP for any net/http middleware (a
	// rate limiter, typically). See the rate-limited groups in this package.
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

	// PSY-423: admin group — JWT auth + IsAdmin enforced via middleware so
	// pure-admin handlers don't have to call shared.RequireAdmin(ctx)
	// individually. Conditional-admin endpoints (owner-or-admin, etc.) stay
	// on protectedGroup with handler-side logic.
	adminGroup := huma.NewGroup(api, "")
	adminGroup.UseMiddleware(middleware.HumaJWTMiddleware(sc.JWT, cfg.Session))
	adminGroup.UseMiddleware(middleware.HumaSentryContextMiddleware)
	adminGroup.UseMiddleware(middleware.HumaAdminMiddleware)

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
	setupSavedReleaseRoutes(rc)
	setupReleaseRoutes(rc)
	setupLabelRoutes(rc)
	setupFestivalRoutes(rc)
	setupVenueRoutes(rc)
	setupCalendarRoutes(rc)
	setupSavedShowRoutes(rc)
	setupShowReportRoutes(rc)
	setupAdminRoutes(rc)
	setupPipelineRoutes(rc)
	setupAIExtractionRoutes(rc)
	setupSourceRoutes(rc)
	setupContributorProfileRoutes(rc)
	setupCollectionRoutes(rc)
	setupRequestRoutes(rc)
	setupRevisionRoutes(rc)
	setupEntityExistenceRoutes(rc)
	setupTagRoutes(rc)
	setupArtistRelationshipRoutes(rc)
	setupSceneRoutes(rc)
	setupFollowRoutes(rc)
	setupNotificationFilterRoutes(rc)
	setupChartsRoutes(rc)
	setupPendingEditRoutes(rc)
	setupEntityReportRoutes(rc)
	setupEntityRequestRoutes(rc)
	setupContributeRoutes(rc)
	setupLeaderboardRoutes(rc)
	setupRadioRoutes(rc)
	setupCommentRoutes(rc)
	setupCommentVoteRoutes(rc)
	setupCommentSubscriptionRoutes(rc)
	setupFieldNoteRoutes(rc)
	setupExploreRoutes(rc)
	setupGraphRoutes(rc)
	setupSitemapRoutes(rc)

	// PSY-432: test-fixtures reset endpoint — only registered when the env
	// flag is set. cmd/server/main.go refuses to boot if the flag is on and
	// ENVIRONMENT is not one of {test, ci, development}, so reaching this
	// branch in a non-test env isn't possible.
	setupTestFixtureRoutes(rc)

	return api
}
