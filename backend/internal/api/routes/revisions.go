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
	// A SECOND policy now rides the same credential, and it reads more of it
	// (PSY-1715). A non-approved show's revisions are withheld ENTIRELY rather
	// than masked, from everyone except an admin and the show's own submitter,
	// mirroring what GET /shows/{id} already does with the same two facts. So the
	// user id matters here and not only the admin bit, and the entity-history
	// route answers 404 rather than a redacted 200. See
	// admin/revision_visibility.go.
	//
	// CACHING: these three responses now vary by CREDENTIAL, which they did not
	// before. Two caches matter, and only one of them is currently safe.
	//
	// The NETWORK path is clear. None of these sets Cache-Control, the frontend
	// calls the API host directly over TLS with credentials:'include', and there
	// is no CDN on that path. Anything that puts a SHARED cache in front of these
	// routes has to carry `Vary: Authorization, Cookie` (or mark the responses
	// private), or it will serve one admin's unmasked history to the next
	// anonymous reader.
	//
	// The CLIENT cache is a known gap this change does NOT close. The frontend
	// keys revision queries on entity identity alone — no viewer — with a
	// 15-minute staleTime, and login neither clears nor invalidates it (only
	// logout, a failed token refresh, and account deletion call
	// queryClient.clear()). So a moderator who opens a gated venue's History
	// while logged out and then signs in WITHOUT a full page load keeps being
	// served the masked payload, beside a Rollback button that restores the real
	// value — the state this tier exists to remove, surviving in the client.
	// It is not revision-specific: follows, tags and comments are all
	// optional-auth and cached the same way, so the fix is one deliberate
	// "invalidate credential-varying queries on login" change (invalidateQueries
	// .revisions() already exists and nothing calls it on login), not a patch
	// here.
	//
	// The show gate widens that gap in one direction and narrows it in another.
	// It can leave a submitter looking at a cached 404 for their own show's
	// history after signing in without a page load — annoying rather than
	// unsafe, and React Query refetches an errored query on the next mount. It
	// cannot go the other way: a cached 200 is a payload the caller was already
	// served, and a logged-in reader who signs OUT has queryClient.clear() run
	// for them.
	optionalAuthGroup := huma.NewGroup(rc.API, "")
	optionalAuthGroup.UseMiddleware(middleware.OptionalHumaJWTMiddleware(rc.SC.JWT))
	huma.Get(optionalAuthGroup, "/revisions/{entity_type}/{entity_id}", revisionHandler.GetEntityHistoryHandler)
	huma.Get(optionalAuthGroup, "/revisions/{revision_id}", revisionHandler.GetRevisionHandler)
	huma.Get(optionalAuthGroup, "/users/{user_id}/revisions", revisionHandler.GetUserRevisionsHandler)

	// Admin rollback endpoint (PSY-423: rc.Admin enforces auth + IsAdmin)
	huma.Post(rc.Admin, "/admin/revisions/{revision_id}/rollback", revisionHandler.RollbackRevisionHandler)
}
