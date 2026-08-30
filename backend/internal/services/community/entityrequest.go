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
//
// replaced reports that the submission landed on the requester's EXISTING
// pending row rather than a new one (PSY-1948); see the dedup branch below. It
// is always false for an auto-approving tier: the row is stamped 'approved'
// BEFORE the INSERT, so it never collides with the pending-only dedup index.
func (s *EntityRequestService) CreateRequest(
	user *authm.User,
	entityType string,
	payload []byte,
	sourceContext string,
	sourceDetail []byte,
	confirmed bool,
) (*communitym.EntityRequest, bool, error) {
	if s.db == nil {
		return nil, false, fmt.Errorf("database not initialized")
	}
	if user == nil {
		return nil, false, fmt.Errorf("user is required")
	}
	if !communitym.IsValidEntityRequestType(entityType) {
		return nil, false, apperrors.ErrEntityRequestInvalidType(entityType)
	}
	if !isValidSourceContext(sourceContext) {
		return nil, false, apperrors.ErrEntityRequestInvalidSource(sourceContext)
	}
	if len(payload) == 0 {
		return nil, false, apperrors.ErrEntityRequestEmptyPayload(entityType)
	}

	raw := json.RawMessage(payload)
	req := &communitym.EntityRequest{
		EntityType:    entityType,
		Payload:       &raw,
		RequesterID:   user.ID,
		SourceContext: sourceContext,
		DecisionState: communitym.EntityRequestStatePending,
	}
	req.SourceDetail = nullableJSONB(sourceDetail)

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
		// request for the same (entity_type, requester, normalized name). The
		// index is partial on decision_state='pending', so only pending rows can
		// collide; this never masks a clash on an already-decided row.
		//
		// PSY-1948: the collision REPLACES that pending row's submission rather
		// than returning it untouched. A resubmission is the requester's current
		// intent, most often a correction, and this is the ONLY way to change a
		// queued payload — answering with the stored one discarded the correction
		// behind a success response.
		if servicesshared.IsDuplicateKey(err) {
			if existing, ferr := s.findPendingDuplicate(entityType, user.ID, payload); ferr == nil && existing != nil {
				refreshed, rerr := s.replacePendingSubmission(existing, entityType, payload, sourceContext, sourceDetail)
				if rerr != nil {
					return nil, false, rerr
				}
				if refreshed != nil {
					return refreshed, true, nil
				}
			}
			// Three cases fall through to the wrapped duplicate-key error, and
			// none of them writes anything: the lookup errored, or the colliding
			// row was decided between the constraint violation and the lookup (so
			// the lookup finds nothing — the partial index no longer covers it),
			// or between the lookup and the conditional update (so the update
			// matches no row). The last two are narrow races that leave a rare
			// transient error the caller can simply retry, since a retry now
			// inserts a fresh pending row, not a data fault.
		}
		return nil, false, fmt.Errorf("failed to create entity request: %w", err)
	}
	return req, false, nil
}

// replacePendingSubmission overwrites the submission fields of a PENDING request
// with a resubmission's and returns the refreshed row (PSY-1948). Payload,
// source_context and source_detail move together because they describe ONE
// submission: a resubmission carrying no source_detail clears the stored one
// rather than inheriting a detail that described the superseded payload. The
// replacement is total, never a merge — the new submission is the requester's
// current intent, and a merge would preserve exactly the stale fields the
// resubmission exists to correct.
//
// decision_state, requester and created_at are untouched, so the row keeps its
// identity and its place in the moderation queue.
//
// The UPDATE carries the whole precondition — this row, this entity type, this
// requester, still pending — the way Decide and the rescue writers do, so the
// destructive write is self-guarding rather than trusting its caller's lookup.
// entity_type is in there because it is what decides which payload schema the
// validation above applies: without it a mismatched (id, entityType) pair would
// write a well-formed payload of the WRONG type and both guards would pass.
// Conditioning on the state is what makes a replace racing an admin decision lose
// instead of resurrecting a payload onto a just-decided row.
//
// Returns (nil, nil) ONLY when no row matched, i.e. nothing was written. Once the
// UPDATE commits this cannot fail: the refreshed row is built from `existing`
// plus the fields just written, never re-read.
func (s *EntityRequestService) replacePendingSubmission(
	existing *communitym.EntityRequest,
	entityType string,
	payload []byte,
	sourceContext string,
	sourceDetail []byte,
) (*communitym.EntityRequest, error) {
	requestID := existing.ID
	// The write DESTROYS the stored payload, and the superseded one is not
	// recoverable, so the structure is checked here as well as at the API
	// boundary: a caller that skipped validation would replace a good queued
	// payload with junk. Mirrors fulfillEntity, which re-validates the STORED
	// payload at the other end of the same row's lifecycle. Reaching this is a
	// caller bug, not contributor input — the API boundary answers a bad payload
	// with a 422 long before here.
	if err := communitym.ValidateEntityRequestPayload(entityType, payload); err != nil {
		return nil, fmt.Errorf("refusing to replace pending entity request %d with an invalid %s payload: %w",
			requestID, entityType, err)
	}

	// updated_at is set explicitly rather than left to GORM's auto-timestamp so
	// the returned row can carry the value that was actually stored. It is the
	// version the decide path claims against, so a guess would be worse than
	// useless.
	now := time.Now().UTC()
	newPayload := json.RawMessage(payload)
	newDetail := nullableJSONB(sourceDetail)
	updates := map[string]interface{}{
		"payload":        newPayload,
		"source_context": sourceContext,
		"source_detail":  newDetail,
		"updated_at":     now,
	}

	result := s.db.Model(&communitym.EntityRequest{}).
		Where("id = ? AND entity_type = ? AND requester_id = ? AND decision_state = ?",
			requestID, entityType, existing.RequesterID, communitym.EntityRequestStatePending).
		Updates(updates)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to replace pending entity request: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}

	// The refreshed row is BUILT from the row we matched plus the fields we just
	// wrote, not re-read. A second statement here would be a second way to fail
	// AFTER the destructive write committed — reporting a 500 for a correction
	// that landed, and skipping the audit row that is the only record it landed.
	// Every column that changed is known here, and no other writer can touch a
	// pending row's remaining columns.
	refreshed := *existing
	refreshed.Payload = &newPayload
	refreshed.SourceContext = sourceContext
	refreshed.SourceDetail = newDetail
	refreshed.UpdatedAt = now
	return &refreshed, nil
}

