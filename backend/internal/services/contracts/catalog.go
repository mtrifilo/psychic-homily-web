// Package contracts defines the interfaces and shared types for services.
// Services must NEVER be imported by this package — only models and third-party libs.
package contracts

import (
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"

	"gorm.io/gorm"
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
	Title          string    `json:"title" validate:"required"`
	EventDate      time.Time `json:"event_date" validate:"required"`
	City           string    `json:"city"`
	State          string    `json:"state"`
	Price          *float64  `json:"price"`
	AgeRequirement string    `json:"age_requirement"`
	Description    string    `json:"description"`
	TicketURL      string    `json:"ticket_url"`
	// ImageURL is populated by the entity_request fulfiller (PSY-1037, the
	// payload's flyer). The direct create handler does not expose it yet (set
	// post-create via the update endpoint), so it leaves it nil here.
	ImageURL *string            `json:"image_url"`
	Venues   []CreateShowVenue  `json:"venues" validate:"required,min=1"`
	Artists  []CreateShowArtist `json:"artists" validate:"required,min=1"`

	// User context for determining show status
	SubmittedByUserID *uint `json:"-"` // User ID of submitter (set by handler)
	SubmitterIsAdmin  bool  `json:"-"` // Whether submitter is admin (set by handler)
	IsPrivate         bool  `json:"-"` // Whether show should be private (user's list only)
}

// UpdateShowRequest represents the basic show fields that can be updated.
// A nil field means "leave unchanged"; only non-nil fields are written.
// Artist and venue association replacement is handled separately via the
// venues/artists params on UpdateShowWithRelations.
type UpdateShowRequest struct {
	Title          *string    `json:"title"`
	EventDate      *time.Time `json:"event_date"`
	City           *string    `json:"city"`
	State          *string    `json:"state"`
	Price          *float64   `json:"price"`
	AgeRequirement *string    `json:"age_requirement"`
	Description    *string    `json:"description"`
	TicketURL      *string    `json:"ticket_url"`
	ImageURL       *string    `json:"image_url"`
}

// ShowResponse represents the show data returned to clients
type ShowResponse struct {
	ID                uint             `json:"id"`
	Slug              string           `json:"slug"`
	Title             string           `json:"title"`
	EventDate         time.Time        `json:"event_date"`
	City              *string          `json:"city"`
	State             *string          `json:"state"`
	Price             *float64         `json:"price"`
	AgeRequirement    *string          `json:"age_requirement"`
	Description       *string          `json:"description"`
	TicketURL         *string          `json:"ticket_url,omitempty"`
	ImageURL          *string          `json:"image_url"` // Optional show flyer (PSY-521)
	Status            string           `json:"status"`
	SubmittedBy       *uint            `json:"submitted_by,omitempty"`
	RejectionReason   *string          `json:"rejection_reason,omitempty"`
	RejectionCategory *string          `json:"rejection_category,omitempty"`
	Venues            []VenueResponse  `json:"venues"`
	Artists           []ArtistResponse `json:"artists"`
	CreatedAt         time.Time        `json:"created_at"`
	UpdatedAt         time.Time        `json:"updated_at"`

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
	Timezone   *string `json:"timezone"`     // IANA zone for rendering this show's time in venue-local time (PSY-985)
	Verified   bool    `json:"verified"`     // Admin-verified as legitimate venue
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
	// TagSlugs narrows results to shows tagged with these slugs.
	// Empty slice means "no tag filter".
	TagSlugs []string
	// TagMatchAny switches the tag filter to OR semantics. When false
	// (default) the shows must have every tag in TagSlugs (AND).
	TagMatchAny bool
}

// ShowCityResponse represents a city with the count of upcoming shows.
//
// Latitude/Longitude are the city's geocoded centroid (the same offline
// GeoNames source PSY-985 uses for venue coordinates), resolved from the
// (city, state) pair. They let the frontend pick the geographically NEAREST
// has-shows city for a new visitor whose exact city has no shows (PSY-981).
// Both are nil together when the geocoder can't resolve the city (an obscure
// place, or a non-US/CA city the GeoNames slice doesn't cover) — callers
// fall back to exact city-name matching, so a miss degrades gracefully.
type ShowCityResponse struct {
	City      string   `json:"city"`
	State     string   `json:"state"`
	ShowCount int      `json:"show_count"`
	Latitude  *float64 `json:"latitude,omitempty"`  // Geocoded city centroid (PSY-985 source, PSY-981)
	Longitude *float64 `json:"longitude,omitempty"` // Geocoded city centroid (PSY-985 source, PSY-981)
}

