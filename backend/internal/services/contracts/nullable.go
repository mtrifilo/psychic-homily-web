package contracts

// Nullable carries the three states a PATCH field can be in, which a plain
// pointer cannot tell apart: ABSENT (leave the stored value alone), CLEARED
// (write SQL NULL), and SET (write this value).
//
// A *T collapses the first two. `encoding/json` leaves a pointer nil both when
// the key is missing and when it is explicitly `null`, so every nullable column
// reachable through an update request was write-only: a value could be replaced
// but never retracted (PSY-1961). For shows.door_price that is the routine
// correction, not an edge case — the door price is the OPT-IN half of the
// advance/door split, so "this show does not actually have one" is something a
// curator says often.
//
// DELIBERATELY HUMA-FREE, and that is why the wire decoding lives one layer up
// in api/handlers/shared rather than here: nothing under internal/services
// imports the HTTP framework, and this package is read by services that have no
// business knowing a request shape. This type is the INTENT; how the intent is
// spelled on the wire is the boundary's problem.
//
// The fields are unexported so the invalid fourth state — "absent, but with a
// value" — cannot be written. Build one with the three constructors, read it
// with Present and Value.
type Nullable[T any] struct {
	present bool
	value   *T
}

// NullableUnset is the zero value spelled out: the field was not mentioned, so
// the stored value stands. Returned for completeness at call sites that choose
// between the three states; a zero Nullable means the same thing.
func NullableUnset[T any]() Nullable[T] {
	return Nullable[T]{}
}

// NullableClear is an explicit request to write SQL NULL.
func NullableClear[T any]() Nullable[T] {
	return Nullable[T]{present: true}
}

// NullableSet is an explicit request to write v.
func NullableSet[T any](v T) Nullable[T] {
	return Nullable[T]{present: true, value: &v}
}

// Value returns the requested value and whether there was one. The bool is
// false both for an absent field and for an explicit clear, so a caller that
// only wants "is there a number here" can read it alone; a caller deciding
// between unchanged and NULL must consult Present too.
//
// A method rather than a bare pointer accessor so no caller can retain and
// mutate the stored value.
func (n Nullable[T]) Value() (T, bool) {
	if n.value == nil {
		var zero T
		return zero, false
	}
	return *n.value, true
}

// Present reports whether the request mentioned this field at all. False means
// leave the column as it is; it says nothing about whether a value was given.
func (n Nullable[T]) Present() bool { return n.present }

// Clears reports whether this field is an explicit request for SQL NULL:
// mentioned, with no value. The one question showUpdatesToMap-style translators
// ask that neither Present nor Value answers on its own.
func (n Nullable[T]) Clears() bool { return n.present && n.value == nil }
