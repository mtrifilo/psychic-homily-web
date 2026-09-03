package community

import (
	"context"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
	communitym "psychic-homily-backend/internal/models/community"
	servicesshared "psychic-homily-backend/internal/services/shared"
)

// ============================================================================
// User: Withdraw one's own queued request — POST /entity-requests/{id}/withdraw
// ============================================================================

// WithdrawEntityRequestRequest is the Huma request for
// POST /entity-requests/{id}/withdraw.
//
// A verb sub-resource POST, not a DELETE, and the shape is the one the family
// already uses: /admin/entity-requests/{id}/decide and .../fulfill are both a
// named transition posted onto a row. This is a third transition on the same
// row, and it destroys nothing — the row survives in 'withdrawn' — so a DELETE
// would describe the wrong thing to every reader of the route table.
//
// No body: a withdrawal states nothing but its own occurrence. decision_note is
// the moderator's field, and a note from a requester on their own retraction has
// no reader.
type WithdrawEntityRequestRequest struct {
	ID string `path:"id" doc:"Entity request ID to withdraw"`
}

// WithdrawEntityRequestResponse returns the withdrawn row, so a client can read
// the state it landed in rather than assuming the one it asked for.
type WithdrawEntityRequestResponse struct {
	Body struct {
		Request *communitym.EntityRequest `json:"request"`
	}
}

// WithdrawEntityRequestHandler handles POST /entity-requests/{id}/withdraw.
//
// Registered on rc.Protected: any authenticated user may call it, and WHOSE row
// it is is the service's conditional UPDATE, which carries the requester in its
// WHERE. There is no inline ownership read here, because a read followed by a
// write is two statements a concurrent decision can land between.
//
// The refusals, and why they differ:
//
//   - a row that does not exist, or exists and belongs to someone else, is 404.
//     The two are ONE answer on purpose: a distinct 403 for someone else's row
//     confirms that row exists, which turns the id space into an oracle for who
//     has requested what.
//   - the caller's OWN row that is no longer pending is 409, naming the state.
//     It is their row, so there is nothing to withhold, and it is the only
//     answer that explains why the affordance did nothing.
func (h *EntityRequestHandler) WithdrawEntityRequestHandler(ctx context.Context, req *WithdrawEntityRequestRequest) (*WithdrawEntityRequestResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	requestID, err := strconv.ParseUint(req.ID, 10, 64)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid request ID")
	}

	withdrawn, err := h.entityRequestService.Withdraw(uint(requestID), user.ID)
	if err != nil {
		if mapped := shared.MapEntityRequestError(err); mapped != nil {
			return nil, mapped
		}
		logger.FromContext(ctx).Error("entity_request_withdraw_failed",
			"request_id", requestID,
			"user_id", user.ID,
			"error", err.Error(),
		)
		return nil, huma.Error500InternalServerError("Failed to withdraw the request")
	}

	// Fire-and-forget audit log, matching the queue-create path's. The row's own
	// decision_state is the durable record; this is what puts the withdrawal in
	// the same log as the filing it retracts.
	//
	// The metadata carries no payload and no name: the row still holds both, and
	// what a contributor asked for and then thought better of is not something an
	// audit row needs to restate.
	if h.auditLogService != nil {
		reqID := withdrawn.ID
		entityType := withdrawn.EntityType
		metadata := map[string]interface{}{
			"request_id":     reqID,
			"decision_state": string(withdrawn.DecisionState),
		}
		servicesshared.GoSafe(ctx, "audit_log", func() {
			h.auditLogService.LogAction(user.ID, "withdraw_entity_request", entityType, reqID, metadata)
		})
	}

	resp := &WithdrawEntityRequestResponse{}
	resp.Body.Request = withdrawn
	return resp, nil
}
