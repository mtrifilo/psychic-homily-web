package contracts

import (
	"time"

	"psychic-homily-backend/internal/models"
)

// ──────────────────────────────────────────────
// Tag types
// ──────────────────────────────────────────────

// TagResponse represents a tag returned to clients.
type TagResponse struct {
	ID                uint      `json:"id"`
	Name              string    `json:"name"`
	Slug              string    `json:"slug"`
	Description       *string   `json:"description,omitempty"`
	ParentID          *uint     `json:"parent_id,omitempty"`
	ParentName        string    `json:"parent_name,omitempty"`
	Category          string    `json:"category"`
	IsOfficial        bool      `json:"is_official"`
	UsageCount        int       `json:"usage_count"`
	ChildCount        int       `json:"child_count"`
	Aliases           []string  `json:"aliases,omitempty"`
	CreatedByUserID   *uint     `json:"created_by_user_id,omitempty"`
	CreatedByUsername *string   `json:"created_by_username,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// TagListItem represents a tag in a list response.
//
// MatchedViaAlias is populated only by the tag search/autocomplete endpoint
// when the query matched a row in `tag_aliases` rather than `tags.name`. The
// field holds the specific alias that matched, so the UI can show the user
// which term was interpreted as the canonical tag ("matched `punk-rock`").
// Empty/omitted for all other list contexts.
type TagListItem struct {
	ID              uint      `json:"id"`
	Name            string    `json:"name"`
	Slug            string    `json:"slug"`
	Category        string    `json:"category"`
	IsOfficial      bool      `json:"is_official"`
	UsageCount      int       `json:"usage_count"`
	CreatedAt       time.Time `json:"created_at"`
	MatchedViaAlias string    `json:"matched_via_alias,omitempty"`
}

// TagSearchResult pairs a tag with any alias (on that tag) that matched the
// search query. MatchedAlias is empty when the query matched the tag's name
// directly, or when both the name and an alias matched (name match takes
// precedence so the canonical form is surfaced without extra noise).
type TagSearchResult struct {
	Tag          models.Tag
	MatchedAlias string
}

// EntityTagResponse represents a tag applied to an entity with vote info.
type EntityTagResponse struct {
	TagID           uint    `json:"tag_id"`
	Name            string  `json:"name"`
	Slug            string  `json:"slug"`
	Category        string  `json:"category"`
	IsOfficial      bool    `json:"is_official"`
	Upvotes         int     `json:"upvotes"`
	Downvotes       int     `json:"downvotes"`
	WilsonScore     float64 `json:"wilson_score"`
	UserVote        *int    `json:"user_vote,omitempty"`
	AddedByUsername string  `json:"added_by_username,omitempty"`
}

// TaggedEntityItem represents a single entity tagged with a given tag.
type TaggedEntityItem struct {
	EntityType string `json:"entity_type"`
	EntityID   uint   `json:"entity_id"`
	Name       string `json:"name"`
	Slug       string `json:"slug"`
}

// TagAliasResponse represents a tag alias returned to clients.
type TagAliasResponse struct {
	ID        uint      `json:"id"`
	Alias     string    `json:"alias"`
	CreatedAt time.Time `json:"created_at"`
}

// TagServiceInterface defines the contract for tag operations.
type TagServiceInterface interface {
	// CRUD
	CreateTag(name string, description *string, parentID *uint, category string, isOfficial bool, userID *uint) (*models.Tag, error)
	GetTag(tagID uint) (*models.Tag, error)
	GetTagBySlug(slug string) (*models.Tag, error)
	ListTags(category string, search string, parentID *uint, sort string, limit, offset int) ([]models.Tag, int64, error)
	UpdateTag(tagID uint, name *string, description *string, parentID *uint, category *string, isOfficial *bool) (*models.Tag, error)
	DeleteTag(tagID uint) error

	// Entity tagging
	AddTagToEntity(tagID uint, tagName string, entityType string, entityID uint, userID uint, category string) (*models.EntityTag, error)
	RemoveTagFromEntity(tagID uint, entityType string, entityID uint) error
	ListEntityTags(entityType string, entityID uint, userID uint) ([]EntityTagResponse, error)

	// Voting
	VoteOnTag(tagID uint, entityType string, entityID uint, userID uint, isUpvote bool) error
	RemoveTagVote(tagID uint, entityType string, entityID uint, userID uint) error

	// Aliases
	CreateAlias(tagID uint, alias string) (*models.TagAlias, error)
	DeleteAlias(aliasID uint) error
	ListAliases(tagID uint) ([]models.TagAlias, error)
	ResolveAlias(alias string) (*models.Tag, error)

	// Tag entities
	GetTagEntities(tagID uint, entityType string, limit, offset int) ([]TaggedEntityItem, int64, error)

	// Utility
	SearchTags(query string, limit int, category string) ([]TagSearchResult, error)
	GetTrendingTags(limit int, category string) ([]models.Tag, error)
	PruneDownvotedTags() (int64, error)
}
