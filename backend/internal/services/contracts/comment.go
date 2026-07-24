package contracts

import (
	"encoding/json"
	"time"
)

// ──────────────────────────────────────────────
// Comment request types
// ──────────────────────────────────────────────

// CreateCommentRequest contains the fields needed to create a comment.
type CreateCommentRequest struct {
	EntityType      string `json:"entity_type"`
	EntityID        uint   `json:"entity_id"`
	Body            string `json:"body"`
	ParentID        *uint  `json:"parent_id,omitempty"`
	Kind            string `json:"kind,omitempty"`             // default "comment"
	ReplyPermission string `json:"reply_permission,omitempty"` // default "anyone"
}

// UpdateCommentRequest contains the fields that can be updated on a comment.
//
// `StructuredData` is optional and only meaningful when the target comment is
// a field note (PSY-567). When supplied AND the target is a field note, it
// REPLACES the existing structured_data row atomically with the body update —
// ratings, spoiler are edited as a single unit, never merged with stored
// values. On a regular comment the field is ignored.
// On a field-note edit it is optional: nil leaves the existing structured_data
// untouched (body-only edit still works).
type UpdateCommentRequest struct {
	Body           string                   `json:"body"`
	StructuredData *FieldNoteStructuredData `json:"structured_data,omitempty"`
}

// CreateFieldNoteRequest contains the fields needed to create a field note on a show.
type CreateFieldNoteRequest struct {
	ShowID         uint    `json:"show_id"`
	Body           string  `json:"body"`
	ShowArtistID   *uint   `json:"show_artist_id,omitempty"`
	SongPosition   *int    `json:"song_position,omitempty"`
	SoundQuality   *int    `json:"sound_quality,omitempty"`
	CrowdEnergy    *int    `json:"crowd_energy,omitempty"`
	NotableMoments *string `json:"notable_moments,omitempty"`
	SetlistSpoiler bool    `json:"setlist_spoiler"`
}

// FieldNoteStructuredData represents the JSONB structured data stored with field note comments.
type FieldNoteStructuredData struct {
	ShowArtistID   *uint   `json:"show_artist_id,omitempty"`
	SongPosition   *int    `json:"song_position,omitempty"`
	SoundQuality   *int    `json:"sound_quality,omitempty"`
	CrowdEnergy    *int    `json:"crowd_energy,omitempty"`
	NotableMoments *string `json:"notable_moments,omitempty"`
	SetlistSpoiler bool    `json:"setlist_spoiler"`
}

// CommentListFilters defines filtering and sorting options for listing comments.
type CommentListFilters struct {
	Sort       string // best, new, top, controversial
	Visibility string // visible, hidden_by_user, hidden_by_mod, pending_review, or empty for visible only
	// Kind: "comment", "field_note", or empty for the default ("comment").
	// Field notes have a dedicated `/shows/{id}/field-notes` endpoint and must
	// never leak into the discussion list (PSY-588). Callers that legitimately
	// need both must request each kind separately.
	Kind   string
	Limit  int
	Offset int
}

// ──────────────────────────────────────────────
// Comment response types
// ──────────────────────────────────────────────

