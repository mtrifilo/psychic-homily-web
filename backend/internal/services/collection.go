package services

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/utils"
)

// CollectionService handles collection-related business logic
type CollectionService struct {
	db *gorm.DB
}

// NewCollectionService creates a new collection service
func NewCollectionService(database *gorm.DB) *CollectionService {
	if database == nil {
		database = db.GetDB()
	}
	return &CollectionService{
		db: database,
	}
}

// CreateCollection creates a new collection and auto-subscribes the creator
func (s *CollectionService) CreateCollection(creatorID uint, req *contracts.CreateCollectionRequest) (*contracts.CollectionDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Generate unique slug from title
	baseSlug := utils.GenerateArtistSlug(req.Title)
	slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
		var count int64
		s.db.Model(&models.Collection{}).Where("slug = ?", candidate).Count(&count)
		return count > 0
	})

	description := ""
	if req.Description != nil {
		description = *req.Description
	}

	displayMode := models.CollectionDisplayModeUnranked
	if req.DisplayMode != nil && *req.DisplayMode != "" {
		if !models.IsValidCollectionDisplayMode(*req.DisplayMode) {
			return nil, apperrors.ErrCollectionInvalidRequest(
				fmt.Sprintf("display_mode must be 'ranked' or 'unranked', got %q", *req.DisplayMode),
			)
		}
		displayMode = *req.DisplayMode
	}

	collection := &models.Collection{
		Title:         req.Title,
		Slug:          slug,
		Description:   description,
		CreatorID:     creatorID,
		Collaborative: true, // Create with true defaults to avoid GORM zero-value skip
		CoverImageURL: req.CoverImageURL,
		IsPublic:      true,
		IsFeatured:    false,
		DisplayMode:   displayMode,
	}

	if err := s.db.Create(collection).Error; err != nil {
		return nil, fmt.Errorf("failed to create collection: %w", err)
	}

	// GORM bool gotcha: false is zero-value, GORM skips it on Create → DB default wins.
	// Fix: create with true, then update to false for any bool fields that should be false.
	boolUpdates := map[string]interface{}{}
	if !req.Collaborative {
		boolUpdates["collaborative"] = false
	}
	if !req.IsPublic {
		boolUpdates["is_public"] = false
	}
	if len(boolUpdates) > 0 {
		if err := s.db.Model(collection).Updates(boolUpdates).Error; err != nil {
			return nil, fmt.Errorf("failed to update collection bools: %w", err)
		}
	}

	// Auto-subscribe creator
	now := time.Now()
	subscriber := &models.CollectionSubscriber{
		CollectionID:  collection.ID,
		UserID:        creatorID,
		LastVisitedAt: &now,
	}
	if err := s.db.Create(subscriber).Error; err != nil {
		// Non-fatal: collection was created successfully
		_ = err
	}

	return s.GetBySlug(slug, creatorID)
}

// GetBySlug retrieves a collection by slug with full detail
func (s *CollectionService) GetBySlug(slug string, viewerID uint) (*contracts.CollectionDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var collection models.Collection
	err := s.db.Where("slug = ?", slug).First(&collection).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrCollectionNotFound(slug)
		}
		return nil, fmt.Errorf("failed to get collection: %w", err)
	}

	// Check access: private collections are only visible to the creator
	if !collection.IsPublic && collection.CreatorID != viewerID {
		return nil, apperrors.ErrCollectionForbidden(slug)
	}

	// Load creator name
	creatorName := s.resolveUserName(collection.CreatorID)

	// Load items
	var items []models.CollectionItem
	s.db.Where("collection_id = ?", collection.ID).Order("position ASC, created_at ASC").Find(&items)

	// Resolve entity names for items
	itemResponses := s.buildItemResponses(items)

	// Count subscribers
	var subscriberCount int64
	s.db.Model(&models.CollectionSubscriber{}).Where("collection_id = ?", collection.ID).Count(&subscriberCount)

	// Count distinct contributors
	var contributorCount int64
	s.db.Model(&models.CollectionItem{}).Where("collection_id = ?", collection.ID).
		Distinct("added_by_user_id").Count(&contributorCount)

	// Check if viewer is subscribed
	isSubscribed := false
	if viewerID > 0 {
		var subCount int64
		s.db.Model(&models.CollectionSubscriber{}).
			Where("collection_id = ? AND user_id = ?", collection.ID, viewerID).
			Count(&subCount)
		isSubscribed = subCount > 0
	}

	return &contracts.CollectionDetailResponse{
		ID:               collection.ID,
		Title:            collection.Title,
		Slug:             collection.Slug,
		Description:      collection.Description,
		CreatorID:        collection.CreatorID,
		CreatorName:      creatorName,
		Collaborative:    collection.Collaborative,
		CoverImageURL:    collection.CoverImageURL,
		IsPublic:         collection.IsPublic,
		IsFeatured:       collection.IsFeatured,
		DisplayMode:      collection.DisplayMode,
		ItemCount:        len(itemResponses),
		SubscriberCount:  int(subscriberCount),
		ContributorCount: int(contributorCount),
		Items:            itemResponses,
		IsSubscribed:     isSubscribed,
		CreatedAt:        collection.CreatedAt,
		UpdatedAt:        collection.UpdatedAt,
	}, nil
}

