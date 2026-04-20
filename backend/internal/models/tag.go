package models

import "time"

// Tag category constants
const (
	TagCategoryGenre  = "genre"
	TagCategoryLocale = "locale"
	TagCategoryOther  = "other"
)

// TagCategories is the set of valid tag categories.
var TagCategories = []string{
	TagCategoryGenre,
	TagCategoryLocale,
	TagCategoryOther,
}

// Tag entity type constants (same values as CollectionEntity* / RequestEntity*)
const (
	TagEntityArtist   = "artist"
	TagEntityRelease  = "release"
	TagEntityLabel    = "label"
	TagEntityShow     = "show"
	TagEntityVenue    = "venue"
	TagEntityFestival = "festival"
)

// TagEntityTypes is the set of valid entity types for tagging.
var TagEntityTypes = []string{
	TagEntityArtist,
	TagEntityRelease,
	TagEntityLabel,
	TagEntityShow,
	TagEntityVenue,
	TagEntityFestival,
}

// Tag represents a user-facing tag for categorizing entities.
type Tag struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	Name        string    `json:"name" gorm:"column:name;not null;size:100"`
	Slug        string    `json:"slug" gorm:"column:slug;not null;uniqueIndex;size:120"`
	Description *string   `json:"description,omitempty" gorm:"column:description"`
	ParentID    *uint     `json:"parent_id,omitempty" gorm:"column:parent_id"`
	Category    string    `json:"category" gorm:"column:category;not null;default:'genre';size:50"`
	IsOfficial      bool       `json:"is_official" gorm:"column:is_official;not null;default:false"`
	UsageCount      int        `json:"usage_count" gorm:"column:usage_count;not null;default:0"`
	CreatedByUserID *uint      `json:"created_by_user_id,omitempty" gorm:"column:created_by_user_id"`
	ReviewedAt      *time.Time `json:"reviewed_at,omitempty" gorm:"column:reviewed_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`

	// Relationships
	Parent    *Tag        `json:"parent,omitempty" gorm:"foreignKey:ParentID"`
	Children  []Tag       `json:"children,omitempty" gorm:"foreignKey:ParentID"`
	Aliases   []TagAlias  `json:"aliases,omitempty" gorm:"foreignKey:TagID"`
	Entities  []EntityTag `json:"-" gorm:"foreignKey:TagID"`
	CreatedBy *User       `json:"-" gorm:"foreignKey:CreatedByUserID"`
}

// TableName specifies the table name for Tag.
func (Tag) TableName() string { return "tags" }

// EntityTag represents a tag applied to an entity (junction table).
type EntityTag struct {
	ID            uint      `json:"id" gorm:"primaryKey"`
	TagID         uint      `json:"tag_id" gorm:"column:tag_id;not null"`
	EntityType    string    `json:"entity_type" gorm:"column:entity_type;not null;size:50"`
	EntityID      uint      `json:"entity_id" gorm:"column:entity_id;not null"`
	AddedByUserID uint      `json:"added_by_user_id" gorm:"column:added_by_user_id;not null"`
	CreatedAt     time.Time `json:"created_at"`

	// Relationships
	Tag     Tag  `json:"-" gorm:"foreignKey:TagID"`
	AddedBy User `json:"-" gorm:"foreignKey:AddedByUserID"`
}

// TableName specifies the table name for EntityTag.
func (EntityTag) TableName() string { return "entity_tags" }

// TagVote represents a user's relevance vote on a tag for a specific entity.
type TagVote struct {
	TagID      uint      `json:"tag_id" gorm:"column:tag_id;primaryKey"`
	EntityType string    `json:"entity_type" gorm:"column:entity_type;primaryKey;size:50"`
	EntityID   uint      `json:"entity_id" gorm:"column:entity_id;primaryKey"`
	UserID     uint      `json:"user_id" gorm:"column:user_id;primaryKey"`
	Vote       int       `json:"vote" gorm:"column:vote;not null"`
	CreatedAt  time.Time `json:"created_at"`

	// Relationships
	Tag  Tag  `json:"-" gorm:"foreignKey:TagID"`
	User User `json:"-" gorm:"foreignKey:UserID"`
}

// TableName specifies the table name for TagVote.
func (TagVote) TableName() string { return "tag_votes" }

// TagAlias represents an alternate name that resolves to a canonical tag.
type TagAlias struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	TagID     uint      `json:"tag_id" gorm:"column:tag_id;not null"`
	Alias     string    `json:"alias" gorm:"column:alias;not null;size:100"`
	CreatedAt time.Time `json:"created_at"`

	// Relationships
	Tag Tag `json:"-" gorm:"foreignKey:TagID"`
}

// TableName specifies the table name for TagAlias.
func (TagAlias) TableName() string { return "tag_aliases" }

// IsValidTagCategory returns true if the given category is valid.
func IsValidTagCategory(category string) bool {
	for _, c := range TagCategories {
		if c == category {
			return true
		}
	}
	return false
}

// IsValidTagEntityType returns true if the given entity type is valid for tagging.
func IsValidTagEntityType(entityType string) bool {
	for _, t := range TagEntityTypes {
		if t == entityType {
			return true
		}
	}
	return false
}
