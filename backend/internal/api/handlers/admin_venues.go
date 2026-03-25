package handlers

import (
	"context"
	"fmt"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/contracts"
)

// AdminVenueHandler handles admin venue management
type AdminVenueHandler struct {
	venueService    contracts.VenueServiceInterface
	auditLogService contracts.AuditLogServiceInterface
}

// NewAdminVenueHandler creates a new admin venue handler
func NewAdminVenueHandler(
	venueService contracts.VenueServiceInterface,
	auditLogService contracts.AuditLogServiceInterface,
) *AdminVenueHandler {
	return &AdminVenueHandler{
		venueService:    venueService,
		auditLogService: auditLogService,
	}
}

// VerifyVenueRequest represents the HTTP request for verifying a venue
type VerifyVenueRequest struct {
	VenueID string `path:"venue_id" validate:"required" doc:"Venue ID"`
}

// VerifyVenueResponse represents the HTTP response for verifying a venue
type VerifyVenueResponse struct {
	Body contracts.VenueDetailResponse `json:"body"`
}

// GetUnverifiedVenuesRequest represents the HTTP request for listing unverified venues
type GetUnverifiedVenuesRequest struct {
	Limit  int `query:"limit" default:"50" doc:"Number of venues to return (max 100)"`
	Offset int `query:"offset" default:"0" doc:"Offset for pagination"`
}

// GetUnverifiedVenuesResponse represents the HTTP response for listing unverified venues
type GetUnverifiedVenuesResponse struct {
	Body struct {
		Venues []*contracts.UnverifiedVenueResponse `json:"venues"`
		Total  int64                               `json:"total"`
	}
}

