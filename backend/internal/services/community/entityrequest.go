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
	servicesshared "psychic-homily-backend/internal/services/shared"
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
	sourceDetail []byte,
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
	if len(sourceDetail) > 0 {
		sd := json.RawMessage(sourceDetail)
		req.SourceDetail = &sd
	}

	if autoApproves(user, confirmed) {
		now := time.Now().UTC()
		// Copy to a local before taking its address — &user.ID would alias the
		// caller's struct field, which is a footgun if the caller later reuses
		// or mutates the user value.
		deciderID := user.ID
		req.DecisionState = communitym.EntityRequestStateApproved
		req.DecidedBy = &deciderID
		req.DecidedAt = &now
	}

	if err := s.db.Create(req).Error; err != nil {
		// Dedup (PSY-1008): the partial unique index blocks a second PENDING
		// request for the same (entity_type, requester, normalized name). Treat
		// the collision as idempotent — return the existing pending row so a
		// repeated paste/extraction line resolves to the same queued request
		// instead of erroring. The index is partial on decision_state='pending',
		// so only pending rows can collide; this never masks a clash on an
		// already-decided row.
		if servicesshared.IsDuplicateKey(err) {
			if existing, ferr := s.findPendingDuplicate(entityType, user.ID, payload); ferr == nil && existing != nil {
				return existing, nil
			}
		}
		return nil, fmt.Errorf("failed to create entity request: %w", err)
	}
	return req, nil
}

// findPendingDuplicate returns the existing PENDING request that collides with a
// would-be-new request on the dedup key (entity_type, requester, normalized
// name), or (nil, nil) if none. The name comparison uses the SAME Postgres
// expression as the uq_entity_requests_pending_dedup index —
// lower(trim(coalesce(payload->>'name', payload->>'title'))) — applied to BOTH
// the stored row's payload and the candidate payload, so there is no Go-vs-SQL
// normalization mismatch (e.g. collation-sensitive lowercasing). Requester is
// preloaded to match GetRequest's shape.
func (s *EntityRequestService) findPendingDuplicate(entityType string, requesterID uint, payload []byte) (*communitym.EntityRequest, error) {
	const storedName = "lower(trim(coalesce(payload->>'name', payload->>'title')))"
	const candidateName = "lower(trim(coalesce(?::jsonb->>'name', ?::jsonb->>'title')))"
	candidate := string(payload)

	var existing communitym.EntityRequest
	err := s.db.Preload("Requester").
		Where("entity_type = ? AND requester_id = ? AND decision_state = ?",
			entityType, requesterID, communitym.EntityRequestStatePending).
		Where(storedName+" = "+candidateName, candidate, candidate).
		First(&existing).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &existing, nil
}

// RecordFulfillment persists created_entity_id on a fulfilled request (PSY-1008).
// The handler calls it after the fulfiller creates the catalog entity, on both
// the auto-approve create path and the admin approve path. A scoped UPDATE of
// the single column; created_entity_id has no FK (cross-type id keyed by
// entity_type), so this is a plain write. Not-found is an error so a caller
// passing a stale id learns of it rather than silently succeeding.
func (s *EntityRequestService) RecordFulfillment(requestID, createdEntityID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}
	result := s.db.Model(&communitym.EntityRequest{}).
		Where("id = ?", requestID).
		Update("created_entity_id", createdEntityID)
	if result.Error != nil {
		return fmt.Errorf("failed to record fulfillment: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperrors.ErrEntityRequestNotFound(requestID)
	}
	return nil
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
//
// Admin authorization is the CALLER's responsibility — this service has only
// the adminID, not the user record. The HTTP handlers added by PSY-853/PSY-845
// MUST register this on an admin-gated route (rc.Admin middleware) per the
// project's admin-gating pattern; the adminID here is recorded as the decider,
// not authenticated by the service.
//
// The state transition is applied ATOMICALLY with a conditional UPDATE
// (... WHERE decision_state = 'pending'), so two concurrent Decide calls on
// the same row can't both win: the second observes RowsAffected == 0 and gets
// the invalid-state conflict instead of silently clobbering the first
// decision. The pre-read distinguishes not-found from already-decided so the
// caller gets the right error.
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

	// Conditional update guards the read-modify-write against a concurrent
	// decision on the same row. WHERE decision_state = 'pending' so only the
	// first writer flips it; a racing second writer matches 0 rows.
	result := s.db.Model(&communitym.EntityRequest{}).
		Where("id = ? AND decision_state = ?", requestID, communitym.EntityRequestStatePending).
		Updates(updates)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to record decision: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		// Lost the race (or the row was decided between our read and write):
		// re-read to report the current state. Fall back to the pre-read state
		// if the re-read fails so we still surface a conflict, not a 500.
		current := string(req.DecisionState)
		if fresh, ferr := s.GetRequest(requestID); ferr == nil && fresh != nil {
			current = string(fresh.DecisionState)
		}
		return nil, apperrors.ErrEntityRequestInvalidState(requestID, current)
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
