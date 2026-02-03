package models

import "time"

// ShowReportType represents the type of issue being reported
type ShowReportType string

const (
	ShowReportTypeCancelled  ShowReportType = "cancelled"
	ShowReportTypeSoldOut    ShowReportType = "sold_out"
	ShowReportTypeInaccurate ShowReportType = "inaccurate"
)

// ShowReportStatus represents the status of a report
type ShowReportStatus string

const (
	ShowReportStatusPending   ShowReportStatus = "pending"
	ShowReportStatusDismissed ShowReportStatus = "dismissed"
	ShowReportStatusResolved  ShowReportStatus = "resolved"
)

// ShowReport represents a user report about a show issue
type ShowReport struct {
	ID         uint             `gorm:"primaryKey"`
	ShowID     uint             `gorm:"not null"`
	ReportedBy uint             `gorm:"column:reported_by;not null"`
	ReportType ShowReportType   `gorm:"type:show_report_type;not null"`
	Details    *string          `gorm:"column:details"`
	Status     ShowReportStatus `gorm:"type:show_report_status;not null;default:'pending'"`
	AdminNotes *string          `gorm:"column:admin_notes"`
	ReviewedBy *uint            `gorm:"column:reviewed_by"`
	ReviewedAt *time.Time       `gorm:"column:reviewed_at"`
	CreatedAt  time.Time        `gorm:"not null"`
	UpdatedAt  time.Time        `gorm:"not null"`

	// Relationships
	Show     Show  `gorm:"foreignKey:ShowID"`
	Reporter *User `gorm:"foreignKey:ReportedBy"`
	Reviewer *User `gorm:"foreignKey:ReviewedBy"`
}

// TableName specifies the table name for ShowReport
func (ShowReport) TableName() string {
	return "show_reports"
}
