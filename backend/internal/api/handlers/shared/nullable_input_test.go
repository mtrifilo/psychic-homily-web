package shared

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type nullableBody struct {
	Title     *string                  `json:"title,omitempty" required:"false"`
	DoorPrice NullableInput[float64]   `json:"door_price,omitempty" required:"false"`
	DoorsAt   NullableInput[time.Time] `json:"doors_at,omitempty" required:"false"`
}

// The three states a PATCH field can arrive in, read off the decoded struct
// rather than off the raw JSON, because the whole point is that the STRUCT can
// still tell them apart afterwards.
func TestNullableInput_UnmarshalThreeStates(t *testing.T) {
	t.Run("absent leaves the field unset", func(t *testing.T) {
		var body nullableBody
		require.NoError(t, json.Unmarshal([]byte(`{"title":"Boris"}`), &body))
		assert.False(t, body.DoorPrice.Present(), "an absent key must not read as mentioned")
		_, ok := body.DoorPrice.Value()
		assert.False(t, ok)
	})

	t.Run("explicit null is a clear", func(t *testing.T) {
		var body nullableBody
		require.NoError(t, json.Unmarshal([]byte(`{"door_price":null}`), &body))
		assert.True(t, body.DoorPrice.Present())
		assert.True(t, body.DoorPrice.Clears(), "null must read as a request for SQL NULL")
		_, ok := body.DoorPrice.Value()
		assert.False(t, ok)
	})

	t.Run("a value is a value", func(t *testing.T) {
		var body nullableBody
		require.NoError(t, json.Unmarshal([]byte(`{"door_price":40}`), &body))
		assert.True(t, body.DoorPrice.Present())
		assert.False(t, body.DoorPrice.Clears())
		v, ok := body.DoorPrice.Value()
		require.True(t, ok)
		assert.Equal(t, 40.0, v)
	})

	// Zero is a real price the site renders as "Free", so it must survive as a
	// SET value and never collapse into the clear or the absent state.
	t.Run("zero is a value, not a clear", func(t *testing.T) {
		var body nullableBody
		require.NoError(t, json.Unmarshal([]byte(`{"door_price":0}`), &body))
		assert.True(t, body.DoorPrice.Present())
		assert.False(t, body.DoorPrice.Clears(), "0 must not read as a clear")
		v, ok := body.DoorPrice.Value()
		require.True(t, ok)
		assert.Equal(t, 0.0, v)
	})

	t.Run("a malformed value is an error, not a clear", func(t *testing.T) {
		var body nullableBody
		assert.Error(t, json.Unmarshal([]byte(`{"door_price":"free"}`), &body))
	})

	t.Run("timestamps take the same three states", func(t *testing.T) {
		var body nullableBody
		require.NoError(t, json.Unmarshal([]byte(`{"doors_at":"2026-05-01T19:00:00Z"}`), &body))
		v, ok := body.DoorsAt.Value()
		require.True(t, ok)
		assert.Equal(t, time.Date(2026, 5, 1, 19, 0, 0, 0, time.UTC), v.UTC())

		var cleared nullableBody
		require.NoError(t, json.Unmarshal([]byte(`{"doors_at":null}`), &cleared))
		assert.True(t, cleared.DoorsAt.Clears())
	})
}

// Encoding has to land inside the schema Schema publishes. Left to struct
// reflection the type marshals as `{}` -- the embedded contract keeps its state
// unexported -- which is an empty object where the document promises a number
// or null, so this asserts the spelling rather than the state.
func TestNullableInput_MarshalMatchesSchema(t *testing.T) {
	set, err := json.Marshal(NullableInputSet(35.0))
	require.NoError(t, err)
	assert.Equal(t, "35", string(set))

	cleared, err := json.Marshal(NullableInputClear[float64]())
	require.NoError(t, err)
	assert.Equal(t, "null", string(cleared))

	// Absence has no spelling: `omitempty` does not omit a struct, so an
	// unmentioned field encodes as an explicit clear. Documented rather than
	// fixed, because a response field wanting "no value" should use a plain *T.
	absent, err := json.Marshal(NullableInput[float64]{})
	require.NoError(t, err)
	assert.Equal(t, "null", string(absent))
}

// The generated schema is what huma VALIDATES against, before UnmarshalJSON is
// reached. Without SchemaProvider the Go type would publish an object schema and
// every real number would be refused as the wrong type, so this asserts the
// published shape rather than trusting the round trip below alone.
func TestNullableInput_Schema(t *testing.T) {
	registry := huma.NewMapRegistry("#/components/schemas/", huma.DefaultSchemaNamer)

	price := huma.SchemaFromType(registry, reflect.TypeOf(NullableInput[float64]{}))
	assert.Equal(t, huma.TypeNumber, price.Type)
	assert.True(t, price.Nullable, "a nullable field must accept null in the schema")

	// T's own format survives: the wrapper defers to huma for it rather than
	// restating it, which is why one generic covers prices and timestamps.
	doors := huma.SchemaFromType(registry, reflect.TypeOf(NullableInput[time.Time]{}))
	assert.Equal(t, huma.TypeString, doors.Type)
	assert.Equal(t, "date-time", doors.Format)
	assert.True(t, doors.Nullable)
}

// End to end through a real huma operation, because the two halves above can
// each be right while the pair still fails: huma validates the parsed body
// against the schema and only then unmarshals into the struct, so a schema that
// forbids null would reject the clear before the decoder ever saw it.
func TestNullableInput_ThroughHumaValidation(t *testing.T) {
	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("test", "1.0.0"))

	type resp struct {
		Body struct {
			Present bool `json:"present"`
			Clears  bool `json:"clears"`
			Value   float64
		}
	}
	huma.Register(api, huma.Operation{
		OperationID: "patch-thing",
		Method:      http.MethodPatch,
		Path:        "/thing",
	}, func(_ context.Context, in *struct{ Body nullableBody }) (*resp, error) {
		out := &resp{}
		out.Body.Present = in.Body.DoorPrice.Present()
		out.Body.Clears = in.Body.DoorPrice.Clears()
		out.Body.Value, _ = in.Body.DoorPrice.Value()
		return out, nil
	})

	call := func(payload string) (int, string) {
		req := httptest.NewRequest(http.MethodPatch, "/thing", strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Code, rec.Body.String()
	}

	status, body := call(`{"door_price":40}`)
	require.Equal(t, http.StatusOK, status, body)
	assert.Contains(t, body, `"present":true`)
	assert.Contains(t, body, `"clears":false`)

	status, body = call(`{"door_price":null}`)
	require.Equal(t, http.StatusOK, status, body)
	assert.Contains(t, body, `"clears":true`)

	status, body = call(`{"title":"Boris"}`)
	require.Equal(t, http.StatusOK, status, body)
	assert.Contains(t, body, `"present":false`)

	// A wrong type is still a 422 from huma's own validation, so the wrapper
	// does not buy leniency along with nullability.
	status, _ = call(`{"door_price":"free"}`)
	assert.Equal(t, http.StatusUnprocessableEntity, status)
}
