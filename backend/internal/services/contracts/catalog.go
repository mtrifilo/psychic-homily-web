// Package contracts defines the interfaces and shared types for services.
// Services must NEVER be imported by this package — only models and third-party libs.
package contracts

import (
	"time"

	"psychic-homily-backend/internal/models"
)

// ──────────────────────────────────────────────
// Show types
// ──────────────────────────────────────────────

// CreateShowVenue represents a venue in a show creation request.
type CreateShowVenue struct {
	ID      *uint  `json:"id"`
	Name    string `json:"name"`
	City    string `json:"city"`
	State   string `json:"state"`
	Address string `json:"address,omitempty"`
}

// CreateShowArtist represents an artist in a show creation request.
// IsHeadliner is used for duplicate prevention (headliners can't perform at same venue on same date).
type CreateShowArtist struct {
	ID              *uint   `json:"id"`
	Name            string  `json:"name"`
	IsHeadliner     *bool   `json:"is_headliner"`
	InstagramHandle *string `json:"instagram_handle,omitempty"`
}

// CreateShowRequest represents the data needed to create a new show.
// The service will prevent duplicate headliners at the same venue on the same date/time
// and reuse existing venues by name and city (venues are unique by name within a city).
type CreateShowRequest struct {
	Title          string             `json:"title" validate:"required"`
	EventDate      time.Time          `json:"event_date" validate:"required"`
	City           string             `json:"city"`
	State          string             `json:"state"`
	Price          *float64           `json:"price"`
	AgeRequirement string             `json:"age_requirement"`
	Description    string             `json:"description"`
	Venues         []CreateShowVenue  `json:"venues" validate:"required,min=1"`
	Artists        []CreateShowArtist `json:"artists" validate:"required,min=1"`

	// User context for determining show status
	SubmittedByUserID *uint `json:"-"` // User ID of submitter (set by handler)
	SubmitterIsAdmin  bool  `json:"-"` // Whether submitter is admin (set by handler)
	IsPrivate         bool  `json:"-"` // Whether show should be private (user's list only)
}

