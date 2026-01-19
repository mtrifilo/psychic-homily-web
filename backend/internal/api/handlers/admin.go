package handlers

import (
	"context"
	"fmt"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services"
)

// AdminHandler handles admin-related HTTP requests
type AdminHandler struct {
	showService    *services.ShowService
	venueService   *services.VenueService
	discordService *services.DiscordService
}

// NewAdminHandler creates a new admin handler
func NewAdminHandler(cfg *config.Config) *AdminHandler {
	return &AdminHandler{
		showService:    services.NewShowService(),
		venueService:   services.NewVenueService(),
		discordService: services.NewDiscordService(cfg),
	}
}

// GetPendingShowsRequest represents the HTTP request for listing pending shows
type GetPendingShowsRequest struct {
	Limit  int `query:"limit" default:"50" doc:"Number of shows to return (max 100)"`
	Offset int `query:"offset" default:"0" doc:"Offset for pagination"`
}

// GetPendingShowsResponse represents the HTTP response for listing pending shows
type GetPendingShowsResponse struct {
	Body struct {
		Shows []*services.ShowResponse `json:"shows"`
		Total int64                    `json:"total"`
	}
}

// ApproveShowRequest represents the HTTP request for approving a show
type ApproveShowRequest struct {
	ShowID string `path:"show_id" validate:"required" doc:"Show ID"`
	Body   struct {
		VerifyVenues bool `json:"verify_venues" doc:"Whether to also verify unverified venues associated with this show"`
	}
}

// ApproveShowResponse represents the HTTP response for approving a show
type ApproveShowResponse struct {
	Body services.ShowResponse `json:"body"`
}

// RejectShowRequest represents the HTTP request for rejecting a show
type RejectShowRequest struct {
	ShowID string `path:"show_id" validate:"required" doc:"Show ID"`
	Body   struct {
		Reason string `json:"reason" validate:"required" doc:"Reason for rejecting the show"`
	}
}

// RejectShowResponse represents the HTTP response for rejecting a show
type RejectShowResponse struct {
	Body services.ShowResponse `json:"body"`
}

// VerifyVenueRequest represents the HTTP request for verifying a venue
type VerifyVenueRequest struct {
	VenueID string `path:"venue_id" validate:"required" doc:"Venue ID"`
}

// VerifyVenueResponse represents the HTTP response for verifying a venue
type VerifyVenueResponse struct {
	Body services.VenueDetailResponse `json:"body"`
}

