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
