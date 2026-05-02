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
type NotificationLogEntry struct {
	ID         uint       `json:"id"`
	FilterID   *uint      `json:"filter_id,omitempty"`
	FilterName string     `json:"filter_name,omitempty"`
	EntityType string     `json:"entity_type"`
	EntityID   uint       `json:"entity_id"`
	Channel    string     `json:"channel"`
	SentAt     time.Time  `json:"sent_at"`
	ReadAt     *time.Time `json:"read_at,omitempty"`
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
	GetUserNotifications(userID uint, limit, offset int) ([]NotificationLogEntry, error)
	GetUnreadCount(userID uint) (int64, error)

	// Unsubscribe
	PauseFilter(filterID uint) error
}
