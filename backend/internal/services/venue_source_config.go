package services

import (
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/models"
)

// VenueSourceConfigService manages per-venue extraction configuration and run history.
type VenueSourceConfigService struct {
	db *gorm.DB
}

// NewVenueSourceConfigService creates a new venue source config service.
func NewVenueSourceConfigService(database *gorm.DB) *VenueSourceConfigService {
	if database == nil {
		database = db.GetDB()
	}
	return &VenueSourceConfigService{
		db: database,
	}
}

// GetByVenueID returns the source config for a venue, or nil if not configured.
func (s *VenueSourceConfigService) GetByVenueID(venueID uint) (*models.VenueSourceConfig, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var config models.VenueSourceConfig
	err := s.db.Where("venue_id = ?", venueID).First(&config).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get venue source config: %w", err)
	}

	return &config, nil
}

// CreateOrUpdate upserts a venue source config. If a config for the venue already
// exists, it updates the mutable fields; otherwise it creates a new record.
func (s *VenueSourceConfigService) CreateOrUpdate(config *models.VenueSourceConfig) (*models.VenueSourceConfig, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	if config.VenueID == 0 {
		return nil, fmt.Errorf("venue_id is required")
	}

	result := s.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "venue_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"calendar_url", "preferred_source", "render_method", "feed_url",
			"strategy_locked", "auto_approve", "updated_at",
		}),
	}).Create(config)

	if result.Error != nil {
		return nil, fmt.Errorf("failed to upsert venue source config: %w", result.Error)
	}

	return s.GetByVenueID(config.VenueID)
}

// UpdateAfterRun updates extraction metadata after a successful run.
func (s *VenueSourceConfigService) UpdateAfterRun(venueID uint, contentHash, etag *string, eventsExtracted int) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	now := time.Now()
	result := s.db.Model(&models.VenueSourceConfig{}).
		Where("venue_id = ?", venueID).
		Updates(map[string]interface{}{
			"last_extracted_at":    now,
			"last_content_hash":    contentHash,
			"last_etag":            etag,
			"consecutive_failures": 0,
			"events_expected":      gorm.Expr("(events_expected + ?) / 2", eventsExtracted),
			"updated_at":           now,
		})

	if result.Error != nil {
		return fmt.Errorf("failed to update after run: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("venue source config not found for venue %d", venueID)
	}

	return nil
}

// IncrementFailures increments the consecutive_failures counter for a venue.
func (s *VenueSourceConfigService) IncrementFailures(venueID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	result := s.db.Model(&models.VenueSourceConfig{}).
		Where("venue_id = ?", venueID).
		Updates(map[string]interface{}{
			"consecutive_failures": gorm.Expr("consecutive_failures + 1"),
			"updated_at":           time.Now(),
		})

	if result.Error != nil {
		return fmt.Errorf("failed to increment failures: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("venue source config not found for venue %d", venueID)
	}

	return nil
}

// RecordRun inserts a new extraction run record.
func (s *VenueSourceConfigService) RecordRun(run *models.VenueExtractionRun) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	if run.VenueID == 0 {
		return fmt.Errorf("venue_id is required")
	}

	if err := s.db.Create(run).Error; err != nil {
		return fmt.Errorf("failed to record extraction run: %w", err)
	}

	return nil
}

// GetRecentRuns returns the most recent extraction runs for a venue, ordered by run_at desc.
func (s *VenueSourceConfigService) GetRecentRuns(venueID uint, limit int) ([]models.VenueExtractionRun, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	var runs []models.VenueExtractionRun
	err := s.db.Where("venue_id = ?", venueID).
		Order("run_at DESC").
		Limit(limit).
		Find(&runs).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get recent runs: %w", err)
	}

	return runs, nil
}

// ListConfigured returns all venue source configs, preloading the venue association.
func (s *VenueSourceConfigService) ListConfigured() ([]models.VenueSourceConfig, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var configs []models.VenueSourceConfig
	err := s.db.Preload("Venue").Find(&configs).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list configured venues: %w", err)
	}

	return configs, nil
}
