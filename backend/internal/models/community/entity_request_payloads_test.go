package community

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func strptr(s string) *string   { return &s }
func intptr(i int) *int         { return &i }
func fltptr(f float64) *float64 { return &f }

// TestRoundTrip_AllFieldsSet serializes a fully-populated payload of each type,
// deserializes it back via UnmarshalPayload, and asserts NO field-level data
// loss. This is the core "polymorphism in the table, typing in the code"
// guarantee — what goes into the JSONB column comes back out identically.
func TestRoundTrip_AllFieldsSet(t *testing.T) {
	t.Run("artist", func(t *testing.T) {
		in := ArtistRequestPayload{
			Name:             "Sun City Girls",
			City:             strptr("Phoenix"),
			State:            strptr("AZ"),
			Country:          strptr("USA"),
			Description:      strptr("Experimental trio."),
			ImageURL:         strptr("https://img.example/scg.jpg"),
			BandcampEmbedURL: strptr("https://bandcamp.example/scg"),
		}
		raw, err := MarshalPayload(in)
		require.NoError(t, err)
		out, err := UnmarshalPayload[ArtistRequestPayload](raw)
		require.NoError(t, err)
		assert.Equal(t, in, out)
	})

	t.Run("release", func(t *testing.T) {
		in := ReleaseRequestPayload{
			Title:       "Torch of the Mystics",
			ReleaseType: strptr("lp"),
			ReleaseYear: intptr(1990),
			ReleaseDate: strptr("1990-01-01"),
			CoverArtURL: strptr("https://img.example/totm.jpg"),
			Description: strptr("Classic."),
		}
		raw, err := MarshalPayload(in)
		require.NoError(t, err)
		out, err := UnmarshalPayload[ReleaseRequestPayload](raw)
		require.NoError(t, err)
		assert.Equal(t, in, out)
	})

	t.Run("label", func(t *testing.T) {
		in := LabelRequestPayload{
			Name:        "Abduction",
			City:        strptr("Seattle"),
			State:       strptr("WA"),
			Country:     strptr("USA"),
			FoundedYear: intptr(1996),
			Description: strptr("Reissue label."),
			ImageURL:    strptr("https://img.example/abduction.jpg"),
		}
		raw, err := MarshalPayload(in)
		require.NoError(t, err)
		out, err := UnmarshalPayload[LabelRequestPayload](raw)
		require.NoError(t, err)
		assert.Equal(t, in, out)
	})

	t.Run("show", func(t *testing.T) {
		in := ShowRequestPayload{
			Title:          "Secret Show",
			EventDate:      "2026-07-04T20:00:00Z",
			City:           strptr("Tucson"),
			State:          strptr("AZ"),
			Price:          fltptr(15.5),
			AgeRequirement: strptr("21+"),
			Description:    strptr("BYO."),
			TicketURL:      strptr("https://tix.example/secret"),
			ImageURL:       strptr("https://img.example/secret.jpg"),
		}
		raw, err := MarshalPayload(in)
		require.NoError(t, err)
		out, err := UnmarshalPayload[ShowRequestPayload](raw)
		require.NoError(t, err)
		assert.Equal(t, in, out)
	})

	t.Run("venue", func(t *testing.T) {
		in := VenueRequestPayload{
			Name:        "The Trunk Space",
			City:        "Phoenix",
			State:       "AZ",
			Address:     strptr("1124 N 3rd St"),
			Country:     strptr("USA"),
			Zipcode:     strptr("85004"),
			Description: strptr("All-ages DIY."),
			ImageURL:    strptr("https://img.example/trunk.jpg"),
		}
		raw, err := MarshalPayload(in)
		require.NoError(t, err)
		out, err := UnmarshalPayload[VenueRequestPayload](raw)
		require.NoError(t, err)
		assert.Equal(t, in, out)
	})

	t.Run("festival", func(t *testing.T) {
		in := FestivalRequestPayload{
			Name:         "Desert Daze",
			EditionYear:  2026,
			StartDate:    "2026-09-25",
			EndDate:      "2026-09-27",
			Description:  strptr("Psych fest."),
			LocationName: strptr("Lake Perris"),
			City:         strptr("Perris"),
			State:        strptr("CA"),
			Country:      strptr("USA"),
			Website:      strptr("https://desertdaze.example"),
			TicketURL:    strptr("https://tix.example/dd"),
			FlyerURL:     strptr("https://img.example/dd.jpg"),
		}
		raw, err := MarshalPayload(in)
		require.NoError(t, err)
		out, err := UnmarshalPayload[FestivalRequestPayload](raw)
		require.NoError(t, err)
		assert.Equal(t, in, out)
	})
}

