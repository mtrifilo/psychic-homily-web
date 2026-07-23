package engagement

import (
	"encoding/json"
	"time"
)

// BookmarkEntityType represents the type of entity being bookmarked
type BookmarkEntityType string

const (
	BookmarkEntityShow     BookmarkEntityType = "show"
	BookmarkEntityVenue    BookmarkEntityType = "venue"
	BookmarkEntityArtist   BookmarkEntityType = "artist"
	BookmarkEntityRelease  BookmarkEntityType = "release"
	BookmarkEntityLabel    BookmarkEntityType = "label"
	BookmarkEntityFestival BookmarkEntityType = "festival"
	// BookmarkEntityTag follows a tags row (PSY-1064) — display-only in v1
	// (profile TAGS row + library); no notification / show-alert fan-out.
	BookmarkEntityTag BookmarkEntityType = "tag"
	// BookmarkEntityScene follows a scenes-registry row (PSY-1339) — the
	// lazily-materialized identity for a computed metro scene.
	BookmarkEntityScene BookmarkEntityType = "scene"
	// BookmarkEntityRadioShow follows a radio_shows row (PSY-1356) — enables
	// the Following Feed's "followed radio show → new episode" signal.
	BookmarkEntityRadioShow BookmarkEntityType = "radio_show"
)

// BookmarkAction represents the type of bookmark action
type BookmarkAction string

const (
	// BookmarkActionSave is the single "keep this on my radar" action for shows.
	// A user's saved list is private; only the per-show save count is public.
	BookmarkActionSave     BookmarkAction = "save"
	BookmarkActionFollow   BookmarkAction = "follow"
	BookmarkActionBookmark BookmarkAction = "bookmark"
	// BookmarkActionReleaseSave names the release Save/Saved relationship while
	// preserving compatibility with historical release bookmark rows.
	BookmarkActionReleaseSave BookmarkAction = BookmarkActionBookmark
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
	// Settings holds follow-scoped preferences (PSY-1341, +off in PSY-1466).
	// First key: "scene_notify_mode" — "all" (default when absent),
	// "followed_bands_only", or "off" for scene follows' new-show
	// notifications. Does not affect the separate weekly scene digest opt-in.
	Settings *json.RawMessage `gorm:"type:jsonb;column:settings"`
	// SceneDigestSentAt is the per-scene-follow weekly-digest cursor (PSY-1342):
	// the digest job includes new bands with created_at after this. NULL until
	// the first digest; the first cycle looks back to CreatedAt instead.
	SceneDigestSentAt *time.Time `gorm:"column:scene_digest_sent_at"`
}

// TableName specifies the table name for UserBookmark
func (UserBookmark) TableName() string {
	return "user_bookmarks"
}
