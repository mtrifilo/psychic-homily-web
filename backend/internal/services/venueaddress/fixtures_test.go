package venueaddress

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/net/html"

	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/geo"
)

// addressFixture is one recorded venue page and the street it prints for the
// named venue. Expected lists every acceptable spelling of that street as the
// page prints it (empty: the page prints no usable street for the venue).
// Structured marks a page whose schema.org data carries the address, so no AI
// call is needed; every other page must be decided by the AI path.
type addressFixture struct {
	Slug  string     `json:"slug"`
	Page  string     `json:"page"`
	URL   string     `json:"url"`
	Kind  SourceKind `json:"kind"`
	Venue struct {
		Name  string `json:"name"`
		City  string `json:"city"`
		State string `json:"state"`
	} `json:"venue"`
	Expected   []string `json:"expected"`
	Structured bool     `json:"structured"`
}

func (f addressFixture) venue() Venue {
	return Venue{Name: f.Venue.Name, City: f.Venue.City, State: f.Venue.State}
}

// loadAddressFixtures reads the recorded page fixtures.
func loadAddressFixtures(t *testing.T) []addressFixture {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "venue_address_fixtures.json"))
	if err != nil {
		t.Fatalf("read fixtures: %v", err)
	}
	var fixtures []addressFixture
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatalf("parse fixtures: %v", err)
	}
	if len(fixtures) < 10 {
		t.Fatalf("want at least 10 recorded fixtures, have %d", len(fixtures))
	}
	return fixtures
}

func (f addressFixture) html(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", f.Page))
	if err != nil {
		t.Fatalf("%s: read page: %v", f.Slug, err)
	}
	return b
}

func sameStreet(got string, want []string) bool {
	for _, w := range want {
		if geo.FoldPlaceName(got) == geo.FoldPlaceName(w) {
			return true
		}
	}
	return false
}

// TestFixtures_StructuredExtraction pins which pages the schema.org reader
// decides on its own, and that it decides them correctly; every other page
// must reach the AI path with nothing accepted beforehand.
func TestFixtures_StructuredExtraction(t *testing.T) {
	for _, f := range loadAddressFixtures(t) {
		t.Run(f.Slug, func(t *testing.T) {
			doc, err := html.Parse(bytes.NewReader(f.html(t)))
			if err != nil {
				t.Fatal(err)
			}
			c, ok, why := chooseCandidate(structuredCandidates(doc), f.venue(), f.Kind)
			if f.Structured {
				if !ok || !sameStreet(c.Street, f.Expected) {
					t.Fatalf("structured = %q ok=%v (%s), want one of %q", c.Street, ok, why, f.Expected)
				}
				return
			}
			if ok {
				t.Fatalf("structured unexpectedly accepted %q; this page is an AI-path fixture", c.Street)
			}
		})
	}
}

// TestFixtures_ExpectedStreetIsPrinted proves each expected street survives
// the visible-text reduction the AI path and its verbatim guard both read, so
// a correct model answer is never rejected as "not printed on the page".
func TestFixtures_ExpectedStreetIsPrinted(t *testing.T) {
	for _, f := range loadAddressFixtures(t) {
		if len(f.Expected) == 0 {
			continue
		}
		t.Run(f.Slug, func(t *testing.T) {
			doc, err := html.Parse(bytes.NewReader(f.html(t)))
			if err != nil {
				t.Fatal(err)
			}
			text := trimForPrompt(visibleText(doc), promptHeadChars, promptTailChars)
			for _, want := range f.Expected {
				if appearsIn(want, text) {
					return
				}
			}
			t.Fatalf("none of %q appears in the prompt text", f.Expected)
		})
	}
}

// fixturePages serves recorded fixture pages by URL.
type fixturePages map[string][]byte

func (p fixturePages) Fetch(_ context.Context, rawURL string) (*Page, error) {
	b, ok := p[rawURL]
	if !ok {
		return nil, ErrUnavailable
	}
	return &Page{URL: rawURL, HTML: b}, nil
}

// answerAI returns a fixed extraction, standing in for the model.
type answerAI struct {
	street string
	city   string
	calls  int
}

