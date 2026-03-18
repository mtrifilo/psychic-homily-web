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
// Activity Feed types
// ──────────────────────────────────────────────

// ActivityEvent represents a single event in the admin activity feed.
type ActivityEvent struct {
	ID          uint      `json:"id"`
	EventType   string    `json:"event_type"`
	Description string    `json:"description"`
	EntityType  string    `json:"entity_type,omitempty"`
	EntitySlug  string    `json:"entity_slug,omitempty"`
	Timestamp   time.Time `json:"timestamp"`
	ActorName   string    `json:"actor_name,omitempty"`
}

// ActivityFeedResponse contains the list of recent activity events.
type ActivityFeedResponse struct {
	Events []ActivityEvent `json:"events"`
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

// ──────────────────────────────────────────────
// Analytics types
// ──────────────────────────────────────────────

// MonthlyCount represents a count for a specific month.
type MonthlyCount struct {
	Month string `json:"month"` // "2026-01", "2026-02", etc.
	Count int    `json:"count"`
}

// GrowthMetricsResponse contains time-series entity growth over N months.
type GrowthMetricsResponse struct {
	Shows    []MonthlyCount `json:"shows"`
	Artists  []MonthlyCount `json:"artists"`
	Venues   []MonthlyCount `json:"venues"`
	Releases []MonthlyCount `json:"releases"`
	Labels   []MonthlyCount `json:"labels"`
	Users    []MonthlyCount `json:"users"`
}

// EngagementMetric represents an engagement count for a specific month.
type EngagementMetric struct {
	Month string `json:"month"`
	Count int    `json:"count"`
}

// EngagementMetricsResponse contains monthly engagement metrics.
type EngagementMetricsResponse struct {
	Bookmarks       []EngagementMetric `json:"bookmarks"`
	TagsAdded       []EngagementMetric `json:"tags_added"`
	TagVotes        []EngagementMetric `json:"tag_votes"`
	CollectionItems []EngagementMetric `json:"collection_items"`
	Requests        []EngagementMetric `json:"requests"`
	RequestVotes    []EngagementMetric `json:"request_votes"`
	Revisions       []EngagementMetric `json:"revisions"`
	Follows         []EngagementMetric `json:"follows"`
	Attendance      []EngagementMetric `json:"attendance"`
}

// TopContributor represents a user ranked by contribution count.
type TopContributor struct {
	UserID      uint   `json:"user_id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name,omitempty"`
	Count       int    `json:"count"`
}

// WeeklyContributions represents contributions for a specific week.
type WeeklyContributions struct {
	Week  string `json:"week"` // "2026-W10"
	Count int    `json:"count"`
}

// CommunityHealthResponse contains community health metrics.
type CommunityHealthResponse struct {
	ActiveContributors30d  int                   `json:"active_contributors_30d"`
	ContributionsPerWeek   []WeeklyContributions `json:"contributions_per_week"`
	RequestFulfillmentRate float64               `json:"request_fulfillment_rate"`
	NewCollections30d      int                   `json:"new_collections_30d"`
	TopContributors        []TopContributor      `json:"top_contributors"`
}

// DataQualityTrendsResponse contains data quality trend metrics over time.
type DataQualityTrendsResponse struct {
	ShowsApproved          []MonthlyCount `json:"shows_approved"`
	ShowsRejected          []MonthlyCount `json:"shows_rejected"`
	PendingReviewCount     int            `json:"pending_review_count"`
	ArtistsWithoutReleases int            `json:"artists_without_releases"`
	InactiveVenues90d      int            `json:"inactive_venues_90d"`
}
