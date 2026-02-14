package services

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/models"
)

// ArtistReportService handles artist report business logic
type ArtistReportService struct {
	db *gorm.DB
}

// NewArtistReportService creates a new artist report service
func NewArtistReportService(database *gorm.DB) *ArtistReportService {
	if database == nil {
		database = db.GetDB()
	}
	return &ArtistReportService{
		db: database,
	}
}

// ArtistReportResponse represents an artist report response with artist info
type ArtistReportResponse struct {
	ID         uint      `json:"id"`
	ArtistID   uint      `json:"artist_id"`
	ReportType string    `json:"report_type"`
	Details    *string   `json:"details"`
	Status     string    `json:"status"`
	AdminNotes *string   `json:"admin_notes,omitempty"`
	ReviewedBy *uint     `json:"reviewed_by,omitempty"`
	ReviewedAt *string   `json:"reviewed_at,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`

	// Artist info (for admin view)
	Artist *ArtistReportArtistInfo `json:"artist,omitempty"`
}

// ArtistReportArtistInfo contains artist information for report responses
type ArtistReportArtistInfo struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// CreateReport creates a new artist report
func (s *ArtistReportService) CreateReport(userID, artistID uint, reportType string, details *string) (*ArtistReportResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Validate report type
	if reportType != string(models.ArtistReportTypeInaccurate) &&
		reportType != string(models.ArtistReportTypeRemovalRequest) {
		return nil, fmt.Errorf("invalid report type: %s", reportType)
	}

	// Verify artist exists
	var artist models.Artist
	if err := s.db.First(&artist, artistID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("artist not found")
		}
		return nil, fmt.Errorf("failed to verify artist: %w", err)
	}

	// Check for existing report from this user for this artist
	var existingCount int64
	if err := s.db.Model(&models.ArtistReport{}).
		Where("artist_id = ? AND reported_by = ?", artistID, userID).
		Count(&existingCount).Error; err != nil {
		return nil, fmt.Errorf("failed to check existing report: %w", err)
	}

	if existingCount > 0 {
		return nil, fmt.Errorf("you have already reported this artist")
	}

	// Create the report
	report := models.ArtistReport{
		ArtistID:   artistID,
		ReportedBy: userID,
		ReportType: models.ArtistReportType(reportType),
		Details:    details,
		Status:     models.ShowReportStatusPending,
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}

	if err := s.db.Create(&report).Error; err != nil {
		return nil, fmt.Errorf("failed to create report: %w", err)
	}

	return s.buildReportResponse(&report, &artist), nil
}

// GetUserReportForArtist returns the user's existing report for an artist, if any
func (s *ArtistReportService) GetUserReportForArtist(userID, artistID uint) (*ArtistReportResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var report models.ArtistReport
	err := s.db.Where("artist_id = ? AND reported_by = ?", artistID, userID).
		First(&report).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil // No report found
		}
		return nil, fmt.Errorf("failed to get report: %w", err)
	}

	return s.buildReportResponse(&report, nil), nil
}

// GetPendingReports returns pending reports for admin review
func (s *ArtistReportService) GetPendingReports(limit, offset int) ([]*ArtistReportResponse, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	// Get total count
	var total int64
	if err := s.db.Model(&models.ArtistReport{}).
		Where("status = ?", models.ShowReportStatusPending).
		Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count pending reports: %w", err)
	}

	// Get reports with artist info
	var reports []models.ArtistReport
	err := s.db.Preload("Artist").
		Where("status = ?", models.ShowReportStatusPending).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&reports).Error

	if err != nil {
		return nil, 0, fmt.Errorf("failed to get pending reports: %w", err)
	}

	// Build responses
	responses := make([]*ArtistReportResponse, len(reports))
	for i, report := range reports {
		responses[i] = s.buildReportResponse(&report, &report.Artist)
	}

	return responses, total, nil
}

// DismissReport marks a report as dismissed (spam/invalid)
func (s *ArtistReportService) DismissReport(reportID, adminID uint, notes *string) (*ArtistReportResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var report models.ArtistReport
	if err := s.db.Preload("Artist").First(&report, reportID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("report not found")
		}
		return nil, fmt.Errorf("failed to get report: %w", err)
	}

	if report.Status != models.ShowReportStatusPending {
		return nil, fmt.Errorf("report has already been reviewed")
	}

	now := time.Now().UTC()
	report.Status = models.ShowReportStatusDismissed
	report.ReviewedBy = &adminID
	report.ReviewedAt = &now
	report.AdminNotes = notes
	report.UpdatedAt = now

	if err := s.db.Save(&report).Error; err != nil {
		return nil, fmt.Errorf("failed to dismiss report: %w", err)
	}

	return s.buildReportResponse(&report, &report.Artist), nil
}

// ResolveReport marks a report as resolved (action was taken)
func (s *ArtistReportService) ResolveReport(reportID, adminID uint, notes *string) (*ArtistReportResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var report models.ArtistReport
	if err := s.db.Preload("Artist").First(&report, reportID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("report not found")
		}
		return nil, fmt.Errorf("failed to get report: %w", err)
	}

	if report.Status != models.ShowReportStatusPending {
		return nil, fmt.Errorf("report has already been reviewed")
	}

	now := time.Now().UTC()
	report.Status = models.ShowReportStatusResolved
	report.ReviewedBy = &adminID
	report.ReviewedAt = &now
	report.AdminNotes = notes
	report.UpdatedAt = now

	if err := s.db.Save(&report).Error; err != nil {
		return nil, fmt.Errorf("failed to resolve report: %w", err)
	}

	return s.buildReportResponse(&report, &report.Artist), nil
}

// GetReportByID returns a report by ID (used for Discord notifications)
func (s *ArtistReportService) GetReportByID(reportID uint) (*models.ArtistReport, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var report models.ArtistReport
	if err := s.db.Preload("Artist").First(&report, reportID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("report not found")
		}
		return nil, fmt.Errorf("failed to get report: %w", err)
	}

	return &report, nil
}

// buildReportResponse builds an ArtistReportResponse from a model
func (s *ArtistReportService) buildReportResponse(report *models.ArtistReport, artist *models.Artist) *ArtistReportResponse {
	resp := &ArtistReportResponse{
		ID:         report.ID,
		ArtistID:   report.ArtistID,
		ReportType: string(report.ReportType),
		Details:    report.Details,
		Status:     string(report.Status),
		AdminNotes: report.AdminNotes,
		ReviewedBy: report.ReviewedBy,
		CreatedAt:  report.CreatedAt,
		UpdatedAt:  report.UpdatedAt,
	}

	if report.ReviewedAt != nil {
		reviewedAtStr := report.ReviewedAt.Format(time.RFC3339)
		resp.ReviewedAt = &reviewedAtStr
	}

	if artist != nil {
		slug := ""
		if artist.Slug != nil {
			slug = *artist.Slug
		}
		resp.Artist = &ArtistReportArtistInfo{
			ID:   artist.ID,
			Name: artist.Name,
			Slug: slug,
		}
	}

	return resp
}
