package admin

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"

	apperrors "psychic-homily-backend/internal/errors"
	adminm "psychic-homily-backend/internal/models/admin"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/shared/revisiondiff"
)

// OLD-VALUE CONTRACT
//
// Every pending edit this file writes carries a FieldChange.OldValue the server
// read off the entity, never one the submitter supplied. Rows already in the
// table are not rewritten, so the table as a whole holds both kinds and the
// apply-side gates still have to assume the worst of an old row.
//
// The reason it is load bearing is Rollback: an approved edit's old/new pair is
// copied verbatim into revisions.field_changes, and Rollback writes OldValue back
// into the column. A client-supplied OldValue is therefore a write of arbitrary
// contributor input into any allowlisted column, reached by pressing undo.
//
// The apply-side URL gates in pending_edit.go stay where they are and are NOT
// made redundant by this file. They are defence in depth over the same value:
// they run at the moment a value goes live, over rows this process did not
// necessarily write, including every row already in the table.
//
// A submitter's OldValue is still read, for one purpose: it is a CLAIM about
// what the field held when the form was loaded, and a claim that disagrees with
// the entity means the edit was composed against a value the entity no longer
// has. That is a conflict, answered with 409, not a value to store.
//
// The value is derived at SUBMIT time and CHECKED AGAIN at approve time. The
// unique index admits one pending edit per submitter per entity, so two
// submitters can queue edits against the same value; the recorded old_value of
// the second describes the entity as it was before the first was applied.
// ApprovePendingEdit therefore re-reads the entity under a row lock and refuses
// the whole edit when any recorded old_value no longer describes it
// (verifyOldValuesAtApprove). The stored value is never re-stamped, so what an
// approved row records is what the entity held at the moment it was applied.
//
// The exception is a WITHHELD field, where the recorded value is the withheld
// view and not the column: an unverified venue's address records "" while the
// column holds a street address. Rollback writes OldValue back verbatim
// (RevisionService.Rollback), so restoring such a revision blanks the column
// rather than restoring it. Nothing here narrows that; the gate only stops a
// row from being applied over a value nobody observed.

// entityModels pairs each pending-edit entity type with the GORM model whose
// columns a pending edit may name.
//
// Keyed by the same strings adminm.IsValidPendingEditEntityType accepts, and
// TestEntityModelsCoverPendingEditTypes fails when the two disagree, so an
// entity type gaining pending edits cannot reach this file with no model.
var entityModels = map[string]func() interface{}{
	adminm.PendingEditEntityArtist:   func() interface{} { return &catalogm.Artist{} },
	adminm.PendingEditEntityVenue:    func() interface{} { return &catalogm.Venue{} },
	adminm.PendingEditEntityFestival: func() interface{} { return &catalogm.Festival{} },
	adminm.PendingEditEntityRelease:  func() interface{} { return &catalogm.Release{} },
	adminm.PendingEditEntityLabel:    func() interface{} { return &catalogm.Label{} },
}

// modelSchemaCache is the per-process store gorm's schema parser memoizes into,
// so a model is walked once for the life of the process rather than once per
// submission. It is separate from the connection's own store only because that
// one is unexported.
//
// Entries are keyed by model TYPE, and the parse also depends on the namer, which
// is not part of that key. One naming strategy exists in this process, so the two
// stores cannot currently disagree; a second one would need this cache keyed by
// namer as well, or dropped in favour of the connection's.
var modelSchemaCache sync.Map

// withheldEditFieldsReporter is implemented by an entity model that withholds
// one of its own stored columns from the payload readers get.
//
// The derived old_value for such a field is the WITHHELD view, not the column.
// A pending edit is served back to its submitter and listed on their submissions
// page, so a derived value taken from the column would publish, to any
// authenticated user who asks to edit the field, exactly the value the entity
// payload refuses to serve them. An unverified venue is routinely somebody's
// house, and its address is withheld from every reader including admins.
//
// Deriving the withheld view rather than exempting the field from the conflict
// check is what makes the comparison say nothing about the column: the derived
// value is the same whatever the column holds, so a claim matches or not on its
// own merits and the stored value is unobservable either way.
//
// The model implements this rather than a list living here, because the model is
// where the gate that does the withholding lives and the two would otherwise
// drift apart silently: a field withheld by a new accessor but absent from a list
// over here would be published by this path the day the accessor was added.
type withheldEditFieldsReporter interface {
	WithheldEditFields() []string
}

// The venue address gate is the one that exists, and it is asserted rather than
// discovered: the reporter is looked up by type assertion, so a model that stops
// implementing it goes back to deriving from the column with nothing failing.
// A model that GAINS a privacy gate has to be added here in the same change, and
// TestWithheldFieldsAreEditable checks that whatever a reporter names is a field
// a submission can actually carry.
var _ withheldEditFieldsReporter = (*catalogm.Venue)(nil)

