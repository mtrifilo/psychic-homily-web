package notification

import (
	"encoding/json"
	"time"

	"github.com/lib/pq"
)

// Filter ownership sources (PSY-1467). Quick entity toggles only create/delete
// "managed" rows; settings-authored filters are "user" and must never be
// mutated by the NotifyMeButton lifecycle.
const (
	FilterSourceUser    = "user"
	FilterSourceManaged = "managed"
)

// NotificationFilter represents a user-created filter for automatic show notifications.
// When a show is approved, all active filters are evaluated against it.
type NotificationFilter struct {
	ID     uint   `gorm:"primaryKey" json:"id"`
	UserID uint   `gorm:"not null" json:"user_id"`
	Name   string `gorm:"size:128;not null" json:"name"`

	// Source is "user" (settings-authored) or "managed" (entity-page quick toggle).
	// Default "user". PSY-1467.
	Source string `gorm:"size:16;not null;default:user" json:"source"`

	// IsActive allows pausing without deleting. Default true.
	IsActive bool `gorm:"not null;default:true" json:"is_active"`

	// Match criteria — all nullable, NULL means "any".
	// OR logic within a criteria type, AND logic across types.
	ArtistIDs     pq.Int64Array    `gorm:"type:bigint[]" json:"artist_ids,omitempty"`
	VenueIDs      pq.Int64Array    `gorm:"type:bigint[]" json:"venue_ids,omitempty"`
	LabelIDs      pq.Int64Array    `gorm:"type:bigint[]" json:"label_ids,omitempty"`
	TagIDs        pq.Int64Array    `gorm:"type:bigint[]" json:"tag_ids,omitempty"`
	ExcludeTagIDs pq.Int64Array    `gorm:"type:bigint[];column:exclude_tag_ids" json:"exclude_tag_ids,omitempty"`
	Cities        *json.RawMessage `gorm:"type:jsonb" json:"cities,omitempty"`
	PriceMaxCents *int             `gorm:"column:price_max_cents" json:"price_max_cents,omitempty"`

	// Delivery preferences
	NotifyEmail bool `gorm:"not null;default:true" json:"notify_email"`
	NotifyInApp bool `gorm:"not null;default:true" json:"notify_in_app"`
	NotifyPush  bool `gorm:"not null;default:false" json:"notify_push"`

	// Metadata
	LastMatchedAt *time.Time `json:"last_matched_at,omitempty"`
	MatchCount    int        `gorm:"not null;default:0" json:"match_count"`
	CreatedAt     time.Time  `gorm:"not null" json:"created_at"`
	UpdatedAt     time.Time  `gorm:"not null" json:"updated_at"`
}

// TableName specifies the table name for NotificationFilter.
func (NotificationFilter) TableName() string {
	return "notification_filters"
}

// NotificationLog records every notification sent, for deduplication and user history.
type NotificationLog struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	UserID     uint       `gorm:"not null" json:"user_id"`
	FilterID   *uint      `json:"filter_id,omitempty"`
	EntityType string     `gorm:"size:50;not null" json:"entity_type"`
	EntityID   uint       `gorm:"column:entity_id;not null" json:"entity_id"`
	Channel    string     `gorm:"size:20;not null" json:"channel"`
	SentAt     time.Time  `gorm:"not null" json:"sent_at"`
	ReadAt     *time.Time `json:"read_at,omitempty"`

	// SubjectEntityID is the FOLLOWED entity whose subscription produced this
	// row, where (EntityType, EntityID) is what the row is ABOUT (PSY-1896).
	// For an artist show alert that split is artist vs show: the inbox line
	// reads "<artist> announced a show" while the row points at the show.
	//
	// The subject's type is implied by EntityType rather than stored beside it,
	// so the two can never disagree. NULL for every writer that has no followed
	// subject: filter matches, scene follows, comment replies, mentions, and
	// request fulfillments.
	SubjectEntityID *uint `gorm:"column:subject_entity_id" json:"subject_entity_id,omitempty"`
}

// TableName specifies the table name for NotificationLog.
func (NotificationLog) TableName() string {
	return "notification_log"
}

// In-app notification_log discriminators shared across domain services that
// write rows and the notification read service that enriches them. Kept here
// (next to the NotificationLog model) so a domain service can mint a row
// without importing another service package. The comment-driven channel +
// entity-type constants predate this and live in the engagement package
// (engagement.NotificationChannelInApp / NotificationEntityCommentReply); the
// channel value is identical by design.
const (
	// NotificationChannelInApp is the channel value for in-app notification_log
	// rows surfaced by the bell/inbox (vs the "email" channel used by the
	// show-filter matcher).
	NotificationChannelInApp = "in_app"

	// NotificationChannelEmail is the channel value for rows that stand for an
	// email. It is also what the per-user daily email budget counts
	// (maxFilterEmailsPerDay), which is the reason a row exists for the email
	// lane at all.
	//
	// Read the column honestly: it says WHICH LANE a row belongs to, not which
	// lanes the user has enabled. The bell renders rows of either channel today
	// (the show-filter and scene-follow writers stamp "email" on rows that are
	// the user's only in-app record), so it is NOT a visibility switch for those
	// writers. PSY-1896's rows are the exception and say so at their own read
	// site: they use one row per lane, and the email lane is not a bell entry.
	NotificationChannelEmail = "email"

	// NotificationEntityShow is the entity_type the show-filter matcher and the
	// scene-follow fanout write. Spelled once here so a cross-system dedup can
	// name it without a literal.
	NotificationEntityShow = "show"

	// NotificationEntityArtistShowAlert marks a notification_log row created
	// because a show was announced by an ARTIST the user follows (PSY-1896).
	// entity_id holds the show id and subject_entity_id holds the artist id.
	//
	// A discriminator of its own rather than reusing NotificationEntityShow,
	// because the read path has to tell the three follow-driven show rows apart:
	// a filter match names its filter, a scene follow is labelled from the
	// show's venues, and this one names the artist. Sharing "show" would have
	// made this row inherit the scene label, which is the wrong sentence.
	NotificationEntityArtistShowAlert = "artist_show_alert"

	// NotificationEntityRequestFulfillmentProposed marks a notification_log row
	// created when someone proposes a fulfillment for a community request (the
	// request enters pending_fulfillment). entity_id holds the request_id; the
	// requester is notified so they can approve or reject. PSY-890.
	NotificationEntityRequestFulfillmentProposed = "request_fulfillment_proposed"
)
