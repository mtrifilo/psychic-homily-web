package community

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/middleware"
	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
	communitym "psychic-homily-backend/internal/models/community"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
)

// RequestHandler handles request-related API requests
type RequestHandler struct {
	requestService contracts.RequestServiceInterface
	auditLog       contracts.AuditLogServiceInterface
}

// NewRequestHandler creates a new RequestHandler
func NewRequestHandler(requestService contracts.RequestServiceInterface, auditLog contracts.AuditLogServiceInterface) *RequestHandler {
	return &RequestHandler{
		requestService: requestService,
		auditLog:       auditLog,
	}
}

// ============================================================================
// Create Request
// ============================================================================

// CreateRequestHandlerRequest represents the request for creating a request
type CreateRequestHandlerRequest struct {
	Body struct {
		Title             string  `json:"title" doc:"Request title" example:"Add Local Band XYZ"`
		Description       *string `json:"description,omitempty" required:"false" doc:"Detailed description of the request" example:"They play at The Rebel Lounge frequently"`
		EntityType        string  `json:"entity_type" doc:"Entity type (artist, release, label, show, venue, festival)" example:"artist"`
		RequestedEntityID *uint   `json:"requested_entity_id,omitempty" required:"false" doc:"Optional ID of an existing entity this relates to"`
	}
}

// CreateRequestHandlerResponse represents the response for creating a request
type CreateRequestHandlerResponse struct {
	Body *contracts.RequestResponse
}

// CreateRequestHandler handles POST /requests
func (h *RequestHandler) CreateRequestHandler(ctx context.Context, req *CreateRequestHandlerRequest) (*CreateRequestHandlerResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	if req.Body.Title == "" {
		return nil, huma.Error422UnprocessableEntity("Title is required")
	}
	if req.Body.EntityType == "" {
		return nil, huma.Error422UnprocessableEntity("Entity type is required")
	}

	description := ""
	if req.Body.Description != nil {
		description = *req.Body.Description
	}
	request, err := h.requestService.CreateRequest(user.ID, req.Body.Title, description, req.Body.EntityType, req.Body.RequestedEntityID)
	if err != nil {
		logger.FromContext(ctx).Error("create_request_failed",
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to create request (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLog != nil {
		shared.GoSafe(ctx, "audit_log", func() {
			h.auditLog.LogAction(user.ID, "create_request", "request", request.ID, map[string]interface{}{
				"entity_type": req.Body.EntityType,
			})
		})
	}

	resp := buildRequestResponse(request, nil)
	return &CreateRequestHandlerResponse{Body: resp}, nil
}

// ============================================================================
// List Requests
// ============================================================================

// ListRequestsHandlerRequest represents the request for listing requests
type ListRequestsHandlerRequest struct {
	Status     string `query:"status" required:"false" doc:"Filter by status (pending, in_progress, pending_fulfillment, fulfilled, rejected, cancelled)"`
	EntityType string `query:"entity_type" required:"false" doc:"Filter by entity type (artist, release, label, show, venue, festival)"`
	Sort       string `query:"sort" required:"false" doc:"Sort by: newest, votes, oldest (default: votes)" example:"votes"`
	Limit      int    `query:"limit" required:"false" minimum:"1" maximum:"100" doc:"Max results (default 20)" example:"20"`
	Offset     int    `query:"offset" required:"false" minimum:"0" doc:"Offset for pagination" example:"0"`
}

// ListRequestsHandlerResponse represents the response for listing requests
type ListRequestsHandlerResponse struct {
	Body struct {
		Requests []*contracts.RequestResponse `json:"requests" doc:"List of requests"`
		Total    int64                        `json:"total" doc:"Total number of matching requests"`
	}
}

// ListRequestsHandler handles GET /requests
func (h *RequestHandler) ListRequestsHandler(ctx context.Context, req *ListRequestsHandlerRequest) (*ListRequestsHandlerResponse, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}

	requests, total, err := h.requestService.ListRequests(req.Status, req.EntityType, req.Sort, limit, req.Offset)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to fetch requests", err)
	}

	// Check if user is authenticated for including their votes
	user := middleware.GetUserFromContext(ctx)

	responses := make([]*contracts.RequestResponse, len(requests))
	for i := range requests {
		var userVote *int
		if user != nil {
			vote, err := h.requestService.GetUserVote(requests[i].ID, user.ID)
			if err == nil && vote != nil {
				userVote = &vote.Vote
			}
		}
		responses[i] = buildRequestResponse(&requests[i], userVote)
	}

	resp := &ListRequestsHandlerResponse{}
	resp.Body.Requests = responses
	resp.Body.Total = total

	return resp, nil
}

// ============================================================================
// Get Request
// ============================================================================

// GetRequestHandlerRequest represents the request for getting a single request
type GetRequestHandlerRequest struct {
	RequestID string `path:"request_id" doc:"Request ID" example:"1"`
}

// GetRequestHandlerResponse represents the response for getting a request
type GetRequestHandlerResponse struct {
	Body *contracts.RequestResponse
}

// GetRequestHandler handles GET /requests/{request_id}
func (h *RequestHandler) GetRequestHandler(ctx context.Context, req *GetRequestHandlerRequest) (*GetRequestHandlerResponse, error) {
	id, err := strconv.ParseUint(req.RequestID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid request ID")
	}

	request, err := h.requestService.GetRequest(uint(id))
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to fetch request")
	}
	if request == nil {
		return nil, huma.Error404NotFound("Request not found")
	}

	// Include user's vote if authenticated
	var userVote *int
	user := middleware.GetUserFromContext(ctx)
	if user != nil {
		vote, err := h.requestService.GetUserVote(request.ID, user.ID)
		if err == nil && vote != nil {
			userVote = &vote.Vote
		}
	}

	resp := buildRequestResponse(request, userVote)

	// PSY-917: resolve the referenced entity to a linkable slug + name so the
	// pending-fulfillment review panel can render a "View proposed {entity}"
	// link. Detail-path only — list responses skip this to avoid an N+1
	// resolve. Resolution failure is non-fatal: the request still renders,
	// just without the entity link.
	if request.RequestedEntityID != nil {
		if ref, refErr := h.requestService.ResolveEntityRef(request.EntityType, *request.RequestedEntityID); refErr != nil {
			logger.FromContext(ctx).Warn("request_entity_ref_resolve_failed",
				"request_id", request.ID, "entity_type", request.EntityType,
				"entity_id", *request.RequestedEntityID, "error", refErr.Error())
		} else if ref != nil {
			resp.RequestedEntitySlug = ref.Slug
			if ref.Name != "" {
				resp.RequestedEntityName = &ref.Name
			}
		}
	}

	return &GetRequestHandlerResponse{Body: resp}, nil
}

