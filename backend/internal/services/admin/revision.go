package admin

import (
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	adminm "psychic-homily-backend/internal/models/admin"
)

// RevisionService handles revision history business logic.
type RevisionService struct {
	db *gorm.DB
}

// NewRevisionService creates a new revision service.
func NewRevisionService(database *gorm.DB) *RevisionService {
	if database == nil {
		database = db.GetDB()
	}
	return &RevisionService{db: database}
}

// RecordRevision creates a new revision entry for an entity edit.
// If changes is empty, it is a no-op (no revision recorded).
func (s *RevisionService) RecordRevision(entityType string, entityID uint, userID uint, changes []adminm.FieldChange, summary string) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}
	if len(changes) == 0 {
		return nil // No changes, nothing to record
	}

	changesJSON, err := json.Marshal(changes)
	if err != nil {
		return fmt.Errorf("failed to marshal field changes: %w", err)
	}
	raw := json.RawMessage(changesJSON)

	var summaryPtr *string
	if summary != "" {
		summaryPtr = &summary
	}

	revision := &adminm.Revision{
		EntityType:   entityType,
		EntityID:     entityID,
		UserID:       userID,
		FieldChanges: &raw,
		Summary:      summaryPtr,
	}

	if err := s.db.Create(revision).Error; err != nil {
		return fmt.Errorf("failed to create revision: %w", err)
	}
	return nil
}

// GetEntityHistory returns paginated revision history for a specific entity.
func (s *RevisionService) GetEntityHistory(entityType string, entityID uint, limit, offset int) ([]adminm.Revision, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	var total int64
	s.db.Model(&adminm.Revision{}).
		Where("entity_type = ? AND entity_id = ?", entityType, entityID).
		Count(&total)

	var revisions []adminm.Revision
	err := s.db.Where("entity_type = ? AND entity_id = ?", entityType, entityID).
		Preload("User").
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&revisions).Error
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get entity history: %w", err)
	}

	return revisions, total, nil
}

// GetRevision retrieves a single revision by ID.
// Returns nil, nil if not found.
func (s *RevisionService) GetRevision(revisionID uint) (*adminm.Revision, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var revision adminm.Revision
	err := s.db.Preload("User").First(&revision, revisionID).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get revision: %w", err)
	}
	return &revision, nil
}

// GetUserRevisions returns paginated revisions made by a specific user.
func (s *RevisionService) GetUserRevisions(userID uint, limit, offset int) ([]adminm.Revision, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	var total int64
	s.db.Model(&adminm.Revision{}).Where("user_id = ?", userID).Count(&total)

	var revisions []adminm.Revision
	err := s.db.Where("user_id = ?", userID).
		Preload("User").
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&revisions).Error
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get user revisions: %w", err)
	}

	return revisions, total, nil
}

// Rollback applies the inverse of a revision's changes to the entity.
// It creates a new revision recording the rollback.
func (s *RevisionService) Rollback(revisionID uint, adminUserID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	revision, err := s.GetRevision(revisionID)
	if err != nil {
		return err
	}
	if revision == nil {
		return fmt.Errorf("revision not found")
	}

	// Parse field changes
	var changes []adminm.FieldChange
	if err := json.Unmarshal(*revision.FieldChanges, &changes); err != nil {
		return fmt.Errorf("failed to parse field changes: %w", err)
	}

	// Build update map from old values (reversing the change)
	updates := make(map[string]interface{})
	var rollbackChanges []adminm.FieldChange
	for _, c := range changes {
		updates[c.Field] = c.OldValue
		rollbackChanges = append(rollbackChanges, adminm.FieldChange{
			Field:    c.Field,
			OldValue: c.NewValue,
			NewValue: c.OldValue,
		})
	}

	// Apply update to the entity table
	tableName := revision.EntityType + "s" // artist -> artists, show -> shows, etc.
	updates["updated_at"] = time.Now()

	result := s.db.Table(tableName).Where("id = ?", revision.EntityID).Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("failed to apply rollback: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("entity not found: %s %d", revision.EntityType, revision.EntityID)
	}

	// Record the rollback as a new revision
	summary := fmt.Sprintf("Rollback of revision #%d", revisionID)
	return s.RecordRevision(revision.EntityType, revision.EntityID, adminUserID, rollbackChanges, summary)
}
