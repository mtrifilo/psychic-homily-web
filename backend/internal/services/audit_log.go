package services

import (
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/models"
)

// AuditLogService handles audit log business logic
type AuditLogService struct {
	db *gorm.DB
}

// NewAuditLogService creates a new audit log service
func NewAuditLogService() *AuditLogService {
	return &AuditLogService{
		db: db.GetDB(),
	}
}

// AuditLogFilters represents optional filters for querying audit logs
type AuditLogFilters struct {
	EntityType string
	Action     string
	ActorID    *uint
}

// AuditLogResponse represents an audit log entry in API responses
type AuditLogResponse struct {
	ID         uint                   `json:"id"`
	ActorID    *uint                  `json:"actor_id"`
	ActorEmail string                 `json:"actor_email,omitempty"`
	Action     string                 `json:"action"`
	EntityType string                 `json:"entity_type"`
	EntityID   uint                   `json:"entity_id"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt  time.Time              `json:"created_at"`
}

// LogAction records an admin action in the audit log.
// Errors are logged but not returned — audit logging should not fail the parent operation.
func (s *AuditLogService) LogAction(actorID uint, action string, entityType string, entityID uint, metadata map[string]interface{}) {
	if s.db == nil {
		logger.Default().Error("audit_log_failed", "error", "database not initialized")
		return
	}

	log := models.AuditLog{
		ActorID:    &actorID,
		Action:     action,
		EntityType: entityType,
		EntityID:   entityID,
		CreatedAt:  time.Now().UTC(),
	}

	if metadata != nil {
		metadataJSON, err := json.Marshal(metadata)
		if err != nil {
			logger.Default().Error("audit_log_metadata_marshal_failed",
				"error", err.Error(),
				"action", action,
				"entity_type", entityType,
				"entity_id", entityID,
			)
		} else {
			raw := json.RawMessage(metadataJSON)
			log.Metadata = &raw
		}
	}

	if err := s.db.Create(&log).Error; err != nil {
		logger.Default().Error("audit_log_create_failed",
			"error", err.Error(),
			"action", action,
			"entity_type", entityType,
			"entity_id", entityID,
			"actor_id", actorID,
		)
	}
}

// GetAuditLogs returns paginated audit log entries with optional filters
func (s *AuditLogService) GetAuditLogs(limit, offset int, filters AuditLogFilters) ([]*AuditLogResponse, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	query := s.db.Model(&models.AuditLog{})

	if filters.EntityType != "" {
		query = query.Where("entity_type = ?", filters.EntityType)
	}
	if filters.Action != "" {
		query = query.Where("action = ?", filters.Action)
	}
	if filters.ActorID != nil {
		query = query.Where("actor_id = ?", *filters.ActorID)
	}

	// Get total count
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count audit logs: %w", err)
	}

	// Get logs with actor preloaded
	var logs []models.AuditLog
	err := query.Preload("Actor").
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&logs).Error
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get audit logs: %w", err)
	}

	// Build responses
	responses := make([]*AuditLogResponse, len(logs))
	for i, log := range logs {
		responses[i] = s.buildResponse(&log)
	}

	return responses, total, nil
}

func (s *AuditLogService) buildResponse(log *models.AuditLog) *AuditLogResponse {
	resp := &AuditLogResponse{
		ID:         log.ID,
		ActorID:    log.ActorID,
		Action:     log.Action,
		EntityType: log.EntityType,
		EntityID:   log.EntityID,
		CreatedAt:  log.CreatedAt,
	}

	if log.Actor != nil && log.Actor.Email != nil {
		resp.ActorEmail = *log.Actor.Email
	}

	if log.Metadata != nil {
		var metadata map[string]interface{}
		if err := json.Unmarshal(*log.Metadata, &metadata); err == nil {
			resp.Metadata = metadata
		}
	}

	return resp
}
