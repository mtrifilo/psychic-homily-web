package contracts

import (
	"time"

	"psychic-homily-backend/internal/models"
)

// MaxCollectionDescriptionLength is the maximum length, in bytes, accepted for
// a collection's `description` field. Aliases models.MaxCommentBodyLength
// (10,000) so the markdown editor experience and limits are consistent across
// comments, field notes, and collections (PSY-349) — there is no parallel
// limit to keep in sync.
const MaxCollectionDescriptionLength = models.MaxCommentBodyLength

// MaxCollectionItemNotesLength is the maximum length, in bytes, accepted for
// per-item `notes` on a collection item. Aliases models.MaxCommentBodyLength.
const MaxCollectionItemNotesLength = models.MaxCommentBodyLength

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
	ID               uint                     `json:"id"`
	Title            string                   `json:"title"`
	Slug             string                   `json:"slug"`
	Description      string                   `json:"description"`
	DescriptionHTML  string                   `json:"description_html,omitempty"`
	CreatorID        uint                     `json:"creator_id"`
	CreatorName      string                   `json:"creator_name"`
	Collaborative    bool                     `json:"collaborative"`
	CoverImageURL    *string                  `json:"cover_image_url"`
	IsPublic         bool                     `json:"is_public"`
	IsFeatured       bool                     `json:"is_featured"`
	DisplayMode      string                   `json:"display_mode"`
	ItemCount        int                      `json:"item_count"`
	SubscriberCount  int                      `json:"subscriber_count"`
	ContributorCount int                      `json:"contributor_count"`
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
	UserLikesThis bool      `json:"user_likes_this"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
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
	ID               uint           `json:"id"`
	Title            string         `json:"title"`
	Slug             string         `json:"slug"`
	Description      string         `json:"description"`
	DescriptionHTML  string         `json:"description_html,omitempty"`
	CreatorID        uint           `json:"creator_id"`
	CreatorName      string         `json:"creator_name"`
	Collaborative    bool           `json:"collaborative"`
	CoverImageURL    *string        `json:"cover_image_url"`
	IsPublic         bool           `json:"is_public"`
	IsFeatured       bool           `json:"is_featured"`
	DisplayMode      string         `json:"display_mode"`
	ItemCount        int            `json:"item_count"`
	SubscriberCount  int            `json:"subscriber_count"`
	ContributorCount int            `json:"contributor_count"`
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
	UserLikesThis bool      `json:"user_likes_this"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// CollectionItemResponse represents an item in a collection.
// Notes is the raw markdown; NotesHTML is sanitized rendered HTML, computed on
// read. Existing plain-text notes still render correctly because plain text is
// valid markdown, and the sanitizer guarantees safe output for any stored row.
type CollectionItemResponse struct {
	ID            uint      `json:"id"`
	EntityType    string    `json:"entity_type"`
	EntityID      uint      `json:"entity_id"`
	EntityName    string    `json:"entity_name"`
	EntitySlug    string    `json:"entity_slug"`
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
	GetEntityCollections(entityType string, entityID uint, limit int) ([]*CollectionListResponse, error)
	GetUserPublicCollections(userID uint, limit, offset int) ([]*CollectionListResponse, int64, error)
	GetUserPublicCollectionsByUsername(username string, limit, offset int) ([]*CollectionListResponse, int64, error)
	SetFeatured(slug string, featured bool) error
}
