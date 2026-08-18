package routes

import (
	"github.com/danielgtaylor/huma/v2"

	adminh "psychic-homily-backend/internal/api/handlers/admin"
	"psychic-homily-backend/internal/api/middleware"
)

// setupRevisionRoutes configures revision history endpoints.
// Public endpoints for viewing history; admin endpoint for rollback.
func setupRevisionRoutes(rc RouteContext) {
	revisionHandler := adminh.NewRevisionHandler(rc.SC.Revision, rc.SC.AuditLog)

	// Public revision endpoints, OPTIONALLY authenticated (PSY-1717).
	//
	// Optional, not the protected group: nothing here 401s, and the credential
	// only selects the tier. The strict middleware would reject unauthenticated
	// requests outright and take revision history away from the anonymous
	// readers it exists for.
	//
	// The middleware is here so the handler can tell an admin from everyone else:
	// an unverified venue's address is masked in this history for the public tier
	// and served to admins, whose Rollback button restores it either way. See
	// admin.RevisionService.applyPrivacyRedaction for the policy and for why it
	// diverges from the tier-less live venue payload.
	//
	// CACHING, for whoever adds it here next: these three responses now vary by
	// CREDENTIAL, which they did not before. None of them sets Cache-Control
	// today and nothing between the browser and this service caches them — the
	// frontend calls the API host directly over TLS with credentials:'include',
	// and there is no CDN on that path — so there is nothing to leak through
	// right now. Anything that puts a shared cache in front of these routes has
	// to carry `Vary: Authorization, Cookie` (or mark them private), or it will
	// serve one admin's unmasked history to the next anonymous reader.
	optionalAuthGroup := huma.NewGroup(rc.API, "")
	optionalAuthGroup.UseMiddleware(middleware.OptionalHumaJWTMiddleware(rc.SC.JWT))
	huma.Get(optionalAuthGroup, "/revisions/{entity_type}/{entity_id}", revisionHandler.GetEntityHistoryHandler)
	huma.Get(optionalAuthGroup, "/revisions/{revision_id}", revisionHandler.GetRevisionHandler)
	huma.Get(optionalAuthGroup, "/users/{user_id}/revisions", revisionHandler.GetUserRevisionsHandler)

	// Admin rollback endpoint (PSY-423: rc.Admin enforces auth + IsAdmin)
	huma.Post(rc.Admin, "/admin/revisions/{revision_id}/rollback", revisionHandler.RollbackRevisionHandler)
}
