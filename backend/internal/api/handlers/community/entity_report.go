package community

import (
	"context"
	"strconv"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
)

// EntityReportHandler handles entity report API endpoints.
type EntityReportHandler struct {
	entityReportService contracts.EntityReportServiceInterface
	auditLogService     contracts.AuditLogServiceInterface
}

// NewEntityReportHandler creates a new entity report handler.
func NewEntityReportHandler(
	entityReportService contracts.EntityReportServiceInterface,
	auditLogService contracts.AuditLogServiceInterface,
) *EntityReportHandler {
	return &EntityReportHandler{
		entityReportService: entityReportService,
		auditLogService:     auditLogService,
	}
}

// ============================================================================
// User: Report an entity — POST /{entity_type}/{entity_id}/report
// ============================================================================

// ReportEntityRequest is the Huma request for POST /{entity_type}/{entity_id}/report
type ReportEntityRequest struct {
	EntityID string `path:"entity_id" doc:"Entity ID"`
	Body     struct {
		ReportType string  `json:"report_type" doc:"Type of report"`
		Details    *string `json:"details" required:"false" doc:"Optional details about the issue"`
	}
}

// ReportEntityResponse is the Huma response for POST /{entity_type}/{entity_id}/report
type ReportEntityResponse struct {
	Body *contracts.EntityReportResponse
}

// ReportArtistHandler handles POST /artists/{entity_id}/report
func (h *EntityReportHandler) ReportArtistHandler(ctx context.Context, req *ReportEntityRequest) (*ReportEntityResponse, error) {
	return h.reportEntity(ctx, "artist", req)
}

// ReportVenueHandler handles POST /venues/{entity_id}/report
func (h *EntityReportHandler) ReportVenueHandler(ctx context.Context, req *ReportEntityRequest) (*ReportEntityResponse, error) {
	return h.reportEntity(ctx, "venue", req)
}

// ReportFestivalHandler handles POST /festivals/{entity_id}/report
func (h *EntityReportHandler) ReportFestivalHandler(ctx context.Context, req *ReportEntityRequest) (*ReportEntityResponse, error) {
	return h.reportEntity(ctx, "festival", req)
}

// ReportShowHandler handles POST /shows/{entity_id}/report
func (h *EntityReportHandler) ReportShowHandler(ctx context.Context, req *ReportEntityRequest) (*ReportEntityResponse, error) {
	return h.reportEntity(ctx, "show", req)
}

// ReportCommentHandler handles POST /comments/{entity_id}/report
func (h *EntityReportHandler) ReportCommentHandler(ctx context.Context, req *ReportEntityRequest) (*ReportEntityResponse, error) {
	return h.reportEntity(ctx, "comment", req)
}

// reportEntity is the shared implementation for all report endpoints.
func (h *EntityReportHandler) reportEntity(ctx context.Context, entityType string, req *ReportEntityRequest) (*ReportEntityResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	entityID, err := strconv.ParseUint(req.EntityID, 10, 64)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid entity ID")
	}

	reportType := strings.TrimSpace(req.Body.ReportType)
	if reportType == "" {
		return nil, huma.Error400BadRequest("Report type is required")
	}

	if !models.IsValidReportType(entityType, reportType) {
		return nil, huma.Error400BadRequest("Invalid report type '" + reportType + "' for " + entityType)
	}

	report, err := h.entityReportService.CreateEntityReport(&contracts.CreateEntityReportRequest{
		EntityType: entityType,
		EntityID:   uint(entityID),
		UserID:     user.ID,
		ReportType: reportType,
		Details:    req.Body.Details,
	})
	if err != nil {
		if strings.Contains(err.Error(), "entity not found") {
			return nil, huma.Error404NotFound(err.Error())
		}
		if strings.Contains(err.Error(), "already have a pending report") {
			return nil, huma.Error409Conflict(err.Error())
		}
		logger.FromContext(ctx).Error("entity_report_create_failed",
			"user_id", user.ID,
			"entity_type", entityType,
			"entity_id", entityID,
			"error", err.Error(),
		)
		return nil, huma.Error500InternalServerError("Failed to create report")
	}

	// Fire-and-forget audit log
	if h.auditLogService != nil {
		go h.auditLogService.LogAction(user.ID, "report_"+entityType, entityType, uint(entityID), map[string]interface{}{
			"report_id":   report.ID,
			"report_type": reportType,
		})
	}

	return &ReportEntityResponse{Body: report}, nil
}

