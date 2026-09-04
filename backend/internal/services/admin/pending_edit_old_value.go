package admin

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	apperrors "psychic-homily-backend/internal/errors"
	adminm "psychic-homily-backend/internal/models/admin"
	catalogm "psychic-homily-backend/internal/models/catalog"
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
	current, withheld, err := currentEntityValues(db, entityType, entityID)
	if err != nil {
		return nil, err
	}

	out := make([]adminm.FieldChange, len(changes))
	copy(out, changes)

	var stale []string
	for i := range out {
		field := out[i].Field
		value, known := current[field]
		if !known {
			// Fail closed. The suggest-edit handler rejects a non-allowlisted
			// field before this runs, so reaching here means a caller outside
			// that handler named a column the entity does not have, and the
			// alternative is storing its unverified claim.
			return nil, apperrors.ErrPendingEditInvalidRequest(
				fmt.Sprintf("field '%s' is not a column on %s entities", field, entityType))
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

// currentEntityValues loads one entity and returns its column values in the
// shape a field change stores, plus the columns whose value the entity withholds
// from readers.
//
// Every column of the model is returned, not only the ones a caller asked
// about, so the caller can tell an unknown field from a NULL one.
func currentEntityValues(db *gorm.DB, entityType string, entityID uint) (values map[string]interface{}, withheld map[string]bool, err error) {
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

	columns := map[string]reflect.Value{}
	collectColumns(reflect.ValueOf(model).Elem(), db.NamingStrategy, columns)

	values = make(map[string]interface{}, len(columns))
	for column, field := range columns {
		v, convErr := emitValue(field)
		if convErr != nil {
			// Not reachable for any allowlisted column
			// (TestAllowedEditFieldsAreDerivable pins that), so this is the
			// unreachable-branch report rather than a user-facing condition.
			return nil, nil, apperrors.ErrPendingEditInternal(
				fmt.Errorf("%s.%s: %w", entityType, column, convErr))
		}
		values[column] = v
	}

	withheld = map[string]bool{}
	if reporter, ok := model.(withheldEditFieldsReporter); ok {
		for _, f := range reporter.WithheldEditFields() {
			withheld[f] = true
		}
	}
	return values, withheld, nil
}

// collectColumns maps every SCALAR column name the model declares to the struct
// field holding it, descending into GORM-embedded structs so the social columns
// (which live in catalogm.Social) are addressed by their own names.
//
// The column name comes from an explicit `column:` tag where there is one and
// from the connection's naming strategy otherwise, which is exactly how GORM
// resolves it when it writes the same row.
//
// A field whose type emitValue cannot convert is not a column here at all, which
// is what keeps the model's relations (the show and label slices, the preloaded
// user structs) out of a map whose keys a pending edit may name. The consequence
// worth knowing: a real column of an unsupported type would go missing just as
// quietly, and what makes that loud is TestAllowedEditFieldsAreDerivable, which
// reports any allowlisted field this map does not hold.
func collectColumns(v reflect.Value, namer schema.Namer, out map[string]reflect.Value) {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if !sf.IsExported() {
			continue
		}
		tag := sf.Tag.Get("gorm")
		if tag == "-" {
			continue
		}
		settings := schema.ParseTagSetting(tag, ";")
		if _, embedded := settings["EMBEDDED"]; embedded {
			collectColumns(v.Field(i), namer, out)
			continue
		}
		if !convertibleFieldType(sf.Type) {
			continue
		}
		column := settings["COLUMN"]
		if column == "" {
			column = namer.ColumnName("", sf.Name)
		}
		out[column] = v.Field(i)
	}
}

// convertibleFieldType reports whether emitValue can convert a field of this
// type. It is the type list emitValue switches on, asked ahead of time.
func convertibleFieldType(t reflect.Type) bool {
	if t == timeType {
		return true
	}
	switch t.Kind() {
	case reflect.String, reflect.Int:
		return true
	case reflect.Ptr:
		if t.Elem() == timeType {
			return true
		}
		switch t.Elem().Kind() {
		case reflect.String, reflect.Int, reflect.Float64:
			return true
		}
	}
	return false
}

var timeType = reflect.TypeOf(time.Time{})

// emitValue converts a model field to the JSON shape a field change stores.
//
// The rules are revisiondiff's, deliberately and not by coincidence: an admin's
// typed edit records its old value through revisiondiff.Compare, a contributor's
// edit records it through here, and Rollback feeds either one back into the same
// untyped update map. Two spellings of "the previous value" that disagreed on
// how to write an unset column would make an undo depend on which surface made
// the edit.
//
// So a nil *string emits "" while every other nullable kind emits nil: the text
// columns take the empty string and have no reader that can tell it from NULL,
// while a zero written to a nullable number or timestamp is a value nobody
// entered. revisiondiff.diffPtr carries the full argument.
func emitValue(v reflect.Value) (interface{}, error) {
	t := v.Type()
	if t == timeType {
		return v.Interface().(time.Time).Format(time.RFC3339), nil
	}
	switch t.Kind() {
	case reflect.String:
		return v.String(), nil
	case reflect.Int:
		return int(v.Int()), nil
	case reflect.Ptr:
		return emitPtrValue(v, t.Elem())
	default:
		return nil, fmt.Errorf("unsupported field kind %s", t.Kind())
	}
}

func emitPtrValue(v reflect.Value, elem reflect.Type) (interface{}, error) {
	if elem == timeType {
		if v.IsNil() {
			return nil, nil
		}
		return v.Elem().Interface().(time.Time).Format(time.RFC3339), nil
	}
	switch elem.Kind() {
	case reflect.String:
		if v.IsNil() {
			return "", nil
		}
		return v.Elem().String(), nil
	case reflect.Int:
		if v.IsNil() {
			return nil, nil
		}
		return int(v.Elem().Int()), nil
	case reflect.Float64:
		if v.IsNil() {
			return nil, nil
		}
		return v.Elem().Float(), nil
	default:
		return nil, fmt.Errorf("unsupported pointer element kind %s", elem.Kind())
	}
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
	if cs, ok := claim.(string); ok {
		vs, ok := current.(string)
		return ok && cs == vs
	}
	if cb, ok := claim.(bool); ok {
		vb, ok := current.(bool)
		return ok && cb == vb
	}
	return false
}

func isBlankValue(v interface{}) bool {
	if v == nil {
		return true
	}
	s, ok := v.(string)
	return ok && s == ""
}

func numericValue(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}
