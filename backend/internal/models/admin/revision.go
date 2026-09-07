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
//
// OldValueWithheld is a THREE-state stamp, and the third state is the reason it
// is a pointer:
//
//   - true: OldValue is the blank served in place of a column this change's
//     audience is not shown. It is NOT what the field held.
//   - false: OldValue is the value the recorder read off the entity.
//   - nil: nothing recorded either way. Every row written before the stamp
//     existed reads this way, and so does any row a writer that does not stamp
//     produces.
//
// A plain bool would collapse nil into false, which is the one collapse a
// consumer must not make: a blank the pipeline WITHHELD and a blank the column
// genuinely held are the same three characters in the same slot, and only this
// stamp separates them. Rollback writes OldValue back into the column, so
// treating an unstamped blank as observed is how a real street address gets
// overwritten with "".
//
// It is a stamp beside the value rather than a sentinel inside it because
// OldValue is untyped and reaches a column verbatim: any sentinel string would
// be a value some column could legitimately hold, and every reader of the
// history would have to know it.
type FieldChange struct {
	Field            string      `json:"field"`
	OldValue         interface{} `json:"old_value"`
	NewValue         interface{} `json:"new_value"`
	OldValueWithheld *bool       `json:"old_value_withheld,omitempty"`
}

// WithOldValueWithheld returns a copy of the change carrying the stamp. Value
// receiver and a fresh pointer per call, so no two changes share the target.
func (c FieldChange) WithOldValueWithheld(withheld bool) FieldChange {
	c.OldValueWithheld = &withheld
	return c
}

// OldValueIsWithheld reports whether the change positively records that its
// OldValue is a withheld blank. An unstamped change reports false: absence of a
// stamp is absence of knowledge, not a claim that the value was observed. Use
// OldValueUnstamped to tell the two apart.
func (c FieldChange) OldValueIsWithheld() bool {
	return c.OldValueWithheld != nil && *c.OldValueWithheld
}

// OldValueUnstamped reports whether the change records nothing about where its
// OldValue came from.
func (c FieldChange) OldValueUnstamped() bool { return c.OldValueWithheld == nil }

// ForServing returns the changes with the stamp cleared, and never mutates the
// input, which is unmarshalled from a stored row other code paths read raw.
//
// THE STAMP IS STORAGE, NOT PAYLOAD, and serving it would publish the one bit
// the gate behind it exists to hide. A field is stamped withheld only when its
// column is SET, so `true` on an unverified venue's address says "this house
// has a street address on record" to anyone who asks to edit it, which is
// exactly what deriving the mask rather than the column was for: the derived
// value is the same whatever the column holds. Every path that serves a
// FieldChange to a client goes through here.
func ForServing(changes []FieldChange) []FieldChange {
	out := make([]FieldChange, len(changes))
	copy(out, changes)
	for i := range out {
		out[i].OldValueWithheld = nil
	}
	return out
}
