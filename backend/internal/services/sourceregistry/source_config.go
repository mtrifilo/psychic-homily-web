// Package sourceregistry manages the polymorphic source-config registry that
// drives the Catalog Refresh stale-first refresh loop (PSY-1149). It tracks an
// external source per catalog entity (venue calendar / label roster) and the
// staleness signal (LastRefreshedAt) the loop orders by. It is intentionally
// independent of the legacy venue extraction pipeline (PSY-1158); the /ingest
// skill is the executor and stamps refreshes via RecordRefresh.
package sourceregistry

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"psychic-homily-backend/db"
	adminm "psychic-homily-backend/internal/models/admin"
)

// SourceConfigService manages source-config registry rows.
type SourceConfigService struct {
	db *gorm.DB
}

// NewSourceConfigService creates a new source config service.
func NewSourceConfigService(database *gorm.DB) *SourceConfigService {
	if database == nil {
		database = db.GetDB()
	}
	return &SourceConfigService{db: database}
}

func isValidEntityType(entityType string) bool {
	return entityType == adminm.SourceEntityVenue || entityType == adminm.SourceEntityLabel
}

// GetByEntity returns the source config for an entity, or nil if not registered.
func (s *SourceConfigService) GetByEntity(entityType string, entityID uint) (*adminm.SourceConfig, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var cfg adminm.SourceConfig
	err := s.db.Where("entity_type = ? AND entity_id = ?", entityType, entityID).First(&cfg).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get source config: %w", err)
	}

	return &cfg, nil
}

// CreateOrUpdate upserts a source config keyed by (entity_type, entity_id),
// updating the source URL when the row already exists.
func (s *SourceConfigService) CreateOrUpdate(cfg *adminm.SourceConfig) (*adminm.SourceConfig, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if cfg.EntityID == 0 {
		return nil, fmt.Errorf("entity_id is required")
	}
	if !isValidEntityType(cfg.EntityType) {
		return nil, fmt.Errorf("invalid entity_type %q", cfg.EntityType)
	}

	result := s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "entity_type"}, {Name: "entity_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"source_url", "updated_at"}),
	}).Create(cfg)

	if result.Error != nil {
		return nil, fmt.Errorf("failed to upsert source config: %w", result.Error)
	}

	return s.GetByEntity(cfg.EntityType, cfg.EntityID)
}

// RecordRefresh stamps a successful refresh: sets last_refreshed_at to now,
// resets consecutive_failures, and updates the content hash when provided.
// Errors if the entity has no registered source config.
func (s *SourceConfigService) RecordRefresh(entityType string, entityID uint, contentHash *string) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	now := time.Now()
	updates := map[string]interface{}{
		"last_refreshed_at":    now,
		"consecutive_failures": 0,
		"updated_at":           now,
	}
	if contentHash != nil {
		updates["last_content_hash"] = contentHash
	}

	result := s.db.Model(&adminm.SourceConfig{}).
		Where("entity_type = ? AND entity_id = ?", entityType, entityID).
		Updates(updates)

	if result.Error != nil {
		return fmt.Errorf("failed to record refresh: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("source config not found for %s %d", entityType, entityID)
	}

	return nil
}

// IncrementFailures increments the consecutive_failures counter after a failed
// refresh. Errors if the entity has no registered source config.
func (s *SourceConfigService) IncrementFailures(entityType string, entityID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	result := s.db.Model(&adminm.SourceConfig{}).
		Where("entity_type = ? AND entity_id = ?", entityType, entityID).
		Updates(map[string]interface{}{
			"consecutive_failures": gorm.Expr("consecutive_failures + 1"),
			"updated_at":           time.Now(),
		})

	if result.Error != nil {
		return fmt.Errorf("failed to increment failures: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("source config not found for %s %d", entityType, entityID)
	}

	return nil
}

// ListStale returns source configs ordered stalest-first (never-refreshed rows
// first, then oldest last_refreshed_at). When maxFailures > 0, rows at or over
// that failure count are excluded (circuit-broken sources). When limit > 0, the
// result is capped to that many rows.
func (s *SourceConfigService) ListStale(limit, maxFailures int) ([]adminm.SourceConfig, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	q := s.db.Model(&adminm.SourceConfig{}).Order("last_refreshed_at ASC NULLS FIRST")
	if maxFailures > 0 {
		q = q.Where("consecutive_failures < ?", maxFailures)
	}
	if limit > 0 {
		q = q.Limit(limit)
	}

	var configs []adminm.SourceConfig
	if err := q.Find(&configs).Error; err != nil {
		return nil, fmt.Errorf("failed to list stale source configs: %w", err)
	}

	return configs, nil
}