// TestRoundTrip_OnlyRequiredFields proves optional pointer fields round-trip as
// nil (omitempty drops them from JSON, and they come back nil — not "" or a
// zero pointer-to-empty). Confirms the absence of a field is preserved, not
// silently coerced.
func TestRoundTrip_OnlyRequiredFields(t *testing.T) {
	in := ArtistRequestPayload{Name: "Minimal"}
	raw, err := MarshalPayload(in)
	require.NoError(t, err)

	// omitempty: optional fields should not appear in the wire form.
	var asMap map[string]any
	require.NoError(t, json.Unmarshal(raw, &asMap))
	assert.Equal(t, map[string]any{"name": "Minimal"}, asMap)

	out, err := UnmarshalPayload[ArtistRequestPayload](raw)
	require.NoError(t, err)
	assert.Equal(t, in, out)
	assert.Nil(t, out.City)
	assert.Nil(t, out.Description)
}

// TestUnmarshalPayload_FailsLoudOnUnknownField is the schema-drift guard: a
// stored payload carrying a field the struct does not declare must ERROR, not
// silently drop the field and return a partial struct.
func TestUnmarshalPayload_FailsLoudOnUnknownField(t *testing.T) {
	raw := json.RawMessage(`{"name":"X","bogus_field":"surprise"}`)
	_, err := UnmarshalPayload[ArtistRequestPayload](raw)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "artist")
}

// TestUnmarshalPayload_FailsLoudOnWrongType guards against decoding a row's
// payload with the WRONG T for its entity_type. A festival payload (which has
// edition_year / start_date the artist struct doesn't declare) decoded as an
// artist must error rather than silently returning an artist with just a name.
func TestUnmarshalPayload_FailsLoudOnWrongType(t *testing.T) {
	festival := FestivalRequestPayload{Name: "DD", EditionYear: 2026, StartDate: "2026-09-25", EndDate: "2026-09-27"}
	raw, err := MarshalPayload(festival)
	require.NoError(t, err)

	_, err = UnmarshalPayload[ArtistRequestPayload](raw)
	require.Error(t, err, "decoding a festival payload as an artist must fail loud")
}

// TestUnmarshalPayload_FailsLoudOnEmpty rejects empty/whitespace input — the
// column is NOT NULL so empty signals corruption, not a valid empty request.
func TestUnmarshalPayload_FailsLoudOnEmpty(t *testing.T) {
	for _, raw := range []json.RawMessage{nil, json.RawMessage(""), json.RawMessage("   ")} {
		_, err := UnmarshalPayload[VenueRequestPayload](raw)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty")
	}
}

// TestUnmarshalPayload_FailsLoudOnTrailingData rejects concatenated/garbage
// trailing content after a valid JSON object.
func TestUnmarshalPayload_FailsLoudOnTrailingData(t *testing.T) {
	raw := json.RawMessage(`{"name":"X"}{"name":"Y"}`)
	_, err := UnmarshalPayload[ArtistRequestPayload](raw)
	require.Error(t, err)
}

// TestPayloadRegistry_MatchesEntityTypeConstants asserts the registry keys
// equal the entity_type discriminator constants — the Go-side anchor the CI
// parity check compares against the migration CHECK constraint.
func TestPayloadRegistry_MatchesEntityTypeConstants(t *testing.T) {
	want := map[string]bool{
		EntityRequestArtist:   true,
		EntityRequestRelease:  true,
		EntityRequestLabel:    true,
		EntityRequestShow:     true,
		EntityRequestVenue:    true,
		EntityRequestFestival: true,
	}
	got := map[string]bool{}
	for _, et := range ValidEntityRequestTypes() {
		got[et] = true
	}
	assert.Equal(t, want, got)

	// Every registered payload reports its own entity_type back, so the map
	// key and the struct can't drift.
	for et, p := range payloadRegistry {
		assert.Equal(t, et, p.entityRequestType(), "payload for %q reports wrong type", et)
	}
}

// TestIsValidEntityRequestType covers the trust-boundary validator.
func TestIsValidEntityRequestType(t *testing.T) {
	assert.True(t, IsValidEntityRequestType(EntityRequestArtist))
	assert.True(t, IsValidEntityRequestType(EntityRequestFestival))
	assert.False(t, IsValidEntityRequestType("podcast"))
	assert.False(t, IsValidEntityRequestType(""))
}

