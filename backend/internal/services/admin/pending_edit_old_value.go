package admin

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"sync"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	apperrors "psychic-homily-backend/internal/errors"
	adminm "psychic-homily-backend/internal/models/admin"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/shared/revisiondiff"
)

// OLD-VALUE CONTRACT
//
// FieldChange.OldValue on a stored pending edit is a value this server read off
// the entity, never a value the submitter supplied. That is the whole point of
// this file, and the reason it is load bearing is Rollback: an approved edit's
// old/new pair is copied verbatim into revisions.field_changes, and Rollback
// writes OldValue back into the column. A client-supplied OldValue is therefore
// a write of arbitrary contributor input into any allowlisted column, reached by
// pressing undo.
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
// submission. Separate from the connection's own store only because that one is
// unexported; the parse is pure, so two caches cannot disagree.
var modelSchemaCache sync.Map

// withheldEditFieldsReporter is implemented by an entity model that withholds
// one of its own stored columns from the payload readers get.
//
// A field the submitter cannot read cannot be claimed, so a claim about it is
// not evidence of anything and must not raise a conflict. The model implements
// this rather than a list living here, because the model is where the gate that
// does the withholding lives and the two would otherwise drift apart silently:
// a field withheld by a new accessor but absent from a list over here reads as
// an ordinary conflict and blocks the edit forever.
type withheldEditFieldsReporter interface {
	WithheldEditFields() []string
}

// deriveOldValues replaces every OldValue in changes with the value the entity
// currently holds, and reports a conflict when the submitter claimed something
// else.
//
// Returns a new slice; the input is left alone because callers marshal the
// original for logging.
//
// A claim is compared with sameFieldValue, which treats nil and "" as one
// state: the two shipping clients spell an empty field differently (the edit
// drawer sends null, the inline editors send ""), and the columns behind these
// fields have no reader that can tell NULL from "".
func deriveOldValues(db *gorm.DB, entityType string, entityID uint, changes []adminm.FieldChange) ([]adminm.FieldChange, error) {
	allowed, ok := adminm.AllowedEditFields(entityType)
	if !ok {
		return nil, apperrors.ErrPendingEditInvalidEntityType(entityType)
	}
	columns, withheld, err := currentEntityColumns(db, entityType, entityID)
	if err != nil {
		return nil, err
	}

	out := make([]adminm.FieldChange, len(changes))
	copy(out, changes)

	var stale []string
	for i := range out {
		field := out[i].Field
		// Fail closed, twice, because the alternative to knowing what a field
		// IS is storing the submitter's unverified claim about it. The
		// suggest-edit handler rejects a non-allowlisted field before this
		// runs; this function does not depend on that, because the value it
		// derives is the one Rollback later writes.
		if !allowed[field] {
			return nil, apperrors.ErrPendingEditInvalidRequest(
				fmt.Sprintf("field '%s' is not editable on %s entities", field, entityType))
		}
		column, known := columns[field]
		if !known {
			return nil, apperrors.ErrPendingEditInvalidRequest(
				fmt.Sprintf("field '%s' is not a column on %s entities", field, entityType))
		}
		value, err := revisiondiff.EmitValue(column)
		if err != nil {
			// Unreachable for an allowlisted field: TestAllowedEditFieldsAreDerivable
			// pins every one of them against this rule.
			return nil, apperrors.ErrPendingEditInternal(fmt.Errorf("%s.%s: %w", entityType, field, err))
		}
		if !withheld[field] && !sameFieldValue(out[i].OldValue, value) {
			stale = append(stale, field)
		}
		out[i].OldValue = value
	}
	if len(stale) > 0 {
		sort.Strings(stale)
		return nil, apperrors.ErrPendingEditStaleValue(stale)
	}
	return out, nil
}

// currentEntityColumns loads one entity and returns each of its columns paired
// with the struct field holding it, plus the columns whose value the entity
// withholds from readers.
//
// Every column is returned, not only the ones a caller asked about, so the
// caller can tell an unknown field from a NULL one. Values are left as
// reflect.Values and converted per field by the caller, since a submission names
// a handful of the thirty-odd columns a model carries.
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
