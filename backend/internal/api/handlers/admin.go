package handlers

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services"
)

// AdminHandler handles admin-related HTTP requests
type AdminHandler struct {
	showService           services.ShowServiceInterface
	venueService          services.VenueServiceInterface
	discordService        services.DiscordServiceInterface
	musicDiscoveryService services.MusicDiscoveryServiceInterface
	discoveryService      services.DiscoveryServiceInterface
	apiTokenService       services.APITokenServiceInterface
	dataSyncService       services.DataSyncServiceInterface
	auditLogService       services.AuditLogServiceInterface
	userService           services.UserServiceInterface
	adminStatsService     services.AdminStatsServiceInterface
}

// NewAdminHandler creates a new admin handler
func NewAdminHandler(
	showService services.ShowServiceInterface,
	venueService services.VenueServiceInterface,
	discordService services.DiscordServiceInterface,
	musicDiscoveryService services.MusicDiscoveryServiceInterface,
	discoveryService services.DiscoveryServiceInterface,
	apiTokenService services.APITokenServiceInterface,
	dataSyncService services.DataSyncServiceInterface,
	auditLogService services.AuditLogServiceInterface,
	userService services.UserServiceInterface,
	adminStatsService services.AdminStatsServiceInterface,
) *AdminHandler {
	return &AdminHandler{
		showService:           showService,
		venueService:          venueService,
		discordService:        discordService,
		musicDiscoveryService: musicDiscoveryService,
		discoveryService:      discoveryService,
		apiTokenService:       apiTokenService,
		dataSyncService:       dataSyncService,
		auditLogService:       auditLogService,
		userService:           userService,
		adminStatsService:     adminStatsService,
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

// GetRejectedShowsRequest represents the HTTP request for listing rejected shows
type GetRejectedShowsRequest struct {
	Limit  int    `query:"limit" default:"50" doc:"Number of shows to return (max 100)"`
	Offset int    `query:"offset" default:"0" doc:"Offset for pagination"`
	Search string `query:"search" doc:"Search by show title or rejection reason"`
}

// GetRejectedShowsResponse represents the HTTP response for listing rejected shows
type GetRejectedShowsResponse struct {
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
		Reason string `json:"reason" validate:"required,max=1000" doc:"Reason for rejecting the show"`
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

// GetUnverifiedVenuesRequest represents the HTTP request for listing unverified venues
type GetUnverifiedVenuesRequest struct {
	Limit  int `query:"limit" default:"50" doc:"Number of venues to return (max 100)"`
	Offset int `query:"offset" default:"0" doc:"Offset for pagination"`
}

// GetUnverifiedVenuesResponse represents the HTTP response for listing unverified venues
type GetUnverifiedVenuesResponse struct {
	Body struct {
		Venues []*services.UnverifiedVenueResponse `json:"venues"`
		Total  int64                               `json:"total"`
	}
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

	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	logger.FromContext(ctx).Debug("admin_pending_shows_attempt",
		"limit", limit,
		"offset", offset,
	)

	// Get pending shows
	shows, total, err := h.showService.GetPendingShows(limit, offset)
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

// GetRejectedShowsHandler handles GET /admin/shows/rejected
func (h *AdminHandler) GetRejectedShowsHandler(ctx context.Context, req *GetRejectedShowsRequest) (*GetRejectedShowsResponse, error) {
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

	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	logger.FromContext(ctx).Debug("admin_rejected_shows_attempt",
		"limit", limit,
		"offset", offset,
		"search", req.Search,
	)

	// Get rejected shows
	shows, total, err := h.showService.GetRejectedShows(limit, offset, req.Search)
	if err != nil {
		logger.FromContext(ctx).Error("admin_rejected_shows_failed",
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to get rejected shows (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Debug("admin_rejected_shows_success",
		"count", len(shows),
		"total", total,
	)

	return &GetRejectedShowsResponse{
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
			fmt.Sprintf("Failed to approve show (request_id: %s)", requestID),
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

	// Audit log
	h.auditLogService.LogAction(user.ID, "approve_show", "show", uint(showID), map[string]interface{}{
		"verify_venues": req.Body.VerifyVenues,
	})

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
			fmt.Sprintf("Failed to reject show (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Info("admin_reject_show_success",
		"show_id", showID,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	// Send Discord notification for show rejection
	h.discordService.NotifyShowRejected(show, req.Body.Reason)

	// Audit log
	h.auditLogService.LogAction(user.ID, "reject_show", "show", uint(showID), map[string]interface{}{
		"reason": req.Body.Reason,
	})

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
func (h *AdminHandler) GetUnverifiedVenuesHandler(ctx context.Context, req *GetUnverifiedVenuesRequest) (*GetUnverifiedVenuesResponse, error) {
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

// ============================================================================
// Show Import Admin Handlers
// ============================================================================

// ImportShowPreviewRequest represents the HTTP request for previewing a show import
type ImportShowPreviewRequest struct {
	Body struct {
		// Content is the base64-encoded markdown file content
		Content string `json:"content" validate:"required" doc:"Base64-encoded markdown file content"`
	}
}

// ImportShowPreviewResponse represents the HTTP response for previewing a show import
type ImportShowPreviewResponse struct {
	Body services.ImportPreviewResponse `json:"body"`
}

// ImportShowPreviewHandler handles POST /admin/shows/import/preview
func (h *AdminHandler) ImportShowPreviewHandler(ctx context.Context, req *ImportShowPreviewRequest) (*ImportShowPreviewResponse, error) {
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

	// Decode base64 content
	content, err := base64.StdEncoding.DecodeString(req.Body.Content)
	if err != nil {
		logger.FromContext(ctx).Warn("import_preview_decode_failed",
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error400BadRequest("Invalid base64 content")
	}

	logger.FromContext(ctx).Debug("admin_import_preview_attempt",
		"content_size", len(content),
		"admin_id", user.ID,
	)

	// Preview the import
	preview, err := h.showService.PreviewShowImport(content)
	if err != nil {
		logger.FromContext(ctx).Error("admin_import_preview_failed",
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error422UnprocessableEntity(
			fmt.Sprintf("Failed to preview import (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Debug("admin_import_preview_success",
		"can_import", preview.CanImport,
		"warning_count", len(preview.Warnings),
		"venue_count", len(preview.Venues),
		"artist_count", len(preview.Artists),
	)

	return &ImportShowPreviewResponse{Body: *preview}, nil
}

// ImportShowConfirmRequest represents the HTTP request for confirming a show import
type ImportShowConfirmRequest struct {
	Body struct {
		// Content is the base64-encoded markdown file content
		Content string `json:"content" validate:"required" doc:"Base64-encoded markdown file content"`
	}
}

// ImportShowConfirmResponse represents the HTTP response for confirming a show import
type ImportShowConfirmResponse struct {
	Body services.ShowResponse `json:"body"`
}

// ImportShowConfirmHandler handles POST /admin/shows/import/confirm
func (h *AdminHandler) ImportShowConfirmHandler(ctx context.Context, req *ImportShowConfirmRequest) (*ImportShowConfirmResponse, error) {
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

	// Decode base64 content
	content, err := base64.StdEncoding.DecodeString(req.Body.Content)
	if err != nil {
		logger.FromContext(ctx).Warn("import_confirm_decode_failed",
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error400BadRequest("Invalid base64 content")
	}

	logger.FromContext(ctx).Debug("admin_import_confirm_attempt",
		"content_size", len(content),
		"admin_id", user.ID,
	)

	// Confirm the import (admin imports auto-verify venues)
	show, err := h.showService.ConfirmShowImport(content, true)
	if err != nil {
		logger.FromContext(ctx).Error("admin_import_confirm_failed",
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error422UnprocessableEntity(
			fmt.Sprintf("Failed to import show (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Info("admin_import_confirm_success",
		"show_id", show.ID,
		"title", show.Title,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	// Send Discord notification for new show
	h.discordService.NotifyNewShow(show, "")

	// Trigger music discovery for any newly created artists
	for _, artist := range show.Artists {
		if artist.IsNewArtist != nil && *artist.IsNewArtist {
			h.musicDiscoveryService.DiscoverMusicForArtist(artist.ID, artist.Name)
		}
	}

	return &ImportShowConfirmResponse{Body: *show}, nil
}

// ============================================================================
// Admin Show Export/Import Bulk Handlers (for CLI)
// ============================================================================

// GetAdminShowsRequest represents the HTTP request for listing all shows (admin)
type GetAdminShowsRequest struct {
	Limit    int    `query:"limit" default:"50" doc:"Number of shows to return (max 100)"`
	Offset   int    `query:"offset" default:"0" doc:"Offset for pagination"`
	Status   string `query:"status" doc:"Filter by status (pending, approved, rejected, private)"`
	FromDate string `query:"from_date" doc:"Filter shows from this date (RFC3339 format)"`
	ToDate   string `query:"to_date" doc:"Filter shows until this date (RFC3339 format)"`
	City     string `query:"city" doc:"Filter by city"`
}

// GetAdminShowsResponse represents the HTTP response for listing all shows (admin)
type GetAdminShowsResponse struct {
	Body struct {
		Shows []*services.ShowResponse `json:"shows"`
		Total int64                    `json:"total"`
	}
}

// GetAdminShowsHandler handles GET /admin/shows
// Returns paginated show list with full details for admin export purposes
func (h *AdminHandler) GetAdminShowsHandler(ctx context.Context, req *GetAdminShowsRequest) (*GetAdminShowsResponse, error) {
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

	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	logger.FromContext(ctx).Debug("admin_shows_list_attempt",
		"limit", limit,
		"offset", offset,
		"status", req.Status,
		"from_date", req.FromDate,
		"to_date", req.ToDate,
		"city", req.City,
	)

	// Build filters
	filters := services.AdminShowFilters{
		Status:   req.Status,
		FromDate: req.FromDate,
		ToDate:   req.ToDate,
		City:     req.City,
	}

	// Get shows
	shows, total, err := h.showService.GetAdminShows(limit, offset, filters)
	if err != nil {
		logger.FromContext(ctx).Error("admin_shows_list_failed",
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to get shows (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Debug("admin_shows_list_success",
		"count", len(shows),
		"total", total,
	)

	return &GetAdminShowsResponse{
		Body: struct {
			Shows []*services.ShowResponse `json:"shows"`
			Total int64                    `json:"total"`
		}{
			Shows: shows,
			Total: total,
		},
	}, nil
}

// BulkExportShowsRequest represents the HTTP request for bulk exporting shows
type BulkExportShowsRequest struct {
	Body struct {
		ShowIDs []uint `json:"show_ids" validate:"required,min=1" doc:"IDs of shows to export"`
	}
}

// BulkExportShowsResponse represents the HTTP response for bulk exporting shows
type BulkExportShowsResponse struct {
	Body struct {
		Exports []string `json:"exports" doc:"Base64-encoded markdown exports"`
	}
}

// BulkExportShowsHandler handles POST /admin/shows/export/bulk
// Exports multiple shows as base64-encoded markdown
func (h *AdminHandler) BulkExportShowsHandler(ctx context.Context, req *BulkExportShowsRequest) (*BulkExportShowsResponse, error) {
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

	if len(req.Body.ShowIDs) == 0 {
		return nil, huma.Error400BadRequest("At least one show ID is required")
	}

	if len(req.Body.ShowIDs) > 50 {
		return nil, huma.Error400BadRequest("Maximum 50 shows can be exported at once")
	}

	logger.FromContext(ctx).Debug("admin_bulk_export_attempt",
		"show_count", len(req.Body.ShowIDs),
		"admin_id", user.ID,
	)

	// Export each show
	exports := make([]string, 0, len(req.Body.ShowIDs))
	for _, showID := range req.Body.ShowIDs {
		content, _, err := h.showService.ExportShowToMarkdown(showID)
		if err != nil {
			logger.FromContext(ctx).Error("admin_bulk_export_show_failed",
				"show_id", showID,
				"error", err.Error(),
				"request_id", requestID,
			)
			return nil, huma.Error422UnprocessableEntity(
				fmt.Sprintf("Failed to export show %d (request_id: %s)", showID, requestID),
			)
		}
		exports = append(exports, base64.StdEncoding.EncodeToString(content))
	}

	logger.FromContext(ctx).Info("admin_bulk_export_success",
		"show_count", len(exports),
		"admin_id", user.ID,
		"request_id", requestID,
	)

	return &BulkExportShowsResponse{
		Body: struct {
			Exports []string `json:"exports" doc:"Base64-encoded markdown exports"`
		}{
			Exports: exports,
		},
	}, nil
}

// BulkImportPreviewRequest represents the HTTP request for bulk import preview
type BulkImportPreviewRequest struct {
	Body struct {
		Shows []string `json:"shows" validate:"required,min=1" doc:"Base64-encoded markdown content for each show"`
	}
}

// BulkImportPreviewSummary represents a summary of the bulk import preview
type BulkImportPreviewSummary struct {
	TotalShows      int `json:"total_shows"`
	NewArtists      int `json:"new_artists"`
	NewVenues       int `json:"new_venues"`
	ExistingArtists int `json:"existing_artists"`
	ExistingVenues  int `json:"existing_venues"`
	WarningCount    int `json:"warning_count"`
	CanImportAll    bool `json:"can_import_all"`
}

// BulkImportPreviewResponse represents the HTTP response for bulk import preview
type BulkImportPreviewResponse struct {
	Body struct {
		Previews []services.ImportPreviewResponse `json:"previews"`
		Summary  BulkImportPreviewSummary         `json:"summary"`
	}
}

// BulkImportPreviewHandler handles POST /admin/shows/import/bulk/preview
// Previews import of multiple shows with conflict detection
func (h *AdminHandler) BulkImportPreviewHandler(ctx context.Context, req *BulkImportPreviewRequest) (*BulkImportPreviewResponse, error) {
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

	if len(req.Body.Shows) == 0 {
		return nil, huma.Error400BadRequest("At least one show is required")
	}

	if len(req.Body.Shows) > 50 {
		return nil, huma.Error400BadRequest("Maximum 50 shows can be imported at once")
	}

	logger.FromContext(ctx).Debug("admin_bulk_import_preview_attempt",
		"show_count", len(req.Body.Shows),
		"admin_id", user.ID,
	)

	// Preview each show
	previews := make([]services.ImportPreviewResponse, 0, len(req.Body.Shows))
	summary := BulkImportPreviewSummary{
		TotalShows:   len(req.Body.Shows),
		CanImportAll: true,
	}

	for i, encodedContent := range req.Body.Shows {
		content, err := base64.StdEncoding.DecodeString(encodedContent)
		if err != nil {
			logger.FromContext(ctx).Warn("admin_bulk_import_preview_decode_failed",
				"show_index", i,
				"error", err.Error(),
				"request_id", requestID,
			)
			return nil, huma.Error400BadRequest(fmt.Sprintf("Invalid base64 content for show %d", i+1))
		}

		preview, err := h.showService.PreviewShowImport(content)
		if err != nil {
			logger.FromContext(ctx).Error("admin_bulk_import_preview_failed",
				"show_index", i,
				"error", err.Error(),
				"request_id", requestID,
			)
			return nil, huma.Error422UnprocessableEntity(
				fmt.Sprintf("Failed to preview show %d (request_id: %s)", i+1, requestID),
			)
		}

		previews = append(previews, *preview)

		// Update summary
		for _, venue := range preview.Venues {
			if venue.WillCreate {
				summary.NewVenues++
			} else {
				summary.ExistingVenues++
			}
		}
		for _, artist := range preview.Artists {
			if artist.WillCreate {
				summary.NewArtists++
			} else {
				summary.ExistingArtists++
			}
		}
		summary.WarningCount += len(preview.Warnings)
		if !preview.CanImport {
			summary.CanImportAll = false
		}
	}

	logger.FromContext(ctx).Debug("admin_bulk_import_preview_success",
		"show_count", len(previews),
		"new_artists", summary.NewArtists,
		"new_venues", summary.NewVenues,
		"warnings", summary.WarningCount,
	)

	return &BulkImportPreviewResponse{
		Body: struct {
			Previews []services.ImportPreviewResponse `json:"previews"`
			Summary  BulkImportPreviewSummary         `json:"summary"`
		}{
			Previews: previews,
			Summary:  summary,
		},
	}, nil
}

// BulkImportConfirmRequest represents the HTTP request for bulk import confirmation
type BulkImportConfirmRequest struct {
	Body struct {
		Shows []string `json:"shows" validate:"required,min=1" doc:"Base64-encoded markdown content for each show"`
	}
}

// BulkImportResult represents the result of importing a single show
type BulkImportResult struct {
	Success bool                    `json:"success"`
	Show    *services.ShowResponse  `json:"show,omitempty"`
	Error   string                  `json:"error,omitempty"`
}

// BulkImportConfirmResponse represents the HTTP response for bulk import confirmation
type BulkImportConfirmResponse struct {
	Body struct {
		Results      []BulkImportResult `json:"results"`
		SuccessCount int                `json:"success_count"`
		ErrorCount   int                `json:"error_count"`
	}
}

// BulkImportConfirmHandler handles POST /admin/shows/import/bulk/confirm
// Executes the import of multiple shows
func (h *AdminHandler) BulkImportConfirmHandler(ctx context.Context, req *BulkImportConfirmRequest) (*BulkImportConfirmResponse, error) {
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

	if len(req.Body.Shows) == 0 {
		return nil, huma.Error400BadRequest("At least one show is required")
	}

	if len(req.Body.Shows) > 50 {
		return nil, huma.Error400BadRequest("Maximum 50 shows can be imported at once")
	}

	logger.FromContext(ctx).Debug("admin_bulk_import_confirm_attempt",
		"show_count", len(req.Body.Shows),
		"admin_id", user.ID,
	)

	// Import each show
	results := make([]BulkImportResult, 0, len(req.Body.Shows))
	successCount := 0
	errorCount := 0

	for i, encodedContent := range req.Body.Shows {
		content, err := base64.StdEncoding.DecodeString(encodedContent)
		if err != nil {
			results = append(results, BulkImportResult{
				Success: false,
				Error:   "Invalid base64 content",
			})
			errorCount++
			continue
		}

		show, err := h.showService.ConfirmShowImport(content, true)
		if err != nil {
			results = append(results, BulkImportResult{
				Success: false,
				Error:   "Failed to import show",
			})
			errorCount++
			logger.FromContext(ctx).Warn("admin_bulk_import_show_failed",
				"show_index", i,
				"error", err.Error(),
			)
			continue
		}

		results = append(results, BulkImportResult{
			Success: true,
			Show:    show,
		})
		successCount++

		// Send Discord notification for new show
		h.discordService.NotifyNewShow(show, "")

		// Trigger music discovery for any newly created artists
		for _, artist := range show.Artists {
			if artist.IsNewArtist != nil && *artist.IsNewArtist {
				h.musicDiscoveryService.DiscoverMusicForArtist(artist.ID, artist.Name)
			}
		}
	}

	logger.FromContext(ctx).Info("admin_bulk_import_confirm_complete",
		"success_count", successCount,
		"error_count", errorCount,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	return &BulkImportConfirmResponse{
		Body: struct {
			Results      []BulkImportResult `json:"results"`
			SuccessCount int                `json:"success_count"`
			ErrorCount   int                `json:"error_count"`
		}{
			Results:      results,
			SuccessCount: successCount,
			ErrorCount:   errorCount,
		},
	}, nil
}

// ============================================================================
// Discovery Import Handlers (for local discovery app)
// ============================================================================

// DiscoveryImportEventInput represents a single discovered event for import
type DiscoveryImportEventInput struct {
	ID        string   `json:"id" doc:"External event ID from the venue's system"`
	Title     string   `json:"title" doc:"Event title"`
	Date      string   `json:"date" doc:"Event date in ISO format (YYYY-MM-DD)"`
	Venue     string   `json:"venue" doc:"Venue name"`
	VenueSlug string   `json:"venueSlug" doc:"Venue identifier (e.g., valley-bar)"`
	ImageURL  *string  `json:"imageUrl,omitempty" doc:"Event image URL"`
	DoorsTime *string  `json:"doorsTime,omitempty" doc:"Doors time (e.g., 6:30 pm)"`
	ShowTime  *string  `json:"showTime,omitempty" doc:"Show time (e.g., 7:00 pm)"`
	TicketURL *string  `json:"ticketUrl,omitempty" doc:"Ticket purchase URL"`
	Artists        []string `json:"artists" doc:"List of artist names"`
	ScrapedAt      string   `json:"scrapedAt" doc:"When the event was scraped (ISO timestamp)"`
	Price          *string  `json:"price,omitempty" doc:"Price string (e.g., $18, Free)"`
	AgeRestriction *string  `json:"ageRestriction,omitempty" doc:"Age restriction (e.g., 16+, All Ages)"`
	IsSoldOut      *bool    `json:"isSoldOut,omitempty" doc:"Whether the event is sold out"`
	IsCancelled    *bool    `json:"isCancelled,omitempty" doc:"Whether the event is cancelled"`
}

// DiscoveryImportRequest represents the HTTP request for importing discovered events
type DiscoveryImportRequest struct {
	Body struct {
		Events       []DiscoveryImportEventInput `json:"events" validate:"required,min=1" doc:"Array of discovered events to import"`
		DryRun       bool                        `json:"dryRun" doc:"If true, preview import without persisting"`
		AllowUpdates bool                        `json:"allowUpdates" doc:"If true, update existing shows with new data"`
	}
}

// DiscoveryImportResponse represents the HTTP response for importing discovered events
type DiscoveryImportResponse struct {
	Body services.ImportResult `json:"body"`
}

// DiscoveryImportHandler handles POST /admin/discovery/import
func (h *AdminHandler) DiscoveryImportHandler(ctx context.Context, req *DiscoveryImportRequest) (*DiscoveryImportResponse, error) {
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

	if len(req.Body.Events) == 0 {
		return nil, huma.Error400BadRequest("At least one event is required")
	}

	if len(req.Body.Events) > 100 {
		return nil, huma.Error400BadRequest("Maximum 100 events can be imported at once")
	}

	logger.FromContext(ctx).Debug("admin_discovery_import_attempt",
		"event_count", len(req.Body.Events),
		"dry_run", req.Body.DryRun,
		"admin_id", user.ID,
	)

	// Convert input events to DiscoveredEvent format
	events := make([]services.DiscoveredEvent, len(req.Body.Events))
	for i, e := range req.Body.Events {
		events[i] = services.DiscoveredEvent{
			ID:             e.ID,
			Title:          e.Title,
			Date:           e.Date,
			Venue:          e.Venue,
			VenueSlug:      e.VenueSlug,
			ImageURL:       e.ImageURL,
			DoorsTime:      e.DoorsTime,
			ShowTime:       e.ShowTime,
			TicketURL:      e.TicketURL,
			Artists:        e.Artists,
			ScrapedAt:      e.ScrapedAt,
			Price:          e.Price,
			AgeRestriction: e.AgeRestriction,
			IsSoldOut:      e.IsSoldOut,
			IsCancelled:    e.IsCancelled,
		}
	}

	// Import events
	result, err := h.discoveryService.ImportEvents(events, req.Body.DryRun, req.Body.AllowUpdates)
	if err != nil {
		logger.FromContext(ctx).Error("admin_discovery_import_failed",
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to import events (request_id: %s)", requestID),
		)
	}

	action := "imported"
	if req.Body.DryRun {
		action = "previewed"
	}

	logger.FromContext(ctx).Info("admin_discovery_import_success",
		"action", action,
		"total", result.Total,
		"imported", result.Imported,
		"duplicates", result.Duplicates,
		"rejected", result.Rejected,
		"pending_review", result.PendingReview,
		"errors", result.Errors,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	return &DiscoveryImportResponse{Body: *result}, nil
}

// ============================================================================
// Discovery Check Handlers (check if events already exist)
// ============================================================================

// DiscoveryCheckEventInput represents a single event to check
type DiscoveryCheckEventInput struct {
	ID        string `json:"id" doc:"External event ID from the venue's system"`
	VenueSlug string `json:"venueSlug" doc:"Venue identifier (e.g., valley-bar)"`
}

// DiscoveryCheckRequest represents the HTTP request for checking discovered events
type DiscoveryCheckRequest struct {
	Body struct {
		Events []DiscoveryCheckEventInput `json:"events" validate:"required,min=1" doc:"Array of events to check"`
	}
}

// DiscoveryCheckResponse represents the HTTP response for checking discovered events
type DiscoveryCheckResponse struct {
	Body services.CheckEventsResult `json:"body"`
}

// DiscoveryCheckHandler handles POST /admin/discovery/check
func (h *AdminHandler) DiscoveryCheckHandler(ctx context.Context, req *DiscoveryCheckRequest) (*DiscoveryCheckResponse, error) {
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

	if len(req.Body.Events) == 0 {
		return nil, huma.Error400BadRequest("At least one event is required")
	}

	if len(req.Body.Events) > 200 {
		return nil, huma.Error400BadRequest("Maximum 200 events can be checked at once")
	}

	logger.FromContext(ctx).Debug("admin_discovery_check_attempt",
		"event_count", len(req.Body.Events),
		"admin_id", user.ID,
	)

	// Convert input to service types
	events := make([]services.CheckEventInput, len(req.Body.Events))
	for i, e := range req.Body.Events {
		events[i] = services.CheckEventInput{
			ID:        e.ID,
			VenueSlug: e.VenueSlug,
		}
	}

	result, err := h.discoveryService.CheckEvents(events)
	if err != nil {
		logger.FromContext(ctx).Error("admin_discovery_check_failed",
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to check events (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Debug("admin_discovery_check_success",
		"checked", len(req.Body.Events),
		"found", len(result.Events),
		"admin_id", user.ID,
	)

	return &DiscoveryCheckResponse{Body: *result}, nil
}

// ============================================================================
// API Token Management Handlers
// ============================================================================

// CreateAPITokenRequest represents the HTTP request for creating an API token
type CreateAPITokenRequest struct {
	Body struct {
		Description    string `json:"description" doc:"Optional description for the token (e.g., 'Mike laptop discovery')"`
		ExpirationDays int    `json:"expiration_days" doc:"Token expiration in days (default: 90, max: 365)"`
	}
}

// CreateAPITokenResponse represents the HTTP response for creating an API token
type CreateAPITokenResponse struct {
	Body services.APITokenCreateResponse `json:"body"`
}

// CreateAPITokenHandler handles POST /admin/tokens
func (h *AdminHandler) CreateAPITokenHandler(ctx context.Context, req *CreateAPITokenRequest) (*CreateAPITokenResponse, error) {
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

	// Validate expiration days
	expirationDays := req.Body.ExpirationDays
	if expirationDays <= 0 {
		expirationDays = 90 // Default
	}
	if expirationDays > 365 {
		return nil, huma.Error400BadRequest("Token expiration cannot exceed 365 days")
	}

	logger.FromContext(ctx).Debug("admin_create_token_attempt",
		"admin_id", user.ID,
		"expiration_days", expirationDays,
	)

	// Create description pointer
	var description *string
	if req.Body.Description != "" {
		description = &req.Body.Description
	}

	// Create the token
	tokenResponse, err := h.apiTokenService.CreateToken(user.ID, description, expirationDays)
	if err != nil {
		logger.FromContext(ctx).Error("admin_create_token_failed",
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to create token (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Info("admin_create_token_success",
		"token_id", tokenResponse.ID,
		"admin_id", user.ID,
		"expires_at", tokenResponse.ExpiresAt,
		"request_id", requestID,
	)

	return &CreateAPITokenResponse{Body: *tokenResponse}, nil
}

// ListAPITokensRequest represents the HTTP request for listing API tokens
type ListAPITokensRequest struct{}

// ListAPITokensResponse represents the HTTP response for listing API tokens
type ListAPITokensResponse struct {
	Body struct {
		Tokens []services.APITokenResponse `json:"tokens"`
	}
}

// ListAPITokensHandler handles GET /admin/tokens
func (h *AdminHandler) ListAPITokensHandler(ctx context.Context, req *ListAPITokensRequest) (*ListAPITokensResponse, error) {
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

	logger.FromContext(ctx).Debug("admin_list_tokens_attempt",
		"admin_id", user.ID,
	)

	tokens, err := h.apiTokenService.ListTokens(user.ID)
	if err != nil {
		logger.FromContext(ctx).Error("admin_list_tokens_failed",
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to list tokens (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Debug("admin_list_tokens_success",
		"count", len(tokens),
		"admin_id", user.ID,
	)

	return &ListAPITokensResponse{
		Body: struct {
			Tokens []services.APITokenResponse `json:"tokens"`
		}{
			Tokens: tokens,
		},
	}, nil
}

// RevokeAPITokenRequest represents the HTTP request for revoking an API token
type RevokeAPITokenRequest struct {
	TokenID string `path:"token_id" validate:"required" doc:"Token ID to revoke"`
}

// RevokeAPITokenResponse represents the HTTP response for revoking an API token
type RevokeAPITokenResponse struct {
	Body struct {
		Message string `json:"message"`
	}
}

// RevokeAPITokenHandler handles DELETE /admin/tokens/{token_id}
func (h *AdminHandler) RevokeAPITokenHandler(ctx context.Context, req *RevokeAPITokenRequest) (*RevokeAPITokenResponse, error) {
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

	// Parse token ID
	tokenID, err := strconv.ParseUint(req.TokenID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid token ID")
	}

	logger.FromContext(ctx).Debug("admin_revoke_token_attempt",
		"token_id", tokenID,
		"admin_id", user.ID,
	)

	err = h.apiTokenService.RevokeToken(user.ID, uint(tokenID))
	if err != nil {
		logger.FromContext(ctx).Error("admin_revoke_token_failed",
			"token_id", tokenID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error404NotFound("Token not found or already revoked")
	}

	logger.FromContext(ctx).Info("admin_revoke_token_success",
		"token_id", tokenID,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	return &RevokeAPITokenResponse{
		Body: struct {
			Message string `json:"message"`
		}{
			Message: "Token revoked successfully",
		},
	}, nil
}

// ============================================================================
// Data Export/Import Handlers (for syncing local data to Stage/Production)
// ============================================================================

// ExportShowsRequest represents the HTTP request for exporting shows
type ExportShowsRequest struct {
	Limit    int    `query:"limit" default:"50" doc:"Number of shows to return (max 200)"`
	Offset   int    `query:"offset" default:"0" doc:"Offset for pagination"`
	Status   string `query:"status" doc:"Filter by status: approved, pending, rejected, all"`
	FromDate string `query:"from_date" doc:"Filter shows from this date (YYYY-MM-DD)"`
	City     string `query:"city" doc:"Filter by city"`
	State    string `query:"state" doc:"Filter by state"`
}

// ExportShowsResponse represents the HTTP response for exporting shows
type ExportShowsResponse struct {
	Body services.ExportShowsResult `json:"body"`
}

// ExportShowsHandler handles GET /admin/export/shows
func (h *AdminHandler) ExportShowsHandler(ctx context.Context, req *ExportShowsRequest) (*ExportShowsResponse, error) {
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

	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	// Build params
	params := services.ExportShowsParams{
		Limit:  req.Limit,
		Offset: offset,
		Status: req.Status,
		City:   req.City,
		State:  req.State,
	}

	// Parse date filter
	if req.FromDate != "" {
		fromDate, err := parseDate(req.FromDate)
		if err != nil {
			return nil, huma.Error400BadRequest("Invalid from_date format, expected YYYY-MM-DD")
		}
		params.FromDate = &fromDate
	}

	logger.FromContext(ctx).Debug("admin_export_shows_attempt",
		"limit", params.Limit,
		"offset", params.Offset,
		"status", params.Status,
	)

	result, err := h.dataSyncService.ExportShows(params)
	if err != nil {
		logger.FromContext(ctx).Error("admin_export_shows_failed",
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to export shows (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Debug("admin_export_shows_success",
		"count", len(result.Shows),
		"total", result.Total,
	)

	return &ExportShowsResponse{Body: *result}, nil
}

// ExportArtistsRequest represents the HTTP request for exporting artists
type ExportArtistsRequest struct {
	Limit  int    `query:"limit" default:"50" doc:"Number of artists to return (max 200)"`
	Offset int    `query:"offset" default:"0" doc:"Offset for pagination"`
	Search string `query:"search" doc:"Search by name"`
}

// ExportArtistsResponse represents the HTTP response for exporting artists
type ExportArtistsResponse struct {
	Body services.ExportArtistsResult `json:"body"`
}

// ExportArtistsHandler handles GET /admin/export/artists
func (h *AdminHandler) ExportArtistsHandler(ctx context.Context, req *ExportArtistsRequest) (*ExportArtistsResponse, error) {
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

	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	params := services.ExportArtistsParams{
		Limit:  req.Limit,
		Offset: offset,
		Search: req.Search,
	}

	logger.FromContext(ctx).Debug("admin_export_artists_attempt",
		"limit", params.Limit,
		"offset", params.Offset,
		"search", params.Search,
	)

	result, err := h.dataSyncService.ExportArtists(params)
	if err != nil {
		logger.FromContext(ctx).Error("admin_export_artists_failed",
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to export artists (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Debug("admin_export_artists_success",
		"count", len(result.Artists),
		"total", result.Total,
	)

	return &ExportArtistsResponse{Body: *result}, nil
}

// ExportVenuesRequest represents the HTTP request for exporting venues
type ExportVenuesRequest struct {
	Limit    int    `query:"limit" default:"50" doc:"Number of venues to return (max 200)"`
	Offset   int    `query:"offset" default:"0" doc:"Offset for pagination"`
	Search   string `query:"search" doc:"Search by name"`
	Verified string `query:"verified" doc:"Filter by verified status: true, false, or empty for all"`
	City     string `query:"city" doc:"Filter by city"`
	State    string `query:"state" doc:"Filter by state"`
}

// ExportVenuesResponse represents the HTTP response for exporting venues
type ExportVenuesResponse struct {
	Body services.ExportVenuesResult `json:"body"`
}

// ExportVenuesHandler handles GET /admin/export/venues
func (h *AdminHandler) ExportVenuesHandler(ctx context.Context, req *ExportVenuesRequest) (*ExportVenuesResponse, error) {
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

	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	params := services.ExportVenuesParams{
		Limit:  req.Limit,
		Offset: offset,
		Search: req.Search,
		City:   req.City,
		State:  req.State,
	}

	// Parse verified filter
	if req.Verified == "true" {
		verified := true
		params.Verified = &verified
	} else if req.Verified == "false" {
		verified := false
		params.Verified = &verified
	}

	logger.FromContext(ctx).Debug("admin_export_venues_attempt",
		"limit", params.Limit,
		"offset", params.Offset,
		"search", params.Search,
	)

	result, err := h.dataSyncService.ExportVenues(params)
	if err != nil {
		logger.FromContext(ctx).Error("admin_export_venues_failed",
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to export venues (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Debug("admin_export_venues_success",
		"count", len(result.Venues),
		"total", result.Total,
	)

	return &ExportVenuesResponse{Body: *result}, nil
}

// DataImportRequest represents the HTTP request for importing data
type DataImportRequest struct {
	Body services.DataImportRequest `json:"body"`
}

// DataImportResponse represents the HTTP response for importing data
type DataImportResponse struct {
	Body services.DataImportResult `json:"body"`
}

// DataImportHandler handles POST /admin/data/import
func (h *AdminHandler) DataImportHandler(ctx context.Context, req *DataImportRequest) (*DataImportResponse, error) {
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

	// Validate limits
	totalItems := len(req.Body.Shows) + len(req.Body.Artists) + len(req.Body.Venues)
	if totalItems == 0 {
		return nil, huma.Error400BadRequest("At least one show, artist, or venue is required")
	}
	if totalItems > 500 {
		return nil, huma.Error400BadRequest("Maximum 500 total items can be imported at once")
	}

	logger.FromContext(ctx).Debug("admin_data_import_attempt",
		"shows", len(req.Body.Shows),
		"artists", len(req.Body.Artists),
		"venues", len(req.Body.Venues),
		"dry_run", req.Body.DryRun,
		"admin_id", user.ID,
	)

	result, err := h.dataSyncService.ImportData(req.Body)
	if err != nil {
		logger.FromContext(ctx).Error("admin_data_import_failed",
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to import data (request_id: %s)", requestID),
		)
	}

	action := "imported"
	if req.Body.DryRun {
		action = "previewed"
	}

	logger.FromContext(ctx).Info("admin_data_import_success",
		"action", action,
		"shows_imported", result.Shows.Imported,
		"artists_imported", result.Artists.Imported,
		"venues_imported", result.Venues.Imported,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	return &DataImportResponse{Body: *result}, nil
}

// ============================================================================
// Admin User List Handlers
// ============================================================================

// GetAdminUsersRequest represents the HTTP request for listing users
type GetAdminUsersRequest struct {
	Limit  int    `query:"limit" default:"50" doc:"Number of users to return (max 100)"`
	Offset int    `query:"offset" default:"0" doc:"Offset for pagination"`
	Search string `query:"search" doc:"Search by email or username"`
}

// GetAdminUsersResponse represents the HTTP response for listing users
type GetAdminUsersResponse struct {
	Body struct {
		Users []*services.AdminUserResponse `json:"users"`
		Total int64                         `json:"total"`
	}
}

// GetAdminUsersHandler handles GET /admin/users
func (h *AdminHandler) GetAdminUsersHandler(ctx context.Context, req *GetAdminUsersRequest) (*GetAdminUsersResponse, error) {
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

	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	logger.FromContext(ctx).Debug("admin_users_list_attempt",
		"limit", limit,
		"offset", offset,
		"search", req.Search,
	)

	// Build filters
	filters := services.AdminUserFilters{
		Search: req.Search,
	}

	// Get users
	users, total, err := h.userService.ListUsers(limit, offset, filters)
	if err != nil {
		logger.FromContext(ctx).Error("admin_users_list_failed",
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to get users (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Debug("admin_users_list_success",
		"count", len(users),
		"total", total,
	)

	return &GetAdminUsersResponse{
		Body: struct {
			Users []*services.AdminUserResponse `json:"users"`
			Total int64                         `json:"total"`
		}{
			Users: users,
			Total: total,
		},
	}, nil
}

// ============================================================================
// Admin Dashboard Stats Handler
// ============================================================================

// GetAdminStatsRequest represents the HTTP request for getting admin dashboard stats
type GetAdminStatsRequest struct{}

// GetAdminStatsResponse represents the HTTP response for admin dashboard stats
type GetAdminStatsResponse struct {
	Body services.AdminDashboardStats
}

// GetAdminStatsHandler handles GET /admin/stats
func (h *AdminHandler) GetAdminStatsHandler(ctx context.Context, req *GetAdminStatsRequest) (*GetAdminStatsResponse, error) {
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

	logger.FromContext(ctx).Debug("admin_stats_attempt",
		"admin_id", user.ID,
	)

	stats, err := h.adminStatsService.GetDashboardStats()
	if err != nil {
		logger.FromContext(ctx).Error("admin_stats_failed",
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to get dashboard stats (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Debug("admin_stats_success",
		"admin_id", user.ID,
	)

	return &GetAdminStatsResponse{Body: *stats}, nil
}

// parseDate parses a date string in YYYY-MM-DD format
func parseDate(dateStr string) (time.Time, error) {
	return time.Parse("2006-01-02", dateStr)
}

