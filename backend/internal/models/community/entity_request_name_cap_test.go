package community

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// namedPayload builds a minimal VALID payload of entityType whose name/title —
// the field the dedup key's name term reads — is name. Every other required
// field carries a short legal value, so a failure is always about the field
// under test.
func namedPayload(t *testing.T, entityType, name string) json.RawMessage {
	t.Helper()
	var p EntityRequestPayload
	switch entityType {
	case EntityRequestArtist:
		p = ArtistRequestPayload{Name: name}
	case EntityRequestRelease:
		p = ReleaseRequestPayload{Title: name}
	case EntityRequestLabel:
		p = LabelRequestPayload{Name: name}
	case EntityRequestShow:
		p = ShowRequestPayload{Title: name, EventDate: "2026-09-03"}
	case EntityRequestVenue:
		p = VenueRequestPayload{Name: name, City: "Phoenix", State: "AZ"}
	case EntityRequestFestival:
		p = FestivalRequestPayload{
			Name: name, EditionYear: 2026,
			StartDate: "2026-06-01", EndDate: "2026-06-03",
		}
	default:
		t.Fatalf("namedPayload has no fixture for %q; a new payload type needs one", entityType)
	}
	raw, err := MarshalPayload(p)
	require.NoError(t, err)
	return raw
}

// nameFieldFor returns the JSON field each type's name term reads.
func nameFieldFor(entityType string) string {
	if entityType == EntityRequestRelease || entityType == EntityRequestShow {
		return "title"
	}
	return "name"
}

// PSY-1990: the name term of uq_entity_requests_pending_dedup was uncapped on
// every type but show. This asserts the cap on EVERY registered type, driven off
// the registry so a seventh type fails here rather than shipping uncapped.
func TestValidatePayload_NameLengthCappedOnEveryType(t *testing.T) {
	for _, entityType := range ValidEntityRequestTypes() {
		t.Run(entityType, func(t *testing.T) {
			field := nameFieldFor(entityType)
			wantErr := field + " must be " + strconv.Itoa(maxRequestNameLen) + " characters or fewer"

			atCap := strings.Repeat("a", maxRequestNameLen)
			assert.NoError(t, ValidateEntityRequestPayload(entityType, namedPayload(t, entityType, atCap)),
				"a name exactly at the cap is legal; the boundary must not be off by one")

			overCap := strings.Repeat("a", maxRequestNameLen+1)
			err := ValidateEntityRequestPayload(entityType, namedPayload(t, entityType, overCap))
			require.Error(t, err, "one byte past the cap must be refused")
			assert.Contains(t, err.Error(), wantErr)

			// 2.6 KB is the size that aborted a dedup-index build.
			err = ValidateEntityRequestPayload(entityType, namedPayload(t, entityType, strings.Repeat("a", 2600)))
			require.Error(t, err)
			assert.Contains(t, err.Error(), wantErr)

			// UNICODE WHITESPACE must not smuggle an oversized value past the cap.
			// Go's strings.TrimSpace strips 25 space runes while SQL trim() strips
			// ASCII 0x20 only, so a name measured after a Go trim would be 5 bytes
			// here and kilobytes in the expression Postgres indexes. The cap
			// therefore measures the UNTRIMMED value.
			padded := "Boris" + strings.Repeat("　", 1000)
			require.Equal(t, "Boris", strings.TrimSpace(padded),
				"the fixture must LOOK short to Go, or it is not testing the bypass")
			err = ValidateEntityRequestPayload(entityType, namedPayload(t, entityType, padded))
			require.Error(t, err, "a Unicode-padded name must not pass the cap")
			assert.Contains(t, err.Error(), wantErr)

			// Emptiness still reads as missing rather than oversized, including a
			// value that is nothing but (oversized) whitespace.
			err = ValidateEntityRequestPayload(entityType, namedPayload(t, entityType, strings.Repeat(" ", 3000)))
			require.Error(t, err)
			assert.Contains(t, err.Error(), field+" is required")
		})
	}
}

// PSY-1990: a venue's city and state are its dedup-key occurrence terms
// (PSY-1989), so they are index terms and capped for the same reason the name
// is. Values exactly at the caps must be accepted, because the index truncates
// each term to that same width.
func TestValidateVenue_CityAndStateCapped(t *testing.T) {
	base := VenueRequestPayload{Name: "The Fillmore", City: "Phoenix", State: "AZ"}

	overCity := base
	overCity.City = strings.Repeat("c", maxRequestCityLen+1)
	raw, err := MarshalPayload(overCity)
	require.NoError(t, err)
	err = ValidateEntityRequestPayload(EntityRequestVenue, raw)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "city must be "+strconv.Itoa(maxRequestCityLen)+" characters or fewer")

	overState := base
	overState.State = strings.Repeat("s", maxRequestStateLen+1)
	raw, err = MarshalPayload(overState)
	require.NoError(t, err)
	err = ValidateEntityRequestPayload(EntityRequestVenue, raw)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "state must be "+strconv.Itoa(maxRequestStateLen)+" characters or fewer")

	atCap := base
	atCap.City = strings.Repeat("c", maxRequestCityLen)
	atCap.State = strings.Repeat("s", maxRequestStateLen)
	raw, err = MarshalPayload(atCap)
	require.NoError(t, err)
	assert.NoError(t, ValidateEntityRequestPayload(EntityRequestVenue, raw),
		"values exactly at the caps are legal, and the index must not truncate them")
}

// PSY-1990: the cap exists to bound the value POSTGRES INDEXES, and SQL's trim
// is not Go's. This pins the property the index depends on — an ACCEPTED
// payload's name term is within the cap under either trim — rather than
// restating the check.
func TestAcceptedNameIsIndexTermSafe(t *testing.T) {
	// sqlTrim mirrors Postgres trim(): ASCII space only, both ends.
	sqlTrim := func(s string) string { return strings.Trim(s, " ") }

	candidates := []string{
		strings.Repeat("a", maxRequestNameLen),
		"  " + strings.Repeat("b", maxRequestNameLen-4) + "  ",
		"Boris　", // one U+3000, which SQL trim() does NOT strip
		"Boris" + strings.Repeat("　", 1000),
		strings.Repeat("a", 2600),
		"Sun City Girls",
	}

	for _, entityType := range ValidEntityRequestTypes() {
		t.Run(entityType, func(t *testing.T) {
			accepted := 0
			for _, name := range candidates {
				if err := ValidateEntityRequestPayload(entityType, namedPayload(t, entityType, name)); err != nil {
					continue // refused values never reach the index
				}
				accepted++
				assert.LessOrEqual(t, len(sqlTrim(name)), maxRequestNameLen,
					"an ACCEPTED payload's SQL-trimmed name term exceeds the cap, "+
						"so the boundary does not bound what the index stores")
				assert.LessOrEqual(t, len(strings.TrimSpace(name)), maxRequestNameLen)
			}
			require.NotZero(t, accepted, "every candidate was refused; the assertions above ran on nothing")
		})
	}
}
