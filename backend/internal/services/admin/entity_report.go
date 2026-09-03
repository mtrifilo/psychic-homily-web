package admin

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	apperrors "psychic-homily-backend/internal/errors"
	communitym "psychic-homily-backend/internal/models/community"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
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

// commentParentVisible reports whether viewer may see the entity comment
// commentID hangs off.
//
// A comment that does not exist answers false, which is what lets a gated parent
// and a missing comment id produce one response. Which entity types have a rule
// is services/shared's registry, so a comment on an always-visible parent passes
// without a second lookup.
func (s *EntityReportService) commentParentVisible(commentID uint, viewer contracts.ShowViewer) bool {
	var parent struct {
		EntityType string
		EntityID   uint
	}
	err := s.db.Table("comments").
		Select("entity_type, entity_id").
		Where("id = ?", commentID).
		Scan(&parent).Error
	if err != nil || parent.EntityType == "" {
		return false
	}
	return shared.EntityVisibleTo(
		shared.NewShowVisibilityService(s.db), parent.EntityType, parent.EntityID, viewer)
}

// CreateEntityReport submits a new report for an entity.
func (s *EntityReportService) CreateEntityReport(req *contracts.CreateEntityReportRequest) (*contracts.EntityReportResponse, error) {
	if s.db == nil {
		return nil, apperrors.ErrEntityReportInternal(fmt.Errorf("database not initialized"))
	}

	if !communitym.IsValidEntityReportEntityType(req.EntityType) {
		return nil, apperrors.ErrEntityReportInvalidEntityType(req.EntityType)
	}
	if !communitym.IsValidReportType(req.EntityType, req.ReportType) {
		return nil, apperrors.ErrEntityReportInvalidReportType(req.ReportType, req.EntityType)
	}

	// Verify the entity exists AND that the reporter may see it.
	//
	// The existence probe alone is an oracle over a dense id space, and the
	// response this call returns carries the entity's resolved name and slug
	// (toResponse), so a bare probe hands a private collection's identity to
	// whoever guesses its id. A collection the reporter may not see answers
	// exactly as one that does not exist.
	//
	// A COLLECTION is the only reportable type gated HERE. Shows have a read-time
	// rule of their own and are deliberately not gated on this route: refusing a
	// report on a gated show would remove the only way a submitter can report
	// their own withdrawn show. That trade does not arise for collections, whose
	// creator is the one person who can see them. Any OTHER reportable type that
	// gains a read-time rule needs a case here rather than the default branch,
	// which probes existence and nothing else.
	//
	// A COMMENT is decided by its PARENT, because that is where the rule lives
	// and because the response resolves a comment's name as the first 60
	// characters of its body. Comment ids are dense, so an ungated probe is both
	// a content disclosure and an enumeration primitive.
	viewer := contracts.ShowViewer{UserID: req.UserID}
	switch req.EntityType {
	case communitym.EntityReportEntityCollection:
		if !shared.CollectionVisibleTo(s.db, req.EntityID, viewer) {
			return nil, apperrors.ErrEntityReportEntityNotFound(req.EntityType, req.EntityID)
		}
	case communitym.EntityReportEntityComment:
		if !s.commentParentVisible(req.EntityID, viewer) {
			return nil, apperrors.ErrEntityReportEntityNotFound(req.EntityType, req.EntityID)
		}
	default:
		// Every type reaching here has a plural table name and no read-time rule.
		// The two that do not are the cases above.
		tableName := req.EntityType + "s"
		var count int64
		if err := s.db.Table(tableName).Where("id = ?", req.EntityID).Count(&count).Error; err != nil {
			return nil, apperrors.ErrEntityReportInternal(fmt.Errorf("failed to verify entity: %w", err))
		}
		if count == 0 {
			return nil, apperrors.ErrEntityReportEntityNotFound(req.EntityType, req.EntityID)
		}
	}

	// Check for existing pending report from this user for this entity
	var existingCount int64
	if err := s.db.Model(&communitym.EntityReport{}).
		Where("entity_type = ? AND entity_id = ? AND reported_by = ? AND status = ?",
			req.EntityType, req.EntityID, req.UserID, communitym.EntityReportStatusPending).
		Count(&existingCount).Error; err != nil {
		return nil, apperrors.ErrEntityReportInternal(fmt.Errorf("failed to check existing report: %w", err))
	}
	if existingCount > 0 {
		return nil, apperrors.ErrEntityReportDuplicatePending()
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
		return nil, apperrors.ErrEntityReportInternal(fmt.Errorf("failed to create entity report: %w", err))
	}

	// Auto-hide comments with 3+ reports
	if req.EntityType == communitym.EntityReportEntityComment {
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
		return nil, apperrors.ErrEntityReportInternal(fmt.Errorf("database not initialized"))
	}

	var report communitym.EntityReport
	err := s.db.Preload("Reporter").Preload("Reviewer").First(&report, reportID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, apperrors.ErrEntityReportInternal(fmt.Errorf("failed to get entity report: %w", err))
	}

	return s.toResponse(&report), nil
}

