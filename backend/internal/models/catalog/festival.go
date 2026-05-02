package catalog

import (
	"encoding/json"
	"time"
)

// FestivalStatus represents the current status of a festival
type FestivalStatus string

const (
	FestivalStatusAnnounced FestivalStatus = "announced"
	FestivalStatusConfirmed FestivalStatus = "confirmed"
	FestivalStatusCancelled FestivalStatus = "cancelled"
	FestivalStatusCompleted FestivalStatus = "completed"
)

// BillingTier represents the billing level of an artist at a festival
type BillingTier string

const (
	BillingTierHeadliner    BillingTier = "headliner"
	BillingTierSubHeadliner BillingTier = "sub_headliner"
	BillingTierMidCard      BillingTier = "mid_card"
	BillingTierUndercard    BillingTier = "undercard"
	BillingTierLocal        BillingTier = "local"
	BillingTierDJ           BillingTier = "dj"
	BillingTierHost         BillingTier = "host"
)

// Festival represents a music festival (distinct from a show — multi-day, tiered billing, multi-venue)
type Festival struct {
	ID           uint             `gorm:"primaryKey"`
	Name         string           `gorm:"not null"`
	Slug         string           `gorm:"not null;uniqueIndex"`
	SeriesSlug   string           `gorm:"column:series_slug;not null"`
	EditionYear  int              `gorm:"column:edition_year;not null"`
	Description  *string          `gorm:"column:description"`
	LocationName *string          `gorm:"column:location_name"`
	City         *string          `gorm:"column:city"`
	State        *string          `gorm:"column:state"`
	Country      *string          `gorm:"column:country"`
	StartDate    string           `gorm:"column:start_date;type:date;not null"`
	EndDate      string           `gorm:"column:end_date;type:date;not null"`
	Website      *string          `gorm:"column:website"`
	TicketURL    *string          `gorm:"column:ticket_url"`
	FlyerURL     *string          `gorm:"column:flyer_url"`
	Status       FestivalStatus   `gorm:"column:status;not null;default:'announced'"`
	Social       *json.RawMessage `gorm:"column:social;type:jsonb;default:'{}'"`

	// Data provenance fields
	DataSource       *string    `json:"data_source,omitempty" gorm:"column:data_source;size:50"`
	SourceConfidence *float64   `json:"source_confidence,omitempty" gorm:"column:source_confidence;type:numeric(3,2)"`
	LastVerifiedAt   *time.Time `json:"last_verified_at,omitempty" gorm:"column:last_verified_at"`

	CreatedAt time.Time `gorm:"not null"`
	UpdatedAt time.Time `gorm:"not null"`

	// Relationships
	Artists []FestivalArtist `gorm:"foreignKey:FestivalID"`
	Venues  []FestivalVenue  `gorm:"foreignKey:FestivalID"`
}

// TableName specifies the table name for Festival
func (Festival) TableName() string {
	return "festivals"
}

// FestivalArtist represents the junction table between festivals and artists
type FestivalArtist struct {
	ID          uint        `gorm:"primaryKey"`
	FestivalID  uint        `gorm:"column:festival_id;not null"`
	ArtistID    uint        `gorm:"column:artist_id;not null"`
	BillingTier BillingTier `gorm:"column:billing_tier;not null;default:'mid_card'"`
	Position    int         `gorm:"not null;default:0"`
	DayDate     *string     `gorm:"column:day_date;type:date"`
	Stage       *string     `gorm:"column:stage"`
	SetTime     *string     `gorm:"column:set_time;type:time"`
	VenueID     *uint       `gorm:"column:venue_id"`
	CreatedAt   time.Time   `gorm:"not null"`

	// Relationships
	Festival Festival `gorm:"foreignKey:FestivalID"`
	Artist   Artist   `gorm:"foreignKey:ArtistID"`
	Venue    *Venue   `gorm:"foreignKey:VenueID"`
}

// TableName specifies the table name for FestivalArtist
func (FestivalArtist) TableName() string {
	return "festival_artists"
}

// FestivalVenue represents the junction table between festivals and venues
type FestivalVenue struct {
	ID         uint      `gorm:"primaryKey"`
	FestivalID uint      `gorm:"column:festival_id;not null"`
	VenueID    uint      `gorm:"column:venue_id;not null"`
	IsPrimary  bool      `gorm:"column:is_primary;not null;default:false"`
	CreatedAt  time.Time `gorm:"not null"`

	// Relationships
	Festival Festival `gorm:"foreignKey:FestivalID"`
	Venue    Venue    `gorm:"foreignKey:VenueID"`
}

// TableName specifies the table name for FestivalVenue
func (FestivalVenue) TableName() string {
	return "festival_venues"
}
