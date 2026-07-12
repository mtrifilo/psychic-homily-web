package contracts

import (
	"time"

	authm "psychic-homily-backend/internal/models/auth"
	engagementm "psychic-homily-backend/internal/models/engagement"
)

// ──────────────────────────────────────────────
// Saved Show types
// ──────────────────────────────────────────────

// SavedShowResponse represents a saved show with metadata
type SavedShowResponse struct {
	ShowResponse
	SavedAt time.Time `json:"saved_at"`
}

// SavedReleaseResponse represents a release saved by a user. Releases retain
// the historical `bookmark` storage action internally, but every public API
// and UI surface calls the relationship Save/Saved.
type SavedReleaseResponse struct {
	ReleaseListResponse
	SavedAt time.Time `json:"saved_at"`
}

// ──────────────────────────────────────────────
// Show Report types
// ──────────────────────────────────────────────

// ShowReportResponse represents a show report response with show info
type ShowReportResponse struct {
	ID         uint      `json:"id"`
	ShowID     uint      `json:"show_id"`
	ReportType string    `json:"report_type"`
	Details    *string   `json:"details"`
	Status     string    `json:"status"`
	AdminNotes *string   `json:"admin_notes,omitempty"`
	ReviewedBy *uint     `json:"reviewed_by,omitempty"`
	ReviewedAt *string   `json:"reviewed_at,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`

	// Show info (for admin view)
	Show *ShowReportShowInfo `json:"show,omitempty"`
}

// ShowReportShowInfo contains show information for report responses
type ShowReportShowInfo struct {
	ID        uint      `json:"id"`
	Title     string    `json:"title"`
	Slug      string    `json:"slug"`
	EventDate time.Time `json:"event_date"`
	City      *string   `json:"city"`
	State     *string   `json:"state"`
}

// ──────────────────────────────────────────────
// Artist Report types
// ──────────────────────────────────────────────

// ArtistReportResponse represents an artist report response with artist info
type ArtistReportResponse struct {
	ID         uint      `json:"id"`
	ArtistID   uint      `json:"artist_id"`
	ReportType string    `json:"report_type"`
	Details    *string   `json:"details"`
	Status     string    `json:"status"`
	AdminNotes *string   `json:"admin_notes,omitempty"`
	ReviewedBy *uint     `json:"reviewed_by,omitempty"`
	ReviewedAt *string   `json:"reviewed_at,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`

	// Artist info (for admin view)
	Artist *ArtistReportArtistInfo `json:"artist,omitempty"`
}

// ArtistReportArtistInfo contains artist information for report responses
type ArtistReportArtistInfo struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// ──────────────────────────────────────────────
// Calendar types
// ──────────────────────────────────────────────

// CalendarTokenCreateResponse is returned when a token is created
type CalendarTokenCreateResponse struct {
	Token     string    `json:"token"`
	FeedURL   string    `json:"feed_url"`
	CreatedAt time.Time `json:"created_at"`
}

// CalendarTokenStatusResponse is returned for token status checks
type CalendarTokenStatusResponse struct {
	HasToken  bool       `json:"has_token"`
	CreatedAt *time.Time `json:"created_at,omitempty"`
}

// ──────────────────────────────────────────────
// Follow types
// ──────────────────────────────────────────────

// FollowingEntityResponse represents an entity a user is following.
type FollowingEntityResponse struct {
	EntityType string    `json:"entity_type"`
	EntityID   uint      `json:"entity_id"`
	Name       string    `json:"name"`
	Slug       string    `json:"slug"`
	FollowedAt time.Time `json:"followed_at"`

	// Radio-show-only enriched fields (PSY-1356), nil for every other entity
	// type — additive so the base shape is unchanged. StationSlug is required
	// to build the two-segment radio href /radio/{station_slug}/{show_slug}
	// (the base Slug carries the show slug). LastEpisodeDate is the show's most
	// recent radio_episodes.air_date.
	StationName     *string `json:"station_name,omitempty"`
	StationSlug     *string `json:"station_slug,omitempty"`
	HostName        *string `json:"host_name,omitempty"`
	LastEpisodeDate *string `json:"last_episode_date,omitempty"`
}

// FollowerResponse represents a follower of an entity.
type FollowerResponse struct {
	UserID      uint   `json:"user_id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name,omitempty"`
}

// ──────────────────────────────────────────────
// Saved Show Service Interface
// ──────────────────────────────────────────────