// VerifyVenueHandler handles POST /admin/venues/{venue_id}/verify
func (h *AdminVenueHandler) VerifyVenueHandler(ctx context.Context, req *VerifyVenueRequest) (*VerifyVenueResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	// Parse venue ID
	venueID, err := strconv.ParseUint(req.VenueID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid venue ID")
	}

	logger.FromContext(ctx).Debug("admin_verify_venue_attempt",
		"venue_id", venueID,
		"admin_id", user.ID,
	)

	// Verify the venue
	venue, err := h.venueService.VerifyVenue(uint(venueID))
	if err != nil {
		logger.FromContext(ctx).Error("admin_verify_venue_failed",
			"venue_id", venueID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error422UnprocessableEntity(
			fmt.Sprintf("Failed to verify venue (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Info("admin_verify_venue_success",
		"venue_id", venueID,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	// Audit log
	h.auditLogService.LogAction(user.ID, "verify_venue", "venue", uint(venueID), nil)

	return &VerifyVenueResponse{Body: *venue}, nil
}

// GetUnverifiedVenuesHandler handles GET /admin/venues/unverified
// Returns venues that have not been verified by an admin, for admin review.
func (h *AdminVenueHandler) GetUnverifiedVenuesHandler(ctx context.Context, req *GetUnverifiedVenuesRequest) (*GetUnverifiedVenuesResponse, error) {
	requestID := logger.GetRequestID(ctx)

	_, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	// Validate limit
	limit := req.Limit
	if limit < 1 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	logger.FromContext(ctx).Debug("admin_unverified_venues_attempt",
		"limit", limit,
		"offset", offset,
	)

	// Get unverified venues
	venues, total, err := h.venueService.GetUnverifiedVenues(limit, offset)
	if err != nil {
		logger.FromContext(ctx).Error("admin_unverified_venues_failed",
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to get unverified venues (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Debug("admin_unverified_venues_success",
		"count", len(venues),
		"total", total,
	)

	resp := &GetUnverifiedVenuesResponse{}
	resp.Body.Venues = venues
	resp.Body.Total = total
	return resp, nil
}

// ============================================================================
// Pending Venue Edit Admin Handlers
// ============================================================================

// GetPendingVenueEditsRequest represents the HTTP request for listing pending venue edits
type GetPendingVenueEditsRequest struct {
	Limit  int `query:"limit" default:"50" doc:"Number of edits to return (max 100)"`
	Offset int `query:"offset" default:"0" doc:"Offset for pagination"`
}

// GetPendingVenueEditsResponse represents the HTTP response for listing pending venue edits
type GetPendingVenueEditsResponse struct {
	Body struct {
		Edits []*contracts.PendingVenueEditResponse `json:"edits"`
		Total int64                                 `json:"total"`
	}
}

// GetPendingVenueEditsHandler handles GET /admin/venues/pending-edits
func (h *AdminVenueHandler) GetPendingVenueEditsHandler(ctx context.Context, req *GetPendingVenueEditsRequest) (*GetPendingVenueEditsResponse, error) {
	requestID := logger.GetRequestID(ctx)

	_, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	// Validate limit
	limit := req.Limit
	if limit < 1 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	logger.FromContext(ctx).Debug("admin_pending_venue_edits_attempt",
		"limit", limit,
		"offset", offset,
	)

	// Get pending venue edits
	edits, total, err := h.venueService.GetPendingVenueEdits(limit, offset)
	if err != nil {
		logger.FromContext(ctx).Error("admin_pending_venue_edits_failed",
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to get pending venue edits (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Debug("admin_pending_venue_edits_success",
		"count", len(edits),
		"total", total,
	)

	return &GetPendingVenueEditsResponse{
		Body: struct {
			Edits []*contracts.PendingVenueEditResponse `json:"edits"`
			Total int64                                 `json:"total"`
		}{
			Edits: edits,
			Total: total,
		},
	}, nil
}

// ApproveVenueEditRequest represents the HTTP request for approving a venue edit
type ApproveVenueEditRequest struct {
	EditID string `path:"edit_id" validate:"required" doc:"Pending venue edit ID"`
}

// ApproveVenueEditResponse represents the HTTP response for approving a venue edit
type ApproveVenueEditResponse struct {
	Body contracts.VenueDetailResponse `json:"body"`
}

// ApproveVenueEditHandler handles POST /admin/venues/pending-edits/{edit_id}/approve
func (h *AdminVenueHandler) ApproveVenueEditHandler(ctx context.Context, req *ApproveVenueEditRequest) (*ApproveVenueEditResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	// Parse edit ID
	editID, err := strconv.ParseUint(req.EditID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid edit ID")
	}

	logger.FromContext(ctx).Debug("admin_approve_venue_edit_attempt",
		"edit_id", editID,
		"admin_id", user.ID,
	)

	// Approve the venue edit
	venue, err := h.venueService.ApproveVenueEdit(uint(editID), user.ID)
	if err != nil {
		logger.FromContext(ctx).Error("admin_approve_venue_edit_failed",
			"edit_id", editID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error422UnprocessableEntity(
			fmt.Sprintf("Failed to approve venue edit (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Info("admin_approve_venue_edit_success",
		"edit_id", editID,
		"venue_id", venue.ID,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	// Audit log
	h.auditLogService.LogAction(user.ID, "approve_venue_edit", "venue_edit", uint(editID), map[string]interface{}{
		"venue_id": venue.ID,
	})

	return &ApproveVenueEditResponse{Body: *venue}, nil
}

// RejectVenueEditRequest represents the HTTP request for rejecting a venue edit
type RejectVenueEditRequest struct {
	EditID string `path:"edit_id" validate:"required" doc:"Pending venue edit ID"`
	Body   struct {
		Reason string `json:"reason" validate:"required,max=1000" doc:"Reason for rejecting the edit"`
	}
}

// RejectVenueEditResponse represents the HTTP response for rejecting a venue edit
type RejectVenueEditResponse struct {
	Body contracts.PendingVenueEditResponse `json:"body"`
}

// RejectVenueEditHandler handles POST /admin/venues/pending-edits/{edit_id}/reject
func (h *AdminVenueHandler) RejectVenueEditHandler(ctx context.Context, req *RejectVenueEditRequest) (*RejectVenueEditResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	// Parse edit ID
	editID, err := strconv.ParseUint(req.EditID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid edit ID")
	}

	// Validate reason
	if req.Body.Reason == "" {
		return nil, huma.Error400BadRequest("Rejection reason is required")
	}

	logger.FromContext(ctx).Debug("admin_reject_venue_edit_attempt",
		"edit_id", editID,
		"admin_id", user.ID,
	)

	// Reject the venue edit
	edit, err := h.venueService.RejectVenueEdit(uint(editID), user.ID, req.Body.Reason)
	if err != nil {
		logger.FromContext(ctx).Error("admin_reject_venue_edit_failed",
			"edit_id", editID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error422UnprocessableEntity(
			fmt.Sprintf("Failed to reject venue edit (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Info("admin_reject_venue_edit_success",
		"edit_id", editID,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	// Audit log
	h.auditLogService.LogAction(user.ID, "reject_venue_edit", "venue_edit", uint(editID), map[string]interface{}{
		"reason": req.Body.Reason,
	})

	return &RejectVenueEditResponse{Body: *edit}, nil
}