// ListCollections retrieves collections with optional filtering
func (s *CollectionService) ListCollections(filters contracts.CollectionFilters, limit, offset int) ([]*contracts.CollectionListResponse, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	query := s.db.Model(&models.Collection{})

	if filters.PublicOnly {
		query = query.Where("is_public = ?", true)
	}
	if filters.CreatorID > 0 {
		query = query.Where("creator_id = ?", filters.CreatorID)
	}
	if filters.Featured {
		query = query.Where("is_featured = ?", true)
	}
	if filters.Search != "" {
		query = query.Where("title ILIKE ?", "%"+filters.Search+"%")
	}
	if filters.EntityType != "" {
		query = query.Where("id IN (?)",
			s.db.Model(&models.CollectionItem{}).
				Select("DISTINCT collection_id").
				Where("entity_type = ?", filters.EntityType),
		)
	}

	// Count total before pagination
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count collections: %w", err)
	}

	// Apply pagination and ordering
	if limit <= 0 {
		limit = 20
	}
	query = query.Order("updated_at DESC").Limit(limit).Offset(offset)

	var collections []models.Collection
	if err := query.Find(&collections).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list collections: %w", err)
	}

	if len(collections) == 0 {
		return []*contracts.CollectionListResponse{}, total, nil
	}

	// Batch-load counts and creator names
	collectionIDs := make([]uint, len(collections))
	creatorIDs := make([]uint, 0)
	creatorIDSet := make(map[uint]bool)
	for i, c := range collections {
		collectionIDs[i] = c.ID
		if !creatorIDSet[c.CreatorID] {
			creatorIDs = append(creatorIDs, c.CreatorID)
			creatorIDSet[c.CreatorID] = true
		}
	}

	// Batch-load item counts
	itemCounts := s.batchCountItems(collectionIDs)

	// Batch-load subscriber counts
	subscriberCounts := s.batchCountSubscribers(collectionIDs)

	// Batch-load contributor counts
	contributorCounts := s.batchCountContributors(collectionIDs)

	// Batch-load entity type counts
	entityTypeCounts := s.batchEntityTypeCounts(collectionIDs)

	// Batch-load creator names
	creatorNames := s.batchResolveUserNames(creatorIDs)

	// Build responses
	responses := make([]*contracts.CollectionListResponse, len(collections))
	for i, c := range collections {
		responses[i] = &contracts.CollectionListResponse{
			ID:               c.ID,
			Title:            c.Title,
			Slug:             c.Slug,
			Description:      c.Description,
			CreatorID:        c.CreatorID,
			CreatorName:      creatorNames[c.CreatorID],
			Collaborative:    c.Collaborative,
			CoverImageURL:    c.CoverImageURL,
			IsPublic:         c.IsPublic,
			IsFeatured:       c.IsFeatured,
			DisplayMode:      c.DisplayMode,
			ItemCount:        itemCounts[c.ID],
			SubscriberCount:  subscriberCounts[c.ID],
			ContributorCount: contributorCounts[c.ID],
			EntityTypeCounts: entityTypeCounts[c.ID],
			CreatedAt:        c.CreatedAt,
			UpdatedAt:        c.UpdatedAt,
		}
	}

	return responses, total, nil
}

// UpdateCollection updates an existing collection
func (s *CollectionService) UpdateCollection(slug string, userID uint, isAdmin bool, req *contracts.UpdateCollectionRequest) (*contracts.CollectionDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var collection models.Collection
	err := s.db.Where("slug = ?", slug).First(&collection).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrCollectionNotFound(slug)
		}
		return nil, fmt.Errorf("failed to get collection: %w", err)
	}

	// Check ownership
	if collection.CreatorID != userID && !isAdmin {
		return nil, apperrors.ErrCollectionForbidden(slug)
	}

	updates := map[string]interface{}{}

	if req.Title != nil {
		updates["title"] = *req.Title
		// Regenerate slug when title changes
		baseSlug := utils.GenerateArtistSlug(*req.Title)
		newSlug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
			var count int64
			s.db.Model(&models.Collection{}).Where("slug = ? AND id != ?", candidate, collection.ID).Count(&count)
			return count > 0
		})
		updates["slug"] = newSlug
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.Collaborative != nil {
		updates["collaborative"] = *req.Collaborative
	}
	if req.CoverImageURL != nil {
		updates["cover_image_url"] = *req.CoverImageURL
	}
	if req.IsPublic != nil {
		updates["is_public"] = *req.IsPublic
	}
	if req.DisplayMode != nil {
		if !models.IsValidCollectionDisplayMode(*req.DisplayMode) {
			return nil, apperrors.ErrCollectionInvalidRequest(
				fmt.Sprintf("display_mode must be 'ranked' or 'unranked', got %q", *req.DisplayMode),
			)
		}
		updates["display_mode"] = *req.DisplayMode
	}

	if len(updates) > 0 {
		err = s.db.Model(&models.Collection{}).Where("id = ?", collection.ID).Updates(updates).Error
		if err != nil {
			return nil, fmt.Errorf("failed to update collection: %w", err)
		}
	}

	// Determine the slug to use for retrieval (may have changed)
	retrieveSlug := slug
	if newSlug, ok := updates["slug"].(string); ok {
		retrieveSlug = newSlug
	}

	return s.GetBySlug(retrieveSlug, userID)
}

