package admin

import (
	"time"

	"psychic-homily-backend/internal/models/auth"
)

// Featured-slot type discriminators. The /explore landing has two
// admin-curated editorial slots: a Featured Bill (a show) and a Featured
// Collection. entity_id resolves to the corresponding table by slot_type
// at the service layer — there is no DB-level FK because the referent
// table varies (polymorphic, mirrors pending_entity_edits).
const (
	FeaturedSlotTypeBill       = "bill"
	FeaturedSlotTypeCollection = "collection"
)

// MaxFeaturedSlotCuratorNoteLength caps the per-pick editorial note. The
// 10,000-character mirror of comment / field-note bodies keeps the
// markdown renderer / sanitizer policy consistent across surfaces.
const MaxFeaturedSlotCuratorNoteLength = 10000

// IsValidFeaturedSlotType reports whether s is one of the recognised
// slot types. Used by service + handler input validation before hitting
// the DB CHECK constraint.
func IsValidFeaturedSlotType(s string) bool {
	return s == FeaturedSlotTypeBill || s == FeaturedSlotTypeCollection
}

// FeaturedSlot is a single admin-curated pick for the /explore landing.
// One row per slot_type is active at a time (active_until IS NULL),
// enforced by a partial unique index. Setting a new active row retires
// the previous one (active_until = NOW()) atomically inside a
// transaction so the unique-index invariant always holds.
type FeaturedSlot struct {
	ID          uint       `gorm:"primaryKey"`
	SlotType    string     `gorm:"column:slot_type;not null"`
	EntityID    uint       `gorm:"column:entity_id;not null"`
	CuratorNote *string    `gorm:"column:curator_note"`
	ActiveFrom  time.Time  `gorm:"column:active_from;not null"`
	ActiveUntil *time.Time `gorm:"column:active_until"`
	CreatedBy   uint       `gorm:"column:created_by;not null"`
	CreatedAt   time.Time  `gorm:"not null"`
	UpdatedAt   time.Time  `gorm:"not null"`

	Creator auth.User `gorm:"foreignKey:CreatedBy"`
}

// TableName specifies the table name for FeaturedSlot.
func (FeaturedSlot) TableName() string {
	return "featured_slots"
}
