package contracts

import (
	"time"

	adminm "psychic-homily-backend/internal/models/admin"
	authm "psychic-homily-backend/internal/models/auth"
	communitym "psychic-homily-backend/internal/models/community"
)

// ──────────────────────────────────────────────
// Audit Log types
// ──────────────────────────────────────────────

// AuditLogFilters represents optional filters for querying audit logs
type AuditLogFilters struct {
	EntityType string
	Action     string
	ActorID    *uint
}

// AuditLogResponse represents an audit log entry in API responses.
//
// ActorEmail is retained alongside ActorName/ActorUsername for backward
// compatibility with existing frontend consumers; new consumers should
// prefer the resolved name (and the optional /users/:slug link via
// ActorUsername) and treat ActorEmail as deprecated.
type AuditLogResponse struct {
	ID            uint                   `json:"id"`
	ActorID       *uint                  `json:"actor_id"`
	ActorEmail    string                 `json:"actor_email,omitempty"`
	ActorName     string                 `json:"actor_name,omitempty"`
	ActorUsername *string                `json:"actor_username,omitempty"`
	Action        string                 `json:"action"`
	EntityType    string                 `json:"entity_type"`
	EntityID      uint                   `json:"entity_id"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt     time.Time              `json:"created_at"`
}

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

	// Period-over-period trends (current 7 days vs previous 7 days)
	TotalShowsTrend   int64 `json:"total_shows_trend"` // delta: current - previous
	TotalVenuesTrend  int64 `json:"total_venues_trend"`
	TotalArtistsTrend int64 `json:"total_artists_trend"`
	TotalUsersTrend   int64 `json:"total_users_trend"`
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
	ID          uint      `json:"id"`
	Token       string    `json:"token"` // Plaintext token - only shown once!
	Description *string   `json:"description"`
	Scope       string    `json:"scope"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
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

// ──────────────────────────────────────────────
// Show Report Service Interface
// ──────────────────────────────────────────────

// ShowReportServiceInterface defines the contract for show report operations.
type ShowReportServiceInterface interface {
	CreateReport(userID, showID uint, reportType string, details *string) (*ShowReportResponse, error)
	GetUserReportForShow(userID, showID uint) (*ShowReportResponse, error)
	GetPendingReports(limit, offset int) ([]*ShowReportResponse, int64, error)
	DismissReport(reportID, adminID uint, notes *string) (*ShowReportResponse, error)
	ResolveReport(reportID, adminID uint, notes *string) (*ShowReportResponse, error)
	ResolveReportWithFlag(reportID, adminID uint, notes *string, setShowFlag bool) (*ShowReportResponse, error)
	GetReportByID(reportID uint) (*communitym.ShowReport, error)
}

// ──────────────────────────────────────────────
// Artist Report Service Interface
// ──────────────────────────────────────────────

// ArtistReportServiceInterface defines the contract for artist report operations.
type ArtistReportServiceInterface interface {
	CreateReport(userID, artistID uint, reportType string, details *string) (*ArtistReportResponse, error)
	GetUserReportForArtist(userID, artistID uint) (*ArtistReportResponse, error)
	GetPendingReports(limit, offset int) ([]*ArtistReportResponse, int64, error)
	DismissReport(reportID, adminID uint, notes *string) (*ArtistReportResponse, error)
	ResolveReport(reportID, adminID uint, notes *string) (*ArtistReportResponse, error)
	GetReportByID(reportID uint) (*communitym.ArtistReport, error)
}

// ──────────────────────────────────────────────
// Audit Log Service Interface
// ──────────────────────────────────────────────

// AuditLogServiceInterface defines the contract for audit log operations.
type AuditLogServiceInterface interface {
	LogAction(actorID uint, action string, entityType string, entityID uint, metadata map[string]interface{})
	GetAuditLogs(limit, offset int, filters AuditLogFilters) ([]*AuditLogResponse, int64, error)
}

// ──────────────────────────────────────────────
// API Token Service Interface
// ──────────────────────────────────────────────

// APITokenServiceInterface defines the contract for API token operations.
type APITokenServiceInterface interface {
	CreateToken(userID uint, description *string, expirationDays int) (*APITokenCreateResponse, error)
	ValidateToken(plainToken string) (*authm.User, *adminm.APIToken, error)
	ListTokens(userID uint) ([]APITokenResponse, error)
	RevokeToken(userID uint, tokenID uint) error
	GetToken(userID uint, tokenID uint) (*APITokenResponse, error)
	CleanupExpiredTokens() (int64, error)
}

// ──────────────────────────────────────────────
// Data Sync Service Interface
// ──────────────────────────────────────────────

// DataSyncServiceInterface defines the contract for data export/import operations.
type DataSyncServiceInterface interface {
	ExportShows(params ExportShowsParams) (*ExportShowsResult, error)
	ExportArtists(params ExportArtistsParams) (*ExportArtistsResult, error)
	ExportVenues(params ExportVenuesParams) (*ExportVenuesResult, error)
	ImportData(req DataImportRequest) (*DataImportResult, error)
}

// ──────────────────────────────────────────────
// Admin Stats Service Interface
// ──────────────────────────────────────────────

// AdminStatsServiceInterface defines the contract for admin statistics operations.
type AdminStatsServiceInterface interface {
	GetDashboardStats() (*AdminDashboardStats, error)
	GetRecentActivity() (*ActivityFeedResponse, error)
}

// ──────────────────────────────────────────────
// Revision Service Interface
// ──────────────────────────────────────────────

// RevisionServiceInterface defines the contract for revision history operations.
type RevisionServiceInterface interface {
	RecordRevision(entityType string, entityID uint, userID uint, changes []adminm.FieldChange, summary string) error
	GetEntityHistory(entityType string, entityID uint, limit, offset int) ([]adminm.Revision, int64, error)
	GetRevision(revisionID uint) (*adminm.Revision, error)
	GetUserRevisions(userID uint, limit, offset int) ([]adminm.Revision, int64, error)
	Rollback(revisionID uint, adminUserID uint) error
}

// ──────────────────────────────────────────────
// Data Quality Service Interface
// ──────────────────────────────────────────────

// DataQualityServiceInterface defines the contract for data quality dashboard operations.
type DataQualityServiceInterface interface {
	GetSummary() (*DataQualitySummary, error)
	GetCategoryItems(category string, limit, offset int) ([]*DataQualityItem, int64, error)
}

// ──────────────────────────────────────────────
// Analytics Service Interface
// ──────────────────────────────────────────────

// AnalyticsServiceInterface defines the contract for platform analytics dashboard operations.
type AnalyticsServiceInterface interface {
	GetGrowthMetrics(months int) (*GrowthMetricsResponse, error)
	GetEngagementMetrics(months int) (*EngagementMetricsResponse, error)
	GetCommunityHealth() (*CommunityHealthResponse, error)
	GetDataQualityTrends(months int) (*DataQualityTrendsResponse, error)
}
