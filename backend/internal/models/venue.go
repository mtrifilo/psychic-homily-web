package models

import (
	"time"
)

type Venue struct {
	ID          uint    `gorm:"primaryKey"`
	Name        string  `gorm:"not null"` // Unique with city via composite index
	Slug        *string `gorm:"column:slug;uniqueIndex"`
	Address     *string
	City        string  `gorm:"not null"` // Required
	State       string  `gorm:"not null"` // Required
	Country     *string `gorm:"column:country;size:100"`
	Zipcode     *string
	Description *string `json:"description,omitempty" gorm:"column:description;type:text"`
	Social      Social  `gorm:"embedded"`
	Verified    bool
	SubmittedBy *uint     `gorm:"column:submitted_by"` // User ID of the person who originally submitted this venue

	// Data provenance fields
	DataSource       *string    `json:"data_source,omitempty" gorm:"column:data_source;size:50"`
	SourceConfidence *float64   `json:"source_confidence,omitempty" gorm:"column:source_confidence;type:numeric(3,2)"`
	LastVerifiedAt   *time.Time `json:"last_verified_at,omitempty" gorm:"column:last_verified_at"`

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

