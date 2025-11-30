package models

import "time"

type Artist struct {
	ID        uint      `gorm:"primaryKey"`
	Name      string    `gorm:"uniqueIndex"`
	State     *string   `gorm:"column:state"`
	City      *string   `gorm:"column:city"`
	Social    Social    `gorm:"embedded"`
	CreatedAt time.Time `gorm:"not null"`
	UpdatedAt time.Time `gorm:"not null"`

	// Relationships
	Shows []Show `gorm:"many2many:show_artists;"`
}

func (Artist) TableName() string {
	return "artists"
}
