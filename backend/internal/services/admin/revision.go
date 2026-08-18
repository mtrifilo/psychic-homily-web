package admin

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/logger"
	adminm "psychic-homily-backend/internal/models/admin"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/shared/revisiondiff"
)

// RevisionService handles revision history business logic.
type RevisionService struct {
	db *gorm.DB
}

// NewRevisionService creates a new revision service.
func NewRevisionService(database *gorm.DB) *RevisionService {
	if database == nil {
		database = db.GetDB()
	}
	return &RevisionService{db: database}
}

// RecordRevision creates a new revision entry for an entity edit.
// If changes is empty, it is a no-op (no revision recorded).
func (s *RevisionService) RecordRevision(entityType string, entityID uint, userID uint, changes []adminm.FieldChange, summary string) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}
	if len(changes) == 0 {
		return nil // No changes, nothing to record
	}

	changesJSON, err := json.Marshal(changes)
	if err != nil {
		return fmt.Errorf("failed to marshal field changes: %w", err)
	}
	raw := json.RawMessage(changesJSON)

	var summaryPtr *string
	if summary != "" {
		summaryPtr = &summary
	}

	revision := &adminm.Revision{
		EntityType:   entityType,
		EntityID:     entityID,
		UserID:       userID,
		FieldChanges: &raw,
		Summary:      summaryPtr,
	}

	if err := s.db.Create(revision).Error; err != nil {
		return fmt.Errorf("failed to create revision: %w", err)
	}
	return nil
}

// GetEntityHistory returns paginated revision history for a specific entity.
//
// viewerIsAdmin selects the caller tier the result is redacted for; see
// applyPrivacyRedaction. False is the masked view, so a caller that cannot prove
// admin gets the public one.
func (s *RevisionService) GetEntityHistory(entityType string, entityID uint, limit, offset int, viewerIsAdmin bool) ([]adminm.Revision, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	var total int64
	s.db.Model(&adminm.Revision{}).
		Where("entity_type = ? AND entity_id = ?", entityType, entityID).
		Count(&total)

	var revisions []adminm.Revision
	err := s.db.Where("entity_type = ? AND entity_id = ?", entityType, entityID).
		Preload("User").
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&revisions).Error
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get entity history: %w", err)
	}

	s.applyPrivacyRedaction(revisions, viewerIsAdmin)
	return revisions, total, nil
}

// GetRevision retrieves a single revision by ID, redacted for serving.
// Returns nil, nil if not found.
//
// viewerIsAdmin selects the caller tier; see applyPrivacyRedaction.
func (s *RevisionService) GetRevision(revisionID uint, viewerIsAdmin bool) (*adminm.Revision, error) {
	revision, err := s.getRevisionRaw(revisionID)
	if err != nil || revision == nil {
		return nil, err
	}

	// Redact through a one-element batch so the served copy goes through
	// exactly the same code path as a list read; there is no second spelling
	// of the policy to fall out of sync.
	batch := []adminm.Revision{*revision}
	s.applyPrivacyRedaction(batch, viewerIsAdmin)
	return &batch[0], nil
}

// getRevisionRaw loads a revision with its stored field_changes untouched.
//
// This is the INTERNAL accessor. Rollback needs it: redacted values are display
// strings, and writing one back would overwrite a venue's real address with
// "(hidden)". Anything that serves a revision to a client must go through
// GetRevision instead.
func (s *RevisionService) getRevisionRaw(revisionID uint) (*adminm.Revision, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var revision adminm.Revision
	err := s.db.Preload("User").First(&revision, revisionID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get revision: %w", err)
	}
	return &revision, nil
}

// GetUserRevisions returns paginated revisions made by a specific user.
//
// userID names the revision AUTHOR; viewerIsAdmin describes the caller READING
// the page. The two are unrelated — an admin reading their own contributions
// gets the admin tier, and a contributor reading an admin's page does not.
func (s *RevisionService) GetUserRevisions(userID uint, limit, offset int, viewerIsAdmin bool) ([]adminm.Revision, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	var total int64
	s.db.Model(&adminm.Revision{}).Where("user_id = ?", userID).Count(&total)

	var revisions []adminm.Revision
	err := s.db.Where("user_id = ?", userID).
		Preload("User").
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&revisions).Error
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get user revisions: %w", err)
	}

	s.applyPrivacyRedaction(revisions, viewerIsAdmin)
	return revisions, total, nil
}

