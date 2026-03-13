package services

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/models"
)

// RequestService handles community request business logic.
type RequestService struct {
	db *gorm.DB
}

// NewRequestService creates a new request service.
func NewRequestService(database *gorm.DB) *RequestService {
	if database == nil {
		database = db.GetDB()
	}
	return &RequestService{db: database}
}

// validEntityTypes lists the allowed entity types for requests.
var validRequestEntityTypes = map[string]bool{
	models.RequestEntityArtist:   true,
	models.RequestEntityRelease:  true,
	models.RequestEntityLabel:    true,
	models.RequestEntityShow:     true,
	models.RequestEntityVenue:    true,
	models.RequestEntityFestival: true,
}

// CreateRequest creates a new community request.
func (s *RequestService) CreateRequest(userID uint, title, description, entityType string, requestedEntityID *uint) (*models.Request, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	if !validRequestEntityTypes[entityType] {
		return nil, fmt.Errorf("invalid entity type: %s", entityType)
	}

	var desc *string
	if description != "" {
		desc = &description
	}

	request := &models.Request{
		Title:             title,
		Description:       desc,
		EntityType:        entityType,
		RequestedEntityID: requestedEntityID,
		Status:            models.RequestStatusPending,
		RequesterID:       userID,
	}

	if err := s.db.Create(request).Error; err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	return request, nil
}

// GetRequest retrieves a request by ID with the requester preloaded.
func (s *RequestService) GetRequest(requestID uint) (*models.Request, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var request models.Request
	err := s.db.Preload("Requester").Preload("Fulfiller").First(&request, requestID).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get request: %w", err)
	}

	return &request, nil
}

// ListRequests retrieves requests with optional filtering and sorting.
func (s *RequestService) ListRequests(status string, entityType string, sortBy string, limit, offset int) ([]models.Request, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	query := s.db.Model(&models.Request{})

	if status != "" {
		query = query.Where("status = ?", status)
	}
	if entityType != "" {
		query = query.Where("entity_type = ?", entityType)
	}

	// Count total before pagination
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count requests: %w", err)
	}

	// Apply sorting
	switch sortBy {
	case "newest":
		query = query.Order("created_at DESC")
	case "oldest":
		query = query.Order("created_at ASC")
	case "votes":
		// Use Wilson score for ranking, computed from upvotes/downvotes
		// We sort by vote_score DESC as a simple proxy; Wilson is computed on read
		query = query.Order("vote_score DESC, created_at DESC")
	default:
		// Default to votes
		query = query.Order("vote_score DESC, created_at DESC")
	}

	// Apply pagination
	if limit <= 0 {
		limit = 20
	}
	query = query.Preload("Requester").Limit(limit).Offset(offset)

	var requests []models.Request
	if err := query.Find(&requests).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list requests: %w", err)
	}

	return requests, total, nil
}

// UpdateRequest updates a request. Only the requester can update.
func (s *RequestService) UpdateRequest(requestID, userID uint, title, description *string) (*models.Request, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var request models.Request
	err := s.db.First(&request, requestID).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrRequestNotFound(requestID)
		}
		return nil, fmt.Errorf("failed to get request: %w", err)
	}

	// Only the requester can update
	if request.RequesterID != userID {
		return nil, apperrors.ErrRequestForbidden(requestID)
	}

	updates := map[string]interface{}{}
	if title != nil {
		updates["title"] = *title
	}
	if description != nil {
		updates["description"] = *description
	}

	if len(updates) > 0 {
		if err := s.db.Model(&request).Updates(updates).Error; err != nil {
			return nil, fmt.Errorf("failed to update request: %w", err)
		}
	}

	// Re-fetch with preloads
	return s.GetRequest(requestID)
}

// DeleteRequest deletes a request. Only the requester or admin can delete.
func (s *RequestService) DeleteRequest(requestID, userID uint, isAdmin bool) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	var request models.Request
	err := s.db.First(&request, requestID).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.ErrRequestNotFound(requestID)
		}
		return fmt.Errorf("failed to get request: %w", err)
	}

	// Only the requester or admin can delete
	if request.RequesterID != userID && !isAdmin {
		return apperrors.ErrRequestForbidden(requestID)
	}

	// Delete votes first (FK cascade should handle this, but be explicit)
	if err := s.db.Where("request_id = ?", requestID).Delete(&models.RequestVote{}).Error; err != nil {
		return fmt.Errorf("failed to delete request votes: %w", err)
	}

	if err := s.db.Delete(&request).Error; err != nil {
		return fmt.Errorf("failed to delete request: %w", err)
	}

	return nil
}

