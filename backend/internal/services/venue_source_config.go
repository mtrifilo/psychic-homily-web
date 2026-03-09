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
			"strategy_locked", "auto_approve", "extraction_notes", "updated_at",
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

// VenueRejectionStats contains rejection breakdown and approval rate for a venue's pipeline shows.
type VenueRejectionStats struct {
	TotalExtracted       int64              `json:"total_extracted"`
	Approved             int64              `json:"approved"`
	Rejected             int64              `json:"rejected"`
	Pending              int64              `json:"pending"`
	RejectionBreakdown   map[string]int64   `json:"rejection_breakdown"`
	ApprovalRate         float64            `json:"approval_rate"`
	SuggestedAutoApprove bool               `json:"suggested_auto_approve"`
}

// GetRejectionStats computes approval/rejection statistics for pipeline-sourced shows at a venue.
func (s *VenueSourceConfigService) GetRejectionStats(venueID uint) (*VenueRejectionStats, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Get the venue's slug for matching source_venue
	var venue models.Venue
	if err := s.db.First(&venue, venueID).Error; err != nil {
		return nil, fmt.Errorf("venue not found: %w", err)
	}
	if venue.Slug == nil || *venue.Slug == "" {
		return nil, fmt.Errorf("venue %d has no slug", venueID)
	}

	// Count shows by status where source = 'discovery' and source_venue matches
	type statusCount struct {
		Status string
		Count  int64
	}
	var statusCounts []statusCount
	err := s.db.Model(&models.Show{}).
		Select("status, COUNT(*) as count").
		Where("source = ? AND source_venue = ?", "discovery", *venue.Slug).
		Group("status").
		Find(&statusCounts).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get status counts: %w", err)
	}

	stats := &VenueRejectionStats{
		RejectionBreakdown: make(map[string]int64),
	}
	for _, sc := range statusCounts {
		switch sc.Status {
		case "approved":
			stats.Approved = sc.Count
		case "rejected":
			stats.Rejected = sc.Count
		case "pending":
			stats.Pending = sc.Count
		}
		stats.TotalExtracted += sc.Count
	}

	// Get rejection category breakdown
	type categoryCount struct {
		RejectionCategory string
		Count             int64
	}
	var categoryCounts []categoryCount
	err = s.db.Model(&models.Show{}).
		Select("COALESCE(rejection_category, 'uncategorized') as rejection_category, COUNT(*) as count").
		Where("source = ? AND source_venue = ? AND status = ?", "discovery", *venue.Slug, "rejected").
		Group("rejection_category").
		Find(&categoryCounts).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get rejection breakdown: %w", err)
	}

	for _, cc := range categoryCounts {
		stats.RejectionBreakdown[cc.RejectionCategory] = cc.Count
	}

	// Compute approval rate (only count decided shows, not pending)
	decided := stats.Approved + stats.Rejected
	if decided > 0 {
		stats.ApprovalRate = float64(stats.Approved) / float64(decided)
	}

	// Suggest auto-approve if approval rate > 90% and at least 20 decided shows
	stats.SuggestedAutoApprove = stats.ApprovalRate > 0.9 && decided > 20

	return stats, nil
}

// UpdateExtractionNotes updates the extraction_notes field for a venue config.
func (s *VenueSourceConfigService) UpdateExtractionNotes(venueID uint, notes *string) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	result := s.db.Model(&models.VenueSourceConfig{}).
		Where("venue_id = ?", venueID).
		Updates(map[string]interface{}{
			"extraction_notes": notes,
			"updated_at":       time.Now(),
		})

	if result.Error != nil {
		return fmt.Errorf("failed to update extraction notes: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("venue source config not found for venue %d", venueID)
	}

	return nil
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
