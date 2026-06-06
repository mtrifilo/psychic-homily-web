package community

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
	communitym "psychic-homily-backend/internal/models/community"
	"psychic-homily-backend/internal/services/contracts"
	servicesshared "psychic-homily-backend/internal/services/shared"
)

// PSY-997: HTTP endpoints for the polymorphic entity_requests moderation queue
// (built on PSY-869's service in services/community/entityrequest.go). Mirrors
// the polymorphic shape of entity_report.go: one user-facing create endpoint +
// admin list/decide endpoints, with audit-log writes fire-and-forget.
//
// These endpoints are intentionally SEPARATE from the pending_entity_edits
// admin endpoints (PSY-871's frontend unifies the two queues into one page;
// the backend keeps them parallel and independently testable).

// EntityRequestHandler handles entity-request queue API endpoints.
type EntityRequestHandler struct {
	entityRequestService contracts.EntityRequestServiceInterface
	fulfiller            contracts.EntityRequestFulfillerInterface
	auditLogService      contracts.AuditLogServiceInterface
}

// NewEntityRequestHandler creates a new entity-request handler. fulfiller is
// used only on the admin decide-approve path to create the actual entity from
// the payload; it may be nil in unit tests that don't exercise approval.
func NewEntityRequestHandler(
	entityRequestService contracts.EntityRequestServiceInterface,
	fulfiller contracts.EntityRequestFulfillerInterface,
	auditLogService contracts.AuditLogServiceInterface,
) *EntityRequestHandler {
	return &EntityRequestHandler{
		entityRequestService: entityRequestService,
		fulfiller:            fulfiller,
		auditLogService:      auditLogService,
	}
}

// ============================================================================
// User: Queue an entity-creation request — POST /entity-requests
// ============================================================================

// CreateEntityRequestRequest is the Huma request for POST /entity-requests.
//
// Payload is the typed, per-entity_type creation payload (the shapes in
// communitym/entity_request_payloads.go). It is carried as a raw JSON object
// and validated against the entity_type's registered struct on the read side;
// the handler only enforces it is present + non-empty here.
type CreateEntityRequestRequest struct {
	Body struct {
		EntityType    string          `json:"entity_type" doc:"Entity type to request (artist, venue, label, release, show, festival)"`
		Payload       json.RawMessage `json:"payload" doc:"Typed creation payload for the entity_type"`
		SourceContext string          `json:"source_context" required:"false" doc:"How the request originated (ai_extraction, paste_mode, manual); defaults to manual"`
		Confirmed     bool            `json:"confirmed" required:"false" doc:"FE-side confirm step (only relevant to trusted_contributor tier)"`
	}
}

// CreateEntityRequestResponse is the Huma response for POST /entity-requests.
type CreateEntityRequestResponse struct {
	Body *communitym.EntityRequest
}

// CreateEntityRequestHandler handles POST /entity-requests.
//
// Tier policy lives in PSY-869's service (autoApproves): contributor/new_user
// file a PENDING request (never autonomously create the entity, per
// feedback_human_verify_ai_entity_data); admin/local_ambassador (and confirmed
// trusted_contributor) auto-approve. The service stamps decided_by/at on
// auto-approve. This handler is a thin validator + pass-through.
func (h *EntityRequestHandler) CreateEntityRequestHandler(ctx context.Context, req *CreateEntityRequestRequest) (*CreateEntityRequestResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	entityType := strings.TrimSpace(req.Body.EntityType)
	if !communitym.IsValidEntityRequestType(entityType) {
		return nil, huma.Error422UnprocessableEntity("Invalid entity type '" + entityType + "'")
	}

	// source_context defaults to manual when omitted; any provided value must
	// be a recognized source.
	sourceContext := strings.TrimSpace(req.Body.SourceContext)
	if sourceContext == "" {
		sourceContext = communitym.EntityRequestSourceManual
	}
	if !communitym.IsValidEntityRequestSource(sourceContext) {
		return nil, huma.Error422UnprocessableEntity("Invalid source context '" + sourceContext + "'")
	}

	if len(strings.TrimSpace(string(req.Body.Payload))) == 0 {
		return nil, huma.Error422UnprocessableEntity("Payload is required")
	}

	created, err := h.entityRequestService.CreateRequest(user, entityType, req.Body.Payload, sourceContext, req.Body.Confirmed)
	if err != nil {
		if mapped := shared.MapEntityRequestError(err); mapped != nil {
			return nil, mapped
		}
		logger.FromContext(ctx).Error("entity_request_create_failed",
			"user_id", user.ID,
			"entity_type", entityType,
			"source_context", sourceContext,
			"error", err.Error(),
		)
		return nil, huma.Error500InternalServerError("Failed to create entity request")
	}

	// Fire-and-forget audit log. Distinguish auto-approved (trusted tiers) from
	// queued so the activity feed reads correctly.
	if h.auditLogService != nil {
		action := "queue_entity_request"
		if created.DecisionState == communitym.EntityRequestStateApproved {
			action = "auto_approve_entity_request"
		}
		reqID := created.ID
		state := string(created.DecisionState)
		servicesshared.GoSafe(ctx, "audit_log", func() {
			h.auditLogService.LogAction(user.ID, action, entityType, reqID, map[string]interface{}{
				"request_id":     reqID,
				"source_context": sourceContext,
				"decision_state": state,
			})
		})
	}

	return &CreateEntityRequestResponse{Body: created}, nil
}

