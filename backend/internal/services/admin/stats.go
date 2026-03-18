package admin

import (
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
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

// GetDashboardStats returns all dashboard statistics
func (s *AdminStatsService) GetDashboardStats() (*contracts.AdminDashboardStats, error) {
	stats := &contracts.AdminDashboardStats{}
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
	if err := s.db.Model(&models.ArtistReport{}).Where("status = ?", models.ShowReportStatusPending).Count(&stats.PendingArtistReports).Error; err != nil {
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

	// Period-over-period trends (current 7 days vs previous 7 days)
	fourteenDaysAgo := time.Now().AddDate(0, 0, -14)

	var showsCurrent, showsPrevious int64
	if err := s.db.Model(&models.Show{}).Where("status = ? AND created_at > ?", models.ShowStatusApproved, sevenDaysAgo).Count(&showsCurrent).Error; err != nil {
		// Log but don't fail — trends are non-critical
		showsCurrent = 0
	}
	if err := s.db.Model(&models.Show{}).Where("status = ? AND created_at > ? AND created_at <= ?", models.ShowStatusApproved, fourteenDaysAgo, sevenDaysAgo).Count(&showsPrevious).Error; err != nil {
		showsPrevious = 0
	}
	stats.TotalShowsTrend = showsCurrent - showsPrevious

	var venuesCurrent, venuesPrevious int64
	if err := s.db.Model(&models.Venue{}).Where("verified = ? AND created_at > ?", true, sevenDaysAgo).Count(&venuesCurrent).Error; err != nil {
		venuesCurrent = 0
	}
	if err := s.db.Model(&models.Venue{}).Where("verified = ? AND created_at > ? AND created_at <= ?", true, fourteenDaysAgo, sevenDaysAgo).Count(&venuesPrevious).Error; err != nil {
		venuesPrevious = 0
	}
	stats.TotalVenuesTrend = venuesCurrent - venuesPrevious

	var artistsCurrent, artistsPrevious int64
	if err := s.db.Model(&models.Artist{}).Where("created_at > ?", sevenDaysAgo).Count(&artistsCurrent).Error; err != nil {
		artistsCurrent = 0
	}
	if err := s.db.Model(&models.Artist{}).Where("created_at > ? AND created_at <= ?", fourteenDaysAgo, sevenDaysAgo).Count(&artistsPrevious).Error; err != nil {
		artistsPrevious = 0
	}
	stats.TotalArtistsTrend = artistsCurrent - artistsPrevious

	var usersCurrent, usersPrevious int64
	if err := s.db.Model(&models.User{}).Where("created_at > ?", sevenDaysAgo).Count(&usersCurrent).Error; err != nil {
		usersCurrent = 0
	}
	if err := s.db.Model(&models.User{}).Where("created_at > ? AND created_at <= ?", fourteenDaysAgo, sevenDaysAgo).Count(&usersPrevious).Error; err != nil {
		usersPrevious = 0
	}
	stats.TotalUsersTrend = usersCurrent - usersPrevious

	return stats, nil
}
