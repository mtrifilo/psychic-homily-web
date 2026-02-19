package handlers

import (
	"context"
	"fmt"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services"
)

// ArtistReportHandler handles artist report HTTP requests
type ArtistReportHandler struct {
	artistReportService services.ArtistReportServiceInterface
	discordService      services.DiscordServiceInterface
	userService         services.UserServiceInterface
	auditLogService     services.AuditLogServiceInterface
}

// NewArtistReportHandler creates a new artist report handler
func NewArtistReportHandler(
	artistReportService services.ArtistReportServiceInterface,
	discordService services.DiscordServiceInterface,
	userService services.UserServiceInterface,
	auditLogService services.AuditLogServiceInterface,
) *ArtistReportHandler {
	return &ArtistReportHandler{
		artistReportService: artistReportService,
		discordService:      discordService,
		userService:         userService,
		auditLogService:     auditLogService,
	}
}

// ============================================================================
// User Endpoints
// ============================================================================

// ReportArtistRequest represents the HTTP request for reporting an artist
type ReportArtistRequest struct {
	ArtistID string `path:"artist_id" validate:"required" doc:"Artist ID"`
	Body     struct {
		ReportType string  `json:"report_type" validate:"required" doc:"Type of report: inaccurate or removal_request"`
		Details    *string `json:"details" doc:"Optional details about the issue"`
	}
}

// ReportArtistResponse represents the HTTP response for reporting an artist
type ReportArtistResponse struct {
	Body services.ArtistReportResponse `json:"body"`
}