// findPendingDuplicate returns the existing PENDING request that collides with a
// would-be-new request on the dedup key (entity_type, requester, normalized
// name), or (nil, nil) if none. The name comparison uses the SAME Postgres
// expression as the uq_entity_requests_pending_dedup index —
// lower(trim(coalesce(payload->>'name', payload->>'title'))) — applied to BOTH
// the stored row's payload and the candidate payload, so there is no Go-vs-SQL
// normalization mismatch (e.g. collation-sensitive lowercasing).
//
// The row is used only to identify WHICH pending row the resubmission replaces;
// the refreshed row the caller returns comes from GetRequest, so nothing is
// preloaded here.
func (s *EntityRequestService) findPendingDuplicate(entityType string, requesterID uint, payload []byte) (*communitym.EntityRequest, error) {
	const storedName = "lower(trim(coalesce(payload->>'name', payload->>'title')))"
	const candidateName = "lower(trim(coalesce(?::jsonb->>'name', ?::jsonb->>'title')))"
	candidate := string(payload)

	var existing communitym.EntityRequest
	err := s.db.
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
//
// expectedUpdatedAt makes the claim OPTIMISTIC (PSY-1948). PSY-1948 made a queued
// payload mutable by its requester, so a caller that validated the payload before
// calling here — which the approve path does, and must, because its SSRF host
// guard and its show-bill admission run pre-claim and are NOT re-run at
// fulfillment — can otherwise claim a row whose payload changed underneath it and
// fulfill content nothing validated. Pass the updated_at that read saw and the
// claim refuses a revised row with ErrEntityRequestStale, leaving it pending for
// the caller to re-read. nil skips the check, which is correct only for a
// decision that reads nothing off the row (a rejection creates no entity).
func (s *EntityRequestService) Decide(
	requestID, adminID uint,
	newState communitym.EntityRequestDecisionState,
	note *string,
	expectedUpdatedAt *time.Time,
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
	// first writer flips it; a racing second writer matches 0 rows. When the
	// caller supplied one, updated_at joins the condition so a row REVISED since
	// their read matches 0 rows too.
	claim := s.db.Model(&communitym.EntityRequest{}).
		Where("id = ? AND decision_state = ?", requestID, communitym.EntityRequestStatePending)
	if expectedUpdatedAt != nil {
		claim = claim.Where("updated_at = ?", *expectedUpdatedAt)
	}
	result := claim.Updates(updates)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to record decision: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		// Lost the race: re-read to tell the two losses apart. A row that is no
		// longer pending was decided by someone else; a row that is STILL pending
		// matched nothing only because its payload was replaced since the caller's
		// read, which is a different conflict and a different instruction to the
		// admin (look again, then decide). Fall back to the pre-read state if the
		// re-read fails so we still surface a conflict, not a 500.
		current := string(req.DecisionState)
		if fresh, ferr := s.GetRequest(requestID); ferr == nil && fresh != nil {
			current = string(fresh.DecisionState)
		}
		if current == string(communitym.EntityRequestStatePending) && expectedUpdatedAt != nil {
			return nil, apperrors.ErrEntityRequestStale(requestID)
		}
		return nil, apperrors.ErrEntityRequestInvalidState(requestID, current)
	}

	return s.GetRequest(requestID)
}

// nullableJSONB wraps already-marshalled JSONB bytes for storage, answering nil
// for empty input so the column holds NULL rather than an empty object. Shared by
// the create and the replace paths so "no source detail" means the same thing in
// both: a resubmission with no detail CLEARS the stored one.
func nullableJSONB(b []byte) *json.RawMessage {
	if len(b) == 0 {
		return nil
	}
	raw := json.RawMessage(b)
	return &raw
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