// DeleteCollection deletes a collection
func (s *CollectionService) DeleteCollection(slug string, userID uint, isAdmin bool) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	var collection models.Collection
	err := s.db.Where("slug = ?", slug).First(&collection).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.ErrCollectionNotFound(slug)
		}
		return fmt.Errorf("failed to get collection: %w", err)
	}

	// Check ownership
	if collection.CreatorID != userID && !isAdmin {
		return apperrors.ErrCollectionForbidden(slug)
	}

	// Delete collection (FK cascades handle items and subscribers)
	if err := s.db.Delete(&collection).Error; err != nil {
		return fmt.Errorf("failed to delete collection: %w", err)
	}

	return nil
}

// AddItem adds an entity to a collection
func (s *CollectionService) AddItem(slug string, userID uint, req *contracts.AddCollectionItemRequest) (*contracts.CollectionItemResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var collection models.Collection
	err := s.db.Where("slug = ?", slug).First(&collection).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrCollectionNotFound(slug)
		}
		return nil, fmt.Errorf("failed to get collection: %w", err)
	}

	// Check permission: creator or (collaborative and authenticated)
	if collection.CreatorID != userID && !collection.Collaborative {
		return nil, apperrors.ErrCollectionForbidden(slug)
	}

	// Check for duplicate
	var existingCount int64
	s.db.Model(&models.CollectionItem{}).
		Where("collection_id = ? AND entity_type = ? AND entity_id = ?", collection.ID, req.EntityType, req.EntityID).
		Count(&existingCount)
	if existingCount > 0 {
		return nil, apperrors.ErrCollectionItemExists(collection.ID, req.EntityType, req.EntityID)
	}

	// Get max position
	var maxPosition int
	row := s.db.Model(&models.CollectionItem{}).
		Where("collection_id = ?", collection.ID).
		Select("COALESCE(MAX(position), -1)").
		Row()
	if row != nil {
		_ = row.Scan(&maxPosition)
	}

	item := &models.CollectionItem{
		CollectionID:  collection.ID,
		EntityType:    req.EntityType,
		EntityID:      req.EntityID,
		Position:      maxPosition + 1,
		AddedByUserID: userID,
		Notes:         req.Notes,
	}

	if err := s.db.Create(item).Error; err != nil {
		return nil, fmt.Errorf("failed to add item to collection: %w", err)
	}

	// Resolve entity name and slug
	entityName, entitySlug := s.resolveEntityNameAndSlug(req.EntityType, req.EntityID)
	addedByName := s.resolveUserName(userID)

	return &contracts.CollectionItemResponse{
		ID:            item.ID,
		EntityType:    item.EntityType,
		EntityID:      item.EntityID,
		EntityName:    entityName,
		EntitySlug:    entitySlug,
		Position:      item.Position,
		AddedByUserID: item.AddedByUserID,
		AddedByName:   addedByName,
		Notes:         item.Notes,
		CreatedAt:     item.CreatedAt,
	}, nil
}

// UpdateItem updates an item's notes in a collection
func (s *CollectionService) UpdateItem(slug string, itemID uint, userID uint, isAdmin bool, req *contracts.UpdateCollectionItemRequest) (*contracts.CollectionItemResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var collection models.Collection
	err := s.db.Where("slug = ?", slug).First(&collection).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrCollectionNotFound(slug)
		}
		return nil, fmt.Errorf("failed to get collection: %w", err)
	}

	var item models.CollectionItem
	err = s.db.Where("id = ? AND collection_id = ?", itemID, collection.ID).First(&item).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrCollectionItemNotFound(itemID)
		}
		return nil, fmt.Errorf("failed to get collection item: %w", err)
	}

	// Check permission: collection creator, item adder, or admin
	if collection.CreatorID != userID && item.AddedByUserID != userID && !isAdmin {
		return nil, apperrors.ErrCollectionForbidden(slug)
	}

	// Update notes
	if req.Notes != nil {
		item.Notes = req.Notes
	}

	if err := s.db.Save(&item).Error; err != nil {
		return nil, fmt.Errorf("failed to update collection item: %w", err)
	}

	// Resolve entity name and slug
	entityName, entitySlug := s.resolveEntityNameAndSlug(item.EntityType, item.EntityID)
	addedByName := s.resolveUserName(item.AddedByUserID)

	return &contracts.CollectionItemResponse{
		ID:            item.ID,
		EntityType:    item.EntityType,
		EntityID:      item.EntityID,
		EntityName:    entityName,
		EntitySlug:    entitySlug,
		Position:      item.Position,
		AddedByUserID: item.AddedByUserID,
		AddedByName:   addedByName,
		Notes:         item.Notes,
		CreatedAt:     item.CreatedAt,
	}, nil
}

