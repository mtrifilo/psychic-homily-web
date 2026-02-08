package handlers

import (
	"context"
	"fmt"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services"
)

// AuditLogHandler handles audit log HTTP requests
type AuditLogHandler struct {
	auditLogService *services.AuditLogService
}

// NewAuditLogHandler creates a new audit log handler
func NewAuditLogHandler() *AuditLogHandler {
	return &AuditLogHandler{
		auditLogService: services.NewAuditLogService(nil),
	}
}

// GetAuditLogsRequest represents the HTTP request for listing audit logs
type GetAuditLogsRequest struct {
	Limit      int    `query:"limit" default:"50" doc:"Number of logs to return (max 100)"`
	Offset     int    `query:"offset" default:"0" doc:"Offset for pagination"`
	EntityType string `query:"entity_type" doc:"Filter by entity type (show, venue, venue_edit, show_report)"`
	Action     string `query:"action" doc:"Filter by action (approve_show, reject_show, etc.)"`
}

// GetAuditLogsResponse represents the HTTP response for listing audit logs
type GetAuditLogsResponse struct {
	Body struct {
		Logs  []*services.AuditLogResponse `json:"logs"`
		Total int64                        `json:"total"`
	}
}

// GetAuditLogsHandler handles GET /admin/audit-logs
func (h *AuditLogHandler) GetAuditLogsHandler(ctx context.Context, req *GetAuditLogsRequest) (*GetAuditLogsResponse, error) {
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

	logger.FromContext(ctx).Debug("admin_audit_logs_attempt",
		"limit", limit,
		"offset", req.Offset,
		"entity_type", req.EntityType,
		"action", req.Action,
	)

	// Build filters
	filters := services.AuditLogFilters{
		EntityType: req.EntityType,
		Action:     req.Action,
	}

	// Get audit logs
	logs, total, err := h.auditLogService.GetAuditLogs(limit, req.Offset, filters)
	if err != nil {
		logger.FromContext(ctx).Error("admin_audit_logs_failed",
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to get audit logs (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Debug("admin_audit_logs_success",
		"count", len(logs),
		"total", total,
	)

	return &GetAuditLogsResponse{
		Body: struct {
			Logs  []*services.AuditLogResponse `json:"logs"`
			Total int64                        `json:"total"`
		}{
			Logs:  logs,
			Total: total,
		},
	}, nil
}
