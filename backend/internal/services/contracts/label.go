package contracts

import "time"

// ──────────────────────────────────────────────
// Label types
// ──────────────────────────────────────────────

// CreateLabelRequest represents the data needed to create a new label
type CreateLabelRequest struct {
	Name        string  `json:"name" validate:"required"`
	City        *string `json:"city"`
	State       *string `json:"state"`
	Country     *string `json:"country"`
	FoundedYear *int    `json:"founded_year"`
	Status      string  `json:"status"`
	Description *string `json:"description"`
	Instagram   *string `json:"instagram"`
	Facebook    *string `json:"facebook"`
	Twitter     *string `json:"twitter"`
	YouTube     *string `json:"youtube"`
	Spotify     *string `json:"spotify"`
	SoundCloud  *string `json:"soundcloud"`
	Bandcamp    *string `json:"bandcamp"`
	Website     *string `json:"website"`
	ImageURL    *string `json:"image_url"`
}

// UpdateLabelRequest represents the data that can be updated on a label
type UpdateLabelRequest struct {
	Name        *string `json:"name"`
	City        *string `json:"city"`
	State       *string `json:"state"`
	Country     *string `json:"country"`
	FoundedYear *int    `json:"founded_year"`
	Status      *string `json:"status"`
	Description *string `json:"description"`
	ImageURL    *string `json:"image_url"`
	Instagram   *string `json:"instagram"`
	Facebook    *string `json:"facebook"`
	Twitter     *string `json:"twitter"`
	YouTube     *string `json:"youtube"`
	Spotify     *string `json:"spotify"`
	SoundCloud  *string `json:"soundcloud"`
	Bandcamp    *string `json:"bandcamp"`
	Website     *string `json:"website"`
}

// LabelDetailResponse represents the label data returned to clients
type LabelDetailResponse struct {
	ID           uint           `json:"id"`
	Name         string         `json:"name"`
	Slug         string         `json:"slug"`
	City         *string        `json:"city"`
	State        *string        `json:"state"`
	Country      *string        `json:"country"`
	FoundedYear  *int           `json:"founded_year"`
	Status       string         `json:"status"`
	Description  *string        `json:"description"`
	ImageURL     *string        `json:"image_url"` // Optional label logo (PSY-521)
	Social       SocialResponse `json:"social"`
	ArtistCount  int            `json:"artist_count"`
	ReleaseCount int            `json:"release_count"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

// LabelListResponse represents a label in list views.
//
// Carries the dedup-relevant identity fields so the `ph` CLI can compare them
// against proposed values and avoid spurious UPDATEs on re-ingest (PSY-1157).
// PSY-1179 widened this to the FULL social set + founded_year so label-social
// enrichment round-trips on re-ingest (previously only website/bandcamp were
// carried, so the other socials + founded_year read empty and forced a spurious
// update every run). The primary consumer of these flat list socials is the
// `ph` CLI dedup (searchLabels). The social fields live under the model's
// embedded Social struct; they're flattened here to match this response's
// existing flat convention (PSY-1157) rather than nesting a `social` object like
// LabelDetailResponse — an additive, non-breaking change for existing consumers.
type LabelListResponse struct {
	ID           uint    `json:"id"`
	Name         string  `json:"name"`
	Slug         string  `json:"slug"`
	City         *string `json:"city"`
	State        *string `json:"state"`
	Country      *string `json:"country"`
	FoundedYear  *int    `json:"founded_year"`
	Website      *string `json:"website"`
	Bandcamp     *string `json:"bandcamp"`
	Instagram    *string `json:"instagram"`
	Facebook     *string `json:"facebook"`
	Twitter      *string `json:"twitter"`
	YouTube      *string `json:"youtube"`
	Spotify      *string `json:"spotify"`
	SoundCloud   *string `json:"soundcloud"`
	Description  *string `json:"description"`
	Status       string  `json:"status"`
	ArtistCount  int     `json:"artist_count"`
	ReleaseCount int     `json:"release_count"`
}

// LabelArtistResponse represents an artist on a label
type LabelArtistResponse struct {
	ID   uint   `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
}

// LabelReleaseResponse represents a release on a label
type LabelReleaseResponse struct {
	ID            uint    `json:"id"`
	Title         string  `json:"title"`
	Slug          string  `json:"slug"`
	ReleaseType   string  `json:"release_type"`
	ReleaseYear   *int    `json:"release_year"`
	CoverArtURL   *string `json:"cover_art_url"`
	CatalogNumber *string `json:"catalog_number"`
}

// ──────────────────────────────────────────────
// Label Service Interface
// ──────────────────────────────────────────────

// LabelServiceInterface defines the contract for label operations.
type LabelServiceInterface interface {
	CreateLabel(req *CreateLabelRequest) (*LabelDetailResponse, error)
	GetLabel(labelID uint) (*LabelDetailResponse, error)
	GetLabelBySlug(slug string) (*LabelDetailResponse, error)
	ListLabels(filters map[string]interface{}) ([]*LabelListResponse, error)
	SearchLabels(query string) ([]*LabelListResponse, error)
	UpdateLabel(labelID uint, req *UpdateLabelRequest) (*LabelDetailResponse, error)
	DeleteLabel(labelID uint) error
	GetLabelRoster(labelID uint) ([]*LabelArtistResponse, error)
	GetLabelCatalog(labelID uint) ([]*LabelReleaseResponse, error)
	AddArtistToLabel(labelID, artistID uint) error
	AddReleaseToLabel(labelID, releaseID uint, catalogNumber *string, overwriteCatalogNumber bool) error
}