// RemoveItem removes an item from a collection
func (s *CollectionService) RemoveItem(slug string, itemID uint, userID uint, isAdmin bool) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	var collection models.Collection
	err := s.db.Where("slug = ?", slug).First(&collection).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.ErrCollectionNotFound(slug)
		}
		return fmt.Errorf("failed to get collection: %w", err)
	}

	var item models.CollectionItem
	err = s.db.Where("id = ? AND collection_id = ?", itemID, collection.ID).First(&item).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.ErrCollectionItemNotFound(itemID)
		}
		return fmt.Errorf("failed to get collection item: %w", err)
	}

	// Check permission: collection creator, item adder, or admin
	if collection.CreatorID != userID && item.AddedByUserID != userID && !isAdmin {
		return apperrors.ErrCollectionForbidden(slug)
	}

	if err := s.db.Delete(&item).Error; err != nil {
		return fmt.Errorf("failed to remove item from collection: %w", err)
	}

	return nil
}

// ReorderItems reorders items in a collection
func (s *CollectionService) ReorderItems(slug string, userID uint, req *contracts.ReorderCollectionItemsRequest) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	var collection models.Collection
	err := s.db.Where("slug = ?", slug).First(&collection).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.ErrCollectionNotFound(slug)
		}
		return fmt.Errorf("failed to get collection: %w", err)
	}

	// Only creator can reorder
	if collection.CreatorID != userID {
		return apperrors.ErrCollectionForbidden(slug)
	}

	// Update positions in a transaction
	return s.db.Transaction(func(tx *gorm.DB) error {
		for _, item := range req.Items {
			err := tx.Model(&models.CollectionItem{}).
				Where("id = ? AND collection_id = ?", item.ItemID, collection.ID).
				Update("position", item.Position).Error
			if err != nil {
				return fmt.Errorf("failed to update item position: %w", err)
			}
		}
		return nil
	})
}

// Subscribe subscribes a user to a collection
func (s *CollectionService) Subscribe(slug string, userID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	var collection models.Collection
	err := s.db.Where("slug = ?", slug).First(&collection).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.ErrCollectionNotFound(slug)
		}
		return fmt.Errorf("failed to get collection: %w", err)
	}

	// Check if collection is accessible
	if !collection.IsPublic && collection.CreatorID != userID {
		return apperrors.ErrCollectionForbidden(slug)
	}

	now := time.Now()
	subscriber := &models.CollectionSubscriber{
		CollectionID:  collection.ID,
		UserID:        userID,
		LastVisitedAt: &now,
	}

	// Use FirstOrCreate to handle idempotent subscribe
	result := s.db.Where("collection_id = ? AND user_id = ?", collection.ID, userID).
		FirstOrCreate(subscriber)
	if result.Error != nil {
		return fmt.Errorf("failed to subscribe to collection: %w", result.Error)
	}

	return nil
}

// Unsubscribe removes a user's subscription from a collection
func (s *CollectionService) Unsubscribe(slug string, userID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	var collection models.Collection
	err := s.db.Where("slug = ?", slug).First(&collection).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.ErrCollectionNotFound(slug)
		}
		return fmt.Errorf("failed to get collection: %w", err)
	}

	result := s.db.Where("collection_id = ? AND user_id = ?", collection.ID, userID).
		Delete(&models.CollectionSubscriber{})
	if result.Error != nil {
		return fmt.Errorf("failed to unsubscribe from collection: %w", result.Error)
	}

	return nil
}

// MarkVisited updates the last_visited_at timestamp for a subscriber
func (s *CollectionService) MarkVisited(slug string, userID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	var collection models.Collection
	err := s.db.Where("slug = ?", slug).First(&collection).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.ErrCollectionNotFound(slug)
		}
		return fmt.Errorf("failed to get collection: %w", err)
	}

	now := time.Now()
	result := s.db.Model(&models.CollectionSubscriber{}).
		Where("collection_id = ? AND user_id = ?", collection.ID, userID).
		Update("last_visited_at", now)
	if result.Error != nil {
		return fmt.Errorf("failed to mark collection visited: %w", result.Error)
	}

	return nil
}

// GetStats retrieves statistics for a collection
func (s *CollectionService) GetStats(slug string) (*contracts.CollectionStatsResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var collection models.Collection
	err := s.db.Where("slug = ?", slug).First(&collection).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrCollectionNotFound(slug)
		}
		return nil, fmt.Errorf("failed to get collection: %w", err)
	}

	// Item count
	var itemCount int64
	s.db.Model(&models.CollectionItem{}).Where("collection_id = ?", collection.ID).Count(&itemCount)

	// Subscriber count
	var subscriberCount int64
	s.db.Model(&models.CollectionSubscriber{}).Where("collection_id = ?", collection.ID).Count(&subscriberCount)

	// Contributor count (distinct users who added items)
	var contributorCount int64
	s.db.Model(&models.CollectionItem{}).Where("collection_id = ?", collection.ID).
		Distinct("added_by_user_id").Count(&contributorCount)

	// Entity type counts
	type TypeCount struct {
		EntityType string
		Count      int
	}
	var typeCounts []TypeCount
	s.db.Model(&models.CollectionItem{}).
		Select("entity_type, COUNT(*) as count").
		Where("collection_id = ?", collection.ID).
		Group("entity_type").
		Find(&typeCounts)

	entityTypeCounts := make(map[string]int)
	for _, tc := range typeCounts {
		entityTypeCounts[tc.EntityType] = tc.Count
	}

	return &contracts.CollectionStatsResponse{
		ItemCount:        int(itemCount),
		SubscriberCount:  int(subscriberCount),
		ContributorCount: int(contributorCount),
		EntityTypeCounts: entityTypeCounts,
	}, nil
}

