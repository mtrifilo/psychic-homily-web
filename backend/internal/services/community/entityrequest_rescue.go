package community

import (
	"fmt"
	"time"

	communitym "psychic-homily-backend/internal/models/community"
)

// PSY-1088: the rescue path for approved-but-unfulfilled entity_requests.
//
// A row can reach decision_state='approved' AND created_entity_id IS NULL with
// no way to re-process it via Decide (which only claims PENDING rows). Two
// by-design routes lead here:
//   - a trusted-tier auto-approved SHOW request (the auto-approve path can't
//     supply the venue + artist associations CreateShow needs — PSY-1037 — so
//     it logs entity_request_autoapprove_fulfill_deferred and leaves the row);
//   - a post-claim fulfillment failure on the admin decide path (Decide
//     committed, then CreateX failed — e.g. a duplicate headliner, a transient
//     DB error).
//
// These methods, added as a SIBLING to PSY-869's entityrequest.go (mirroring
// entityrequest_list.go), let an admin fulfill or void such a row directly,
// bypassing Decide. Both use a conditional UPDATE scoped to the orphan state
// (decision_state='approved' AND created_entity_id IS NULL) so a concurrent
// rescue, or a row that was already fulfilled/voided, can't be double-acted.

// ClaimRescueFulfillment atomically stamps created_entity_id onto an
// approved-but-unfulfilled row. It is the double-fulfill guard the ticket
// requires: the WHERE clause matches only an orphan, so if two admins rescue
// the same row concurrently (each already created a catalog entity), only the
// first UPDATE affects a row — the second sees claimed=false and its created
// entity is a recoverable stray (the same rare-orphan trade-off the decide
// path accepts to never corrupt the link). Distinct from RecordFulfillment
// (PSY-1008), which unconditionally overwrites and is used on the
// create/decide paths where the row was just claimed in the same request.
//
// Returns (false, nil) — NOT an error — when no row matched: the row is
// missing, not approved, or already has a created_entity_id. The caller maps
// that to the appropriate HTTP conflict.
func (s *EntityRequestService) ClaimRescueFulfillment(requestID, createdEntityID uint) (bool, error) {
	if s.db == nil {
		return false, fmt.Errorf("database not initialized")
	}
	result := s.db.Model(&communitym.EntityRequest{}).
		Where("id = ? AND decision_state = ? AND created_entity_id IS NULL",
			requestID, communitym.EntityRequestStateApproved).
		Update("created_entity_id", createdEntityID)
	if result.Error != nil {
		return false, fmt.Errorf("failed to claim rescue fulfillment: %w", result.Error)
	}
	return result.RowsAffected == 1, nil
}

// VoidApprovedUnfulfilled atomically transitions an approved-but-unfulfilled
// row to 'rejected' so an admin can dismiss an orphan that should not have been
// approved (e.g. a bad auto-approve), without DB surgery. decided_by/at are
// re-stamped with the voiding admin + now, and the optional note records why.
//
// The conditional UPDATE (WHERE decision_state='approved' AND created_entity_id
// IS NULL) guarantees a FULFILLED row can never be voided — voiding a row whose
// entity already exists would strand that entity behind a 'rejected' request.
// Returns (false, nil) when no row matched (missing / not approved / already
// fulfilled or voided), which the caller maps to the right conflict.
func (s *EntityRequestService) VoidApprovedUnfulfilled(requestID, adminID uint, note *string) (bool, error) {
	if s.db == nil {
		return false, fmt.Errorf("database not initialized")
	}
	now := time.Now().UTC()
	updates := map[string]interface{}{
		"decision_state": communitym.EntityRequestStateRejected,
		"decided_by":     adminID,
		"decided_at":     now,
	}
	if note != nil {
		updates["decision_note"] = *note
	}
	result := s.db.Model(&communitym.EntityRequest{}).
		Where("id = ? AND decision_state = ? AND created_entity_id IS NULL",
			requestID, communitym.EntityRequestStateApproved).
		Updates(updates)
	if result.Error != nil {
		return false, fmt.Errorf("failed to void approved request: %w", result.Error)
	}
	return result.RowsAffected == 1, nil
}
