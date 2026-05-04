package contracts

import (
	engagementm "psychic-homily-backend/internal/models/engagement"
	"time"
)

// MaxCollectionDescriptionLength is the maximum length, in bytes, accepted for
// a collection's `description` field. Aliases engagementm.MaxCommentBodyLength
// (10,000) so the markdown editor experience and limits are consistent across
// comments, field notes, and collections (PSY-349) — there is no parallel
// limit to keep in sync.
const MaxCollectionDescriptionLength = engagementm.MaxCommentBodyLength

// MaxCollectionItemNotesLength is the maximum length, in bytes, accepted for
// per-item `notes` on a collection item. Aliases engagementm.MaxCommentBodyLength.
const MaxCollectionItemNotesLength = engagementm.MaxCommentBodyLength

// ──────────────────────────────────────────────
// Collection types
// ──────────────────────────────────────────────

// CreateCollectionRequest represents the data needed to create a new collection
type CreateCollectionRequest struct {
	Title         string  `json:"title" validate:"required"`
	Description   *string `json:"description"`
	Collaborative bool    `json:"collaborative"`
	CoverImageURL *string `json:"cover_image_url"`
	IsPublic      bool    `json:"is_public"`
	DisplayMode   *string `json:"display_mode"`
}

// UpdateCollectionRequest represents the data that can be updated on a collection
type UpdateCollectionRequest struct {
	Title         *string `json:"title"`
	Description   *string `json:"description"`
	Collaborative *bool   `json:"collaborative"`
	CoverImageURL *string `json:"cover_image_url"`
	IsPublic      *bool   `json:"is_public"`
	DisplayMode   *string `json:"display_mode"`
}

// AddCollectionItemRequest represents the data needed to add an item to a collection
type AddCollectionItemRequest struct {
	EntityType string  `json:"entity_type" validate:"required"`
	EntityID   uint    `json:"entity_id" validate:"required"`
	Notes      *string `json:"notes"`
}

// ReorderCollectionItemsRequest represents a request to reorder items in a collection
type ReorderCollectionItemsRequest struct {
	Items []ReorderItem `json:"items" validate:"required"`
}

// ReorderItem represents a single item's new position
type ReorderItem struct {
	ItemID   uint `json:"item_id"`
	Position int  `json:"position"`
}

// CollectionFilters represents filters for listing collections
type CollectionFilters struct {
	CreatorID  uint
	EntityType string
	Featured   bool
	// Search is a case-insensitive ILIKE-substring query matched against
	// the collection title, description, any item's notes, and any applied
	// tag's name (or alias). Empty string disables search. When the default
	// sort is in effect, results are tier-ranked title > description >
	// notes > tag, then by updated_at DESC. PSY-355.
	Search     string
	PublicOnly bool
	// Sort selects the ordering for list results. Recognized values:
	//   ""        — default: updated_at DESC
	//   "popular" — HN-gravity: like_count / POWER(age_hours+2, 1.8) DESC
	//               with updated_at DESC as a tiebreaker. PSY-352.
	// Unknown values are rejected at the handler layer.
	Sort string
	// ViewerID is the authenticated viewer's user ID (or 0 when anonymous).
	// Carried on the filter struct so we can populate UserLikesThis on each
	// list row without changing the function signature. PSY-352.
	ViewerID uint
	// Tag is the slug of a single tag to filter the listing by (PSY-354).
	// Empty string means no tag filter. Multi-tag filtering is intentionally
	// out of scope for the MVP — the URL surface stays `?tag=<slug>`.
	Tag string
}

// CollectionSortPopular is the recognized sort value for the HN-gravity
// "popular" ordering. PSY-352.
const CollectionSortPopular = "popular"