// GetPendingShowsHandler handles GET /admin/shows/pending
func (h *AdminHandler) GetPendingShowsHandler(ctx context.Context, req *GetPendingShowsRequest) (*GetPendingShowsResponse, error) {
	requestID := logger.GetRequestID(ctx)

	// Verify admin access
	user := middleware.GetUserFromContext(ctx)
	if user == nil || !user.IsAdmin {
		logger.FromContext(ctx).Warn("admin_access_denied",
			"user_id", getUserID(user),
			"request_id", requestID,
		)
		return nil, huma.Error403Forbidden("Admin access required")
	}

	// Validate limit
	limit := req.Limit
	if limit < 1 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	logger.FromContext(ctx).Debug("admin_pending_shows_attempt",
		"limit", limit,
		"offset", req.Offset,
	)

	// Get pending shows
	shows, total, err := h.showService.GetPendingShows(limit, req.Offset)
	if err != nil {
		logger.FromContext(ctx).Error("admin_pending_shows_failed",
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to get pending shows (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Debug("admin_pending_shows_success",
		"count", len(shows),
		"total", total,
	)

	return &GetPendingShowsResponse{
		Body: struct {
			Shows []*services.ShowResponse `json:"shows"`
			Total int64                    `json:"total"`
		}{
			Shows: shows,
			Total: total,
		},
	}, nil
}

// ApproveShowHandler handles POST /admin/shows/{show_id}/approve
func (h *AdminHandler) ApproveShowHandler(ctx context.Context, req *ApproveShowRequest) (*ApproveShowResponse, error) {
	requestID := logger.GetRequestID(ctx)

	// Verify admin access
	user := middleware.GetUserFromContext(ctx)
	if user == nil || !user.IsAdmin {
		logger.FromContext(ctx).Warn("admin_access_denied",
			"user_id", getUserID(user),
			"request_id", requestID,
		)
		return nil, huma.Error403Forbidden("Admin access required")
	}

	// Parse show ID
	showID, err := strconv.ParseUint(req.ShowID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid show ID")
	}

	logger.FromContext(ctx).Debug("admin_approve_show_attempt",
		"show_id", showID,
		"verify_venues", req.Body.VerifyVenues,
		"admin_id", user.ID,
	)

	// Approve the show
	show, err := h.showService.ApproveShow(uint(showID), req.Body.VerifyVenues)
	if err != nil {
		logger.FromContext(ctx).Error("admin_approve_show_failed",
			"show_id", showID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error422UnprocessableEntity(
			fmt.Sprintf("Failed to approve show: %s (request_id: %s)", err.Error(), requestID),
		)
	}

	logger.FromContext(ctx).Info("admin_approve_show_success",
		"show_id", showID,
		"verified_venues", req.Body.VerifyVenues,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	// Send Discord notification for show approval
	h.discordService.NotifyShowApproved(show)

	return &ApproveShowResponse{Body: *show}, nil
}

// RejectShowHandler handles POST /admin/shows/{show_id}/reject
func (h *AdminHandler) RejectShowHandler(ctx context.Context, req *RejectShowRequest) (*RejectShowResponse, error) {
	requestID := logger.GetRequestID(ctx)

	// Verify admin access
	user := middleware.GetUserFromContext(ctx)
	if user == nil || !user.IsAdmin {
		logger.FromContext(ctx).Warn("admin_access_denied",
			"user_id", getUserID(user),
			"request_id", requestID,
		)
		return nil, huma.Error403Forbidden("Admin access required")
	}

	// Parse show ID
	showID, err := strconv.ParseUint(req.ShowID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid show ID")
	}

	// Validate reason
	if req.Body.Reason == "" {
		return nil, huma.Error400BadRequest("Rejection reason is required")
	}

	logger.FromContext(ctx).Debug("admin_reject_show_attempt",
		"show_id", showID,
		"admin_id", user.ID,
	)

	// Reject the show
	show, err := h.showService.RejectShow(uint(showID), req.Body.Reason)
	if err != nil {
		logger.FromContext(ctx).Error("admin_reject_show_failed",
			"show_id", showID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error422UnprocessableEntity(
			fmt.Sprintf("Failed to reject show: %s (request_id: %s)", err.Error(), requestID),
		)
	}

	logger.FromContext(ctx).Info("admin_reject_show_success",
		"show_id", showID,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	// Send Discord notification for show rejection
	h.discordService.NotifyShowRejected(show, req.Body.Reason)

	return &RejectShowResponse{Body: *show}, nil
}

// VerifyVenueHandler handles POST /admin/venues/{venue_id}/verify
func (h *AdminHandler) VerifyVenueHandler(ctx context.Context, req *VerifyVenueRequest) (*VerifyVenueResponse, error) {
	requestID := logger.GetRequestID(ctx)

	// Verify admin access
	user := middleware.GetUserFromContext(ctx)
	if user == nil || !user.IsAdmin {
		logger.FromContext(ctx).Warn("admin_access_denied",
			"user_id", getUserID(user),
			"request_id", requestID,
		)
		return nil, huma.Error403Forbidden("Admin access required")
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
			fmt.Sprintf("Failed to verify venue: %s (request_id: %s)", err.Error(), requestID),
		)
	}

	logger.FromContext(ctx).Info("admin_verify_venue_success",
		"venue_id", venueID,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	return &VerifyVenueResponse{Body: *venue}, nil
}

// getUserID safely gets user ID or returns 0 if user is nil
func getUserID(user *models.User) uint {
	if user == nil {
		return 0
	}
	return user.ID
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
		Edits []*services.PendingVenueEditResponse `json:"edits"`
		Total int64                                 `json:"total"`
	}
}

// GetPendingVenueEditsHandler handles GET /admin/venues/pending-edits
func (h *AdminHandler) GetPendingVenueEditsHandler(ctx context.Context, req *GetPendingVenueEditsRequest) (*GetPendingVenueEditsResponse, error) {
	requestID := logger.GetRequestID(ctx)

	// Verify admin access
	user := middleware.GetUserFromContext(ctx)
	if user == nil || !user.IsAdmin {
		logger.FromContext(ctx).Warn("admin_access_denied",
			"user_id", getUserID(user),
			"request_id", requestID,
		)
		return nil, huma.Error403Forbidden("Admin access required")
	}

	// Validate limit
	limit := req.Limit
	if limit < 1 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	logger.FromContext(ctx).Debug("admin_pending_venue_edits_attempt",
		"limit", limit,
		"offset", req.Offset,
	)

	// Get pending venue edits
	edits, total, err := h.venueService.GetPendingVenueEdits(limit, req.Offset)
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
			Edits []*services.PendingVenueEditResponse `json:"edits"`
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
	Body services.VenueDetailResponse `json:"body"`
}

// ApproveVenueEditHandler handles POST /admin/venues/pending-edits/{edit_id}/approve
func (h *AdminHandler) ApproveVenueEditHandler(ctx context.Context, req *ApproveVenueEditRequest) (*ApproveVenueEditResponse, error) {
	requestID := logger.GetRequestID(ctx)

	// Verify admin access
	user := middleware.GetUserFromContext(ctx)
	if user == nil || !user.IsAdmin {
		logger.FromContext(ctx).Warn("admin_access_denied",
			"user_id", getUserID(user),
			"request_id", requestID,
		)
		return nil, huma.Error403Forbidden("Admin access required")
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
			fmt.Sprintf("Failed to approve venue edit: %s (request_id: %s)", err.Error(), requestID),
		)
	}

	logger.FromContext(ctx).Info("admin_approve_venue_edit_success",
		"edit_id", editID,
		"venue_id", venue.ID,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	return &ApproveVenueEditResponse{Body: *venue}, nil
}

// RejectVenueEditRequest represents the HTTP request for rejecting a venue edit
type RejectVenueEditRequest struct {
	EditID string `path:"edit_id" validate:"required" doc:"Pending venue edit ID"`
	Body   struct {
		Reason string `json:"reason" validate:"required" doc:"Reason for rejecting the edit"`
	}
}

// RejectVenueEditResponse represents the HTTP response for rejecting a venue edit
type RejectVenueEditResponse struct {
	Body services.PendingVenueEditResponse `json:"body"`
}

// RejectVenueEditHandler handles POST /admin/venues/pending-edits/{edit_id}/reject
func (h *AdminHandler) RejectVenueEditHandler(ctx context.Context, req *RejectVenueEditRequest) (*RejectVenueEditResponse, error) {
	requestID := logger.GetRequestID(ctx)

	// Verify admin access
	user := middleware.GetUserFromContext(ctx)
	if user == nil || !user.IsAdmin {
		logger.FromContext(ctx).Warn("admin_access_denied",
			"user_id", getUserID(user),
			"request_id", requestID,
		)
		return nil, huma.Error403Forbidden("Admin access required")
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
			fmt.Sprintf("Failed to reject venue edit: %s (request_id: %s)", err.Error(), requestID),
		)
	}

	logger.FromContext(ctx).Info("admin_reject_venue_edit_success",
		"edit_id", editID,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	return &RejectVenueEditResponse{Body: *edit}, nil
}