// ============================================================================
// Update Request
// ============================================================================

// UpdateRequestHandlerRequest represents the request for updating a request
type UpdateRequestHandlerRequest struct {
	RequestID string `path:"request_id" doc:"Request ID" example:"1"`
	Body      struct {
		Title       *string `json:"title,omitempty" required:"false" doc:"Request title"`
		Description *string `json:"description,omitempty" required:"false" doc:"Request description"`
	}
}

// UpdateRequestHandlerResponse represents the response for updating a request
type UpdateRequestHandlerResponse struct {
	Body *contracts.RequestResponse
}

// UpdateRequestHandler handles PUT /requests/{request_id}
func (h *RequestHandler) UpdateRequestHandler(ctx context.Context, req *UpdateRequestHandlerRequest) (*UpdateRequestHandlerResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	id, err := strconv.ParseUint(req.RequestID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid request ID")
	}

	request, err := h.requestService.UpdateRequest(uint(id), user.ID, req.Body.Title, req.Body.Description)
	if err != nil {
		mappedErr := mapRequestError(err)
		if mappedErr != nil {
			return nil, mappedErr
		}
		return nil, huma.Error500InternalServerError("Failed to update request")
	}

	// Audit log (fire and forget)
	if h.auditLog != nil {
		shared.GoSafe(ctx, "audit_log", func() {
			h.auditLog.LogAction(user.ID, "update_request", "request", uint(id), nil)
		})
	}

	resp := buildRequestResponse(request, nil)
	return &UpdateRequestHandlerResponse{Body: resp}, nil
}

// ============================================================================
// Delete Request
// ============================================================================

// DeleteRequestHandlerRequest represents the request for deleting a request
type DeleteRequestHandlerRequest struct {
	RequestID string `path:"request_id" doc:"Request ID" example:"1"`
}

