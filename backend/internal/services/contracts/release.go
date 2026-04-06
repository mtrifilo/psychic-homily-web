package contracts

import "time"

// ──────────────────────────────────────────────
// Release types
// ──────────────────────────────────────────────

// CreateReleaseRequest represents the data needed to create a new release
type CreateReleaseRequest struct {
	Title       string                      `json:"title" validate:"required"`
	ReleaseType string                      `json:"release_type"`
	ReleaseYear *int                        `json:"release_year"`
	ReleaseDate *string                     `json:"release_date"`
	CoverArtURL *string                     `json:"cover_art_url"`
	Description *string                     `json:"description"`
	Artists     []CreateReleaseArtistEntry  `json:"artists"`
	ExternalLinks []CreateReleaseLinkEntry  `json:"external_links"`
}

// CreateReleaseArtistEntry represents an artist-role pair for release creation
type CreateReleaseArtistEntry struct {
	ArtistID uint   `json:"artist_id"`
	Role     string `json:"role"`
}

// CreateReleaseLinkEntry represents an external link for release creation
type CreateReleaseLinkEntry struct {
	Platform string `json:"platform"`
	URL      string `json:"url"`
}

// UpdateReleaseRequest represents the data that can be updated on a release
type UpdateReleaseRequest struct {
	Title       *string `json:"title"`
	ReleaseType *string `json:"release_type"`
	ReleaseYear *int    `json:"release_year"`
	ReleaseDate *string `json:"release_date"`
	CoverArtURL *string `json:"cover_art_url"`
	Description *string `json:"description"`
}

// ReleaseDetailResponse represents the release data returned to clients
type ReleaseDetailResponse struct {
	ID            uint                         `json:"id"`
	Title         string                       `json:"title"`
	Slug          string                       `json:"slug"`
	ReleaseType   string                       `json:"release_type"`
	ReleaseYear   *int                         `json:"release_year"`
	ReleaseDate   *string                      `json:"release_date"`
	CoverArtURL   *string                      `json:"cover_art_url"`
	Description   *string                      `json:"description"`
	Artists       []ReleaseArtistResponse      `json:"artists"`
	Labels        []ReleaseLabelResponse       `json:"labels"`
	ExternalLinks []ReleaseExternalLinkResponse `json:"external_links"`
	CreatedAt     time.Time                    `json:"created_at"`
	UpdatedAt     time.Time                    `json:"updated_at"`
}

// ReleaseArtistResponse represents an artist on a release
type ReleaseArtistResponse struct {
	ID   uint   `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
	Role string `json:"role"`
}

// ReleaseLabelResponse represents a label associated with a release
type ReleaseLabelResponse struct {
	ID            uint    `json:"id"`
	Name          string  `json:"name"`
	Slug          string  `json:"slug"`
	CatalogNumber *string `json:"catalog_number,omitempty"`
}

// ReleaseExternalLinkResponse represents an external link for a release
type ReleaseExternalLinkResponse struct {
	ID       uint   `json:"id"`
	Platform string `json:"platform"`
	URL      string `json:"url"`
}

// ReleaseListResponse represents a release in list views
type ReleaseListResponse struct {
	ID          uint    `json:"id"`
	Title       string  `json:"title"`
	Slug        string  `json:"slug"`
	ReleaseType string  `json:"release_type"`
	ReleaseYear *int    `json:"release_year"`
	CoverArtURL *string `json:"cover_art_url"`
	ArtistCount int     `json:"artist_count"`
}

// ArtistReleaseListResponse extends ReleaseListResponse with the artist's role on that release
type ArtistReleaseListResponse struct {
	ReleaseListResponse
	Role string `json:"role"`
}
