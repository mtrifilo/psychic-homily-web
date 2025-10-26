package models

import (
	"time"
)

type Venue struct {
	ID        uint   `gorm:"primaryKey"`
	Name      string `gorm:"not null"` // Unique with city via composite index
	Address   *string
	City      string `gorm:"not null"` // Required
	State     string `gorm:"not null"` // Required
	Zipcode   *string
	Social    Social    `gorm:"embedded"`
	CreatedAt time.Time `gorm:"not null"`
	UpdatedAt time.Time `gorm:"not null"`

	// Relationships
	Shows []Show `gorm:"many2many:show_venues;"`
}

// TableName specifies the table name for Venue
func (Venue) TableName() string {
	return "venues"
}
