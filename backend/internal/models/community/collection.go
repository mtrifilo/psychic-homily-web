package community

import (
	"time"

	"psychic-homily-backend/internal/models/auth"
)

// Collection entity type constants for CollectionItem.EntityType
const (
	CollectionEntityArtist   = "artist"
	CollectionEntityRelease  = "release"
	CollectionEntityLabel    = "label"
	CollectionEntityShow     = "show"
	CollectionEntityVenue    = "venue"
	CollectionEntityFestival = "festival"
)

// Collection display_mode values. Ranked surfaces numbered positions and
// drag-and-drop reordering; unranked is a flat list with no numbering.
const (
	CollectionDisplayModeRanked   = "ranked"
	CollectionDisplayModeUnranked = "unranked"
)

// IsValidCollectionDisplayMode returns true if mode is a recognized display
// mode. Used by services/handlers to reject bad client input before it hits
// the DB CHECK constraint.
func IsValidCollectionDisplayMode(mode string) bool {
	return mode == CollectionDisplayModeRanked || mode == CollectionDisplayModeUnranked
}

// IsValidCollectionEntityType returns true if entityType is one of the six
// indexed KG entity types accepted as a collection item. Used by the bulk-add
// + URL-resolve paths (PSY-823) to reject typos before they reach the DB
// (collection_items.entity_type has no FK constraint — polymorphic).
func IsValidCollectionEntityType(entityType string) bool {
	switch entityType {
	case CollectionEntityArtist,
		CollectionEntityRelease,
		CollectionEntityLabel,
		CollectionEntityShow,
		CollectionEntityVenue,
		CollectionEntityFestival:
		return true
	}
	return false
}

// Collection represents a user-curated collection of entities
type Collection struct {
	ID            uint    `gorm:"primaryKey"`
	Title         string  `gorm:"not null"`
	Slug          string  `gorm:"not null;uniqueIndex"`
	Description   string  `gorm:"not null;default:''"`
	CreatorID     uint    `gorm:"column:creator_id;not null"`
	Collaborative bool    `gorm:"not null;default:true"`
	CoverImageURL *string `gorm:"column:cover_image_url"`
	IsPublic      bool    `gorm:"column:is_public;not null;default:true"`
	IsFeatured    bool    `gorm:"column:is_featured;not null;default:false"`
	DisplayMode   string  `gorm:"column:display_mode;not null;default:unranked"`
	// ForkedFromCollectionID is set when this collection was created via clone.
	// FK uses ON DELETE SET NULL so deleting the source does not cascade-delete
	// forks (see migration 20260427173004). PSY-351.
	ForkedFromCollectionID *uint     `gorm:"column:forked_from_collection_id"`
	CreatedAt              time.Time `gorm:"not null"`
	UpdatedAt              time.Time `gorm:"not null"`

	// Relationships
	Creator     auth.User              `gorm:"foreignKey:CreatorID"`
	Items       []CollectionItem       `gorm:"foreignKey:CollectionID"`
	Subscribers []CollectionSubscriber `gorm:"foreignKey:CollectionID"`
}

// TableName specifies the table name for Collection
func (Collection) TableName() string {
	return "collections"
}

// CollectionItem represents an entity added to a collection
type CollectionItem struct {
	ID            uint      `gorm:"primaryKey"`
	CollectionID  uint      `gorm:"column:collection_id;not null"`
	EntityType    string    `gorm:"column:entity_type;not null"`
	EntityID      uint      `gorm:"column:entity_id;not null"`
	Position      int       `gorm:"not null;default:0"`
	AddedByUserID uint      `gorm:"column:added_by_user_id;not null"`
	Notes         *string   `gorm:"column:notes"`
	CreatedAt     time.Time `gorm:"not null"`

	// Relationships
	Collection Collection `gorm:"foreignKey:CollectionID"`
	AddedBy    auth.User  `gorm:"foreignKey:AddedByUserID"`
}

