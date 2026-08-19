package admin

import (
	"encoding/json"
	"time"

	"psychic-homily-backend/internal/models/auth"
)

// Revision tracks a single edit to an entity with field-level diffs.
type Revision struct {
	ID           uint             `json:"id" gorm:"primaryKey"`
	EntityType   string           `json:"entity_type" gorm:"column:entity_type;not null;size:50"`
	EntityID     uint             `json:"entity_id" gorm:"column:entity_id;not null"`
	UserID       uint             `json:"user_id" gorm:"column:user_id;not null"`
	FieldChanges *json.RawMessage `json:"field_changes" gorm:"column:field_changes;type:jsonb;not null"`
	Summary      *string          `json:"summary,omitempty" gorm:"column:summary"`
	CreatedAt    time.Time        `json:"created_at"`

	// FromUnverifiedVenue records that a venue merge re-pointed this revision
	// off an UNVERIFIED venue onto some other venue row.
	//
	// It exists because address redaction is decided at read time from the
	// venue the revision currently points at, and the merge deletes the venue
	// it was decided from. Written only by the merge (see catalog.MergeVenues);
	// read only by the redaction gate, which masks a marked row whatever the
	// current venue says. It is NOT part of any API response; the served shape
	// is handlers/admin.RevisionResponseItem.
	FromUnverifiedVenue bool `json:"-" gorm:"column:from_unverified_venue;not null;default:false"`

	// FromGatedShow records that a show merge re-pointed this revision off a
	// NON-APPROVED show onto some other show row.
	//
	// The show-side twin of FromUnverifiedVenue, and it exists for the same
	// reason: show revision visibility is decided at read time from
	// shows.status for the show the revision currently points at, and
	// catalog.MergeDuplicateShow deletes the show it was decided from. Written
	// only by that merge; read only by the visibility gate, which suppresses a
	// marked row for non-admin callers whatever the current show says. It is
	// NOT part of any API response.
	FromGatedShow bool `json:"-" gorm:"column:from_gated_show;not null;default:false"`

	User auth.User `json:"-" gorm:"foreignKey:UserID"`
}

// TableName specifies the table name for Revision.
func (Revision) TableName() string { return "revisions" }

// FieldChange represents a single field's before/after values.
type FieldChange struct {
	Field    string      `json:"field"`
	OldValue interface{} `json:"old_value"`
	NewValue interface{} `json:"new_value"`
}