// DeleteRequestHandler handles DELETE /requests/{request_id}
func (h *RequestHandler) DeleteRequestHandler(ctx context.Context, req *DeleteRequestHandlerRequest) (*struct{}, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	id, err := strconv.ParseUint(req.RequestID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid request ID")
	}

	err = h.requestService.DeleteRequest(uint(id), user.ID, user.IsAdmin)
	if err != nil {
		mappedErr := mapRequestError(err)
		if mappedErr != nil {
			return nil, mappedErr
		}
		return nil, huma.Error500InternalServerError("Failed to delete request")
	}

	// Audit log (fire and forget)
	if h.auditLog != nil {
		shared.GoSafe(ctx, "audit_log", func() {
			h.auditLog.LogAction(user.ID, "delete_request", "request", uint(id), nil)
		})
	}

	return nil, nil
}

// ============================================================================
// Vote
// ============================================================================

// VoteRequestHandlerRequest represents the request for voting on a request
type VoteRequestHandlerRequest struct {
	RequestID string `path:"request_id" doc:"Request ID" example:"1"`
	Body      struct {
		IsUpvote bool `json:"is_upvote" doc:"True for upvote, false for downvote"`
	}
}

// VoteRequestHandler handles POST /requests/{request_id}/vote
func (h *RequestHandler) VoteRequestHandler(ctx context.Context, req *VoteRequestHandlerRequest) (*struct{}, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	id, err := strconv.ParseUint(req.RequestID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid request ID")
	}

	err = h.requestService.Vote(uint(id), user.ID, req.Body.IsUpvote)
	if err != nil {
		mappedErr := mapRequestError(err)
		if mappedErr != nil {
			return nil, mappedErr
		}
		return nil, huma.Error500InternalServerError("Failed to vote")
	}

	return nil, nil
}

// ============================================================================
// Remove Vote
// ============================================================================

// RemoveVoteRequestHandlerRequest represents the request for removing a vote
type RemoveVoteRequestHandlerRequest struct {
	RequestID string `path:"request_id" doc:"Request ID" example:"1"`
}

// RemoveVoteRequestHandler handles DELETE /requests/{request_id}/vote
func (h *RequestHandler) RemoveVoteRequestHandler(ctx context.Context, req *RemoveVoteRequestHandlerRequest) (*struct{}, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	id, err := strconv.ParseUint(req.RequestID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid request ID")
	}

	err = h.requestService.RemoveVote(uint(id), user.ID)
	if err != nil {
		mappedErr := mapRequestError(err)
		if mappedErr != nil {
			return nil, mappedErr
		}
		return nil, huma.Error500InternalServerError("Failed to remove vote")
	}

	return nil, nil
}

// ============================================================================
// Fulfill Request
// ============================================================================

// FulfillRequestHandlerRequest represents the request for fulfilling a request
type FulfillRequestHandlerRequest struct {
	RequestID string `path:"request_id" doc:"Request ID" example:"1"`
	Body      struct {
		FulfilledEntityID *uint `json:"fulfilled_entity_id,omitempty" required:"false" doc:"Optional ID of the entity that fulfills this request"`
	}
}

// FulfillRequestHandler handles POST /requests/{request_id}/fulfill.
//
// PSY-748: open to any authenticated user. Submitting a fulfillment
// moves the request into pending_fulfillment; the original requester
// or an admin then has to approve (or reject) via the dedicated
// endpoints below. The pre-748 behavior — direct, unguarded write to
// fulfilled — let any caller hijack the request's entity link.
func (h *RequestHandler) FulfillRequestHandler(ctx context.Context, req *FulfillRequestHandlerRequest) (*struct{}, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	id, err := strconv.ParseUint(req.RequestID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid request ID")
	}

	// Resolve the request up front so the audit log can capture
	// requester_id + entity_type even if the fulfillment is later
	// rejected. Failing here is non-fatal: we degrade to logging just
	// what we have from the handler context.
	var requesterID uint
	var entityType string
	if r, getErr := h.requestService.GetRequest(uint(id)); getErr == nil && r != nil {
		requesterID = r.RequesterID
		entityType = r.EntityType
	}

	err = h.requestService.FulfillRequest(uint(id), user.ID, req.Body.FulfilledEntityID)
	if err != nil {
		mappedErr := mapRequestError(err)
		if mappedErr != nil {
			return nil, mappedErr
		}
		return nil, huma.Error500InternalServerError("Failed to submit fulfillment")
	}

	// Audit log (fire and forget). Includes requester_id (so the
	// observer can see WHO is being asked to approve), fulfiller_id
	// (the submitter), the proposed entity payload, and the resolved
	// entity_type — see Finding 7 / PSY-748 acceptance criteria.
	if h.auditLog != nil {
		metadata := map[string]interface{}{
			"requester_id": requesterID,
			"fulfiller_id": user.ID,
			"entity_type":  entityType,
		}
		if req.Body.FulfilledEntityID != nil {
			metadata["fulfilled_entity_id"] = *req.Body.FulfilledEntityID
		}
		// Audit action name stays "fulfill_request" for backward compat
		// with ContributionTimeline (frontend already maps that
		// discriminator to a contribution-feed entry). The metadata
		// expanded under PSY-748 — adding requester_id / fulfiller_id /
		// entity_type / fulfilled_entity_id so reviewers can see who is
		// being asked to approve what.
		shared.GoSafe(ctx, "audit_log", func() {
			h.auditLog.LogAction(user.ID, "fulfill_request", "request", uint(id), metadata)
		})
	}

	// Notify the requester (in-app, fire-and-forget) that a fulfillment now
	// awaits their approval. No-op on self-fulfill or when requesterID didn't
	// resolve above; never blocks the fulfillment response. PSY-890.
	shared.GoSafe(ctx, "request_fulfillment_notify", func() {
		if notifyErr := h.requestService.NotifyRequesterFulfillmentProposed(uint(id), requesterID, user.ID); notifyErr != nil {
			logger.FromContext(ctx).Warn("request_fulfillment_notify_failed",
				"request_id", id, "error", notifyErr.Error())
		}
	})

	return nil, nil
}