// ReportArtistHandler handles POST /artists/{artist_id}/report
func (h *ArtistReportHandler) ReportArtistHandler(ctx context.Context, req *ReportArtistRequest) (*ReportArtistResponse, error) {
	requestID := logger.GetRequestID(ctx)

	// Get authenticated user
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	// Parse artist ID
	artistID, err := strconv.ParseUint(req.ArtistID, 10, 32)
	if err != nil {
		logger.FromContext(ctx).Warn("report_artist_invalid_id",
			"artist_id_str", req.ArtistID,
			"request_id", requestID,
		)
		return nil, huma.Error400BadRequest("Invalid artist ID")
	}

	logger.FromContext(ctx).Debug("report_artist_attempt",
		"user_id", user.ID,
		"artist_id", artistID,
		"report_type", req.Body.ReportType,
	)

	// Create the report
	report, err := h.artistReportService.CreateReport(user.ID, uint(artistID), req.Body.ReportType, req.Body.Details)
	if err != nil {
		logger.FromContext(ctx).Error("report_artist_failed",
			"user_id", user.ID,
			"artist_id", artistID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error422UnprocessableEntity(
			fmt.Sprintf("Failed to report artist (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Info("report_artist_success",
		"user_id", user.ID,
		"artist_id", artistID,
		"report_id", report.ID,
		"report_type", req.Body.ReportType,
		"request_id", requestID,
	)

	// Send Discord notification
	reportModel, _ := h.artistReportService.GetReportByID(report.ID)
	if reportModel != nil {
		reporterEmail := ""
		if user.Email != nil {
			reporterEmail = *user.Email
		}
		h.discordService.NotifyArtistReport(reportModel, reporterEmail)
	}

	return &ReportArtistResponse{Body: *report}, nil
}

// GetMyArtistReportRequest represents the HTTP request for checking user's report for an artist
type GetMyArtistReportRequest struct {
	ArtistID string `path:"artist_id" validate:"required" doc:"Artist ID"`
}

// GetMyArtistReportResponse represents the HTTP response for checking user's report
type GetMyArtistReportResponse struct {
	Body struct {
		Report *services.ArtistReportResponse `json:"report"`
	}
}

// GetMyArtistReportHandler handles GET /artists/{artist_id}/my-report
func (h *ArtistReportHandler) GetMyArtistReportHandler(ctx context.Context, req *GetMyArtistReportRequest) (*GetMyArtistReportResponse, error) {
	requestID := logger.GetRequestID(ctx)

	// Get authenticated user
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	// Parse artist ID
	artistID, err := strconv.ParseUint(req.ArtistID, 10, 32)
	if err != nil {
		logger.FromContext(ctx).Warn("get_my_artist_report_invalid_id",
			"artist_id_str", req.ArtistID,
			"request_id", requestID,
		)
		return nil, huma.Error400BadRequest("Invalid artist ID")
	}

	// Get user's report for this artist
	report, err := h.artistReportService.GetUserReportForArtist(user.ID, uint(artistID))
	if err != nil {
		logger.FromContext(ctx).Error("get_my_artist_report_failed",
			"user_id", user.ID,
			"artist_id", artistID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to get report (request_id: %s)", requestID),
		)
	}

	return &GetMyArtistReportResponse{
		Body: struct {
			Report *services.ArtistReportResponse `json:"report"`
		}{
			Report: report,
		},
	}, nil
}

// ============================================================================
// Admin Endpoints
// ============================================================================

// GetPendingArtistReportsRequest represents the HTTP request for listing pending artist reports
type GetPendingArtistReportsRequest struct {
	Limit  int `query:"limit" default:"50" doc:"Number of reports to return (max 100)"`
	Offset int `query:"offset" default:"0" doc:"Offset for pagination"`
}

// GetPendingArtistReportsResponse represents the HTTP response for listing pending artist reports
type GetPendingArtistReportsResponse struct {
	Body struct {
		Reports []*services.ArtistReportResponse `json:"reports"`
		Total   int64                            `json:"total"`
	}
}

// GetPendingArtistReportsHandler handles GET /admin/artist-reports
func (h *ArtistReportHandler) GetPendingArtistReportsHandler(ctx context.Context, req *GetPendingArtistReportsRequest) (*GetPendingArtistReportsResponse, error) {
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

	logger.FromContext(ctx).Debug("admin_pending_artist_reports_attempt",
		"limit", limit,
		"offset", req.Offset,
	)

	// Get pending reports
	reports, total, err := h.artistReportService.GetPendingReports(limit, req.Offset)
	if err != nil {
		logger.FromContext(ctx).Error("admin_pending_artist_reports_failed",
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to get pending artist reports (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Debug("admin_pending_artist_reports_success",
		"count", len(reports),
		"total", total,
	)

	return &GetPendingArtistReportsResponse{
		Body: struct {
			Reports []*services.ArtistReportResponse `json:"reports"`
			Total   int64                            `json:"total"`
		}{
			Reports: reports,
			Total:   total,
		},
	}, nil
}

// DismissArtistReportRequest represents the HTTP request for dismissing an artist report
type DismissArtistReportRequest struct {
	ReportID string `path:"report_id" validate:"required" doc:"Report ID"`
	Body     struct {
		Notes *string `json:"notes" doc:"Optional admin notes about the dismissal"`
	}
}

// DismissArtistReportResponse represents the HTTP response for dismissing an artist report
type DismissArtistReportResponse struct {
	Body services.ArtistReportResponse `json:"body"`
}

// DismissArtistReportHandler handles POST /admin/artist-reports/{report_id}/dismiss
func (h *ArtistReportHandler) DismissArtistReportHandler(ctx context.Context, req *DismissArtistReportRequest) (*DismissArtistReportResponse, error) {
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

	// Parse report ID
	reportID, err := strconv.ParseUint(req.ReportID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid report ID")
	}

	logger.FromContext(ctx).Debug("admin_dismiss_artist_report_attempt",
		"report_id", reportID,
		"admin_id", user.ID,
	)

	// Dismiss the report
	report, err := h.artistReportService.DismissReport(uint(reportID), user.ID, req.Body.Notes)
	if err != nil {
		logger.FromContext(ctx).Error("admin_dismiss_artist_report_failed",
			"report_id", reportID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error422UnprocessableEntity(
			fmt.Sprintf("Failed to dismiss artist report (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Info("admin_dismiss_artist_report_success",
		"report_id", reportID,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	// Audit log
	metadata := map[string]interface{}{"artist_id": report.ArtistID}
	if req.Body.Notes != nil {
		metadata["notes"] = *req.Body.Notes
	}
	h.auditLogService.LogAction(user.ID, "dismiss_artist_report", "artist_report", uint(reportID), metadata)

	return &DismissArtistReportResponse{Body: *report}, nil
}

// ResolveArtistReportRequest represents the HTTP request for resolving an artist report
type ResolveArtistReportRequest struct {
	ReportID string `path:"report_id" validate:"required" doc:"Report ID"`
	Body     struct {
		Notes *string `json:"notes" doc:"Optional admin notes about the resolution"`
	}
}

// ResolveArtistReportResponse represents the HTTP response for resolving an artist report
type ResolveArtistReportResponse struct {
	Body services.ArtistReportResponse `json:"body"`
}

// ResolveArtistReportHandler handles POST /admin/artist-reports/{report_id}/resolve
func (h *ArtistReportHandler) ResolveArtistReportHandler(ctx context.Context, req *ResolveArtistReportRequest) (*ResolveArtistReportResponse, error) {
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

	// Parse report ID
	reportID, err := strconv.ParseUint(req.ReportID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid report ID")
	}

	logger.FromContext(ctx).Debug("admin_resolve_artist_report_attempt",
		"report_id", reportID,
		"admin_id", user.ID,
	)

	// Resolve the report
	report, err := h.artistReportService.ResolveReport(uint(reportID), user.ID, req.Body.Notes)
	if err != nil {
		logger.FromContext(ctx).Error("admin_resolve_artist_report_failed",
			"report_id", reportID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error422UnprocessableEntity(
			fmt.Sprintf("Failed to resolve artist report (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Info("admin_resolve_artist_report_success",
		"report_id", reportID,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	// Audit log
	auditMeta := map[string]interface{}{"artist_id": report.ArtistID}
	if req.Body.Notes != nil {
		auditMeta["notes"] = *req.Body.Notes
	}
	h.auditLogService.LogAction(user.ID, "resolve_artist_report", "artist_report", uint(reportID), auditMeta)

	return &ResolveArtistReportResponse{Body: *report}, nil
}