// CollectionDetailResponse represents the full collection data returned to clients.
// Description is the raw markdown source; DescriptionHTML is rendered + sanitized
// HTML produced on each read via utils.MarkdownRenderer (goldmark + bluemonday),
// matching the comment-system policy. Description (raw) is preserved so editors
// can re-populate the textarea without re-parsing HTML back to markdown.
type CollectionDetailResponse struct {
	ID              uint   `json:"id"`
	Title           string `json:"title"`
	Slug            string `json:"slug"`
	Description     string `json:"description"`
	DescriptionHTML string `json:"description_html,omitempty"`
	CreatorID       uint   `json:"creator_id"`
	CreatorName     string `json:"creator_name"`
	// CreatorUsername is the creator's username when set, used by the
	// frontend to link the attribution to /users/:username. Pointer so the
	// JSON encodes null (not "") for accounts that never set a username —
	// the frontend renders the name as plain text in that case (PSY-353).
	CreatorUsername  *string `json:"creator_username"`
	Collaborative    bool    `json:"collaborative"`
	CoverImageURL    *string `json:"cover_image_url"`
	IsPublic         bool    `json:"is_public"`
	IsFeatured       bool    `json:"is_featured"`
	DisplayMode      string  `json:"display_mode"`
	ItemCount        int     `json:"item_count"`
	SubscriberCount  int     `json:"subscriber_count"`
	ContributorCount int     `json:"contributor_count"`
	// ForksCount is a public social signal — number of collections that
	// declared this one as their `forked_from_collection_id`. Computed live
	// on read (see CollectionService.batchCountForks). PSY-351.
	ForksCount int `json:"forks_count"`
	// ForkedFromCollectionID is non-nil when this collection was cloned.
	// May be set even if ForkedFrom is nil (when the source was deleted).
	ForkedFromCollectionID *uint `json:"forked_from_collection_id,omitempty"`
	// ForkedFrom is a minimal snapshot of the source collection for
	// rendering inline attribution. nil when the source was deleted
	// (FK was set to NULL) or this collection wasn't forked.
	ForkedFrom   *ForkedFromInfo          `json:"forked_from,omitempty"`
	Items        []CollectionItemResponse `json:"items"`
	IsSubscribed bool                     `json:"is_subscribed"`
	// LikeCount is the aggregate count of likes on this collection. PSY-352.
	LikeCount int `json:"like_count"`
	// UserLikesThis is true when the authenticated viewer has liked this
	// collection. Always false for anonymous viewers. PSY-352.
	UserLikesThis bool `json:"user_likes_this"`
	// Tags applied to this collection (PSY-354). Always non-nil — empty
	// array when the collection has no tags. Reuses the polymorphic
	// EntityTagResponse so the same chip component renders here as on
	// artist/release/etc detail pages.
	Tags      []EntityTagResponse `json:"tags"`
	CreatedAt time.Time           `json:"created_at"`
	UpdatedAt time.Time           `json:"updated_at"`
}

// ForkedFromInfo carries the minimum data needed to render the
// "Forked from [Title] by [Curator]" inline attribution as a link.
// Distinct from CollectionListResponse to keep the payload small and
// to avoid recursive nesting (a fork's source may itself be a fork).
type ForkedFromInfo struct {
	ID          uint   `json:"id"`
	Title       string `json:"title"`
	Slug        string `json:"slug"`
	CreatorID   uint   `json:"creator_id"`
	CreatorName string `json:"creator_name"`
}

