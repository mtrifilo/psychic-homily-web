package community

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	apperrors "psychic-homily-backend/internal/errors"
	communitym "psychic-homily-backend/internal/models/community"
	notificationm "psychic-homily-backend/internal/models/notification"
)

// requestEntityTables maps a request's declared entity_type to the
// physical table the FulfilledEntityID is expected to point at. Used by
// FulfillRequest for the entity-existence + type-match check (PSY-748).
// Keep aligned with validRequestEntityTypes — any new entity type added
// to the request enum must also map to a table here or fulfillment will
// silently fail-closed with a misleading error.
var requestEntityTables = map[string]string{
	communitym.RequestEntityArtist:   "artists",
	communitym.RequestEntityVenue:    "venues",
	communitym.RequestEntityShow:     "shows",
	communitym.RequestEntityRelease:  "releases",
	communitym.RequestEntityLabel:    "labels",
	communitym.RequestEntityFestival: "festivals",
}

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
	communitym.RequestEntityArtist:   true,
	communitym.RequestEntityRelease:  true,
	communitym.RequestEntityLabel:    true,
	communitym.RequestEntityShow:     true,
	communitym.RequestEntityVenue:    true,
	communitym.RequestEntityFestival: true,
}

// CreateRequest creates a new community request.
func (s *RequestService) CreateRequest(userID uint, title, description, entityType string, requestedEntityID *uint) (*communitym.Request, error) {
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

	request := &communitym.Request{
		Title:             title,
		Description:       desc,
		EntityType:        entityType,
		RequestedEntityID: requestedEntityID,
		Status:            communitym.RequestStatusPending,
		RequesterID:       userID,
	}

	if err := s.db.Create(request).Error; err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	return request, nil
}

// GetRequest retrieves a request by ID with the requester preloaded.
func (s *RequestService) GetRequest(requestID uint) (*communitym.Request, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var request communitym.Request
	err := s.db.Preload("Requester").Preload("Fulfiller").First(&request, requestID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get request: %w", err)
	}

	return &request, nil
}

// ListRequests retrieves requests with optional filtering and sorting.
func (s *RequestService) ListRequests(status string, entityType string, sortBy string, limit, offset int) ([]communitym.Request, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	query := s.db.Model(&communitym.Request{})

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

	var requests []communitym.Request
	if err := query.Find(&requests).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list requests: %w", err)
	}

	return requests, total, nil
}

