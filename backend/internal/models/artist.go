package models

import "time"

type Artist struct {
	ID               uint      `gorm:"primaryKey"`
	Name             string    `gorm:"uniqueIndex"`
	Slug             *string   `gorm:"column:slug;uniqueIndex"`
	State            *string   `gorm:"column:state"`
	City             *string   `gorm:"column:city"`
	Country          *string   `gorm:"column:country;size:100"`
	BandcampEmbedURL *string   `gorm:"column:bandcamp_embed_url"`
	Description      *string   `json:"description,omitempty" gorm:"column:description;type:text"`
	Social           Social    `gorm:"embedded"`

	// Data provenance fields
	DataSource       *string    `json:"data_source,omitempty" gorm:"column:data_source;size:50"`
	SourceConfidence *float64   `json:"source_confidence,omitempty" gorm:"column:source_confidence;type:numeric(3,2)"`
	LastVerifiedAt   *time.Time `json:"last_verified_at,omitempty" gorm:"column:last_verified_at"`

	CreatedAt        time.Time `gorm:"not null"`
	UpdatedAt        time.Time `gorm:"not null"`

	// Relationships
	Shows   []Show        `gorm:"many2many:show_artists;"`
	Aliases []ArtistAlias `gorm:"foreignKey:ArtistID"`
}

func (Artist) TableName() string {
	return "artists"
}

// ArtistAlias represents an alternate name that resolves to a canonical artist.
type ArtistAlias struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	ArtistID  uint      `gorm:"not null" json:"artist_id"`
	Alias     string    `gorm:"not null;size:255" json:"alias"`
	CreatedAt time.Time `gorm:"not null" json:"created_at"`
}

func (ArtistAlias) TableName() string {
	return "artist_aliases"
}
