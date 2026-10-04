package venueaddress_test

import (
	"context"
	"os"
	"testing"

	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/services/pipeline"
	"psychic-homily-backend/internal/services/venueaddress"
)

// TestAddressExtractionEval scores the real AI extraction path against every
// recorded fixture. It calls the Anthropic API, so it runs only on request:
//
//	VENUE_ADDRESS_EVAL=1 ANTHROPIC_API_KEY=... go test ./internal/services/venueaddress/ -run TestAddressExtractionEval -v
//
// Like the /ingest extraction evals it reports a score rather than gating on
// one: a fixture that misses is logged, and the run fails only when the
// extractor returns an address the fixture says is wrong.
func TestAddressExtractionEval(t *testing.T) {
	if os.Getenv("VENUE_ADDRESS_EVAL") == "" {
		t.Skip("set VENUE_ADDRESS_EVAL=1 and ANTHROPIC_API_KEY to run the address extraction eval")
	}
	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		t.Fatal("VENUE_ADDRESS_EVAL is set but ANTHROPIC_API_KEY is empty")
	}
	cfg := &config.Config{}
	cfg.Anthropic.APIKey = key
	ai := pipeline.NewExtractionService(nil, cfg, nil, nil)

	fixtures := venueaddress.LoadEvalFixtures(t)
	correct, missed, wrong := 0, 0, 0
	for _, f := range fixtures {
		finder := venueaddress.NewFinder(evalPages{f.URL: f.HTML}, ai)
		res, err := finder.Find(context.Background(), f.Venue, []venueaddress.Source{{URL: f.URL, Kind: f.Kind}})
		switch {
		case err != nil:
			missed++
			t.Logf("ERROR   %-28s %v", f.Slug, err)
		case len(f.Expected) == 0 && !res.Found:
			correct++
			t.Logf("correct %-28s (no address)", f.Slug)
		case len(f.Expected) == 0:
			wrong++
			t.Errorf("WRONG   %-28s found %q on a page with no usable street", f.Slug, res.Street)
		case !res.Found:
			missed++
			t.Logf("missed  %-28s want %q; notes %v", f.Slug, f.Expected, res.Notes)
		case venueaddress.SameStreet(res.Street, f.Expected):
			correct++
			t.Logf("correct %-28s %q via %s", f.Slug, res.Street, res.Method)
		default:
			wrong++
			t.Errorf("WRONG   %-28s got %q, want %q", f.Slug, res.Street, f.Expected)
		}
	}
	t.Logf("score: %d/%d correct, %d missed, %d wrong", correct, len(fixtures), missed, wrong)
}

type evalPages map[string][]byte

func (p evalPages) Fetch(_ context.Context, rawURL string) (*venueaddress.Page, error) {
	b, ok := p[rawURL]
	if !ok {
		return nil, venueaddress.ErrUnavailable
	}
	return &venueaddress.Page{URL: rawURL, HTML: b}, nil
}