// TestValidateEntityRequestPayload covers the PSY-997 create-time payload guard:
// clean payloads pass; malformed shape / unknown fields / missing required
// fields are rejected.
func TestValidateEntityRequestPayload(t *testing.T) {
	t.Run("valid artist", func(t *testing.T) {
		assert.NoError(t, ValidateEntityRequestPayload(EntityRequestArtist, json.RawMessage(`{"name":"Boris"}`)))
	})
	t.Run("valid venue with required city+state", func(t *testing.T) {
		assert.NoError(t, ValidateEntityRequestPayload(EntityRequestVenue, json.RawMessage(`{"name":"Trunk Space","city":"Phoenix","state":"AZ"}`)))
	})
	t.Run("artist missing name", func(t *testing.T) {
		assert.Error(t, ValidateEntityRequestPayload(EntityRequestArtist, json.RawMessage(`{"name":""}`)))
	})
	t.Run("artist blank-only name", func(t *testing.T) {
		assert.Error(t, ValidateEntityRequestPayload(EntityRequestArtist, json.RawMessage(`{"name":"   "}`)))
	})
	t.Run("unknown field rejected", func(t *testing.T) {
		assert.Error(t, ValidateEntityRequestPayload(EntityRequestArtist, json.RawMessage(`{"name":"Boris","sneaky":1}`)))
	})
	t.Run("venue missing required state", func(t *testing.T) {
		assert.Error(t, ValidateEntityRequestPayload(EntityRequestVenue, json.RawMessage(`{"name":"X","city":"Phoenix","state":""}`)))
	})
	t.Run("malformed json", func(t *testing.T) {
		assert.Error(t, ValidateEntityRequestPayload(EntityRequestArtist, json.RawMessage(`{"name":`)))
	})
	t.Run("unsupported type", func(t *testing.T) {
		assert.Error(t, ValidateEntityRequestPayload("podcast", json.RawMessage(`{}`)))
	})
	t.Run("festival requires dates", func(t *testing.T) {
		assert.Error(t, ValidateEntityRequestPayload(EntityRequestFestival, json.RawMessage(`{"name":"Desert Daze","edition_year":2026,"start_date":"2026-01-01","end_date":""}`)))
	})
	t.Run("valid festival", func(t *testing.T) {
		assert.NoError(t, ValidateEntityRequestPayload(EntityRequestFestival, json.RawMessage(`{"name":"Desert Daze","edition_year":2026,"start_date":"2026-01-01","end_date":"2026-01-03"}`)))
	})
	t.Run("festival rejects malformed start_date", func(t *testing.T) {
		// Slash-formatted date passes a non-empty check but isn't YYYY-MM-DD;
		// fulfillment derives edition_year from it and feeds a DATE column.
		assert.Error(t, ValidateEntityRequestPayload(EntityRequestFestival, json.RawMessage(`{"name":"Desert Daze","start_date":"2026/01/01","end_date":"2026-01-03"}`)))
	})
	t.Run("festival rejects impossible date", func(t *testing.T) {
		assert.Error(t, ValidateEntityRequestPayload(EntityRequestFestival, json.RawMessage(`{"name":"Desert Daze","start_date":"2026-13-99","end_date":"2026-01-03"}`)))
	})
	t.Run("festival rejects negative edition_year", func(t *testing.T) {
		assert.Error(t, ValidateEntityRequestPayload(EntityRequestFestival, json.RawMessage(`{"name":"Desert Daze","edition_year":-5,"start_date":"2026-01-01","end_date":"2026-01-03"}`)))
	})
	t.Run("festival accepts non-Latin name", func(t *testing.T) {
		// A name that slugifies to "" (non-ASCII) must NOT be rejected — the
		// fulfiller tolerates an empty derived series_slug (same as the display
		// slug), consistent with how artist/venue/label requests behave.
		assert.NoError(t, ValidateEntityRequestPayload(EntityRequestFestival, json.RawMessage(`{"name":"東京フェス","start_date":"2026-01-01","end_date":"2026-01-03"}`)))
	})
	// PSY-1038: the nullable URL fields now carry onto the created entity, so
	// they are scheme-validated at the boundary (a hostile scheme must not ride
	// the payload onto a real artist/venue/label).
	t.Run("artist accepts valid image_url + bandcamp_embed_url", func(t *testing.T) {
		assert.NoError(t, ValidateEntityRequestPayload(EntityRequestArtist, json.RawMessage(`{"name":"Boris","image_url":"https://example.com/b.jpg","bandcamp_embed_url":"https://boris.bandcamp.com/album/x"}`)))
	})
	t.Run("artist rejects javascript: image_url", func(t *testing.T) {
		assert.Error(t, ValidateEntityRequestPayload(EntityRequestArtist, json.RawMessage(`{"name":"Boris","image_url":"javascript:alert(1)"}`)))
	})
	t.Run("artist rejects non-http bandcamp_embed_url", func(t *testing.T) {
		assert.Error(t, ValidateEntityRequestPayload(EntityRequestArtist, json.RawMessage(`{"name":"Boris","bandcamp_embed_url":"data:text/html,evil"}`)))
	})
	t.Run("venue rejects non-http image_url", func(t *testing.T) {
		assert.Error(t, ValidateEntityRequestPayload(EntityRequestVenue, json.RawMessage(`{"name":"Trunk Space","city":"Phoenix","state":"AZ","image_url":"ftp://example.com/x.jpg"}`)))
	})
	t.Run("label rejects non-http image_url", func(t *testing.T) {
		assert.Error(t, ValidateEntityRequestPayload(EntityRequestLabel, json.RawMessage(`{"name":"Hydra Head","image_url":"javascript:void(0)"}`)))
	})
	t.Run("empty image_url is allowed", func(t *testing.T) {
		assert.NoError(t, ValidateEntityRequestPayload(EntityRequestLabel, json.RawMessage(`{"name":"Hydra Head","image_url":""}`)))
	})
}
