package catalog

import (
	"context"
	"errors"
	"testing"

	"psychic-homily-backend/internal/services/geo"
)

// recordingGeocoder answers a fixed set of streets and records every street it
// was asked for, in order — so a test can assert not just the outcome but how
// many Nominatim requests it cost. That second part is the point: this path
// shares a 1 req/s budget that is a ToS obligation, not best-effort.
type recordingGeocoder struct {
	hits  map[string]geo.AddressResult
	asked []string
	err   error
}

func (g *recordingGeocoder) GeocodeAddress(_ context.Context, q geo.AddressQuery) (geo.AddressResult, bool, error) {
	g.asked = append(g.asked, q.Street)
	if g.err != nil {
		return geo.AddressResult{}, false, g.err
	}
	if res, ok := g.hits[q.Street]; ok {
		return res, true, nil
	}
	return geo.AddressResult{}, false, nil
}

const (
	rawMLK      = "75 M.L.K. Jr Dr SW"
	expandedMLK = "75 Martin Luther King Jr Dr SW"
)

func mlkQuery() geo.AddressQuery {
	return geo.AddressQuery{
		Street:  expandedMLK,
		City:    "Atlanta",
		State:   "GA",
		Country: "US",
	}
}

// TestExpandedFormHitsCostsOneRequest is the steady-state case, and the reason
// the fallback was acceptable at all: a hit must not spend a second request, or
// the whole catalog's re-geocode doubles against the shared rate limit.
func TestExpandedFormHitsCostsOneRequest(t *testing.T) {
	g := &recordingGeocoder{hits: map[string]geo.AddressResult{
		expandedMLK: {Latitude: 33.75, Longitude: -84.39, Precision: "rooftop"},
	}}

	res, ok, err := geocodeExpandingAbbreviations(context.Background(), g, mlkQuery(), rawMLK)

	if err != nil || !ok {
		t.Fatalf("expected a hit: ok=%v err=%v", ok, err)
	}
	if res.Precision != "rooftop" {
		t.Fatalf("precision = %q", res.Precision)
	}
	if len(g.asked) != 1 {
		t.Fatalf("a hit on the expanded form must cost exactly one request, got %d: %v", len(g.asked), g.asked)
	}
}

// TestFallsBackToTheRawStreet covers the case that made "expand only" unsafe:
// OSM stores some streets abbreviated, so a one-sided expansion just moves the
// miss to a different street.
func TestFallsBackToTheRawStreet(t *testing.T) {
	g := &recordingGeocoder{hits: map[string]geo.AddressResult{
		rawMLK: {Latitude: 33.75, Longitude: -84.39, Precision: "rooftop"},
	}}

	_, ok, err := geocodeExpandingAbbreviations(context.Background(), g, mlkQuery(), rawMLK)

	if err != nil || !ok {
		t.Fatalf("expected the raw-street retry to hit: ok=%v err=%v", ok, err)
	}
	if len(g.asked) != 2 || g.asked[0] != expandedMLK || g.asked[1] != rawMLK {
		t.Fatalf("expected expanded-then-raw, got %v", g.asked)
	}
}

// TestNoSecondRequestWhenNothingWasExpanded: the overwhelming majority of
// addresses contain no abbreviation at all, so the expanded and raw forms are
// identical. Retrying an identical query would be a pure waste of the budget.
func TestNoSecondRequestWhenNothingWasExpanded(t *testing.T) {
	q := geo.AddressQuery{Street: "1234 Elm Street", City: "Austin", State: "TX"}
	g := &recordingGeocoder{}

	_, ok, _ := geocodeExpandingAbbreviations(context.Background(), g, q, "1234 Elm Street")

	if ok {
		t.Fatal("precondition: this address should miss")
	}
	if len(g.asked) != 1 {
		t.Fatalf("an unexpanded address must never cost two requests, got %d: %v", len(g.asked), g.asked)
	}
}

// TestUnresolvableAddressStopsAfterBothForms bounds the failure path: two
// requests, then the caller memoizes the miss so it is not retried.
func TestUnresolvableAddressStopsAfterBothForms(t *testing.T) {
	g := &recordingGeocoder{}

	_, ok, err := geocodeExpandingAbbreviations(context.Background(), g, mlkQuery(), rawMLK)

	if ok || err != nil {
		t.Fatalf("expected a clean miss: ok=%v err=%v", ok, err)
	}
	if len(g.asked) != 2 {
		t.Fatalf("a genuine miss costs at most two requests, got %d: %v", len(g.asked), g.asked)
	}
}

// TestTransportErrorDoesNotBurnASecondRequest: an error is not a miss. Retrying
// the other form when Nominatim is unreachable doubles traffic against a service
// that is already failing, and the caller retries the whole venue next run.
func TestTransportErrorDoesNotBurnASecondRequest(t *testing.T) {
	g := &recordingGeocoder{err: errors.New("connection refused")}

	_, _, err := geocodeExpandingAbbreviations(context.Background(), g, mlkQuery(), rawMLK)

	if err == nil {
		t.Fatal("expected the transport error to propagate")
	}
	if len(g.asked) != 1 {
		t.Fatalf("an error must not trigger the fallback, got %d requests: %v", len(g.asked), g.asked)
	}
}
