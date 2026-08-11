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
// Calendar types
// ──────────────────────────────────────────────

// CalendarTokenCreateResponse is returned when a token is created.
// One personal feed token unlocks both the saved-shows iCal URL and the
// followed-artist activity Atom URL (PSY-1430 / PSY-1505).
type CalendarTokenCreateResponse struct {
	Token          string    `json:"token"`
	FeedURL        string    `json:"feed_url"`
	FollowsFeedURL string    `json:"follows_feed_url"`
	CreatedAt      time.Time `json:"created_at"`
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

// LibraryFollowingCounts contains the follow totals surfaced by Library tabs.
// Radio shows are intentionally excluded because they are managed in Radio.
type LibraryFollowingCounts struct {
	Artists   int64 `json:"artists"`
	Venues    int64 `json:"venues"`
	Scenes    int64 `json:"scenes"`
	Labels    int64 `json:"labels"`
	Festivals int64 `json:"festivals"`
	Tags      int64 `json:"tags"`
}

// LibraryFollowingEntityResponse is the non-radio entity shape returned by
// Library pages. Keeping it separate prevents radio-only enrichment fields
// from leaking into generated clients for this endpoint.
type LibraryFollowingEntityResponse struct {
	EntityType string    `json:"entity_type"`
	EntityID   uint      `json:"entity_id"`
	Name       string    `json:"name"`
	Slug       string    `json:"slug"`
	FollowedAt time.Time `json:"followed_at"`
}

// LibraryFollowingCursor is the stable keyset boundary for an alphabetical
// Library page. SortName is the database-normalized name used by ORDER BY.
type LibraryFollowingCursor struct {
	SortName string
	Name     string
	EntityID uint
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
	GetLibraryFollowingCounts(userID uint) (*LibraryFollowingCounts, error)
	GetLibraryFollowing(userID uint, entityType string, limit int, cursor *LibraryFollowingCursor) ([]*LibraryFollowingEntityResponse, *LibraryFollowingCursor, error)
	GetFollowers(entityType string, entityID uint, limit, offset int) ([]*FollowerResponse, int64, error)
}

// ──────────────────────────────────────────────
// Calendar Service Interface
// ──────────────────────────────────────────────

// VenueCalendarFeed is a rendered public iCalendar feed for one venue, plus the
// metadata the HTTP layer needs to serve it: the venue's display name and slug
// (filename), and a content ETag so an aggressively-polling calendar client can
// be answered with a 304 instead of the whole payload.
type VenueCalendarFeed struct {
	VenueName string
	VenueSlug string
	ICS       []byte
	ETag      string
}

// VenueCalendarServiceInterface defines the contract for the PUBLIC, unauthenticated
// per-venue iCalendar feed. It is deliberately separate from
// CalendarServiceInterface: that one is entirely user/token-scoped (a personal
// feed authenticated by an unguessable URL token), while this one takes no
// identity at all and must never grow one. Keeping them apart means a change to
// personal-feed auth cannot silently alter what an anonymous caller can read.
type VenueCalendarServiceInterface interface {
	// GenerateVenueFeed renders upcoming shows at the venue identified by
	// idOrSlug. Returns an error wrapping apperrors.CodeVenueNotFound when the
	// venue does not exist, so the handler can 404 rather than 500.
	GenerateVenueFeed(idOrSlug string, frontendURL string) (*VenueCalendarFeed, error)
}

// SceneCalendarFeed is a rendered public iCalendar feed for one scene, plus the
// only two things the HTTP layer needs to serve it: the scene's slug, for the
// download filename, and a content ETag for the 304 path.
//
// The slug is the CANONICAL scene's, not whatever alias the caller asked for: a
// metro member slug (mesa-az) resolves to its principal city, and a feed that
// echoed the requested alias back would name one calendar's file two ways. The
// scene's display name is deliberately not a field — it is inside the rendered
// calendar, where the only consumer that wants it is already looking.
type SceneCalendarFeed struct {
	SceneSlug string
	ICS       []byte
	ETag      string
}

// SceneCalendarServiceInterface defines the contract for the PUBLIC,
// unauthenticated per-scene iCalendar feed.
//
// Same posture as VenueCalendarServiceInterface and for the same reason: it
// takes no identity at all and must never grow one, so a change to personal-feed
// auth cannot silently alter what an anonymous caller can read. A scene is a
// computed aggregation of public listings, so there is nothing here to scope to
// a user in the first place.
type SceneCalendarServiceInterface interface {
	// GenerateSceneFeed renders the scene's upcoming shows. Returns an error
	// wrapping apperrors.CodeSceneNotFound when the slug names no scene, so the
	// handler can 404 rather than 500.
	GenerateSceneFeed(slug string, frontendURL string) (*SceneCalendarFeed, error)
}

// ShowCalendarEvent is a rendered single-VEVENT iCalendar document for one
// show, plus the metadata the HTTP layer needs to serve it as a download.
type ShowCalendarEvent struct {
	ShowSlug string
	ICS      []byte
	ETag     string
}

// ShowCalendarServiceInterface defines the contract for the PUBLIC,
// unauthenticated per-show iCalendar download — the one-shot "Add to calendar"
// export, as opposed to VenueCalendarServiceInterface's live-updating feed.
// Like that interface, it takes no identity and must never grow one: anything
// a non-approved-show viewer could learn through this endpoint would bypass
// the show handler's access control.
type ShowCalendarServiceInterface interface {
	// GenerateShowEvent renders the show identified by idOrSlug. Returns an
	// error wrapping apperrors.CodeShowNotFound when the show does not exist
	// OR is not approved — an anonymous caller must not be able to tell those
	// apart.
	GenerateShowEvent(idOrSlug string, frontendURL string) (*ShowCalendarEvent, error)
}

// CalendarServiceInterface defines the contract for personal feed-token
// operations (saved-shows iCal + followed-artist Atom activity).
type CalendarServiceInterface interface {
	CreateToken(userID uint, apiBaseURL string) (*CalendarTokenCreateResponse, error)
	GetTokenStatus(userID uint) (*CalendarTokenStatusResponse, error)
	DeleteToken(userID uint) error
	ValidateCalendarToken(plainToken string) (*authm.User, error)
	GenerateICSFeed(userID uint, frontendURL string) ([]byte, error)
	// GenerateFollowsActivityFeed builds an Atom 1.0 feed of recent shows +
	// releases involving artists the user follows (PSY-1505).
	GenerateFollowsActivityFeed(userID uint, frontendURL string) ([]byte, error)
}