// Vote adds or updates a vote on a request.
func (s *RequestService) Vote(requestID, userID uint, isUpvote bool) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	// Verify request exists
	var request models.Request
	if err := s.db.First(&request, requestID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.ErrRequestNotFound(requestID)
		}
		return fmt.Errorf("failed to get request: %w", err)
	}

	voteValue := -1
	if isUpvote {
		voteValue = 1
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		// Upsert the vote
		var existingVote models.RequestVote
		err := tx.Where("request_id = ? AND user_id = ?", requestID, userID).First(&existingVote).Error

		if err == gorm.ErrRecordNotFound {
			// New vote
			vote := models.RequestVote{
				RequestID: requestID,
				UserID:    userID,
				Vote:      voteValue,
			}
			if err := tx.Create(&vote).Error; err != nil {
				return fmt.Errorf("failed to create vote: %w", err)
			}
		} else if err != nil {
			return fmt.Errorf("failed to check existing vote: %w", err)
		} else {
			// Update existing vote
			if err := tx.Model(&existingVote).Update("vote", voteValue).Error; err != nil {
				return fmt.Errorf("failed to update vote: %w", err)
			}
		}

		// Recalculate vote counts
		return s.recalculateVoteCounts(tx, requestID)
	})
}

// RemoveVote removes a user's vote on a request.
func (s *RequestService) RemoveVote(requestID, userID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	// Verify request exists
	var request models.Request
	if err := s.db.First(&request, requestID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.ErrRequestNotFound(requestID)
		}
		return fmt.Errorf("failed to get request: %w", err)
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Where("request_id = ? AND user_id = ?", requestID, userID).Delete(&models.RequestVote{})
		if result.Error != nil {
			return fmt.Errorf("failed to remove vote: %w", result.Error)
		}

		// Recalculate vote counts
		return s.recalculateVoteCounts(tx, requestID)
	})
}

// FulfillRequest marks a request as fulfilled.
func (s *RequestService) FulfillRequest(requestID, fulfillerID uint, fulfilledEntityID *uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	var request models.Request
	err := s.db.First(&request, requestID).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.ErrRequestNotFound(requestID)
		}
		return fmt.Errorf("failed to get request: %w", err)
	}

	// Only pending or in_progress requests can be fulfilled
	if request.Status != models.RequestStatusPending && request.Status != models.RequestStatusInProgress {
		return apperrors.ErrRequestAlreadyFulfilled(requestID)
	}

	now := time.Now()
	updates := map[string]interface{}{
		"status":       models.RequestStatusFulfilled,
		"fulfiller_id": fulfillerID,
		"fulfilled_at": now,
	}
	if fulfilledEntityID != nil {
		updates["requested_entity_id"] = *fulfilledEntityID
	}

	if err := s.db.Model(&request).Updates(updates).Error; err != nil {
		return fmt.Errorf("failed to fulfill request: %w", err)
	}

	return nil
}

// CloseRequest closes a request (cancelled by owner, rejected by admin).
func (s *RequestService) CloseRequest(requestID, userID uint, isAdmin bool) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	var request models.Request
	err := s.db.First(&request, requestID).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.ErrRequestNotFound(requestID)
		}
		return fmt.Errorf("failed to get request: %w", err)
	}

	// Must be the requester or admin
	if request.RequesterID != userID && !isAdmin {
		return apperrors.ErrRequestForbidden(requestID)
	}

	// Determine the new status
	newStatus := models.RequestStatusCancelled
	if isAdmin && request.RequesterID != userID {
		newStatus = models.RequestStatusRejected
	}

	if err := s.db.Model(&request).Update("status", newStatus).Error; err != nil {
		return fmt.Errorf("failed to close request: %w", err)
	}

	return nil
}

// GetUserVote returns the user's vote on a request, or nil if not voted.
func (s *RequestService) GetUserVote(requestID, userID uint) (*models.RequestVote, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var vote models.RequestVote
	err := s.db.Where("request_id = ? AND user_id = ?", requestID, userID).First(&vote).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get user vote: %w", err)
	}

	return &vote, nil
}

// recalculateVoteCounts recalculates upvotes, downvotes, and vote_score for a request.
func (s *RequestService) recalculateVoteCounts(tx *gorm.DB, requestID uint) error {
	var upvotes, downvotes int64

	tx.Model(&models.RequestVote{}).
		Where("request_id = ? AND vote = 1", requestID).
		Count(&upvotes)

	tx.Model(&models.RequestVote{}).
		Where("request_id = ? AND vote = -1", requestID).
		Count(&downvotes)

	voteScore := int(upvotes - downvotes)

	return tx.Model(&models.Request{}).
		Where("id = ?", requestID).
		Updates(map[string]interface{}{
			"upvotes":    int(upvotes),
			"downvotes":  int(downvotes),
			"vote_score": voteScore,
		}).Error
}