// ============================================================================
// Admin: List entity requests — GET /admin/entity-requests
// ============================================================================

// AdminListEntityRequestsRequest is the Huma request for GET /admin/entity-requests.
type AdminListEntityRequestsRequest struct {
	State         string `query:"state" required:"false" doc:"Filter by decision state (pending, approved, rejected); defaults to pending"`
	EntityType    string `query:"entity_type" required:"false" doc:"Filter by entity type (artist, venue, label, release, show, festival)"`
	SourceContext string `query:"source_context" required:"false" doc:"Filter by source context (ai_extraction, paste_mode, manual)"`
	Limit         int    `query:"limit" required:"false" minimum:"1" maximum:"100" doc:"Max results (default 20, max 100)"`
	Offset        int    `query:"offset" required:"false" minimum:"0" doc:"Offset for pagination"`
}

// AdminListEntityRequestsResponse is the Huma response for GET /admin/entity-requests.
type AdminListEntityRequestsResponse struct {
	Body struct {
		Requests []communitym.EntityRequest `json:"requests"`
		Total    int64                      `json:"total"`
	}
}

// AdminListEntityRequestsHandler handles GET /admin/entity-requests.
// Admin-gated via rc.Admin middleware (no inline admin check, per PSY-423).
func (h *EntityRequestHandler) AdminListEntityRequestsHandler(ctx context.Context, req *AdminListEntityRequestsRequest) (*AdminListEntityRequestsResponse, error) {
	if req.EntityType != "" && !communitym.IsValidEntityRequestType(req.EntityType) {
		return nil, huma.Error422UnprocessableEntity("Invalid entity type '" + req.EntityType + "'")
	}
	if req.State != "" && !communitym.IsValidEntityRequestState(req.State) {
		return nil, huma.Error422UnprocessableEntity("Invalid state '" + req.State + "'")
	}
	if req.SourceContext != "" && !communitym.IsValidEntityRequestSource(req.SourceContext) {
		return nil, huma.Error422UnprocessableEntity("Invalid source context '" + req.SourceContext + "'")
	}

	requests, total, err := h.entityRequestService.ListRequests(&contracts.EntityRequestFilters{
		EntityType:    req.EntityType,
		State:         req.State,
		SourceContext: req.SourceContext,
		Limit:         req.Limit,
		Offset:        req.Offset,
	})
	if err != nil {
		logger.FromContext(ctx).Error("entity_request_list_failed", "error", err.Error())
		return nil, huma.Error500InternalServerError("Failed to list entity requests")
	}

	resp := &AdminListEntityRequestsResponse{}
	resp.Body.Requests = requests
	resp.Body.Total = total
	return resp, nil
}

// ============================================================================
// Admin: Decide an entity request — POST /admin/entity-requests/{id}/decide
// ============================================================================

// AdminDecideEntityRequestRequest is the Huma request for
// POST /admin/entity-requests/{id}/decide.
type AdminDecideEntityRequestRequest struct {
	ID   string `path:"id" doc:"Entity request ID to decide"`
	Body struct {
		Decision string  `json:"decision" doc:"Decision: approved or rejected"`
		Note     *string `json:"note" required:"false" doc:"Optional decision note (shown to the requester)"`
	}
}