// ============================================================================
// Approve Fulfillment (PSY-748)
// ============================================================================

// ApproveFulfillmentHandlerRequest represents the request for approving
// a pending fulfillment.
type ApproveFulfillmentHandlerRequest struct {
	RequestID string `path:"request_id" doc:"Request ID" example:"1"`
}

// ApproveFulfillmentHandler handles POST /requests/{request_id}/approve-fulfillment.
// PSY-748: only the original requester or an admin may approve.
func (h *RequestHandler) ApproveFulfillmentHandler(ctx context.Context, req *ApproveFulfillmentHandlerRequest) (*struct{}, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	id, err := strconv.ParseUint(req.RequestID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid request ID")
	}

	// Resolve audit context before mutation so we log fulfiller +
	// entity even if the request row later changes.
	var fulfillerID *uint
	var entityType string
	var requesterID uint
	var fulfilledEntityID *uint
	if r, getErr := h.requestService.GetRequest(uint(id)); getErr == nil && r != nil {
		fulfillerID = r.FulfillerID
		entityType = r.EntityType
		requesterID = r.RequesterID
		fulfilledEntityID = r.RequestedEntityID
	}

	err = h.requestService.ApproveFulfillment(uint(id), user.ID, user.IsAdmin)
	if err != nil {
		mappedErr := mapRequestError(err)
		if mappedErr != nil {
			return nil, mappedErr
		}
		return nil, huma.Error500InternalServerError("Failed to approve fulfillment")
	}

	if h.auditLog != nil {
		metadata := map[string]interface{}{
			"requester_id": requesterID,
			"entity_type":  entityType,
		}
		if fulfillerID != nil {
			metadata["fulfiller_id"] = *fulfillerID
		}
		if fulfilledEntityID != nil {
			metadata["fulfilled_entity_id"] = *fulfilledEntityID
		}
		shared.GoSafe(ctx, "audit_log", func() {
			h.auditLog.LogAction(user.ID, "approve_fulfillment", "request", uint(id), metadata)
		})
	}

	return nil, nil
}

// ============================================================================
// Reject Fulfillment (PSY-748)
// ============================================================================

// RejectFulfillmentHandlerRequest represents the request for rejecting
// a pending fulfillment.
type RejectFulfillmentHandlerRequest struct {
	RequestID string `path:"request_id" doc:"Request ID" example:"1"`
}

// RejectFulfillmentHandler handles POST /requests/{request_id}/reject-fulfillment.
// PSY-748: only the original requester or an admin may reject. On
// success the request returns to "pending" with fulfiller_id and
// requested_entity_id cleared.
func (h *RequestHandler) RejectFulfillmentHandler(ctx context.Context, req *RejectFulfillmentHandlerRequest) (*struct{}, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	id, err := strconv.ParseUint(req.RequestID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid request ID")
	}

	// Resolve audit context before the mutation clears it.
	var fulfillerID *uint
	var entityType string
	var requesterID uint
	var fulfilledEntityID *uint
	if r, getErr := h.requestService.GetRequest(uint(id)); getErr == nil && r != nil {
		fulfillerID = r.FulfillerID
		entityType = r.EntityType
		requesterID = r.RequesterID
		fulfilledEntityID = r.RequestedEntityID
	}

	err = h.requestService.RejectFulfillment(uint(id), user.ID, user.IsAdmin)
	if err != nil {
		mappedErr := mapRequestError(err)
		if mappedErr != nil {
			return nil, mappedErr
		}
		return nil, huma.Error500InternalServerError("Failed to reject fulfillment")
	}

	if h.auditLog != nil {
		metadata := map[string]interface{}{
			"requester_id": requesterID,
			"entity_type":  entityType,
		}
		if fulfillerID != nil {
			metadata["fulfiller_id"] = *fulfillerID
		}
		if fulfilledEntityID != nil {
			metadata["fulfilled_entity_id"] = *fulfilledEntityID
		}
		shared.GoSafe(ctx, "audit_log", func() {
			h.auditLog.LogAction(user.ID, "reject_fulfillment", "request", uint(id), metadata)
		})
	}

	return nil, nil
}

