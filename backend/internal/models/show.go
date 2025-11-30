package models

import "time"

type Show struct {
	ID             uint `gorm:"primaryKey"`
	Title          string
	EventDate      time.Time `gorm:"not null"`
	City           *string
	State          *string
	Price          *float64
	AgeRequirement *string
	Description    *string
	CreatedAt      time.Time `gorm:"not null"`
	UpdatedAt      time.Time `gorm:"not null"`

	// Relationships
	Venues  []Venue  `gorm:"many2many:show_venues;"`
	Artists []Artist `gorm:"many2many:show_artists;"`
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
