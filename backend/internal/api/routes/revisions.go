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
	// A THIRD policy joined them, on the same credential, reading only the admin
	// bit (PSY-1940): WHO MADE THE EDIT. For the public tier an author who set
	// privacy_settings.contributions = "hidden", or whose only resolvable name
	// would come from their email address, is not named at all — the revision is
	// served, the byline is absent. Admins see the canonical name. Unlike the
	// other two this one is decided in the HANDLER, because it needs no query and
	// touches only response strings; see admin/revision.go revisionAuthorCredit
	// for that argument and services/shared/public_attribution.go for the rule.
	//
	// CACHING: these three responses now vary by CREDENTIAL, which they did not
	// before. Two caches matter, and only one of them is currently safe.
	//
	// The NETWORK path is clear. None of these sets Cache-Control, the frontend
	// calls the API host directly over TLS with credentials:'include', and there
	// is no CDN on that path. Anything that puts a SHARED cache in front of these
	// routes has to carry `Vary: Authorization, Cookie` (or mark the responses
	// private), or it will serve one admin's unmasked history to the next
	// anonymous reader. Note what "unmasked" now covers: not just an unverified
	// venue's address, but the NAME of every contributor who asked not to be
	// credited. A leak there discloses a person, not a street.
	//
	// The CLIENT cache was a known gap when the first of these tiers landed, and
	// it is CLOSED — this comment used to say otherwise and was stale. The
	// frontend keys revision queries on entity identity with no viewer, but
	// queryKeys.revisions.all is the first entry in VIEWER_TIER_QUERY_KEYS
	// (lib/queryClient.ts), which refreshViewerTierQueries drops on every
	// transition INTO an authenticated session (useAuth's
	// refreshCachesForNewSession, and both passkey flows). Signing out still runs
	// queryClient.clear(). So both directions are handled without a page load.
	//
	// Anything added to these responses that varies by credential must be
	// covered by that key list. All three tiers are today.
	//
	// The residual is a stale-but-SAFE window, not a leak: a reader whose tier
	// just narrowed can briefly see a cached richer payload only if they had
	// already been served it. React Query refetches an errored query on the next
	// mount, so a submitter who signed in after a cached 404 recovers too.
	optionalAuthGroup := huma.NewGroup(rc.API, "")
	optionalAuthGroup.UseMiddleware(middleware.OptionalHumaJWTMiddleware(rc.SC.JWT))
	huma.Get(optionalAuthGroup, "/revisions/{entity_type}/{entity_id}", revisionHandler.GetEntityHistoryHandler)
	huma.Get(optionalAuthGroup, "/revisions/{revision_id}", revisionHandler.GetRevisionHandler)
	huma.Get(optionalAuthGroup, "/users/{user_id}/revisions", revisionHandler.GetUserRevisionsHandler)

	// Admin rollback endpoint (PSY-423: rc.Admin enforces auth + IsAdmin)
	huma.Post(rc.Admin, "/admin/revisions/{revision_id}/rollback", revisionHandler.RollbackRevisionHandler)
}