// CollectionListResponse represents a collection in list views (without items).
// DescriptionHTML mirrors the detail response — sanitized markdown render of
// Description, computed on read. See CollectionDetailResponse for rationale.
type CollectionListResponse struct {
	ID              uint   `json:"id"`
	Title           string `json:"title"`
	Slug            string `json:"slug"`
	Description     string `json:"description"`
	DescriptionHTML string `json:"description_html,omitempty"`
	CreatorID       uint   `json:"creator_id"`
	CreatorName     string `json:"creator_name"`
	// CreatorUsername mirrors CollectionDetailResponse — see PSY-353.
	CreatorUsername  *string `json:"creator_username"`
	Collaborative    bool    `json:"collaborative"`
	CoverImageURL    *string `json:"cover_image_url"`
	IsPublic         bool    `json:"is_public"`
	IsFeatured       bool    `json:"is_featured"`
	DisplayMode      string  `json:"display_mode"`
	ItemCount        int     `json:"item_count"`
	SubscriberCount  int     `json:"subscriber_count"`
	ContributorCount int     `json:"contributor_count"`
	// ForksCount is a public social signal exposed on list cards too,
	// so original collections can advertise how often they've been forked.
	// PSY-351.
	ForksCount             int            `json:"forks_count"`
	ForkedFromCollectionID *uint          `json:"forked_from_collection_id,omitempty"`
	EntityTypeCounts       map[string]int `json:"entity_type_counts"`
	// NewSinceLastVisit is the count of items added to this collection after
	// the viewer's `last_visited_at` cursor on the subscription. Always 0
	// for collections the viewer is not subscribed to (or for unauthed
	// viewers); only populated by the user-collections endpoint. PSY-350.
	NewSinceLastVisit int `json:"new_since_last_visit,omitempty"`
	// LikeCount is the aggregate count of likes on this collection. PSY-352.
	LikeCount int `json:"like_count"`
	// UserLikesThis is true when the authenticated viewer has liked this
	// collection. Always false for anonymous viewers. PSY-352.
	UserLikesThis bool `json:"user_likes_this"`
	// Tags applied to this collection (PSY-354). Reuses TagSummary so list
	// rows stay lightweight (no vote counts on cards). Always non-nil; empty
	// array when the collection has no tags.
	Tags      []TagSummary `json:"tags"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}

// CollectionItemResponse represents an item in a collection.
// Notes is the raw markdown; NotesHTML is sanitized rendered HTML, computed on
// read. Existing plain-text notes still render correctly because plain text is
// valid markdown, and the sanitizer guarantees safe output for any stored row.
//
// ImageURL is the entity's representative image (PSY-360, "visual grid"
// rendering on the collection-detail page). Surfaced for all six entity
// types; column name varies by domain:
//   - release  → cover_art_url
//   - festival → flyer_url
//   - artist / venue / show / label → image_url (PSY-521)
//
// Rows where the curator has not added a URL surface as nil and the frontend
// renders a typed Lucide icon as a fallback.
type CollectionItemResponse struct {
	ID            uint      `json:"id"`
	EntityType    string    `json:"entity_type"`
	EntityID      uint      `json:"entity_id"`
	EntityName    string    `json:"entity_name"`
	EntitySlug    string    `json:"entity_slug"`
	ImageURL      *string   `json:"image_url"`
	Position      int       `json:"position"`
	AddedByUserID uint      `json:"added_by_user_id"`
	AddedByName   string    `json:"added_by_name"`
	Notes         *string   `json:"notes"`
	NotesHTML     string    `json:"notes_html,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// CollectionStatsResponse represents statistics for a collection
type CollectionStatsResponse struct {
	ItemCount        int            `json:"item_count"`
	SubscriberCount  int            `json:"subscriber_count"`
	ContributorCount int            `json:"contributor_count"`
	EntityTypeCounts map[string]int `json:"entity_type_counts"`
}

// UpdateCollectionItemRequest represents the data that can be updated on a collection item
type UpdateCollectionItemRequest struct {
	Notes *string `json:"notes"`
}

// CollectionLikeResponse is returned by POST/DELETE on the /like endpoint.
// Surfaces only aggregates and the caller's like state; the per-user list
// of likes is intentionally not exposed (privacy decision, PSY-352).
type CollectionLikeResponse struct {
	LikeCount     int  `json:"like_count"`
	UserLikesThis bool `json:"user_likes_this"`
}

// MaxCollectionTags is the cap on tag applications per collection (PSY-354).
// Mirrors the modest tag bar the rest of the project applies to entity
// tagging — collections aren't the canonical taxonomy, and a 10-tag ceiling
// keeps cards readable while still allowing meaningful classification.
const MaxCollectionTags = 10

// AddCollectionTagRequest is the body for POST /collections/{slug}/tags
// (PSY-354). Mirrors the Add-Tag-To-Entity endpoint: callers either pass a
// known tag_id or a free-form tag_name (with alias resolution + inline
// creation gated by trust tier in the tag service).
type AddCollectionTagRequest struct {
	TagID    uint   `json:"tag_id"`
	TagName  string `json:"tag_name"`
	Category string `json:"category"`
}

// AddCollectionTagResponse returns the post-mutation tag list so the
// frontend can refresh chips without a follow-up GET.
type AddCollectionTagResponse struct {
	Tags []EntityTagResponse `json:"tags"`
}

// ──────────────────────────────────────────────
// Collection graph (PSY-366, PSY-555) — multi-type per-collection knowledge graph
// ──────────────────────────────────────────────
//
// PSY-555 broadened the graph from artist-only to a full multi-type graph
// (Option B in the ticket): every collection item — artist, venue, show,
// release, label, festival — becomes a node. Edges are derived from the
// existing relational model so no new storage is needed:
//
//   - artist ↔ artist  : stored `artist_relationships` rows (PSY-366 origin)
//   - artist ↔ venue   : artist played the venue, via show_artists ⋈ show_venues
//   - artist ↔ release : artist made the release, via artist_releases
//   - artist ↔ label   : artist signed to the label, via artist_labels
//   - artist ↔ festival: artist played the festival, via festival_artists
//   - show   ↔ artist  : show's lineup, via show_artists
//   - show   ↔ venue   : show's location, via show_venues
//
// Edges are emitted ONLY when both endpoints are present in the collection.
// We never invent phantom nodes for relationships that pull in entities the
// curator did not choose — that would explode the graph. Edges between
// non-artist nodes (venue↔festival etc.) are intentionally out of scope.
//
// Mirrors the {Info, Nodes, Links} shape so a shared frontend ForceGraphView
// can render the payload — no clusters in v1 (collections have no natural
// cluster signal). See docs/research/knowledge-graph-viz-prior-art.md §5.4
// for the entry-point-invisibility motivation.

// CollectionGraphResponse is the payload for GET /collections/{slug}/graph.
// Returned for any collection; collections with no items return empty
// nodes/links (200, not 404).
type CollectionGraphResponse struct {
	Collection CollectionGraphInfo   `json:"collection"`
	Nodes      []CollectionGraphNode `json:"nodes"`
	Links      []CollectionGraphLink `json:"links"`
}

// CollectionGraphInfo holds collection metadata for the graph response.
//
// EntityCounts is the per-type breakdown of nodes returned in the response
// (artist / venue / show / release / label / festival). The frontend uses
// this to render the subtitle copy ("1 artist · 1 venue · 1 release"). Always
// non-nil; empty map when the collection has no items.
//
// ArtistCount is preserved for backward compatibility with PSY-366-era
// callers and equals EntityCounts["artist"].
type CollectionGraphInfo struct {
	Slug         string         `json:"slug"`
	Name         string         `json:"name"`
	ArtistCount  int            `json:"artist_count"` // distinct artists in the collection (includes isolates)
	EdgeCount    int            `json:"edge_count"`   // total edges in the response (post type-filter)
	EntityCounts map[string]int `json:"entity_counts"`
}

// CollectionGraphNode represents a single collection item in the graph.
//
// EntityType is one of the six community.CollectionEntity* constants:
// "artist", "venue", "show", "release", "label", "festival". Frontend uses
// it to pick the node icon and color.
//
// City and State are populated when the underlying record has them (artist,
// venue) and empty otherwise (releases / labels / shows / festivals don't
// always have a stable place — keep the field in the union shape and let
// the renderer omit it).
//
// UpcomingShowCount is meaningful only for artists; non-artist nodes return
// 0. Kept on the shared shape so the frontend ForceGraphView (PSY-365)
// renders the same node type regardless of entity.
type CollectionGraphNode struct {
	ID                uint   `json:"id"`
	EntityType        string `json:"entity_type"`
	Name              string `json:"name"`
	Slug              string `json:"slug"`
	City              string `json:"city,omitempty"`
	State             string `json:"state,omitempty"`
	UpcomingShowCount int    `json:"upcoming_show_count"`
	IsIsolate         bool   `json:"is_isolate"` // true when node has no in-set edges (post type-filter)
}

// CollectionGraphLink represents an edge in the collection graph.
//
// Type uses the existing artist-relationship grammar where it applies
// (shared_bills, shared_label, member_of, side_project, similar,
// radio_cooccurrence) and adds derived types for the multi-type cases:
//   - "played_at"     : artist played the venue (artist ↔ venue)
//   - "discography"   : artist made the release (artist ↔ release)
//   - "signed_to"     : artist signed to the label (artist ↔ label)
//   - "lineup"        : artist played the festival (artist ↔ festival)
//   - "show_lineup"   : show ↔ artist (the show's billed acts)
//   - "show_venue"    : show ↔ venue (the show's location)
//
// SourceID/TargetID are NODE IDs unique within the response — for two
// different entity types with the same DB ID (e.g. artist 5 and venue 5),
// the IDs are namespaced by buildNodeID below to keep them distinct.
//
// Voting/user-vote data are intentionally omitted — collection graph is
// read-only, like scene graph.
type CollectionGraphLink struct {
	SourceID uint    `json:"source_id"`
	TargetID uint    `json:"target_id"`
	Type     string  `json:"type"`
	Score    float64 `json:"score"`
	Detail   any     `json:"detail,omitempty"`
}

// CollectionServiceInterface defines the contract for collection operations.
type CollectionServiceInterface interface {
	CreateCollection(creatorID uint, req *CreateCollectionRequest) (*CollectionDetailResponse, error)
	CloneCollection(srcSlug string, callerID uint) (*CollectionDetailResponse, error)
	GetBySlug(slug string, viewerID uint) (*CollectionDetailResponse, error)
	ListCollections(filters CollectionFilters, limit, offset int) ([]*CollectionListResponse, int64, error)
	UpdateCollection(slug string, userID uint, isAdmin bool, req *UpdateCollectionRequest) (*CollectionDetailResponse, error)
	DeleteCollection(slug string, userID uint, isAdmin bool) error
	AddItem(slug string, userID uint, req *AddCollectionItemRequest) (*CollectionItemResponse, error)
	UpdateItem(slug string, itemID uint, userID uint, isAdmin bool, req *UpdateCollectionItemRequest) (*CollectionItemResponse, error)
	RemoveItem(slug string, itemID uint, userID uint, isAdmin bool) error
	ReorderItems(slug string, userID uint, req *ReorderCollectionItemsRequest) error
	Subscribe(slug string, userID uint) error
	Unsubscribe(slug string, userID uint) error
	MarkVisited(slug string, userID uint) error
	// Like records a user's like on the collection. Idempotent — calling
	// twice for the same (user, collection) is a no-op. PSY-352.
	Like(slug string, userID uint) (*CollectionLikeResponse, error)
	// Unlike removes a user's like on the collection. Idempotent — calling
	// when the like doesn't exist is a no-op. PSY-352.
	Unlike(slug string, userID uint) (*CollectionLikeResponse, error)
	GetStats(slug string) (*CollectionStatsResponse, error)
	GetUserCollections(userID uint, limit, offset int) ([]*CollectionListResponse, int64, error)
	// GetUserCollectionsContainingEntity returns the IDs of the user's
	// editable collections (creator + subscribed) that already contain
	// the supplied entity. Backs the multi-select Add-to-Collection
	// popover (PSY-359). Returns empty slice for userID == 0.
	GetUserCollectionsContainingEntity(userID uint, entityType string, entityID uint) ([]uint, error)
	GetEntityCollections(entityType string, entityID uint, limit int) ([]*CollectionListResponse, error)
	GetUserPublicCollections(userID uint, limit, offset int) ([]*CollectionListResponse, int64, error)
	GetUserPublicCollectionsByUsername(username string, limit, offset int) ([]*CollectionListResponse, int64, error)
	SetFeatured(slug string, featured bool) error
	// AddTagToCollection applies a tag to a collection (PSY-354). Caller must
	// have edit access (creator OR collaborative-and-authenticated, mirroring
	// AddItem). Enforces MaxCollectionTags. Returns the post-mutation tag
	// list.
	AddTagToCollection(slug string, userID uint, req *AddCollectionTagRequest) (*AddCollectionTagResponse, error)
	// RemoveTagFromCollection removes a tag from a collection (PSY-354).
	// Same edit-access rule as AddTagToCollection.
	RemoveTagFromCollection(slug string, tagID uint, userID uint) error
	// GetCollectionGraph returns the artist-relationship subgraph for the
	// collection's artist items. Visibility gate mirrors GetBySlug
	// (private → ErrCollectionForbidden unless viewer is creator). PSY-366.
	GetCollectionGraph(slug string, viewerID uint, types []string) (*CollectionGraphResponse, error)
}
