package catalog

import "time"

// VenueConfirmation is one user's "this venue's info is still accurate" tap.
//
// The cheapest possible contribution: it edits nothing, so any authenticated
// user at any trust tier can leave one, and the only thing it costs a reader
// is a timestamp they can trust. Composite PK (user_id, venue_id) makes the
// confirmation inherently unique, so the write is INSERT ... ON CONFLICT DO
// NOTHING and a repeat tap is a no-op rather than an error -- the same shape
// collection likes use.
//
// Deliberately NOT denormalised onto venues: no confirmation_count column.
// Counts are aggregated at read time, batched over a page of venues.
type VenueConfirmation struct {
	UserID    uint      `gorm:"primaryKey;column:user_id" json:"user_id"`
	VenueID   uint      `gorm:"primaryKey;column:venue_id" json:"venue_id"`
	CreatedAt time.Time `gorm:"not null;column:created_at;default:CURRENT_TIMESTAMP" json:"created_at"`
}

// TableName specifies the table name for VenueConfirmation.
func (VenueConfirmation) TableName() string {
	return "venue_confirmations"
}