// deriveOldValues replaces every OldValue in changes with the value the entity
// currently holds, and reports a conflict when the submitter claimed something
// else.
//
// A claim is compared with sameFieldValue, which treats nil and "" as one
// state: the two shipping clients spell an empty field differently (the edit
// drawer sends null, the inline editors send ""), and the columns behind these
// fields have no reader that can tell NULL from "".
//
// The comparison is against the value the submitter could OBSERVE, which for a
// withheld field is not the column. See withheldEditFieldsReporter.
func deriveOldValues(db *gorm.DB, entityType string, entityID uint, changes []adminm.FieldChange) ([]adminm.FieldChange, error) {
	out, stale, err := resolveOldValues(db, entityType, entityID, changes)
	if err != nil {
		return nil, err
	}
	if len(stale) > 0 {
		return nil, apperrors.ErrPendingEditStaleValue(stale)
	}
	return out, nil
}

// verifyOldValuesAtApprove reports whether every OldValue already recorded on a
// pending edit still describes the entity, and refuses the approval when one
// does not.
//
// Same derivation and same comparison as the submit path, over the values that
// path stored, so a row cannot be applied over a value nobody observed.
//
// tx must be the transaction that goes on to write the entity. The read takes
// the row FOR UPDATE, and that lock is what makes the answer hold until the
// write lands: this reads a value it is about to overwrite, so an unlocked read
// is a check a concurrent approval can invalidate in between. Passing a handle
// that is not in a transaction leaves the check with no guarantee at all.
//
// Nothing is re-stamped on a mismatch and nothing is applied. The edit stays
// pending for the moderator to reject: a re-stamped previous value is one no
// reviewer ever saw, and Rollback would restore it.
//
// This is the transaction's FIRST statement, so the entity row is locked before
// the pending_entity_edits row the status flip takes. Every approval acquires
// them in that order.
func verifyOldValuesAtApprove(tx *gorm.DB, entityType string, entityID uint, changes []adminm.FieldChange) error {
	locked := tx.Clauses(clause.Locking{Strength: "UPDATE"})
	_, stale, err := resolveOldValues(locked, entityType, entityID, changes)
	if err != nil {
		// An entity that vanished between submission and approval is the approve
		// path's ENTITY_GONE (422, this edit can no longer be applied), not the
		// submit path's 404 for an entity that never existed.
		var editErr *apperrors.PendingEditError
		if errors.As(err, &editErr) && editErr.Code == apperrors.CodePendingEditEntityNotFound {
			return apperrors.ErrPendingEditEntityGone(entityType, entityID)
		}
		return err
	}
	if len(stale) > 0 {
		return apperrors.ErrPendingEditStaleValueAtApprove(stale)
	}
	return nil
}

// resolveOldValues derives the entity's current value for every field in changes
// and reports which recorded previous values no longer describe it.
//
// The two callers differ in the copy they attach to a mismatch and in the handle
// they pass, which is where the approve path's row lock lives. Sharing the body
// is what keeps "the value the submitter observed" and "the value the approval
// writes over" the same question; two implementations of it would be free to
// disagree about a withheld field, an empty string, or a number's encoding.
//
// The returned changes carry the derived OldValue whether or not any field is
// stale, so the submit path can store them and the approve path can ignore them.
func resolveOldValues(db *gorm.DB, entityType string, entityID uint, changes []adminm.FieldChange) ([]adminm.FieldChange, []apperrors.StaleFieldValue, error) {
	allowed, ok := adminm.AllowedEditFields(entityType)
	if !ok {
		return nil, nil, apperrors.ErrPendingEditInvalidEntityType(entityType)
	}
	columns, withheld, err := currentEntityColumns(db, entityType, entityID)
	if err != nil {
		return nil, nil, err
	}

	out := make([]adminm.FieldChange, len(changes))
	copy(out, changes)

	var stale []apperrors.StaleFieldValue
	for i := range out {
		field := out[i].Field
		// Fail closed, twice, because the alternative to knowing what a field
		// IS is storing the submitter's unverified claim about it. The
		// suggest-edit handler rejects a non-allowlisted field before this
		// runs; this function does not depend on that, because the value it
		// derives is the one Rollback later writes.
		if !allowed[field] {
			return nil, nil, apperrors.ErrPendingEditInvalidRequest(
				fmt.Sprintf("field '%s' is not editable on %s entities", field, entityType))
		}
		column, known := columns[field]
		if !known {
			return nil, nil, apperrors.ErrPendingEditInvalidRequest(
				fmt.Sprintf("field '%s' is not a column on %s entities", field, entityType))
		}
		// A withheld field derives from the UNSET value of its type, which is
		// what its reader is served, rather than from the column. See
		// withheldEditFieldsReporter: a pending edit is read back by its
		// submitter, so deriving from the column would publish the value the
		// entity payload withholds.
		if withheld[field] {
			column = reflect.Zero(column.Type())
		}
		value, err := revisiondiff.EmitValue(column)
		if err != nil {
			// Unreachable for an allowlisted field: TestAllowedEditFieldsAreDerivable
			// pins every one of them against this rule.
			return nil, nil, apperrors.ErrPendingEditInternal(fmt.Errorf("%s.%s: %w", entityType, field, err))
		}
		if !sameFieldValue(out[i].OldValue, value) {
			stale = append(stale, apperrors.StaleFieldValue{Field: field, Current: value})
		}
		out[i].OldValue = value
	}
	return out, stale, nil
}

