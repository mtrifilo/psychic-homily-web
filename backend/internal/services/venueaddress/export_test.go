package venueaddress

import "testing"

// EvalFixture is a recorded fixture as the external AI eval reads it.
type EvalFixture struct {
	Slug     string
	URL      string
	Kind     SourceKind
	Venue    Venue
	Expected []string
	HTML     []byte
}

// LoadEvalFixtures exposes the recorded fixtures to the external eval test,
// which cannot live in this package because the real extractor's package
// depends on this one.
func LoadEvalFixtures(t *testing.T) []EvalFixture {
	var out []EvalFixture
	for _, f := range loadAddressFixtures(t) {
		out = append(out, EvalFixture{
			Slug: f.Slug, URL: f.URL, Kind: f.Kind, Venue: f.venue(),
			Expected: f.Expected, HTML: f.html(t),
		})
	}
	return out
}

// SameStreet compares a found street against the expected spellings.
func SameStreet(got string, want []string) bool { return sameStreet(got, want) }