// UpdateRequest updates a request. Only the requester can update.
func (s *RequestService) UpdateRequest(requestID, userID uint, title, description *string) (*communitym.Request, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var request communitym.Request
	err := s.db.First(&request, requestID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
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

	var request communitym.Request
	err := s.db.First(&request, requestID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperrors.ErrRequestNotFound(requestID)
		}
		return fmt.Errorf("failed to get request: %w", err)
	}

	// Only the requester or admin can delete
	if request.RequesterID != userID && !isAdmin {
		return apperrors.ErrRequestForbidden(requestID)
	}

	// Delete votes first (FK cascade should handle this, but be explicit)
	if err := s.db.Where("request_id = ?", requestID).Delete(&communitym.RequestVote{}).Error; err != nil {
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
	var request communitym.Request
	if err := s.db.First(&request, requestID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
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
		var existingVote communitym.RequestVote
		err := tx.Where("request_id = ? AND user_id = ?", requestID, userID).First(&existingVote).Error

		if errors.Is(err, gorm.ErrRecordNotFound) {
			// New vote
			vote := communitym.RequestVote{
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
	var request communitym.Request
	if err := s.db.First(&request, requestID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperrors.ErrRequestNotFound(requestID)
		}
		return fmt.Errorf("failed to get request: %w", err)
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Where("request_id = ? AND user_id = ?", requestID, userID).Delete(&communitym.RequestVote{})
		if result.Error != nil {
			return fmt.Errorf("failed to remove vote: %w", result.Error)
		}

		// Recalculate vote counts
		return s.recalculateVoteCounts(tx, requestID)
	})
}

// FulfillRequest submits a proposed fulfillment for community review.
//
// Authorization: any authenticated user may submit; this is the
// community-contribution side of the workflow. The original requester
// (or an admin) still has to call ApproveFulfillment to flip the
// request to "fulfilled" — see PSY-748 for the threat model that
// motivated the two-step gate (Finding 7: pre-PSY-748 any user could
// directly mark any request as fulfilled and hijack the entity link).
//
// Validation: when fulfilledEntityID is provided, the referenced entity
// MUST exist AND its table MUST match the request's declared
// EntityType. Mismatch → ErrRequestEntityTypeMismatch (400). Missing
// row → ErrRequestEntityNotFound (400). This stops a caller from
// pointing an "artist" request at a venue ID.
//
// State transition: pending | in_progress → pending_fulfillment.
// Already-fulfilled or already-pending-fulfillment requests return
// ErrRequestAlreadyFulfilled (409).
func (s *RequestService) FulfillRequest(requestID, fulfillerID uint, fulfilledEntityID *uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	var request communitym.Request
	err := s.db.First(&request, requestID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperrors.ErrRequestNotFound(requestID)
		}
		return fmt.Errorf("failed to get request: %w", err)
	}

	// Only pending or in_progress requests can enter pending_fulfillment.
	// Anything in pending_fulfillment / fulfilled / cancelled / rejected is
	// out of scope for a fresh submission and surfaces as 409.
	if request.Status != communitym.RequestStatusPending && request.Status != communitym.RequestStatusInProgress {
		return apperrors.ErrRequestAlreadyFulfilled(requestID)
	}

	// Validate the proposed entity exists AND matches the request's type
	// before any state mutation, so a bad payload can't poison the row.
	if fulfilledEntityID != nil {
		if err := s.validateFulfillmentEntity(requestID, request.EntityType, *fulfilledEntityID); err != nil {
			return err
		}
	}

	updates := map[string]interface{}{
		"status":       communitym.RequestStatusPendingFulfillment,
		"fulfiller_id": fulfillerID,
	}
	if fulfilledEntityID != nil {
		updates["requested_entity_id"] = *fulfilledEntityID
	}

	if err := s.db.Model(&request).Updates(updates).Error; err != nil {
		return fmt.Errorf("failed to submit fulfillment: %w", err)
	}

	return nil
}

// validateFulfillmentEntity confirms that entityID exists in the table
// associated with requestType. Returns ErrRequestEntityTypeMismatch when
// the request's EntityType isn't in the supported map (defensive: should
// never trip in production because CreateRequest gates entity_type up
// front, but the guard keeps the failure mode loud if a new type is
// added to one map but not the other). Returns ErrRequestEntityNotFound
// when the row doesn't exist.
func (s *RequestService) validateFulfillmentEntity(requestID uint, requestType string, entityID uint) error {
	table, ok := requestEntityTables[requestType]
	if !ok {
		return apperrors.ErrRequestEntityTypeMismatch(requestID, requestType, "<unknown>")
	}

	var count int64
	if err := s.db.Table(table).Where("id = ?", entityID).Count(&count).Error; err != nil {
		return fmt.Errorf("failed to validate fulfillment entity: %w", err)
	}
	if count == 0 {
		return apperrors.ErrRequestEntityNotFound(requestID, requestType, entityID)
	}
	return nil
}

// ApproveFulfillment finalizes a pending_fulfillment request, flipping
// it to fulfilled and stamping fulfilled_at. Only the original requester
// or an admin may approve — non-requester non-admin returns
// ErrRequestForbidden (403). Request must be in pending_fulfillment
// state; any other state returns ErrRequestInvalidState (409). PSY-748.
func (s *RequestService) ApproveFulfillment(requestID, userID uint, isAdmin bool) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	var request communitym.Request
	err := s.db.First(&request, requestID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperrors.ErrRequestNotFound(requestID)
		}
		return fmt.Errorf("failed to get request: %w", err)
	}

	if request.RequesterID != userID && !isAdmin {
		return apperrors.ErrRequestForbidden(requestID)
	}

	if request.Status != communitym.RequestStatusPendingFulfillment {
		return apperrors.ErrRequestInvalidState(requestID, request.Status, communitym.RequestStatusPendingFulfillment)
	}

	now := time.Now()
	updates := map[string]interface{}{
		"status":       communitym.RequestStatusFulfilled,
		"fulfilled_at": now,
	}

	if err := s.db.Model(&request).Updates(updates).Error; err != nil {
		return fmt.Errorf("failed to approve fulfillment: %w", err)
	}

	return nil
}

