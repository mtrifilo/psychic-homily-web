package models

import (
	"encoding/json"
	"time"
)

// PendingEditStatus represents the status of a pending entity edit.
type PendingEditStatus string

const (
	PendingEditStatusPending  PendingEditStatus = "pending"
	PendingEditStatusApproved PendingEditStatus = "approved"
	PendingEditStatusRejected PendingEditStatus = "rejected"
)

// Supported entity types for pending edits.
const (
	PendingEditEntityArtist   = "artist"
	PendingEditEntityVenue    = "venue"
	PendingEditEntityFestival = "festival"
	PendingEditEntityRelease  = "release"
)

// PendingEntityEdit represents a proposed edit to an entity awaiting review.
// Uses JSONB field_changes (same format as revisions) instead of per-entity nullable columns.
type PendingEntityEdit struct {
	ID              uint              `json:"id" gorm:"primaryKey"`
	EntityType      string            `json:"entity_type" gorm:"column:entity_type;not null;size:50"`
	EntityID        uint              `json:"entity_id" gorm:"column:entity_id;not null"`
	SubmittedBy     uint              `json:"submitted_by" gorm:"column:submitted_by;not null"`
	FieldChanges    *json.RawMessage  `json:"field_changes" gorm:"column:field_changes;type:jsonb;not null"`
	Summary         string            `json:"summary" gorm:"column:summary;not null"`
	Status          PendingEditStatus `json:"status" gorm:"column:status;not null;default:'pending'"`
	ReviewedBy      *uint             `json:"reviewed_by,omitempty" gorm:"column:reviewed_by"`
	ReviewedAt      *time.Time        `json:"reviewed_at,omitempty" gorm:"column:reviewed_at"`
	RejectionReason *string           `json:"rejection_reason,omitempty" gorm:"column:rejection_reason"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`

	Submitter User  `json:"-" gorm:"foreignKey:SubmittedBy"`
	Reviewer  *User `json:"-" gorm:"foreignKey:ReviewedBy"`
}

// TableName specifies the table name for PendingEntityEdit.
func (PendingEntityEdit) TableName() string { return "pending_entity_edits" }

// ValidEntityTypes returns the set of entity types that support pending edits.
func ValidPendingEditEntityTypes() []string {
	return []string{
		PendingEditEntityArtist,
		PendingEditEntityVenue,
		PendingEditEntityFestival,
		PendingEditEntityRelease,
	}
}

// IsValidPendingEditEntityType checks if the given entity type supports pending edits.
func IsValidPendingEditEntityType(entityType string) bool {
	for _, t := range ValidPendingEditEntityTypes() {
		if t == entityType {
			return true
		}
	}
	return false
}
