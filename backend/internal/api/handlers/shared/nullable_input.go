package shared

import (
	"encoding/json"
	"reflect"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/services/contracts"
)

// NullableInput is the wire half of contracts.Nullable: a PATCH body field that
// tells an ABSENT key apart from an explicit `null` (PSY-1961).
//
// Declare it by VALUE, never as a pointer, and tag the field `omitempty`:
//
//	DoorPrice shared.NullableInput[float64] `json:"door_price,omitempty" required:"false" doc:"..."`
//
// By value is load-bearing. UnmarshalJSON only runs for a key that is actually
// in the body, so an absent field keeps the zero value and reads as unset,
// which is precisely the distinction a *T loses. A POINTER to one would
// reintroduce the ambiguity it exists to remove.
//
// Reaching the service is one field read — `req.Body.DoorPrice.Nullable` — not
// a conversion, because the embedded contract value IS the payload. The split
// exists because internal/services must not import huma (nothing under it does),
// so the schema and the decoding stay on this side of the boundary and only the
// INTENT crosses it.
type NullableInput[T any] struct {
	contracts.Nullable[T]
}

// NullableInputSet and NullableInputClear build a body field directly, for the
// callers that assemble a request in Go rather than decoding one off the wire:
// handler tests, and any future server-side client. The zero value is the third
// state, so there is no NullableInputUnset.
func NullableInputSet[T any](v T) NullableInput[T] {
	return NullableInput[T]{Nullable: contracts.NullableSet(v)}
}

func NullableInputClear[T any]() NullableInput[T] {
	return NullableInput[T]{Nullable: contracts.NullableClear[T]()}
}

// UnmarshalJSON records that the field was mentioned, then reads `null` as a
// clear and anything else as a value.
//
// Not called at all for an absent key, which is how absence is detected. A
// malformed value is returned as an error rather than swallowed as a clear: the
// three states are absent, cleared and set, and "unparseable" is none of them.
func (n *NullableInput[T]) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		n.Nullable = contracts.NullableClear[T]()
		return nil
	}
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	n.Nullable = contracts.NullableSet(v)
	return nil
}

// MarshalJSON mirrors UnmarshalJSON so the type round-trips through the schema
// Schema advertises: a set field writes its value, and a cleared one writes
// `null`. Without it `encoding/json` would reach for the embedded contract's
// unexported fields, find none exported, and emit `{}` against a
// number-or-null schema — a body that validates nowhere and reads as an empty
// object to every client.
//
// ABSENCE DOES NOT SURVIVE THE ROUND TRIP, and cannot: `omitempty` never omits
// a struct, so an unmentioned field marshals as `null` and would come back as
// an explicit clear. That is harmless for the request bodies this type exists
// for, since a body is decoded and read, not re-encoded. A RESPONSE field that
// needs "this column has no value" wants a plain *T; reach for this type only
// where the three-state distinction is the point.
func (n NullableInput[T]) MarshalJSON() ([]byte, error) {
	if v, ok := n.Value(); ok {
		return json.Marshal(v)
	}
	return []byte("null"), nil
}

// Schema publishes T's own schema, marked nullable, so the OpenAPI document
// describes `door_price` as a number-or-null rather than as the two-field
// struct huma would otherwise generate from the Go type. It is also what makes
// huma's request VALIDATION accept a bare number: validation runs against this
// schema before UnmarshalJSON is ever reached, so without it every real value
// would be rejected as "expected object".
//
// Generic over T rather than one type per column: the same declaration serves
// float64 (the two prices) and time.Time (the two show times), and T's format
// (`date-time` for a timestamp) comes through untouched because huma derives it
// rather than this method restating it.
func (n NullableInput[T]) Schema(r huma.Registry) *huma.Schema {
	s := huma.SchemaFromType(r, reflect.TypeOf((*T)(nil)).Elem())
	s.Nullable = true
	return s
}