// RejectFulfillment returns a pending_fulfillment request to the
// pending state, clearing the proposed fulfiller and (if it was set
// during submission) the proposed entity link. Only the original
// requester or an admin may reject. State must be pending_fulfillment.
// PSY-748.
//
// We zero requested_entity_id on reject because the value was overwritten
// by FulfillRequest with the proposed fulfilling entity (the same column
// is reused — see model comment). Restoring it would require persisting
// the pre-fulfill value separately; the simpler and safer default is to
// clear it so the requester can re-link if they want. Documented as a
// known constraint in the ticket.
func (s *RequestService) RejectFulfillment(requestID, userID uint, isAdmin bool) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	var request communitym.Request
	err := s.db.First(&request, requestID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperrors.ErrRequestNotFound(requestID)
		}
		return fmt.Errorf("failed to get request: %w", err)
	}

	if request.RequesterID != userID && !isAdmin {
		return apperrors.ErrRequestForbidden(requestID)
	}

	if request.Status != communitym.RequestStatusPendingFulfillment {
		return apperrors.ErrRequestInvalidState(requestID, request.Status, communitym.RequestStatusPendingFulfillment)
	}

	// GORM Updates with map skips zero values for pointer fields, so use
	// Update with nil/typed-zero to actually clear fulfiller_id and
	// requested_entity_id back to NULL. Status drops back to pending so
	// the request reappears in browse listings as an open contribution
	// surface.
	if err := s.db.Model(&request).Updates(map[string]interface{}{
		"status":              communitym.RequestStatusPending,
		"fulfiller_id":        gorm.Expr("NULL"),
		"requested_entity_id": gorm.Expr("NULL"),
	}).Error; err != nil {
		return fmt.Errorf("failed to reject fulfillment: %w", err)
	}

	return nil
}

// NotifyRequesterFulfillmentProposed writes an in-app notification_log row to
// the request's owner letting them know a fulfillment was proposed and is
// awaiting their approval. Surfaces in the bell/inbox built for PSY-595.
//
// No-op when requesterID is 0 (the handler couldn't resolve it) or when the
// fulfiller IS the requester (self-fulfill — they already know). Fire-and-
// forget: the caller invokes this via shared.GoSafe; the returned error is for
// logging only and never blocks the fulfillment response. PSY-890.
func (s *RequestService) NotifyRequesterFulfillmentProposed(requestID, requesterID, fulfillerID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}
	if requesterID == 0 || requesterID == fulfillerID {
		return nil
	}

	entry := notificationm.NotificationLog{
		UserID:     requesterID,
		EntityType: notificationm.NotificationEntityRequestFulfillmentProposed,
		EntityID:   requestID,
		Channel:    notificationm.NotificationChannelInApp,
		SentAt:     time.Now().UTC(),
	}
	// Plain insert — one row per proposal. FulfillRequest only succeeds on a
	// pending/in_progress → pending_fulfillment transition and the handler
	// fires this exactly once per success, so no dedup clause is needed. (A
	// re-proposal after a reject is a genuinely new event and SHOULD produce a
	// fresh notification; an ON CONFLICT clause would be a no-op here anyway
	// because filter_id is NULL and Postgres treats NULLs as distinct.)
	if err := s.db.Create(&entry).Error; err != nil {
		return fmt.Errorf("failed to write request-fulfillment notification: %w", err)
	}
	return nil
}

// CloseRequest closes a request (cancelled by owner, rejected by admin).
func (s *RequestService) CloseRequest(requestID, userID uint, isAdmin bool) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	var request communitym.Request
	err := s.db.First(&request, requestID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperrors.ErrRequestNotFound(requestID)
		}
		return fmt.Errorf("failed to get request: %w", err)
	}

	// Must be the requester or admin
	if request.RequesterID != userID && !isAdmin {
		return apperrors.ErrRequestForbidden(requestID)
	}

	// Determine the new status
	newStatus := communitym.RequestStatusCancelled
	if isAdmin && request.RequesterID != userID {
		newStatus = communitym.RequestStatusRejected
	}

	if err := s.db.Model(&request).Update("status", newStatus).Error; err != nil {
		return fmt.Errorf("failed to close request: %w", err)
	}

	return nil
}

// GetUserVote returns the user's vote on a request, or nil if not voted.
func (s *RequestService) GetUserVote(requestID, userID uint) (*communitym.RequestVote, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var vote communitym.RequestVote
	err := s.db.Where("request_id = ? AND user_id = ?", requestID, userID).First(&vote).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get user vote: %w", err)
	}

	return &vote, nil
}

// recalculateVoteCounts recalculates upvotes, downvotes, and vote_score for a request.
func (s *RequestService) recalculateVoteCounts(tx *gorm.DB, requestID uint) error {
	var upvotes, downvotes int64

	tx.Model(&communitym.RequestVote{}).
		Where("request_id = ? AND vote = 1", requestID).
		Count(&upvotes)

	tx.Model(&communitym.RequestVote{}).
		Where("request_id = ? AND vote = -1", requestID).
		Count(&downvotes)

	voteScore := int(upvotes - downvotes)

	return tx.Model(&communitym.Request{}).
		Where("id = ?", requestID).
		Updates(map[string]interface{}{
			"upvotes":    int(upvotes),
			"downvotes":  int(downvotes),
			"vote_score": voteScore,
		}).Error
}
