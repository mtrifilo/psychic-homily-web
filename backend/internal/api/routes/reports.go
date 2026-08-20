package routes

import (
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/httprate"

	communityh "psychic-homily-backend/internal/api/handlers/community"
	"psychic-homily-backend/internal/api/middleware"
)

// setupShowReportRoutes configures show report endpoints
// All endpoints require authentication via protected group
func setupShowReportRoutes(rc RouteContext) {
	showReportHandler := communityh.NewShowReportHandler(rc.SC.ShowReport, rc.SC.Discord, rc.SC.User, rc.SC.AuditLog)

	// Rate-limited report submission: 5 requests per minute per IP.
	// Prevents spamming admins with reports.
	//
	// PSY-1598: registered on the MAIN api via a Huma group, not on its own
	// humachi.New inside a chi.Group. A separate instance owns a separate
	// OpenAPI document, so this operation was absent from the published spec.
	// The limiter is the same httprate middleware as before, bridged by
	// humaFromHTTP — see its doc for why the existing one is reused rather than
	// reimplemented.
	reportSubmitGroup := huma.NewGroup(rc.API, "")
	reportSubmitGroup.UseMiddleware(humaFromHTTP(httprate.Limit(
		middleware.ReportRequestsPerMinute,
		time.Minute,
		httprate.WithKeyFuncs(middleware.KeyByClientIP),
		httprate.WithLimitHandler(rateLimitHandler),
	)))
	reportSubmitGroup.UseMiddleware(middleware.HumaRequestIDMiddleware)
	reportSubmitGroup.UseMiddleware(middleware.HumaJWTMiddleware(rc.SC.JWT, rc.Cfg.Session))
	huma.Post(reportSubmitGroup, "/shows/{show_id}/report", showReportHandler.ReportShowHandler)

	// Protected report endpoints (no additional rate limiting)
	huma.Get(rc.Protected, "/shows/{show_id}/my-report", showReportHandler.GetMyReportHandler)

	// Admin endpoints for managing reports (PSY-423: rc.Admin enforces auth + IsAdmin)
	huma.Get(rc.Admin, "/admin/reports", showReportHandler.GetPendingReportsHandler)
	huma.Post(rc.Admin, "/admin/reports/{report_id}/dismiss", showReportHandler.DismissReportHandler)
	huma.Post(rc.Admin, "/admin/reports/{report_id}/resolve", showReportHandler.ResolveReportHandler)
}