// TableName specifies the table name for CollectionItem
func (CollectionItem) TableName() string {
	return "collection_items"
}

// CollectionSubscriber represents a user subscribed to a collection
type CollectionSubscriber struct {
	CollectionID  uint       `gorm:"primaryKey;column:collection_id"`
	UserID        uint       `gorm:"primaryKey;column:user_id"`
	LastVisitedAt *time.Time `gorm:"column:last_visited_at"`
	// LastDigestSentAt is the per-subscriber cursor for the weekly digest
	// job (PSY-350). Null = "no digest sent yet"; the cycle then looks back
	// to the subscription's CreatedAt so we don't miss items added between
	// subscribing and the first cycle.
	LastDigestSentAt *time.Time `gorm:"column:last_digest_sent_at"`
	CreatedAt        time.Time  `gorm:"not null"`

	// Relationships
	Collection Collection `gorm:"foreignKey:CollectionID"`
	User       auth.User  `gorm:"foreignKey:UserID"`
}

// TableName specifies the table name for CollectionSubscriber
func (CollectionSubscriber) TableName() string {
	return "collection_subscribers"
}

// CollectionLike represents a single user's like on a collection.
// Composite PK (user_id, collection_id) makes the like inherently unique
// and POST idempotent via INSERT ... ON CONFLICT DO NOTHING. PSY-352.
type CollectionLike struct {
	UserID       uint      `gorm:"primaryKey;column:user_id" json:"user_id"`
	CollectionID uint      `gorm:"primaryKey;column:collection_id" json:"collection_id"`
	CreatedAt    time.Time `gorm:"not null;column:created_at;default:CURRENT_TIMESTAMP" json:"created_at"`
}

// TableName specifies the table name for CollectionLike.
func (CollectionLike) TableName() string {
	return "collection_likes"
}

// CollectionFeatureRun is one featuring stint for a collection (PSY-1500) — a
// journal row that opens when a collection is featured and closes when it is
// unfeatured. It replaces the history-destroying bare boolean as the source of
// truth for "who has been featured, when, and in what order," which is what
// PSY-1411's "most recently featured" lock needs. Shape mirrors the
// radio_sync_runs lifecycle table (catalog.RadioSyncRun), the house pattern for
// "a thing that starts, runs, and ends."
//
// UnfeaturedAt NULL means the run is OPEN (still featured). A partial unique
// index (collection_feature_runs_one_open) enforces at most one open run per
// collection; closed runs are unconstrained so re-featuring accrues history.
// collections.is_featured is kept as a denormalised cache of "has an open run"
// and CollectionService.SetFeatured writes both in one transaction so they can
// never drift.
type CollectionFeatureRun struct {
	ID           uint       `gorm:"primaryKey"`
	CollectionID uint       `gorm:"column:collection_id;not null"`
	FeaturedAt   time.Time  `gorm:"column:featured_at;not null"`
	UnfeaturedAt *time.Time `gorm:"column:unfeatured_at"`
	// FeaturedBy/UnfeaturedBy are soft references (ON DELETE SET NULL) so
	// deleting the acting admin never deletes the historical run.
	FeaturedBy   *uint `gorm:"column:featured_by"`
	UnfeaturedBy *uint `gorm:"column:unfeatured_by"`
	// FeaturedAtEstimated marks a start reconstructed at backfill from
	// collections.created_at rather than an observed audit event — the archive
	// must not render a precise date for an estimated row.
	FeaturedAtEstimated bool      `gorm:"column:featured_at_estimated;not null;default:false"`
	CreatedAt           time.Time `gorm:"column:created_at;not null"`

	// Relationships
	Collection Collection `gorm:"foreignKey:CollectionID"`
}

// TableName specifies the table name for CollectionFeatureRun.
func (CollectionFeatureRun) TableName() string {
	return "collection_feature_runs"
}
