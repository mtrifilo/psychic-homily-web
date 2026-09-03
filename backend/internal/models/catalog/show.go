package catalog

import (
	"time"

	"psychic-homily-backend/internal/models/auth"
)

// ShowStatus represents the approval status of a show submission
type ShowStatus string

const (
	ShowStatusPending  ShowStatus = "pending"
	ShowStatusApproved ShowStatus = "approved"
	ShowStatusRejected ShowStatus = "rejected"
	ShowStatusPrivate  ShowStatus = "private"
)

// ShowSource represents where a show came from
type ShowSource string

const (
	ShowSourceUser      ShowSource = "user"      // Manually submitted by a user
	ShowSourceDiscovery ShowSource = "discovery" // Automatically imported from the discovery app
)

// DataSource constants for provenance tracking across all entities
const (
	DataSourceUser          = "user"
	DataSourceAIExtraction  = "ai_extraction"
	DataSourceMusicBrainz   = "musicbrainz"
	DataSourceBandcamp      = "bandcamp"
	DataSourceFestivalData  = "festival_data"
	DataSourceDiscovery     = "discovery"
	DataSourceCommunity     = "community"
	DataSourceAPIEnrichment = "api_enrichment"
)

type Show struct {
	ID        uint `gorm:"primaryKey"`
	Title     string
	Slug      *string   `gorm:"column:slug;uniqueIndex"`
	EventDate time.Time `gorm:"not null"`
	// DoorsAt and MusicAt are optional display times, not a second source of
	// truth for when the show is: EventDate remains the canonical instant that
	// sorting, dedup, slugs, and structured data all read. Nil means unknown,
	// which is the common case. Pointers keep the GORM zero-value trap out of
	// the picture.
	DoorsAt *time.Time `gorm:"column:doors_at"`
	MusicAt *time.Time `gorm:"column:music_at"`
	City    *string
	State   *string
	// Price is the show's price, and the ADVANCE price on the rows that also
	// record a DoorPrice. DoorPrice is the price at the door (PSY-1864).
	//
	// Nil means "not known" on either; zero means free. Both are pointers so
	// that distinction survives — a free show is a real fact the ticket line
	// prints as "Free", and a float64 zero-value could not tell it from
	// silence. Neither is inferred from the other: a show with an advance
	// price says nothing about what the door costs.
	//
	// EVERY WEB, EMAIL AND CALENDAR SURFACE renders the split (PSY-1962). Two
	// registers, one rule: the show detail page qualifies the pair as
	// `$35 ADV · DOOR $40`, and everything denser — /shows cards and compact
	// rows, the venue and artist show tables, the scene day and calendar lists,
	// the atlas venue panel, the discovery rails, the submissions console, the
	// admin pending queue, the ICS feed descriptions and the three alert
	// emails — spells it `$35/$40`, advance first. A lone price renders bare in
	// both, and equal numbers collapse to one.
	//
	// A NEW SURFACE MUST USE THE SHARED DERIVATION, not read Price alone.
	// Frontend: lib/utils/showPrice.ts (or the ShowPrice component, which also
	// carries the accessible reading of the pair). Backend:
	// internal/services/shared.ShowPriceText — the SERVICES shared package, not
	// api/handlers/shared. Reading the advance half by itself is not a smaller
	// version of the truth, it is a wrong number about money: a reader who
	// budgets $35 for a $40 door was misinformed by the site, not merely
	// under-informed.
	//
	// THREE consumers answer a different question and are NOT list registers.
	// Each has to reduce the pair to ONE number, and each falls back to the door
	// price only when no advance price is recorded: the notification price-cap
	// filter (effectiveShowPriceCents), the schema.org Offer (offerShowPrice on
	// the frontend), and the data-quality "missing price" report, which asks
	// whether the site knows the cost AT ALL and so accepts either column.
	//
	// STILL UNSWEPT on non-web clients: the dev-seed exemplars never produce a
	// split price, and a discovery re-scrape discards an extracted door price.
	// The `ph` CLI and the iOS client do carry the pair.
	Price          *float64
	DoorPrice      *float64 `gorm:"column:door_price"`
	AgeRequirement *string
	Description    *string
	CreatedAt      time.Time `gorm:"not null"`
	UpdatedAt      time.Time `gorm:"not null"`

	// Approval workflow fields
	Status            ShowStatus `gorm:"type:show_status;not null;default:'approved'"`
	SubmittedBy       *uint      `gorm:"column:submitted_by"`
	RejectionReason   *string    `gorm:"column:rejection_reason"`
	RejectionCategory *string    `gorm:"column:rejection_category"`

	// Source tracking fields (for discovered shows)
	Source        ShowSource `gorm:"type:show_source;not null;default:'user'"`
	SourceVenue   *string    `gorm:"column:source_venue"`    // e.g., 'valley-bar', 'crescent-ballroom'
	SourceEventID *string    `gorm:"column:source_event_id"` // External event ID for deduplication
	ScrapedAt     *time.Time `gorm:"column:scraped_at"`      // When the event was scraped

	// Data provenance fields (generalized across all entities)
	DataSource       *string    `json:"data_source,omitempty" gorm:"column:data_source;size:50"`
	SourceConfidence *float64   `json:"source_confidence,omitempty" gorm:"column:source_confidence;type:numeric(3,2)"`
	LastVerifiedAt   *time.Time `json:"last_verified_at,omitempty" gorm:"column:last_verified_at"`

	// Duplicate detection (for discovery imports flagged as potential duplicates)
	DuplicateOfShowID *uint `gorm:"column:duplicate_of_show_id"`

	// Ticket URL (optional)
	TicketURL *string `json:"ticket_url,omitempty" gorm:"type:varchar(500)"`

	// Image URL (optional) — show flyer when distinct from associated
	// release/festival imagery. PSY-521.
	ImageURL *string `json:"image_url,omitempty" gorm:"column:image_url"`

	// Status flags (admin-controlled)
	IsSoldOut   bool `gorm:"column:is_sold_out;not null;default:false"`
	IsCancelled bool `gorm:"column:is_cancelled;not null;default:false"`

	// Relationships
	Venues  []Venue  `gorm:"many2many:show_venues;"`
	Artists []Artist `gorm:"many2many:show_artists;"`
	// Submitter relationship (optional, for eager loading)
	Submitter *auth.User `gorm:"foreignKey:SubmittedBy"`
}

