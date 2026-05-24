package catalog

import "time"

type Artist struct {
	ID               uint    `gorm:"primaryKey"`
	Name             string  `gorm:"uniqueIndex"`
	Slug             *string `gorm:"column:slug;uniqueIndex"`
	State            *string `gorm:"column:state"`
	City             *string `gorm:"column:city"`
	Country          *string `gorm:"column:country;size:100"`
	BandcampEmbedURL *string `gorm:"column:bandcamp_embed_url"`
	Description      *string `json:"description,omitempty" gorm:"column:description;type:text"`
	ImageURL         *string `json:"image_url,omitempty" gorm:"column:image_url"`
	Social           Social  `gorm:"embedded"`

	// Data provenance fields
	DataSource       *string    `json:"data_source,omitempty" gorm:"column:data_source;size:50"`
	SourceConfidence *float64   `json:"source_confidence,omitempty" gorm:"column:source_confidence;type:numeric(3,2)"`
	LastVerifiedAt   *time.Time `json:"last_verified_at,omitempty" gorm:"column:last_verified_at"`

	// Streaming discovery review state. Drives the admin worklist that walks
	// artists missing music-platform links (spotify/bandcamp/youtube/soundcloud)
	// and records the outcome of each review. CHECK-constrained at the DB.
	StreamingDiscoveryStatus string  `json:"streaming_discovery_status" gorm:"column:streaming_discovery_status;size:32;not null;default:unreviewed"`
	StreamingDiscoveryReason *string `json:"streaming_discovery_reason,omitempty" gorm:"column:streaming_discovery_reason;type:text"`

	CreatedAt time.Time `gorm:"not null"`
	UpdatedAt time.Time `gorm:"not null"`

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