// SavedShowServiceInterface defines the contract for saved show operations.
type SavedShowServiceInterface interface {
	SaveShow(userID, showID uint) error
	UnsaveShow(userID, showID uint) error
	// GetUserSavedShows lists a user's saved shows. timeFilter "" preserves the
	// original ordering (most recently saved first); "upcoming"/"past" partition
	// by the show's venue-local event date and order by event date (ASC/DESC).
	GetUserSavedShows(userID uint, limit, offset int, timeFilter string) ([]*SavedShowResponse, int64, error)
	IsShowSaved(userID, showID uint) (bool, error)
	GetSavedShowIDs(userID uint, showIDs []uint) (map[uint]bool, error)
	GetSaveCount(showID uint) (int, error)
	GetBatchSaveCounts(showIDs []uint) (map[uint]int, error)
}

// SavedReleaseServiceInterface defines the release-save surface. It mirrors
// saved shows while keeping release bookmarks behind a release-specific
// boundary, so callers never need to know the legacy storage action.
type SavedReleaseServiceInterface interface {
	SaveRelease(userID, releaseID uint) error
	UnsaveRelease(userID, releaseID uint) error
	GetUserSavedReleases(userID uint, limit, offset int) ([]*SavedReleaseResponse, int64, error)
	IsReleaseSaved(userID, releaseID uint) (bool, error)
	GetSavedReleaseIDs(userID uint, releaseIDs []uint) (map[uint]bool, error)
	GetSaveCount(releaseID uint) (int, error)
	GetBatchSaveCounts(releaseIDs []uint) (map[uint]int, error)
}

// ──────────────────────────────────────────────
// Bookmark Service Interface
// ──────────────────────────────────────────────

// BookmarkServiceInterface defines the contract for generic bookmark operations.
type BookmarkServiceInterface interface {
	CreateBookmark(userID uint, entityType engagementm.BookmarkEntityType, entityID uint, action engagementm.BookmarkAction) error
	DeleteBookmark(userID uint, entityType engagementm.BookmarkEntityType, entityID uint, action engagementm.BookmarkAction) error
	IsBookmarked(userID uint, entityType engagementm.BookmarkEntityType, entityID uint, action engagementm.BookmarkAction) (bool, error)
	GetBookmarkedEntityIDs(userID uint, entityType engagementm.BookmarkEntityType, action engagementm.BookmarkAction, entityIDs []uint) (map[uint]bool, error)
	GetUserBookmarks(userID uint, entityType engagementm.BookmarkEntityType, action engagementm.BookmarkAction, limit, offset int) ([]engagementm.UserBookmark, int64, error)
	GetUserBookmarksByEntityType(userID uint, entityType engagementm.BookmarkEntityType, action engagementm.BookmarkAction) ([]engagementm.UserBookmark, error)
	CountUserBookmarks(userID uint, entityType engagementm.BookmarkEntityType, action engagementm.BookmarkAction) (int64, error)
}

// ──────────────────────────────────────────────
// Follow Service Interface
// ──────────────────────────────────────────────

// FollowServiceInterface defines the contract for entity follow operations.
type FollowServiceInterface interface {
	Follow(userID uint, entityType string, entityID uint) error
	Unfollow(userID uint, entityType string, entityID uint) error
	IsFollowing(userID uint, entityType string, entityID uint) (bool, error)
	GetFollowerCount(entityType string, entityID uint) (int64, error)
	// Scene-follow notify mode (PSY-1341): "all" (default) or
	// "followed_bands_only", stored on the follow row's settings JSONB.
	SetSceneNotifyMode(userID uint, sceneID uint, mode string) error
	SceneNotifyMode(userID uint, sceneID uint) (string, error)
	GetBatchFollowerCounts(entityType string, entityIDs []uint) (map[uint]int64, error)
	GetBatchUserFollowing(userID uint, entityType string, entityIDs []uint) (map[uint]bool, error)
	GetUserFollowing(userID uint, entityType string, limit, offset int) ([]*FollowingEntityResponse, int64, error)
	GetFollowers(entityType string, entityID uint, limit, offset int) ([]*FollowerResponse, int64, error)
}

// ──────────────────────────────────────────────
// Calendar Service Interface
// ──────────────────────────────────────────────

// CalendarServiceInterface defines the contract for calendar feed operations.
type CalendarServiceInterface interface {
	CreateToken(userID uint, apiBaseURL string) (*CalendarTokenCreateResponse, error)
	GetTokenStatus(userID uint) (*CalendarTokenStatusResponse, error)
	DeleteToken(userID uint) error
	ValidateCalendarToken(plainToken string) (*authm.User, error)
	GenerateICSFeed(userID uint, frontendURL string) ([]byte, error)
}