// GetUserCollections retrieves collections created by or subscribed to by a user
func (s *CollectionService) GetUserCollections(userID uint, limit, offset int) ([]*contracts.CollectionListResponse, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	if limit <= 0 {
		limit = 20
	}

	// Get collection IDs the user created or is subscribed to
	subQuery := s.db.Model(&models.CollectionSubscriber{}).
		Select("collection_id").
		Where("user_id = ?", userID)

	query := s.db.Model(&models.Collection{}).
		Where("creator_id = ? OR id IN (?)", userID, subQuery)

	// Count total
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count user collections: %w", err)
	}

	var collections []models.Collection
	if err := query.Order("updated_at DESC").Limit(limit).Offset(offset).Find(&collections).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to get user collections: %w", err)
	}

	if len(collections) == 0 {
		return []*contracts.CollectionListResponse{}, total, nil
	}

	// Batch-load counts and creator names
	collectionIDs := make([]uint, len(collections))
	creatorIDs := make([]uint, 0)
	creatorIDSet := make(map[uint]bool)
	for i, c := range collections {
		collectionIDs[i] = c.ID
		if !creatorIDSet[c.CreatorID] {
			creatorIDs = append(creatorIDs, c.CreatorID)
			creatorIDSet[c.CreatorID] = true
		}
	}

	itemCounts := s.batchCountItems(collectionIDs)
	subscriberCounts := s.batchCountSubscribers(collectionIDs)
	contributorCounts := s.batchCountContributors(collectionIDs)
	entityTypeCounts := s.batchEntityTypeCounts(collectionIDs)
	creatorNames := s.batchResolveUserNames(creatorIDs)

	responses := make([]*contracts.CollectionListResponse, len(collections))
	for i, c := range collections {
		responses[i] = &contracts.CollectionListResponse{
			ID:               c.ID,
			Title:            c.Title,
			Slug:             c.Slug,
			Description:      c.Description,
			CreatorID:        c.CreatorID,
			CreatorName:      creatorNames[c.CreatorID],
			Collaborative:    c.Collaborative,
			CoverImageURL:    c.CoverImageURL,
			IsPublic:         c.IsPublic,
			IsFeatured:       c.IsFeatured,
			DisplayMode:      c.DisplayMode,
			ItemCount:        itemCounts[c.ID],
			SubscriberCount:  subscriberCounts[c.ID],
			ContributorCount: contributorCounts[c.ID],
			EntityTypeCounts: entityTypeCounts[c.ID],
			CreatedAt:        c.CreatedAt,
			UpdatedAt:        c.UpdatedAt,
		}
	}

	return responses, total, nil
}

// GetEntityCollections returns public collections that contain the given entity
func (s *CollectionService) GetEntityCollections(entityType string, entityID uint, limit int) ([]*contracts.CollectionListResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	if limit <= 0 {
		limit = 10
	}

	// Find collection IDs that contain this entity (public collections only)
	var collectionIDs []uint
	err := s.db.Model(&models.CollectionItem{}).
		Select("DISTINCT collection_items.collection_id").
		Joins("JOIN collections ON collections.id = collection_items.collection_id").
		Where("collection_items.entity_type = ? AND collection_items.entity_id = ? AND collections.is_public = ?", entityType, entityID, true).
		Limit(limit).
		Pluck("collection_items.collection_id", &collectionIDs).Error
	if err != nil {
		return nil, fmt.Errorf("failed to find entity collections: %w", err)
	}

	if len(collectionIDs) == 0 {
		return []*contracts.CollectionListResponse{}, nil
	}

	var collections []models.Collection
	if err := s.db.Where("id IN ?", collectionIDs).Order("updated_at DESC").Find(&collections).Error; err != nil {
		return nil, fmt.Errorf("failed to load entity collections: %w", err)
	}

	// Batch-load counts and creator names
	creatorIDs := make([]uint, 0)
	creatorIDSet := make(map[uint]bool)
	for _, c := range collections {
		if !creatorIDSet[c.CreatorID] {
			creatorIDs = append(creatorIDs, c.CreatorID)
			creatorIDSet[c.CreatorID] = true
		}
	}

	itemCounts := s.batchCountItems(collectionIDs)
	subscriberCounts := s.batchCountSubscribers(collectionIDs)
	contributorCounts := s.batchCountContributors(collectionIDs)
	entityTypeCounts := s.batchEntityTypeCounts(collectionIDs)
	creatorNames := s.batchResolveUserNames(creatorIDs)

	responses := make([]*contracts.CollectionListResponse, len(collections))
	for i, c := range collections {
		responses[i] = &contracts.CollectionListResponse{
			ID:               c.ID,
			Title:            c.Title,
			Slug:             c.Slug,
			Description:      c.Description,
			CreatorID:        c.CreatorID,
			CreatorName:      creatorNames[c.CreatorID],
			Collaborative:    c.Collaborative,
			CoverImageURL:    c.CoverImageURL,
			IsPublic:         c.IsPublic,
			IsFeatured:       c.IsFeatured,
			DisplayMode:      c.DisplayMode,
			ItemCount:        itemCounts[c.ID],
			SubscriberCount:  subscriberCounts[c.ID],
			ContributorCount: contributorCounts[c.ID],
			EntityTypeCounts: entityTypeCounts[c.ID],
			CreatedAt:        c.CreatedAt,
			UpdatedAt:        c.UpdatedAt,
		}
	}

	return responses, nil
}

