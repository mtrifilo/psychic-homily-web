package models

import "time"

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
	ID             uint    `gorm:"primaryKey"`
	Title          string
	Slug           *string `gorm:"column:slug;uniqueIndex"`
	EventDate      time.Time `gorm:"not null"`
	City           *string
	State          *string
	Price          *float64
	AgeRequirement *string
	Description    *string
	CreatedAt      time.Time `gorm:"not null"`
	UpdatedAt      time.Time `gorm:"not null"`

	// Approval workflow fields
	Status          ShowStatus `gorm:"type:show_status;not null;default:'approved'"`
	SubmittedBy     *uint      `gorm:"column:submitted_by"`
	RejectionReason   *string `gorm:"column:rejection_reason"`
	RejectionCategory *string `gorm:"column:rejection_category"`

	// Source tracking fields (for discovered shows)
	Source        ShowSource `gorm:"type:show_source;not null;default:'user'"`
	SourceVenue   *string    `gorm:"column:source_venue"`   // e.g., 'valley-bar', 'crescent-ballroom'
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

	// Status flags (admin-controlled)
	IsSoldOut   bool `gorm:"column:is_sold_out;not null;default:false"`
	IsCancelled bool `gorm:"column:is_cancelled;not null;default:false"`

	// Relationships
	Venues  []Venue  `gorm:"many2many:show_venues;"`
	Artists []Artist `gorm:"many2many:show_artists;"`
	// Submitter relationship (optional, for eager loading)
	Submitter *User `gorm:"foreignKey:SubmittedBy"`
}

// TableName specifies the table name for Show
func (Show) TableName() string {
	return "shows"
}

// ShowArtist represents the junction table with ordering information
type ShowArtist struct {
	ShowID   uint   `gorm:"primaryKey;column:show_id"`
	ArtistID uint   `gorm:"primaryKey;column:artist_id"`
	Position int    `gorm:"not null;default:0"`
	SetType  string `gorm:"default:performer"`
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