// ============================================================================
// Admin: List Entity Reports — GET /admin/entity-reports
// ============================================================================

// AdminListEntityReportsRequest is the Huma request for GET /admin/entity-reports
type AdminListEntityReportsRequest struct {
	Status     string `query:"status" required:"false" doc:"Filter by status (pending, resolved, dismissed)"`
	EntityType string `query:"entity_type" required:"false" doc:"Filter by entity type (artist, venue, festival, show)"`
	Limit      int    `query:"limit" required:"false" doc:"Max results (default 20, max 100)"`
	Offset     int    `query:"offset" required:"false" doc:"Offset for pagination"`
}

// AdminListEntityReportsResponse is the Huma response for GET /admin/entity-reports
type AdminListEntityReportsResponse struct {
	Body struct {
		Reports []contracts.EntityReportResponse `json:"reports"`
		Total   int64                            `json:"total"`
	}
}

// AdminListEntityReportsHandler handles GET /admin/entity-reports
func (h *EntityReportHandler) AdminListEntityReportsHandler(ctx context.Context, req *AdminListEntityReportsRequest) (*AdminListEntityReportsResponse, error) {
	if _, err := shared.RequireAdmin(ctx); err != nil {
		return nil, err
	}

	reports, total, err := h.entityReportService.ListEntityReports(&contracts.EntityReportFilters{
		Status:     req.Status,
		EntityType: req.EntityType,
		Limit:      req.Limit,
		Offset:     req.Offset,
	})
	if err != nil {
		logger.FromContext(ctx).Error("entity_report_list_failed", "error", err.Error())
		return nil, huma.Error500InternalServerError("Failed to list entity reports")
	}

	resp := &AdminListEntityReportsResponse{}
	resp.Body.Reports = reports
	resp.Body.Total = total
	return resp, nil
}

// ============================================================================
// Admin: Get Single Entity Report — GET /admin/entity-reports/{report_id}
// ============================================================================

// AdminGetEntityReportRequest is the Huma request for GET /admin/entity-reports/{report_id}
type AdminGetEntityReportRequest struct {
	ReportID string `path:"report_id" doc:"Report ID"`
}

// AdminGetEntityReportResponse is the Huma response for GET /admin/entity-reports/{report_id}
type AdminGetEntityReportResponse struct {
	Body *contracts.EntityReportResponse
}

// AdminGetEntityReportHandler handles GET /admin/entity-reports/{report_id}
func (h *EntityReportHandler) AdminGetEntityReportHandler(ctx context.Context, req *AdminGetEntityReportRequest) (*AdminGetEntityReportResponse, error) {
	if _, err := shared.RequireAdmin(ctx); err != nil {
		return nil, err
	}

	reportID, err := strconv.ParseUint(req.ReportID, 10, 64)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid report ID")
	}

	report, err := h.entityReportService.GetEntityReport(uint(reportID))
	if err != nil {
		logger.FromContext(ctx).Error("entity_report_get_failed", "report_id", reportID, "error", err.Error())
		return nil, huma.Error500InternalServerError("Failed to get entity report")
	}
	if report == nil {
		return nil, huma.Error404NotFound("Entity report not found")
	}

	return &AdminGetEntityReportResponse{Body: report}, nil
}

// ============================================================================
// Admin: Resolve Entity Report — POST /admin/entity-reports/{report_id}/resolve
// ============================================================================

// AdminResolveEntityReportRequest is the Huma request for POST /admin/entity-reports/{report_id}/resolve
type AdminResolveEntityReportRequest struct {
	ReportID string `path:"report_id" doc:"Report ID to resolve"`
	Body     struct {
		Notes string `json:"notes" required:"false" doc:"Optional admin notes about the resolution"`
	}
}