// Rollback applies the inverse of a revision's changes to the entity.
// It creates a new revision recording the rollback.
func (s *RevisionService) Rollback(revisionID uint, adminUserID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	// Raw, not GetRevision: a rollback restores stored values, so it must read
	// the row as written. See getRevisionRaw.
	revision, err := s.getRevisionRaw(revisionID)
	if err != nil {
		return err
	}
	if revision == nil {
		return fmt.Errorf("revision not found")
	}

	// Parse field changes
	var changes []adminm.FieldChange
	if err := json.Unmarshal(*revision.FieldChanges, &changes); err != nil {
		return fmt.Errorf("failed to parse field changes: %w", err)
	}

	// Build update map from old values (reversing the change)
	updates := make(map[string]interface{})
	var rollbackChanges []adminm.FieldChange
	for _, c := range changes {
		updates[c.Field] = c.OldValue
		rollbackChanges = append(rollbackChanges, adminm.FieldChange{
			Field:    c.Field,
			OldValue: c.NewValue,
			NewValue: c.OldValue,
		})
	}

	// Apply update to the entity table
	tableName := revision.EntityType + "s" // artist -> artists, show -> shows, etc.
	updates["updated_at"] = time.Now()

	// Old values come back out of revisions.field_changes as JSONB, so a number
	// is a float64 here just as it is on the approve path. Writing that to an
	// integer column succeeds and truncates rather than failing, so a rollback
	// would quietly restore a DIFFERENT value than the one being undone.
	//
	// Narrowing only: no range check. This restores a value the system already
	// stored, and history can hold values that predate a bound, so refusing them
	// would break undo for precisely the rows most likely to need it.
	//
	// SEPARATE, PRE-EXISTING, not fixed here: revisiondiff flattens a nil *int
	// to 0 when it records a diff (derefInt), so an ADMIN edit that set a
	// previously-NULL integer column writes old_value 0, and rolling it back
	// restores 0 rather than NULL. Contributor edits are unaffected because
	// their revisions are recorded from the raw field_changes, which carry a
	// true null. Fixing that means teaching revisiondiff to emit null, which
	// changes the shape of every historical *int diff.
	if err := NarrowNumericUpdates(updates); err != nil {
		return err
	}

	// A rollback that restores city/state must re-derive whatever the system
	// derives FROM that location, or the entity lands back in its old city still
	// carrying what was resolved for the city it was moved away from. Shared with
	// the approve path, and a no-op for entity types and writes it does not apply
	// to; see applyDerivedLocation.
	applyDerivedLocation(s.db, revision.EntityType, revision.EntityID, updates)

	result := s.db.Table(tableName).Where("id = ?", revision.EntityID).Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("failed to apply rollback: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("entity not found: %s %d", revision.EntityType, revision.EntityID)
	}

	// Record the rollback as a new revision
	summary := fmt.Sprintf("Rollback of revision #%d", revisionID)
	return s.RecordRevision(revision.EntityType, revision.EntityID, adminUserID, rollbackChanges, summary)
}

// =============================================================================
// READ-TIME PRIVACY REDACTION
// =============================================================================

// applyPrivacyRedaction masks, in place, the parts of a revision that revision
// history is not allowed to publish: the private FieldChanges values, and the
// contributor-authored Summary that sits beside them. The policy it enforces,
// the field list, and the gaps it does not cover are stated once on the
// revisiondiff package doc; this is the mechanism.
//
// It takes a caller tier: an authenticated ADMIN reads revision history
// unmasked, everyone else gets the masked view (PSY-1717). WHY that tier exists,
// and why the live venue payload deliberately has none, is argued once on
// revisiondiff.venuePrivateFields — read it before changing the policy. What
// follows here is only what a reader of THIS function needs.
//
// Admin means admin, not merely authenticated. viewerIsAdmin is resolved in the
// handler as `user != nil && user.IsAdmin` over the user the route's
// OptionalHumaJWTMiddleware attached, and that user is loaded FROM THE DATABASE
// during token validation, so a demoted admin loses the tier on their next
// request rather than carrying it in a stale claim. Anonymous callers, invalid
// or expired tokens, and inactive users all leave the context userless, which
// makes the value false and the view masked — the tier fails closed with the
// rest of this function.
//
// It unmasks the WHOLE view for an admin, prose included, not just the address
// family: the summary withholding below rides the same verdict, and a moderator
// reading a rollback candidate needs the contributor's stated reason as much as
// the values. That is also the standing instruction for the NEXT privacy family
// added here — everything in this function sits behind the admin return below,
// so a new gate is admin-transparent unless its author decides otherwise.
// Decide it rather than inherit it.
//
// Fail closed. A lookup error, a nil db, or a missing venue row leaves the
// venue out of the verified set, so its history is masked. Withholding an
// address that turned out to be publishable is recoverable; the reverse is not.
//
// Two conditions mask, not one. venues.verified answers "is the venue this row
// points at gated today"; revisions.from_unverified_venue answers "did this row
// come off a venue that was gated when it was merged away". The second exists
// because a venue merge re-points revisions and then DELETES the venue the
// first condition would have read, so without the marker a merge into a
// verified room republishes the loser's address history. Both conditions have to
// hold for a row to be served, and the marker is per ROW, not per venue: after a
// merge the canonical venue's own publishable history sits beside the loser's
// masked rows under one entity_id.
//
// It masks VALUES, not the fact of an edit: a masked revision still names the
// field, the author and the timestamp. That is the residual, and it is
// deliberate — revision history exists to be auditable.
//
// Summary is the exception to "values, not fields": it is withheld whole. See
// the package doc named above for why, and for what that costs.
func (s *RevisionService) applyPrivacyRedaction(revisions []adminm.Revision, viewerIsAdmin bool) {
	// Ordering, not just placement: this must precede the venue lookup, which
	// fails CLOSED. Applied after it, a DB blip would mask an admin.
	if viewerIsAdmin {
		return
	}

	venueIDs := make([]uint, 0, len(revisions))
	seen := make(map[uint]struct{}, len(revisions))
	for i := range revisions {
		if revisions[i].EntityType != "venue" {
			continue
		}
		if _, dup := seen[revisions[i].EntityID]; dup {
			continue
		}
		seen[revisions[i].EntityID] = struct{}{}
		venueIDs = append(venueIDs, revisions[i].EntityID)
	}
	if len(venueIDs) == 0 {
		return
	}

	verified := s.verifiedVenueIDs(venueIDs)
	for i := range revisions {
		if revisions[i].EntityType != "venue" {
			continue
		}
		// Publishable only if BOTH hold: the venue it points at is verified
		// today, and it was not carried here off an unverified one by a merge.
		_, venueVerified := verified[revisions[i].EntityID]
		if venueVerified && !revisions[i].FromUnverifiedVenue {
			continue
		}
		redactVenueRevision(&revisions[i])
	}
}

