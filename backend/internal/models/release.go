package models

import "time"

// ReleaseType represents the type/format of a release
type ReleaseType string

const (
	ReleaseTypeLP          ReleaseType = "lp"
	ReleaseTypeEP          ReleaseType = "ep"
	ReleaseTypeSingle      ReleaseType = "single"
	ReleaseTypeCompilation ReleaseType = "compilation"
	ReleaseTypeLive        ReleaseType = "live"
	ReleaseTypeRemix       ReleaseType = "remix"
	ReleaseTypeDemo        ReleaseType = "demo"
)

// ArtistReleaseRole represents the role an artist played on a release
type ArtistReleaseRole string

const (
	ArtistReleaseRoleMain     ArtistReleaseRole = "main"
	ArtistReleaseRoleFeatured ArtistReleaseRole = "featured"
	ArtistReleaseRoleProducer ArtistReleaseRole = "producer"
	ArtistReleaseRoleRemixer  ArtistReleaseRole = "remixer"
	ArtistReleaseRoleComposer ArtistReleaseRole = "composer"
	ArtistReleaseRoleDJ       ArtistReleaseRole = "dj"
)

// Release represents a music release (album, EP, single, etc.)
type Release struct {
	ID          uint        `gorm:"primaryKey"`
	Title       string      `gorm:"not null"`
	Slug        *string     `gorm:"column:slug;uniqueIndex"`
	ReleaseType ReleaseType `gorm:"column:release_type;not null;default:'lp'"`
	ReleaseYear *int        `gorm:"column:release_year"`
	ReleaseDate *string     `gorm:"column:release_date;type:date"` // DATE stored as string (YYYY-MM-DD)
	CoverArtURL *string     `gorm:"column:cover_art_url"`
	Description *string     `gorm:"column:description"`
	CreatedAt   time.Time   `gorm:"not null"`
	UpdatedAt   time.Time   `gorm:"not null"`

	// Relationships
	Artists       []Artist              `gorm:"many2many:artist_releases;"`
	ExternalLinks []ReleaseExternalLink `gorm:"foreignKey:ReleaseID"`
}

// TableName specifies the table name for Release
func (Release) TableName() string {
	return "releases"
}

// ArtistRelease represents the junction table between artists and releases with role information
type ArtistRelease struct {
	ArtistID  uint              `gorm:"primaryKey;column:artist_id"`
	ReleaseID uint              `gorm:"primaryKey;column:release_id"`
	Role      ArtistReleaseRole `gorm:"primaryKey;column:role;not null;default:'main'"`
	Position  int               `gorm:"not null;default:0"`
}

// TableName specifies the table name for ArtistRelease
func (ArtistRelease) TableName() string {
	return "artist_releases"
}

// ReleaseExternalLink represents a link to an external platform for a release
type ReleaseExternalLink struct {
	ID        uint      `gorm:"primaryKey"`
	ReleaseID uint      `gorm:"not null"`
	Platform  string    `gorm:"not null"`
	URL       string    `gorm:"not null"`
	CreatedAt time.Time `gorm:"not null"`
}

// TableName specifies the table name for ReleaseExternalLink
func (ReleaseExternalLink) TableName() string {
	return "release_external_links"
}
