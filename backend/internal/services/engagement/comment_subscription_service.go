package engagement

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
)

// CommentSubscriptionService implements CommentSubscriptionServiceInterface.
type CommentSubscriptionService struct {
	db *gorm.DB
}

// NewCommentSubscriptionService creates a new CommentSubscriptionService.
func NewCommentSubscriptionService(db *gorm.DB) *CommentSubscriptionService {
	return &CommentSubscriptionService{db: db}
}

// Subscribe adds a subscription for a user to an entity's comments.
// Idempotent — if already subscribed, no error (ON CONFLICT DO NOTHING).
func (s *CommentSubscriptionService) Subscribe(userID uint, entityType string, entityID uint) error {
	if s.db == nil {
		return errors.New("database not initialized")
	}

	if _, err := validateCommentEntityType(entityType); err != nil {
		return err
	}

	sub := models.CommentSubscription{
		UserID:       userID,
		EntityType:   entityType,
		EntityID:     entityID,
		SubscribedAt: time.Now().UTC(),
	}

	return s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&sub).Error
}

// Unsubscribe removes a subscription. Idempotent — if not subscribed, no error.
func (s *CommentSubscriptionService) Unsubscribe(userID uint, entityType string, entityID uint) error {
	if s.db == nil {
		return errors.New("database not initialized")
	}

	if _, err := validateCommentEntityType(entityType); err != nil {
		return err
	}

	return s.db.Where(
		"user_id = ? AND entity_type = ? AND entity_id = ?",
		userID, entityType, entityID,
	).Delete(&models.CommentSubscription{}).Error
}

// IsSubscribed checks whether a user is subscribed to an entity's comments.
func (s *CommentSubscriptionService) IsSubscribed(userID uint, entityType string, entityID uint) (bool, error) {
	if s.db == nil {
		return false, errors.New("database not initialized")
	}

	if _, err := validateCommentEntityType(entityType); err != nil {
		return false, err
	}

	var count int64
	err := s.db.Model(&models.CommentSubscription{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ?", userID, entityType, entityID).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("failed to check subscription: %w", err)
	}
	return count > 0, nil
}

// MarkRead updates the last-read pointer for a user on an entity to the latest comment ID.
func (s *CommentSubscriptionService) MarkRead(userID uint, entityType string, entityID uint) error {
	if s.db == nil {
		return errors.New("database not initialized")
	}

	if _, err := validateCommentEntityType(entityType); err != nil {
		return err
	}

	// Find the max comment ID for this entity
	var maxID uint
	err := s.db.Model(&models.Comment{}).
		Where("entity_type = ? AND entity_id = ?", entityType, entityID).
		Select("COALESCE(MAX(id), 0)").
		Scan(&maxID).Error
	if err != nil {
		return fmt.Errorf("failed to get max comment ID: %w", err)
	}

	// Upsert the last-read record
	lastRead := models.CommentLastRead{
		UserID:            userID,
		EntityType:        entityType,
		EntityID:          entityID,
		LastReadCommentID: maxID,
		UpdatedAt:         time.Now().UTC(),
	}

	return s.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "user_id"},
			{Name: "entity_type"},
			{Name: "entity_id"},
		},
		DoUpdates: clause.AssignmentColumns([]string{"last_read_comment_id", "updated_at"}),
	}).Create(&lastRead).Error
}

// GetUnreadCount returns the number of visible comments newer than the user's last-read pointer.
func (s *CommentSubscriptionService) GetUnreadCount(userID uint, entityType string, entityID uint) (int, error) {
	if s.db == nil {
		return 0, errors.New("database not initialized")
	}

	if _, err := validateCommentEntityType(entityType); err != nil {
		return 0, err
	}

	// Get the last-read comment ID (0 if never read)
	var lastReadID uint
	err := s.db.Model(&models.CommentLastRead{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ?", userID, entityType, entityID).
		Select("last_read_comment_id").
		Scan(&lastReadID).Error
	if err != nil {
		return 0, fmt.Errorf("failed to get last read: %w", err)
	}

	// Count visible comments with ID > lastReadID
	var count int64
	err = s.db.Model(&models.Comment{}).
		Where("entity_type = ? AND entity_id = ? AND visibility = ? AND id > ?",
			entityType, entityID, models.CommentVisibilityVisible, lastReadID).
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("failed to count unread comments: %w", err)
	}

	return int(count), nil
}

// GetSubscriptionsForUser returns paginated subscriptions with unread counts.
func (s *CommentSubscriptionService) GetSubscriptionsForUser(userID uint, limit, offset int) ([]contracts.SubscriptionResponse, int64, error) {
	if s.db == nil {
		return nil, 0, errors.New("database not initialized")
	}

	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	// Count total subscriptions
	var total int64
	err := s.db.Model(&models.CommentSubscription{}).
		Where("user_id = ?", userID).
		Count(&total).Error
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count subscriptions: %w", err)
	}

	if total == 0 {
		return []contracts.SubscriptionResponse{}, 0, nil
	}

	// Fetch subscriptions ordered by most recent first
	var subs []models.CommentSubscription
	err = s.db.Where("user_id = ?", userID).
		Order("subscribed_at DESC").
		Limit(limit).Offset(offset).
		Find(&subs).Error
	if err != nil {
		return nil, 0, fmt.Errorf("failed to fetch subscriptions: %w", err)
	}

	// Build responses with unread counts
	responses := make([]contracts.SubscriptionResponse, len(subs))
	for i, sub := range subs {
		unread, _ := s.GetUnreadCount(userID, sub.EntityType, sub.EntityID)
		responses[i] = contracts.SubscriptionResponse{
			EntityType:   sub.EntityType,
			EntityID:     sub.EntityID,
			SubscribedAt: sub.SubscribedAt,
			UnreadCount:  unread,
		}
	}

	return responses, total, nil
}

// GetSubscribersForEntity returns user IDs of all subscribers for an entity.
func (s *CommentSubscriptionService) GetSubscribersForEntity(entityType string, entityID uint) ([]uint, error) {
	if s.db == nil {
		return nil, errors.New("database not initialized")
	}

	if _, err := validateCommentEntityType(entityType); err != nil {
		return nil, err
	}

	var userIDs []uint
	err := s.db.Model(&models.CommentSubscription{}).
		Where("entity_type = ? AND entity_id = ?", entityType, entityID).
		Pluck("user_id", &userIDs).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get subscribers: %w", err)
	}

	return userIDs, nil
}