// GetEntityReports returns all reports for a specific entity.
func (s *EntityReportService) GetEntityReports(entityType string, entityID uint) ([]contracts.EntityReportResponse, error) {
	if s.db == nil {
		return nil, apperrors.ErrEntityReportInternal(fmt.Errorf("database not initialized"))
	}

	var reports []communitym.EntityReport
	err := s.db.Where("entity_type = ? AND entity_id = ?", entityType, entityID).
		Preload("Reporter").
		Preload("Reviewer").
		Order("created_at DESC").
		Find(&reports).Error
	if err != nil {
		return nil, apperrors.ErrEntityReportInternal(fmt.Errorf("failed to get entity reports: %w", err))
	}

	return s.toResponses(reports), nil
}

// GetUserPendingReport returns the caller's own pending report for an entity,
// or (nil, nil) when they have none.
//
// Scoped to pending for two independent reasons, so do not widen it:
//
//   - Correctness. It backs the "you already reported this" state, and
//     CreateEntityReport only rejects a duplicate while a prior report is still
//     pending. Returning a resolved or dismissed report would block a
//     submission the API would actually accept.
//   - Disclosure. This is the one report endpoint a non-admin can read, and it
//     returns the full EntityReportResponse. A reviewed report carries
//     AdminNotes and the reviewing admin's name — moderator-internal fields.
//     On a pending report both are still nil, so the response cannot carry
//     them.
func (s *EntityReportService) GetUserPendingReport(userID uint, entityType string, entityID uint) (*contracts.EntityReportResponse, error) {
	if s.db == nil {
		return nil, apperrors.ErrEntityReportInternal(fmt.Errorf("database not initialized"))
	}

	// No Reviewer preload: a pending report has not been reviewed by
	// definition, so the join could only ever return nothing.
	var report communitym.EntityReport
	err := s.db.
		Where("entity_type = ? AND entity_id = ? AND reported_by = ? AND status = ?",
			entityType, entityID, userID, communitym.EntityReportStatusPending).
		Preload("Reporter").
		First(&report).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, apperrors.ErrEntityReportInternal(fmt.Errorf("failed to get user report: %w", err))
	}

	return s.toResponse(&report), nil
}

// ListEntityReports returns reports for the admin review queue.
func (s *EntityReportService) ListEntityReports(filters *contracts.EntityReportFilters) ([]contracts.EntityReportResponse, int64, error) {
	if s.db == nil {
		return nil, 0, apperrors.ErrEntityReportInternal(fmt.Errorf("database not initialized"))
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
		return nil, 0, apperrors.ErrEntityReportInternal(fmt.Errorf("failed to list entity reports: %w", err))
	}

	return s.toResponses(reports), total, nil
}

// ResolveEntityReport marks a report as resolved (action was taken).
func (s *EntityReportService) ResolveEntityReport(reportID uint, reviewerID uint, notes string) (*contracts.EntityReportResponse, error) {
	if s.db == nil {
		return nil, apperrors.ErrEntityReportInternal(fmt.Errorf("database not initialized"))
	}

	var report communitym.EntityReport
	if err := s.db.First(&report, reportID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrEntityReportNotFound()
		}
		return nil, apperrors.ErrEntityReportInternal(fmt.Errorf("failed to get report: %w", err))
	}

	if report.Status != communitym.EntityReportStatusPending {
		return nil, apperrors.ErrEntityReportAlreadyReviewed(string(report.Status))
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
		return nil, apperrors.ErrEntityReportInternal(fmt.Errorf("failed to resolve report: %w", err))
	}

	return s.GetEntityReport(reportID)
}

// DismissEntityReport marks a report as dismissed (spam/invalid).
func (s *EntityReportService) DismissEntityReport(reportID uint, reviewerID uint, notes string) (*contracts.EntityReportResponse, error) {
	if s.db == nil {
		return nil, apperrors.ErrEntityReportInternal(fmt.Errorf("database not initialized"))
	}

	var report communitym.EntityReport
	if err := s.db.First(&report, reportID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrEntityReportNotFound()
		}
		return nil, apperrors.ErrEntityReportInternal(fmt.Errorf("failed to get report: %w", err))
	}

	if report.Status != communitym.EntityReportStatusPending {
		return nil, apperrors.ErrEntityReportAlreadyReviewed(string(report.Status))
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
		return nil, apperrors.ErrEntityReportInternal(fmt.Errorf("failed to dismiss report: %w", err))
	}

	return s.GetEntityReport(reportID)
}

// toResponse converts an EntityReport model to a response DTO.
func (s *EntityReportService) toResponse(report *communitym.EntityReport) *contracts.EntityReportResponse {
	name, slug := resolveEntityNameAndSlug(s.db, report.EntityType, report.EntityID)
	resp := &contracts.EntityReportResponse{
		ID:         report.ID,
		EntityType: report.EntityType,
		EntityID:   report.EntityID,
		EntityName: name,
		EntitySlug: slug,
		ReportedBy: report.ReportedBy,
		ReportType: report.ReportType,
		Details:    report.Details,
		Status:     string(report.Status),
		AdminNotes: report.AdminNotes,
		ReviewedBy: report.ReviewedBy,
		ReviewedAt: report.ReviewedAt,
		CreatedAt:  report.CreatedAt,
	}

	if report.Reporter.ID != 0 {
		resp.ReporterName = shared.ResolveUserName(&report.Reporter)
		resp.ReporterUsername = shared.ResolveUserUsername(&report.Reporter)
	}

	if report.Reviewer != nil && report.Reviewer.ID != 0 {
		resp.ReviewerName = shared.ResolveUserName(report.Reviewer)
		resp.ReviewerUsername = shared.ResolveUserUsername(report.Reviewer)
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
