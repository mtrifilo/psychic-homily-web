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

// PSY-1990: a venue's city is its dedup-key occurrence term (PSY-1989), so it is
// an index term and capped for the same reason the name is; its state is column
// parity only and deliberately not in the key. A city exactly at the cap must be
// accepted, because the index truncates that term to the same width.
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

// occurrenceKeyBounds records, for every JSON key an occurrence term may read,
// what bounds the value that key puts in the index. A key is either capped at the
// API boundary — in which case the term's truncation width must EQUAL that cap,
// so the index never keys a prefix of a value the boundary accepted whole — or
// its term is truncated for a SEMANTIC reason, where the width is the meaning
// (four digits of a year) rather than a safety bound.
//
// Adding an entity type with a new occurrence key without adding it here fails
// the test below, which is the point: an occurrence term reading an UNBOUNDED
// payload field is the SQLSTATE 54000 that PSY-1990 exists to prevent — a 500 on
// a contributor's own input, and an index build that aborts on it.
var occurrenceKeyBounds = map[string]struct {
	cap      int
	semantic bool
}{
	"event_date":   {cap: maxRequestDateLen},
	"city":         {cap: maxRequestCityLen},
	"edition_year": {semantic: true},
	"start_date":   {semantic: true},
}

func TestDedupOccurrenceTermsAreBoundedAtTheBoundary(t *testing.T) {
	declared := 0
	for _, entityType := range ValidEntityRequestTypes() {
		term := DedupOccurrenceTermFor(entityType)
		if len(term.JSONKeys) == 0 {
			continue
		}
		declared++
		t.Run(entityType, func(t *testing.T) {
			require.NotZero(t, term.Width, "an occurrence term with no width is an untruncated index term")
			for _, key := range term.JSONKeys {
				bound, known := occurrenceKeyBounds[key]
				require.Truef(t, known,
					"occurrence key %q is not recorded in occurrenceKeyBounds: state whether the "+
						"API boundary caps it, and pair the term's width with that cap", key)
				if bound.semantic {
					continue
				}
				assert.Equalf(t, bound.cap, term.Width,
					"the occurrence term reading %q truncates at %d but the boundary caps it at %d; "+
						"a width below the cap keys a prefix of a value the boundary accepted whole",
					key, term.Width, bound.cap)
			}
		})
	}
	require.NotZero(t, declared, "no type declares an occurrence term; the dedup key lost its occurrence half")
}

// PSY-1990: the cap exists to bound the value POSTGRES INDEXES, and SQL's trim
// is not Go's. This pins the property the index depends on — an ACCEPTED
// payload's name term is within the cap under either trim — rather than
// restating the check. It is not a tautology: a cap that started measuring the
// TRIMMED value would accept the U+3000-padded fixture below, and the SQL-trimmed
// length of that value is kilobytes, so this fails on exactly the regression it
// exists to catch.
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
