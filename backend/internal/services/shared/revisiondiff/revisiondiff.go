// Package revisiondiff computes field-level diffs between two structs for
// revision history (PSY-563/759). It replaces the six near-identical
// compute*Changes functions that lived in handlers/catalog with a single
// reflection-driven comparator.
//
// Each entity declares an ordered list of Fields (output name + struct path).
// Compare walks that list, reads the named field from the before/after structs
// via reflection, and emits an adminm.FieldChange whenever the value differs.
// The emitted value type and comparison semantics are inferred from the
// field's static Go type so output stays byte-identical with the hand-written
// predecessors:
//
//   - string            → compare with ==, emit string
//   - *string           → deref or "" (matches the old ptrToStr helper)
//   - *float64          → deref or 0, emit float64
//   - int               → compare with ==, emit int
//   - *int              → deref or 0, emit int (both-nil counts as equal)
//   - time.Time         → compare with Equal, emit RFC3339 string
//
// The per-entity field lists — not the contributor allowlist — are the source
// of truth for which fields appear in revision history. They intentionally
// diverge from models/admin allowlists (e.g. festival tracks admin-only
// start_date/status; show has no allowlist entry at all), so driving Compare
// off the allowlist would change the recorded diffs. Where a list happens to
// match an allowlist that is coincidence, not coupling.
//
// Field paths are validated against their struct once at package init via
// ValidateAll, so a renamed or mistyped field fails loudly at startup (and in
// tests) rather than silently dropping from every future revision row.
package revisiondiff

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	adminm "psychic-homily-backend/internal/models/admin"
)

// Field maps an output field name (the snake_case name stored in
// revisions.field_changes) to a dot-separated path into the struct being
// diffed. Nested structs are addressed with dots, e.g. "Social.Instagram".
type Field struct {
	// Name is the value written to FieldChange.Field — the column-style name
	// the frontend renders and the allowlist keys on.
	Name string
	// Path is the dot-separated Go field path resolved by reflection,
	// e.g. "EventDate" or "Social.Instagram".
	Path string
}

var timeType = reflect.TypeOf(time.Time{})

// Compare returns the field-level diffs between before and after for the given
// ordered field list. before and after must be the same struct type (or
// pointer to it). Fields are emitted in list order, only when they differ.
//
// Compare panics if a path does not resolve to a supported field type — that
// is a programming error caught by ValidateAll at init and in tests, never a
// runtime data condition.
func Compare(before, after interface{}, fields []Field) []adminm.FieldChange {
	bv := derefStruct(before)
	av := derefStruct(after)

	var changes []adminm.FieldChange
	for _, f := range fields {
		bf := resolvePath(bv, f.Path)
		af := resolvePath(av, f.Path)

		oldVal, newVal, changed := diffValue(bf, af)
		if changed {
			changes = append(changes, adminm.FieldChange{
				Field:    f.Name,
				OldValue: oldVal,
				NewValue: newVal,
			})
		}
	}
	return changes
}

// derefStruct returns the struct reflect.Value behind an interface that may be
// a struct or a pointer to one.
func derefStruct(v interface{}) reflect.Value {
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	return rv
}

// resolvePath walks a dot-separated field path from a struct value, descending
// through nested struct values (not pointers — the response structs nest
// SocialResponse by value).
func resolvePath(structVal reflect.Value, path string) reflect.Value {
	cur := structVal
	for _, part := range strings.Split(path, ".") {
		cur = cur.FieldByName(part)
	}
	return cur
}

// diffValue compares two reflect.Values of the same supported type and returns
// the old/new emit values plus whether they differ. The emit types match the
// original compute*Changes helpers exactly so JSONB output is byte-identical.
func diffValue(before, after reflect.Value) (oldVal, newVal interface{}, changed bool) {
	t := before.Type()

	// time.Time: compare with Equal, emit RFC3339 string (matches the old
	// `EventDate.Format(time.RFC3339)` calls).
	if t == timeType {
		bt := before.Interface().(time.Time)
		at := after.Interface().(time.Time)
		return bt.Format(time.RFC3339), at.Format(time.RFC3339), !bt.Equal(at)
	}

	switch t.Kind() {
	case reflect.String:
		b := before.String()
		a := after.String()
		return b, a, b != a

	case reflect.Int:
		b := int(before.Int())
		a := int(after.Int())
		return b, a, b != a

	case reflect.Ptr:
		return diffPtr(before, after, t.Elem())

	default:
		panic(fmt.Sprintf("revisiondiff: unsupported field kind %s", t.Kind()))
	}
}

// diffPtr handles the pointer field types the compute*Changes helpers used:
// *string (deref or ""), *float64 (deref or 0), *int (deref or 0). A nil
// pointer is treated as the zero value, matching ptrToStr / shared.Deref /
// intPtrVal so a nil↔value transition is a change and nil↔nil is not.
func diffPtr(before, after reflect.Value, elem reflect.Type) (oldVal, newVal interface{}, changed bool) {
	switch elem.Kind() {
	case reflect.String:
		b := derefString(before)
		a := derefString(after)
		return b, a, b != a

	case reflect.Float64:
		b := derefFloat64(before)
		a := derefFloat64(after)
		return b, a, b != a

	case reflect.Int:
		b := derefInt(before)
		a := derefInt(after)
		return b, a, b != a

	default:
		panic(fmt.Sprintf("revisiondiff: unsupported pointer element kind %s", elem.Kind()))
	}
}

func derefString(p reflect.Value) string {
	if p.IsNil() {
		return ""
	}
	return p.Elem().String()
}

func derefFloat64(p reflect.Value) float64 {
	if p.IsNil() {
		return 0
	}
	return p.Elem().Float()
}

func derefInt(p reflect.Value) int {
	if p.IsNil() {
		return 0
	}
	return int(p.Elem().Int())
}
