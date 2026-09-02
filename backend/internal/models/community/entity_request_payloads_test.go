package community

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

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
			DoorPrice:      fltptr(20),
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
	// PSY-1966: the embed URL must be a Bandcamp RELEASE page, not merely an
	// http(s) URL. The queue is the second reachable write path onto this
	// column, and fulfilment re-runs this same validator, so a value refused
	// here cannot arrive on a live artist by being approved later.
	t.Run("artist rejects a foreign host in bandcamp_embed_url", func(t *testing.T) {
		assert.Error(t, ValidateEntityRequestPayload(EntityRequestArtist, json.RawMessage(`{"name":"Boris","bandcamp_embed_url":"https://evil.test/album/checkout"}`)))
		assert.Error(t, ValidateEntityRequestPayload(EntityRequestArtist, json.RawMessage(`{"name":"Boris","bandcamp_embed_url":"https://bandcamp.com.attacker.test/album/x"}`)))
	})
	t.Run("artist rejects an on-platform non-release bandcamp_embed_url", func(t *testing.T) {
		assert.Error(t, ValidateEntityRequestPayload(EntityRequestArtist, json.RawMessage(`{"name":"Boris","bandcamp_embed_url":"https://boris.bandcamp.com"}`)))
		assert.Error(t, ValidateEntityRequestPayload(EntityRequestArtist, json.RawMessage(`{"name":"Boris","bandcamp_embed_url":"https://boris.bandcamp.com/merch/shirt"}`)))
	})
	t.Run("artist accepts a track page and an empty bandcamp_embed_url", func(t *testing.T) {
		assert.NoError(t, ValidateEntityRequestPayload(EntityRequestArtist, json.RawMessage(`{"name":"Boris","bandcamp_embed_url":"https://boris.bandcamp.com/track/x"}`)))
		assert.NoError(t, ValidateEntityRequestPayload(EntityRequestArtist, json.RawMessage(`{"name":"Boris","bandcamp_embed_url":""}`)))
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
	// PSY-1038 (adversarial): length caps mirror the catalog columns so an
	// over-long value is rejected here (422) not at INSERT (500), and URL
	// validation covers every fulfillable type (festival/release too).
	t.Run("artist rejects over-long image_url", func(t *testing.T) {
		long := "https://example.com/" + strings.Repeat("a", 2100)
		assert.Error(t, ValidateEntityRequestPayload(EntityRequestArtist, json.RawMessage(`{"name":"Boris","image_url":"`+long+`"}`)))
	})
	t.Run("artist rejects over-long description", func(t *testing.T) {
		long := strings.Repeat("x", 5001)
		assert.Error(t, ValidateEntityRequestPayload(EntityRequestArtist, json.RawMessage(`{"name":"Boris","description":"`+long+`"}`)))
	})
	t.Run("festival rejects javascript: website", func(t *testing.T) {
		assert.Error(t, ValidateEntityRequestPayload(EntityRequestFestival, json.RawMessage(`{"name":"Desert Daze","start_date":"2026-01-01","end_date":"2026-01-03","website":"javascript:alert(1)"}`)))
	})
	t.Run("release rejects data: cover_art_url", func(t *testing.T) {
		assert.Error(t, ValidateEntityRequestPayload(EntityRequestRelease, json.RawMessage(`{"title":"Pink","cover_art_url":"data:image/png,evil"}`)))
	})
	t.Run("festival accepts valid website", func(t *testing.T) {
		assert.NoError(t, ValidateEntityRequestPayload(EntityRequestFestival, json.RawMessage(`{"name":"Desert Daze","start_date":"2026-01-01","end_date":"2026-01-03","website":"https://desertdaze.org"}`)))
	})
	t.Run("festival rejects over-long website (VARCHAR(500))", func(t *testing.T) {
		// 501–2048 chars: a valid https URL that would 500 at INSERT into the
		// festival website VARCHAR(500) column if the cap weren't column-accurate.
		long := "https://example.com/?" + strings.Repeat("a", 600)
		assert.Error(t, ValidateEntityRequestPayload(EntityRequestFestival, json.RawMessage(`{"name":"Desert Daze","start_date":"2026-01-01","end_date":"2026-01-03","website":"`+long+`"}`)))
	})
	// PSY-1037: show is now fulfillable (admin-supplied associations), so its
	// payload fields are validated like every other fulfillable type.
	t.Run("show accepts RFC3339 event_date", func(t *testing.T) {
		assert.NoError(t, ValidateEntityRequestPayload(EntityRequestShow, json.RawMessage(`{"title":"Boris","event_date":"2026-07-04T21:30:00-07:00"}`)))
	})
	t.Run("show accepts date-only event_date", func(t *testing.T) {
		assert.NoError(t, ValidateEntityRequestPayload(EntityRequestShow, json.RawMessage(`{"title":"Boris","event_date":"2026-07-04"}`)))
	})
	t.Run("show rejects unparseable event_date", func(t *testing.T) {
		assert.Error(t, ValidateEntityRequestPayload(EntityRequestShow, json.RawMessage(`{"title":"Boris","event_date":"next summer"}`)))
	})
	t.Run("show rejects javascript: image_url", func(t *testing.T) {
		assert.Error(t, ValidateEntityRequestPayload(EntityRequestShow, json.RawMessage(`{"title":"Boris","event_date":"2026-07-04","image_url":"javascript:alert(1)"}`)))
	})
	t.Run("show rejects over-long ticket_url (VARCHAR(500))", func(t *testing.T) {
		long := "https://example.com/?" + strings.Repeat("t", 600)
		assert.Error(t, ValidateEntityRequestPayload(EntityRequestShow, json.RawMessage(`{"title":"Boris","event_date":"2026-07-04","ticket_url":"`+long+`"}`)))
	})
	// PSY-1037 (adversarial): the remaining show fields are capped to the
	// direct create handler's limits — a value that slipped past here would
	// 500 at INSERT after the row is claimed, orphaning the request.
	t.Run("show rejects over-long title", func(t *testing.T) {
		long := strings.Repeat("t", 256)
		assert.Error(t, ValidateEntityRequestPayload(EntityRequestShow, json.RawMessage(`{"title":"`+long+`","event_date":"2026-07-04"}`)))
	})
	t.Run("show rejects negative price", func(t *testing.T) {
		assert.Error(t, ValidateEntityRequestPayload(EntityRequestShow, json.RawMessage(`{"title":"Boris","event_date":"2026-07-04","price":-5}`)))
	})
	t.Run("show rejects absurd price", func(t *testing.T) {
		assert.Error(t, ValidateEntityRequestPayload(EntityRequestShow, json.RawMessage(`{"title":"Boris","event_date":"2026-07-04","price":20000}`)))
	})
	// PSY-1864: door_price rides onto the created show the same way price
	// does, so it needs the same cap. Without its own check a value that
	// cleared the price gate on the other field would 500 at INSERT.
	t.Run("show rejects negative door_price", func(t *testing.T) {
		assert.Error(t, ValidateEntityRequestPayload(EntityRequestShow, json.RawMessage(`{"title":"Boris","event_date":"2026-07-04","door_price":-5}`)))
	})
	t.Run("show rejects absurd door_price", func(t *testing.T) {
		assert.Error(t, ValidateEntityRequestPayload(EntityRequestShow, json.RawMessage(`{"title":"Boris","event_date":"2026-07-04","door_price":20000}`)))
	})
	t.Run("show accepts an advance and door price pair", func(t *testing.T) {
		assert.NoError(t, ValidateEntityRequestPayload(EntityRequestShow, json.RawMessage(`{"title":"Boris","event_date":"2026-07-04","price":35,"door_price":40}`)))
	})
	t.Run("show rejects over-long age_requirement", func(t *testing.T) {
		long := strings.Repeat("a", 51)
		assert.Error(t, ValidateEntityRequestPayload(EntityRequestShow, json.RawMessage(`{"title":"Boris","event_date":"2026-07-04","age_requirement":"`+long+`"}`)))
	})
	t.Run("show rejects over-long description", func(t *testing.T) {
		long := strings.Repeat("d", 5001)
		assert.Error(t, ValidateEntityRequestPayload(EntityRequestShow, json.RawMessage(`{"title":"Boris","event_date":"2026-07-04","description":"`+long+`"}`)))
	})
	t.Run("show rejects over-long state (VARCHAR(10))", func(t *testing.T) {
		assert.Error(t, ValidateEntityRequestPayload(EntityRequestShow, json.RawMessage(`{"title":"Boris","event_date":"2026-07-04","state":"New South Wales"}`)))
	})
}

// TestPayloadImageURL is the extraction half of the PSY-1675 SSRF guard: the
// handler resolves whatever this returns, so a type whose image_url it fails to
// surface is a type whose flyer is never host-checked.
func TestPayloadImageURL(t *testing.T) {
	cases := []struct {
		name       string
		entityType string
		raw        string
		want       *string
	}{
		{"artist", EntityRequestArtist, `{"name":"Boris","image_url":"https://example.com/a.jpg"}`, strptr("https://example.com/a.jpg")},
		{"label", EntityRequestLabel, `{"name":"Hydra Head","image_url":"https://example.com/l.jpg"}`, strptr("https://example.com/l.jpg")},
		{"venue", EntityRequestVenue, `{"name":"Trunk Space","city":"Phoenix","state":"AZ","image_url":"https://example.com/v.jpg"}`, strptr("https://example.com/v.jpg")},
		{"show", EntityRequestShow, `{"title":"Boris","event_date":"2026-07-04","image_url":"https://example.com/s.jpg"}`, strptr("https://example.com/s.jpg")},
		{"artist without image_url", EntityRequestArtist, `{"name":"Boris"}`, nil},
		{"release carries cover_art_url, not image_url", EntityRequestRelease, `{"title":"Pink","cover_art_url":"https://example.com/c.jpg"}`, nil},
		{"festival carries flyer_url, not image_url", EntityRequestFestival, `{"name":"Fest","edition_year":2026,"start_date":"2026-07-04","end_date":"2026-07-05","flyer_url":"https://example.com/f.jpg"}`, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := PayloadImageURL(c.entityType, json.RawMessage(c.raw))
			require.NoError(t, err)
			if c.want == nil {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.Equal(t, *c.want, *got)
		})
	}
}

// TestPayloadImageURL_FailsClosed: a payload that will not decode, and an
// entity type nobody taught this function about, are errors rather than "no
// image URL" — otherwise adding an entity_type would silently skip the guard.
func TestPayloadImageURL_FailsClosed(t *testing.T) {
	_, err := PayloadImageURL(EntityRequestArtist, json.RawMessage(`{"name":"Boris","sneaky":1}`))
	assert.Error(t, err)

	_, err = PayloadImageURL("podcast", json.RawMessage(`{"name":"Boris"}`))
	assert.Error(t, err)
}

// TestPayloadImageURL_CoversEveryRegisteredType keeps the switch honest: a new
// entity_type added to the registry without a branch here fails this test
// rather than quietly returning "unknown entity type" at a write boundary.
func TestPayloadImageURL_CoversEveryRegisteredType(t *testing.T) {
	minimal := map[string]string{
		EntityRequestArtist:   `{"name":"Boris"}`,
		EntityRequestRelease:  `{"title":"Pink"}`,
		EntityRequestLabel:    `{"name":"Hydra Head"}`,
		EntityRequestVenue:    `{"name":"Trunk Space","city":"Phoenix","state":"AZ"}`,
		EntityRequestShow:     `{"title":"Boris","event_date":"2026-07-04"}`,
		EntityRequestFestival: `{"name":"Fest","edition_year":2026,"start_date":"2026-07-04","end_date":"2026-07-05"}`,
	}
	for entityType := range payloadRegistry {
		raw, ok := minimal[entityType]
		require.Truef(t, ok, "entity type %q has no fixture here — add one, and give PayloadImageURL a branch for it", entityType)
		_, err := PayloadImageURL(entityType, json.RawMessage(raw))
		assert.NoErrorf(t, err, "PayloadImageURL has no branch for registered entity type %q", entityType)
	}
}

// PSY-1977: event_date feeds a btree index, so its LENGTH is a trust-boundary
// concern and not a formatting one. time.Parse accepts an arbitrarily long
// fractional second — it truncates past 10 digits rather than rejecting — so
// without this cap a multi-kilobyte event_date parses cleanly, reaches the
// INSERT, and blows the btree's index-row limit. That is SQLSTATE 54000, which is
// not a duplicate-key error, so the dedup branch never fires and a contributor
// converts their own input into a 500.
func TestValidateShow_OversizedEventDate(t *testing.T) {
	oversized := "2026-09-03T20:00:00." + strings.Repeat("1", 3000) + "-07:00"

	// The premise: this value is NOT rejected by parsing, which is why a separate
	// length check has to exist.
	_, parseErr := time.Parse(time.RFC3339, oversized)
	require.NoError(t, parseErr, "if this ever fails to parse, the length cap's rationale changed")

	raw, err := MarshalPayload(ShowRequestPayload{Title: "Long Night", EventDate: oversized})
	require.NoError(t, err)

	err = ValidateEntityRequestPayload(EntityRequestShow, raw)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "event_date must be 64 characters or fewer")

	// UNICODE WHITESPACE must not smuggle a huge value past the cap. Go's
	// strings.TrimSpace strips 25 space runes; SQL trim() strips ASCII 0x20 only,
	// so a value padded with U+3000 trims to 10 bytes in Go while the full 40 KB
	// goes to the payload column. The index survives it regardless (the key term
	// is truncated), so what this pins is the BOUNDARY measuring the untrimmed
	// value, which is what keeps the stored payload sane.
	padded := "2026-09-03" + strings.Repeat("　", 4000)
	require.Equal(t, "2026-09-03", strings.TrimSpace(padded),
		"the fixture must LOOK short to Go, or it is not testing the bypass")
	paddedRaw, err := MarshalPayload(ShowRequestPayload{Title: "Padded", EventDate: padded})
	require.NoError(t, err)
	err = ValidateEntityRequestPayload(EntityRequestShow, paddedRaw)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "event_date must be 64 characters or fewer")

	// The cap must not reject anything legal. 35 characters is the longest
	// spelling Go preserves: RFC 3339 permits more fractional digits, but
	// time.Parse truncates past 9.
	longestLegal := "2026-09-03T20:00:00.123456789-07:00"
	require.LessOrEqual(t, len(longestLegal), maxRequestDateLen)
	okRaw, err := MarshalPayload(ShowRequestPayload{Title: "Long Night", EventDate: longestLegal})
	require.NoError(t, err)
	assert.NoError(t, ValidateEntityRequestPayload(EntityRequestShow, okRaw))

	// A legal value with ordinary surrounding spaces still validates: the cap is
	// generous enough that padding does not turn a legal date into a 422.
	spacedRaw, err := MarshalPayload(ShowRequestPayload{Title: "Spaced", EventDate: "  2026-09-03  "})
	require.NoError(t, err)
	assert.NoError(t, ValidateEntityRequestPayload(EntityRequestShow, spacedRaw))
}
