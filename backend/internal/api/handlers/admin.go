package handlers

import (
	"context"
	"encoding/base64"
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
	showService           *services.ShowService
	venueService          *services.VenueService
	discordService        *services.DiscordService
	musicDiscoveryService *services.MusicDiscoveryService
}

// NewAdminHandler creates a new admin handler
func NewAdminHandler(cfg *config.Config) *AdminHandler {
	return &AdminHandler{
		showService:           services.NewShowService(),
		venueService:          services.NewVenueService(),
		discordService:        services.NewDiscordService(cfg),
		musicDiscoveryService: services.NewMusicDiscoveryService(cfg),
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

	logger.FromContext(ctx).Debug("admin_rejected_shows_attempt",
		"limit", limit,
		"offset", req.Offset,
		"search", req.Search,
	)

	// Get rejected shows
	shows, total, err := h.showService.GetRejectedShows(limit, req.Offset, req.Search)
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

	logger.FromContext(ctx).Debug("admin_unverified_venues_attempt",
		"limit", limit,
		"offset", req.Offset,
	)

	// Get unverified venues
	venues, total, err := h.venueService.GetUnverifiedVenues(limit, req.Offset)
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
			fmt.Sprintf("Failed to preview import: %s (request_id: %s)", err.Error(), requestID),
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
			fmt.Sprintf("Failed to import show: %s (request_id: %s)", err.Error(), requestID),
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

	logger.FromContext(ctx).Debug("admin_shows_list_attempt",
		"limit", limit,
		"offset", req.Offset,
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
	shows, total, err := h.showService.GetAdminShows(limit, req.Offset, filters)
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
				fmt.Sprintf("Failed to export show %d: %s (request_id: %s)", showID, err.Error(), requestID),
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
				fmt.Sprintf("Failed to preview show %d: %s (request_id: %s)", i+1, err.Error(), requestID),
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
				Error:   fmt.Sprintf("Invalid base64 content: %s", err.Error()),
			})
			errorCount++
			continue
		}

		show, err := h.showService.ConfirmShowImport(content, true)
		if err != nil {
			results = append(results, BulkImportResult{
				Success: false,
				Error:   err.Error(),
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

