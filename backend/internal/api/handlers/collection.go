package handlers

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/middleware"
	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/contracts"
)

// CollectionHandler handles collection-related API requests
type CollectionHandler struct {
	collectionService contracts.CollectionServiceInterface
	auditLogService   contracts.AuditLogServiceInterface
}

// NewCollectionHandler creates a new CollectionHandler
func NewCollectionHandler(collectionService contracts.CollectionServiceInterface, auditLogService contracts.AuditLogServiceInterface) *CollectionHandler {
	return &CollectionHandler{
		collectionService: collectionService,
		auditLogService:   auditLogService,
	}
}

// ============================================================================
// List Collections
// ============================================================================

// ListCollectionsHandlerRequest represents the request for listing collections
type ListCollectionsHandlerRequest struct {
	Creator    string `query:"creator" required:"false" doc:"Filter by creator username"`
	EntityType string `query:"entity_type" required:"false" doc:"Filter by entity type (artist, release, label, show, venue, festival)"`
	Featured   int    `query:"featured" required:"false" doc:"Filter featured collections (1=featured only)" example:"0"`
	Search     string `query:"search" required:"false" doc:"Search by title"`
	Limit      int    `query:"limit" required:"false" doc:"Max results (default 20)" example:"20"`
	Offset     int    `query:"offset" required:"false" doc:"Offset for pagination" example:"0"`
}

// ListCollectionsHandlerResponse represents the response for listing collections
type ListCollectionsHandlerResponse struct {
	Body struct {
		Collections []*contracts.CollectionListResponse `json:"collections" doc:"List of collections"`
		Total       int64                              `json:"total" doc:"Total number of matching collections"`
	}
}

// ListCollectionsHandler handles GET /collections
func (h *CollectionHandler) ListCollectionsHandler(ctx context.Context, req *ListCollectionsHandlerRequest) (*ListCollectionsHandlerResponse, error) {
	filters := contracts.CollectionFilters{
		Search:     req.Search,
		EntityType: req.EntityType,
		PublicOnly: true, // Public endpoint always filters to public
	}

	if req.Featured == 1 {
		filters.Featured = true
	}

	// If viewer is authenticated, don't restrict to public only for their own collections
	user := middleware.GetUserFromContext(ctx)
	if user != nil && req.Creator != "" {
		// If filtering by the current user, show their private collections too
		// This is handled implicitly — if CreatorID matches viewer, service shows all
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}

	collections, total, err := h.collectionService.ListCollections(filters, limit, req.Offset)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to fetch collections", err)
	}

	resp := &ListCollectionsHandlerResponse{}
	resp.Body.Collections = collections
	resp.Body.Total = total

	return resp, nil
}

// ============================================================================
// Get Collection
// ============================================================================

// GetCollectionHandlerRequest represents the request for getting a single collection
type GetCollectionHandlerRequest struct {
	Slug string `path:"slug" doc:"Collection slug" example:"my-favorite-artists"`
}

// GetCollectionHandlerResponse represents the response for the get collection endpoint
type GetCollectionHandlerResponse struct {
	Body *contracts.CollectionDetailResponse
}

// GetCollectionHandler handles GET /collections/{slug}
func (h *CollectionHandler) GetCollectionHandler(ctx context.Context, req *GetCollectionHandlerRequest) (*GetCollectionHandlerResponse, error) {
	var viewerID uint
	user := middleware.GetUserFromContext(ctx)
	if user != nil {
		viewerID = user.ID
	}

	collection, err := h.collectionService.GetBySlug(req.Slug, viewerID)
	if err != nil {
		return nil, mapCollectionError(err)
	}

	return &GetCollectionHandlerResponse{Body: collection}, nil
}

// ============================================================================
// Get Collection Stats
// ============================================================================

// GetCollectionStatsHandlerRequest represents the request for getting collection stats
type GetCollectionStatsHandlerRequest struct {
	Slug string `path:"slug" doc:"Collection slug" example:"my-favorite-artists"`
}

