package contracts

import (
	"encoding/json"
	catalogm "psychic-homily-backend/internal/models/catalog"
	notificationm "psychic-homily-backend/internal/models/notification"
	"time"
)

// ──────────────────────────────────────────────
// Notification Filter types
// ──────────────────────────────────────────────

// CreateFilterInput describes the fields needed to create a notification filter.
type CreateFilterInput struct {
	Name          string
	ArtistIDs     []int64
	VenueIDs      []int64
	LabelIDs      []int64
	TagIDs        []int64
	ExcludeTagIDs []int64
	Cities        json.RawMessage // [{city, state}]
	PriceMaxCents *int
	NotifyEmail   bool
	NotifyInApp   bool
}

// UpdateFilterInput describes the fields that can be updated on a filter.
type UpdateFilterInput struct {
	Name          *string
	IsActive      *bool
	ArtistIDs     *[]int64
	VenueIDs      *[]int64
	LabelIDs      *[]int64
	TagIDs        *[]int64
	ExcludeTagIDs *[]int64
	Cities        *json.RawMessage
	PriceMaxCents *int
	NotifyEmail   *bool
	NotifyInApp   *bool
}

// NotificationFilterResponse represents a notification filter in API responses.
type NotificationFilterResponse struct {
	ID            uint             `json:"id"`
	Name          string           `json:"name"`
	Source        string           `json:"source"` // "user" | "managed" (PSY-1467)
	IsActive      bool             `json:"is_active"`
	ArtistIDs     []int64          `json:"artist_ids,omitempty"`
	VenueIDs      []int64          `json:"venue_ids,omitempty"`
	LabelIDs      []int64          `json:"label_ids,omitempty"`
	TagIDs        []int64          `json:"tag_ids,omitempty"`
	ExcludeTagIDs []int64          `json:"exclude_tag_ids,omitempty"`
	Cities        *json.RawMessage `json:"cities,omitempty"`
	PriceMaxCents *int             `json:"price_max_cents,omitempty"`
	NotifyEmail   bool             `json:"notify_email"`
	NotifyInApp   bool             `json:"notify_in_app"`
	NotifyPush    bool             `json:"notify_push"`
	MatchCount    int              `json:"match_count"`
	LastMatchedAt *time.Time       `json:"last_matched_at,omitempty"`
	CreatedAt     time.Time        `json:"created_at"`
	UpdatedAt     time.Time        `json:"updated_at"`
}