// GetUserPublicCollections returns public collections created by a specific user
func (s *CollectionService) GetUserPublicCollections(userID uint, limit, offset int) ([]*contracts.CollectionListResponse, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	if limit <= 0 {
		limit = 20
	}

	query := s.db.Model(&models.Collection{}).
		Where("creator_id = ? AND is_public = ?", userID, true)

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count user public collections: %w", err)
	}

	var collections []models.Collection
	if err := query.Order("updated_at DESC").Limit(limit).Offset(offset).Find(&collections).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to get user public collections: %w", err)
	}

	if len(collections) == 0 {
		return []*contracts.CollectionListResponse{}, total, nil
	}

	collectionIDs := make([]uint, len(collections))
	creatorIDs := make([]uint, 0)
	creatorIDSet := make(map[uint]bool)
	for i, c := range collections {
		collectionIDs[i] = c.ID
		if !creatorIDSet[c.CreatorID] {
			creatorIDs = append(creatorIDs, c.CreatorID)
			creatorIDSet[c.CreatorID] = true
		}
	}

	itemCounts := s.batchCountItems(collectionIDs)
	subscriberCounts := s.batchCountSubscribers(collectionIDs)
	contributorCounts := s.batchCountContributors(collectionIDs)
	entityTypeCounts := s.batchEntityTypeCounts(collectionIDs)
	creatorNames := s.batchResolveUserNames(creatorIDs)

	responses := make([]*contracts.CollectionListResponse, len(collections))
	for i, c := range collections {
		responses[i] = &contracts.CollectionListResponse{
			ID:               c.ID,
			Title:            c.Title,
			Slug:             c.Slug,
			Description:      c.Description,
			CreatorID:        c.CreatorID,
			CreatorName:      creatorNames[c.CreatorID],
			Collaborative:    c.Collaborative,
			CoverImageURL:    c.CoverImageURL,
			IsPublic:         c.IsPublic,
			IsFeatured:       c.IsFeatured,
			DisplayMode:      c.DisplayMode,
			ItemCount:        itemCounts[c.ID],
			SubscriberCount:  subscriberCounts[c.ID],
			ContributorCount: contributorCounts[c.ID],
			EntityTypeCounts: entityTypeCounts[c.ID],
			CreatedAt:        c.CreatedAt,
			UpdatedAt:        c.UpdatedAt,
		}
	}

	return responses, total, nil
}

// GetUserPublicCollectionsByUsername returns public collections by username lookup
func (s *CollectionService) GetUserPublicCollectionsByUsername(username string, limit, offset int) ([]*contracts.CollectionListResponse, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	// Look up user by username
	var user models.User
	err := s.db.Where("username = ?", username).First(&user).Error
	if err != nil {
		// User not found - return empty
		return []*contracts.CollectionListResponse{}, 0, nil
	}

	return s.GetUserPublicCollections(user.ID, limit, offset)
}

// SetFeatured sets or unsets the featured flag on a collection
func (s *CollectionService) SetFeatured(slug string, featured bool) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	var collection models.Collection
	err := s.db.Where("slug = ?", slug).First(&collection).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.ErrCollectionNotFound(slug)
		}
		return fmt.Errorf("failed to get collection: %w", err)
	}

	if err := s.db.Model(&collection).Update("is_featured", featured).Error; err != nil {
		return fmt.Errorf("failed to update featured status: %w", err)
	}

	return nil
}

// ============================================================================
// Helper methods
// ============================================================================

// resolveUserName returns the display name for a user ID
func (s *CollectionService) resolveUserName(userID uint) string {
	var user models.User
	if err := s.db.Select("id, username, first_name, last_name, email").First(&user, userID).Error; err != nil {
		return "Anonymous"
	}
	if user.Username != nil && *user.Username != "" {
		return *user.Username
	}
	if user.FirstName != nil && *user.FirstName != "" {
		name := *user.FirstName
		if user.LastName != nil && *user.LastName != "" {
			name += " " + *user.LastName
		}
		return name
	}
	if user.Email != nil && *user.Email != "" {
		if idx := strings.Index(*user.Email, "@"); idx > 0 {
			return (*user.Email)[:idx]
		}
	}
	return "Anonymous"
}

