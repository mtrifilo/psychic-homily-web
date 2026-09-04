package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
	adminm "psychic-homily-backend/internal/models/admin"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared/revisiondiff"
)

// AUTHOR COLUMN CONTRACT — why all three reads below Preload the WHOLE user
// row, and what breaks if someone narrows that.
//
// The handler resolves the author's byline from this preloaded relation, and
// the two tiers it serves need DIFFERENT columns (PSY-1940):
//
//   - the public tier reads display_name, username, first_name, last_name to
//     resolve a name, plus privacy_settings and profile_visibility to decide
//     whether it may publish one;
//   - the admin tier reads email as well, as the last resort for an account
//     that set no public name.
//
// So the union is every identity column, and the union is what a bare
// Preload("User") gives.
//
// Narrowing it is a tempting optimisation: this pulls password_hash and the
// rest for every row of every history page. Do not. A Select that omits
// privacy_settings does not fail loudly — the column scans as nil, which
// unmarshals to the DEFAULTS, which have contributions VISIBLE, and every
// contributor who asked not to be credited is silently named again. Dropping
// profile_visibility restores links to profiles that 404. Dropping email
// breaks the admin tier's last resort.
//
// If this must be narrowed, list all of: id, username, display_name,
// first_name, last_name, email, privacy_settings, profile_visibility — and add
// a test that reads a hidden contributor's history through the real query, of
// which routes/revision_viewer_tier_test.go is the exemplar. An in-memory test
// of the mapper cannot catch a column that was never selected.

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
// viewer selects the caller the result is gated and redacted for; see
// revision_visibility.go for the entity gate and applyPrivacyRedaction for the
// field masking. contracts.RevisionViewer{} is the public view, so a caller
// that cannot prove anything gets it.
//
// Returns contracts.ErrRevisionEntityHidden when the entity itself is one this
// viewer may not see. The handler turns that into the same 404 an absent entity
// gets.
func (s *RevisionService) GetEntityHistory(entityType string, entityID uint, limit, offset int, viewer contracts.RevisionViewer) ([]adminm.Revision, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	if err := s.requireEntityVisible(entityType, entityID, viewer); err != nil {
		return nil, 0, err
	}

	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	// Row filtering runs even though the entity gate above already passed: a
	// visible show can still hold rows a merge carried off a gated one.
	//
	// The count error is returned rather than dropped. Since PSY-1715 the total
	// is a claim about the page beside it, and the page's own query carries a
	// LIMIT this one does not — so a statement timeout can fail the count while
	// the page succeeds, answering 200 with rows beside total 0. That inverts the
	// exact invariant the filter exists to hold.
	var total int64
	if err := visibleRevisionsOnly(s.db.Model(&adminm.Revision{}).
		Where("entity_type = ? AND entity_id = ?", entityType, entityID), viewer).
		Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count entity history: %w", err)
	}

	var revisions []adminm.Revision
	err := visibleRevisionsOnly(s.db.Model(&adminm.Revision{}).
		Where("entity_type = ? AND entity_id = ?", entityType, entityID), viewer).
		// DO NOT NARROW THIS PRELOAD WITH A Select — see the AUTHOR COLUMN CONTRACT note at the top of this file.
		Preload("User").
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&revisions).Error
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get entity history: %w", err)
	}

	s.applyPrivacyRedaction(revisions, viewer.IsAdmin)
	return revisions, total, nil
}

// GetRevision retrieves a single revision by ID, gated and redacted for
// serving. Returns nil, nil if not found.
//
// viewer selects the caller; see revision_visibility.go and
// applyPrivacyRedaction.
//
// A revision whose entity this viewer may not see returns nil, nil — the same
// answer a missing row gives, and deliberately indistinguishable from it. This
// route is the one that takes an opaque id, so a distinguishable "hidden"
// answer would let a caller sweep the id space for unpublished shows.
func (s *RevisionService) GetRevision(revisionID uint, viewer contracts.RevisionViewer) (*adminm.Revision, error) {
	revision, err := s.getRevisionRaw(revisionID)
	if err != nil || revision == nil {
		return nil, err
	}

	if !s.revisionVisibleTo(revision, viewer) {
		return nil, nil
	}

	// Redact through a one-element batch so the served copy goes through
	// exactly the same code path as a list read; there is no second spelling
	// of the policy to fall out of sync.
	batch := []adminm.Revision{*revision}
	s.applyPrivacyRedaction(batch, viewer.IsAdmin)
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
	// DO NOT NARROW THIS PRELOAD WITH A Select — see the AUTHOR COLUMN CONTRACT note at the top of this file.
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
// userID names the revision AUTHOR; viewer describes the caller READING the
// page. The two are unrelated — an admin reading their own contributions gets
// the admin tier, and a contributor reading an admin's page does not.
//
// Authorship is not visibility either. A contributor's edit to a show that was
// later unpublished drops off their own contributions page, because the page is
// world-readable and the row would be published to anyone who opened it. The
// author keeps the edit on any show they SUBMITTED, which is the tier the detail
// route grants them.
func (s *RevisionService) GetUserRevisions(userID uint, limit, offset int, viewer contracts.RevisionViewer) ([]adminm.Revision, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	// This route is indexed by a PERSON, which makes the whole listing a
	// contributions page and puts it under the setting that governs those. See
	// requireAuthorContributionsVisible for why suppressing the byline alone
	// would not have been enough.
	if err := s.requireAuthorContributionsVisible(userID, viewer); err != nil {
		return nil, 0, err
	}

	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	// Returned, not dropped — see the note on GetEntityHistory's count.
	var total int64
	if err := visibleRevisionsOnly(s.db.Model(&adminm.Revision{}).Where("user_id = ?", userID), viewer).
		Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count user revisions: %w", err)
	}

	var revisions []adminm.Revision
	err := visibleRevisionsOnly(s.db.Model(&adminm.Revision{}).Where("user_id = ?", userID), viewer).
		// DO NOT NARROW THIS PRELOAD WITH A Select — see the AUTHOR COLUMN CONTRACT note at the top of this file.
		Preload("User").
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&revisions).Error
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get user revisions: %w", err)
	}

	s.applyPrivacyRedaction(revisions, viewer.IsAdmin)
	return revisions, total, nil
}

