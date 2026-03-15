package engagement

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
)

// validFollowEntityTypes lists entity types that support following.
// Shows use going/interested (attendance) instead of follow.
var validFollowEntityTypes = map[string]bool{
	string(models.BookmarkEntityArtist):   true,
	string(models.BookmarkEntityVenue):    true,
	string(models.BookmarkEntityLabel):    true,
	string(models.BookmarkEntityFestival): true,
}

// FollowService handles follow/unfollow operations on entities.
// It wraps the generic user_bookmarks table with follow-specific logic.
type FollowService struct {
	db *gorm.DB
}

// NewFollowService creates a new follow service.
func NewFollowService(database *gorm.DB) *FollowService {
	if database == nil {
		database = db.GetDB()
	}
	return &FollowService{db: database}
}

// validateEntityType checks that entityType is a valid follow target.
func validateEntityType(entityType string) error {
	if !validFollowEntityTypes[entityType] {
		return fmt.Errorf("invalid entity type for follow: %s", entityType)
	}
	return nil
}

// Follow creates a follow bookmark. Idempotent — if already following, no error.
func (s *FollowService) Follow(userID uint, entityType string, entityID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}
	if err := validateEntityType(entityType); err != nil {
		return err
	}

	bookmark := models.UserBookmark{
		UserID:     userID,
		EntityType: models.BookmarkEntityType(entityType),
		EntityID:   entityID,
		Action:     models.BookmarkActionFollow,
		CreatedAt:  time.Now().UTC(),
	}

	return s.db.Where(models.UserBookmark{
		UserID:     userID,
		EntityType: models.BookmarkEntityType(entityType),
		EntityID:   entityID,
		Action:     models.BookmarkActionFollow,
	}).FirstOrCreate(&bookmark).Error
}

// Unfollow removes a follow bookmark. Idempotent — if not following, no error.
func (s *FollowService) Unfollow(userID uint, entityType string, entityID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}
	if err := validateEntityType(entityType); err != nil {
		return err
	}

	result := s.db.Where(
		"user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
		userID, models.BookmarkEntityType(entityType), entityID, models.BookmarkActionFollow,
	).Delete(&models.UserBookmark{})

	if result.Error != nil {
		return fmt.Errorf("failed to unfollow: %w", result.Error)
	}
	return nil
}

// IsFollowing checks if a user follows a specific entity.
func (s *FollowService) IsFollowing(userID uint, entityType string, entityID uint) (bool, error) {
	if s.db == nil {
		return false, fmt.Errorf("database not initialized")
	}
	if err := validateEntityType(entityType); err != nil {
		return false, err
	}

	var count int64
	err := s.db.Model(&models.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
			userID, models.BookmarkEntityType(entityType), entityID, models.BookmarkActionFollow).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("failed to check follow status: %w", err)
	}
	return count > 0, nil
}

// GetFollowerCount returns the number of followers for a specific entity.
func (s *FollowService) GetFollowerCount(entityType string, entityID uint) (int64, error) {
	if s.db == nil {
		return 0, fmt.Errorf("database not initialized")
	}
	if err := validateEntityType(entityType); err != nil {
		return 0, err
	}

	var count int64
	err := s.db.Model(&models.UserBookmark{}).
		Where("entity_type = ? AND entity_id = ? AND action = ?",
			models.BookmarkEntityType(entityType), entityID, models.BookmarkActionFollow).
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("failed to get follower count: %w", err)
	}
	return count, nil
}

// GetBatchFollowerCounts returns follower counts for multiple entities of the same type.
func (s *FollowService) GetBatchFollowerCounts(entityType string, entityIDs []uint) (map[uint]int64, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if err := validateEntityType(entityType); err != nil {
		return nil, err
	}

	result := make(map[uint]int64)
	if len(entityIDs) == 0 {
		return result, nil
	}

	type countRow struct {
		EntityID uint
		Count    int64
	}
	var rows []countRow

	err := s.db.Model(&models.UserBookmark{}).
		Select("entity_id, COUNT(*) as count").
		Where("entity_type = ? AND entity_id IN ? AND action = ?",
			models.BookmarkEntityType(entityType), entityIDs, models.BookmarkActionFollow).
		Group("entity_id").
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get batch follower counts: %w", err)
	}

	// Initialize all requested IDs with 0
	for _, id := range entityIDs {
		result[id] = 0
	}
	for _, row := range rows {
		result[row.EntityID] = row.Count
	}
	return result, nil
}

// GetBatchUserFollowing returns which entities the user follows from a list.
func (s *FollowService) GetBatchUserFollowing(userID uint, entityType string, entityIDs []uint) (map[uint]bool, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if err := validateEntityType(entityType); err != nil {
		return nil, err
	}

	result := make(map[uint]bool)
	if len(entityIDs) == 0 {
		return result, nil
	}

	var bookmarks []models.UserBookmark
	err := s.db.Where(
		"user_id = ? AND entity_type = ? AND entity_id IN ? AND action = ?",
		userID, models.BookmarkEntityType(entityType), entityIDs, models.BookmarkActionFollow,
	).Find(&bookmarks).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get batch user following: %w", err)
	}

	for _, b := range bookmarks {
		result[b.EntityID] = true
	}
	return result, nil
}

