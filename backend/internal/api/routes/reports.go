package routes

import (
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httprate"

	communityh "psychic-homily-backend/internal/api/handlers/community"
	"psychic-homily-backend/internal/api/middleware"
)

// setupShowReportRoutes configures show report endpoints
// All endpoints require authentication via protected group
func setupShowReportRoutes(rc RouteContext) {
	showReportHandler := communityh.NewShowReportHandler(rc.SC.ShowReport, rc.SC.Discord, rc.SC.User, rc.SC.AuditLog)

	// Rate-limited report submission: 5 requests per minute per IP
	// Prevents spamming admins with reports
	rc.Router.Group(func(r chi.Router) {
		r.Use(httprate.Limit(
			middleware.ReportRequestsPerMinute,
			time.Minute,
			httprate.WithKeyFuncs(httprate.KeyByIP),
			httprate.WithLimitHandler(rateLimitHandler),
		))
		reportAPI := humachi.New(r, huma.DefaultConfig("Psychic Homily Reports", "1.0.0"))
		reportAPI.UseMiddleware(middleware.HumaRequestIDMiddleware)
		reportAPI.UseMiddleware(middleware.HumaJWTMiddleware(rc.SC.JWT, rc.Cfg.Session))
		huma.Post(reportAPI, "/shows/{show_id}/report", showReportHandler.ReportShowHandler)
	})

	// Protected report endpoints (no additional rate limiting)
	huma.Get(rc.Protected, "/shows/{show_id}/my-report", showReportHandler.GetMyReportHandler)

	// Admin endpoints for managing reports (PSY-423: rc.Admin enforces auth + IsAdmin)
	huma.Get(rc.Admin, "/admin/reports", showReportHandler.GetPendingReportsHandler)
	huma.Post(rc.Admin, "/admin/reports/{report_id}/dismiss", showReportHandler.DismissReportHandler)
	huma.Post(rc.Admin, "/admin/reports/{report_id}/resolve", showReportHandler.ResolveReportHandler)
}

// setupArtistReportRoutes configures artist report endpoints
func setupArtistReportRoutes(rc RouteContext) {
	artistReportHandler := communityh.NewArtistReportHandler(rc.SC.ArtistReport, rc.SC.Discord, rc.SC.User, rc.SC.AuditLog)

	// Rate-limited report submission: 5 requests per minute per IP
	rc.Router.Group(func(r chi.Router) {
		r.Use(httprate.Limit(
			middleware.ReportRequestsPerMinute,
			time.Minute,
			httprate.WithKeyFuncs(httprate.KeyByIP),
			httprate.WithLimitHandler(rateLimitHandler),
		))
		reportAPI := humachi.New(r, huma.DefaultConfig("Psychic Homily Artist Reports", "1.0.0"))
		reportAPI.UseMiddleware(middleware.HumaRequestIDMiddleware)
		reportAPI.UseMiddleware(middleware.HumaJWTMiddleware(rc.SC.JWT, rc.Cfg.Session))
		huma.Post(reportAPI, "/artists/{artist_id}/report", artistReportHandler.ReportArtistHandler)
	})

	// Protected report endpoints (no additional rate limiting)
	huma.Get(rc.Protected, "/artists/{artist_id}/my-report", artistReportHandler.GetMyArtistReportHandler)

	// Admin endpoints for managing artist reports (PSY-423: rc.Admin enforces auth + IsAdmin)
	huma.Get(rc.Admin, "/admin/artist-reports", artistReportHandler.GetPendingArtistReportsHandler)
	huma.Post(rc.Admin, "/admin/artist-reports/{report_id}/dismiss", artistReportHandler.DismissArtistReportHandler)
	huma.Post(rc.Admin, "/admin/artist-reports/{report_id}/resolve", artistReportHandler.ResolveArtistReportHandler)
}

// setupEntityReportRoutes configures entity report endpoints.
// Protected endpoints for submitting reports.
// Admin endpoints for reviewing, resolving, and dismissing reports.
func setupEntityReportRoutes(rc RouteContext) {
	entityReportHandler := communityh.NewEntityReportHandler(rc.SC.EntityReport, rc.SC.AuditLog)

	// Rate-limited report submission: 5 requests per minute per IP
	rc.Router.Group(func(r chi.Router) {
		r.Use(httprate.Limit(
			middleware.ReportRequestsPerMinute,
			time.Minute,
			httprate.WithKeyFuncs(httprate.KeyByIP),
			httprate.WithLimitHandler(rateLimitHandler),
		))
		reportAPI := humachi.New(r, huma.DefaultConfig("Psychic Homily Entity Reports", "1.0.0"))
		reportAPI.UseMiddleware(middleware.HumaRequestIDMiddleware)
		reportAPI.UseMiddleware(middleware.HumaJWTMiddleware(rc.SC.JWT, rc.Cfg.Session))
		huma.Post(reportAPI, "/artists/{entity_id}/report", entityReportHandler.ReportArtistHandler)
		huma.Post(reportAPI, "/venues/{entity_id}/report", entityReportHandler.ReportVenueHandler)
		huma.Post(reportAPI, "/festivals/{entity_id}/report", entityReportHandler.ReportFestivalHandler)
		// Note: shows already have /shows/{show_id}/report in setupShowReportRoutes.
		// The generic entity report handler + service support shows, so the admin queue
		// can display show reports submitted through the existing endpoint or this one.
		huma.Post(reportAPI, "/shows/{entity_id}/entity-report", entityReportHandler.ReportShowHandler)
		huma.Post(reportAPI, "/comments/{entity_id}/report", entityReportHandler.ReportCommentHandler)
	})

	// Admin: entity report management (PSY-423: rc.Admin enforces auth + IsAdmin)
	huma.Get(rc.Admin, "/admin/entity-reports", entityReportHandler.AdminListEntityReportsHandler)
	huma.Get(rc.Admin, "/admin/entity-reports/{report_id}", entityReportHandler.AdminGetEntityReportHandler)
	huma.Post(rc.Admin, "/admin/entity-reports/{report_id}/resolve", entityReportHandler.AdminResolveEntityReportHandler)
	huma.Post(rc.Admin, "/admin/entity-reports/{report_id}/dismiss", entityReportHandler.AdminDismissEntityReportHandler)
}
