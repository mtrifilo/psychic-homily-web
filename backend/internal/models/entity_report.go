package models

import "time"

// EntityReportStatus represents the status of an entity report.
type EntityReportStatus string

const (
	EntityReportStatusPending   EntityReportStatus = "pending"
	EntityReportStatusResolved  EntityReportStatus = "resolved"
	EntityReportStatusDismissed EntityReportStatus = "dismissed"
)

// Supported entity types for entity reports.
const (
	EntityReportEntityArtist   = "artist"
	EntityReportEntityVenue    = "venue"
	EntityReportEntityFestival = "festival"
	EntityReportEntityShow     = "show"
)

// Valid report types per entity type.
var validReportTypes = map[string]map[string]bool{
	EntityReportEntityArtist: {
		"inaccurate":      true,
		"duplicate":       true,
		"wrong_image":     true,
		"removal_request": true,
		"missing_info":    true,
	},
	EntityReportEntityVenue: {
		"closed_permanently": true,
		"wrong_address":      true,
		"duplicate":          true,
		"inaccurate":         true,
		"missing_info":       true,
	},
	EntityReportEntityFestival: {
		"cancelled":  true,
		"wrong_dates": true,
		"duplicate":  true,
		"inaccurate": true,
	},
	EntityReportEntityShow: {
		"cancelled":   true,
		"sold_out":    true,
		"inaccurate":  true,
		"wrong_venue": true,
		"wrong_date":  true,
	},
}

// EntityReport represents a user report about an entity issue.
// Uses entity_type + entity_id polymorphism instead of per-entity tables.
type EntityReport struct {
	ID         uint               `json:"id" gorm:"primaryKey"`
	EntityType string             `json:"entity_type" gorm:"column:entity_type;not null;size:50"`
	EntityID   uint               `json:"entity_id" gorm:"column:entity_id;not null"`
	ReportedBy uint               `json:"reported_by" gorm:"column:reported_by;not null"`
	ReportType string             `json:"report_type" gorm:"column:report_type;not null;size:50"`
	Details    *string            `json:"details,omitempty" gorm:"column:details"`
	Status     EntityReportStatus `json:"status" gorm:"column:status;not null;default:'pending'"`
	AdminNotes *string            `json:"admin_notes,omitempty" gorm:"column:admin_notes"`
	ReviewedBy *uint              `json:"reviewed_by,omitempty" gorm:"column:reviewed_by"`
	ReviewedAt *time.Time         `json:"reviewed_at,omitempty" gorm:"column:reviewed_at"`
	CreatedAt  time.Time          `json:"created_at"`

	Reporter User  `json:"-" gorm:"foreignKey:ReportedBy"`
	Reviewer *User `json:"-" gorm:"foreignKey:ReviewedBy"`
}

// TableName specifies the table name for EntityReport.
func (EntityReport) TableName() string { return "entity_reports" }

// ValidEntityReportEntityTypes returns the set of entity types that support reports.
func ValidEntityReportEntityTypes() []string {
	return []string{EntityReportEntityArtist, EntityReportEntityVenue, EntityReportEntityFestival, EntityReportEntityShow}
}

// IsValidEntityReportEntityType checks if the given entity type supports reports.
func IsValidEntityReportEntityType(entityType string) bool {
	_, ok := validReportTypes[entityType]
	return ok
}

// IsValidReportType checks if the given report type is valid for the entity type.
func IsValidReportType(entityType, reportType string) bool {
	types, ok := validReportTypes[entityType]
	if !ok {
		return false
	}
	return types[reportType]
}

// ValidReportTypesForEntity returns the valid report types for a given entity type.
func ValidReportTypesForEntity(entityType string) []string {
	types, ok := validReportTypes[entityType]
	if !ok {
		return nil
	}
	result := make([]string, 0, len(types))
	for t := range types {
		result = append(result, t)
	}
	return result
}