// CommentResponse represents a comment with author info for API responses.
type CommentResponse struct {
	ID         uint   `json:"id"`
	EntityType string `json:"entity_type"`
	EntityID   uint   `json:"entity_id"`
	Kind       string `json:"kind"`
	UserID     uint   `json:"user_id"`
	// AuthorName is the resolved display name for the comment's author —
	// never empty. Resolution chain mirrors PSY-353: username → first/last
	// → email-prefix → "Anonymous".
	AuthorName string `json:"author_name"`
	// AuthorUsername is the author's username when set — used by the
	// frontend to link the byline to /users/:username. Pointer so the JSON
	// encodes null (not "") for accounts that never set a username, the
	// same shape PSY-353 standardized for collection contributor
	// attribution. PSY-552.
	AuthorUsername  *string          `json:"author_username"`
	ParentID        *uint            `json:"parent_id,omitempty"`
	RootID          *uint            `json:"root_id,omitempty"`
	Depth           int              `json:"depth"`
	Body            string           `json:"body"`
	BodyHTML        string           `json:"body_html"`
	StructuredData  *json.RawMessage `json:"structured_data,omitempty"`
	Visibility      string           `json:"visibility"`
	ReplyPermission string           `json:"reply_permission"`
	Ups             int              `json:"ups"`
	Downs           int              `json:"downs"`
	Score           float64          `json:"score"`
	IsEdited        bool             `json:"is_edited"`
	EditCount       int              `json:"edit_count"`
	// ReplyCount is the number of direct replies (depth = depth+1, parent_id = this).
	// Currently populated only by ListCommentsForEntity for top-level comments so
	// the UI can suppress an "expand replies" affordance on zero-reply threads
	// (PSY-514). Other paths leave this at the zero value.
	ReplyCount int       `json:"reply_count"`
	UserVote   *int      `json:"user_vote,omitempty"` // 1, -1, or nil if no vote
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// CommentListResponse wraps a list of comments with pagination metadata.
type CommentListResponse struct {
	Comments []*CommentResponse `json:"comments"`
	Total    int64              `json:"total"`
	HasMore  bool               `json:"has_more"`
}

// AuthoredFieldNote is a field note listed on its author's public profile
// (PSY-1046). Show title/slug are enriched so the frontend can render a
// linked "note on <show>" line without re-fetching each show.
type AuthoredFieldNote struct {
	CommentResponse
	ShowTitle string `json:"show_title"`
	ShowSlug  string `json:"show_slug,omitempty"`
}

// ──────────────────────────────────────────────
// Comment service interface
// ──────────────────────────────────────────────

// CommentServiceInterface defines the contract for comment CRUD and threading.
type CommentServiceInterface interface {
	CreateComment(userID uint, req *CreateCommentRequest) (*CommentResponse, error)
	GetComment(commentID uint) (*CommentResponse, error)
	ListCommentsForEntity(entityType string, entityID uint, filters CommentListFilters) (*CommentListResponse, error)
	GetThread(rootID uint) ([]*CommentResponse, error)
	UpdateComment(userID uint, commentID uint, req *UpdateCommentRequest) (*CommentResponse, error)
	UpdateReplyPermission(userID uint, commentID uint, permission string) (*CommentResponse, error)
	DeleteComment(userID uint, commentID uint, isAdmin bool) error
}

// FieldNoteServiceInterface defines the contract for field note operations on shows.
type FieldNoteServiceInterface interface {
	CreateFieldNote(userID uint, req *CreateFieldNoteRequest) (*CommentResponse, error)
	ListFieldNotesForShow(showID uint, limit, offset int) (*CommentListResponse, error)
	ListFieldNotesByAuthor(userID uint, limit, offset int) ([]*AuthoredFieldNote, int64, error)
}

// ──────────────────────────────────────────────
// Comment admin service interface
// ──────────────────────────────────────────────

// CommentEditHistoryEntry represents a single historical edit of a comment.
type CommentEditHistoryEntry struct {
	ID             uint      `json:"id"`
	CommentID      uint      `json:"comment_id"`
	OldBody        string    `json:"old_body"`
	EditedAt       time.Time `json:"edited_at"`
	EditorUserID   *uint     `json:"editor_user_id,omitempty"`
	EditorName     string    `json:"editor_name,omitempty"`
	EditorUsername string    `json:"editor_username,omitempty"`
}

// CommentEditHistoryResponse wraps the current comment body plus its edit history.
// Entries are ordered oldest-first for chronological walkback.
type CommentEditHistoryResponse struct {
	CommentID   uint                      `json:"comment_id"`
	CurrentBody string                    `json:"current_body"`
	Edits       []CommentEditHistoryEntry `json:"edits"`
}

// CommentAdminServiceInterface defines the contract for comment moderation operations.
type CommentAdminServiceInterface interface {
	// HideComment hides a comment with a reason (admin action).
	HideComment(adminUserID uint, commentID uint, reason string) error

	// RestoreComment restores a hidden comment to visible (admin action).
	RestoreComment(adminUserID uint, commentID uint) error

	// ListPendingComments returns comments with pending_review visibility.
	ListPendingComments(limit, offset int) ([]*CommentResponse, int64, error)

	// ApproveComment approves a pending comment (sets visibility to visible).
	ApproveComment(adminUserID uint, commentID uint) error

	// RejectComment rejects a pending comment (sets visibility to hidden_by_mod).
	RejectComment(adminUserID uint, commentID uint, reason string) error

	// GetCommentEditHistory returns the chronological edit history for a comment.
	// Admin-only: returns an "admin access required" error for non-admin requesters.
	GetCommentEditHistory(requesterID uint, commentID uint) (*CommentEditHistoryResponse, error)
}

// CommentVoteResponse contains vote counts and the current user's vote.
type CommentVoteResponse struct {
	Ups      int     `json:"ups"`
	Downs    int     `json:"downs"`
	Score    float64 `json:"score"`
	UserVote *int    `json:"user_vote"` // 1, -1, or null
}

// ──────────────────────────────────────────────
// Comment Vote service interface
// ──────────────────────────────────────────────

// CommentVoteServiceInterface defines the contract for comment voting operations.
type CommentVoteServiceInterface interface {
	// Vote casts or updates a vote on a comment.
	// direction must be 1 (upvote) or -1 (downvote).
	Vote(userID uint, commentID uint, direction int) error

	// Unvote removes a user's vote on a comment.
	Unvote(userID uint, commentID uint) error

	// GetUserVote returns the user's vote direction (1 or -1) or nil if not voted.
	GetUserVote(userID uint, commentID uint) (*int, error)

	// GetUserVotesForComments returns a map of commentID→direction for batch lookups.
	GetUserVotesForComments(userID uint, commentIDs []uint) (map[uint]int, error)

	// GetCommentVoteCounts returns the current ups, downs, and Wilson score for a comment.
	GetCommentVoteCounts(commentID uint) (int, int, float64, error)
}

// ──────────────────────────────────────────────
// Comment subscription response types
// ──────────────────────────────────────────────

// WatchingItem is one row of a user's comment-thread watch list: the
// subscription plus entity context and thread activity.
type WatchingItem struct {
	EntityType        string     `json:"entity_type" doc:"Entity type (artist, venue, show, release, label, festival, collection)"`
	EntityID          uint       `json:"entity_id" doc:"Entity ID"`
	EntityName        string     `json:"entity_name" doc:"Entity display name (falls back to '<type> #<id>' when the entity row is gone)"`
	EntitySlug        string     `json:"entity_slug,omitempty" required:"false" doc:"Entity slug, empty when the entity has none"`
	EntityURL         string     `json:"entity_url" doc:"Root-relative frontend path for the entity (slug when present, ID otherwise)"`
	SubscribedAt      time.Time  `json:"subscribed_at" doc:"When the user subscribed"`
	CommentCount      int        `json:"comment_count" doc:"Number of visible comments on the entity"`
	LastCommentAt     *time.Time `json:"last_comment_at,omitempty" required:"false" doc:"Timestamp of the most recent visible comment, null when the thread is empty"`
	LastCommenterName string     `json:"last_commenter_name,omitempty" required:"false" doc:"Display name of the most recent commenter, empty when the thread is empty"`
	UnreadCount       int        `json:"unread_count" doc:"Visible comments (any kind) newer than the user's last-read marker; matches the subscribe/status badge"`
}

// WatchingListResponse is the paginated watch-list payload.
type WatchingListResponse struct {
	Items  []WatchingItem `json:"items" doc:"Watch-list rows ordered by last comment activity, newest first"`
	Total  int64          `json:"total" doc:"Total subscription count for the user (for the watching tab label)"`
	Limit  int            `json:"limit" doc:"Page size used"`
	Offset int            `json:"offset" doc:"Offset used"`
}

// SubscriptionStatusResponse represents subscription status and unread count for an entity.
type SubscriptionStatusResponse struct {
	Subscribed  bool `json:"subscribed"`
	UnreadCount int  `json:"unread_count"`
}

// ──────────────────────────────────────────────
// Comment subscription service interface
// ──────────────────────────────────────────────

// CommentSubscriptionServiceInterface defines the contract for comment subscription operations.
type CommentSubscriptionServiceInterface interface {
	// Subscribe adds a subscription for a user to an entity's comments.
	Subscribe(userID uint, entityType string, entityID uint) error

	// Unsubscribe removes a subscription for a user from an entity's comments.
	Unsubscribe(userID uint, entityType string, entityID uint) error

	// IsSubscribed checks whether a user is subscribed to an entity's comments.
	IsSubscribed(userID uint, entityType string, entityID uint) (bool, error)

	// MarkRead updates the last-read pointer for a user on an entity to the latest comment.
	MarkRead(userID uint, entityType string, entityID uint) error

	// GetUnreadCount returns the number of unread comments for a user on an entity.
	GetUnreadCount(userID uint, entityType string, entityID uint) (int, error)

	// ListWatching returns the user's subscriptions enriched with entity
	// context and last comment activity, ordered by last activity (newest
	// first), plus the total subscription count.
	ListWatching(userID uint, limit, offset int) ([]WatchingItem, int64, error)

	// GetSubscribersForEntity returns user IDs of all subscribers for an entity.
	GetSubscribersForEntity(entityType string, entityID uint) ([]uint, error)
}
