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
type UpdateCommentRequest struct {
	Body string `json:"body"`
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
	ShowArtistID       *uint   `json:"show_artist_id,omitempty"`
	SongPosition       *int    `json:"song_position,omitempty"`
	SoundQuality       *int    `json:"sound_quality,omitempty"`
	CrowdEnergy        *int    `json:"crowd_energy,omitempty"`
	NotableMoments     *string `json:"notable_moments,omitempty"`
	SetlistSpoiler     bool    `json:"setlist_spoiler"`
	IsVerifiedAttendee bool    `json:"is_verified_attendee"`
}

// CommentListFilters defines filtering and sorting options for listing comments.
type CommentListFilters struct {
	Sort       string // best, new, top, controversial
	Visibility string // visible, hidden_by_user, hidden_by_mod, pending_review, or empty for visible only
	Kind       string // comment, field_note, or empty for all
	Limit      int
	Offset     int
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

// SubscriptionResponse represents a user's subscription to an entity with unread count.
type SubscriptionResponse struct {
	EntityType   string    `json:"entity_type"`
	EntityID     uint      `json:"entity_id"`
	SubscribedAt time.Time `json:"subscribed_at"`
	UnreadCount  int       `json:"unread_count"`
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

	// GetSubscriptionsForUser returns paginated subscriptions with unread counts.
	GetSubscriptionsForUser(userID uint, limit, offset int) ([]SubscriptionResponse, int64, error)

	// GetSubscribersForEntity returns user IDs of all subscribers for an entity.
	GetSubscribersForEntity(entityType string, entityID uint) ([]uint, error)
}