// ShowResponse represents the show data returned to clients
type ShowResponse struct {
	ID              uint             `json:"id"`
	Slug            string           `json:"slug"`
	Title           string           `json:"title"`
	EventDate       time.Time        `json:"event_date"`
	City            *string          `json:"city"`
	State           *string          `json:"state"`
	Price           *float64         `json:"price"`
	AgeRequirement  *string          `json:"age_requirement"`
	Description     *string          `json:"description"`
	Status          string           `json:"status"`
	SubmittedBy     *uint            `json:"submitted_by,omitempty"`
	RejectionReason   *string          `json:"rejection_reason,omitempty"`
	RejectionCategory *string          `json:"rejection_category,omitempty"`
	Venues            []VenueResponse  `json:"venues"`
	Artists         []ArtistResponse `json:"artists"`
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`

	// Status flags (admin-controlled)
	IsSoldOut   bool `json:"is_sold_out"`
	IsCancelled bool `json:"is_cancelled"`

	// Source tracking (for admin view to identify discovered shows)
	Source      string     `json:"source,omitempty"`       // "user" or "discovery"
	SourceVenue *string    `json:"source_venue,omitempty"` // Venue slug for scraped shows
	ScrapedAt   *time.Time `json:"scraped_at,omitempty"`   // When the show was scraped

	// Duplicate detection context
	DuplicateOfShowID *uint `json:"duplicate_of_show_id,omitempty"` // ID of show this may duplicate
}

// VenueResponse represents venue data in show responses
type VenueResponse struct {
	ID         uint    `json:"id"`
	Slug       string  `json:"slug"`
	Name       string  `json:"name"`
	Address    *string `json:"address"`
	City       string  `json:"city"`
	State      string  `json:"state"`
	Verified   bool    `json:"verified"`    // Admin-verified as legitimate venue
	IsNewVenue *bool   `json:"is_new_venue"` // True if venue was created during this show submission
}

// ShowArtistSocials represents social media links for artists in show responses
type ShowArtistSocials struct {
	Instagram  *string `json:"instagram"`
	Facebook   *string `json:"facebook"`
	Twitter    *string `json:"twitter"`
	YouTube    *string `json:"youtube"`
	Spotify    *string `json:"spotify"`
	SoundCloud *string `json:"soundcloud"`
	Bandcamp   *string `json:"bandcamp"`
	Website    *string `json:"website"`
}

// ArtistResponse represents artist data in show responses
type ArtistResponse struct {
	ID               uint              `json:"id"`
	Slug             string            `json:"slug"`
	Name             string            `json:"name"`
	State            *string           `json:"state"`
	City             *string           `json:"city"`
	IsHeadliner      *bool             `json:"is_headliner"`
	SetType          string            `json:"set_type"`
	Position         int               `json:"position"`
	IsNewArtist      *bool             `json:"is_new_artist"`
	BandcampEmbedURL *string           `json:"bandcamp_embed_url"`
	Socials          ShowArtistSocials `json:"socials"`
}

// BatchShowResult contains the outcome of a batch approve/reject operation.
type BatchShowResult struct {
	Succeeded []uint           `json:"succeeded"`
	Errors    []BatchShowError `json:"errors"`
}

// BatchShowError describes a failure for a single show in a batch operation.
type BatchShowError struct {
	ShowID uint   `json:"show_id"`
	Error  string `json:"error"`
}

// PendingShowsFilter contains optional filters for pending shows queries.
type PendingShowsFilter struct {
	VenueID *uint
	Source  *string // "discovery" or "user"
}

// CityStateFilter represents a city+state pair for multi-city filtering.
type CityStateFilter struct {
	City  string
	State string
}

// UpcomingShowsFilter contains optional filters for GetUpcomingShows
type UpcomingShowsFilter struct {
	City   string
	State  string
	Cities []CityStateFilter
}

// ShowCityResponse represents a city with the count of upcoming shows
type ShowCityResponse struct {
	City      string `json:"city"`
	State     string `json:"state"`
	ShowCount int    `json:"show_count"`
}

// OrphanedArtist represents an artist with no remaining show associations.
type OrphanedArtist struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// AdminShowFilters contains filter criteria for admin show queries.
type AdminShowFilters struct {
	Status   string // pending, approved, rejected, private
	FromDate string // RFC3339 format
	ToDate   string // RFC3339 format
	City     string
}

// ParsedShowImport contains the parsed result of a markdown show import.
type ParsedShowImport struct {
	Frontmatter ExportFrontmatter
	Description string
}

// VenueMatchResult represents the result of matching a venue
type VenueMatchResult struct {
	Name       string `json:"name"`
	City       string `json:"city"`
	State      string `json:"state"`
	ExistingID *uint  `json:"existing_id,omitempty"`
	WillCreate bool   `json:"will_create"`
}

// ArtistMatchResult represents the result of matching an artist
type ArtistMatchResult struct {
	Name       string `json:"name"`
	Position   int    `json:"position"`
	SetType    string `json:"set_type"`
	ExistingID *uint  `json:"existing_id,omitempty"`
	WillCreate bool   `json:"will_create"`
}

// ImportPreviewResponse represents the preview response for show import
type ImportPreviewResponse struct {
	Show      ExportShowData      `json:"show"`
	Venues    []VenueMatchResult  `json:"venues"`
	Artists   []ArtistMatchResult `json:"artists"`
	Warnings  []string            `json:"warnings"`
	CanImport bool                `json:"can_import"`
}

// ExportShowData represents show data in export frontmatter
type ExportShowData struct {
	Title          string   `yaml:"title" json:"title"`
	EventDate      string   `yaml:"event_date" json:"event_date"`
	City           string   `yaml:"city,omitempty" json:"city,omitempty"`
	State          string   `yaml:"state,omitempty" json:"state,omitempty"`
	Price          *float64 `yaml:"price,omitempty" json:"price,omitempty"`
	AgeRequirement string   `yaml:"age_requirement,omitempty" json:"age_requirement,omitempty"`
	Status         string   `yaml:"status" json:"status"`
}

// ExportVenueSocial represents venue social links in export
type ExportVenueSocial struct {
	Instagram  string `yaml:"instagram,omitempty"`
	Facebook   string `yaml:"facebook,omitempty"`
	Twitter    string `yaml:"twitter,omitempty"`
	YouTube    string `yaml:"youtube,omitempty"`
	Spotify    string `yaml:"spotify,omitempty"`
	SoundCloud string `yaml:"soundcloud,omitempty"`
	Bandcamp   string `yaml:"bandcamp,omitempty"`
	Website    string `yaml:"website,omitempty"`
}

// ExportVenueData represents a venue in the markdown frontmatter
type ExportVenueData struct {
	Name    string            `yaml:"name"`
	City    string            `yaml:"city"`
	State   string            `yaml:"state"`
	Address string            `yaml:"address,omitempty"`
	Zipcode string            `yaml:"zipcode,omitempty"`
	Social  ExportVenueSocial `yaml:"social,omitempty"`
}

// ExportArtistSocial represents artist social links in export
type ExportArtistSocial struct {
	Instagram  string `yaml:"instagram,omitempty"`
	Facebook   string `yaml:"facebook,omitempty"`
	Twitter    string `yaml:"twitter,omitempty"`
	YouTube    string `yaml:"youtube,omitempty"`
	Spotify    string `yaml:"spotify,omitempty"`
	SoundCloud string `yaml:"soundcloud,omitempty"`
	Bandcamp   string `yaml:"bandcamp,omitempty"`
	Website    string `yaml:"website,omitempty"`
}

// ExportArtistData represents an artist in the markdown frontmatter
type ExportArtistData struct {
	Name     string             `yaml:"name"`
	Position int                `yaml:"position"`
	SetType  string             `yaml:"set_type"`
	City     string             `yaml:"city,omitempty"`
	State    string             `yaml:"state,omitempty"`
	Social   ExportArtistSocial `yaml:"social,omitempty"`
}

// ExportFrontmatter represents the complete markdown frontmatter
type ExportFrontmatter struct {
	Version    string             `yaml:"version"`
	ExportedAt string             `yaml:"exported_at"`
	Show       ExportShowData     `yaml:"show"`
	Venues     []ExportVenueData  `yaml:"venues"`
	Artists    []ExportArtistData `yaml:"artists"`
}

// ──────────────────────────────────────────────
// Venue types
// ──────────────────────────────────────────────

// CreateVenueRequest represents the data needed to create a new venue
type CreateVenueRequest struct {
	Name       string  `json:"name" validate:"required"`
	Address    *string `json:"address"`
	City       string  `json:"city" validate:"required"`
	State      string  `json:"state" validate:"required"`
	Zipcode    *string `json:"zipcode"`
	Instagram  *string `json:"instagram"`
	Facebook   *string `json:"facebook"`
	Twitter    *string `json:"twitter"`
	YouTube    *string `json:"youtube"`
	Spotify    *string `json:"spotify"`
	SoundCloud *string `json:"soundcloud"`
	Bandcamp   *string `json:"bandcamp"`
	Website    *string `json:"website"`
}

// VenueDetailResponse represents the venue data returned to clients
type VenueDetailResponse struct {
	ID          uint           `json:"id"`
	Slug        string         `json:"slug"`
	Name        string         `json:"name"`
	Address     *string        `json:"address"`
	City        string         `json:"city"`
	State       string         `json:"state"`
	Zipcode     *string        `json:"zipcode"`
	Verified    bool           `json:"verified"`    // Admin-verified as legitimate venue
	SubmittedBy *uint          `json:"submitted_by"` // User ID who originally submitted this venue
	Social      SocialResponse `json:"social"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// VenueWithShowCountResponse includes upcoming show count for a venue.
type VenueWithShowCountResponse struct {
	VenueDetailResponse
	UpcomingShowCount int `json:"upcoming_show_count"`
}

// VenueListFilters contains filter options for listing venues
type VenueListFilters struct {
	State    string
	City     string
	Cities   []CityStateFilter
	Verified *bool
}

// VenueShowResponse represents a show in the venue shows endpoint
type VenueShowResponse struct {
	ID             uint             `json:"id"`
	Title          string           `json:"title"`
	EventDate      time.Time        `json:"event_date"`
	City           *string          `json:"city"`
	State          *string          `json:"state"`
	Price          *float64         `json:"price"`
	AgeRequirement *string          `json:"age_requirement"`
	Artists        []ArtistResponse `json:"artists"`
}

// VenueCityResponse represents a city with venue count for filtering
type VenueCityResponse struct {
	City       string `json:"city"`
	State      string `json:"state"`
	VenueCount int    `json:"venue_count"`
}

// VenueEditRequest represents the data for updating a venue
type VenueEditRequest struct {
	Name       *string `json:"name"`
	Address    *string `json:"address"`
	City       *string `json:"city"`
	State      *string `json:"state"`
	Zipcode    *string `json:"zipcode"`
	Instagram  *string `json:"instagram"`
	Facebook   *string `json:"facebook"`
	Twitter    *string `json:"twitter"`
	YouTube    *string `json:"youtube"`
	Spotify    *string `json:"spotify"`
	SoundCloud *string `json:"soundcloud"`
	Bandcamp   *string `json:"bandcamp"`
	Website    *string `json:"website"`
}

// PendingVenueEditResponse represents a pending venue edit returned to clients
type PendingVenueEditResponse struct {
	ID          uint                    `json:"id"`
	VenueID     uint                    `json:"venue_id"`
	SubmittedBy uint                    `json:"submitted_by"`
	Status      models.VenueEditStatus  `json:"status"`

	// Proposed changes
	Name       *string `json:"name,omitempty"`
	Address    *string `json:"address,omitempty"`
	City       *string `json:"city,omitempty"`
	State      *string `json:"state,omitempty"`
	Zipcode    *string `json:"zipcode,omitempty"`
	Instagram  *string `json:"instagram,omitempty"`
	Facebook   *string `json:"facebook,omitempty"`
	Twitter    *string `json:"twitter,omitempty"`
	YouTube    *string `json:"youtube,omitempty"`
	Spotify    *string `json:"spotify,omitempty"`
	SoundCloud *string `json:"soundcloud,omitempty"`
	Bandcamp   *string `json:"bandcamp,omitempty"`
	Website    *string `json:"website,omitempty"`

	// Workflow fields
	RejectionReason *string    `json:"rejection_reason,omitempty"`
	ReviewedBy      *uint      `json:"reviewed_by,omitempty"`
	ReviewedAt      *time.Time `json:"reviewed_at,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Embedded venue info for context
	Venue         *VenueDetailResponse `json:"venue,omitempty"`
	SubmitterName *string              `json:"submitter_name,omitempty"`
	ReviewerName  *string              `json:"reviewer_name,omitempty"`
}

// UnverifiedVenueResponse represents an unverified venue for admin review
type UnverifiedVenueResponse struct {
	ID          uint      `json:"id"`
	Slug        string    `json:"slug"`
	Name        string    `json:"name"`
	Address     *string   `json:"address"`
	City        string    `json:"city"`
	State       string    `json:"state"`
	Zipcode     *string   `json:"zipcode"`
	SubmittedBy *uint     `json:"submitted_by"`
	CreatedAt   time.Time `json:"created_at"`
	ShowCount   int       `json:"show_count"` // Number of shows using this venue
}

// ──────────────────────────────────────────────
// Artist types
// ──────────────────────────────────────────────

// CreateArtistRequest represents the data needed to create a new artist
type CreateArtistRequest struct {
	Name       string  `json:"name" validate:"required"`
	State      *string `json:"state"`
	City       *string `json:"city"`
	Instagram  *string `json:"instagram"`
	Facebook   *string `json:"facebook"`
	Twitter    *string `json:"twitter"`
	YouTube    *string `json:"youtube"`
	Spotify    *string `json:"spotify"`
	SoundCloud *string `json:"soundcloud"`
	Bandcamp   *string `json:"bandcamp"`
	Website    *string `json:"website"`
}

// ArtistDetailResponse represents the artist data returned to clients
type ArtistDetailResponse struct {
	ID               uint           `json:"id"`
	Slug             string         `json:"slug"`
	Name             string         `json:"name"`
	State            *string        `json:"state"`
	City             *string        `json:"city"`
	BandcampEmbedURL *string        `json:"bandcamp_embed_url"`
	Social           SocialResponse `json:"social"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

// SocialResponse represents social media links
type SocialResponse struct {
	Instagram  *string `json:"instagram"`
	Facebook   *string `json:"facebook"`
	Twitter    *string `json:"twitter"`
	YouTube    *string `json:"youtube"`
	Spotify    *string `json:"spotify"`
	SoundCloud *string `json:"soundcloud"`
	Bandcamp   *string `json:"bandcamp"`
	Website    *string `json:"website"`
}

// ArtistWithShowCountResponse includes upcoming show count for an artist.
type ArtistWithShowCountResponse struct {
	ArtistDetailResponse
	UpcomingShowCount int `json:"upcoming_show_count"`
}

// ArtistCityResponse represents a city with artist count for filtering
type ArtistCityResponse struct {
	City        string `json:"city"`
	State       string `json:"state"`
	ArtistCount int    `json:"artist_count"`
}

// ArtistLabelResponse represents a label associated with an artist
type ArtistLabelResponse struct {
	ID    uint    `json:"id"`
	Name  string  `json:"name"`
	Slug  string  `json:"slug"`
	City  *string `json:"city"`
	State *string `json:"state"`
}

// ArtistShowResponse represents a show in the artist shows endpoint
type ArtistShowResponse struct {
	ID             uint                     `json:"id"`
	Title          string                   `json:"title"`
	EventDate      time.Time                `json:"event_date"`
	Price          *float64                 `json:"price"`
	AgeRequirement *string                  `json:"age_requirement"`
	Venue          *ArtistShowVenueResponse `json:"venue"`
	Artists        []ArtistShowArtist       `json:"artists"`
}

// ArtistShowVenueResponse represents venue info in artist show response
type ArtistShowVenueResponse struct {
	ID    uint   `json:"id"`
	Slug  string `json:"slug"`
	Name  string `json:"name"`
	City  string `json:"city"`
	State string `json:"state"`
}

// ArtistShowArtist represents an artist on a show bill
type ArtistShowArtist struct {
	ID   uint   `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
}
