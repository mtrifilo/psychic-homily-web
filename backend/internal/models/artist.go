package models

import "time"

type Artist struct {
	ID               uint      `gorm:"primaryKey"`
	Name             string    `gorm:"uniqueIndex"`
	Slug             *string   `gorm:"column:slug;uniqueIndex"`
	State            *string   `gorm:"column:state"`
	City             *string   `gorm:"column:city"`
	BandcampEmbedURL *string   `gorm:"column:bandcamp_embed_url"`
	Social           Social    `gorm:"embedded"`

	// Data provenance fields
	DataSource       *string    `json:"data_source,omitempty" gorm:"column:data_source;size:50"`
	SourceConfidence *float64   `json:"source_confidence,omitempty" gorm:"column:source_confidence;type:numeric(3,2)"`
	LastVerifiedAt   *time.Time `json:"last_verified_at,omitempty" gorm:"column:last_verified_at"`

	CreatedAt        time.Time `gorm:"not null"`
	UpdatedAt        time.Time `gorm:"not null"`

	// Relationships
	Shows []Show `gorm:"many2many:show_artists;"`
}

func (Artist) TableName() string {
	return "artists"
}