// batchResolveUserNames resolves user names for multiple user IDs
func (s *CollectionService) batchResolveUserNames(userIDs []uint) map[uint]string {
	result := make(map[uint]string)
	if len(userIDs) == 0 {
		return result
	}

	var users []models.User
	s.db.Select("id, username, first_name, last_name, email").Where("id IN ?", userIDs).Find(&users)

	for _, user := range users {
		if user.Username != nil && *user.Username != "" {
			result[user.ID] = *user.Username
		} else if user.FirstName != nil && *user.FirstName != "" {
			name := *user.FirstName
			if user.LastName != nil && *user.LastName != "" {
				name += " " + *user.LastName
			}
			result[user.ID] = name
		} else if user.Email != nil && *user.Email != "" {
			if idx := strings.Index(*user.Email, "@"); idx > 0 {
				result[user.ID] = (*user.Email)[:idx]
			} else {
				result[user.ID] = "Anonymous"
			}
		} else {
			result[user.ID] = "Anonymous"
		}
	}

	return result
}

// resolveEntityNameAndSlug resolves the name and slug for a single entity
func (s *CollectionService) resolveEntityNameAndSlug(entityType string, entityID uint) (string, string) {
	switch entityType {
	case models.CollectionEntityArtist:
		var artist models.Artist
		if err := s.db.Select("id, name, slug").First(&artist, entityID).Error; err == nil {
			slug := ""
			if artist.Slug != nil {
				slug = *artist.Slug
			}
			return artist.Name, slug
		}
	case models.CollectionEntityVenue:
		var venue models.Venue
		if err := s.db.Select("id, name, slug").First(&venue, entityID).Error; err == nil {
			slug := ""
			if venue.Slug != nil {
				slug = *venue.Slug
			}
			return venue.Name, slug
		}
	case models.CollectionEntityShow:
		var show models.Show
		if err := s.db.Select("id, title, slug").First(&show, entityID).Error; err == nil {
			slug := ""
			if show.Slug != nil {
				slug = *show.Slug
			} else {
				slug = strconv.FormatUint(uint64(show.ID), 10)
			}
			return show.Title, slug
		}
	case models.CollectionEntityRelease:
		var release models.Release
		if err := s.db.Select("id, title, slug").First(&release, entityID).Error; err == nil {
			slug := ""
			if release.Slug != nil {
				slug = *release.Slug
			}
			return release.Title, slug
		}
	case models.CollectionEntityLabel:
		var label models.Label
		if err := s.db.Select("id, name, slug").First(&label, entityID).Error; err == nil {
			slug := ""
			if label.Slug != nil {
				slug = *label.Slug
			}
			return label.Name, slug
		}
	case models.CollectionEntityFestival:
		var festival models.Festival
		if err := s.db.Select("id, name, slug").First(&festival, entityID).Error; err == nil {
			return festival.Name, festival.Slug
		}
	}
	return "Unknown", ""
}

// buildItemResponses converts model items to response items with resolved entity names
func (s *CollectionService) buildItemResponses(items []models.CollectionItem) []contracts.CollectionItemResponse {
	if len(items) == 0 {
		return []contracts.CollectionItemResponse{}
	}

	// Group entity IDs by type for batch resolution
	entityIDsByType := make(map[string][]uint)
	userIDs := make([]uint, 0)
	userIDSet := make(map[uint]bool)

	for _, item := range items {
		entityIDsByType[item.EntityType] = append(entityIDsByType[item.EntityType], item.EntityID)
		if !userIDSet[item.AddedByUserID] {
			userIDs = append(userIDs, item.AddedByUserID)
			userIDSet[item.AddedByUserID] = true
		}
	}

	// Batch-resolve entity names and slugs
	entityNames, entitySlugs := s.batchResolveEntityNames(entityIDsByType)

	// Batch-resolve user names
	userNames := s.batchResolveUserNames(userIDs)

	// Build responses
	responses := make([]contracts.CollectionItemResponse, len(items))
	for i, item := range items {
		key := fmt.Sprintf("%s:%d", item.EntityType, item.EntityID)
		responses[i] = contracts.CollectionItemResponse{
			ID:            item.ID,
			EntityType:    item.EntityType,
			EntityID:      item.EntityID,
			EntityName:    entityNames[key],
			EntitySlug:    entitySlugs[key],
			Position:      item.Position,
			AddedByUserID: item.AddedByUserID,
			AddedByName:   userNames[item.AddedByUserID],
			Notes:         item.Notes,
			CreatedAt:     item.CreatedAt,
		}
	}

	return responses
}