// Rollback restores the previous value of every field a revision recorded that
// the apply-side gates still accept, and reports the ones it refused.
//
// PER FIELD, NOT PER REVISION, and that is the load-bearing property. The values
// written here come out of revisions.field_changes, and a stored old_value can be
// one no forward path would accept: rows written before the submit-time
// derivation took the submitter's word for it, so a contributor could pair a
// legitimate new value with an arbitrary previous one and have an admin's undo
// write it live. Refusing the whole revision for one such field also refused the
// undo of every honest field recorded beside it, which let a contributor deny
// undo of their own edit and made a legacy value enough to strand an admin's own
// revision.
//
// A rollback that can restore NOTHING is an error, so a caller never reports a
// rollback that did nothing.
//
// The recorded rollback revision and the returned result describe the SAME
// fields: history claiming to have restored a field this call skipped would be a
// second wrong answer on top of the first.
func (s *RevisionService) Rollback(ctx context.Context, revisionID uint, adminUserID uint) (*contracts.RollbackResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Raw, not GetRevision: a rollback restores stored values, so it must read
	// the row as written. See getRevisionRaw.
	revision, err := s.getRevisionRaw(revisionID)
	if err != nil {
		return nil, err
	}
	if revision == nil {
		return nil, fmt.Errorf("revision not found")
	}

	// Parse field changes
	var changes []adminm.FieldChange
	if err := json.Unmarshal(*revision.FieldChanges, &changes); err != nil {
		return nil, fmt.Errorf("failed to parse field changes: %w", err)
	}

	// Build update map from old values (reversing the change)
	updates := make(map[string]interface{})
	for _, c := range changes {
		updates[c.Field] = c.OldValue
	}

	// Judge each field on its own, then drop the refused ones from the write.
	// Every rule the approve path runs over a whole map runs here over one field,
	// sharing the rule rather than holding a second copy of it.
	//
	// Old values come back out of revisions.field_changes as JSONB, so a number
	// is a float64 here just as it is on the approve path. Writing that to an
	// integer column succeeds and truncates rather than failing, so a rollback
	// would quietly restore a DIFFERENT value than the one being undone.
	//
	// Narrowing only: no range check. This restores a value the system already
	// stored, and history can hold values that predate a bound, so refusing them
	// would break undo for precisely the rows most likely to need it.
	//
	// A nil OldValue means the column was NULL before the edit, and it has to
	// land as SQL NULL for the undo to be faithful. narrowNumericUpdate turns a
	// REGISTERED field's nil into a typed (*int)(nil) for exactly that reason;
	// the unregistered nullable columns here (both prices, both timestamps) pass
	// through as an untyped nil, which GORM already writes as NULL.
	//
	// revisiondiff emits that nil for every nullable numeric kind as of PSY-1960.
	// Rows written before that fix hold a 0 where the column was NULL and are
	// deliberately NOT backfilled; the reasoning is on the revisiondiff package
	// doc. Rolling one of those back therefore still writes 0, and what makes
	// that recoverable is that the show edit form can clear a price back to NULL.
	//
	// The URL rules are the reason a field can be refused at all. A rollback is a
	// WRITE of a stored value into a live column, and for any row whose old_value
	// predates the submit-time derivation that value never met a forward gate.
	// Deriving old_value at submit time closes the path for NEW rows only; these
	// checks are what stands between a planted value already in the table and the
	// column it names.
	//
	// SCOPE: a rollback can write ANY field in the entity's edit allowlist, so
	// all three checks are needed to reproduce what the forward paths enforce:
	// the shape rule for bandcamp_embed_url, the scheme + platform-host rules for
	// the other URL fields (SocialLinks and the ticket link render each as an
	// href under a trusted label), and the SSRF host guard for image_url.
	//
	// image_url is the one that needs a context, and it is the one that most
	// needs the check: urlguard's package doc explains that the fetch-time layer
	// re-checks IP LITERALS only, so a HOSTNAME resolving to 169.254.169.254 is
	// invisible to it and this write boundary is the only layer with a resolver.
	var applied []string
	var skipped []contracts.RollbackSkippedField
	seen := make(map[string]bool, len(changes))
	rollbackChanges := make([]adminm.FieldChange, 0, len(changes))
	for _, c := range changes {
		if seen[c.Field] {
			continue
		}
		seen[c.Field] = true
		if err := rollbackFieldError(ctx, updates, c.Field); err != nil {
			delete(updates, c.Field)
			skipped = append(skipped, contracts.RollbackSkippedField{
				Field:  c.Field,
				Reason: refusalReason(err),
			})
			continue
		}
		applied = append(applied, c.Field)
		rollbackChanges = append(rollbackChanges, adminm.FieldChange{
			Field:    c.Field,
			OldValue: c.NewValue,
			NewValue: c.OldValue,
		})
	}
	if len(applied) == 0 {
		return nil, fmt.Errorf("no field of this revision can be restored: %s", describeSkipped(skipped))
	}

	// The blank normalization mirrors ApprovePendingEdit's, and it is not an edge
	// case here: the edit drawer sends an empty old value whenever a contributor
	// sets a previously-empty field, so "set it, approve, roll back" is the
	// ORDINARY way to reach it. Without this the column lands blank-but-not-null,
	// which every `IS NULL` repair path skips forever while it renders nothing.
	//
	// AFTER the gate, never before: ValidateBandcampEmbedURL refuses a
	// whitespace-only value and passes only the empty string, so normalizing
	// first would turn "   " into nil and the gate would then skip it.
	normalizeBlankShapedURLs(updates)

	// A rollback that restores city/state must re-derive whatever the system
	// derives FROM that location, or the entity lands back in its old city still
	// carrying what was resolved for the city it was moved away from. Shared with
	// the approve path, and a no-op for entity types and writes it does not apply
	// to; see applyDerivedLocation. It reads the SURVIVING fields, so a refused
	// location field cannot pull a derived column with it.
	applyDerivedLocation(s.db, revision.EntityType, revision.EntityID, updates)

	// Apply update to the entity table
	tableName := revision.EntityType + "s" // artist -> artists, show -> shows, etc.
	updates["updated_at"] = time.Now()

	result := s.db.Table(tableName).Where("id = ?", revision.EntityID).Updates(updates)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to apply rollback: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, fmt.Errorf("entity not found: %s %d", revision.EntityType, revision.EntityID)
	}

	// Record the rollback as a new revision. The summary names the skipped fields
	// because the row it heads carries only the restored ones, and a reader of
	// history has no other way to learn the undo was partial.
	summary := fmt.Sprintf("Rollback of revision #%d", revisionID)
	if len(skipped) > 0 {
		summary = fmt.Sprintf("%s (skipped: %s)", summary, strings.Join(skippedFieldNames(skipped), ", "))
	}
	if err := s.RecordRevision(revision.EntityType, revision.EntityID, adminUserID, rollbackChanges, summary); err != nil {
		return nil, err
	}
	return &contracts.RollbackResult{AppliedFields: applied, SkippedFields: skipped}, nil
}

