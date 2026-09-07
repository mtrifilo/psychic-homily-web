package catalog

import (
	"strings"
	"time"

	"psychic-homily-backend/internal/models/auth"
)

// Tag category constants
const (
	TagCategoryGenre  = "genre"
	TagCategoryLocale = "locale"
	TagCategoryOther  = "other"
	// TagCategoryCrew marks a tag naming a MUSIC BOOKER: a promoter, a DIY
	// crew or collective, or a named music series or residency that books
	// live music. Non-music bar programming (bingo, karaoke, comedy open
	// mics) is outside the category and belongs under TagCategoryOther.
	TagCategoryCrew = "crew"
)

// TagCategories is the set of valid tag categories, enforced by
// IsValidTagCategory in TagService's create, update, and inline-create paths.
// Nothing below those enforces it: tags.category is a plain VARCHAR with no
// CHECK constraint, so seeders and migrations that write the column directly
// can store any string.
var TagCategories = []string{
	TagCategoryGenre,
	TagCategoryLocale,
	TagCategoryOther,
	TagCategoryCrew,
}

// Tag entity type constants (same values as CollectionEntity* / RequestEntity*)
const (
	TagEntityArtist   = "artist"
	TagEntityRelease  = "release"
	TagEntityLabel    = "label"
	TagEntityShow     = "show"
	TagEntityVenue    = "venue"
	TagEntityFestival = "festival"
	// TagEntityCollection enables polymorphic tagging on collections (PSY-354).
	// The shared entity_tags table is unchanged — collections piggy-back on the
	// existing (entity_type, entity_id) shape so no new migration is required.
	TagEntityCollection = "collection"
)

// TagSlugAllAges is the canonical slug for "this venue hosts all-ages shows".
//
// SEMANTICS (user decision, PSY-1573): the tag says the room hosts all-ages
// shows AT LEAST SOMETIMES. It does NOT claim every show there is all-ages,
// and it is NOT the venue's house-default age rule — that is the free-text
// venues.age_policy column (PSY-1682), which a per-show age_requirement
// overrides. A 21+ house that books the occasional all-ages matinee carries
// this tag and an "21+" age_policy at the same time, and both are true. Copy
// rendered from this tag must never promise more than "sometimes".
//
// Slug REUSED, not minted: cmd/seed's archive venue exemplar already applies
// {"All Ages", "all-ages", other}. Tags are free-form (no vocabulary table
// constrains the slug set), so this constant is the only thing making one
// spelling canonical — apply it rather than re-typing the literal.
const TagSlugAllAges = "all-ages"

// TagEntityTypes is the set of valid entity types for tagging.
var TagEntityTypes = []string{
	TagEntityArtist,
	TagEntityRelease,
	TagEntityLabel,
	TagEntityShow,
	TagEntityVenue,
	TagEntityFestival,
	TagEntityCollection,
}

// MaxTagNameLength is the width of the tags.name column. Stated here because
// the gorm tag below cannot reference a constant, and a writer that bounds the
// value it stores has to read the bound from somewhere.
const MaxTagNameLength = 100

// Tag represents a user-facing tag for categorizing entities.
type Tag struct {
	ID              uint       `json:"id" gorm:"primaryKey"`
	Name            string     `json:"name" gorm:"column:name;not null;size:100"`
	Slug            string     `json:"slug" gorm:"column:slug;not null;uniqueIndex;size:120"`
	Description     *string    `json:"description,omitempty" gorm:"column:description"`
	ParentID        *uint      `json:"parent_id,omitempty" gorm:"column:parent_id"`
	Category        string     `json:"category" gorm:"column:category;not null;default:'genre';size:50"`
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
	CreatedBy *auth.User  `json:"-" gorm:"foreignKey:CreatedByUserID"`
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
	Tag     Tag       `json:"-" gorm:"foreignKey:TagID"`
	AddedBy auth.User `json:"-" gorm:"foreignKey:AddedByUserID"`
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
	Tag  Tag       `json:"-" gorm:"foreignKey:TagID"`
	User auth.User `json:"-" gorm:"foreignKey:UserID"`
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

// IsAdminMintOnlyTagCategory reports whether CREATING a tag in this category
// requires admin.
//
// The rule governs which NAMES may enter the tag vocabulary, and nothing else.
// Applying an existing tag of the category, and removing an application, are
// unrestricted by it, so what a crew tag is attached to is not admin-controlled
// even though what a crew tag is called is.
//
// Crew is restricted because the value names a real party. The other categories
// describe rather than name, so a wrong one is noise a curator fixes.
func IsAdminMintOnlyTagCategory(category string) bool {
	return category == TagCategoryCrew
}

// normalizeTagCategory folds a STORED tags.category value, which the column
// does not constrain, to the spelling the constants use.
//
// IsAdminMintOnlyTagCategory does not fold: its input is the category on the
// REQUEST, already matched exactly against TagCategories.
func normalizeTagCategory(category string) string {
	return strings.ToLower(strings.TrimSpace(category))
}

// IsTierGatedTagCategory reports whether APPLYING a tag of this category to an
// entity, or REMOVING one, requires the trusted contributor tier.
//
// Crew is gated because the value names a real party, so an application is a
// claim that a named booker put on a show.
//
// Answers a different question from IsAdminMintOnlyTagCategory (who may bring a
// NAME into the vocabulary) and IsDescriptiveTagCategory (where a tag may be
// ranked). The three agree on crew.
func IsTierGatedTagCategory(category string) bool {
	return normalizeTagCategory(category) == TagCategoryCrew
}

// IsDescriptiveTagCategory reports whether a category describes its subject
// rather than naming a party.
//
// A ranking that mixes the two reads a booker's name as a genre, so the readers
// that rank tags against each other keep to the descriptive ones.
//
// An unrecognized category counts as descriptive, so a category this build has
// not heard of is ranked rather than silently dropped.
//
// MIRRORS isDescriptiveTagCategory in frontend/features/tags/types.ts, down to
// the normalization and the unrecognized-category answer;
// TestDescriptiveTagCategoriesMatchFrontend fails when the two disagree.
func IsDescriptiveTagCategory(category string) bool {
	return normalizeTagCategory(category) != TagCategoryCrew
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