// AdminResolveEntityReportResponse is the Huma response for POST /admin/entity-reports/{report_id}/resolve
type AdminResolveEntityReportResponse struct {
	Body *contracts.EntityReportResponse
}

// AdminResolveEntityReportHandler handles POST /admin/entity-reports/{report_id}/resolve
func (h *EntityReportHandler) AdminResolveEntityReportHandler(ctx context.Context, req *AdminResolveEntityReportRequest) (*AdminResolveEntityReportResponse, error) {
	user, err := shared.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	reportID, err := strconv.ParseUint(req.ReportID, 10, 64)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid report ID")
	}

	notes := strings.TrimSpace(req.Body.Notes)

	resolved, err := h.entityReportService.ResolveEntityReport(uint(reportID), user.ID, notes)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, huma.Error404NotFound("Report not found")
		}
		if strings.Contains(err.Error(), "already been reviewed") {
			return nil, huma.Error409Conflict(err.Error())
		}
		logger.FromContext(ctx).Error("entity_report_resolve_failed",
			"report_id", reportID,
			"admin_id", user.ID,
			"error", err.Error(),
		)
		return nil, huma.Error500InternalServerError("Failed to resolve report")
	}

	logger.FromContext(ctx).Info("entity_report_resolved",
		"report_id", reportID,
		"admin_id", user.ID,
		"entity_type", resolved.EntityType,
		"entity_id", resolved.EntityID,
	)

	// Fire-and-forget audit log
	if h.auditLogService != nil {
		go h.auditLogService.LogAction(user.ID, "resolve_entity_report", resolved.EntityType, resolved.EntityID, map[string]interface{}{
			"report_id":   resolved.ID,
			"report_type": resolved.ReportType,
			"notes":       notes,
		})
	}

	return &AdminResolveEntityReportResponse{Body: resolved}, nil
}

// ============================================================================
// Admin: Dismiss Entity Report — POST /admin/entity-reports/{report_id}/dismiss
// ============================================================================

// AdminDismissEntityReportRequest is the Huma request for POST /admin/entity-reports/{report_id}/dismiss
type AdminDismissEntityReportRequest struct {
	ReportID string `path:"report_id" doc:"Report ID to dismiss"`
	Body     struct {
		Notes string `json:"notes" required:"false" doc:"Optional admin notes about the dismissal"`
	}
}

// AdminDismissEntityReportResponse is the Huma response for POST /admin/entity-reports/{report_id}/dismiss
type AdminDismissEntityReportResponse struct {
	Body *contracts.EntityReportResponse
}

// AdminDismissEntityReportHandler handles POST /admin/entity-reports/{report_id}/dismiss
func (h *EntityReportHandler) AdminDismissEntityReportHandler(ctx context.Context, req *AdminDismissEntityReportRequest) (*AdminDismissEntityReportResponse, error) {
	user, err := shared.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	reportID, err := strconv.ParseUint(req.ReportID, 10, 64)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid report ID")
	}

	notes := strings.TrimSpace(req.Body.Notes)

	dismissed, err := h.entityReportService.DismissEntityReport(uint(reportID), user.ID, notes)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, huma.Error404NotFound("Report not found")
		}
		if strings.Contains(err.Error(), "already been reviewed") {
			return nil, huma.Error409Conflict(err.Error())
		}
		logger.FromContext(ctx).Error("entity_report_dismiss_failed",
			"report_id", reportID,
			"admin_id", user.ID,
			"error", err.Error(),
		)
		return nil, huma.Error500InternalServerError("Failed to dismiss report")
	}

	logger.FromContext(ctx).Info("entity_report_dismissed",
		"report_id", reportID,
		"admin_id", user.ID,
		"entity_type", dismissed.EntityType,
		"entity_id", dismissed.EntityID,
	)

	// Fire-and-forget audit log
	if h.auditLogService != nil {
		go h.auditLogService.LogAction(user.ID, "dismiss_entity_report", dismissed.EntityType, dismissed.EntityID, map[string]interface{}{
			"report_id":   dismissed.ID,
			"report_type": dismissed.ReportType,
			"notes":       notes,
		})
	}

	return &AdminDismissEntityReportResponse{Body: dismissed}, nil
}
