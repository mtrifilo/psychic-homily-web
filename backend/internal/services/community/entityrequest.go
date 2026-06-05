package community

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	communitym "psychic-homily-backend/internal/models/community"
)

// PSY-869: EntityRequestService implements the trust-tier-gated creation flow
// for the polymorphic entity_requests moderation queue.
//
// Trust-tier gating (the core behavior this service adds over a plain insert):
//   - admin / local_ambassador  → auto-approve on create (skip the queue).
//   - trusted_contributor       → the confirm step is FE-side; the backend
//     auto-approves on a confirmed request (same effect as the auto-approve
//     tiers — a confirmed trusted request does not wait in the queue).
//   - contributor / new_user    → inserted as 'pending', queued for an admin.
//
// Tier names are mirrored from internal/services/admin/auto_promotion.go
// (TierNewUser/TierContributor/...). We mirror rather than import to match
// the existing community-service convention (see collection.go's
// collectionTierLimits) and avoid coupling community → admin. The CI
// parity check guards the payload structs, not these tier strings; the
// admin package is the canonical source if they ever change.
const (
	tierNewUser            = "new_user"
	tierContributor        = "contributor"
	tierTrustedContributor = "trusted_contributor"
	tierLocalAmbassador    = "local_ambassador"
)

// EntityRequestService handles entity-creation request business logic.
type EntityRequestService struct {
	db *gorm.DB
}

// NewEntityRequestService creates a new entity-request service.
func NewEntityRequestService(database *gorm.DB) *EntityRequestService {
	if database == nil {
		database = db.GetDB()
	}
	return &EntityRequestService{db: database}
}

// autoApproves reports whether a request from this user/confirmation state
// skips the admin queue. The decision is the single place trust-tier policy
// lives — keep it small and total so a new tier is an obvious one-line change.
//
//   - Admins always auto-approve.
//   - local_ambassador auto-approves (highest non-admin trust).
//   - trusted_contributor auto-approves ONLY on a FE-confirmed request; an
//     unconfirmed trusted request still queues (the FE confirm step is the
//     trusted-tier gate per the ticket).
//   - contributor / new_user never auto-approve.
func autoApproves(user *authm.User, confirmed bool) bool {
	if user.IsAdmin {
		return true
	}
	switch user.UserTier {
	case tierLocalAmbassador:
		return true
	case tierTrustedContributor:
		return confirmed
	case tierContributor, tierNewUser:
		return false
	default:
		// Unknown tier → fail closed (queue it). Never auto-approve a tier
		// we don't recognize.
		return false
	}
}

// CreateRequest persists a typed entity-creation request, applying trust-tier
// gating to decide whether it auto-approves or queues for admin review.
//
// payload is the typed, already-marshalled JSONB body (build it with
// communitym.MarshalPayload(typedStruct) at the call site so the entity_type
// and the payload shape can't drift). entityType MUST have a registered
// payload struct; sourceContext is the origin discriminator.
//
// confirmed reflects the FE-side confirm step relevant to trusted_contributor
// (ignored for other tiers). On auto-approve, decided_by is stamped with the
// requester's own ID and decided_at with now — the request didn't pass through
// an admin, but the columns must record WHO/WHEN the auto-decision happened
// for the audit trail.
func (s *EntityRequestService) CreateRequest(
	user *authm.User,
	entityType string,
	payload []byte,
	sourceContext string,
	confirmed bool,
) (*communitym.EntityRequest, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if user == nil {
		return nil, fmt.Errorf("user is required")
	}
	if !communitym.IsValidEntityRequestType(entityType) {
		return nil, apperrors.ErrEntityRequestInvalidType(entityType)
	}
	if !isValidSourceContext(sourceContext) {
		return nil, apperrors.ErrEntityRequestInvalidSource(sourceContext)
	}
	if len(payload) == 0 {
		return nil, apperrors.ErrEntityRequestEmptyPayload(entityType)
	}

	raw := json.RawMessage(payload)
	req := &communitym.EntityRequest{
		EntityType:    entityType,
		Payload:       &raw,
		RequesterID:   user.ID,
		SourceContext: sourceContext,
		DecisionState: communitym.EntityRequestStatePending,
	}

	if autoApproves(user, confirmed) {
		now := time.Now().UTC()
		req.DecisionState = communitym.EntityRequestStateApproved
		req.DecidedBy = &user.ID
		req.DecidedAt = &now
	}

	if err := s.db.Create(req).Error; err != nil {
		return nil, fmt.Errorf("failed to create entity request: %w", err)
	}
	return req, nil
}

// GetRequest retrieves an entity request by ID with requester + decider
// preloaded. Returns (nil, nil) when not found.
func (s *EntityRequestService) GetRequest(requestID uint) (*communitym.EntityRequest, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var req communitym.EntityRequest
	err := s.db.Preload("Requester").Preload("Decider").First(&req, requestID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get entity request: %w", err)
	}
	return &req, nil
}

// ListPending returns pending requests (the admin moderation queue), optionally
// filtered by entity_type, newest-first.
func (s *EntityRequestService) ListPending(entityType string, limit, offset int) ([]communitym.EntityRequest, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	query := s.db.Model(&communitym.EntityRequest{}).
		Where("decision_state = ?", communitym.EntityRequestStatePending)
	if entityType != "" {
		query = query.Where("entity_type = ?", entityType)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count entity requests: %w", err)
	}

	if limit <= 0 {
		limit = 20
	}

	var reqs []communitym.EntityRequest
	if err := query.Preload("Requester").
		Order("created_at DESC").
		Limit(limit).Offset(offset).
		Find(&reqs).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list entity requests: %w", err)
	}
	return reqs, total, nil
}

// Decide records an admin's moderation decision on a pending request. Only
// 'approved' or 'rejected' are valid targets; the request MUST currently be
// 'pending' (re-deciding a resolved request returns ErrEntityRequestInvalidState).
func (s *EntityRequestService) Decide(
	requestID, adminID uint,
	newState communitym.EntityRequestDecisionState,
	note *string,
) (*communitym.EntityRequest, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if newState != communitym.EntityRequestStateApproved && newState != communitym.EntityRequestStateRejected {
		return nil, apperrors.ErrEntityRequestInvalidDecision(string(newState))
	}

	var req communitym.EntityRequest
	if err := s.db.First(&req, requestID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrEntityRequestNotFound(requestID)
		}
		return nil, fmt.Errorf("failed to get entity request: %w", err)
	}

	if req.DecisionState != communitym.EntityRequestStatePending {
		return nil, apperrors.ErrEntityRequestInvalidState(requestID, string(req.DecisionState))
	}

	now := time.Now().UTC()
	updates := map[string]interface{}{
		"decision_state": newState,
		"decided_by":     adminID,
		"decided_at":     now,
	}
	if note != nil {
		updates["decision_note"] = *note
	}
	if err := s.db.Model(&req).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("failed to record decision: %w", err)
	}

	return s.GetRequest(requestID)
}

// isValidSourceContext reports whether sourceContext is a recognized origin.
func isValidSourceContext(sourceContext string) bool {
	switch sourceContext {
	case communitym.EntityRequestSourceAIExtraction,
		communitym.EntityRequestSourcePasteMode,
		communitym.EntityRequestSourceManual:
		return true
	default:
		return false
	}
}
