package contracts

import (
	"time"
)

// ──────────────────────────────────────────────
// Entity Report Service Interface
// ──────────────────────────────────────────────

// EntityReportServiceInterface defines the contract for managing generalized entity reports.
type EntityReportServiceInterface interface {
	// CreateEntityReport submits a new report for an entity.
	CreateEntityReport(req *CreateEntityReportRequest) (*EntityReportResponse, error)

	// GetEntityReport returns a single report by ID.
	GetEntityReport(reportID uint) (*EntityReportResponse, error)

	// GetEntityReports returns all reports for a specific entity.
	GetEntityReports(entityType string, entityID uint) ([]EntityReportResponse, error)

	// ListEntityReports returns reports for the admin review queue.
	ListEntityReports(filters *EntityReportFilters) ([]EntityReportResponse, int64, error)

	// ResolveEntityReport marks a report as resolved (action was taken).
	ResolveEntityReport(reportID uint, reviewerID uint, notes string) (*EntityReportResponse, error)

	// DismissEntityReport marks a report as dismissed (spam/invalid).
	DismissEntityReport(reportID uint, reviewerID uint, notes string) (*EntityReportResponse, error)
}

// ──────────────────────────────────────────────
// Request / Response Types
// ──────────────────────────────────────────────

// CreateEntityReportRequest contains the data needed to submit an entity report.
type CreateEntityReportRequest struct {
	EntityType string  `json:"entity_type"`
	EntityID   uint    `json:"entity_id"`
	UserID     uint    `json:"-"`
	ReportType string  `json:"report_type"`
	Details    *string `json:"details,omitempty"`
}

// EntityReportFilters contains filters for listing entity reports.
type EntityReportFilters struct {
	Status     string `json:"status,omitempty"`      // "pending", "resolved", "dismissed"
	EntityType string `json:"entity_type,omitempty"` // "artist", "venue", "festival", "show"
	Limit      int    `json:"limit,omitempty"`
	Offset     int    `json:"offset,omitempty"`
}

// EntityReportResponse is the API response for an entity report.
type EntityReportResponse struct {
	ID         uint   `json:"id"`
	EntityType string `json:"entity_type"`
	EntityID   uint   `json:"entity_id"`
	EntityName string `json:"entity_name,omitempty"`
	// EntitySlug is populated only for entity types addressed by slug in the
	// public app (currently `collection`). Other entity types use ID-based
	// URLs and leave this nil so the JSON omits the field. PSY-357.
	EntitySlug   *string    `json:"entity_slug,omitempty"`
	ReportedBy   uint       `json:"reported_by"`
	ReporterName string     `json:"reporter_name,omitempty"`
	ReportType   string     `json:"report_type"`
	Details      *string    `json:"details,omitempty"`
	Status       string     `json:"status"`
	AdminNotes   *string    `json:"admin_notes,omitempty"`
	ReviewedBy   *uint      `json:"reviewed_by,omitempty"`
	ReviewerName string     `json:"reviewer_name,omitempty"`
	ReviewedAt   *time.Time `json:"reviewed_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}