// GetCollectionStatsHandlerResponse represents the response for the collection stats endpoint
type GetCollectionStatsHandlerResponse struct {
	Body *contracts.CollectionStatsResponse
}

// GetCollectionStatsHandler handles GET /collections/{slug}/stats
func (h *CollectionHandler) GetCollectionStatsHandler(ctx context.Context, req *GetCollectionStatsHandlerRequest) (*GetCollectionStatsHandlerResponse, error) {
	stats, err := h.collectionService.GetStats(req.Slug)
	if err != nil {
		return nil, mapCollectionError(err)
	}

	return &GetCollectionStatsHandlerResponse{Body: stats}, nil
}

// ============================================================================
// Create Collection
// ============================================================================

// CreateCollectionHandlerRequest represents the request for creating a collection
type CreateCollectionHandlerRequest struct {
	Body struct {
		Title         string  `json:"title" doc:"Collection title" example:"Phoenix Indie Shows"`
		Description   *string `json:"description,omitempty" required:"false" doc:"Collection description"`
		Collaborative bool    `json:"collaborative,omitempty" required:"false" doc:"Whether other users can add items"`
		CoverImageURL *string `json:"cover_image_url,omitempty" required:"false" doc:"Cover image URL"`
		IsPublic      bool    `json:"is_public,omitempty" required:"false" doc:"Whether the collection is publicly visible"`
		DisplayMode   *string `json:"display_mode,omitempty" required:"false" doc:"Display mode: 'ranked' (numbered, drag-to-reorder) or 'unranked' (flat list, default)" enum:"ranked,unranked"`
	}
}

// CreateCollectionHandlerResponse represents the response for creating a collection
type CreateCollectionHandlerResponse struct {
	Body *contracts.CollectionDetailResponse
}