func (a *answerAI) ExtractVenueAddress(_ context.Context, _ contracts.VenueAddressExtractionRequest) (*contracts.VenueAddressExtraction, error) {
	a.calls++
	if a.street == "" {
		return &contracts.VenueAddressExtraction{}, nil
	}
	return &contracts.VenueAddressExtraction{Found: true, Street: a.street, City: a.city}, nil
}

// TestFixtures_FinderAcceptsTheCorrectAnswer runs the whole page path over
// every fixture with a model that answers correctly: structured pages never
// call it, and every AI page accepts the correct street.
func TestFixtures_FinderAcceptsTheCorrectAnswer(t *testing.T) {
	for _, f := range loadAddressFixtures(t) {
		t.Run(f.Slug, func(t *testing.T) {
			ai := &answerAI{}
			if len(f.Expected) > 0 {
				ai.street = f.Expected[0]
			}
			finder := NewFinder(fixturePages{f.URL: f.html(t)}, ai)
			res, err := finder.Find(context.Background(), f.venue(), []Source{{URL: f.URL, Kind: f.Kind}})
			if err != nil {
				t.Fatalf("Find: %v", err)
			}
			if len(f.Expected) == 0 {
				if res.Found {
					t.Fatalf("found %q on a page that prints no usable street", res.Street)
				}
				return
			}
			if !res.Found || !sameStreet(res.Street, f.Expected) {
				t.Fatalf("Find = %q found=%v notes=%v, want one of %q", res.Street, res.Found, res.Notes, f.Expected)
			}
			if f.Structured && ai.calls != 0 {
				t.Fatalf("structured page called the AI %d times", ai.calls)
			}
			if !f.Structured && res.Method != MethodAI {
				t.Fatalf("method = %q, want %q", res.Method, MethodAI)
			}
		})
	}
}

// failingPages fails every fetch the same way.
type failingPages struct{ err error }

func (p failingPages) Fetch(context.Context, string) (*Page, error) { return nil, p.err }

// failingAI fails every extraction call.
type failingAI struct{}

func (failingAI) ExtractVenueAddress(context.Context, contracts.VenueAddressExtractionRequest) (*contracts.VenueAddressExtraction, error) {
	return nil, errors.New("anthropic API error (status 529)")
}

// TestFinder_TransientFailuresAreErrorsAndDefinitiveOnesAreMisses pins the
// contract the backfill relies on: a transient fetch or AI failure makes Find
// return an error (never recorded), and a definitive one is a clean miss.
func TestFinder_TransientFailuresAreErrorsAndDefinitiveOnesAreMisses(t *testing.T) {
	v := Venue{Name: "Lincoln Hall", City: "Chicago", State: "IL"}
	src := []Source{{URL: "https://lh.example", Kind: SourceWebsite}}

	if _, err := NewFinder(failingPages{errors.New("request failed: timeout")}, nil).Find(context.Background(), v, src); err == nil {
		t.Fatal("a transient fetch failure must be an error")
	}
	res, err := NewFinder(failingPages{fmt.Errorf("%w: status 404", ErrUnavailable)}, nil).Find(context.Background(), v, src)
	if err != nil || res.Found {
		t.Fatalf("a definitive failure is a clean miss: res=%+v err=%v", res, err)
	}
	pages := fixturePages{"https://lh.example": []byte("<p>Lincoln Hall, 2424 N Lincoln Ave</p>")}
	if _, err := NewFinder(pages, failingAI{}).Find(context.Background(), v, src); err == nil {
		t.Fatal("a failed AI call must be an error, not a miss")
	}
}

// TestFixtures_FinderRejectsAnUnprintedAnswer is the hallucination guard: a
// model answer that is not on the page is refused even when it looks right.
func TestFixtures_FinderRejectsAnUnprintedAnswer(t *testing.T) {
	for _, f := range loadAddressFixtures(t) {
		if f.Structured {
			continue
		}
		t.Run(f.Slug, func(t *testing.T) {
			ai := &answerAI{street: "9999 Invented Boulevard"}
			finder := NewFinder(fixturePages{f.URL: f.html(t)}, ai)
			res, err := finder.Find(context.Background(), f.venue(), []Source{{URL: f.URL, Kind: f.Kind}})
			if err != nil {
				t.Fatalf("Find: %v", err)
			}
			if res.Found {
				t.Fatalf("accepted an address the page does not print: %q", res.Street)
			}
		})
	}
}
