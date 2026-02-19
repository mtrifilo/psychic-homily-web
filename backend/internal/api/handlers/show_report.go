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

// ShowReportHandler handles show report HTTP requests
type ShowReportHandler struct {
	showReportService services.ShowReportServiceInterface
	discordService    services.DiscordServiceInterface
	userService       services.UserServiceInterface
	auditLogService   services.AuditLogServiceInterface
}

// NewShowReportHandler creates a new show report handler
func NewShowReportHandler(
	showReportService services.ShowReportServiceInterface,
	discordService services.DiscordServiceInterface,
	userService services.UserServiceInterface,
	auditLogService services.AuditLogServiceInterface,
) *ShowReportHandler {
	return &ShowReportHandler{
		showReportService: showReportService,
		discordService:    discordService,
		userService:       userService,
		auditLogService:   auditLogService,
	}
}

// ============================================================================
// User Endpoints
// ============================================================================

// ReportShowRequest represents the HTTP request for reporting a show
type ReportShowRequest struct {
	ShowID string `path:"show_id" validate:"required" doc:"Show ID"`
	Body   struct {
		ReportType string  `json:"report_type" validate:"required" doc:"Type of report: cancelled, sold_out, or inaccurate"`
		Details    *string `json:"details" doc:"Optional details about the issue (primarily for inaccurate reports)"`
	}
}

// ReportShowResponse represents the HTTP response for reporting a show
type ReportShowResponse struct {
	Body services.ShowReportResponse `json:"body"`
}

