package admin

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/logger"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// AdminShowHandler handles admin show management
type AdminShowHandler struct {
	showService               contracts.ShowServiceInterface
	showAdminService          contracts.ShowAdminServiceInterface
	showImportService         contracts.ShowImportServiceInterface
	discordService            contracts.DiscordServiceInterface
	auditLogService           contracts.AuditLogServiceInterface
	notificationFilterService contracts.NotificationFilterServiceInterface
	musicDiscoveryService     contracts.MusicDiscoveryServiceInterface
}

// NewAdminShowHandler creates a new admin show handler
func NewAdminShowHandler(
	showService contracts.ShowServiceInterface,
	showAdminService contracts.ShowAdminServiceInterface,
	showImportService contracts.ShowImportServiceInterface,
	discordService contracts.DiscordServiceInterface,
	auditLogService contracts.AuditLogServiceInterface,
	notificationFilterService contracts.NotificationFilterServiceInterface,
	musicDiscoveryService contracts.MusicDiscoveryServiceInterface,
) *AdminShowHandler {
	return &AdminShowHandler{
		showService:               showService,
		showAdminService:          showAdminService,
		showImportService:         showImportService,
		discordService:            discordService,
		auditLogService:           auditLogService,
		notificationFilterService: notificationFilterService,
		musicDiscoveryService:     musicDiscoveryService,
	}
}

// GetPendingShowsRequest represents the HTTP request for listing pending shows
type GetPendingShowsRequest struct {
	Limit   int    `query:"limit" default:"50" doc:"Number of shows to return (max 100)"`
	Offset  int    `query:"offset" default:"0" doc:"Offset for pagination"`
	VenueID uint   `query:"venue_id" required:"false" doc:"Filter by venue ID (0 = no filter)"`
	Source  string `query:"source" required:"false" doc:"Filter by source (discovery or user)"`
}

// GetPendingShowsResponse represents the HTTP response for listing pending shows
type GetPendingShowsResponse struct {
	Body struct {
		Shows []*contracts.ShowResponse `json:"shows"`
		Total int64                     `json:"total"`
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
		Shows []*contracts.ShowResponse `json:"shows"`
		Total int64                     `json:"total"`
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
	Body contracts.ShowResponse `json:"body"`
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
	Body contracts.ShowResponse `json:"body"`
}

// BatchApproveShowsRequest represents the HTTP request for batch approving shows
type BatchApproveShowsRequest struct {
	Body struct {
		ShowIDs []uint `json:"show_ids" minItems:"1" maxItems:"100" doc:"List of show IDs to approve"`
	}
}

// BatchApproveShowsResponse represents the HTTP response for batch approving shows
type BatchApproveShowsResponse struct {
	Body struct {
		Approved int                        `json:"approved"`
		Errors   []contracts.BatchShowError `json:"errors"`
	}
}

// BatchRejectShowsRequest represents the HTTP request for batch rejecting shows
type BatchRejectShowsRequest struct {
	Body struct {
		ShowIDs  []uint `json:"show_ids" minItems:"1" maxItems:"100" doc:"List of show IDs to reject"`
		Reason   string `json:"reason" doc:"Reason for rejecting the shows"`
		Category string `json:"category,omitempty" enum:"non_music,duplicate,bad_data,past_event,other" doc:"Rejection category"`
	}
}

// BatchRejectShowsResponse represents the HTTP response for batch rejecting shows
type BatchRejectShowsResponse struct {
	Body struct {
		Rejected int                        `json:"rejected"`
		Errors   []contracts.BatchShowError `json:"errors"`
	}
}

// GetPendingShowsHandler handles GET /admin/shows/pending
func (h *AdminShowHandler) GetPendingShowsHandler(ctx context.Context, req *GetPendingShowsRequest) (*GetPendingShowsResponse, error) {
	requestID := logger.GetRequestID(ctx)

	_, err := shared.RequireAdmin(ctx)
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

	// Build filters
	var filters *contracts.PendingShowsFilter
	if req.VenueID != 0 || req.Source != "" {
		venueIDPtr := func() *uint {
			if req.VenueID == 0 {
				return nil
			}
			v := req.VenueID
			return &v
		}()
		sourcePtr := func() *string {
			if req.Source == "" {
				return nil
			}
			s := req.Source
			return &s
		}()
		filters = &contracts.PendingShowsFilter{
			VenueID: venueIDPtr,
			Source:  sourcePtr,
		}
	}

	logger.FromContext(ctx).Debug("admin_pending_shows_attempt",
		"limit", limit,
		"offset", offset,
	)

	// Get pending shows
	shows, total, err := h.showAdminService.GetPendingShows(limit, offset, filters)
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
			Shows []*contracts.ShowResponse `json:"shows"`
			Total int64                     `json:"total"`
		}{
			Shows: shows,
			Total: total,
		},
	}, nil
}