// TableName specifies the table name for Show
func (Show) TableName() string {
	return "shows"
}

// ShowArtist represents the junction table with ordering information.
//
// EventDate + VenueID are denormalized from the parent show + show_venues
// rows, and are NOT what enforces the show dedup key. This row can hold one
// venue id, so for a show billed at two venues the stamping picks the lowest
// and the index over these columns covers that venue alone. The key is
// enforced by the show_dedup_keys table, which holds one row per (show,
// artist, venue) and is derived by trigger; these columns are the narrower
// statement of the same rule, kept until their retirement can be sequenced
// against a deploy. Both are nullable, and the partial index excludes NULL,
// so an unpopulated row inserts uncovered. ShowService.CreateShow and
// UpdateShow populate and cascade-update them.
type ShowArtist struct {
	ShowID    uint       `gorm:"primaryKey;column:show_id"`
	ArtistID  uint       `gorm:"primaryKey;column:artist_id"`
	Position  int        `gorm:"not null;default:0"`
	SetType   string     `gorm:"default:performer"`
	EventDate *time.Time `gorm:"column:event_date"`
	VenueID   *uint      `gorm:"column:venue_id"`
}

// TableName specifies the table name for ShowArtist
func (ShowArtist) TableName() string {
	return "show_artists"
}

// ShowVenue represents the junction table for shows and venues
type ShowVenue struct {
	ShowID  uint `gorm:"primaryKey;column:show_id"`
	VenueID uint `gorm:"primaryKey;column:venue_id"`
}

// TableName specifies the table name for ShowVenue
func (ShowVenue) TableName() string {
	return "show_venues"
}
