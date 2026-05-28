package routes

import (
	"github.com/danielgtaylor/huma/v2"

	communityh "psychic-homily-backend/internal/api/handlers/community"
	"psychic-homily-backend/internal/api/middleware"
)

// setupRequestRoutes configures community request endpoints.
// Public endpoints use optional auth (so authenticated users see their vote).
// CRUD, voting, fulfillment, and closing require authentication.
func setupRequestRoutes(rc RouteContext) {
	requestHandler := communityh.NewRequestHandler(rc.SC.Request, rc.SC.AuditLog)

	// Public request endpoints with optional auth (to include user's vote)
	optionalAuthGroup := huma.NewGroup(rc.API, "")
	optionalAuthGroup.UseMiddleware(middleware.OptionalHumaJWTMiddleware(rc.SC.JWT))
	huma.Get(optionalAuthGroup, "/requests", requestHandler.ListRequestsHandler)
	huma.Get(optionalAuthGroup, "/requests/{request_id}", requestHandler.GetRequestHandler)

	// Protected request endpoints
	huma.Post(rc.Protected, "/requests", requestHandler.CreateRequestHandler)
	huma.Put(rc.Protected, "/requests/{request_id}", requestHandler.UpdateRequestHandler)
	huma.Delete(rc.Protected, "/requests/{request_id}", requestHandler.DeleteRequestHandler)
	huma.Post(rc.Protected, "/requests/{request_id}/vote", requestHandler.VoteRequestHandler)
	huma.Delete(rc.Protected, "/requests/{request_id}/vote", requestHandler.RemoveVoteRequestHandler)
	huma.Post(rc.Protected, "/requests/{request_id}/fulfill", requestHandler.FulfillRequestHandler)
	// PSY-748: two-step fulfillment workflow. Submit (above) is open to
	// any authed user; approve/reject is requester-or-admin only — auth
	// is enforced in the service layer so the route registration only
	// needs the standard Protected gate.
	huma.Post(rc.Protected, "/requests/{request_id}/approve-fulfillment", requestHandler.ApproveFulfillmentHandler)
	huma.Post(rc.Protected, "/requests/{request_id}/reject-fulfillment", requestHandler.RejectFulfillmentHandler)
	huma.Post(rc.Protected, "/requests/{request_id}/close", requestHandler.CloseRequestHandler)
}