// ShowSearchResult is the row shape returned by GET /shows/search.
// Contains just enough data for the frontend's
// "{Headliner} @ {Venue} · {Date}" entity-search label, without the cost of
// hydrating the full ShowResponse (artists slice, venues slice, etc).
//
// Headliner resolution mirrors the existing convention used elsewhere in
// catalog/show.go (e.g. checkDuplicateHeadlinerConflicts): the headliner is
// the show_artists row with set_type = 'headliner', falling back to position
// = 0. There is no `is_headliner` column on show_artists. PSY-520.
type ShowSearchResult struct {
	ID            uint      `json:"id"`
	Slug          string    `json:"slug"`
	Title         string    `json:"title"`
	HeadlinerName string    `json:"headliner_name"`
	VenueName     string    `json:"venue_name"`
	EventDate     time.Time `json:"event_date"`
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
	Name        string  `json:"name" validate:"required"`
	Address     *string `json:"address"`
	City        string  `json:"city" validate:"required"`
	State       string  `json:"state" validate:"required"`
	Country     *string `json:"country"`
	Zipcode     *string `json:"zipcode"`
	Capacity    *int    `json:"capacity"`
	Instagram   *string `json:"instagram"`
	Facebook    *string `json:"facebook"`
	Twitter     *string `json:"twitter"`
	YouTube     *string `json:"youtube"`
	Spotify     *string `json:"spotify"`
	SoundCloud  *string `json:"soundcloud"`
	Bandcamp    *string `json:"bandcamp"`
	Website     *string `json:"website"`
	Description *string `json:"description"`
	ImageURL    *string `json:"image_url"`
	SubmittedBy *uint   `json:"-"` // Set by handler, not from request body
}

