package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"psychic-homily-backend/db"
	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
	adminm "psychic-homily-backend/internal/models/admin"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
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
// A field the entity NO LONGER HOLDS is skipped by the same report. A rollback
// is an undo of one revision, and undoing a revision whose value something else
// has already replaced is not an undo: it discards the later change and records
// a previous value the entity never held, which the next rollback then restores.
// The check is the entity's own value, read inside this call's transaction under
// FOR UPDATE on the row it is about to write, through the derivation the approve
// path uses (observeCurrentValues). An unlocked read would be a check the write
// it guards can invalidate in between.
//
// The recorded rollback revision's old_value is that OBSERVED value, not the
// revision's recorded new_value. The two agree for every field that passes the
// check, and where a stored claim and the column disagree the column is the one
// that was true.
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

	// One entry per field, the last occurrence winning, which is what an update
	// map built from the slice would hold anyway. Collapsing here rather than
	// letting the map do it keeps the value WRITTEN and the change RECORDED as
	// the same entry: a row carrying a field twice would otherwise be gated and
	// recorded from one occurrence while the column took the other.
	fieldOrder := make([]string, 0, len(changes))
	byField := make(map[string]adminm.FieldChange, len(changes))
	for _, c := range changes {
		if _, seen := byField[c.Field]; !seen {
			fieldOrder = append(fieldOrder, c.Field)
		}
		byField[c.Field] = c
	}
	if len(fieldOrder) == 0 {
		return nil, fmt.Errorf("revision %d records no field changes", revisionID)
	}

	// Build update map from old values (reversing the change)
	updates := make(map[string]interface{}, len(fieldOrder))
	for _, field := range fieldOrder {
		updates[field] = byField[field].OldValue
	}

	// Judge each field on its own, then drop the refused ones from the write.
	// What the gates are and why each one runs is on rollbackFieldError.
	//
	// These gates run OUTSIDE the transaction below, and must: revalidateFetchedURLField
	// resolves DNS, so holding a row lock across it would pin the row for the
	// length of a network call. The one check that must run INSIDE the
	// transaction is the entity read, because it reads a value the same
	// transaction overwrites.
	//
	// Refusals accumulate by field name rather than into the reported list
	// directly, so the two rounds of judging can be reported in one pass, in the
	// order the revision recorded the fields.
	refusals := make(map[string]string, len(fieldOrder))
	numericBounds := contracts.NumericEditFieldBounds()
	for _, field := range fieldOrder {
		if err := rollbackFieldError(ctx, updates, field, numericBounds); err != nil {
			delete(updates, field)
			refusals[field] = refusalReason(err)
		}
	}
	if len(refusals) == len(fieldOrder) {
		return nil, errNothingRestorable(fieldOrder, refusals)
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

	tableName := revision.EntityType + "s" // artist -> artists, show -> shows, etc.

	var result *contracts.RollbackResult
	var rollbackChanges []adminm.FieldChange
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// FIRST statement of the transaction, so the entity row is locked before
		// anything else this call touches, and the lock is held until the write
		// below commits.
		observed, err := observeRollbackValues(tx, revision.EntityType, revision.EntityID, fieldOrder, byField, refusals)
		if err != nil {
			return err
		}
		for field, reason := range observed.refused {
			delete(updates, field)
			refusals[field] = reason
		}
		if len(refusals) == len(fieldOrder) {
			return errNothingRestorable(fieldOrder, refusals)
		}

		var applied []string
		// Never nil: an absent key and an empty list read the same to a renderer
		// that checks length, but not to one that checks presence, and this list
		// is the only signal that a rollback was partial.
		skipped := []contracts.RollbackSkippedField{}
		rollbackChanges = make([]adminm.FieldChange, 0, len(fieldOrder))
		for _, field := range fieldOrder {
			if reason, refused := refusals[field]; refused {
				skipped = append(skipped, contracts.RollbackSkippedField{Field: field, Reason: reason})
				continue
			}
			applied = append(applied, field)
			rollbackChanges = append(rollbackChanges, adminm.FieldChange{
				Field:    field,
				OldValue: observed.current[field],
				NewValue: byField[field].OldValue,
			})
		}
		result = &contracts.RollbackResult{AppliedFields: applied, SkippedFields: skipped}

		// A rollback that restores city/state must re-derive whatever the system
		// derives FROM that location, or the entity lands back in its old city
		// still carrying what was resolved for the city it was moved away from.
		// Shared with the approve path, and a no-op for entity types and writes
		// it does not apply to; see applyDerivedLocation. It reads the SURVIVING
		// fields, so a refused location field cannot pull a derived column with
		// it, and it reads through tx because this caller builds the map while
		// holding the row lock — the case applyDerivedVenueLocation's doc names.
		applyDerivedLocation(tx, revision.EntityType, revision.EntityID, updates)

		updates["updated_at"] = time.Now()

		write := tx.Table(tableName).Where("id = ?", revision.EntityID).Updates(updates)
		if write.Error != nil {
			// A column CHECK that columnBoundRollbackError does not cover: a field
			// outside the numeric registry, or a constraint added without an entry.
			// The driver names the constraint, which says nothing an admin can act
			// on, so it goes to the log while the caller gets the reason.
			//
			// Reaching here fails the WHOLE rollback rather than skipping one field,
			// which is what the per-field gate exists to avoid. Treat it as a missing
			// entry in columnBoundedRollbackFields, not as the intended path.
			if shared.IsCheckConstraintViolation(write.Error) {
				logger.FromContext(ctx).Error("revision_rollback_check_constraint",
					"entity_type", revision.EntityType,
					"entity_id", revision.EntityID,
					"revision_id", revision.ID,
					"error", write.Error.Error(),
				)
				return fmt.Errorf(
					"cannot roll back: this revision restores a value a %s column no longer accepts",
					revision.EntityType)
			}
			return fmt.Errorf("failed to apply rollback: %w", write.Error)
		}
		if write.RowsAffected == 0 {
			// The locked read above already found the row, so the only way here
			// is a concurrent delete that beat the lock's acquisition.
			return fmt.Errorf("entity not found: %s %d", revision.EntityType, revision.EntityID)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Record the rollback as a new revision. The summary names the skipped fields
	// because the row it heads carries only the restored ones, and a reader of
	// history has no other way to learn the undo was partial.
	skipped := result.SkippedFields
	summary := fmt.Sprintf("Rollback of revision #%d", revisionID)
	if len(skipped) > 0 {
		summary = fmt.Sprintf("%s (skipped: %s)", summary, strings.Join(result.SkippedFieldNames(), ", "))
	}
	// Fire-and-forget, matching the approve path and for the same reason: the
	// entity write above has already committed, so returning an error here would
	// report a rollback that did not happen as failed. The history row is lost
	// and logged; the undo is not undone by saying so.
	if err := s.RecordRevision(revision.EntityType, revision.EntityID, adminUserID, rollbackChanges, summary); err != nil {
		logger.Default().Error("revision_rollback_record_failed",
			"revision_id", revisionID,
			"entity_type", revision.EntityType,
			"entity_id", revision.EntityID,
			"error", err.Error(),
		)
	}
	return result, nil
}

// rollbackObservation is what the entity itself says about the fields a rollback
// is about to overwrite: the value each one currently holds, and the ones that
// must not be written because that value is not what the revision recorded.
type rollbackObservation struct {
	current map[string]interface{}
	refused map[string]string
}

// changedSinceReason is the sentence an admin reads beside a field the entity no
// longer holds the revision's value for.
//
// It says what was observed and not what to do about it, because what to do
// differs by cause: a later edit, a merge, a direct admin write and a field the
// entity withholds all land here.
const changedSinceReason = "this field changed after the revision was recorded, so restoring it would discard that change"

// observeRollbackValues reads the entity under a row lock and reports, for every
// field a rollback still intends to write, the value it currently holds and
// whether that value is the one the revision recorded writing.
//
// The claim per field is the revision's NEW value. A rollback undoes one
// revision, so it is only an undo while the entity still holds what that
// revision wrote; over anything else it is a silent discard of whatever came
// after, recorded as an old_value nobody observed.
//
// tx must be the transaction that goes on to write the entity: the read takes
// the row FOR UPDATE, and that lock is what makes the answer hold until the
// write lands.
//
// alreadyRefused names the fields the apply-side gates have already dropped.
// They are left out of the read because the observation would change nothing
// about them, and asking would report a second reason for a field that already
// has one.
func observeRollbackValues(
	tx *gorm.DB,
	entityType string,
	entityID uint,
	fieldOrder []string,
	byField map[string]adminm.FieldChange,
	alreadyRefused map[string]string,
) (rollbackObservation, error) {
	claims := make([]adminm.FieldChange, 0, len(fieldOrder))
	for _, field := range fieldOrder {
		if _, refused := alreadyRefused[field]; refused {
			continue
		}
		claims = append(claims, adminm.FieldChange{Field: field, OldValue: byField[field].NewValue})
	}

	locked := tx.Clauses(clause.Locking{Strength: "UPDATE"})
	resolved, stale, err := observeCurrentValues(locked, entityType, entityID, claims)
	if err != nil {
		// An entity that is gone is reported the way the write below reports it,
		// so moving the read earlier did not change what a caller sees.
		var editErr *apperrors.PendingEditError
		if errors.As(err, &editErr) && editErr.Code == apperrors.CodePendingEditEntityNotFound {
			return rollbackObservation{}, fmt.Errorf("entity not found: %s %d", entityType, entityID)
		}
		return rollbackObservation{}, fmt.Errorf("cannot roll back: %s", refusalReason(err))
	}

	observation := rollbackObservation{
		current: make(map[string]interface{}, len(resolved)),
		refused: make(map[string]string, len(stale)),
	}
	for _, c := range resolved {
		observation.current[c.Field] = c.OldValue
	}
	for _, s := range stale {
		observation.refused[s.Field] = changedSinceReason
	}
	return observation, nil
}

// errNothingRestorable is the refusal for a rollback with no field left to
// write. It names every field and its reason because there is no result object
// to carry them, and it walks fieldOrder so the message reads in the order the
// revision recorded.
func errNothingRestorable(fieldOrder []string, refusals map[string]string) error {
	skipped := make([]contracts.RollbackSkippedField, 0, len(fieldOrder))
	for _, field := range fieldOrder {
		if reason, refused := refusals[field]; refused {
			skipped = append(skipped, contracts.RollbackSkippedField{Field: field, Reason: reason})
		}
	}
	return fmt.Errorf("no field of this revision can be restored: %s", describeSkipped(skipped))
}

// rollbackFieldError reports why one restored value must not be written, or nil
// when every apply-side rule that reaches this field accepts it.
//
// It mutates updates for the field it accepts, exactly as the whole-map gates
// do: narrowNumericUpdate rewrites a JSONB float64 into the typed pointer the
// column needs, so "checked" and "written" stay the same value. That narrowing
// is not cosmetic — the driver takes a float64 for an integer column and
// truncates rather than failing, so an ungated rollback restores a DIFFERENT
// value than the one being undone.
//
// Narrowing does not range check, with one exception. An API-only bound is not
// re-applied here: history holds values that predate one, and restoring them is
// the point. A bound the COLUMN carries is different, and the difference is
// columnBoundRollbackError.
//
// The URL rules are the reason a field can be refused at all. A rollback is a
// WRITE of a stored value into a live column, and on any row whose old_value
// predates the submit-time derivation that value never met a forward gate. All
// three are needed to reproduce what the forward paths enforce: the shape rule
// for bandcamp_embed_url, the scheme and platform-host rules for the other URL
// fields (SocialLinks and the ticket link render each as an href under a trusted
// label), and the SSRF host guard for image_url. That last one is the only gate
// with a resolver, and urlguard's package doc explains why it has to be: the
// fetch-time layer re-checks IP LITERALS only, so a HOSTNAME resolving to
// 169.254.169.254 is invisible to it.
func rollbackFieldError(ctx context.Context, updates map[string]interface{}, field string, numericBounds map[string]contracts.NumericEditBounds) error {
	if err := narrowNumericUpdate(updates, field, numericBounds); err != nil {
		return err
	}
	if err := integerColumnRollbackError(updates, field, numericBounds); err != nil {
		return err
	}
	if err := columnBoundRollbackError(updates, field, numericBounds); err != nil {
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

// integerColumnRollbackError refuses a restored value the column could not
// physically store.
//
// Every field in contracts.NumericEditFieldBounds is backed by a Postgres
// INTEGER: venues.capacity, labels.founded_year, releases.release_year. Go's int
// is 64-bit here and utils.WholeNumber accepts anything an int64 holds, so a
// stored old value of 9999999999 narrows cleanly and only dies at the column,
// with SQLSTATE 22003 rather than a CHECK violation.
//
// That matters because the year fields are deliberately exempt from a range
// check: history predates their bound and restoring it is the point. Without a
// width gate, one such value is not a skipped field, it is a failed UPDATE that
// takes every honest field in the revision with it, and its driver message
// reaches the caller.
//
// A width check is not a range policy. It says only what the column can hold, so
// it applies to every registered field, including the ones no CHECK constrains.
func integerColumnRollbackError(updates map[string]interface{}, field string, registry map[string]contracts.NumericEditBounds) error {
	if _, registered := registry[field]; !registered {
		return nil
	}
	narrowed, isPtr := updates[field].(*int)
	if !isPtr || narrowed == nil {
		return nil
	}
	if *narrowed < math.MinInt32 || *narrowed > math.MaxInt32 {
		return apperrors.ErrPendingEditInvalidRequest(fmt.Sprintf(
			"%s is outside the range the column can store", field))
	}
	return nil
}

// columnBoundedRollbackFields are the fields whose COLUMN carries a CHECK
// constraint, so the database refuses an out-of-range value whichever path
// writes it.
//
// Deliberately NOT the whole of contracts.NumericEditFieldBounds. The year
// fields are bounded by the API alone, and a rollback restoring a year from
// before that bound still succeeds, which is the exemption rollbackFieldError
// describes. capacity is here because venues_capacity_range refuses it at the
// column: an unchecked rollback of an out-of-range capacity does not restore an
// old value, it fails the whole UPDATE and takes every honest field recorded
// beside it down with it. Checking the field means it is SKIPPED and reported
// like any other refused field, and its siblings still restore.
//
// The ranges themselves are not repeated here. They come from the same
// contracts registry the column mirrors, so this map only says WHICH fields the
// database also polices.
var columnBoundedRollbackFields = map[string]struct{}{
	"capacity": {}, // venues_capacity_range
}

// columnBoundRollbackError refuses a restored value that its own column would
// refuse, for the fields a CHECK constraint covers.
//
// Runs AFTER narrowNumericUpdate, and reads the narrowed *int it produced: the
// raw JSONB value is a float64, and comparing that against int bounds is the
// conversion this exists to avoid. A nil pointer is the clear gesture, which
// every one of these columns accepts as NULL.
func columnBoundRollbackError(updates map[string]interface{}, field string, registry map[string]contracts.NumericEditBounds) error {
	if _, columnBounded := columnBoundedRollbackFields[field]; !columnBounded {
		return nil
	}
	bounds, registered := registry[field]
	if !registered {
		return nil
	}
	narrowed, isPtr := updates[field].(*int)
	if !isPtr || narrowed == nil {
		return nil
	}
	if *narrowed < bounds.Min || *narrowed > bounds.Max {
		return apperrors.ErrPendingEditInvalidRequest(fmt.Sprintf(
			"%s must be between %d and %d", field, bounds.Min, bounds.Max))
	}
	return nil
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
