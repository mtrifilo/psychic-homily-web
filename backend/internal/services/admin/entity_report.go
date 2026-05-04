package admin

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	communitym "psychic-homily-backend/internal/models/community"
	"psychic-homily-backend/internal/services/contracts"
)

// EntityReportService handles business logic for generalized entity reports.
type EntityReportService struct {
	db *gorm.DB
}

// NewEntityReportService creates a new EntityReportService.
func NewEntityReportService(database *gorm.DB) *EntityReportService {
	if database == nil {
		database = db.GetDB()
	}
	return &EntityReportService{db: database}
}

// CreateEntityReport submits a new report for an entity.
func (s *EntityReportService) CreateEntityReport(req *contracts.CreateEntityReportRequest) (*contracts.EntityReportResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	if !communitym.IsValidEntityReportEntityType(req.EntityType) {
		return nil, fmt.Errorf("invalid entity type: %s", req.EntityType)
	}
	if !communitym.IsValidReportType(req.EntityType, req.ReportType) {
		return nil, fmt.Errorf("invalid report type '%s' for entity type '%s'", req.ReportType, req.EntityType)
	}

	// Verify the entity exists
	tableName := req.EntityType + "s"
	// Comments table is already plural
	if req.EntityType == "comment" {
		tableName = "comments"
	}
	var count int64
	if err := s.db.Table(tableName).Where("id = ?", req.EntityID).Count(&count).Error; err != nil {
		return nil, fmt.Errorf("failed to verify entity: %w", err)
	}
	if count == 0 {
		return nil, fmt.Errorf("entity not found: %s %d", req.EntityType, req.EntityID)
	}

	// Check for existing pending report from this user for this entity
	var existingCount int64
	if err := s.db.Model(&communitym.EntityReport{}).
		Where("entity_type = ? AND entity_id = ? AND reported_by = ? AND status = ?",
			req.EntityType, req.EntityID, req.UserID, communitym.EntityReportStatusPending).
		Count(&existingCount).Error; err != nil {
		return nil, fmt.Errorf("failed to check existing report: %w", err)
	}
	if existingCount > 0 {
		return nil, fmt.Errorf("you already have a pending report for this entity")
	}

	report := &communitym.EntityReport{
		EntityType: req.EntityType,
		EntityID:   req.EntityID,
		ReportedBy: req.UserID,
		ReportType: req.ReportType,
		Details:    req.Details,
		Status:     communitym.EntityReportStatusPending,
	}

	if err := s.db.Create(report).Error; err != nil {
		return nil, fmt.Errorf("failed to create entity report: %w", err)
	}

	// Auto-hide comments with 3+ reports
	if req.EntityType == "comment" {
		var totalReports int64
		if err := s.db.Model(&communitym.EntityReport{}).
			Where("entity_type = 'comment' AND entity_id = ? AND status = ?",
				req.EntityID, communitym.EntityReportStatusPending).
			Count(&totalReports).Error; err == nil && totalReports >= 3 {
			// Auto-hide the comment
			s.db.Table("comments").Where("id = ? AND visibility = 'visible'", req.EntityID).
				Updates(map[string]interface{}{
					"visibility":    "hidden_by_mod",
					"hidden_reason": "auto-hidden: multiple reports",
					"updated_at":    time.Now(),
				})
		}
	}

	// Reload with relationships
	return s.GetEntityReport(report.ID)
}

// GetEntityReport returns a single report by ID.
func (s *EntityReportService) GetEntityReport(reportID uint) (*contracts.EntityReportResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var report communitym.EntityReport
	err := s.db.Preload("Reporter").Preload("Reviewer").First(&report, reportID).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get entity report: %w", err)
	}

	return s.toResponse(&report), nil
}

// GetEntityReports returns all reports for a specific entity.
func (s *EntityReportService) GetEntityReports(entityType string, entityID uint) ([]contracts.EntityReportResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var reports []communitym.EntityReport
	err := s.db.Where("entity_type = ? AND entity_id = ?", entityType, entityID).
		Preload("Reporter").
		Preload("Reviewer").
		Order("created_at DESC").
		Find(&reports).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get entity reports: %w", err)
	}

	return s.toResponses(reports), nil
}

