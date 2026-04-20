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

// TagSummary is a minimal tag representation for use in parent/children/related arrays.
type TagSummary struct {
	ID         uint   `json:"id"`
	Name       string `json:"name"`
	Slug       string `json:"slug"`
	Category   string `json:"category"`
	IsOfficial bool   `json:"is_official"`
	UsageCount int    `json:"usage_count"`
}

// TagUserRef is a minimal user reference for creator attribution and contributor lists.
// Username doubles as the public profile slug (users/{username}).
type TagUserRef struct {
	ID       uint   `json:"id"`
	Username string `json:"username,omitempty"`
}

// TagContributor represents a single top contributor for a tag with their application count.
type TagContributor struct {
	User  TagUserRef `json:"user"`
	Count int64      `json:"count"`
}

// TagDetailResponse is the enriched response for the tag detail page.
// It embeds TagResponse and adds description_html, parent, children, usage breakdown,
// top contributors, created_by, and related tags.
type TagDetailResponse struct {
	TagResponse
	// DescriptionHTML is the description rendered from markdown (goldmark + bluemonday).
	// Empty when there is no description.
	DescriptionHTML string `json:"description_html,omitempty"`
	// Parent is the full parent tag summary, or nil when the tag has no parent.
	Parent *TagSummary `json:"parent,omitempty"`
	// Children is the list of direct child tags (empty when the tag has no children).
	Children []TagSummary `json:"children"`
	// UsageBreakdown maps entity_type → count across all valid tag entity types.
	// Every valid entity type is present; zero-counts are included so the frontend
	// can decide whether to show or hide them.
	UsageBreakdown map[string]int64 `json:"usage_breakdown"`
	// TopContributors lists the top 5 users by count of tag applications.
	TopContributors []TagContributor `json:"top_contributors"`
	// CreatedBy is the creator's user reference, or nil when unknown.
	CreatedBy *TagUserRef `json:"created_by,omitempty"`
	// RelatedTags are the top 5 other tags that co-occur on the same tagged entities.
	RelatedTags []TagSummary `json:"related_tags"`
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

// TagAliasListing represents a global alias listing row, pairing an alias
// with its canonical tag for the admin-wide alias management UI.
type TagAliasListing struct {
	ID             uint      `json:"id"`
	Alias          string    `json:"alias"`
	TagID          uint      `json:"tag_id"`
	TagName        string    `json:"tag_name"`
	TagSlug        string    `json:"tag_slug"`
	TagCategory    string    `json:"tag_category"`
	TagIsOfficial  bool      `json:"tag_is_official"`
	CreatedAt      time.Time `json:"created_at"`
}

// BulkAliasImportItem is one row of a bulk alias import request.
// Admins submit `{alias, canonical}` pairs; canonical can be a tag
// slug or exact name (case-insensitive match).
type BulkAliasImportItem struct {
	Alias     string `json:"alias"`
	Canonical string `json:"canonical"`
}

// BulkAliasImportSkipped describes a single row that was rejected
// during a bulk import so the admin UI can surface per-row errors.
type BulkAliasImportSkipped struct {
	Row       int    `json:"row"`
	Alias     string `json:"alias"`
	Canonical string `json:"canonical"`
	Reason    string `json:"reason"`
}

// BulkAliasImportResult summarizes the outcome of a bulk alias import.
type BulkAliasImportResult struct {
	Imported int                      `json:"imported"`
	Skipped  []BulkAliasImportSkipped `json:"skipped"`
}

// MergeTagsPreview summarizes what a merge would do, without actually doing it.
// Populated by the preview endpoint so the admin dialog can confirm before
// committing. Counts reflect the state at preview time; concurrent writes
// could make the actual merge differ slightly.
type MergeTagsPreview struct {
	MovedEntityTags    int64  `json:"moved_entity_tags"`
	MovedVotes         int64  `json:"moved_votes"`
	SkippedEntityTags  int64  `json:"skipped_entity_tags"`
	SkippedVotes       int64  `json:"skipped_votes"`
	SourceAliasesCount int64  `json:"source_aliases_count"`
	SourceName         string `json:"source_name"`
	TargetName         string `json:"target_name"`
}

// MergeTagsResult summarizes what happened during a merge.
type MergeTagsResult struct {
	MovedEntityTags   int64 `json:"moved_entity_tags"`
	MovedVotes        int64 `json:"moved_votes"`
	SkippedEntityTags int64 `json:"skipped_entity_tags"`
	SkippedVotes      int64 `json:"skipped_votes"`
	AliasCreated      bool  `json:"alias_created"`
	MovedAliases      int64 `json:"moved_aliases"`
}

// LowQualityTagQueueItem is one row in the admin low-quality-tag review queue.
// Reasons are human-readable identifiers describing which criteria triggered
// inclusion ("orphaned", "aging_unused", "downvoted", "short_name", "long_name").
type LowQualityTagQueueItem struct {
	TagListItem
	Upvotes    int64    `json:"upvotes"`
	Downvotes  int64    `json:"downvotes"`
	Reasons    []string `json:"reasons"`
}

// LowQualityTagQueueResponse is the paginated response for the admin queue.
type LowQualityTagQueueResponse struct {
	Tags  []LowQualityTagQueueItem `json:"tags"`
	Total int64                    `json:"total"`
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
	ListAllAliases(search string, limit, offset int) ([]TagAliasListing, int64, error)
	BulkImportAliases(items []BulkAliasImportItem) (*BulkAliasImportResult, error)

	// Merge
	MergeTags(sourceID, targetID uint, actorUserID uint) (*MergeTagsResult, error)
	PreviewMergeTags(sourceID, targetID uint) (*MergeTagsPreview, error)

	// Tag entities
	GetTagEntities(tagID uint, entityType string, limit, offset int) ([]TaggedEntityItem, int64, error)

	// Tag detail enrichment
	GetTagDetail(tagID uint) (*TagDetailResponse, error)

	// Low-quality queue (PSY-310)
	GetLowQualityTagQueue(limit, offset int) (*LowQualityTagQueueResponse, error)
	SnoozeLowQualityTag(tagID uint, actorUserID uint) error

	// Utility
	SearchTags(query string, limit int, category string) ([]TagSearchResult, error)
	GetTrendingTags(limit int, category string) ([]models.Tag, error)
	PruneDownvotedTags() (int64, error)
}