// ============================================================================
// Close Request
// ============================================================================

// CloseRequestHandlerRequest represents the request for closing a request
type CloseRequestHandlerRequest struct {
	RequestID string `path:"request_id" doc:"Request ID" example:"1"`
}

// CloseRequestHandler handles POST /requests/{request_id}/close
func (h *RequestHandler) CloseRequestHandler(ctx context.Context, req *CloseRequestHandlerRequest) (*struct{}, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	id, err := strconv.ParseUint(req.RequestID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid request ID")
	}

	err = h.requestService.CloseRequest(uint(id), user.ID, user.IsAdmin)
	if err != nil {
		mappedErr := mapRequestError(err)
		if mappedErr != nil {
			return nil, mappedErr
		}
		return nil, huma.Error500InternalServerError("Failed to close request")
	}

	// Audit log (fire and forget)
	if h.auditLog != nil {
		shared.GoSafe(ctx, "audit_log", func() {
			h.auditLog.LogAction(user.ID, "close_request", "request", uint(id), nil)
		})
	}

	return nil, nil
}

// ============================================================================
// Helpers
// ============================================================================

// buildRequestResponse converts a communitym.Request to a RequestResponse.
func buildRequestResponse(request *communitym.Request, userVote *int) *contracts.RequestResponse {
	resp := &contracts.RequestResponse{
		ID:                request.ID,
		Title:             request.Title,
		Description:       request.Description,
		EntityType:        request.EntityType,
		RequestedEntityID: request.RequestedEntityID,
		Status:            request.Status,
		RequesterID:       request.RequesterID,
		FulfillerID:       request.FulfillerID,
		VoteScore:         request.VoteScore,
		Upvotes:           request.Upvotes,
		Downvotes:         request.Downvotes,
		WilsonScore:       request.WilsonScore(),
		FulfilledAt:       request.FulfilledAt,
		UserVote:          userVote,
		CreatedAt:         request.CreatedAt,
		UpdatedAt:         request.UpdatedAt,
	}

	if request.Requester.ID > 0 {
		resp.RequesterName = shared.ResolveUserName(&request.Requester)
		resp.RequesterUsername = shared.ResolveUserUsername(&request.Requester)
	}

	if request.Fulfiller != nil && request.Fulfiller.ID > 0 {
		resp.FulfillerName = shared.ResolveUserName(request.Fulfiller)
		resp.FulfillerUsername = shared.ResolveUserUsername(request.Fulfiller)
	}

	return resp
}

// mapRequestError converts a RequestError to an appropriate Huma HTTP error.
//
// EntityTypeMismatch + EntityNotFound surface as 400 because the request
// itself is fine — the caller's payload references the wrong target. We
// don't return 404 there because the request resource still exists.
// InvalidState (e.g. approving a non-pending_fulfillment request) is 409:
// a precondition on the resource state failed.
func mapRequestError(err error) error {
	var requestErr *apperrors.RequestError
	if errors.As(err, &requestErr) {
		switch requestErr.Code {
		case apperrors.CodeRequestNotFound:
			return huma.Error404NotFound(requestErr.Message)
		case apperrors.CodeRequestForbidden:
			return huma.Error403Forbidden(requestErr.Message)
		case apperrors.CodeRequestAlreadyFulfilled:
			return huma.Error409Conflict(requestErr.Message)
		case apperrors.CodeRequestEntityTypeMismatch,
			apperrors.CodeRequestEntityNotFound:
			return huma.Error400BadRequest(requestErr.Message)
		case apperrors.CodeRequestInvalidState:
			return huma.Error409Conflict(requestErr.Message)
		}
	}
	return nil
}
