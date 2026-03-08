package models

import "time"

// BookmarkEntityType represents the type of entity being bookmarked
type BookmarkEntityType string

const (
	BookmarkEntityShow    BookmarkEntityType = "show"
	BookmarkEntityVenue   BookmarkEntityType = "venue"
	BookmarkEntityArtist  BookmarkEntityType = "artist"
	BookmarkEntityRelease BookmarkEntityType = "release"
	BookmarkEntityLabel   BookmarkEntityType = "label"
	BookmarkEntityFestival BookmarkEntityType = "festival"
)

// BookmarkAction represents the type of bookmark action
type BookmarkAction string

const (
	BookmarkActionSave       BookmarkAction = "save"
	BookmarkActionFollow     BookmarkAction = "follow"
	BookmarkActionBookmark   BookmarkAction = "bookmark"
	BookmarkActionGoing      BookmarkAction = "going"
	BookmarkActionInterested BookmarkAction = "interested"
)

// UserBookmark represents a generic user-entity relationship
type UserBookmark struct {
	ID             uint               `gorm:"primaryKey;column:id"`
	UserID         uint               `gorm:"not null;column:user_id"`
	EntityType     BookmarkEntityType `gorm:"not null;column:entity_type"`
	EntityID       uint               `gorm:"not null;column:entity_id"`
	Action         BookmarkAction     `gorm:"not null;column:action"`
	CreatedAt      time.Time          `gorm:"not null;column:created_at"`
	ReminderSentAt *time.Time         `gorm:"column:reminder_sent_at"`
}

// TableName specifies the table name for UserBookmark
func (UserBookmark) TableName() string {
	return "user_bookmarks"
}