// UpdateVenueRequest represents the data that can be updated on a venue.
// A nil field means "leave unchanged"; only non-nil fields are written.
//
// Name/City/State map to NOT NULL columns and are written as-is (the handler
// rejects empty values up front). The remaining optional string columns are
// nullable, so Description and ImageURL normalize an empty string to SQL NULL
// in the service (utils.NilIfEmpty). Address/Country/Zipcode and the social
// fields preserve the prior behavior of writing the value through verbatim.
type UpdateVenueRequest struct {
	Name        *string `json:"name"`
	Address     *string `json:"address"`
	City        *string `json:"city"`
	State       *string `json:"state"`
	Country     *string `json:"country"`
	Zipcode     *string `json:"zipcode"`
	Capacity    *int    `json:"capacity"`
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

// VenueDetailResponse represents the venue data returned to clients
type VenueDetailResponse struct {
	ID          uint           `json:"id"`
	Slug        string         `json:"slug"`
	Name        string         `json:"name"`
	Address     *string        `json:"address"`
	City        string         `json:"city"`
	State       string         `json:"state"`
	Country     *string        `json:"country,omitempty"`
	Latitude    *float64       `json:"latitude,omitempty"`  // Geocoded city centroid (PSY-985)
	Longitude   *float64       `json:"longitude,omitempty"` // Geocoded city centroid (PSY-985)
	Timezone    *string        `json:"timezone"`            // IANA zone resolved from location (PSY-985)
	Zipcode     *string        `json:"zipcode"`
	Capacity    *int           `json:"capacity"` // Venue capacity (PSY-1179); not redacted for unverified venues
	Description *string        `json:"description,omitempty"`
	ImageURL    *string        `json:"image_url"`    // Optional venue photo (PSY-521)
	Verified    bool           `json:"verified"`     // Admin-verified as legitimate venue
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
	// TagSlugs narrows results to venues tagged with these slugs.
	// Empty slice means "no tag filter".
	TagSlugs []string
	// TagMatchAny switches the tag filter to OR semantics. When false
	// (default) the venues must have every tag in TagSlugs (AND).
	TagMatchAny bool
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
	Name        string  `json:"name" validate:"required"`
	State       *string `json:"state"`
	City        *string `json:"city"`
	Country     *string `json:"country"`
	Instagram   *string `json:"instagram"`
	Facebook    *string `json:"facebook"`
	Twitter     *string `json:"twitter"`
	YouTube     *string `json:"youtube"`
	Spotify     *string `json:"spotify"`
	SoundCloud  *string `json:"soundcloud"`
	Bandcamp    *string `json:"bandcamp"`
	Website     *string `json:"website"`
	Description *string `json:"description"`
	// ImageURL + BandcampEmbedURL are populated by the entity_request fulfiller
	// (PSY-1038). The direct admin create handler does not expose them yet (set
	// post-create via the update endpoints), so it leaves them nil here.
	ImageURL         *string `json:"image_url"`
	BandcampEmbedURL *string `json:"bandcamp_embed_url"`
}

// UpdateArtistRequest represents the data that can be updated on an artist.
// A nil field means "leave unchanged"; only non-nil fields are written.
//
// Every column here is nullable, so the service normalizes an empty string to
// SQL NULL (utils.NilIfEmpty) for all fields except Name, which maps to a NOT
// NULL column. Name additionally drives slug regeneration and a uniqueness
// check in the service. BandcampEmbedURL is the embed-specific column distinct
// from the Bandcamp social profile URL.
type UpdateArtistRequest struct {
	Name             *string `json:"name"`
	State            *string `json:"state"`
	City             *string `json:"city"`
	Country          *string `json:"country"`
	Description      *string `json:"description"`
	BandcampEmbedURL *string `json:"bandcamp_embed_url"`
	Instagram        *string `json:"instagram"`
	Facebook         *string `json:"facebook"`
	Twitter          *string `json:"twitter"`
	YouTube          *string `json:"youtube"`
	Spotify          *string `json:"spotify"`
	SoundCloud       *string `json:"soundcloud"`
	Bandcamp         *string `json:"bandcamp"`
	Website          *string `json:"website"`
}

// ArtistDetailResponse represents the artist data returned to clients
type ArtistDetailResponse struct {
	ID               uint           `json:"id"`
	Slug             string         `json:"slug"`
	Name             string         `json:"name"`
	State            *string        `json:"state"`
	City             *string        `json:"city"`
	Country          *string        `json:"country,omitempty"` // PSY-558: optional country (Australia, UK, etc.)
	BandcampEmbedURL *string        `json:"bandcamp_embed_url"`
	Description      *string        `json:"description,omitempty"`
	ImageURL         *string        `json:"image_url"`        // Optional artist photo (PSY-521)
	ImageSource      *string        `json:"image_source"`     // Image provider for attribution (PSY-1175)
	ImageSourceURL   *string        `json:"image_source_url"` // Deep linkback for attribution (PSY-1175)
	ImageLicense     *string        `json:"image_license"`    // CC license for a Commons photo (PSY-1232)
	ImageAuthor      *string        `json:"image_author"`     // Photographer credit for a Commons photo (PSY-1232)
	Social           SocialResponse `json:"social"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	// Stats is populated only by detail-page lookups (GetArtist /
	// GetArtistBySlug — PSY-639). List, search, and mutation responses leave
	// it nil so the omitempty tag drops it from the wire.
	Stats *ArtistStatsResponse `json:"stats,omitempty"`
}

// ArtistStatsResponse carries the at-a-glance counts surfaced on the artist
// detail page sidebar (PSY-639). Folded into ArtistDetailResponse on the
// detail-page lookups; nil on list / search / mutation responses.
type ArtistStatsResponse struct {
	Releases            int `json:"releases"`
	Labels              int `json:"labels"`
	ShowsTracked        int `json:"shows_tracked"` // past + future
	SimilarArtists      int `json:"similar_artists"`
	FestivalAppearances int `json:"festival_appearances"`
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
//
// LastShowDate is the most recent past approved show date for the artist.
// Only populated when the service runs in evergreen mode (e.g. tag-filtered
// /artists per PSY-495); stays nil on the default activity-gated path since
// the caller already knows there is at least one upcoming show.
type ArtistWithShowCountResponse struct {
	ArtistDetailResponse
	UpcomingShowCount int        `json:"upcoming_show_count"`
	LastShowDate      *time.Time `json:"last_show_date,omitempty"`
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
	ID       uint    `json:"id"`
	Slug     string  `json:"slug"`
	Name     string  `json:"name"`
	City     string  `json:"city"`
	State    string  `json:"state"`
	Timezone *string `json:"timezone"` // IANA zone for rendering this show's time in venue-local time (PSY-985)
}

// ArtistShowArtist represents an artist on a show bill
type ArtistShowArtist struct {
	ID   uint   `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
}

// ArtistAliasResponse represents an artist alias in API responses
type ArtistAliasResponse struct {
	ID        uint   `json:"id"`
	ArtistID  uint   `json:"artist_id"`
	Alias     string `json:"alias"`
	CreatedAt string `json:"created_at"`
}

// MergeArtistResult contains the outcome of merging two artists
type MergeArtistResult struct {
	CanonicalArtistID    uint   `json:"canonical_artist_id"`
	MergedArtistID       uint   `json:"merged_artist_id"`
	MergedArtistName     string `json:"merged_artist_name"`
	ShowsMoved           int64  `json:"shows_moved"`
	ReleasesMoved        int64  `json:"releases_moved"`
	LabelsMoved          int64  `json:"labels_moved"`
	FestivalsMoved       int64  `json:"festivals_moved"`
	RelationshipsMoved   int64  `json:"relationships_moved"`
	BookmarksMoved       int64  `json:"bookmarks_moved"`
	CollectionItemsMoved int64  `json:"crate_items_moved"`
	FiltersUpdated       int64  `json:"filters_updated"`
	AliasCreated         bool   `json:"alias_created"`
}

// ──────────────────────────────────────────────
// Scene types (computed city aggregations)
// ──────────────────────────────────────────────

// SceneListResponse represents a scene in the list endpoint. Under metro keying
// (PSY-1255 step C) City/State are the metro's PRINCIPAL city/state (or the
// literal city for a non-US / no-CBSA fallback scene).
type SceneListResponse struct {
	City              string `json:"city"`
	State             string `json:"state"`
	Slug              string `json:"slug"`
	VenueCount        int    `json:"venue_count"`
	UpcomingShowCount int    `json:"upcoming_show_count"`
	TotalShowCount    int    `json:"total_show_count"`
	// ShowsThisWeek is the ≤7-day slice of UpcomingShowCount (PSY-1309) — the
	// "happening this week" signal that drives the Atlas globe's pulse
	// treatment. Same scene scoping as the other counts.
	ShowsThisWeek int `json:"shows_this_week"`
	// Latitude/Longitude position the scene on the geographic-discovery map
	// (PSY-1212): the metro principal city's centroid (or the fallback city's,
	// geocoded the same way as ShowCityResponse — PSY-985/PSY-981), so a scene
	// plots at the same point here and on the shows-by-city map. Omitted (nil) on
	// a geocoder miss; the scene still lists, just unplaceable.
	Latitude  *float64 `json:"latitude,omitempty"`
	Longitude *float64 `json:"longitude,omitempty"`
}

// SceneShowSummary is one upcoming show in a scene's "This week" preview row
// (PSY-1309) — deliberately thin (the Atlas preview panel needs a line, not the
// full ShowResponse payload). VenueName is the first venue on the bill.
type SceneShowSummary struct {
	ID        uint   `json:"id"`
	Slug      string `json:"slug,omitempty"` // canonical /shows/{slug} target; "" when the show has no slug (clients fall back to the id)
	Title     string `json:"title"`
	EventDate string `json:"event_date"` // ISO date (YYYY-MM-DD)
	VenueName string `json:"venue_name,omitempty"`
}

// SceneDetailResponse represents the full computed scene for a metro (or a
// no-CBSA fallback city); City/State are the principal city/state.
type SceneDetailResponse struct {
	City        string     `json:"city"`
	State       string     `json:"state"`
	Slug        string     `json:"slug"`
	Description *string    `json:"description"` // nil until scenes table exists
	Stats       SceneStats `json:"stats"`
	Pulse       ScenePulse `json:"pulse"`
}

// SceneStats holds aggregate counts for a scene
type SceneStats struct {
	VenueCount        int `json:"venue_count"`
	ArtistCount       int `json:"artist_count"`
	UpcomingShowCount int `json:"upcoming_show_count"`
	FestivalCount     int `json:"festival_count"`
}

// ScenePulse holds activity trend data for a scene
type ScenePulse struct {
	ShowsThisMonth        int    `json:"shows_this_month"`
	ShowsPrevMonth        int    `json:"shows_prev_month"`
	ShowsTrend            string `json:"shows_trend"`
	NewArtists30d         int    `json:"new_artists_30d"`
	ActiveVenuesThisMonth int    `json:"active_venues_this_month"`
	ShowsByMonth          []int  `json:"shows_by_month"` // last 6 months
}

// SceneArtistResponse represents an artist in a scene's roster. Under the
// metro-keyed model (PSY-1255 step C) the roster is every band BASED in the
// metro; IsActive flags the ones with an upcoming show or one in the active
// window (played anywhere), which the frontend highlights.
type SceneArtistResponse struct {
	ID        uint    `json:"id"`
	Slug      string  `json:"slug"`
	Name      string  `json:"name"`
	City      *string `json:"city"`
	State     *string `json:"state"`
	ShowCount int     `json:"show_count"`
	IsActive  bool    `json:"is_active"`
	// BandcampEmbedURL is the artist's embeddable Bandcamp /album|/track URL
	// (artists.bandcamp_embed_url, PSY-1187/1188/1189), nil when the artist has
	// none. The /atlas scene preview plays the first active artist that has one
	// as the scene's "instant payoff" track (PSY-1224).
	BandcampEmbedURL *string `json:"bandcamp_embed_url"`
}

// ──────────────────────────────────────────────
// Genre profile types (for scene and venue intelligence)
// ──────────────────────────────────────────────

// GenreCount represents a genre tag with its associated artist count
type GenreCount struct {
	TagID uint   `json:"tag_id"`
	Name  string `json:"name"`
	Slug  string `json:"slug"`
	Count int    `json:"count"`
}

// SceneGenreResponse represents the genre distribution for a scene (city)
type SceneGenreResponse struct {
	Genres         []GenreCount `json:"genres"`
	DiversityIndex float64      `json:"diversity_index"` // -1 if insufficient data
	DiversityLabel string       `json:"diversity_label"` // "Highly diverse", "Mixed", "Genre-focused", ""
}

// VenueGenreResponse represents the genre profile for a venue
type VenueGenreResponse struct {
	Genres []GenreCount `json:"genres"`
}

// ──────────────────────────────────────────────
// Scene graph (PSY-367) — derived per-scene artist relationship graph
// ──────────────────────────────────────────────

// SceneGraphResponse is the payload for GET /scenes/{slug}/graph.
// Cluster IDs are computed at query time from each artist's most-frequent venue
// in the scene (see docs/features/scene-graph-layout.md §4 for the rationale).
type SceneGraphResponse struct {
	Scene    SceneGraphInfo      `json:"scene"`
	Clusters []SceneGraphCluster `json:"clusters"`
	Nodes    []SceneGraphNode    `json:"nodes"`
	Links    []SceneGraphLink    `json:"links"`
}

// SceneGraphInfo holds scene metadata for the graph response.
type SceneGraphInfo struct {
	Slug             string `json:"slug"`
	City             string `json:"city"`
	State            string `json:"state"`
	ArtistCount      int    `json:"artist_count"`       // artists in the response (top-N cap applied)
	EdgeCount        int    `json:"edge_count"`         // total edges in the response (post type-filter)
	MetroRosterTotal int    `json:"metro_roster_total"` // full based-in metro roster before top-N cap
	RosterTruncated  bool   `json:"roster_truncated"`   // true when metro_roster_total > artist_count
}

// SceneGraphCluster groups artists in the scene. v1 cluster signal is the
// artist's most-frequently-played venue within the scene. Clusters with fewer
// than the size threshold are rolled into a single "other" cluster.
type SceneGraphCluster struct {
	ID         string `json:"id"`          // "v_<venue_id>" or "other"
	Label      string `json:"label"`       // venue name or "Other"
	Size       int    `json:"size"`        // number of artists in this cluster
	ColorIndex int    `json:"color_index"` // 0-7 = Okabe-Ito index; -1 = "other" (grey)
}

// SceneGraphNode represents an artist in the scene graph.
type SceneGraphNode struct {
	ID                uint   `json:"id"`
	Name              string `json:"name"`
	Slug              string `json:"slug"`
	City              string `json:"city,omitempty"`
	State             string `json:"state,omitempty"`
	UpcomingShowCount int    `json:"upcoming_show_count"`
	ClusterID         string `json:"cluster_id"` // matches SceneGraphCluster.ID; "other" for tail
	IsIsolate         bool   `json:"is_isolate"` // true when the artist has no in-scene edges (post type-filter)
}

// SceneGraphLink represents an in-scene relationship between two artists.
// Voting and user-vote data are intentionally omitted — scene graph is read-only
// per the spike's out-of-scope list (see docs/features/scene-graph-layout.md §8).
type SceneGraphLink struct {
	SourceID       uint    `json:"source_id"`
	TargetID       uint    `json:"target_id"`
	Type           string  `json:"type"`
	Score          float64 `json:"score"`
	Detail         any     `json:"detail,omitempty"`
	IsCrossCluster bool    `json:"is_cross_cluster"` // derived: source.cluster_id != target.cluster_id
}

// ──────────────────────────────────────────────
// Venue bill network (PSY-365) — co-bill network of artists at a single venue
// ──────────────────────────────────────────────
//
// The venue analog of the scene graph (PSY-367). Edges are weighted by the
// number of shows the two artists shared *at this specific venue* (not
// globally), which is the unfair-advantage signal called out in
// docs/research/knowledge-graph-viz-prior-art.md §6.
//
// Mirrors SceneGraphResponse field-for-field (`scene` → `venue`, `clusters`,
// `nodes`, `links`) so a shared frontend ForceGraphView can render either
// payload. Cluster-aware layout machinery is preserved on the type even when
// no clusters are returned (v1 ships without explicit clusters — see PSY-365
// PR notes for the rationale).

// VenueBillNetworkResponse is the payload for GET /venues/{id}/bill-network.
type VenueBillNetworkResponse struct {
	Venue    VenueBillNetworkInfo      `json:"venue"`
	Clusters []VenueBillNetworkCluster `json:"clusters"`
	Nodes    []VenueBillNetworkNode    `json:"nodes"`
	Links    []VenueBillNetworkLink    `json:"links"`
}

// VenueBillNetworkInfo holds venue metadata and aggregate counts for the graph.
// Fields mirror SceneGraphInfo (slug + counts) plus venue-specific identifiers.
type VenueBillNetworkInfo struct {
	ID          uint   `json:"id"`
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	City        string `json:"city,omitempty"`
	State       string `json:"state,omitempty"`
	ArtistCount int    `json:"artist_count"` // distinct artists matching the time-window filter
	EdgeCount   int    `json:"edge_count"`   // co-bill pairs above the min-shared-shows threshold
	ShowCount   int    `json:"show_count"`   // approved shows used to derive the network
	// WindowLabel describes the active time window in the response so the
	// frontend can label the graph without reverse-engineering the filter.
	// One of: "all_time", "last_12m", "year".
	Window string `json:"window"`
	// Year is populated only when Window=="year"; carries the requested year.
	Year *int `json:"year,omitempty"`
}

// VenueBillNetworkCluster matches the SceneGraphCluster shape so the same
// ForceGraphView legend renders both. v1 ships without explicit clusters at
// venue scope (every artist's primary venue is, by definition, this venue —
// the scene graph's signal collapses), so the array is typically empty.
type VenueBillNetworkCluster struct {
	ID         string `json:"id"`
	Label      string `json:"label"`
	Size       int    `json:"size"`
	ColorIndex int    `json:"color_index"`
}

// VenueBillNetworkNode mirrors SceneGraphNode. ClusterID defaults to "other"
// when no clusters are computed (v1 default).
type VenueBillNetworkNode struct {
	ID                uint   `json:"id"`
	Name              string `json:"name"`
	Slug              string `json:"slug"`
	City              string `json:"city,omitempty"`
	State             string `json:"state,omitempty"`
	UpcomingShowCount int    `json:"upcoming_show_count"`
	ClusterID         string `json:"cluster_id"`
	IsIsolate         bool   `json:"is_isolate"`
	// AtVenueShowCount is the number of approved shows this artist has played
	// at the venue, within the active time window. Surfaces the rank signal
	// the user is intuiting from "who's a regular here".
	AtVenueShowCount int `json:"at_venue_show_count"`
}

// VenueBillNetworkLink mirrors SceneGraphLink. The Detail field carries
// `shared_count` (number of shared shows AT THIS VENUE in the active window)
// and `last_shared` (most recent shared event date) so the frontend tooltip
// stays edge-grammar-compatible with PSY-362.
type VenueBillNetworkLink struct {
	SourceID       uint    `json:"source_id"`
	TargetID       uint    `json:"target_id"`
	Type           string  `json:"type"`
	Score          float64 `json:"score"`
	Detail         any     `json:"detail,omitempty"`
	IsCrossCluster bool    `json:"is_cross_cluster"`
}

// ──────────────────────────────────────────────
// Show Service Interfaces
// ──────────────────────────────────────────────

// ShowServiceInterface defines the contract for core show CRUD and search operations.
type ShowServiceInterface interface {
	CreateShow(req *CreateShowRequest) (*ShowResponse, error)
	GetShow(showID uint) (*ShowResponse, error)
	GetShowBySlug(slug string) (*ShowResponse, error)
	GetShows(filters map[string]interface{}) ([]*ShowResponse, error)
	GetUserSubmissions(userID uint, limit, offset int) ([]ShowResponse, int, error)
	UpdateShow(showID uint, req *UpdateShowRequest) (*ShowResponse, error)
	UpdateShowWithRelations(showID uint, req *UpdateShowRequest, venues []CreateShowVenue, artists []CreateShowArtist, isAdmin bool) (*ShowResponse, []OrphanedArtist, error)
	GetUpcomingShows(timezone string, cursor string, limit int, includeNonApproved bool, filters *UpcomingShowsFilter) ([]*ShowResponse, *string, error)
	GetShowCities(timezone string) ([]ShowCityResponse, error)
	DeleteShow(showID uint) error
	// SearchShows returns up to 20 shows matching the query in show title or
	// any bill artist name (case-insensitive), ordered by event_date DESC.
	// Empty query returns an empty slice. PSY-520.
	SearchShows(query string) ([]*ShowSearchResult, error)
}

// ShowAdminServiceInterface defines the contract for admin show management operations
// including pending/rejected queries, approval flows, and batch operations.
type ShowAdminServiceInterface interface {
	GetPendingShows(limit, offset int, filters *PendingShowsFilter) ([]*ShowResponse, int64, error)
	GetRejectedShows(limit, offset int, search string) ([]*ShowResponse, int64, error)
	ApproveShow(showID uint, verifyVenues bool) (*ShowResponse, error)
	RejectShow(showID uint, reason string) (*ShowResponse, error)
	BatchApproveShows(showIDs []uint) (*BatchShowResult, error)
	BatchRejectShows(showIDs []uint, reason string, category string) (*BatchShowResult, error)
	GetAdminShows(limit, offset int, filters AdminShowFilters) ([]*ShowResponse, int64, error)
}

// ShowImportServiceInterface defines the contract for show import/export operations.
type ShowImportServiceInterface interface {
	ExportShowToMarkdown(showID uint) ([]byte, string, error)
	ParseShowMarkdown(content []byte) (*ParsedShowImport, error)
	PreviewShowImport(content []byte) (*ImportPreviewResponse, error)
	ConfirmShowImport(content []byte, isAdmin bool) (*ShowResponse, error)
}

// ShowStateServiceInterface defines the contract for show state mutation operations
// such as publishing, unpublishing, and setting sold-out/cancelled flags.
type ShowStateServiceInterface interface {
	UnpublishShow(showID uint, userID uint, isAdmin bool) (*ShowResponse, error)
	MakePrivateShow(showID uint, userID uint, isAdmin bool) (*ShowResponse, error)
	PublishShow(showID uint, userID uint, isAdmin bool) (*ShowResponse, error)
	SetShowSoldOut(showID uint, isSoldOut bool) (*ShowResponse, error)
	SetShowCancelled(showID uint, isCancelled bool) (*ShowResponse, error)
}

// ShowFullServiceInterface is the composite interface that embeds all show service
// concerns. The concrete ShowService satisfies this. Useful for the service container
// and backward compatibility where a single reference to all methods is needed.
type ShowFullServiceInterface interface {
	ShowServiceInterface
	ShowAdminServiceInterface
	ShowImportServiceInterface
	ShowStateServiceInterface
}

// ──────────────────────────────────────────────
// Venue Service Interface
// ──────────────────────────────────────────────

// VenueServiceInterface defines the contract for venue operations.
type VenueServiceInterface interface {
	CreateVenue(req *CreateVenueRequest, isAdmin bool) (*VenueDetailResponse, error)
	GetVenue(venueID uint) (*VenueDetailResponse, error)
	GetVenueBySlug(slug string) (*VenueDetailResponse, error)
	GetVenues(filters map[string]interface{}) ([]*VenueDetailResponse, error)
	UpdateVenue(venueID uint, req *UpdateVenueRequest) (*VenueDetailResponse, error)
	DeleteVenue(venueID uint) error
	SearchVenues(query string) ([]*VenueDetailResponse, error)
	FindOrCreateVenue(name, city, state string, address, zipcode *string, db *gorm.DB, isAdmin bool) (*catalogm.Venue, bool, error)
	VerifyVenue(venueID uint) (*VenueDetailResponse, error)
	GetVenuesWithShowCounts(filters VenueListFilters, limit, offset int) ([]*VenueWithShowCountResponse, int64, error)
	GetUpcomingShowsForVenue(venueID uint, timezone string, limit int) ([]*VenueShowResponse, int64, error)
	GetShowsForVenue(venueID uint, timezone string, limit int, timeFilter string) ([]*VenueShowResponse, int64, error)
	GetVenueCities() ([]*VenueCityResponse, error)
	GetVenueModel(venueID uint) (*catalogm.Venue, error)
	GetUnverifiedVenues(limit, offset int) ([]*UnverifiedVenueResponse, int64, error)
	GetVenueGenreProfile(venueID uint) ([]GenreCount, error)
	// PSY-365: venue-rooted co-bill network. Edges are weighted by the
	// number of shared shows AT THIS VENUE (not globally) within the
	// requested time window. Window is one of "all", "12m", "year"; Year
	// is required iff Window=="year". Empty Window defaults to "all".
	GetVenueBillNetwork(venueID uint, window string, year *int) (*VenueBillNetworkResponse, error)
}

// ──────────────────────────────────────────────
// Artist Service Interface
// ──────────────────────────────────────────────

// ArtistServiceInterface defines the contract for artist operations.
type ArtistServiceInterface interface {
	CreateArtist(req *CreateArtistRequest) (*ArtistDetailResponse, error)
	GetArtist(artistID uint) (*ArtistDetailResponse, error)
	GetArtistByName(name string) (*ArtistDetailResponse, error)
	GetArtistBySlug(slug string) (*ArtistDetailResponse, error)
	GetArtists(filters map[string]interface{}) ([]*ArtistDetailResponse, error)
	GetArtistsWithShowCounts(filters map[string]interface{}) ([]*ArtistWithShowCountResponse, error)
	UpdateArtist(artistID uint, req *UpdateArtistRequest) (*ArtistDetailResponse, error)
	DeleteArtist(artistID uint) error
	SearchArtists(query string) ([]*ArtistDetailResponse, error)
	GetShowsForArtist(artistID uint, timezone string, limit int, timeFilter string) ([]*ArtistShowResponse, int64, error)
	GetArtistCities() ([]*ArtistCityResponse, error)
	GetLabelsForArtist(artistID uint) ([]*ArtistLabelResponse, error)
	AddArtistAlias(artistID uint, alias string) (*ArtistAliasResponse, error)
	RemoveArtistAlias(aliasID uint) error
	GetArtistAliases(artistID uint) ([]*ArtistAliasResponse, error)
	MergeArtists(canonicalID, mergeFromID uint) (*MergeArtistResult, error)
}

// ──────────────────────────────────────────────
// Scene Service Interface
// ──────────────────────────────────────────────

// SceneServiceInterface defines the contract for computed scene aggregations.
// Scenes are keyed by US Census CBSA metro (PSY-1255 step C); the (city, state)
// args are the metro's PRINCIPAL city/state (or the literal city for a non-US /
// no-CBSA fallback scene), as returned by ParseSceneSlug.
type SceneServiceInterface interface {
	ListScenes() ([]*SceneListResponse, error)
	GetSceneDetail(city, state string) (*SceneDetailResponse, error)
	// GetActiveArtists returns the scene's full roster — every band based in the
	// metro — with is_active flagged and sorted first. activeWindowDays is the
	// recency window (a band is active if it has a show within it or upcoming);
	// it is NOT a membership filter, so the returned total is the whole roster.
	GetActiveArtists(city, state string, activeWindowDays, limit, offset int) ([]*SceneArtistResponse, int64, error)
	ParseSceneSlug(slug string) (string, string, error)
	GetSceneGenreDistribution(city, state string) ([]GenreCount, error)
	GetGenreDiversityIndex(city, state string) (float64, error)
	GetSceneGraph(city, state string, types []string) (*SceneGraphResponse, error)
	// GetSceneUpcomingShows returns the scene's next approved shows within
	// windowDays, soonest first, capped at limit — the preview panel's "This
	// week" row (PSY-1309). Metro-scoped like every other scene surface (a
	// Tempe show counts toward the Phoenix scene), which is why this isn't the
	// literal-city shows endpoint.
	GetSceneUpcomingShows(city, state string, windowDays, limit int) ([]SceneShowSummary, error)
}
