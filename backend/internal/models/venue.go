package models

import (
	"time"
)

type Venue struct {
	ID          uint    `gorm:"primaryKey"`
	Name        string  `gorm:"not null"` // Unique with city via composite index
	Slug        *string `gorm:"column:slug;uniqueIndex"`
	Address     *string
	City        string `gorm:"not null"` // Required
	State       string `gorm:"not null"` // Required
	Zipcode     *string
	Social      Social `gorm:"embedded"`
	Verified    bool
	SubmittedBy *uint     `gorm:"column:submitted_by"` // User ID of the person who originally submitted this venue
	CreatedAt   time.Time `gorm:"not null"`
	UpdatedAt   time.Time `gorm:"not null"`

	// Relationships
	Shows       []Show `gorm:"many2many:show_venues;"`
	SubmittedByUser *User `gorm:"foreignKey:SubmittedBy"`
}

// TableName specifies the table name for Venue
func (Venue) TableName() string {
	return "venues"
}

// VenueEditStatus represents the status of a pending venue edit
type VenueEditStatus string

const (
	VenueEditStatusPending  VenueEditStatus = "pending"
	VenueEditStatusApproved VenueEditStatus = "approved"
	VenueEditStatusRejected VenueEditStatus = "rejected"
)

// PendingVenueEdit represents a proposed edit to a venue awaiting admin approval
type PendingVenueEdit struct {
	ID          uint            `gorm:"primaryKey"`
	VenueID     uint            `gorm:"not null"`
	SubmittedBy uint            `gorm:"not null"`

	// Proposed changes (nil = no change to that field)
	Name       *string `gorm:"column:name"`
	Address    *string `gorm:"column:address"`
	City       *string `gorm:"column:city"`
	State      *string `gorm:"column:state"`
	Zipcode    *string `gorm:"column:zipcode"`
	Instagram  *string `gorm:"column:instagram"`
	Facebook   *string `gorm:"column:facebook"`
	Twitter    *string `gorm:"column:twitter"`
	YouTube    *string `gorm:"column:youtube"`
	Spotify    *string `gorm:"column:spotify"`
	SoundCloud *string `gorm:"column:soundcloud"`
	Bandcamp   *string `gorm:"column:bandcamp"`
	Website    *string `gorm:"column:website"`

	// Workflow fields
	Status          VenueEditStatus `gorm:"type:venue_edit_status;not null;default:'pending'"`
	RejectionReason *string         `gorm:"column:rejection_reason"`
	ReviewedBy      *uint           `gorm:"column:reviewed_by"`
	ReviewedAt      *time.Time      `gorm:"column:reviewed_at"`

	CreatedAt time.Time `gorm:"not null"`
	UpdatedAt time.Time `gorm:"not null"`

	// Relationships
	Venue           Venue `gorm:"foreignKey:VenueID"`
	SubmittedByUser User  `gorm:"foreignKey:SubmittedBy"`
	ReviewedByUser  *User `gorm:"foreignKey:ReviewedBy"`
}

// TableName specifies the table name for PendingVenueEdit
func (PendingVenueEdit) TableName() string {
	return "pending_venue_edits"
}
