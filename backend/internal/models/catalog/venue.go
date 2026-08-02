package catalog

import (
	"time"

	"psychic-homily-backend/internal/models/auth"
)

type Venue struct {
	ID      uint    `gorm:"primaryKey"`
	Name    string  `gorm:"not null"` // Unique with city via composite index
	Slug    *string `gorm:"column:slug;uniqueIndex"`
	Address *string
	City    string  `gorm:"not null"` // Required
	State   string  `gorm:"not null"` // Required
	Country *string `gorm:"column:country;size:100"`
	Zipcode *string
	// Capacity (PSY-1179): venue capacity captured during ingest. Nullable —
	// unknown for most rows. Not sensitive, so unlike Address/Zipcode it is not
	// redacted for unverified venues.
	Capacity *int `gorm:"column:capacity"`
	// AgePolicy (PSY-1682) is the venue's HOUSE DEFAULT age rule, free text
	// borrowing the vocabulary of shows.age_requirement ("all ages", "17+",
	// "21+"). A show's own age_requirement is the per-event OVERRIDE and always
	// wins where both are present; this field is what makes such an override
	// legible as one, and what a show with no override falls back to.
	//
	// Nullable and unknown for most rows. UNLIKE Capacity (admin/ingest only),
	// this IS on the contributor allowlist, so the value is attacker-influenced
	// free text: see VenueAllowedEditFields and the length/type gate in
	// handlers/shared.boundedTextFieldSpecs.
	//
	// Served unredacted for unverified venues, matching Description (the other
	// ungated contributor-writable free-text venue field) rather than Address.
	// That posture is a deliberate choice, not an assertion that prose is
	// inherently safe: see buildVenueResponse.
	AgePolicy *string `gorm:"column:age_policy;size:100"`
	// Geocoding (PSY-985): resolved offline from city/state/country at create/update.
	// Timezone is the IANA zone used to anchor show times to the venue's locale.
	// Nullable — a geocode miss falls back to the legacy state->tz map.
	Latitude  *float64 `gorm:"column:latitude;type:numeric(9,6)"`
	Longitude *float64 `gorm:"column:longitude;type:numeric(9,6)"`
	Timezone  *string  `gorm:"column:timezone"`
	// Metro is the US Census CBSA code the venue's (city, state, country) rolls up
	// to, set alongside the geocoding in applyGeocoding. DERIVED; NULL on a miss.
	// Internal grouping key, not exposed in the API. (PSY-1255 step B)
	Metro *string `json:"-" gorm:"column:metro;size:10"`
	// Street-level geocoding (PSY-1536): coordinates of Address resolved via
	// Nominatim (network, applyStreetGeocoding + the geocode-venue-addresses
	// backfill CLI). Distinct from the centroid Latitude/Longitude above, which
	// stay untouched. GeocodePrecision is rooftop|interpolated|city.
	// GeocodedAddress is the exact address key that was last geocoded — set on
	// a hit AND on a clean miss (miss memo: key present, coords NULL), so
	// writers skip re-querying an unchanged address. Street coords are served
	// ONLY for verified venues whose stored key still matches the current
	// address AND whose coords are present (streetGeocodeFresh, see
	// buildVenueResponse). All four fields are json:"-" like Metro: the gated
	// contracts.VenueDetailResponse is the ONLY sanctioned serialization — a
	// raw-model marshal must never leak an unverified venue's street position.
	StreetLatitude   *float64 `json:"-" gorm:"column:street_latitude;type:numeric(9,6)"`
	StreetLongitude  *float64 `json:"-" gorm:"column:street_longitude;type:numeric(9,6)"`
	GeocodePrecision *string  `json:"-" gorm:"column:geocode_precision;size:20"`
	GeocodedAddress  *string  `json:"-" gorm:"column:geocoded_address"`
	Description      *string  `json:"description,omitempty" gorm:"column:description;type:text"`
	ImageURL         *string  `json:"image_url,omitempty" gorm:"column:image_url"`
	Social           Social   `gorm:"embedded"`
	Verified         bool
	SubmittedBy      *uint `gorm:"column:submitted_by"` // User ID of the person who originally submitted this venue

	// Data provenance fields
	DataSource       *string    `json:"data_source,omitempty" gorm:"column:data_source;size:50"`
	SourceConfidence *float64   `json:"source_confidence,omitempty" gorm:"column:source_confidence;type:numeric(3,2)"`
	LastVerifiedAt   *time.Time `json:"last_verified_at,omitempty" gorm:"column:last_verified_at"`

	CreatedAt time.Time `gorm:"not null"`
	UpdatedAt time.Time `gorm:"not null"`

	// Relationships
	Shows           []Show     `gorm:"many2many:show_venues;"`
	SubmittedByUser *auth.User `gorm:"foreignKey:SubmittedBy"`
}

// TableName specifies the table name for Venue
func (Venue) TableName() string {
	return "venues"
}

// PublicAddress is THE street-address privacy gate: the way Address moves from
// a venue row into any payload a non-admin client can read.
//
// An unverified venue is frequently a DIY / house show at somebody's home, so
// its street address must not be published before a human has reviewed the row.
// Every response builder that embeds a venue has to apply that rule, and the
// rule was previously re-spelled as an inline `if venue.Verified` at each site.
// That is a drift shape rather than a policy: a builder written later (or in
// another package) simply omits the conditional and the leak is invisible in
// review, which is exactly how the saved-shows payload came to serve raw
// addresses. Naming the gate here gives new builders one obvious thing to call
// and makes the absence of a call the conspicuous part.
//
// Returns nil for an unverified venue, so callers assign the result straight
// into the nullable Address field of their response type.
//
// Note the zipcode and street-level coordinates carry the SAME gate but are not
// covered here: they are served by exactly one builder (buildVenueResponse) and
// the coordinates take an extra freshness condition, so they stay where their
// single caller can see the whole rule at once.
//
// Two readers deliberately bypass this and serve the raw column, so a caller
// found doing so is not automatically a bug: the admin moderation queue
// (VenueService.GetUnverifiedVenues), which exists to show a human the address
// they are being asked to verify, and the markdown export, whose route is
// registered only under ENVIRONMENT=development plus an admin bulk endpoint.
func (v *Venue) PublicAddress() *string {
	if v == nil || !v.Verified {
		return nil
	}
	return v.Address
}