// batchResolveEntityNames resolves names and slugs for groups of entities by type
func (s *CollectionService) batchResolveEntityNames(entityIDsByType map[string][]uint) (map[string]string, map[string]string) {
	names := make(map[string]string)
	slugs := make(map[string]string)

	for entityType, ids := range entityIDsByType {
		if len(ids) == 0 {
			continue
		}

		switch entityType {
		case models.CollectionEntityArtist:
			var artists []models.Artist
			s.db.Select("id, name, slug").Where("id IN ?", ids).Find(&artists)
			for _, a := range artists {
				key := fmt.Sprintf("%s:%d", entityType, a.ID)
				names[key] = a.Name
				if a.Slug != nil {
					slugs[key] = *a.Slug
				}
			}

		case models.CollectionEntityVenue:
			var venues []models.Venue
			s.db.Select("id, name, slug").Where("id IN ?", ids).Find(&venues)
			for _, v := range venues {
				key := fmt.Sprintf("%s:%d", entityType, v.ID)
				names[key] = v.Name
				if v.Slug != nil {
					slugs[key] = *v.Slug
				}
			}

		case models.CollectionEntityShow:
			var shows []models.Show
			s.db.Select("id, title, slug").Where("id IN ?", ids).Find(&shows)
			for _, sh := range shows {
				key := fmt.Sprintf("%s:%d", entityType, sh.ID)
				names[key] = sh.Title
				if sh.Slug != nil {
					slugs[key] = *sh.Slug
				} else {
					slugs[key] = strconv.FormatUint(uint64(sh.ID), 10)
				}
			}

		case models.CollectionEntityRelease:
			var releases []models.Release
			s.db.Select("id, title, slug").Where("id IN ?", ids).Find(&releases)
			for _, r := range releases {
				key := fmt.Sprintf("%s:%d", entityType, r.ID)
				names[key] = r.Title
				if r.Slug != nil {
					slugs[key] = *r.Slug
				}
			}

		case models.CollectionEntityLabel:
			var labels []models.Label
			s.db.Select("id, name, slug").Where("id IN ?", ids).Find(&labels)
			for _, l := range labels {
				key := fmt.Sprintf("%s:%d", entityType, l.ID)
				names[key] = l.Name
				if l.Slug != nil {
					slugs[key] = *l.Slug
				}
			}

		case models.CollectionEntityFestival:
			var festivals []models.Festival
			s.db.Select("id, name, slug").Where("id IN ?", ids).Find(&festivals)
			for _, f := range festivals {
				key := fmt.Sprintf("%s:%d", entityType, f.ID)
				names[key] = f.Name
				slugs[key] = f.Slug
			}
		}
	}

	return names, slugs
}

// batchCountItems returns item counts per collection ID
func (s *CollectionService) batchCountItems(collectionIDs []uint) map[uint]int {
	counts := make(map[uint]int)
	if len(collectionIDs) == 0 {
		return counts
	}

	type CountResult struct {
		CollectionID uint
		Count        int
	}
	var results []CountResult
	s.db.Model(&models.CollectionItem{}).
		Select("collection_id, COUNT(*) as count").
		Where("collection_id IN ?", collectionIDs).
		Group("collection_id").
		Find(&results)

	for _, r := range results {
		counts[r.CollectionID] = r.Count
	}
	return counts
}

// batchEntityTypeCounts returns a breakdown of item counts by entity type per collection ID.
// Used to surface entity type badges on collection cards.
func (s *CollectionService) batchEntityTypeCounts(collectionIDs []uint) map[uint]map[string]int {
	result := make(map[uint]map[string]int)
	if len(collectionIDs) == 0 {
		return result
	}

	type Row struct {
		CollectionID uint
		EntityType   string
		Count        int
	}
	var rows []Row
	s.db.Model(&models.CollectionItem{}).
		Select("collection_id, entity_type, COUNT(*) as count").
		Where("collection_id IN ?", collectionIDs).
		Group("collection_id, entity_type").
		Find(&rows)

	for _, r := range rows {
		if _, ok := result[r.CollectionID]; !ok {
			result[r.CollectionID] = make(map[string]int)
		}
		result[r.CollectionID][r.EntityType] = r.Count
	}
	return result
}

// batchCountSubscribers returns subscriber counts per collection ID
func (s *CollectionService) batchCountSubscribers(collectionIDs []uint) map[uint]int {
	counts := make(map[uint]int)
	if len(collectionIDs) == 0 {
		return counts
	}

	type CountResult struct {
		CollectionID uint
		Count        int
	}
	var results []CountResult
	s.db.Model(&models.CollectionSubscriber{}).
		Select("collection_id, COUNT(*) as count").
		Where("collection_id IN ?", collectionIDs).
		Group("collection_id").
		Find(&results)

	for _, r := range results {
		counts[r.CollectionID] = r.Count
	}
	return counts
}

// batchCountContributors returns distinct contributor counts per collection ID
func (s *CollectionService) batchCountContributors(collectionIDs []uint) map[uint]int {
	counts := make(map[uint]int)
	if len(collectionIDs) == 0 {
		return counts
	}

	type CountResult struct {
		CollectionID uint
		Count        int
	}
	var results []CountResult
	s.db.Model(&models.CollectionItem{}).
		Select("collection_id, COUNT(DISTINCT added_by_user_id) as count").
		Where("collection_id IN ?", collectionIDs).
		Group("collection_id").
		Find(&results)

	for _, r := range results {
		counts[r.CollectionID] = r.Count
	}
	return counts
}
