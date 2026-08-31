package routes

import (
	"os"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/httprate"

	catalogh "psychic-homily-backend/internal/api/handlers/catalog"
	engagementh "psychic-homily-backend/internal/api/handlers/engagement"
	"psychic-homily-backend/internal/api/middleware"
)

// setupShowRoutes configures all show-related endpoints
func setupShowRoutes(rc RouteContext) {
	showHandler := catalogh.NewShowHandler(rc.SC.Show, rc.SC.Show, rc.SC.Show, rc.SC.SavedShow, rc.SC.Discord, rc.SC.Extraction, rc.SC.Revision, rc.SC.ShowVisibility)

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

	// Public one-shot "Add to calendar" export for a single show — a chi route
	// for the same reasons as the venue feed (see routes/venues.go), with the
	// same two constraints: the parameter MUST stay named `show_id` to match
	// its Huma siblings (pinned by show_calendar_routing_test.go), and the
	// path stays on the ordinary public-read rate-limit budget.
	showCalendarHandler := engagementh.NewShowCalendarHandler(rc.SC.ShowCalendar, rc.Cfg)
	rc.Router.Get("/shows/{show_id}/calendar.ics", showCalendarHandler.GetShowCalendarHandler)
	rc.Router.Head("/shows/{show_id}/calendar.ics", showCalendarHandler.GetShowCalendarHandler)

	// The "also tonight" rail: other shows in this show's metro on this show's
	// own date. Served by the SCENE handler because "what else is on in
	// this metro tonight" is a scene question that merely takes a show as its
	// address — one definition of a metro's night, shared with /scenes/{slug}/day.
	//
	// Anonymous and public: no optional-auth group, because a non-approved show is
	// answered exactly like an unknown one here.
	//
	// No HEAD sibling. The huma.Head registrations on /scenes/{slug}/day exist
	// because frontend/proxy.ts HEAD-probes those paths to decide whether a page
	// is real; it existence-checks shows through /entities/shows/{slug}/exists
	// instead, and this path cannot soft-404 anyway — a show with nothing around
	// it is a 200 with an empty rail.
	//
	// The parameter is named show_id to match the rest of the group. chi stores
	// parameter names per endpoint PATTERN, so a differently named parameter on a
	// different shape is harmless (/shows/{entity_id}/entity-report already
	// coexists in reports.go). The hazard chi actually has is two registrations
	// of the SAME shape and method, where the later one silently replaces the
	// earlier: that is what show_also_tonight_routing_test.go pins.
	alsoTonightHandler := catalogh.NewSceneHandler(rc.SC.Scene)
	huma.Get(rc.API, "/shows/{show_id}/also-tonight", alsoTonightHandler.GetShowAlsoTonightHandler)

	// The gig timeline: the headliner's adjacent dates and each billed act's
	// recurrence in this show's place.
	//
	// Anonymous and public for the same reason as the rail above — a
	// non-approved show is answered exactly like an unknown one — so it stays
	// off optionalAuthGroup. A show with nothing around it is a 200 with empty
	// fields, so there is nothing here for proxy.ts to HEAD-probe either.
	huma.Get(rc.API, "/shows/{show_id}/timeline", showHandler.GetShowTimelineHandler)

	// Export endpoint - only register in development environment
	if os.Getenv("ENVIRONMENT") == "development" {
		huma.Get(rc.API, "/shows/{show_id}/export", showHandler.ExportShowHandler)
	}

	// Rate-limited show creation: 10 requests per hour per IP
	// Prevents flooding the admin approval queue
	// API token requests (phk_ prefix) bypass the rate limit — they're trusted admin clients
	//
	// PSY-1598: on the MAIN api via a Huma group, not its own humachi.New — a
	// separate instance owns a separate OpenAPI document, so this operation was
	// absent from the published spec. rateLimitUnlessAPIToken is already a
	// net/http middleware, so humaFromHTTP carries it across unchanged, bypass
	// included; the limiter is still built ONCE here, so its counter state is
	// per-route, not per-request.
	showCreateGroup := huma.NewGroup(rc.API, "")
	showCreateGroup.UseMiddleware(humaFromHTTP(rateLimitUnlessAPIToken(
		middleware.ShowCreateRequestsPerHour,
		time.Hour,
	)))
	showCreateGroup.UseMiddleware(middleware.HumaRequestIDMiddleware)
	showCreateGroup.UseMiddleware(middleware.HumaJWTMiddleware(rc.SC.JWT, rc.Cfg.Session))
	huma.Post(showCreateGroup, "/shows", showHandler.CreateShowHandler)

	// Rate-limited AI processing: 5 requests per minute per IP
	// Calls external Anthropic API — expensive operation
	aiProcessGroup := huma.NewGroup(rc.API, "")
	aiProcessGroup.UseMiddleware(humaFromHTTP(httprate.Limit(
		middleware.AIProcessRequestsPerMinute,
		time.Minute,
		httprate.WithKeyFuncs(middleware.KeyByClientIP),
		httprate.WithLimitHandler(rateLimitHandler),
	)))
	aiProcessGroup.UseMiddleware(middleware.HumaRequestIDMiddleware)
	aiProcessGroup.UseMiddleware(middleware.HumaJWTMiddleware(rc.SC.JWT, rc.Cfg.Session))
	huma.Post(aiProcessGroup, "/shows/ai-process", showHandler.AIProcessShowHandler)

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