// setupEntityReportRoutes configures entity report endpoints.
// Protected endpoints for submitting and reading back reports.
// Admin endpoints for reviewing, resolving, and dismissing reports.
//
// Artists have no dedicated report surface: PSY-1633 folded the former
// artist_reports pipeline (POST /artists/{artist_id}/report, its my-report
// sibling, and the /admin/artist-reports queue) into this one. It had to go
// somewhere — chi keys its routing tree on path SHAPE, not on parameter name,
// so `/artists/{artist_id}/report` and `/artists/{entity_id}/report` were one
// route with two claimants and the later registration silently won. Reports
// therefore landed in entity_reports while the read-back and admin queue still
// searched artist_reports, and neither ever found them.
func setupEntityReportRoutes(rc RouteContext) {
	entityReportHandler := communityh.NewEntityReportHandler(rc.SC.EntityReport, rc.SC.AuditLog)

	// Rate-limited report submission: 5 requests per minute per IP.
	//
	// PSY-1598 (final step): on the MAIN api via a Huma group rather than its own
	// humachi.New. A separate instance owns a separate OpenAPI document, so these
	// eight operations were reachable but undocumented — and this was the very
	// instance whose spec fragment ("Psychic Homily Entity Reports", 8 paths) once
	// shadowed the published document in production (PSY-1554).
	//
	// Three properties this conversion must preserve, each easy to break silently:
	//
	//  1. ONE limiter for the whole group. The eight operations shared a single
	//     chi group, hence a single counter; building the limiter once here and
	//     attaching it to one Huma group keeps that. A per-operation limiter would
	//     look identical and multiply the budget eightfold for anyone willing to
	//     rotate entity types.
	//  2. The limiter stays OUTER of the JWT middleware (PSY-1397 layered
	//     ordering). It ran as chi middleware, i.e. before Huma saw the request at
	//     all, so unauthenticated floods were rejected without touching the auth
	//     path. UseMiddleware order is outer-to-inner, so the limiter must be
	//     registered first — as it is below.
	//  3. NO bypass. Unlike show-create and tag-create this group has never had an
	//     API-token or admin hatch, and reports are the admin queue's inbox: a
	//     bypass appearing here would be an abuse regression, not a convenience.
	entityReportGroup := huma.NewGroup(rc.API, "")
	entityReportGroup.UseMiddleware(humaFromHTTP(httprate.Limit(
		middleware.ReportRequestsPerMinute,
		time.Minute,
		httprate.WithKeyFuncs(middleware.KeyByClientIP),
		httprate.WithLimitHandler(rateLimitHandler),
	)))
	entityReportGroup.UseMiddleware(middleware.HumaRequestIDMiddleware)
	entityReportGroup.UseMiddleware(middleware.HumaJWTMiddleware(rc.SC.JWT, rc.Cfg.Session))

	huma.Post(entityReportGroup, "/artists/{entity_id}/report", entityReportHandler.ReportArtistHandler)
	huma.Post(entityReportGroup, "/venues/{entity_id}/report", entityReportHandler.ReportVenueHandler)
	huma.Post(entityReportGroup, "/festivals/{entity_id}/report", entityReportHandler.ReportFestivalHandler)
	// Note: shows already have /shows/{show_id}/report in setupShowReportRoutes.
	// The generic entity report handler + service support shows, so the admin queue
	// can display show reports submitted through the existing endpoint or this one.
	//
	// The two live on SEPARATE limiters, as they did when one was a chi group and
	// the other a Huma group — same cap, independent counters. Unchanged here.
	huma.Post(entityReportGroup, "/shows/{entity_id}/entity-report", entityReportHandler.ReportShowHandler)
	huma.Post(entityReportGroup, "/comments/{entity_id}/report", entityReportHandler.ReportCommentHandler)
	// PSY-357: report a collection. EntityID is the numeric collection ID
	// (the slug-based detail endpoints elsewhere are unrelated — this
	// stays on the generic /{type}/{id}/report shape so the moderation
	// queue can ingest collection reports through the same pipeline.)
	huma.Post(entityReportGroup, "/collections/{entity_id}/report", entityReportHandler.ReportCollectionHandler)
	// PSY-661: report a release. EntityID is the numeric release ID; the
	// moderation queue deep-links via the resolved slug.
	huma.Post(entityReportGroup, "/releases/{entity_id}/report", entityReportHandler.ReportReleaseHandler)
	// PSY-666: report a label. EntityID is the numeric label ID; the
	// moderation queue deep-links via the resolved slug.
	huma.Post(entityReportGroup, "/labels/{entity_id}/report", entityReportHandler.ReportLabelHandler)

	// Read back the caller's own pending report (no additional rate limiting).
	huma.Get(rc.Protected, "/artists/{entity_id}/my-report", entityReportHandler.GetMyArtistReportHandler)

	// Admin: entity report management (PSY-423: rc.Admin enforces auth + IsAdmin)
	huma.Get(rc.Admin, "/admin/entity-reports", entityReportHandler.AdminListEntityReportsHandler)
	huma.Get(rc.Admin, "/admin/entity-reports/{report_id}", entityReportHandler.AdminGetEntityReportHandler)
	huma.Post(rc.Admin, "/admin/entity-reports/{report_id}/resolve", entityReportHandler.AdminResolveEntityReportHandler)
	huma.Post(rc.Admin, "/admin/entity-reports/{report_id}/dismiss", entityReportHandler.AdminDismissEntityReportHandler)
}
