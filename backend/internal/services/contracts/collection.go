package contracts

import "time"

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
}

// UpdateCollectionRequest represents the data that can be updated on a collection
type UpdateCollectionRequest struct {
	Title         *string `json:"title"`
	Description   *string `json:"description"`
	Collaborative *bool   `json:"collaborative"`
	CoverImageURL *string `json:"cover_image_url"`
	IsPublic      *bool   `json:"is_public"`
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
}

// CollectionDetailResponse represents the full collection data returned to clients
type CollectionDetailResponse struct {
	ID               uint                     `json:"id"`
	Title            string                   `json:"title"`
	Slug             string                   `json:"slug"`
	Description      string                   `json:"description"`
	CreatorID        uint                     `json:"creator_id"`
	CreatorName      string                   `json:"creator_name"`
	Collaborative    bool                     `json:"collaborative"`
	CoverImageURL    *string                  `json:"cover_image_url"`
	IsPublic         bool                     `json:"is_public"`
	IsFeatured       bool                     `json:"is_featured"`
	ItemCount        int                      `json:"item_count"`
	SubscriberCount  int                      `json:"subscriber_count"`
	ContributorCount int                      `json:"contributor_count"`
	Items            []CollectionItemResponse `json:"items"`
	IsSubscribed     bool                     `json:"is_subscribed"`
	CreatedAt        time.Time                `json:"created_at"`
	UpdatedAt        time.Time                `json:"updated_at"`
}

// CollectionListResponse represents a collection in list views (without items)
type CollectionListResponse struct {
	ID               uint      `json:"id"`
	Title            string    `json:"title"`
	Slug             string    `json:"slug"`
	Description      string    `json:"description"`
	CreatorID        uint      `json:"creator_id"`
	CreatorName      string    `json:"creator_name"`
	Collaborative    bool      `json:"collaborative"`
	CoverImageURL    *string   `json:"cover_image_url"`
	IsPublic         bool      `json:"is_public"`
	IsFeatured       bool      `json:"is_featured"`
	ItemCount        int       `json:"item_count"`
	SubscriberCount  int       `json:"subscriber_count"`
	ContributorCount int       `json:"contributor_count"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// CollectionItemResponse represents an item in a collection
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
	CreatedAt     time.Time `json:"created_at"`
}

// CollectionStatsResponse represents statistics for a collection
type CollectionStatsResponse struct {
	ItemCount        int            `json:"item_count"`
	SubscriberCount  int            `json:"subscriber_count"`
	ContributorCount int            `json:"contributor_count"`
	EntityTypeCounts map[string]int `json:"entity_type_counts"`
}

// CollectionServiceInterface defines the contract for collection operations.
type CollectionServiceInterface interface {
	CreateCollection(creatorID uint, req *CreateCollectionRequest) (*CollectionDetailResponse, error)
	GetBySlug(slug string, viewerID uint) (*CollectionDetailResponse, error)
	ListCollections(filters CollectionFilters, limit, offset int) ([]*CollectionListResponse, int64, error)
	UpdateCollection(slug string, userID uint, isAdmin bool, req *UpdateCollectionRequest) (*CollectionDetailResponse, error)
	DeleteCollection(slug string, userID uint, isAdmin bool) error
	AddItem(slug string, userID uint, req *AddCollectionItemRequest) (*CollectionItemResponse, error)
	RemoveItem(slug string, itemID uint, userID uint, isAdmin bool) error
	ReorderItems(slug string, userID uint, req *ReorderCollectionItemsRequest) error
	Subscribe(slug string, userID uint) error
	Unsubscribe(slug string, userID uint) error
	MarkVisited(slug string, userID uint) error
	GetStats(slug string) (*CollectionStatsResponse, error)
	GetUserCollections(userID uint, limit, offset int) ([]*CollectionListResponse, int64, error)
	SetFeatured(slug string, featured bool) error
}