// rollbackFieldError reports why one restored value must not be written, or nil
// when every apply-side rule that reaches this field accepts it.
//
// It mutates updates for the field it accepts, exactly as the whole-map gates
// do: narrowNumericUpdate rewrites a JSONB float64 into the typed pointer the
// column needs, so "checked" and "written" stay the same value.
func rollbackFieldError(ctx context.Context, updates map[string]interface{}, field string) error {
	if err := narrowNumericUpdate(updates, field); err != nil {
		return err
	}
	if err := revalidateShapedURLField(updates, field); err != nil {
		return err
	}
	if err := validateRollbackURLField(updates, field); err != nil {
		return err
	}
	return revalidateFetchedURLField(ctx, updates, field)
}

// refusalReason renders a gate's error as the sentence an admin reads beside the
// field name, without the error-code prefix a PendingEditError carries for logs.
func refusalReason(err error) string {
	var editErr *apperrors.PendingEditError
	if errors.As(err, &editErr) {
		return editErr.Message
	}
	return err.Error()
}

// skippedFieldNames lists just the field names of a refusal set, for the places
// that name the fields without room for a reason each.
func skippedFieldNames(skipped []contracts.RollbackSkippedField) []string {
	names := make([]string, 0, len(skipped))
	for _, s := range skipped {
		names = append(names, s.Field)
	}
	return names
}

// describeSkipped renders field and reason together for a fully-refused
// rollback, which returns an error and so has no result object to carry them.
func describeSkipped(skipped []contracts.RollbackSkippedField) string {
	parts := make([]string, 0, len(skipped))
	for _, s := range skipped {
		parts = append(parts, fmt.Sprintf("%s (%s)", s.Field, s.Reason))
	}
	return strings.Join(parts, "; ")
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