// GetUserFollowing lists all entities a user follows of a given type, ordered by follow date DESC.
// If entityType is empty, returns follows across all types.
func (s *FollowService) GetUserFollowing(userID uint, entityType string, limit, offset int) ([]*contracts.FollowingEntityResponse, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}
	if entityType != "" {
		if err := validateEntityType(entityType); err != nil {
			return nil, 0, err
		}
	}

	// Build base condition
	baseQuery := s.db.Model(&models.UserBookmark{}).
		Where("user_id = ? AND action = ?", userID, models.BookmarkActionFollow)
	if entityType != "" {
		baseQuery = baseQuery.Where("entity_type = ?", models.BookmarkEntityType(entityType))
	}

	// Count total
	var total int64
	if err := baseQuery.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count user following: %w", err)
	}
	if total == 0 {
		return []*contracts.FollowingEntityResponse{}, 0, nil
	}

	// Get bookmarks
	var bookmarks []models.UserBookmark
	if err := baseQuery.Order("created_at DESC").
		Limit(limit).Offset(offset).
		Find(&bookmarks).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to get user following: %w", err)
	}

	// Group bookmarks by entity type for batch name/slug lookups
	type entityKey struct {
		Type string
		ID   uint
	}
	entityNames := make(map[entityKey]struct{ Name, Slug string })

	// Collect IDs by type
	idsByType := make(map[string][]uint)
	for _, b := range bookmarks {
		t := string(b.EntityType)
		idsByType[t] = append(idsByType[t], b.EntityID)
	}

	// Batch lookup for each type
	for t, ids := range idsByType {
		switch t {
		case string(models.BookmarkEntityArtist):
			var artists []struct {
				ID   uint
				Name string
				Slug *string
			}
			s.db.Table("artists").Select("id, name, slug").Where("id IN ?", ids).Find(&artists)
			for _, a := range artists {
				slug := ""
				if a.Slug != nil {
					slug = *a.Slug
				}
				entityNames[entityKey{t, a.ID}] = struct{ Name, Slug string }{a.Name, slug}
			}
		case string(models.BookmarkEntityVenue):
			var venues []struct {
				ID   uint
				Name string
				Slug *string
			}
			s.db.Table("venues").Select("id, name, slug").Where("id IN ?", ids).Find(&venues)
			for _, v := range venues {
				slug := ""
				if v.Slug != nil {
					slug = *v.Slug
				}
				entityNames[entityKey{t, v.ID}] = struct{ Name, Slug string }{v.Name, slug}
			}
		case string(models.BookmarkEntityLabel):
			var labels []struct {
				ID   uint
				Name string
				Slug *string
			}
			s.db.Table("labels").Select("id, name, slug").Where("id IN ?", ids).Find(&labels)
			for _, l := range labels {
				slug := ""
				if l.Slug != nil {
					slug = *l.Slug
				}
				entityNames[entityKey{t, l.ID}] = struct{ Name, Slug string }{l.Name, slug}
			}
		case string(models.BookmarkEntityFestival):
			var festivals []struct {
				ID   uint
				Name string
				Slug string
			}
			s.db.Table("festivals").Select("id, name, slug").Where("id IN ?", ids).Find(&festivals)
			for _, f := range festivals {
				entityNames[entityKey{t, f.ID}] = struct{ Name, Slug string }{f.Name, f.Slug}
			}
		}
	}

	// Build response
	responses := make([]*contracts.FollowingEntityResponse, 0, len(bookmarks))
	for _, b := range bookmarks {
		info := entityNames[entityKey{string(b.EntityType), b.EntityID}]
		responses = append(responses, &contracts.FollowingEntityResponse{
			EntityType: string(b.EntityType),
			EntityID:   b.EntityID,
			Name:       info.Name,
			Slug:       info.Slug,
			FollowedAt: b.CreatedAt,
		})
	}

	return responses, total, nil
}

// GetFollowers lists followers of an entity. Returns user info.
func (s *FollowService) GetFollowers(entityType string, entityID uint, limit, offset int) ([]*contracts.FollowerResponse, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}
	if err := validateEntityType(entityType); err != nil {
		return nil, 0, err
	}

	// Count total followers
	var total int64
	err := s.db.Model(&models.UserBookmark{}).
		Where("entity_type = ? AND entity_id = ? AND action = ?",
			models.BookmarkEntityType(entityType), entityID, models.BookmarkActionFollow).
		Count(&total).Error
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count followers: %w", err)
	}
	if total == 0 {
		return []*contracts.FollowerResponse{}, 0, nil
	}

	// Query bookmarks joined with users
	type followerRow struct {
		UserID      uint
		Username    *string
		DisplayName *string
	}
	var rows []followerRow

	err = s.db.Table("user_bookmarks").
		Select("user_bookmarks.user_id, users.username, users.first_name as display_name").
		Joins("JOIN users ON users.id = user_bookmarks.user_id").
		Where("user_bookmarks.entity_type = ? AND user_bookmarks.entity_id = ? AND user_bookmarks.action = ?",
			models.BookmarkEntityType(entityType), entityID, models.BookmarkActionFollow).
		Where("users.deleted_at IS NULL").
		Order("user_bookmarks.created_at DESC").
		Limit(limit).Offset(offset).
		Find(&rows).Error
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get followers: %w", err)
	}

	responses := make([]*contracts.FollowerResponse, 0, len(rows))
	for _, row := range rows {
		resp := &contracts.FollowerResponse{
			UserID: row.UserID,
		}
		if row.Username != nil {
			resp.Username = *row.Username
		}
		if row.DisplayName != nil {
			resp.DisplayName = *row.DisplayName
		}
		responses = append(responses, resp)
	}

	return responses, total, nil
}
