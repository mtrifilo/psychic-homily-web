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

// BandcampEmbedSource records HOW artists.bandcamp_embed_url was set, so a
// keep-fresh hook (PSY-1189) can safely refresh/clean up the auto-derived
// embeds without ever clobbering a human-curated value. The column is a plain
// VARCHAR(32) with NO CHECK constraint, so THIS const block — not the PSY-1188
// migration's comment (which predates profile_resolved and lists only the first
// two) — is the live source of truth for the legal values. The column is
// nullable; a NULL means legacy/unknown (set before the column existed).
const (
	// BandcampEmbedSourceReleaseDerived marks an embed auto-derived from one of
	// the artist's catalogued release Bandcamp links (the backfill stamps this).
	BandcampEmbedSourceReleaseDerived = "release_derived"
	// BandcampEmbedSourceManual marks an embed set by a human/admin/AI write
	// path (the direct admin endpoint, CreateArtist/UpdateArtist, the community
	// entity-request fulfiller).
	BandcampEmbedSourceManual = "manual"
	// BandcampEmbedSourceProfileResolved marks an embed that the profile→album
	// resolver (PSY-1190) derived by fetching a *.bandcamp.com profile root and
	// extracting its featured/latest /album|/track URL. Like release_derived it is
	// auto-derived (fill-when-empty; never overwrites a manual value).
	BandcampEmbedSourceProfileResolved = "profile_resolved"
)

type Artist struct {
	ID               uint    `gorm:"primaryKey"`
	Name             string  `gorm:"uniqueIndex"`
	Slug             *string `gorm:"column:slug;uniqueIndex"`
	State            *string `gorm:"column:state"`
	City             *string `gorm:"column:city"`
	Country          *string `gorm:"column:country;size:100"`
	BandcampEmbedURL *string `gorm:"column:bandcamp_embed_url"`
	// BandcampEmbedSource is the provenance of BandcampEmbedURL — one of the
	// BandcampEmbedSource* constants, or nil for legacy/unknown (PSY-1188).
	// Internal column: NOT mapped onto any API response.
	BandcampEmbedSource *string `json:"-" gorm:"column:bandcamp_embed_source;size:32"`
	Description         *string `json:"description,omitempty" gorm:"column:description;type:text"`
	ImageURL         *string `json:"image_url,omitempty" gorm:"column:image_url"`
	// Provider + deep linkback for the artist photo, for attribution (PSY-1175).
	// source ∈ spotify|discogs|cover_art_archive|user|commons|public_domain.
	ImageSource    *string `json:"image_source,omitempty" gorm:"column:image_source;size:32"`
	ImageSourceURL *string `json:"image_source_url,omitempty" gorm:"column:image_source_url"`
	// License + author for a Commons-sourced photo (PSY-1232). CC-BY / CC-BY-SA
	// require crediting the photographer + the specific license; nil for providers
	// whose attribution derives from ImageSource alone (Spotify, CAA, Discogs). A
	// public-domain Commons photo still uses ImageSource="commons" with
	// ImageLicense="Public domain" (not the public_domain source id).
	ImageLicense *string `json:"image_license,omitempty" gorm:"column:image_license;size:64"`
	ImageAuthor  *string `json:"image_author,omitempty" gorm:"column:image_author"`
	Social       Social  `gorm:"embedded"`

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
