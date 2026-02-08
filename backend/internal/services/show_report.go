package services

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/models"
)

// ShowReportService handles show report business logic
type ShowReportService struct {
	db *gorm.DB
}

// NewShowReportService creates a new show report service
func NewShowReportService(database *gorm.DB) *ShowReportService {
	if database == nil {
		database = db.GetDB()
	}
	return &ShowReportService{
		db: database,
	}
}

// ShowReportResponse represents a show report response with show info
type ShowReportResponse struct {
	ID         uint      `json:"id"`
	ShowID     uint      `json:"show_id"`
	ReportType string    `json:"report_type"`
	Details    *string   `json:"details"`
	Status     string    `json:"status"`
	AdminNotes *string   `json:"admin_notes,omitempty"`
	ReviewedBy *uint     `json:"reviewed_by,omitempty"`
	ReviewedAt *string   `json:"reviewed_at,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`

	// Show info (for admin view)
	Show *ShowReportShowInfo `json:"show,omitempty"`
}

// ShowReportShowInfo contains show information for report responses
type ShowReportShowInfo struct {
	ID        uint      `json:"id"`
	Title     string    `json:"title"`
	Slug      string    `json:"slug"`
	EventDate time.Time `json:"event_date"`
	City      *string   `json:"city"`
	State     *string   `json:"state"`
}

// CreateReport creates a new show report
func (s *ShowReportService) CreateReport(userID, showID uint, reportType string, details *string) (*ShowReportResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Validate report type
	if reportType != string(models.ShowReportTypeCancelled) &&
		reportType != string(models.ShowReportTypeSoldOut) &&
		reportType != string(models.ShowReportTypeInaccurate) {
		return nil, fmt.Errorf("invalid report type: %s", reportType)
	}

	// Verify show exists
	var show models.Show
	if err := s.db.First(&show, showID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("show not found")
		}
		return nil, fmt.Errorf("failed to verify show: %w", err)
	}

	// Check for existing report from this user for this show
	var existingCount int64
	if err := s.db.Model(&models.ShowReport{}).
		Where("show_id = ? AND reported_by = ?", showID, userID).
		Count(&existingCount).Error; err != nil {
		return nil, fmt.Errorf("failed to check existing report: %w", err)
	}

	if existingCount > 0 {
		return nil, fmt.Errorf("you have already reported this show")
	}

	// Create the report
	report := models.ShowReport{
		ShowID:     showID,
		ReportedBy: userID,
		ReportType: models.ShowReportType(reportType),
		Details:    details,
		Status:     models.ShowReportStatusPending,
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}

	if err := s.db.Create(&report).Error; err != nil {
		return nil, fmt.Errorf("failed to create report: %w", err)
	}

	return s.buildReportResponse(&report, &show), nil
}

// GetUserReportForShow returns the user's existing report for a show, if any
func (s *ShowReportService) GetUserReportForShow(userID, showID uint) (*ShowReportResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var report models.ShowReport
	err := s.db.Where("show_id = ? AND reported_by = ?", showID, userID).
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
func (s *ShowReportService) GetPendingReports(limit, offset int) ([]*ShowReportResponse, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	// Get total count
	var total int64
	if err := s.db.Model(&models.ShowReport{}).
		Where("status = ?", models.ShowReportStatusPending).
		Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count pending reports: %w", err)
	}

	// Get reports with show info
	var reports []models.ShowReport
	err := s.db.Preload("Show").
		Where("status = ?", models.ShowReportStatusPending).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&reports).Error

	if err != nil {
		return nil, 0, fmt.Errorf("failed to get pending reports: %w", err)
	}

	// Build responses
	responses := make([]*ShowReportResponse, len(reports))
	for i, report := range reports {
		responses[i] = s.buildReportResponse(&report, &report.Show)
	}

	return responses, total, nil
}

// DismissReport marks a report as dismissed (spam/invalid)
func (s *ShowReportService) DismissReport(reportID, adminID uint, notes *string) (*ShowReportResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var report models.ShowReport
	if err := s.db.Preload("Show").First(&report, reportID).Error; err != nil {
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

	return s.buildReportResponse(&report, &report.Show), nil
}

// ResolveReport marks a report as resolved (action was taken)
func (s *ShowReportService) ResolveReport(reportID, adminID uint, notes *string) (*ShowReportResponse, error) {
	return s.ResolveReportWithFlag(reportID, adminID, notes, false)
}

// ResolveReportWithFlag marks a report as resolved and optionally sets the corresponding show flag.
// If setShowFlag is true and the report type is cancelled or sold_out, the show's corresponding
// flag (is_cancelled or is_sold_out) will be set to true.
func (s *ShowReportService) ResolveReportWithFlag(reportID, adminID uint, notes *string, setShowFlag bool) (*ShowReportResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var report models.ShowReport
	if err := s.db.Preload("Show").First(&report, reportID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("report not found")
		}
		return nil, fmt.Errorf("failed to get report: %w", err)
	}

	if report.Status != models.ShowReportStatusPending {
		return nil, fmt.Errorf("report has already been reviewed")
	}

	// Use transaction if we're updating show flag
	err := s.db.Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		report.Status = models.ShowReportStatusResolved
		report.ReviewedBy = &adminID
		report.ReviewedAt = &now
		report.AdminNotes = notes
		report.UpdatedAt = now

		if err := tx.Save(&report).Error; err != nil {
			return fmt.Errorf("failed to resolve report: %w", err)
		}

		// If setShowFlag is true, update the show's corresponding flag
		if setShowFlag {
			var updateField string
			switch report.ReportType {
			case models.ShowReportTypeCancelled:
				updateField = "is_cancelled"
			case models.ShowReportTypeSoldOut:
				updateField = "is_sold_out"
			}

			if updateField != "" {
				if err := tx.Model(&models.Show{}).
					Where("id = ?", report.ShowID).
					Update(updateField, true).Error; err != nil {
					return fmt.Errorf("failed to update show flag: %w", err)
				}
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	// Reload to get updated show data
	if err := s.db.Preload("Show").First(&report, reportID).Error; err != nil {
		return nil, fmt.Errorf("failed to reload report: %w", err)
	}

	return s.buildReportResponse(&report, &report.Show), nil
}

// GetReportByID returns a report by ID (used for Discord notifications)
func (s *ShowReportService) GetReportByID(reportID uint) (*models.ShowReport, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var report models.ShowReport
	if err := s.db.Preload("Show").First(&report, reportID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("report not found")
		}
		return nil, fmt.Errorf("failed to get report: %w", err)
	}

	return &report, nil
}

// buildReportResponse builds a ShowReportResponse from a model
func (s *ShowReportService) buildReportResponse(report *models.ShowReport, show *models.Show) *ShowReportResponse {
	resp := &ShowReportResponse{
		ID:         report.ID,
		ShowID:     report.ShowID,
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

	if show != nil {
		slug := ""
		if show.Slug != nil {
			slug = *show.Slug
		}
		resp.Show = &ShowReportShowInfo{
			ID:        show.ID,
			Title:     show.Title,
			Slug:      slug,
			EventDate: show.EventDate,
			City:      show.City,
			State:     show.State,
		}
	}

	return resp
}
