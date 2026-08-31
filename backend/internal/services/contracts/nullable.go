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

// There is no NullableUnset constructor: the ZERO VALUE is the unset state, so
// a field nobody mentioned needs no builder at all. Only the two states that
// carry a request have one.

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
// mentioned, with no value.
//
// The third state named directly, so a test or a branch can say which one it
// means instead of spelling it as `Present() && !ok`. Translators do not need
// it — they use ValueOrNil below, which folds the same distinction into the
// value they were going to write anyway.
func (n Nullable[T]) Clears() bool { return n.present && n.value == nil }

// ValueOrNil is the POINTER this field asks to write: the value when one was
// given, nil when the field is an explicit clear or was never mentioned.
//
// The accessor a translator wants, because the value and the clear become ONE
// expression instead of a branch that could pick the wrong one. Callers still
// have to ask Present first: "absent" and "cleared" both answer nil here and
// mean opposite things, which is the whole distinction this type exists to keep.
//
// A fresh pointer, never the stored one, so no caller can reach back in and
// mutate a Nullable that was handed to it by value.
func (n Nullable[T]) ValueOrNil() *T {
	if n.value == nil {
		return nil
	}
	v := *n.value
	return &v
}