// ReportShowHandler handles POST /shows/{show_id}/report
func (h *ShowReportHandler) ReportShowHandler(ctx context.Context, req *ReportShowRequest) (*ReportShowResponse, error) {
	requestID := logger.GetRequestID(ctx)

	// Get authenticated user
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	// Parse show ID
	showID, err := strconv.ParseUint(req.ShowID, 10, 32)
	if err != nil {
		logger.FromContext(ctx).Warn("report_show_invalid_id",
			"show_id_str", req.ShowID,
			"request_id", requestID,
		)
		return nil, huma.Error400BadRequest("Invalid show ID")
	}

	logger.FromContext(ctx).Debug("report_show_attempt",
		"user_id", user.ID,
		"show_id", showID,
		"report_type", req.Body.ReportType,
	)

	// Create the report
	report, err := h.showReportService.CreateReport(user.ID, uint(showID), req.Body.ReportType, req.Body.Details)
	if err != nil {
		logger.FromContext(ctx).Error("report_show_failed",
			"user_id", user.ID,
			"show_id", showID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error422UnprocessableEntity(
			fmt.Sprintf("Failed to report show (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Info("report_show_success",
		"user_id", user.ID,
		"show_id", showID,
		"report_id", report.ID,
		"report_type", req.Body.ReportType,
		"request_id", requestID,
	)

	// Send Discord notification
	reportModel, _ := h.showReportService.GetReportByID(report.ID)
	if reportModel != nil {
		reporterEmail := ""
		if user.Email != nil {
			reporterEmail = *user.Email
		}
		h.discordService.NotifyShowReport(reportModel, reporterEmail)
	}

	return &ReportShowResponse{Body: *report}, nil
}

// GetMyReportRequest represents the HTTP request for checking user's report for a show
type GetMyReportRequest struct {
	ShowID string `path:"show_id" validate:"required" doc:"Show ID"`
}

// GetMyReportResponse represents the HTTP response for checking user's report
type GetMyReportResponse struct {
	Body struct {
		Report *services.ShowReportResponse `json:"report"`
	}
}

// GetMyReportHandler handles GET /shows/{show_id}/my-report
func (h *ShowReportHandler) GetMyReportHandler(ctx context.Context, req *GetMyReportRequest) (*GetMyReportResponse, error) {
	requestID := logger.GetRequestID(ctx)

	// Get authenticated user
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	// Parse show ID
	showID, err := strconv.ParseUint(req.ShowID, 10, 32)
	if err != nil {
		logger.FromContext(ctx).Warn("get_my_report_invalid_id",
			"show_id_str", req.ShowID,
			"request_id", requestID,
		)
		return nil, huma.Error400BadRequest("Invalid show ID")
	}

	// Get user's report for this show
	report, err := h.showReportService.GetUserReportForShow(user.ID, uint(showID))
	if err != nil {
		logger.FromContext(ctx).Error("get_my_report_failed",
			"user_id", user.ID,
			"show_id", showID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to get report (request_id: %s)", requestID),
		)
	}

	return &GetMyReportResponse{
		Body: struct {
			Report *services.ShowReportResponse `json:"report"`
		}{
			Report: report,
		},
	}, nil
}

// ============================================================================
// Admin Endpoints
// ============================================================================

// GetPendingReportsRequest represents the HTTP request for listing pending reports
type GetPendingReportsRequest struct {
	Limit  int `query:"limit" default:"50" doc:"Number of reports to return (max 100)"`
	Offset int `query:"offset" default:"0" doc:"Offset for pagination"`
}

// GetPendingReportsResponse represents the HTTP response for listing pending reports
type GetPendingReportsResponse struct {
	Body struct {
		Reports []*services.ShowReportResponse `json:"reports"`
		Total   int64                          `json:"total"`
	}
}

// GetPendingReportsHandler handles GET /admin/reports
func (h *ShowReportHandler) GetPendingReportsHandler(ctx context.Context, req *GetPendingReportsRequest) (*GetPendingReportsResponse, error) {
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

	logger.FromContext(ctx).Debug("admin_pending_reports_attempt",
		"limit", limit,
		"offset", req.Offset,
	)

	// Get pending reports
	reports, total, err := h.showReportService.GetPendingReports(limit, req.Offset)
	if err != nil {
		logger.FromContext(ctx).Error("admin_pending_reports_failed",
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to get pending reports (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Debug("admin_pending_reports_success",
		"count", len(reports),
		"total", total,
	)

	return &GetPendingReportsResponse{
		Body: struct {
			Reports []*services.ShowReportResponse `json:"reports"`
			Total   int64                          `json:"total"`
		}{
			Reports: reports,
			Total:   total,
		},
	}, nil
}

// DismissReportRequest represents the HTTP request for dismissing a report
type DismissReportRequest struct {
	ReportID string `path:"report_id" validate:"required" doc:"Report ID"`
	Body     struct {
		Notes *string `json:"notes" doc:"Optional admin notes about the dismissal"`
	}
}

// DismissReportResponse represents the HTTP response for dismissing a report
type DismissReportResponse struct {
	Body services.ShowReportResponse `json:"body"`
}

// DismissReportHandler handles POST /admin/reports/{report_id}/dismiss
func (h *ShowReportHandler) DismissReportHandler(ctx context.Context, req *DismissReportRequest) (*DismissReportResponse, error) {
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

	logger.FromContext(ctx).Debug("admin_dismiss_report_attempt",
		"report_id", reportID,
		"admin_id", user.ID,
	)

	// Dismiss the report
	report, err := h.showReportService.DismissReport(uint(reportID), user.ID, req.Body.Notes)
	if err != nil {
		logger.FromContext(ctx).Error("admin_dismiss_report_failed",
			"report_id", reportID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error422UnprocessableEntity(
			fmt.Sprintf("Failed to dismiss report (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Info("admin_dismiss_report_success",
		"report_id", reportID,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	// Audit log
	metadata := map[string]interface{}{"show_id": report.ShowID}
	if req.Body.Notes != nil {
		metadata["notes"] = *req.Body.Notes
	}
	h.auditLogService.LogAction(user.ID, "dismiss_report", "show_report", uint(reportID), metadata)

	return &DismissReportResponse{Body: *report}, nil
}

// ResolveReportRequest represents the HTTP request for resolving a report
type ResolveReportRequest struct {
	ReportID string `path:"report_id" validate:"required" doc:"Report ID"`
	Body     struct {
		Notes       *string `json:"notes" doc:"Optional admin notes about the resolution"`
		SetShowFlag *bool   `json:"set_show_flag" doc:"For cancelled/sold_out reports, set the corresponding show flag (default: false)"`
	}
}

// ResolveReportResponse represents the HTTP response for resolving a report
type ResolveReportResponse struct {
	Body services.ShowReportResponse `json:"body"`
}

// ResolveReportHandler handles POST /admin/reports/{report_id}/resolve
func (h *ShowReportHandler) ResolveReportHandler(ctx context.Context, req *ResolveReportRequest) (*ResolveReportResponse, error) {
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

	// Determine if we should set the show flag
	setShowFlag := false
	if req.Body.SetShowFlag != nil && *req.Body.SetShowFlag {
		setShowFlag = true
	}

	logger.FromContext(ctx).Debug("admin_resolve_report_attempt",
		"report_id", reportID,
		"admin_id", user.ID,
		"set_show_flag", setShowFlag,
	)

	// Resolve the report (with optional show flag update)
	report, err := h.showReportService.ResolveReportWithFlag(uint(reportID), user.ID, req.Body.Notes, setShowFlag)
	if err != nil {
		logger.FromContext(ctx).Error("admin_resolve_report_failed",
			"report_id", reportID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error422UnprocessableEntity(
			fmt.Sprintf("Failed to resolve report (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Info("admin_resolve_report_success",
		"report_id", reportID,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	// Audit log
	auditAction := "resolve_report"
	if setShowFlag {
		auditAction = "resolve_report_with_flag"
	}
	auditMeta := map[string]interface{}{
		"show_id":       report.ShowID,
		"set_show_flag": setShowFlag,
	}
	if req.Body.Notes != nil {
		auditMeta["notes"] = *req.Body.Notes
	}
	h.auditLogService.LogAction(user.ID, auditAction, "show_report", uint(reportID), auditMeta)

	return &ResolveReportResponse{Body: *report}, nil
}
