package models

import (
	"encoding/json"
	"time"
)

// CommentKind represents the type of comment
type CommentKind string

const (
	CommentKindComment   CommentKind = "comment"
	CommentKindFieldNote CommentKind = "field_note"
)

// CommentVisibility represents the visibility state of a comment
type CommentVisibility string

const (
	CommentVisibilityVisible      CommentVisibility = "visible"
	CommentVisibilityHiddenByUser CommentVisibility = "hidden_by_user"
	CommentVisibilityHiddenByMod  CommentVisibility = "hidden_by_mod"
	CommentVisibilityPendingReview CommentVisibility = "pending_review"
)

// ReplyPermission represents who can reply to a comment
type ReplyPermission string

const (
	ReplyPermissionAnyone    ReplyPermission = "anyone"
	ReplyPermissionAuthorOnly ReplyPermission = "author_only"
)

// CommentEntityType represents the valid entity types for comments
type CommentEntityType string

const (
	CommentEntityArtist     CommentEntityType = "artist"
	CommentEntityVenue      CommentEntityType = "venue"
	CommentEntityShow       CommentEntityType = "show"
	CommentEntityRelease    CommentEntityType = "release"
	CommentEntityLabel      CommentEntityType = "label"
	CommentEntityFestival   CommentEntityType = "festival"
	CommentEntityCollection CommentEntityType = "collection"
)

// ValidCommentEntityTypes is the set of all valid entity types for comments
var ValidCommentEntityTypes = map[CommentEntityType]string{
	CommentEntityArtist:     "artists",
	CommentEntityVenue:      "venues",
	CommentEntityShow:       "shows",
	CommentEntityRelease:    "releases",
	CommentEntityLabel:      "labels",
	CommentEntityFestival:   "festivals",
	CommentEntityCollection: "collections",
}

// MaxCommentDepth is the maximum nesting depth (0=top-level, 1=reply, 2=reply-to-reply)
const MaxCommentDepth = 2

// MaxCommentBodyLength is the maximum body length in characters
const MaxCommentBodyLength = 10000

// MinCommentBodyLength is the minimum body length in characters
const MinCommentBodyLength = 1

// Comment represents a comment or field note on any entity
type Comment struct {
	ID              uint              `gorm:"primaryKey;column:id"`
	EntityType      CommentEntityType `gorm:"not null;column:entity_type"`
	EntityID        uint              `gorm:"not null;column:entity_id"`
	Kind            CommentKind       `gorm:"not null;column:kind;default:'comment'"`
	UserID          uint              `gorm:"not null;column:user_id"`
	ParentID        *uint             `gorm:"column:parent_id"`
	RootID          *uint             `gorm:"column:root_id"`
	Depth           int               `gorm:"not null;column:depth;default:0"`
	Body            string            `gorm:"not null;column:body"`
	BodyHTML        string            `gorm:"not null;column:body_html"`
	StructuredData  *json.RawMessage  `gorm:"column:structured_data;type:jsonb"`
	Visibility      CommentVisibility `gorm:"not null;column:visibility;default:'visible'"`
	HiddenReason    *string           `gorm:"column:hidden_reason"`
	HiddenByUserID  *uint             `gorm:"column:hidden_by_user_id"`
	HiddenAt        *time.Time        `gorm:"column:hidden_at"`
	ReplyPermission ReplyPermission   `gorm:"not null;column:reply_permission;default:'anyone'"`
	Ups             int               `gorm:"not null;column:ups;default:0"`
	Downs           int               `gorm:"not null;column:downs;default:0"`
	Score           float64           `gorm:"not null;column:score;default:0"`
	EditCount       int               `gorm:"not null;column:edit_count;default:0"`
	CreatedAt       time.Time         `gorm:"not null;column:created_at"`
	UpdatedAt       time.Time         `gorm:"not null;column:updated_at"`

	// Relationships (for preloading)
	User   User          `gorm:"foreignKey:UserID"`
	Parent *Comment      `gorm:"foreignKey:ParentID"`
	Edits  []CommentEdit `gorm:"foreignKey:CommentID"`
}

// TableName specifies the table name for Comment
func (Comment) TableName() string {
	return "comments"
}

// CommentEdit represents a historical edit of a comment (append-only)
type CommentEdit struct {
	ID        uint      `gorm:"primaryKey;column:id"`
	CommentID uint      `gorm:"not null;column:comment_id"`
	OldBody   string    `gorm:"not null;column:old_body"`
	EditedAt  time.Time `gorm:"not null;column:edited_at"`
}

// TableName specifies the table name for CommentEdit
func (CommentEdit) TableName() string {
	return "comment_edits"
}

// CommentVote represents a user's vote on a comment
type CommentVote struct {
	CommentID uint      `gorm:"primaryKey;column:comment_id"`
	UserID    uint      `gorm:"primaryKey;column:user_id"`
	Direction int16     `gorm:"not null;column:direction"`
	CreatedAt time.Time `gorm:"not null;column:created_at"`
	UpdatedAt time.Time `gorm:"not null;column:updated_at"`
}

// TableName specifies the table name for CommentVote
func (CommentVote) TableName() string {
	return "comment_votes"
}

// CommentSubscription tracks a user's subscription to comment threads on an entity.
type CommentSubscription struct {
	UserID       uint      `gorm:"primaryKey;column:user_id"`
	EntityType   string    `gorm:"primaryKey;column:entity_type"`
	EntityID     uint      `gorm:"primaryKey;column:entity_id"`
	SubscribedAt time.Time `gorm:"not null;column:subscribed_at"`
}

// TableName specifies the table name for CommentSubscription
func (CommentSubscription) TableName() string {
	return "comment_subscriptions"
}

// CommentLastRead tracks the highest comment ID a user has seen per entity.
type CommentLastRead struct {
	UserID            uint      `gorm:"primaryKey;column:user_id"`
	EntityType        string    `gorm:"primaryKey;column:entity_type"`
	EntityID          uint      `gorm:"primaryKey;column:entity_id"`
	LastReadCommentID uint      `gorm:"not null;column:last_read_comment_id;default:0"`
	UpdatedAt         time.Time `gorm:"not null;column:updated_at"`
}

// TableName specifies the table name for CommentLastRead
func (CommentLastRead) TableName() string {
	return "comment_last_read"
}
