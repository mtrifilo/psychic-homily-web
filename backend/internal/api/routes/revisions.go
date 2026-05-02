package routes

import (
	"github.com/danielgtaylor/huma/v2"

	adminh "psychic-homily-backend/internal/api/handlers/admin"
)

// setupRevisionRoutes configures revision history endpoints.
// Public endpoints for viewing history; admin endpoint for rollback.
func setupRevisionRoutes(rc RouteContext) {
	revisionHandler := adminh.NewRevisionHandler(rc.SC.Revision, rc.SC.AuditLog)

	// Public revision endpoints
	huma.Get(rc.API, "/revisions/{entity_type}/{entity_id}", revisionHandler.GetEntityHistoryHandler)
	huma.Get(rc.API, "/revisions/{revision_id}", revisionHandler.GetRevisionHandler)
	huma.Get(rc.API, "/users/{user_id}/revisions", revisionHandler.GetUserRevisionsHandler)

	// Admin-only rollback endpoint (PSY-423: route-gated by HumaAdminMiddleware)
	huma.Post(rc.Admin, "/admin/revisions/{revision_id}/rollback", revisionHandler.RollbackRevisionHandler)
}
