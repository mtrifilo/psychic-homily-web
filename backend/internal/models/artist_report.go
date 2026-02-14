package models

import "time"

// ArtistReportType represents the type of issue being reported
type ArtistReportType string

const (
	ArtistReportTypeInaccurate     ArtistReportType = "inaccurate"
	ArtistReportTypeRemovalRequest ArtistReportType = "removal_request"
)

// ArtistReport represents a user report about an artist issue
type ArtistReport struct {
	ID         uint             `gorm:"primaryKey"`
	ArtistID   uint             `gorm:"not null"`
	ReportedBy uint             `gorm:"column:reported_by;not null"`
	ReportType ArtistReportType `gorm:"type:artist_report_type;not null"`
	Details    *string          `gorm:"column:details"`
	Status     ShowReportStatus `gorm:"type:show_report_status;not null;default:'pending'"`
	AdminNotes *string          `gorm:"column:admin_notes"`
	ReviewedBy *uint            `gorm:"column:reviewed_by"`
	ReviewedAt *time.Time       `gorm:"column:reviewed_at"`
	CreatedAt  time.Time        `gorm:"not null"`
	UpdatedAt  time.Time        `gorm:"not null"`

	// Relationships
	Artist   Artist `gorm:"foreignKey:ArtistID"`
	Reporter *User  `gorm:"foreignKey:ReportedBy"`
	Reviewer *User  `gorm:"foreignKey:ReviewedBy"`
}

// TableName specifies the table name for ArtistReport
func (ArtistReport) TableName() string {
	return "artist_reports"
}
