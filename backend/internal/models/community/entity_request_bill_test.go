package community

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// PSY-1858: a show request can carry the bill the contributor knew, so the
// approving admin does not have to re-type it out of the source excerpt.

// The bill survives the JSONB round trip with its per-act roles intact: the
// same no-data-loss guarantee every other payload field has.
func TestRoundTrip_ShowRequestPayloadArtists(t *testing.T) {
	in := ShowRequestPayload{
		Title:     "Boris with Earth",
		EventDate: "2026-09-12",
		Artists: []ShowRequestArtist{
			{Name: "Boris", SetType: strptr("headliner")},
			{Name: "Earth", SetType: strptr("direct_support")},
			{Name: "Local Opener"},
		},
	}
	raw, err := MarshalPayload(in)
	require.NoError(t, err)
	out, err := UnmarshalPayload[ShowRequestPayload](raw)
	require.NoError(t, err)
	assert.Equal(t, in, out)
}

// A show payload with no bill at all stays byte-identical to what a client that
// predates this field sends. This is the backward-compatibility guard: every
// row already queued has no "artists" key, and both UnmarshalPayload (which
// rejects UNKNOWN fields, not missing ones) and validation must read those rows
// exactly as before.
func TestShowRequestPayload_AbsentArtistsIsUnchanged(t *testing.T) {
	legacy := json.RawMessage(`{"title":"Old Queued Show","event_date":"2026-09-12"}`)
	require.NoError(t, ValidateEntityRequestPayload(EntityRequestShow, legacy))

	out, err := UnmarshalPayload[ShowRequestPayload](legacy)
	require.NoError(t, err)
	assert.Nil(t, out.Artists, "a payload with no bill must decode to no bill, not an empty one")

	// And the field is omitempty, so a bill-less payload does not start writing
	// an "artists" key into rows that never had one.
	raw, err := MarshalPayload(ShowRequestPayload{Title: "X", EventDate: "2026-09-12"})
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "artists")
}

func TestValidateEntityRequestPayload_ShowArtists(t *testing.T) {
	const base = `"title":"Boris","event_date":"2026-09-12"`

	t.Run("a named bill with roles is accepted", func(t *testing.T) {
		assert.NoError(t, ValidateEntityRequestPayload(EntityRequestShow, json.RawMessage(
			`{`+base+`,"artists":[{"name":"Boris","set_type":"headliner"},{"name":"Earth"}]}`)))
	})

	t.Run("an empty bill is accepted", func(t *testing.T) {
		// The field is optional: a contributor who does not know the bill leaves
		// it out, and the request is still a valid show request.
		assert.NoError(t, ValidateEntityRequestPayload(EntityRequestShow, json.RawMessage(
			`{`+base+`,"artists":[]}`)))
	})

	t.Run("an act with no name is rejected", func(t *testing.T) {
		err := ValidateEntityRequestPayload(EntityRequestShow, json.RawMessage(
			`{`+base+`,"artists":[{"name":"Boris"},{"name":"  "}]}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "artists[1].name is required",
			"the rejection must name the offending bill index")
	})

	t.Run("an over-long act name is rejected", func(t *testing.T) {
		long := strings.Repeat("a", 256)
		assert.Error(t, ValidateEntityRequestPayload(EntityRequestShow, json.RawMessage(
			`{`+base+`,"artists":[{"name":"`+long+`"}]}`)))
	})

	t.Run("a bill over the cap is rejected", func(t *testing.T) {
		// The cap is the queue's half of a shared contract: a bill that clears
		// submit must still be one the approve path (maxShowArtistInputs, which
		// aliases MaxShowRequestArtists) will accept.
		acts := make([]string, 0, MaxShowRequestArtists+1)
		for i := 0; i <= MaxShowRequestArtists; i++ {
			acts = append(acts, fmt.Sprintf(`{"name":"Act %d"}`, i))
		}
		over := `{` + base + `,"artists":[` + strings.Join(acts, ",") + `]}`
		err := ValidateEntityRequestPayload(EntityRequestShow, json.RawMessage(over))
		require.Error(t, err)
		assert.Contains(t, err.Error(), fmt.Sprintf("capped at %d", MaxShowRequestArtists))

		// Exactly at the cap is fine: the boundary is inclusive.
		atCap := `{` + base + `,"artists":[` + strings.Join(acts[:MaxShowRequestArtists], ",") + `]}`
		assert.NoError(t, ValidateEntityRequestPayload(EntityRequestShow, json.RawMessage(atCap)))
	})

	t.Run("an act id is rejected", func(t *testing.T) {
		// Contributors have no artist picker, so the payload has no id field on
		// purpose and DisallowUnknownFields makes that stick rather than silently
		// dropping a value a client thought it was sending.
		assert.Error(t, ValidateEntityRequestPayload(EntityRequestShow, json.RawMessage(
			`{`+base+`,"artists":[{"name":"Boris","id":7}]}`)))
	})

	t.Run("an unknown per-act field is rejected", func(t *testing.T) {
		assert.Error(t, ValidateEntityRequestPayload(EntityRequestShow, json.RawMessage(
			`{`+base+`,"artists":[{"name":"Boris","is_headliner":true}]}`)))
	})

	t.Run("the role vocabulary is NOT checked here", func(t *testing.T) {
		// Deliberate, and the reason validateShowPayloadBillRoles exists at the
		// API boundary: the vocabulary lives in services/contracts, which imports
		// this package. If this ever starts failing, the check moved -- make sure
		// it moved to ONE place, not two.
		assert.NoError(t, ValidateEntityRequestPayload(EntityRequestShow, json.RawMessage(
			`{`+base+`,"artists":[{"name":"Boris","set_type":"co-headliner"}]}`)))
	})
}

func TestShowPayloadArtists(t *testing.T) {
	t.Run("returns the show's bill", func(t *testing.T) {
		got, err := ShowPayloadArtists(EntityRequestShow, json.RawMessage(
			`{"title":"Boris","event_date":"2026-09-12","artists":[{"name":"Boris","set_type":"dj"}]}`))
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "Boris", got[0].Name)
		require.NotNil(t, got[0].SetType)
		assert.Equal(t, "dj", *got[0].SetType)
	})

	t.Run("a show with no bill is nil, not an error", func(t *testing.T) {
		got, err := ShowPayloadArtists(EntityRequestShow, json.RawMessage(
			`{"title":"Boris","event_date":"2026-09-12"}`))
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("every non-show type is nil, not an error", func(t *testing.T) {
		// "No bill" is the permanently correct answer for anything that is not a
		// show, so unlike PayloadImageURL this does not fail closed on a type it
		// has not been taught about.
		for _, entityType := range ValidEntityRequestTypes() {
			if entityType == EntityRequestShow {
				continue
			}
			got, err := ShowPayloadArtists(entityType, json.RawMessage(`{"name":"Boris"}`))
			assert.NoError(t, err, "entity type %q", entityType)
			assert.Nil(t, got, "entity type %q", entityType)
		}
	})

	t.Run("a corrupt show payload surfaces the decode error", func(t *testing.T) {
		_, err := ShowPayloadArtists(EntityRequestShow, json.RawMessage(`{"title":`))
		assert.Error(t, err)
	})
}