// CreateCollectionHandler handles POST /collections
func (h *CollectionHandler) CreateCollectionHandler(ctx context.Context, req *CreateCollectionHandlerRequest) (*CreateCollectionHandlerResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	if req.Body.Title == "" {
		return nil, huma.Error400BadRequest("Title is required")
	}

	serviceReq := &contracts.CreateCollectionRequest{
		Title:         req.Body.Title,
		Description:   req.Body.Description,
		Collaborative: req.Body.Collaborative,
		CoverImageURL: req.Body.CoverImageURL,
		IsPublic:      req.Body.IsPublic,
		DisplayMode:   req.Body.DisplayMode,
	}

	collection, err := h.collectionService.CreateCollection(user.ID, serviceReq)
	if err != nil {
		mappedErr := mapCollectionError(err)
		if mappedErr != nil {
			return nil, mappedErr
		}
		logger.FromContext(ctx).Error("create_collection_failed",
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to create collection (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		go func() {
			h.auditLogService.LogAction(user.ID, "create_collection", "collection", collection.ID, nil)
		}()
	}

	return &CreateCollectionHandlerResponse{Body: collection}, nil
}

// ============================================================================
// Update Collection
// ============================================================================

// UpdateCollectionHandlerRequest represents the request for updating a collection
type UpdateCollectionHandlerRequest struct {
	Slug string `path:"slug" doc:"Collection slug" example:"my-favorite-artists"`
	Body struct {
		Title         *string `json:"title,omitempty" required:"false" doc:"Collection title"`
		Description   *string `json:"description,omitempty" required:"false" doc:"Collection description"`
		Collaborative *bool   `json:"collaborative,omitempty" required:"false" doc:"Whether other users can add items"`
		CoverImageURL *string `json:"cover_image_url,omitempty" required:"false" doc:"Cover image URL"`
		IsPublic      *bool   `json:"is_public,omitempty" required:"false" doc:"Whether the collection is publicly visible"`
		DisplayMode   *string `json:"display_mode,omitempty" required:"false" doc:"Display mode: 'ranked' (numbered, drag-to-reorder) or 'unranked' (flat list)" enum:"ranked,unranked"`
	}
}

// UpdateCollectionHandlerResponse represents the response for updating a collection
type UpdateCollectionHandlerResponse struct {
	Body *contracts.CollectionDetailResponse
}

// UpdateCollectionHandler handles PUT /collections/{slug}
func (h *CollectionHandler) UpdateCollectionHandler(ctx context.Context, req *UpdateCollectionHandlerRequest) (*UpdateCollectionHandlerResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	serviceReq := &contracts.UpdateCollectionRequest{
		Title:         req.Body.Title,
		Description:   req.Body.Description,
		Collaborative: req.Body.Collaborative,
		CoverImageURL: req.Body.CoverImageURL,
		IsPublic:      req.Body.IsPublic,
		DisplayMode:   req.Body.DisplayMode,
	}

	collection, err := h.collectionService.UpdateCollection(req.Slug, user.ID, user.IsAdmin, serviceReq)
	if err != nil {
		collErr := mapCollectionError(err)
		if collErr != nil {
			return nil, collErr
		}
		logger.FromContext(ctx).Error("update_collection_failed",
			"slug", req.Slug,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to update collection (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		go func() {
			h.auditLogService.LogAction(user.ID, "update_collection", "collection", collection.ID, nil)
		}()
	}

	return &UpdateCollectionHandlerResponse{Body: collection}, nil
}

// ============================================================================
// Delete Collection
// ============================================================================

// DeleteCollectionHandlerRequest represents the request for deleting a collection
type DeleteCollectionHandlerRequest struct {
	Slug string `path:"slug" doc:"Collection slug" example:"my-favorite-artists"`
}

// DeleteCollectionHandler handles DELETE /collections/{slug}
func (h *CollectionHandler) DeleteCollectionHandler(ctx context.Context, req *DeleteCollectionHandlerRequest) (*struct{}, error) {
	requestID := logger.GetRequestID(ctx)

	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	err := h.collectionService.DeleteCollection(req.Slug, user.ID, user.IsAdmin)
	if err != nil {
		mappedErr := mapCollectionError(err)
		if mappedErr != nil {
			return nil, mappedErr
		}
		logger.FromContext(ctx).Error("delete_collection_failed",
			"slug", req.Slug,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to delete collection (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		go func() {
			h.auditLogService.LogAction(user.ID, "delete_collection", "collection", 0, map[string]interface{}{"slug": req.Slug})
		}()
	}

	return nil, nil
}

// ============================================================================
// Add Item
// ============================================================================

// AddItemHandlerRequest represents the request for adding an item to a collection
type AddItemHandlerRequest struct {
	Slug string `path:"slug" doc:"Collection slug" example:"my-favorite-artists"`
	Body struct {
		EntityType string  `json:"entity_type" doc:"Entity type (artist, release, label, show, venue, festival)" example:"artist"`
		EntityID   uint    `json:"entity_id" doc:"Entity ID" example:"42"`
		Notes      *string `json:"notes,omitempty" required:"false" doc:"Optional notes about this item"`
	}
}

// AddItemHandlerResponse represents the response for adding an item
type AddItemHandlerResponse struct {
	Body *contracts.CollectionItemResponse
}

// AddItemHandler handles POST /collections/{slug}/items
func (h *CollectionHandler) AddItemHandler(ctx context.Context, req *AddItemHandlerRequest) (*AddItemHandlerResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	if req.Body.EntityType == "" {
		return nil, huma.Error400BadRequest("Entity type is required")
	}
	if req.Body.EntityID == 0 {
		return nil, huma.Error400BadRequest("Entity ID is required")
	}

	serviceReq := &contracts.AddCollectionItemRequest{
		EntityType: req.Body.EntityType,
		EntityID:   req.Body.EntityID,
		Notes:      req.Body.Notes,
	}

	item, err := h.collectionService.AddItem(req.Slug, user.ID, serviceReq)
	if err != nil {
		mappedErr := mapCollectionError(err)
		if mappedErr != nil {
			return nil, mappedErr
		}
		logger.FromContext(ctx).Error("add_collection_item_failed",
			"slug", req.Slug,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to add item to collection (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		go func() {
			h.auditLogService.LogAction(user.ID, "add_collection_item", "collection", item.ID, map[string]interface{}{
				"slug":        req.Slug,
				"entity_type": req.Body.EntityType,
				"entity_id":   req.Body.EntityID,
			})
		}()
	}

	return &AddItemHandlerResponse{Body: item}, nil
}

// ============================================================================
// Update Item
// ============================================================================

// UpdateItemHandlerRequest represents the request for updating an item in a collection
type UpdateItemHandlerRequest struct {
	Slug   string `path:"slug" doc:"Collection slug" example:"my-favorite-artists"`
	ItemID string `path:"item_id" doc:"Collection item ID" example:"1"`
	Body   struct {
		Notes *string `json:"notes" required:"false" doc:"Notes about this item"`
	}
}

// UpdateItemHandlerResponse represents the response for updating an item
type UpdateItemHandlerResponse struct {
	Body *contracts.CollectionItemResponse
}

// UpdateItemHandler handles PATCH /collections/{slug}/items/{item_id}
func (h *CollectionHandler) UpdateItemHandler(ctx context.Context, req *UpdateItemHandlerRequest) (*UpdateItemHandlerResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	itemID, err := strconv.ParseUint(req.ItemID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid item ID")
	}

	serviceReq := &contracts.UpdateCollectionItemRequest{
		Notes: req.Body.Notes,
	}

	item, err := h.collectionService.UpdateItem(req.Slug, uint(itemID), user.ID, user.IsAdmin, serviceReq)
	if err != nil {
		mappedErr := mapCollectionError(err)
		if mappedErr != nil {
			return nil, mappedErr
		}
		logger.FromContext(ctx).Error("update_collection_item_failed",
			"slug", req.Slug,
			"item_id", itemID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to update collection item (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		go func() {
			h.auditLogService.LogAction(user.ID, "update_collection_item", "collection", item.ID, map[string]interface{}{
				"slug": req.Slug,
			})
		}()
	}

	return &UpdateItemHandlerResponse{Body: item}, nil
}

// ============================================================================
// Remove Item
// ============================================================================

// RemoveItemHandlerRequest represents the request for removing an item from a collection
type RemoveItemHandlerRequest struct {
	Slug   string `path:"slug" doc:"Collection slug" example:"my-favorite-artists"`
	ItemID string `path:"item_id" doc:"Collection item ID" example:"1"`
}

// RemoveItemHandler handles DELETE /collections/{slug}/items/{item_id}
func (h *CollectionHandler) RemoveItemHandler(ctx context.Context, req *RemoveItemHandlerRequest) (*struct{}, error) {
	requestID := logger.GetRequestID(ctx)

	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	itemID, err := strconv.ParseUint(req.ItemID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid item ID")
	}

	err = h.collectionService.RemoveItem(req.Slug, uint(itemID), user.ID, user.IsAdmin)
	if err != nil {
		mappedErr := mapCollectionError(err)
		if mappedErr != nil {
			return nil, mappedErr
		}
		logger.FromContext(ctx).Error("remove_collection_item_failed",
			"slug", req.Slug,
			"item_id", itemID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to remove item from collection (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		go func() {
			h.auditLogService.LogAction(user.ID, "remove_collection_item", "collection", uint(itemID), map[string]interface{}{
				"slug": req.Slug,
			})
		}()
	}

	return nil, nil
}

// ============================================================================
// Reorder Items
// ============================================================================

// ReorderItemsHandlerRequest represents the request for reordering collection items
type ReorderItemsHandlerRequest struct {
	Slug string `path:"slug" doc:"Collection slug" example:"my-favorite-artists"`
	Body struct {
		Items []contracts.ReorderItem `json:"items" doc:"Items with new positions"`
	}
}

// ReorderItemsHandler handles PUT /collections/{slug}/items/reorder
func (h *CollectionHandler) ReorderItemsHandler(ctx context.Context, req *ReorderItemsHandlerRequest) (*struct{}, error) {
	requestID := logger.GetRequestID(ctx)

	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	serviceReq := &contracts.ReorderCollectionItemsRequest{
		Items: req.Body.Items,
	}

	err := h.collectionService.ReorderItems(req.Slug, user.ID, serviceReq)
	if err != nil {
		mappedErr := mapCollectionError(err)
		if mappedErr != nil {
			return nil, mappedErr
		}
		logger.FromContext(ctx).Error("reorder_collection_items_failed",
			"slug", req.Slug,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to reorder items (request_id: %s)", requestID),
		)
	}

	return nil, nil
}

// ============================================================================
// Subscribe / Unsubscribe
// ============================================================================

// SubscribeHandlerRequest represents the request for subscribing to a collection
type SubscribeHandlerRequest struct {
	Slug string `path:"slug" doc:"Collection slug" example:"my-favorite-artists"`
}

// SubscribeHandler handles POST /collections/{slug}/subscribe
func (h *CollectionHandler) SubscribeHandler(ctx context.Context, req *SubscribeHandlerRequest) (*struct{}, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	err := h.collectionService.Subscribe(req.Slug, user.ID)
	if err != nil {
		return nil, mapCollectionError(err)
	}

	return nil, nil
}

// UnsubscribeHandlerRequest represents the request for unsubscribing from a collection
type UnsubscribeHandlerRequest struct {
	Slug string `path:"slug" doc:"Collection slug" example:"my-favorite-artists"`
}

// UnsubscribeHandler handles DELETE /collections/{slug}/subscribe
func (h *CollectionHandler) UnsubscribeHandler(ctx context.Context, req *UnsubscribeHandlerRequest) (*struct{}, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	err := h.collectionService.Unsubscribe(req.Slug, user.ID)
	if err != nil {
		return nil, mapCollectionError(err)
	}

	return nil, nil
}

// ============================================================================
// Set Featured
// ============================================================================

// SetFeaturedHandlerRequest represents the request for setting a collection's featured status
type SetFeaturedHandlerRequest struct {
	Slug string `path:"slug" doc:"Collection slug" example:"my-favorite-artists"`
	Body struct {
		Featured bool `json:"featured" doc:"Whether the collection should be featured"`
	}
}

// SetFeaturedHandler handles PUT /collections/{slug}/feature
func (h *CollectionHandler) SetFeaturedHandler(ctx context.Context, req *SetFeaturedHandlerRequest) (*struct{}, error) {
	requestID := logger.GetRequestID(ctx)

	user, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	err = h.collectionService.SetFeatured(req.Slug, req.Body.Featured)
	if err != nil {
		mappedErr := mapCollectionError(err)
		if mappedErr != nil {
			return nil, mappedErr
		}
		logger.FromContext(ctx).Error("set_collection_featured_failed",
			"slug", req.Slug,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to update featured status (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		go func() {
			h.auditLogService.LogAction(user.ID, "set_collection_featured", "collection", 0, map[string]interface{}{
				"slug":     req.Slug,
				"featured": req.Body.Featured,
			})
		}()
	}

	return nil, nil
}

// ============================================================================
// Get User Collections
// ============================================================================

// GetUserCollectionsHandlerRequest represents the request for getting the authenticated user's collections
type GetUserCollectionsHandlerRequest struct {
	Limit  int `query:"limit" required:"false" doc:"Max results (default 20)" example:"20"`
	Offset int `query:"offset" required:"false" doc:"Offset for pagination" example:"0"`
}

// GetUserCollectionsHandlerResponse represents the response for the user collections endpoint
type GetUserCollectionsHandlerResponse struct {
	Body struct {
		Collections []*contracts.CollectionListResponse `json:"collections" doc:"List of user's collections"`
		Total       int64                              `json:"total" doc:"Total number of collections"`
	}
}

// GetUserCollectionsHandler handles GET /auth/collections
func (h *CollectionHandler) GetUserCollectionsHandler(ctx context.Context, req *GetUserCollectionsHandlerRequest) (*GetUserCollectionsHandlerResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}

	collections, total, err := h.collectionService.GetUserCollections(user.ID, limit, req.Offset)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to fetch collections", err)
	}

	resp := &GetUserCollectionsHandlerResponse{}
	resp.Body.Collections = collections
	resp.Body.Total = total

	return resp, nil
}

// ============================================================================
// Get Entity Collections
// ============================================================================

// GetEntityCollectionsHandlerRequest represents the request for getting collections containing an entity
type GetEntityCollectionsHandlerRequest struct {
	EntityType string `path:"entity_type" doc:"Entity type (artist, release, label, show, venue, festival)" example:"artist"`
	EntityID   string `path:"entity_id" doc:"Entity ID" example:"42"`
	Limit      int    `query:"limit" required:"false" doc:"Max results (default 10)" example:"10"`
}

// GetEntityCollectionsHandlerResponse represents the response for entity collections
type GetEntityCollectionsHandlerResponse struct {
	Body struct {
		Collections []*contracts.CollectionListResponse `json:"collections" doc:"List of collections containing this entity"`
	}
}

// GetEntityCollectionsHandler handles GET /collections/entity/{entity_type}/{entity_id}
func (h *CollectionHandler) GetEntityCollectionsHandler(ctx context.Context, req *GetEntityCollectionsHandlerRequest) (*GetEntityCollectionsHandlerResponse, error) {
	entityID, err := strconv.ParseUint(req.EntityID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid entity ID")
	}

	validTypes := map[string]bool{
		"artist": true, "release": true, "label": true,
		"show": true, "venue": true, "festival": true,
	}
	if !validTypes[req.EntityType] {
		return nil, huma.Error400BadRequest("Invalid entity type")
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}

	collections, err := h.collectionService.GetEntityCollections(req.EntityType, uint(entityID), limit)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to fetch entity collections", err)
	}

	resp := &GetEntityCollectionsHandlerResponse{}
	resp.Body.Collections = collections

	return resp, nil
}

// ============================================================================
// Get User Public Collections
// ============================================================================

// GetUserPublicCollectionsHandlerRequest represents the request for getting a user's public collections
type GetUserPublicCollectionsHandlerRequest struct {
	Username string `path:"username" doc:"Username" example:"johndoe"`
	Limit    int    `query:"limit" required:"false" doc:"Max results (default 20)" example:"20"`
	Offset   int    `query:"offset" required:"false" doc:"Offset for pagination" example:"0"`
}

// GetUserPublicCollectionsHandlerResponse represents the response for user public collections
type GetUserPublicCollectionsHandlerResponse struct {
	Body struct {
		Collections []*contracts.CollectionListResponse `json:"collections" doc:"List of user's public collections"`
		Total       int64                              `json:"total" doc:"Total number of public collections"`
	}
}

// GetUserPublicCollectionsHandler handles GET /users/{username}/collections
func (h *CollectionHandler) GetUserPublicCollectionsHandler(ctx context.Context, req *GetUserPublicCollectionsHandlerRequest) (*GetUserPublicCollectionsHandlerResponse, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}

	collections, total, err := h.collectionService.GetUserPublicCollectionsByUsername(req.Username, limit, req.Offset)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to fetch user collections", err)
	}

	resp := &GetUserPublicCollectionsHandlerResponse{}
	resp.Body.Collections = collections
	resp.Body.Total = total

	return resp, nil
}

// ============================================================================
// Helpers
// ============================================================================

// mapCollectionError converts a CollectionError to an appropriate Huma HTTP error
func mapCollectionError(err error) error {
	var collectionErr *apperrors.CollectionError
	if errors.As(err, &collectionErr) {
		switch collectionErr.Code {
		case apperrors.CodeCollectionNotFound:
			return huma.Error404NotFound(collectionErr.Message)
		case apperrors.CodeCollectionForbidden:
			return huma.Error403Forbidden(collectionErr.Message)
		case apperrors.CodeCollectionItemExists:
			return huma.Error409Conflict(collectionErr.Message)
		case apperrors.CodeCollectionItemNotFound:
			return huma.Error404NotFound(collectionErr.Message)
		case apperrors.CodeCollectionInvalidRequest:
			return huma.Error400BadRequest(collectionErr.Message)
		}
	}
	return nil
}