// AdminDecideEntityRequestResponse is the Huma response for the decide endpoint.
// On approve, CreatedEntityID/CreatedEntityType report the catalog entity that
// was created from the payload.
type AdminDecideEntityRequestResponse struct {
	Body struct {
		Request           *communitym.EntityRequest `json:"request"`
		CreatedEntityID   *uint                     `json:"created_entity_id,omitempty"`
		CreatedEntityType *string                   `json:"created_entity_type,omitempty"`
	}
}

// AdminDecideEntityRequestHandler handles POST /admin/entity-requests/{id}/decide.
// Admin-gated via rc.Admin middleware (no inline admin check, per PSY-423).
//
// Approve flow (claim-then-fulfill): the service's atomic Decide claims the
// pending→approved transition FIRST (so two concurrent approvals can't both
// win and double-create the entity), then the handler fulfills the payload into
// a real catalog entity. If fulfillment fails after the claim, the row is
// approved-but-unfulfilled and the error is logged loudly + surfaced — the
// admin can create the entity manually. This trades a rare orphaned-approval
// for never double-creating an entity.
//
// Reject flow: marks rejected + optional note. No entity is created.
func (h *EntityRequestHandler) AdminDecideEntityRequestHandler(ctx context.Context, req *AdminDecideEntityRequestRequest) (*AdminDecideEntityRequestResponse, error) {
	admin := middleware.GetUserFromContext(ctx)

	requestID, err := strconv.ParseUint(req.ID, 10, 64)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid request ID")
	}

	decision := strings.TrimSpace(req.Body.Decision)
	var newState communitym.EntityRequestDecisionState
	switch decision {
	case string(communitym.EntityRequestStateApproved):
		newState = communitym.EntityRequestStateApproved
	case string(communitym.EntityRequestStateRejected):
		newState = communitym.EntityRequestStateRejected
	default:
		return nil, huma.Error422UnprocessableEntity("Decision must be 'approved' or 'rejected'")
	}

	var note *string
	if req.Body.Note != nil {
		trimmed := strings.TrimSpace(*req.Body.Note)
		if trimmed != "" {
			note = &trimmed
		}
	}

	// Claim the decision atomically before any side effect.
	decided, err := h.entityRequestService.Decide(uint(requestID), admin.ID, newState, note)
	if err != nil {
		if mapped := shared.MapEntityRequestError(err); mapped != nil {
			return nil, mapped
		}
		logger.FromContext(ctx).Error("entity_request_decide_failed",
			"request_id", requestID,
			"admin_id", admin.ID,
			"decision", decision,
			"error", err.Error(),
		)
		return nil, huma.Error500InternalServerError("Failed to record decision")
	}

	resp := &AdminDecideEntityRequestResponse{}
	resp.Body.Request = decided

	if newState == communitym.EntityRequestStateApproved {
		createdID, err := h.fulfillEntity(decided)
		if err != nil {
			// The row is already approved (claimed). Surface the fulfillment
			// failure so the admin knows the entity was NOT created and can act,
			// rather than silently returning success.
			logger.FromContext(ctx).Error("entity_request_fulfill_failed",
				"request_id", requestID,
				"admin_id", admin.ID,
				"entity_type", decided.EntityType,
				"error", err.Error(),
			)
			if mapped := shared.MapEntityRequestError(err); mapped != nil {
				return nil, mapped
			}
			return nil, huma.Error500InternalServerError("Request approved but creating the entity failed: " + err.Error())
		}
		resp.Body.CreatedEntityID = &createdID
		et := decided.EntityType
		resp.Body.CreatedEntityType = &et
	}

	// Fire-and-forget audit log.
	if h.auditLogService != nil {
		action := "approve_entity_request"
		if newState == communitym.EntityRequestStateRejected {
			action = "reject_entity_request"
		}
		reqID := decided.ID
		metadata := map[string]interface{}{
			"request_id":   reqID,
			"requester_id": decided.RequesterID,
		}
		if resp.Body.CreatedEntityID != nil {
			metadata["created_entity_id"] = *resp.Body.CreatedEntityID
		}
		entityType := decided.EntityType
		servicesshared.GoSafe(ctx, "audit_log", func() {
			h.auditLogService.LogAction(admin.ID, action, entityType, reqID, metadata)
		})
	}

	return resp, nil
}
