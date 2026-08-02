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
func (s *RevisionService) GetEntityHistory(entityType string, entityID uint, limit, offset int) ([]adminm.Revision, int64, error) {
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

	s.applyPrivacyRedaction(revisions)
	return revisions, total, nil
}

// GetRevision retrieves a single revision by ID, redacted for serving.
// Returns nil, nil if not found.
func (s *RevisionService) GetRevision(revisionID uint) (*adminm.Revision, error) {
	revision, err := s.getRevisionRaw(revisionID)
	if err != nil || revision == nil {
		return nil, err
	}

	// Redact through a one-element batch so the served copy goes through
	// exactly the same code path as a list read; there is no second spelling
	// of the policy to fall out of sync.
	batch := []adminm.Revision{*revision}
	s.applyPrivacyRedaction(batch)
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
func (s *RevisionService) GetUserRevisions(userID uint, limit, offset int) ([]adminm.Revision, int64, error) {
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

	s.applyPrivacyRedaction(revisions)
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

// applyPrivacyRedaction masks, in place, the values that revision history is
// not allowed to publish for the entity a revision belongs to.
//
// # The policy
//
// Every revision read endpoint is anonymous (routes/revisions.go registers all
// three against the bare API group), so recorded values are world-readable.
// Today exactly one FIELD family is withheld: an UNVERIFIED venue's address and
// zipcode, which is the same rule catalog.Venue.PublicAddress / PublicZipcode
// apply to the live venue payload, because an unverified venue is routinely a
// DIY show at somebody's home. Without this, editing the address of such a
// venue once published it here permanently, which made the live gate
// decorative. The field list itself lives in revisiondiff (privacy.go) beside
// the diff it describes.
//
// This function only knows about field-level rules. Two suppressions of a
// different shape are deliberately NOT handled here and are tracked separately,
// so do not read "one field family" as "one gap": a non-approved show is hidden
// wholesale by its detail route while its history is not, and merging an
// unverified venue into a verified one re-points the loser's masked history onto
// a row that passes the check below. Both are recorded on revisiondiff's package
// doc and on MergeVenues respectively.
//
// # Why the gate is caller-independent
//
// It mirrors the live gate exactly, which turns on venues.verified alone and
// has no caller tier: an admin loading an unverified venue's detail page does
// not see its address either. Admin surfaces that legitimately need the raw
// value read it directly (the moderation queue, the data-sync export), and
// rollback reads the stored row through getRevisionRaw, so nothing in the
// moderation workflow depends on the served copy.
//
// # Failure mode
//
// Fail closed. A lookup error or a missing venue row leaves the venue out of
// the verified set, so its history is masked. Withholding an address that
// turned out to be publishable is recoverable; the reverse is not.
func (s *RevisionService) applyPrivacyRedaction(revisions []adminm.Revision) {
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
		if _, ok := verified[revisions[i].EntityID]; ok {
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
	if s.db == nil {
		return verified
	}

	var found []uint
	err := s.db.Model(&catalogm.Venue{}).
		Where("id IN ? AND verified = ?", ids, true).
		Pluck("id", &found).Error
	if err != nil {
		logger.Default().Error("revision_privacy_venue_lookup_failed",
			"venue_ids", len(ids),
			"error", err.Error(),
		)
		return verified
	}

	for _, id := range found {
		verified[id] = struct{}{}
	}
	return verified
}

// redactVenueRevision masks the private field values in one unverified-venue
// revision, rewriting its FieldChanges JSON only when something was actually
// masked. Leaving untouched rows byte-identical keeps every non-address diff
// clear of a marshal round trip that could re-render its stored values.
func redactVenueRevision(r *adminm.Revision) {
	if r.FieldChanges == nil {
		return
	}

	var changes []adminm.FieldChange
	if err := json.Unmarshal(*r.FieldChanges, &changes); err != nil {
		// Unreadable stored JSON: the handler would render no changes anyway,
		// but it renders them from these same bytes, so blank them rather than
		// pass along a blob nobody in this function could inspect.
		blanked := json.RawMessage("[]")
		r.FieldChanges = &blanked
		logger.Default().Error("revision_privacy_field_changes_unreadable",
			"revision_id", r.ID,
			"error", err.Error(),
		)
		return
	}

	redacted, changed := revisiondiff.RedactVenueChanges(changes)
	if !changed {
		return
	}

	encoded, err := json.Marshal(redacted)
	if err != nil {
		blanked := json.RawMessage("[]")
		r.FieldChanges = &blanked
		logger.Default().Error("revision_privacy_remarshal_failed",
			"revision_id", r.ID,
			"error", err.Error(),
		)
		return
	}
	raw := json.RawMessage(encoded)
	r.FieldChanges = &raw
}
