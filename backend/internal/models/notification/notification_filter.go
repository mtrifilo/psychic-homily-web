package notification

import (
	"encoding/json"
	"time"

	"github.com/lib/pq"
)

// NotificationFilter represents a user-created filter for automatic show notifications.
// When a show is approved, all active filters are evaluated against it.
type NotificationFilter struct {
	ID     uint   `gorm:"primaryKey" json:"id"`
	UserID uint   `gorm:"not null" json:"user_id"`
	Name   string `gorm:"size:128;not null" json:"name"`

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

	// NotificationEntityRequestFulfillmentProposed marks a notification_log row
	// created when someone proposes a fulfillment for a community request (the
	// request enters pending_fulfillment). entity_id holds the request_id; the
	// requester is notified so they can approve or reject. PSY-890.
	NotificationEntityRequestFulfillmentProposed = "request_fulfillment_proposed"
)
