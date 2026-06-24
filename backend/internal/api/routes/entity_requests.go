package routes

import (
	"github.com/danielgtaylor/huma/v2"

	communityh "psychic-homily-backend/internal/api/handlers/community"
)

// setupEntityRequestRoutes configures the polymorphic entity_requests queue
// endpoints (PSY-997, built on PSY-869's service).
//
//   - User queue-create on rc.Protected (auth required; trust-tier gating is
//     in the service — contributor/new_user file a pending request).
//   - Admin list + decide on rc.Admin (auth + IsAdmin enforced by middleware,
//     per PSY-423; handlers carry no inline admin check).
//
// These are SEPARATE from the pending_entity_edits admin endpoints — PSY-871's
// frontend unifies the two queues into one page; the backend keeps them
// parallel and independently testable.
func setupEntityRequestRoutes(rc RouteContext) {
	entityRequestHandler := communityh.NewEntityRequestHandler(
		rc.SC.EntityRequest,
		rc.SC.EntityRequestFulfiller,
		rc.SC.AuditLog,
	)

	// User: file an entity-creation request (consumed by PSY-845 + PSY-853).
	huma.Post(rc.Protected, "/entity-requests", entityRequestHandler.CreateEntityRequestHandler)

	// Admin: moderation queue (consumed by PSY-871).
	huma.Get(rc.Admin, "/admin/entity-requests", entityRequestHandler.AdminListEntityRequestsHandler)
	huma.Post(rc.Admin, "/admin/entity-requests/{id}/decide", entityRequestHandler.AdminDecideEntityRequestHandler)

	// Admin: rescue an approved-but-unfulfilled request — fulfill (re-run the
	// catalog create, supplying show associations) or void it (PSY-1088). The
	// approved-but-unfulfilled rows are discoverable via the list endpoint's
	// state=approved + unfulfilled=true filter.
	huma.Post(rc.Admin, "/admin/entity-requests/{id}/fulfill", entityRequestHandler.AdminFulfillEntityRequestHandler)
}
