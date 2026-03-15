package contracts

import "time"

// ──────────────────────────────────────────────
// Admin Stats types
// ──────────────────────────────────────────────

// AdminDashboardStats contains all dashboard statistics
type AdminDashboardStats struct {
	// Action items (things that need admin attention)
	PendingShows         int64 `json:"pending_shows"`
	PendingVenueEdits    int64 `json:"pending_venue_edits"`
	PendingReports       int64 `json:"pending_reports"`
	PendingArtistReports int64 `json:"pending_artist_reports"`
	UnverifiedVenues     int64 `json:"unverified_venues"`

	// Content totals
	TotalShows   int64 `json:"total_shows"`
	TotalVenues  int64 `json:"total_venues"`
	TotalArtists int64 `json:"total_artists"`

	// Users
	TotalUsers int64 `json:"total_users"`

	// Recent activity (last 7 days)
	ShowsSubmittedLast7Days  int64 `json:"shows_submitted_last_7_days"`
	UsersRegisteredLast7Days int64 `json:"users_registered_last_7_days"`
}

// ──────────────────────────────────────────────
// API Token types
// ──────────────────────────────────────────────

// APITokenResponse represents a token in API responses
type APITokenResponse struct {
	ID          uint       `json:"id"`
	Description *string    `json:"description"`
	Scope       string     `json:"scope"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   time.Time  `json:"expires_at"`
	LastUsedAt  *time.Time `json:"last_used_at"`
	IsExpired   bool       `json:"is_expired"`
}

// APITokenCreateResponse includes the plaintext token (only returned on creation)
type APITokenCreateResponse struct {
	ID          uint       `json:"id"`
	Token       string     `json:"token"` // Plaintext token - only shown once!
	Description *string    `json:"description"`
	Scope       string     `json:"scope"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   time.Time  `json:"expires_at"`
}

// ──────────────────────────────────────────────
// Data Sync types
// ──────────────────────────────────────────────

// ExportedArtist represents an artist for export/import
type ExportedArtist struct {
	Name             string  `json:"name"`
	City             *string `json:"city,omitempty"`
	State            *string `json:"state,omitempty"`
	BandcampEmbedURL *string `json:"bandcampEmbedUrl,omitempty"`
	Instagram        *string `json:"instagram,omitempty"`
	Facebook         *string `json:"facebook,omitempty"`
	Twitter          *string `json:"twitter,omitempty"`
	YouTube          *string `json:"youtube,omitempty"`
	Spotify          *string `json:"spotify,omitempty"`
	SoundCloud       *string `json:"soundcloud,omitempty"`
	Bandcamp         *string `json:"bandcamp,omitempty"`
	Website          *string `json:"website,omitempty"`
}

// ExportedVenue represents a venue for export/import
type ExportedVenue struct {
	Name       string  `json:"name"`
	Address    *string `json:"address,omitempty"`
	City       string  `json:"city"`
	State      string  `json:"state"`
	Zipcode    *string `json:"zipcode,omitempty"`
	Verified   bool    `json:"verified"`
	Instagram  *string `json:"instagram,omitempty"`
	Facebook   *string `json:"facebook,omitempty"`
	Twitter    *string `json:"twitter,omitempty"`
	YouTube    *string `json:"youtube,omitempty"`
	Spotify    *string `json:"spotify,omitempty"`
	SoundCloud *string `json:"soundcloud,omitempty"`
	Bandcamp   *string `json:"bandcamp,omitempty"`
	Website    *string `json:"website,omitempty"`
}

// ExportedShowArtist represents an artist in a show lineup
type ExportedShowArtist struct {
	Name     string `json:"name"`
	Position int    `json:"position"`
	SetType  string `json:"setType"`
}

// ExportedShow represents a show for export/import
type ExportedShow struct {
	Title          string               `json:"title"`
	EventDate      string               `json:"eventDate"` // ISO format
	City           *string              `json:"city,omitempty"`
	State          *string              `json:"state,omitempty"`
	Price          *float64             `json:"price,omitempty"`
	AgeRequirement *string              `json:"ageRequirement,omitempty"`
	Description    *string              `json:"description,omitempty"`
	Status         string               `json:"status"`
	IsSoldOut      bool                 `json:"isSoldOut"`
	IsCancelled    bool                 `json:"isCancelled"`
	Venues         []ExportedVenue      `json:"venues"`
	Artists        []ExportedShowArtist `json:"artists"`
}

// ExportShowsParams contains filters for show export
type ExportShowsParams struct {
	Limit      int
	Offset     int
	Status     string // "approved", "pending", "all"
	FromDate   *time.Time
	City       string
	State      string
	IncludeAll bool // Include all related data
}

// ExportShowsResult contains exported shows with pagination info
type ExportShowsResult struct {
	Shows []ExportedShow `json:"shows"`
	Total int64          `json:"total"`
}

// ExportArtistsParams contains filters for artist export
type ExportArtistsParams struct {
	Limit  int
	Offset int
	Search string
}

// ExportArtistsResult contains exported artists with pagination info
type ExportArtistsResult struct {
	Artists []ExportedArtist `json:"artists"`
	Total   int64            `json:"total"`
}

// ExportVenuesParams contains filters for venue export
type ExportVenuesParams struct {
	Limit    int
	Offset   int
	Search   string
	Verified *bool
	City     string
	State    string
}

// ExportVenuesResult contains exported venues with pagination info
type ExportVenuesResult struct {
	Venues []ExportedVenue `json:"venues"`
	Total  int64           `json:"total"`
}

// DataImportRequest represents a data import request
type DataImportRequest struct {
	Shows   []ExportedShow   `json:"shows,omitempty"`
	Artists []ExportedArtist `json:"artists,omitempty"`
	Venues  []ExportedVenue  `json:"venues,omitempty"`
	DryRun  bool             `json:"dryRun"`
}

// DataImportResult contains statistics about the import operation
type DataImportResult struct {
	Shows struct {
		Total      int      `json:"total"`
		Imported   int      `json:"imported"`
		Duplicates int      `json:"duplicates"`
		Errors     int      `json:"errors"`
		Messages   []string `json:"messages"`
	} `json:"shows"`
	Artists struct {
		Total      int      `json:"total"`
		Imported   int      `json:"imported"`
		Duplicates int      `json:"duplicates"`
		Updated    int      `json:"updated"`
		Errors     int      `json:"errors"`
		Messages   []string `json:"messages"`
	} `json:"artists"`
	Venues struct {
		Total      int      `json:"total"`
		Imported   int      `json:"imported"`
		Duplicates int      `json:"duplicates"`
		Updated    int      `json:"updated"`
		Errors     int      `json:"errors"`
		Messages   []string `json:"messages"`
	} `json:"venues"`
}

// ──────────────────────────────────────────────
// Data Quality types
// ──────────────────────────────────────────────

// DataQualitySummary contains counts per data quality category.
type DataQualitySummary struct {
	Categories []DataQualityCategory `json:"categories"`
	TotalItems int                   `json:"total_items"`
}

// DataQualityCategory represents a single data quality check category.
type DataQualityCategory struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	EntityType  string `json:"entity_type"`
	Count       int    `json:"count"`
	Description string `json:"description"`
}

// DataQualityItem represents a single entity needing attention.
type DataQualityItem struct {
	EntityType string `json:"entity_type"`
	EntityID   uint   `json:"entity_id"`
	Name       string `json:"name"`
	Slug       string `json:"slug"`
	Reason     string `json:"reason"`
	ShowCount  int    `json:"show_count"`
}
