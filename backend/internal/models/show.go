package models

import "time"

// ShowStatus represents the approval status of a show submission
type ShowStatus string

const (
	ShowStatusPending  ShowStatus = "pending"
	ShowStatusApproved ShowStatus = "approved"
	ShowStatusRejected ShowStatus = "rejected"
)

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

	// Approval workflow fields
	Status          ShowStatus `gorm:"type:show_status;not null;default:'approved'"`
	SubmittedBy     *uint      `gorm:"column:submitted_by"`
	RejectionReason *string    `gorm:"column:rejection_reason"`

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
