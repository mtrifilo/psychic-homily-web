package catalog

import (
	"context"
	"errors"
	"testing"
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
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

// TestStreetGeocodeQueryExpandsTheStoredAddress covers the single line that
// actually fixes the reported bug.
//
// Without it the whole suite passes with the expansion deleted: every other test
// here hand-builds a query with the expanded street already in it, so they verify
// the FALLBACK but never that anything expands in the first place. This asserts
// the wiring from the stored venue row through to the outbound query.
func TestStreetGeocodeQueryExpandsTheStoredAddress(t *testing.T) {
	addr := rawMLK
	zip := "30303"
	country := "US"
	v := &catalogm.Venue{
		Address: &addr,
		City:    "Atlanta",
		State:   "GA",
		Zipcode: &zip,
		Country: &country,
	}

	q := streetGeocodeQuery(v)

	if q.Street != expandedMLK {
		t.Fatalf("street = %q, want the expanded form %q", q.Street, expandedMLK)
	}
	// The expansion must reach Key() too: a clean miss is memoized against the
	// key, so keying on the raw address would mean a later table change never
	// re-queries a venue already memoized as a miss.
	if got := q.Key(); got != expandedMLK+", Atlanta, GA, 30303, US" {
		t.Fatalf("key = %q — the expansion must flow into the freshness key", got)
	}
	// Scoping fields must survive untouched, or a common street resolves in the
	// wrong city.
	if q.City != "Atlanta" || q.State != "GA" || q.Zipcode != "30303" || q.Country != "US" {
		t.Fatalf("scoping fields changed: %+v", q)
	}
}

// TestStreetGeocodeQueryLeavesOrdinaryAddressesAlone: the vast majority of
// venues contain nothing expandable, and their key must not churn — a changed
// key re-geocodes the venue against a rate-limited third party.
func TestStreetGeocodeQueryLeavesOrdinaryAddressesAlone(t *testing.T) {
	addr := "123 Main St"
	v := &catalogm.Venue{Address: &addr, City: "Austin", State: "TX"}

	q := streetGeocodeQuery(v)

	if q.Street != "123 Main St" {
		t.Fatalf("street = %q — an ordinary address must be untouched (and St is Street, not Saint)", q.Street)
	}
}

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
// TestExpandedFormHitsCostsTwoRequests: venue 128's shape — the stored address
// misses, the expanded form hits. Two requests, and that is the price of the fix.
func TestExpandedFormHitsCostsTwoRequests(t *testing.T) {
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
	if len(g.asked) != 2 || g.asked[0] != rawMLK || g.asked[1] != expandedMLK {
		t.Fatalf("expected raw-then-expanded, got %v", g.asked)
	}
}

// TestRawFormWinsWhenBothResolve is the case that makes the ORDER load-bearing,
// and the one an expanded-first implementation silently fails.
//
// For an address like "3800 MLK Jr Blvd" the expanded form may also resolve — but
// to the multi-mile named ROAD rather than the address node. Expanded-first would
// take that hit, skip the fallback, and overwrite correct rooftop coordinates
// with a worse interpolated point, then memoize it under the new key. Raw-first
// guarantees anything resolving today resolves identically tomorrow.
func TestRawFormWinsWhenBothResolve(t *testing.T) {
	g := &recordingGeocoder{hits: map[string]geo.AddressResult{
		rawMLK:      {Latitude: 33.75, Longitude: -84.39, Precision: "rooftop"},
		expandedMLK: {Latitude: 33.80, Longitude: -84.50, Precision: "interpolated"},
	}}

	res, ok, err := geocodeExpandingAbbreviations(context.Background(), g, mlkQuery(), rawMLK)

	if err != nil || !ok {
		t.Fatalf("expected a hit: ok=%v err=%v", ok, err)
	}
	if res.Precision != "rooftop" {
		t.Fatalf("precision = %q — the stored address must win when both resolve, "+
			"or this change degrades venues that already work", res.Precision)
	}
	if len(g.asked) != 1 {
		t.Fatalf("a hit on the stored address must not cost a second request, got %v", g.asked)
	}
}

// TestFallsBackToTheRawStreet covers the case that made "expand only" unsafe:
// OSM stores some streets abbreviated, so a one-sided expansion just moves the
// miss to a different street.
func TestStoredAddressHitCostsOneRequest(t *testing.T) {
	g := &recordingGeocoder{hits: map[string]geo.AddressResult{
		rawMLK: {Latitude: 33.75, Longitude: -84.39, Precision: "rooftop"},
	}}

	_, ok, err := geocodeExpandingAbbreviations(context.Background(), g, mlkQuery(), rawMLK)

	if err != nil || !ok {
		t.Fatalf("expected the stored address to hit: ok=%v err=%v", ok, err)
	}
	if len(g.asked) != 1 || g.asked[0] != rawMLK {
		t.Fatalf("a street OSM stores abbreviated must cost one request, got %v", g.asked)
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

// TestDeclinedRetryReportsAnErrorNotAMiss guards the subtle half of raw-first
// ordering.
//
// With the stored address tried first, the EXPANDED lookup is the one that
// actually fixes the venue. If the budget guard skipped it and returned the clean
// miss, the caller would memoize that miss under the expanded key — and
// streetGeocodeAttempted would then make the sweep skip the venue PERMANENTLY,
// with no error and no alert. An incomplete attempt is not a miss; reporting an
// error keeps the venue retryable, because errors are never memoized.
func TestDeclinedRetryReportsAnErrorNotAMiss(t *testing.T) {
	g := &recordingGeocoder{hits: map[string]geo.AddressResult{
		expandedMLK: {Latitude: 33.75, Longitude: -84.39, Precision: "rooftop"},
	}}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, ok, err := geocodeExpandingAbbreviations(ctx, g, mlkQuery(), rawMLK)

	if ok {
		t.Fatal("precondition: only the expanded form hits, so the stored-address attempt must miss")
	}
	if err == nil {
		t.Fatal("a declined retry must report an ERROR — returning the miss would memoize it " +
			"under the expanded key and retire the venue from the sweep permanently")
	}
	if len(g.asked) != 1 {
		t.Fatalf("the retry must be declined with the budget nearly spent, got %d requests: %v", len(g.asked), g.asked)
	}
}

// TestFallbackProceedsWithAmpleBudget is the counterpart — the guard must not
// swallow the retry in the normal case, or the fix never fires.
func TestFallbackProceedsWithAmpleBudget(t *testing.T) {
	g := &recordingGeocoder{hits: map[string]geo.AddressResult{
		expandedMLK: {Latitude: 33.75, Longitude: -84.39, Precision: "rooftop"},
	}}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	_, ok, err := geocodeExpandingAbbreviations(ctx, g, mlkQuery(), rawMLK)

	if err != nil || !ok {
		t.Fatalf("with ample budget the expanded retry must run: ok=%v err=%v", ok, err)
	}
	if len(g.asked) != 2 {
		t.Fatalf("expected both forms tried, got %v", g.asked)
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
