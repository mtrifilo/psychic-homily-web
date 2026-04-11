package contracts

import "time"

// ──────────────────────────────────────────────
// Comment request types
// ──────────────────────────────────────────────

// CreateCommentRequest contains the fields needed to create a comment.
type CreateCommentRequest struct {
	EntityType      string  `json:"entity_type"`
	EntityID        uint    `json:"entity_id"`
	Body            string  `json:"body"`
	ParentID        *uint   `json:"parent_id,omitempty"`
	Kind            string  `json:"kind,omitempty"`             // default "comment"
	ReplyPermission string  `json:"reply_permission,omitempty"` // default "anyone"
}

// UpdateCommentRequest contains the fields that can be updated on a comment.
type UpdateCommentRequest struct {
	Body string `json:"body"`
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
	ID              uint       `json:"id"`
	EntityType      string     `json:"entity_type"`
	EntityID        uint       `json:"entity_id"`
	Kind            string     `json:"kind"`
	UserID          uint       `json:"user_id"`
	AuthorName      string     `json:"author_name"`
	AuthorUsername  string     `json:"author_username,omitempty"`
	ParentID        *uint      `json:"parent_id,omitempty"`
	RootID          *uint      `json:"root_id,omitempty"`
	Depth           int        `json:"depth"`
	Body            string     `json:"body"`
	BodyHTML        string     `json:"body_html"`
	Visibility      string     `json:"visibility"`
	ReplyPermission string     `json:"reply_permission"`
	Ups             int        `json:"ups"`
	Downs           int        `json:"downs"`
	Score           float64    `json:"score"`
	IsEdited        bool       `json:"is_edited"`
	EditCount       int        `json:"edit_count"`
	UserVote        *int       `json:"user_vote,omitempty"` // 1, -1, or nil if no vote
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
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
	DeleteComment(userID uint, commentID uint, isAdmin bool) error
}