// verifiedVenueIDs returns the subset of ids whose venue row is verified, as a
// set. One query for the whole page rather than one per revision.
//
// Returns an empty (non-nil) set on error, which redacts everything — see the
// failure-mode note on applyPrivacyRedaction.
func (s *RevisionService) verifiedVenueIDs(ids []uint) map[uint]struct{} {
	verified := make(map[uint]struct{}, len(ids))
	if s.db == nil || len(ids) == 0 {
		return verified
	}

	var found []uint
	err := s.db.Model(&catalogm.Venue{}).
		Where("id IN ? AND verified = ?", ids, true).
		Pluck("id", &found).Error
	if err != nil {
		logger.Default().Error("revision_privacy_venue_lookup_failed",
			"venue_id_count", len(ids),
			"error", err.Error(),
		)
		return verified
	}

	for _, id := range found {
		verified[id] = struct{}{}
	}
	return verified
}

// redactVenueRevision rewrites one unverified-venue revision for serving: it
// drops the Summary and rebuilds FieldChanges from the parsed diff with the
// private values masked.
//
// Summary is dropped rather than replaced with RedactedValue. There is no
// masked-diff row for it to line up with, and "(hidden)" in the prose slot
// would read as "this venue had something to hide" on every unverified venue's
// history, including the overwhelming majority whose summaries say nothing
// sensitive. Absent is the honest shape: the handler already declares summary
// omitempty and the frontend already renders the row without it.
//
// It ALWAYS re-marshals rather than passing the stored bytes through when no
// private field matched. Serving the stored bytes would make the guarantee
// depend on the stored JSON having exactly the shape adminm.FieldChange models:
// encoding/json silently drops keys the struct does not declare, so a row
// carrying an address under an unmodeled key would parse to a clean-looking
// diff and then be served verbatim. Rebuilding the payload from the three
// fields this function can actually inspect makes what is served a function of
// what was checked. The cost is one marshal of a bounded slice per masked row.
//
// It assigns a NEW *json.RawMessage rather than writing through the existing
// one, and reassigns Summary rather than writing through *r.Summary, for the
// same reason: GetRevision hands this a copy of a struct that still shares both
// pointers with the raw row, so mutating either target in place would corrupt
// the values rollback reads.
func redactVenueRevision(r *adminm.Revision) {
	// Unconditional, and before the FieldChanges early return: a revision with
	// no readable diff can still carry a summary, and that is the one case
	// where the prose is the ONLY thing being served.
	r.Summary = nil

	if r.FieldChanges == nil {
		return
	}

	blank := func(event string, err error) {
		blanked := json.RawMessage("[]")
		r.FieldChanges = &blanked
		logger.Default().Error(event, "revision_id", r.ID, "error", err.Error())
	}

	var changes []adminm.FieldChange
	if err := json.Unmarshal(*r.FieldChanges, &changes); err != nil {
		// Unreadable stored JSON: the handler renders changes from these same
		// bytes, so blank them rather than pass along a blob nothing here could
		// inspect.
		blank("revision_privacy_field_changes_unreadable", err)
		return
	}

	encoded, err := json.Marshal(revisiondiff.RedactVenueChanges(changes))
	if err != nil {
		blank("revision_privacy_remarshal_failed", err)
		return
	}
	raw := json.RawMessage(encoded)
	r.FieldChanges = &raw
}