// NotificationLogEntry represents a notification log entry in API responses.
//
// For show-filter rows (entity_type="show", channel="email"), the FilterName,
// EntityType, and EntityID are the only identifying fields the frontend
// needs.
//
// For comment-driven in-app rows (entity_type="comment_reply" or
// "comment_mention", channel="in_app"), entity_id holds the comment_id. The
// service layer enriches each row with the comment's source entity
// (CommentEntityType / CommentEntityID / CommentEntityName / CommentEntityURL),
// the commenter's display name (CommenterName), and a plain-text excerpt
// (CommentExcerpt). The frontend uses these to render a readable popover /
// inbox row + link target (CommentURL).
//
// Enriched fields are JSON-omitempty so legacy clients receiving show-filter
// rows see the same shape as before.
type NotificationLogEntry struct {
	ID         uint       `json:"id"`
	FilterID   *uint      `json:"filter_id,omitempty"`
	FilterName string     `json:"filter_name,omitempty"`
	EntityType string     `json:"entity_type"`
	EntityID   uint       `json:"entity_id"`
	Channel    string     `json:"channel"`
	SentAt     time.Time  `json:"sent_at"`
	ReadAt     *time.Time `json:"read_at,omitempty"`

	// SubjectEntityID is the FOLLOWED entity whose subscription produced the
	// row, where EntityID is what the row is about. Set only on follow-driven
	// alerts; its type is implied by EntityType. PSY-1896.
	SubjectEntityID *uint `json:"subject_entity_id,omitempty"`

	// Comment-driven enrichment fields (populated only for comment_reply /
	// comment_mention rows). PSY-595.
	CommenterName     string `json:"commenter_name,omitempty"`
	CommenterUsername string `json:"commenter_username,omitempty"`
	CommentExcerpt    string `json:"comment_excerpt,omitempty"`
	CommentURL        string `json:"comment_url,omitempty"`
	CommentEntityType string `json:"comment_entity_type,omitempty"`
	CommentEntityID   uint   `json:"comment_entity_id,omitempty"`
	CommentEntityName string `json:"comment_entity_name,omitempty"`

	// Request-driven enrichment fields (populated only for
	// request_fulfillment_proposed rows; entity_id holds the request_id). The
	// requester is notified that a fulfillment awaits their approval. PSY-890.
	RequestTitle string `json:"request_title,omitempty"`
	RequestURL   string `json:"request_url,omitempty"`

	// Artist show-alert enrichment fields (populated only for
	// artist_show_alert rows; entity_id holds the show_id and
	// subject_entity_id the followed artist). PSY-1896.
	//
	// AlertShowURL is absolute and same-origin-relativized by the frontend
	// before use, like CommentURL and RequestURL: a full navigation would
	// cancel the row's mark-read request.
	AlertArtistName string `json:"alert_artist_name,omitempty"`
	AlertShowTitle  string `json:"alert_show_title,omitempty"`
	AlertShowURL    string `json:"alert_show_url,omitempty"`
	// AlertShowVenueName and AlertShowEventDate name where and when the
	// announced show happens. A show booked at several venues names the one
	// with the lowest id, so the choice is stable across reads.
	AlertShowVenueName string     `json:"alert_show_venue_name,omitempty"`
	AlertShowEventDate *time.Time `json:"alert_show_event_date,omitempty"`

	// AlertShowTimezone is the IANA zone every show date on this row reads in:
	// the show's venue on an artist alert, the followed venue on a venue alert.
	// Resolved server-side through utils.EventLocation, so a client formats
	// dates in it without restating the state fallback.
	AlertShowTimezone string `json:"alert_show_timezone,omitempty"`

	// AlertBucket is the calendar day a COALESCED alert covers, as YYYY-MM-DD,
	// and it is part of that alert's identity rather than a timestamp about it:
	// the same venue's alert tomorrow is a different notification. Empty on
	// every per-event row. PSY-1895.
	AlertBucket string `json:"alert_bucket,omitempty"`

	// Venue show-alert enrichment fields (populated only for venue_show_alert
	// rows; entity_id holds the VENUE id and subject_entity_id is NULL). The
	// shows are resolved from venue_show_alert_batch at READ time rather than
	// stamped on the row, which is what lets a show announced later the same day
	// appear in a row that was already delivered. PSY-1895.
	//
	// AlertVenueURL is absolute and same-origin-relativized by the frontend
	// before use, like CommentURL, RequestURL and AlertShowURL: a full
	// navigation would cancel the row's mark-read request.
	AlertVenueName string `json:"alert_venue_name,omitempty"`
	AlertVenueURL  string `json:"alert_venue_url,omitempty"`
	// AlertShowCount is the batch's FULL size, which is not always the number of
	// shows named in AlertShowSummary — the summary is capped.
	AlertShowCount int `json:"alert_show_count,omitempty"`
	// AlertShowFirstDate and AlertShowLastDate bound the event dates of the
	// shows AlertShowCount counts (the same member set, after the same
	// filters). Equal when the batch holds one show; absent when it holds none.
	AlertShowFirstDate *time.Time `json:"alert_show_first_date,omitempty"`
	AlertShowLastDate  *time.Time `json:"alert_show_last_date,omitempty"`
	// AlertShowSummary is a short, already-joined preview of the batch's shows,
	// server-side capped so one busy venue-day cannot produce an inbox row
	// hundreds of entries long.
	AlertShowSummary string `json:"alert_show_summary,omitempty"`

	// Artist release-digest enrichment fields, populated only for
	// artist_release_digest rows (entity_id holds the USER id). Resolved from
	// artist_release_alert_batch at read time rather than stamped on the row, so
	// a member that reaches a delivered week (a boundary commit, an artist merge)
	// appears in its row, and the reader's current follows and release-alert
	// preferences narrow it.
	//
	// AlertReleaseCount is the number of distinct releases in the roundup for
	// this reader.
	AlertReleaseCount int `json:"alert_release_count,omitempty"`
	// AlertReleaseArtistCount is the number of distinct followed artists those
	// releases are credited to. A full count, unlike AlertReleaseArtists.
	AlertReleaseArtistCount int `json:"alert_release_artist_count,omitempty"`
	// AlertReleaseArtists names those artists in name order, already joined
	// ("Dan Deacon, Primitive Man, Russian Circles") and capped server-side,
	// ending in "and N more" when it names fewer than AlertReleaseArtistCount.
	AlertReleaseArtists string `json:"alert_release_artists,omitempty"`
	// AlertReleaseURL is where the row links: absolute (the frontend relativizes
	// same-origin links), and set ONLY when the roundup covers exactly one
	// record. A multi-record roundup has no single page, so it is left empty and
	// the frontend uses its own fallback.
	AlertReleaseURL string `json:"alert_release_url,omitempty"`
}

