package routes

import (
	"github.com/danielgtaylor/huma/v2"

	adminh "psychic-homily-backend/internal/api/handlers/admin"
	catalogh "psychic-homily-backend/internal/api/handlers/catalog"
	pipelineh "psychic-homily-backend/internal/api/handlers/pipeline"
)

// setupAdminRoutes configures admin-only endpoints.
// PSY-423: registered on rc.Admin so the admin gate is enforced by
// HumaAdminMiddleware at the route level — handlers do not call RequireAdmin.
func setupAdminRoutes(rc RouteContext) {
	// Domain-specific admin handlers
	statsHandler := adminh.NewAdminStatsHandler(rc.SC.AdminStats)
	showHandler := adminh.NewAdminShowHandler(
		rc.SC.Show, rc.SC.Show, rc.SC.Show, rc.SC.Discord, rc.SC.AuditLog, rc.SC.NotificationFilter,
		rc.SC.MusicDiscovery,
	)
	venueHandler := adminh.NewAdminVenueHandler(rc.SC.Venue, rc.SC.AuditLog)
	userHandler := adminh.NewAdminUserHandler(rc.SC.User)
	tokenHandler := adminh.NewAdminTokenHandler(rc.SC.APIToken)
	dataHandler := adminh.NewAdminDataHandler(rc.SC.DataSync)
	discoveryHandler := pipelineh.NewAdminDiscoveryHandler(rc.SC.Discovery)

	artistHandler := catalogh.NewArtistHandler(rc.SC.Artist, rc.SC.AuditLog, rc.SC.Revision)
	auditLogHandler := adminh.NewAuditLogHandler(rc.SC.AuditLog)

	// Admin dashboard stats endpoint
	huma.Get(rc.Admin, "/admin/stats", statsHandler.GetAdminStatsHandler)
	huma.Get(rc.Admin, "/admin/activity", statsHandler.GetActivityFeedHandler)

	// Admin show listing endpoint (for CLI export)
	huma.Get(rc.Admin, "/admin/shows", showHandler.GetAdminShowsHandler)

	// Admin show management endpoints
	huma.Get(rc.Admin, "/admin/shows/pending", showHandler.GetPendingShowsHandler)
	huma.Get(rc.Admin, "/admin/shows/rejected", showHandler.GetRejectedShowsHandler)
	huma.Post(rc.Admin, "/admin/shows/{show_id}/approve", showHandler.ApproveShowHandler)
	huma.Post(rc.Admin, "/admin/shows/{show_id}/reject", showHandler.RejectShowHandler)
	huma.Post(rc.Admin, "/admin/shows/batch-approve", showHandler.BatchApproveShowsHandler)
	huma.Post(rc.Admin, "/admin/shows/batch-reject", showHandler.BatchRejectShowsHandler)

	// Admin show import endpoints (single)
	huma.Post(rc.Admin, "/admin/shows/import/preview", showHandler.ImportShowPreviewHandler)
	huma.Post(rc.Admin, "/admin/shows/import/confirm", showHandler.ImportShowConfirmHandler)

	// Admin show export/import endpoints (bulk - for CLI)
	huma.Post(rc.Admin, "/admin/shows/export/bulk", showHandler.BulkExportShowsHandler)
	huma.Post(rc.Admin, "/admin/shows/import/bulk/preview", showHandler.BulkImportPreviewHandler)
	huma.Post(rc.Admin, "/admin/shows/import/bulk/confirm", showHandler.BulkImportConfirmHandler)

	// Admin venue management endpoints
	huma.Get(rc.Admin, "/admin/venues/unverified", venueHandler.GetUnverifiedVenuesHandler)
	huma.Post(rc.Admin, "/admin/venues/{venue_id}/verify", venueHandler.VerifyVenueHandler)

	// Admin artist management endpoints — STAY on rc.Protected because they
	// support an X-Internal-Secret bypass for the automated discovery service
	// (no user context). Handler enforces admin OR internal-secret. Do not
	// migrate to rc.Admin without first refactoring the internal-service auth
	// path.
	huma.Patch(rc.Protected, "/admin/artists/{artist_id}/bandcamp", artistHandler.UpdateArtistBandcampHandler)
	huma.Patch(rc.Protected, "/admin/artists/{artist_id}/spotify", artistHandler.UpdateArtistSpotifyHandler)

	// Admin discovery endpoints (for local discovery app)
	huma.Post(rc.Admin, "/admin/discovery/import", discoveryHandler.DiscoveryImportHandler)
	huma.Post(rc.Admin, "/admin/discovery/check", discoveryHandler.DiscoveryCheckHandler)

	// Admin API token management endpoints
	huma.Post(rc.Admin, "/admin/tokens", tokenHandler.CreateAPITokenHandler)
	huma.Get(rc.Admin, "/admin/tokens", tokenHandler.ListAPITokensHandler)
	huma.Delete(rc.Admin, "/admin/tokens/{token_id}", tokenHandler.RevokeAPITokenHandler)

	// Admin data export endpoints (for syncing local data to Stage/Production)
	huma.Get(rc.Admin, "/admin/export/shows", dataHandler.ExportShowsHandler)
	huma.Get(rc.Admin, "/admin/export/artists", dataHandler.ExportArtistsHandler)
	huma.Get(rc.Admin, "/admin/export/venues", dataHandler.ExportVenuesHandler)

	// Admin data import endpoint (for syncing local data to Stage/Production)
	huma.Post(rc.Admin, "/admin/data/import", dataHandler.DataImportHandler)

	// Admin audit log endpoint
	huma.Get(rc.Admin, "/admin/audit-logs", auditLogHandler.GetAuditLogsHandler)

	// Admin user list endpoint
	huma.Get(rc.Admin, "/admin/users", userHandler.GetAdminUsersHandler)

	// Admin data quality endpoints
	dataQualityHandler := adminh.NewDataQualityHandler(rc.SC.DataQuality)
	huma.Get(rc.Admin, "/admin/data-quality", dataQualityHandler.GetDataQualitySummaryHandler)
	huma.Get(rc.Admin, "/admin/data-quality/{category}", dataQualityHandler.GetDataQualityCategoryHandler)

	// Admin auto-promotion endpoints (manual trigger for tier evaluation)
	autoPromotionHandler := adminh.NewAutoPromotionHandler(rc.SC.AutoPromotion)
	huma.Post(rc.Admin, "/admin/auto-promotion/evaluate", autoPromotionHandler.EvaluateAllUsersHandler)
	huma.Get(rc.Admin, "/admin/auto-promotion/evaluate/{user_id}", autoPromotionHandler.EvaluateUserHandler)

	// Admin analytics endpoints
	analyticsHandler := adminh.NewAnalyticsHandler(rc.SC.Analytics)
	huma.Get(rc.Admin, "/admin/analytics/growth", analyticsHandler.GetGrowthMetricsHandler)
	huma.Get(rc.Admin, "/admin/analytics/engagement", analyticsHandler.GetEngagementMetricsHandler)
	huma.Get(rc.Admin, "/admin/analytics/community", analyticsHandler.GetCommunityHealthHandler)
	huma.Get(rc.Admin, "/admin/analytics/data-quality", analyticsHandler.GetDataQualityTrendsHandler)
}
