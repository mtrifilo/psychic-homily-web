package community

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

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

			// A multi-kilobyte name is what blows the btree index-row limit.
			err = ValidateEntityRequestPayload(entityType, namedPayload(t, entityType, strings.Repeat("a", 2600)))
			require.Error(t, err)
			assert.Contains(t, err.Error(), wantErr)

			// CHARACTERS, not bytes. The cap mirrors a VARCHAR(255), and
			// fulfillEntity re-runs this validation over the STORED payload after
			// the decide path has claimed the row, so a cap stricter than the column
			// turns an already-queued request into an orphan.
			multibyte := strings.Repeat("東", maxRequestNameLen)
			require.Greater(t, len(multibyte), maxRequestNameLen,
				"the fixture must exceed the cap in BYTES, or it is not testing the unit")
			assert.NoError(t, ValidateEntityRequestPayload(entityType, namedPayload(t, entityType, multibyte)),
				"a name the column would hold must not be refused for being multi-byte")

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
	overCity.City = strings.Repeat("c", maxRequestVenueCityLen+1)
	raw, err := MarshalPayload(overCity)
	require.NoError(t, err)
	err = ValidateEntityRequestPayload(EntityRequestVenue, raw)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "city must be "+strconv.Itoa(maxRequestVenueCityLen)+" characters or fewer")

	overState := base
	overState.State = strings.Repeat("s", maxRequestVenueStateLen+1)
	raw, err = MarshalPayload(overState)
	require.NoError(t, err)
	err = ValidateEntityRequestPayload(EntityRequestVenue, raw)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "state must be "+strconv.Itoa(maxRequestVenueStateLen)+" characters or fewer")

	atCap := base
	atCap.City = strings.Repeat("c", maxRequestVenueCityLen)
	atCap.State = strings.Repeat("s", maxRequestVenueStateLen)
	raw, err = MarshalPayload(atCap)
	require.NoError(t, err)
	assert.NoError(t, ValidateEntityRequestPayload(EntityRequestVenue, raw),
		"values exactly at the caps are legal, and the index must not truncate them")

	// CHARACTERS, not bytes, on both halves. The cap has to match the VARCHAR it
	// mirrors, because fulfillEntity re-runs this validation over the STORED
	// payload after the decide path has claimed the row: a cap stricter than the
	// column turns an already-queued request into an approved-but-unfulfilled
	// orphan no decide call can re-process.
	multibyte := base
	multibyte.City = strings.Repeat("東", maxRequestVenueCityLen)
	multibyte.State = strings.Repeat("都", maxRequestVenueStateLen)
	raw, err = MarshalPayload(multibyte)
	require.NoError(t, err)
	require.Greater(t, len(multibyte.City), maxRequestVenueCityLen,
		"the fixture must exceed the cap in BYTES, or it is not testing the unit")
	assert.NoError(t, ValidateEntityRequestPayload(EntityRequestVenue, raw),
		"a city the column would hold must not be refused for being multi-byte")
}

// PSY-1989: the festival occurrence term keeps four digits, so edition_year must
// be bounded to four digits at the boundary. Without that, 20261 truncates to
// "2026" and a mistyped edition DESTROYS the real one on resubmission, which is
// the collision this key exists to remove. The width and the bound are the pair.
func TestFestivalEditionYearIsBoundedToTheTermWidth(t *testing.T) {
	term := DedupOccurrenceTermFor(EntityRequestFestival)
	require.Equal(t, len(strconv.Itoa(maxRequestEditionYear)), term.Width,
		"the edition_year bound and the occurrence term's width must have the same "+
			"number of digits, or a legal year is truncated into another year's bucket")

	base := FestivalRequestPayload{
		Name: "Psycho Las Vegas", StartDate: "2026-08-14", EndDate: "2026-08-16",
	}

	over := base
	over.EditionYear = maxRequestEditionYear + 1
	raw, err := MarshalPayload(over)
	require.NoError(t, err)
	err = ValidateEntityRequestPayload(EntityRequestFestival, raw)
	require.Error(t, err, "a five-digit edition year shares a dedup bucket with a four-digit one")
	assert.Contains(t, err.Error(), "edition_year must be between 0 and "+strconv.Itoa(maxRequestEditionYear))

	negative := base
	negative.EditionYear = -1
	raw, err = MarshalPayload(negative)
	require.NoError(t, err)
	assert.Error(t, ValidateEntityRequestPayload(EntityRequestFestival, raw))

	for _, year := range []int{0, 1, maxRequestEditionYear} {
		ok := base
		ok.EditionYear = year
		raw, err = MarshalPayload(ok)
		require.NoError(t, err)
		assert.NoErrorf(t, ValidateEntityRequestPayload(EntityRequestFestival, raw),
			"edition_year %d is within the bound", year)
	}
}

// PSY-1990: the same Unicode-whitespace bypass the name cap closes was open one
// function over. ValidateShowBill measures the TRIMMED act name, and Go's trim
// strips U+3000 while the stored payload keeps it, so a padded name reached
// artists.name VARCHAR(255) and failed at INSERT after the decide path had
// claimed the row.
func TestValidateShowPayloadArtists_BoundsTheStoredName(t *testing.T) {
	padded := "Boris" + strings.Repeat("　", 1000)
	require.Equal(t, "Boris", strings.TrimSpace(padded),
		"the fixture must LOOK short to Go, or it is not testing the bypass")

	err := ValidateShowPayloadArtists([]ShowRequestArtist{{Name: padded}})
	require.Error(t, err, "a Unicode-padded act name must not pass the bill cap")
	assert.Contains(t, err.Error(), "must be "+strconv.Itoa(MaxShowRequestArtistNameLen)+" characters or fewer")

	// Ordinary padding is still fine, so the new bound refuses nothing a trim would
	// have accepted. It deliberately does NOT widen ValidateShowBill's own cap,
	// which measures the trimmed name in BYTES and is shared with the admin approve
	// path; this check only closes the gap between the raw value and that trim.
	assert.NoError(t, ValidateShowPayloadArtists([]ShowRequestArtist{{Name: "  Boris  "}}))
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
	"event_date": {cap: maxRequestDateLen},
	"city":       {cap: maxRequestVenueCityLen},
	// edition_year and start_date are truncated to the four digits of a year, so
	// the width is the meaning rather than a safety bound. They still have to be
	// BOUNDED, or a value longer than the truncation shares a bucket with a
	// shorter one: maxRequestEditionYear and requireDate do that, and
	// TestFestivalEditionYearIsBoundedToTheTermWidth pins it.
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
		strings.Repeat("東", maxRequestNameLen),
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
				// CHARACTERS under either trim, because Postgres left() truncates in
				// characters too. The BYTE bound follows: a term of n characters is at
				// most 4n bytes, which is what keeps it inside the btree row limit.
				assert.LessOrEqual(t, utf8.RuneCountInString(sqlTrim(name)), maxRequestNameLen,
					"an ACCEPTED payload's SQL-trimmed name term exceeds the cap, "+
						"so the boundary does not bound what the index stores")
				assert.LessOrEqual(t, utf8.RuneCountInString(strings.TrimSpace(name)), maxRequestNameLen)
			}
			require.NotZero(t, accepted, "every candidate was refused; the assertions above ran on nothing")
		})
	}
}
