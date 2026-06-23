package catalog

import "time"

// StreamingDiscoveryStatus tracks where each artist sits in the admin
// worklist that walks artists missing music-platform links (spotify /
// bandcamp / youtube / soundcloud). Values are CHECK-constrained at the DB
// — keep this list in sync with the streaming_discovery_status column
// constraint.
type StreamingDiscoveryStatus string

const (
	StreamingDiscoveryStatusUnreviewed        StreamingDiscoveryStatus = "unreviewed"
	StreamingDiscoveryStatusCandidatesPending StreamingDiscoveryStatus = "candidates_pending"
	StreamingDiscoveryStatusLinked            StreamingDiscoveryStatus = "linked"
	StreamingDiscoveryStatusNoLinksFound      StreamingDiscoveryStatus = "no_links_found"
	StreamingDiscoveryStatusSkipped           StreamingDiscoveryStatus = "skipped"
)

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
	// Provider + deep linkback for the artist photo, for attribution (PSY-1175).
	// source ∈ spotify|discogs|cover_art_archive|user|commons|public_domain.
	ImageSource    *string `json:"image_source,omitempty" gorm:"column:image_source;size:32"`
	ImageSourceURL *string `json:"image_source_url,omitempty" gorm:"column:image_source_url"`
	Social         Social  `gorm:"embedded"`

	// Data provenance fields
	DataSource       *string    `json:"data_source,omitempty" gorm:"column:data_source;size:50"`
	SourceConfidence *float64   `json:"source_confidence,omitempty" gorm:"column:source_confidence;type:numeric(3,2)"`
	LastVerifiedAt   *time.Time `json:"last_verified_at,omitempty" gorm:"column:last_verified_at"`

	// Streaming-discovery review state — see StreamingDiscoveryStatus const block.
	// Reason holds the admin's optional note on no_links_found / skipped outcomes.
	StreamingDiscoveryStatus StreamingDiscoveryStatus `json:"streaming_discovery_status" gorm:"column:streaming_discovery_status;size:32;not null;default:unreviewed"`
	StreamingDiscoveryReason *string                  `json:"streaming_discovery_reason,omitempty" gorm:"column:streaming_discovery_reason;type:text"`

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