// NotificationFilterServiceInterface defines the contract for notification filter operations.
type NotificationFilterServiceInterface interface {
	// CRUD
	CreateFilter(userID uint, input CreateFilterInput) (*notificationm.NotificationFilter, error)
	UpdateFilter(userID uint, filterID uint, input UpdateFilterInput) (*notificationm.NotificationFilter, error)
	DeleteFilter(userID uint, filterID uint) error
	GetUserFilters(userID uint) ([]notificationm.NotificationFilter, error)
	GetFilter(userID uint, filterID uint) (*notificationm.NotificationFilter, error)

	// Quick create from entity
	QuickCreateFilter(userID uint, entityType string, entityID uint) (*notificationm.NotificationFilter, error)

	// Matching
	MatchAndNotify(show *catalogm.Show) error
	MatchAndNotifyBatch(shows []catalogm.Show) error

	// Notification log
	//
	// All four take a ShowViewer rather than a user id. The inbox is
	// self-scoped, so the viewer names both the account whose rows these are
	// and the tier they are read at (PSY-1983), and passing one value rather
	// than a user id beside a viewer carrying the same id keeps the two from
	// disagreeing. Rows leading to a show the viewer may not see are dropped
	// from the list, the unread count and both mark-read writes together, so no
	// pair of them can be differenced. inboxRowsVisibleTo is the authority on
	// WHICH rows those are — today the comment replies and mentions, the artist
	// show alerts, and the show-filter / scene-follow rows that carry a show id
	// directly — and restating its list here is how the two drift apart.
	//
	// All four REJECT the zero viewer. ShowViewer{} is the deliberate spelling
	// for the public tier on the listing gates, and these are self-scoped, so
	// the idiom that is correct there would mean "user 0" here; they error
	// rather than answer emptily.
	//
	// GetUserNotifications' entityTypes, when given, narrows the list to those
	// discriminators; the unread count stays global regardless.
	GetUserNotifications(viewer ShowViewer, limit, offset int, entityTypes ...string) ([]NotificationLogEntry, error)
	GetUnreadCount(viewer ShowViewer) (int64, error)
	// MarkNotificationsRead flips read_at on the given IDs that belong to the
	// viewer. Returns the count actually updated; already-read, gated-show and
	// not-owned-by-viewer IDs are skipped silently. NOT filtered by
	// inboxVisibleRows, so an explicitly-named email-lane id still flips — a
	// pre-existing asymmetry, named here rather than papered over. PSY-595.
	MarkNotificationsRead(viewer ShowViewer, ids []uint) (int64, error)
	// MarkAllNotificationsRead flips read_at on every unread notification
	// the viewer can see. Returns the count updated. PSY-595.
	MarkAllNotificationsRead(viewer ShowViewer) (int64, error)

	// Unsubscribe
	PauseFilter(filterID uint) error
}