// currentEntityColumns loads one entity and returns each of its columns paired
// with the struct field holding it, plus the columns whose value the entity
// withholds from readers.
//
// Every column is returned, not only the ones a caller asked about, so the
// caller can tell an unknown field from a NULL one. Values are left as
// reflect.Values and converted per field by the caller, since a submission names
// a handful of the thirty-odd columns a model carries.
//
// db carries whatever clauses the caller attached, which is how the approve path
// gets its FOR UPDATE. The one read below is the only statement it applies to.
func currentEntityColumns(db *gorm.DB, entityType string, entityID uint) (columns map[string]reflect.Value, withheld map[string]bool, err error) {
	newModel, ok := entityModels[entityType]
	if !ok {
		return nil, nil, apperrors.ErrPendingEditInvalidEntityType(entityType)
	}
	model := newModel()
	if err := db.First(model, entityID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, apperrors.ErrPendingEditEntityNotFound(entityType, entityID)
		}
		return nil, nil, apperrors.ErrPendingEditInternal(fmt.Errorf("failed to read %s %d: %w", entityType, entityID, err))
	}

	columns, err = modelColumns(db, model)
	if err != nil {
		return nil, nil, apperrors.ErrPendingEditInternal(err)
	}

	withheld = map[string]bool{}
	if reporter, ok := model.(withheldEditFieldsReporter); ok {
		for _, f := range reporter.WithheldEditFields() {
			withheld[f] = true
		}
	}
	return columns, withheld, nil
}

// modelColumns maps each of the model's SCALAR column names to the struct field
// holding it, asking gorm's own schema parser rather than re-deriving the
// answer.
//
// That matters because this map decides which COLUMN a submitted field NAME
// refers to, on the same connection gorm uses to write that row: a second set of
// tag and naming rules beside gorm's own is a drift the compiler cannot see, and
// the parser already resolves embedded structs (the social columns live in
// catalogm.Social), explicit `column:` tags, the connection's naming strategy,
// and the fields that are not columns at all.
//
// A field whose type EmitValue cannot convert is dropped, which is what keeps
// the relations and the JSONB columns out of a map whose keys a pending edit may
// name. The consequence worth knowing: a real column of an unsupported type goes
// missing just as quietly, and what makes that loud is
// TestAllowedEditFieldsAreDerivable, which reports any allowlisted field this
// map does not hold.
func modelColumns(db *gorm.DB, model interface{}) (map[string]reflect.Value, error) {
	parsed, err := schema.Parse(model, &modelSchemaCache, db.NamingStrategy)
	if err != nil {
		return nil, fmt.Errorf("failed to parse model schema: %w", err)
	}
	rv := reflect.ValueOf(model)
	columns := make(map[string]reflect.Value, len(parsed.FieldsByDBName))
	for name, field := range parsed.FieldsByDBName {
		if !revisiondiff.SupportedType(field.FieldType) {
			continue
		}
		columns[name] = field.ReflectValueOf(context.Background(), rv)
	}
	return columns, nil
}

// sameFieldValue reports whether a submitter's claim about a field describes the
// value the entity holds.
//
// Blank is one state, not two: nil and "" both mean the field was empty, which
// is what makes an ordinary "fill in an empty field" edit pass rather than
// conflicting with itself.
//
// Numbers compare numerically across encodings because a claim arrives from
// JSON as float64 while the derived value is an int.
func sameFieldValue(claim, current interface{}) bool {
	claimBlank, currentBlank := isBlankValue(claim), isBlankValue(current)
	if claimBlank || currentBlank {
		return claimBlank && currentBlank
	}
	if cn, ok := numericValue(claim); ok {
		vn, ok := numericValue(current)
		return ok && cn == vn
	}
	cs, claimIsString := claim.(string)
	vs, currentIsString := current.(string)
	return claimIsString && currentIsString && cs == vs
}

func isBlankValue(v interface{}) bool {
	if v == nil {
		return true
	}
	s, ok := v.(string)
	return ok && s == ""
}

// numericValue covers the two encodings a number reaches this comparison in: a
// claim decoded from JSON is a float64, and a value emitted from a column is an
// int.
func numericValue(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	default:
		return 0, false
	}
}
