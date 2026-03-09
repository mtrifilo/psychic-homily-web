package models

import "time"

// Collection entity type constants for CollectionItem.EntityType
const (
	CollectionEntityArtist   = "artist"
	CollectionEntityRelease  = "release"
	CollectionEntityLabel    = "label"
	CollectionEntityShow     = "show"
	CollectionEntityVenue    = "venue"
	CollectionEntityFestival = "festival"
)

// Collection represents a user-curated collection of entities
type Collection struct {
	ID            uint      `gorm:"primaryKey"`
	Title         string    `gorm:"not null"`
	Slug          string    `gorm:"not null;uniqueIndex"`
	Description   string    `gorm:"not null;default:''"`
	CreatorID     uint      `gorm:"column:creator_id;not null"`
	Collaborative bool      `gorm:"not null;default:true"`
	CoverImageURL *string   `gorm:"column:cover_image_url"`
	IsPublic      bool      `gorm:"column:is_public;not null;default:true"`
	IsFeatured    bool      `gorm:"column:is_featured;not null;default:false"`
	CreatedAt     time.Time `gorm:"not null"`
	UpdatedAt     time.Time `gorm:"not null"`

	// Relationships
	Creator     User                   `gorm:"foreignKey:CreatorID"`
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
	AddedBy    User       `gorm:"foreignKey:AddedByUserID"`
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
	CreatedAt     time.Time  `gorm:"not null"`

	// Relationships
	Collection Collection `gorm:"foreignKey:CollectionID"`
	User       User       `gorm:"foreignKey:UserID"`
}

// TableName specifies the table name for CollectionSubscriber
func (CollectionSubscriber) TableName() string {
	return "collection_subscribers"
}
