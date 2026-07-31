package community

import (
	"time"

	"psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/models/catalog"
)

// ArtistReportType represents the type of issue being reported
type ArtistReportType string

const (
	ArtistReportTypeInaccurate     ArtistReportType = "inaccurate"
	ArtistReportTypeRemovalRequest ArtistReportType = "removal_request"
)

// ArtistReport is a row in the RETIRED artist_reports table.
//
// PSY-1633 removed every writer: reporting an artist goes through the generic
// entity pipeline and lands in entity_reports, which is also where the
// read-back and the moderation queue look. Nothing creates an ArtistReport any
// more, and nothing should — a second artist report table is what let a report
// be filed in one place and searched for in another.
//
// The type survives only for readers that must still account for rows written
// before the consolidation: artist merges (services/catalog), the contributor
// "reports filed" tally, and the admin pending-count. Dropping the table — and
// this model with it — is deliberately a separate change.
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
	Artist   catalog.Artist `gorm:"foreignKey:ArtistID"`
	Reporter *auth.User     `gorm:"foreignKey:ReportedBy"`
	Reviewer *auth.User     `gorm:"foreignKey:ReviewedBy"`
}

// TableName specifies the table name for ArtistReport
func (ArtistReport) TableName() string {
	return "artist_reports"
}