// ListEntityReports returns reports for the admin review queue.
func (s *EntityReportService) ListEntityReports(filters *contracts.EntityReportFilters) ([]contracts.EntityReportResponse, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	limit := 20
	offset := 0
	if filters != nil {
		if filters.Limit > 0 && filters.Limit <= 100 {
			limit = filters.Limit
		}
		if filters.Offset > 0 {
			offset = filters.Offset
		}
	}

	query := s.db.Model(&communitym.EntityReport{})

	if filters != nil {
		if filters.Status != "" {
			query = query.Where("status = ?", filters.Status)
		}
		if filters.EntityType != "" {
			query = query.Where("entity_type = ?", filters.EntityType)
		}
	}

	var total int64
	query.Count(&total)

	var reports []communitym.EntityReport
	err := query.
		Preload("Reporter").
		Preload("Reviewer").
		Order("created_at ASC").
		Limit(limit).
		Offset(offset).
		Find(&reports).Error
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list entity reports: %w", err)
	}

	return s.toResponses(reports), total, nil
}

// ResolveEntityReport marks a report as resolved (action was taken).
func (s *EntityReportService) ResolveEntityReport(reportID uint, reviewerID uint, notes string) (*contracts.EntityReportResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var report communitym.EntityReport
	if err := s.db.First(&report, reportID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("report not found")
		}
		return nil, fmt.Errorf("failed to get report: %w", err)
	}

	if report.Status != communitym.EntityReportStatusPending {
		return nil, fmt.Errorf("report has already been reviewed (status: %s)", report.Status)
	}

	now := time.Now()
	updates := map[string]interface{}{
		"status":      communitym.EntityReportStatusResolved,
		"reviewed_by": reviewerID,
		"reviewed_at": now,
	}
	if notes != "" {
		updates["admin_notes"] = notes
	}

	if err := s.db.Model(&report).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("failed to resolve report: %w", err)
	}

	return s.GetEntityReport(reportID)
}

// DismissEntityReport marks a report as dismissed (spam/invalid).
func (s *EntityReportService) DismissEntityReport(reportID uint, reviewerID uint, notes string) (*contracts.EntityReportResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var report communitym.EntityReport
	if err := s.db.First(&report, reportID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("report not found")
		}
		return nil, fmt.Errorf("failed to get report: %w", err)
	}

	if report.Status != communitym.EntityReportStatusPending {
		return nil, fmt.Errorf("report has already been reviewed (status: %s)", report.Status)
	}

	now := time.Now()
	updates := map[string]interface{}{
		"status":      communitym.EntityReportStatusDismissed,
		"reviewed_by": reviewerID,
		"reviewed_at": now,
	}
	if notes != "" {
		updates["admin_notes"] = notes
	}

	if err := s.db.Model(&report).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("failed to dismiss report: %w", err)
	}

	return s.GetEntityReport(reportID)
}

// toResponse converts an EntityReport model to a response DTO.
func (s *EntityReportService) toResponse(report *communitym.EntityReport) *contracts.EntityReportResponse {
	resp := &contracts.EntityReportResponse{
		ID:         report.ID,
		EntityType: report.EntityType,
		EntityID:   report.EntityID,
		EntityName: resolveEntityName(s.db, report.EntityType, report.EntityID),
		EntitySlug: resolveEntitySlug(s.db, report.EntityType, report.EntityID),
		ReportedBy: report.ReportedBy,
		ReportType: report.ReportType,
		Details:    report.Details,
		Status:     string(report.Status),
		AdminNotes: report.AdminNotes,
		ReviewedBy: report.ReviewedBy,
		ReviewedAt: report.ReviewedAt,
		CreatedAt:  report.CreatedAt,
	}

	// Resolve reporter name
	if report.Reporter.ID != 0 {
		resp.ReporterName = displayName(&report.Reporter)
	}

	// Resolve reviewer name
	if report.Reviewer != nil && report.Reviewer.ID != 0 {
		resp.ReviewerName = displayName(report.Reviewer)
	}

	return resp
}

// toResponses converts a slice of models to response DTOs.
func (s *EntityReportService) toResponses(reports []communitym.EntityReport) []contracts.EntityReportResponse {
	responses := make([]contracts.EntityReportResponse, len(reports))
	for i := range reports {
		responses[i] = *s.toResponse(&reports[i])
	}
	return responses
}
