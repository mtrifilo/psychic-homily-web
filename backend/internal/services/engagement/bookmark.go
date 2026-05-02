package engagement

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	engagementm "psychic-homily-backend/internal/models/engagement"
)

// BookmarkService handles generic bookmark operations for all entity types
type BookmarkService struct {
	db *gorm.DB
}

// NewBookmarkService creates a new bookmark service
func NewBookmarkService(database *gorm.DB) *BookmarkService {
	if database == nil {
		database = db.GetDB()
	}
	return &BookmarkService{
		db: database,
	}
}

// CreateBookmark creates a bookmark for a user on an entity.
// Idempotent: if the bookmark already exists, it updates the created_at timestamp.
func (s *BookmarkService) CreateBookmark(userID uint, entityType engagementm.BookmarkEntityType, entityID uint, action engagementm.BookmarkAction) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	bookmark := engagementm.UserBookmark{
		UserID:     userID,
		EntityType: entityType,
		EntityID:   entityID,
		Action:     action,
		CreatedAt:  time.Now().UTC(),
	}

	err := s.db.Where(engagementm.UserBookmark{
		UserID:     userID,
		EntityType: entityType,
		EntityID:   entityID,
		Action:     action,
	}).Assign(engagementm.UserBookmark{
		CreatedAt: bookmark.CreatedAt,
	}).FirstOrCreate(&bookmark).Error

	if err != nil {
		return fmt.Errorf("failed to create bookmark: %w", err)
	}

	return nil
}

// DeleteBookmark removes a bookmark for a user on an entity.
// Returns an error if the bookmark does not exist.
func (s *BookmarkService) DeleteBookmark(userID uint, entityType engagementm.BookmarkEntityType, entityID uint, action engagementm.BookmarkAction) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	result := s.db.Where(
		"user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
		userID, entityType, entityID, action,
	).Delete(&engagementm.UserBookmark{})

	if result.Error != nil {
		return fmt.Errorf("failed to delete bookmark: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("bookmark not found")
	}

	return nil
}

// IsBookmarked checks if a user has a specific bookmark on an entity.
func (s *BookmarkService) IsBookmarked(userID uint, entityType engagementm.BookmarkEntityType, entityID uint, action engagementm.BookmarkAction) (bool, error) {
	if s.db == nil {
		return false, fmt.Errorf("database not initialized")
	}

	var count int64
	err := s.db.Model(&engagementm.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
			userID, entityType, entityID, action).
		Count(&count).Error

	if err != nil {
		return false, fmt.Errorf("failed to check bookmark: %w", err)
	}

	return count > 0, nil
}

// GetBookmarkedEntityIDs returns the set of entity IDs that a user has bookmarked
// with the given entity type and action, filtered to the provided entity IDs.
func (s *BookmarkService) GetBookmarkedEntityIDs(userID uint, entityType engagementm.BookmarkEntityType, action engagementm.BookmarkAction, entityIDs []uint) (map[uint]bool, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	result := make(map[uint]bool)

	if len(entityIDs) == 0 {
		return result, nil
	}

	var bookmarks []engagementm.UserBookmark
	err := s.db.Where(
		"user_id = ? AND entity_type = ? AND action = ? AND entity_id IN ?",
		userID, entityType, action, entityIDs,
	).Find(&bookmarks).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get bookmarked entity IDs: %w", err)
	}

	for _, b := range bookmarks {
		result[b.EntityID] = true
	}

	return result, nil
}

// GetUserBookmarks retrieves bookmarks for a user filtered by entity type and action,
// ordered by created_at DESC with pagination.
func (s *BookmarkService) GetUserBookmarks(userID uint, entityType engagementm.BookmarkEntityType, action engagementm.BookmarkAction, limit, offset int) ([]engagementm.UserBookmark, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	var total int64
	if err := s.db.Model(&engagementm.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND action = ?", userID, entityType, action).
		Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count bookmarks: %w", err)
	}

	var bookmarks []engagementm.UserBookmark
	err := s.db.Where("user_id = ? AND entity_type = ? AND action = ?", userID, entityType, action).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&bookmarks).Error

	if err != nil {
		return nil, 0, fmt.Errorf("failed to get bookmarks: %w", err)
	}

	return bookmarks, total, nil
}

// GetUserBookmarksByEntityType retrieves all bookmarks for a user with a given entity type
// (regardless of action), useful for queries like "all venue IDs the user follows."
func (s *BookmarkService) GetUserBookmarksByEntityType(userID uint, entityType engagementm.BookmarkEntityType, action engagementm.BookmarkAction) ([]engagementm.UserBookmark, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var bookmarks []engagementm.UserBookmark
	err := s.db.Where("user_id = ? AND entity_type = ? AND action = ?", userID, entityType, action).
		Find(&bookmarks).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get bookmarks: %w", err)
	}

	return bookmarks, nil
}

// CountUserBookmarks returns the count of bookmarks for a user with a given entity type and action.
func (s *BookmarkService) CountUserBookmarks(userID uint, entityType engagementm.BookmarkEntityType, action engagementm.BookmarkAction) (int64, error) {
	if s.db == nil {
		return 0, fmt.Errorf("database not initialized")
	}

	var count int64
	err := s.db.Model(&engagementm.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND action = ?", userID, entityType, action).
		Count(&count).Error

	if err != nil {
		return 0, fmt.Errorf("failed to count bookmarks: %w", err)
	}

	return count, nil
}
