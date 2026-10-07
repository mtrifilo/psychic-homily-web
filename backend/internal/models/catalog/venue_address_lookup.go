package catalog

import "time"

// Venue address backfill phases and outcomes stored in venue_address_lookups.
const (
	// VenueAddressPhasePage: the address was sought on the venue's own pages
	// (website, registered ingest source, an upcoming show's ticket page).
	VenueAddressPhasePage = "page"
	// VenueAddressPhaseName: the address was sought by searching the venue's
	// name in its city on OpenStreetMap.
	VenueAddressPhaseName = "name"

	VenueAddressOutcomeHit  = "hit"
	VenueAddressOutcomeMiss = "miss"
)

// VenueAddressLookup is the last recorded backfill outcome for one venue in
// one phase. LookupKey is what was tried; a miss (including a hit a reviewer
// refused) is skipped by later lookups only while the venue's current inputs
// still produce the same key and for 90 days from AttemptedAt, which for a
// miss is when the lookup ran and for a hit is when it was written. On a hit, Source is the
// page URL the address came from (page phase) or the matched OSM place's
// display name (name phase), and Address is what was written.
type VenueAddressLookup struct {
	VenueID     uint      `gorm:"column:venue_id;primaryKey;autoIncrement:false"`
	Phase       string    `gorm:"column:phase;primaryKey"`
	LookupKey   string    `gorm:"column:lookup_key;not null"`
	Outcome     string    `gorm:"column:outcome;not null"`
	Source      *string   `gorm:"column:source"`
	Address     *string   `gorm:"column:address"`
	AttemptedAt time.Time `gorm:"column:attempted_at;not null"`
}

// TableName specifies the table name for VenueAddressLookup.
func (VenueAddressLookup) TableName() string {
	return "venue_address_lookups"
}
