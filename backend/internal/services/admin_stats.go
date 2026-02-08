package services

import (
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/models"
)

// AdminStatsService handles admin dashboard statistics
type AdminStatsService struct {
	db *gorm.DB
}

// NewAdminStatsService creates a new admin stats service
func NewAdminStatsService(database *gorm.DB) *AdminStatsService {
	if database == nil {
		database = db.GetDB()
	}
	return &AdminStatsService{
		db: database,
	}
}

// AdminDashboardStats contains all dashboard statistics
type AdminDashboardStats struct {
	// Action items (things that need admin attention)
	PendingShows      int64 `json:"pending_shows"`
	PendingVenueEdits int64 `json:"pending_venue_edits"`
	PendingReports    int64 `json:"pending_reports"`
	UnverifiedVenues  int64 `json:"unverified_venues"`

	// Content totals
	TotalShows   int64 `json:"total_shows"`
	TotalVenues  int64 `json:"total_venues"`
	TotalArtists int64 `json:"total_artists"`

	// Users
	TotalUsers int64 `json:"total_users"`

	// Recent activity (last 7 days)
	ShowsSubmittedLast7Days  int64 `json:"shows_submitted_last_7_days"`
	UsersRegisteredLast7Days int64 `json:"users_registered_last_7_days"`
}

// GetDashboardStats returns all dashboard statistics
func (s *AdminStatsService) GetDashboardStats() (*AdminDashboardStats, error) {
	stats := &AdminDashboardStats{}
	sevenDaysAgo := time.Now().AddDate(0, 0, -7)

	// Action items
	if err := s.db.Model(&models.Show{}).Where("status = ?", models.ShowStatusPending).Count(&stats.PendingShows).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&models.PendingVenueEdit{}).Where("status = ?", "pending").Count(&stats.PendingVenueEdits).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&models.ShowReport{}).Where("status = ?", models.ShowReportStatusPending).Count(&stats.PendingReports).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&models.Venue{}).Where("verified = ?", false).Count(&stats.UnverifiedVenues).Error; err != nil {
		return nil, err
	}

	// Content totals
	if err := s.db.Model(&models.Show{}).Where("status = ?", models.ShowStatusApproved).Count(&stats.TotalShows).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&models.Venue{}).Where("verified = ?", true).Count(&stats.TotalVenues).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&models.Artist{}).Count(&stats.TotalArtists).Error; err != nil {
		return nil, err
	}

	// Users
	if err := s.db.Model(&models.User{}).Count(&stats.TotalUsers).Error; err != nil {
		return nil, err
	}

	// Recent activity
	if err := s.db.Model(&models.Show{}).Where("created_at > ?", sevenDaysAgo).Count(&stats.ShowsSubmittedLast7Days).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&models.User{}).Where("created_at > ?", sevenDaysAgo).Count(&stats.UsersRegisteredLast7Days).Error; err != nil {
		return nil, err
	}

	return stats, nil
}