// GetRejectedShowsHandler handles GET /admin/shows/rejected
func (h *AdminShowHandler) GetRejectedShowsHandler(ctx context.Context, req *GetRejectedShowsRequest) (*GetRejectedShowsResponse, error) {
	requestID := logger.GetRequestID(ctx)

	_, err := shared.RequireAdmin(ctx)
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

	logger.FromContext(ctx).Debug("admin_rejected_shows_attempt",
		"limit", limit,
		"offset", offset,
		"search", req.Search,
	)

	// Get rejected shows
	shows, total, err := h.showAdminService.GetRejectedShows(limit, offset, req.Search)
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
			Shows []*contracts.ShowResponse `json:"shows"`
			Total int64                     `json:"total"`
		}{
			Shows: shows,
			Total: total,
		},
	}, nil
}

// ApproveShowHandler handles POST /admin/shows/{show_id}/approve
func (h *AdminShowHandler) ApproveShowHandler(ctx context.Context, req *ApproveShowRequest) (*ApproveShowResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user, err := shared.RequireAdmin(ctx)
	if err != nil {
		return nil, err
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
	show, err := h.showAdminService.ApproveShow(uint(showID), req.Body.VerifyVenues)
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

	// Fire-and-forget: match notification filters for this newly approved show
	if h.notificationFilterService != nil {
		go func() {
			showModel := &catalogm.Show{ID: uint(showID), Title: show.Title, EventDate: show.EventDate, Price: show.Price, Slug: shared.PtrString(show.Slug)}
			if show.City != nil {
				showModel.City = show.City
			}
			if show.State != nil {
				showModel.State = show.State
			}
			if err := h.notificationFilterService.MatchAndNotify(showModel); err != nil {
				logger.Default().Error("notification_filter_match_failed",
					"show_id", showID,
					"error", err.Error(),
				)
			}
		}()
	}

	// Audit log
	h.auditLogService.LogAction(user.ID, "approve_show", "show", uint(showID), map[string]interface{}{
		"verify_venues": req.Body.VerifyVenues,
	})

	return &ApproveShowResponse{Body: *show}, nil
}

// RejectShowHandler handles POST /admin/shows/{show_id}/reject
func (h *AdminShowHandler) RejectShowHandler(ctx context.Context, req *RejectShowRequest) (*RejectShowResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user, err := shared.RequireAdmin(ctx)
	if err != nil {
		return nil, err
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
	show, err := h.showAdminService.RejectShow(uint(showID), req.Body.Reason)
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

// BatchApproveShowsHandler handles POST /admin/shows/batch-approve
func (h *AdminShowHandler) BatchApproveShowsHandler(ctx context.Context, req *BatchApproveShowsRequest) (*BatchApproveShowsResponse, error) {
	user, err := shared.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	result, err := h.showAdminService.BatchApproveShows(req.Body.ShowIDs)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to batch approve shows")
	}

	// Audit log each approved show (fire-and-forget)
	for _, id := range result.Succeeded {
		h.auditLogService.LogAction(user.ID, "approve_show", "show", id, map[string]interface{}{
			"batch": true,
		})
	}

	// Fire-and-forget: match notification filters for batch-approved shows
	if h.notificationFilterService != nil && len(result.Succeeded) > 0 {
		go func() {
			for _, showID := range result.Succeeded {
				show, err := h.showService.GetShow(showID)
				if err != nil || show == nil {
					continue
				}
				showModel := &catalogm.Show{ID: showID, Title: show.Title, EventDate: show.EventDate, Price: show.Price, Slug: shared.PtrString(show.Slug)}
				if show.City != nil {
					showModel.City = show.City
				}
				if show.State != nil {
					showModel.State = show.State
				}
				if err := h.notificationFilterService.MatchAndNotify(showModel); err != nil {
					logger.Default().Error("notification_filter_batch_match_failed",
						"show_id", showID,
						"error", err.Error(),
					)
				}
			}
		}()
	}

	logger.FromContext(ctx).Info("admin_batch_approve_shows",
		"approved", len(result.Succeeded),
		"errors", len(result.Errors),
		"admin_id", user.ID,
	)

	return &BatchApproveShowsResponse{
		Body: struct {
			Approved int                        `json:"approved"`
			Errors   []contracts.BatchShowError `json:"errors"`
		}{
			Approved: len(result.Succeeded),
			Errors:   result.Errors,
		},
	}, nil
}

// BatchRejectShowsHandler handles POST /admin/shows/batch-reject
func (h *AdminShowHandler) BatchRejectShowsHandler(ctx context.Context, req *BatchRejectShowsRequest) (*BatchRejectShowsResponse, error) {
	user, err := shared.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	// Validate reason
	if req.Body.Reason == "" {
		return nil, huma.Error400BadRequest("Rejection reason is required")
	}

	result, err := h.showAdminService.BatchRejectShows(req.Body.ShowIDs, req.Body.Reason, req.Body.Category)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to batch reject shows")
	}

	// Audit log each rejected show (fire-and-forget)
	for _, id := range result.Succeeded {
		h.auditLogService.LogAction(user.ID, "reject_show", "show", id, map[string]interface{}{
			"batch":    true,
			"reason":   req.Body.Reason,
			"category": req.Body.Category,
		})
	}

	logger.FromContext(ctx).Info("admin_batch_reject_shows",
		"rejected", len(result.Succeeded),
		"errors", len(result.Errors),
		"admin_id", user.ID,
	)

	return &BatchRejectShowsResponse{
		Body: struct {
			Rejected int                        `json:"rejected"`
			Errors   []contracts.BatchShowError `json:"errors"`
		}{
			Rejected: len(result.Succeeded),
			Errors:   result.Errors,
		},
	}, nil
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
	Body contracts.ImportPreviewResponse `json:"body"`
}

// ImportShowPreviewHandler handles POST /admin/shows/import/preview
func (h *AdminShowHandler) ImportShowPreviewHandler(ctx context.Context, req *ImportShowPreviewRequest) (*ImportShowPreviewResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user, err := shared.RequireAdmin(ctx)
	if err != nil {
		return nil, err
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
	preview, err := h.showImportService.PreviewShowImport(content)
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
	Body contracts.ShowResponse `json:"body"`
}

// ImportShowConfirmHandler handles POST /admin/shows/import/confirm
func (h *AdminShowHandler) ImportShowConfirmHandler(ctx context.Context, req *ImportShowConfirmRequest) (*ImportShowConfirmResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user, err := shared.RequireAdmin(ctx)
	if err != nil {
		return nil, err
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
	show, err := h.showImportService.ConfirmShowImport(content, true)
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
		Shows []*contracts.ShowResponse `json:"shows"`
		Total int64                     `json:"total"`
	}
}

// GetAdminShowsHandler handles GET /admin/shows
// Returns paginated show list with full details for admin export purposes
func (h *AdminShowHandler) GetAdminShowsHandler(ctx context.Context, req *GetAdminShowsRequest) (*GetAdminShowsResponse, error) {
	requestID := logger.GetRequestID(ctx)

	_, err := shared.RequireAdmin(ctx)
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

	logger.FromContext(ctx).Debug("admin_shows_list_attempt",
		"limit", limit,
		"offset", offset,
		"status", req.Status,
		"from_date", req.FromDate,
		"to_date", req.ToDate,
		"city", req.City,
	)

	// Build filters
	filters := contracts.AdminShowFilters{
		Status:   req.Status,
		FromDate: req.FromDate,
		ToDate:   req.ToDate,
		City:     req.City,
	}

	// Get shows
	shows, total, err := h.showAdminService.GetAdminShows(limit, offset, filters)
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
			Shows []*contracts.ShowResponse `json:"shows"`
			Total int64                     `json:"total"`
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
func (h *AdminShowHandler) BulkExportShowsHandler(ctx context.Context, req *BulkExportShowsRequest) (*BulkExportShowsResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user, err := shared.RequireAdmin(ctx)
	if err != nil {
		return nil, err
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
		content, _, err := h.showImportService.ExportShowToMarkdown(showID)
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
	TotalShows      int  `json:"total_shows"`
	NewArtists      int  `json:"new_artists"`
	NewVenues       int  `json:"new_venues"`
	ExistingArtists int  `json:"existing_artists"`
	ExistingVenues  int  `json:"existing_venues"`
	WarningCount    int  `json:"warning_count"`
	CanImportAll    bool `json:"can_import_all"`
}

// BulkImportPreviewResponse represents the HTTP response for bulk import preview
type BulkImportPreviewResponse struct {
	Body struct {
		Previews []contracts.ImportPreviewResponse `json:"previews"`
		Summary  BulkImportPreviewSummary          `json:"summary"`
	}
}

// BulkImportPreviewHandler handles POST /admin/shows/import/bulk/preview
// Previews import of multiple shows with conflict detection
func (h *AdminShowHandler) BulkImportPreviewHandler(ctx context.Context, req *BulkImportPreviewRequest) (*BulkImportPreviewResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user, err := shared.RequireAdmin(ctx)
	if err != nil {
		return nil, err
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
	previews := make([]contracts.ImportPreviewResponse, 0, len(req.Body.Shows))
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

		preview, err := h.showImportService.PreviewShowImport(content)
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
			Previews []contracts.ImportPreviewResponse `json:"previews"`
			Summary  BulkImportPreviewSummary          `json:"summary"`
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
	Show    *contracts.ShowResponse `json:"show,omitempty"`
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
func (h *AdminShowHandler) BulkImportConfirmHandler(ctx context.Context, req *BulkImportConfirmRequest) (*BulkImportConfirmResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user, err := shared.RequireAdmin(ctx)
	if err != nil {
		return nil, err
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

		show, err := h.showImportService.ConfirmShowImport(content, true)
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
