package geo

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// newTestClient returns a client pointed at the stub server with a tiny
// request interval so tests stay fast.
func newTestClient(baseURL string) *NominatimClient {
	c := NewNominatimClient(baseURL)
	c.minInterval = time.Millisecond
	return c
}

func TestAddressQueryKey(t *testing.T) {
	tests := []struct {
		name string
		q    AddressQuery
		want string
	}{
		{
			name: "all components",
			q:    AddressQuery{Street: "130 N Central Ave", City: "Phoenix", State: "AZ", Zipcode: "85004", Country: "USA"},
			want: "130 N Central Ave, Phoenix, AZ, 85004, USA",
		},
		{
			name: "empty components dropped",
			q:    AddressQuery{Street: "1 Main St", City: "London", State: "", Zipcode: "", Country: "UK"},
			want: "1 Main St, London, UK",
		},
		{
			name: "whitespace trimmed and dropped",
			q:    AddressQuery{Street: "  1 Main St ", City: " Phoenix", State: "  "},
			want: "1 Main St, Phoenix",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.q.Key(); got != tt.want {
				t.Errorf("Key() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNominatimGeocodeAddress_StructuredQueryAndParse(t *testing.T) {
	var gotQuery map[string]string
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		gotQuery = map[string]string{}
		for k := range r.URL.Query() {
			gotQuery[k] = r.URL.Query().Get(k)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"lat":"33.448227","lon":"-112.073069","osm_type":"way","category":"building","type":"yes","addresstype":"building"}]`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	res, ok, err := c.GeocodeAddress(context.Background(), AddressQuery{
		Street: "130 N Central Ave", City: "Phoenix", State: "AZ", Zipcode: "85004", Country: "USA",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected a hit")
	}
	if res.Latitude != 33.448227 || res.Longitude != -112.073069 {
		t.Errorf("coords = %f,%f", res.Latitude, res.Longitude)
	}
	if res.Precision != PrecisionRooftop {
		t.Errorf("precision = %q, want rooftop", res.Precision)
	}

	// Structured params per the Nominatim search API.
	want := map[string]string{
		"format": "jsonv2", "limit": "1", "addressdetails": "0",
		"street": "130 N Central Ave", "city": "Phoenix", "state": "AZ",
		"postalcode": "85004", "country": "USA",
	}
	for k, v := range want {
		if gotQuery[k] != v {
			t.Errorf("query param %s = %q, want %q", k, gotQuery[k], v)
		}
	}
	// OSM usage policy: identifying User-Agent, not a stock library UA.
	if gotUA != c.userAgent {
		t.Errorf("User-Agent = %q, want %q", gotUA, c.userAgent)
	}
	if !strings.HasPrefix(gotUA, "PsychicHomily/1.0 (") {
		t.Errorf("User-Agent %q must identify the application with a contact channel", gotUA)
	}
}

func TestNominatimUserAgent_ContactOverride(t *testing.T) {
	t.Setenv(EnvNominatimContact, "ops@example.com")
	if got := nominatimUserAgent(); got != "PsychicHomily/1.0 (ops@example.com)" {
		t.Errorf("nominatimUserAgent() = %q", got)
	}
	t.Setenv(EnvNominatimContact, "")
	if got := nominatimUserAgent(); got != "PsychicHomily/1.0 ("+nominatimDefaultContact+")" {
		t.Errorf("default nominatimUserAgent() = %q", got)
	}
}

func TestParseRetryAfter(t *testing.T) {
	tests := []struct {
		in   string
		want time.Duration
	}{
		{"", 0},
		{"garbage", 0},
		{"-3", 0},
		{"0", 0},
		{"5", 5 * time.Second},
		{" 7 ", 7 * time.Second},
		{"9999", nominatimMaxRetryAfter},
		{"Wed, 21 Oct 2026 07:28:00 GMT", 0}, // HTTP-date form unsupported → backoff wins
	}
	for _, tt := range tests {
		if got := parseRetryAfter(tt.in); got != tt.want {
			t.Errorf("parseRetryAfter(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestNominatimGeocodeAddress_CanceledWhileQueuedReturnsPromptly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	c.sem <- struct{}{} // simulate another caller mid-round-trip
	defer func() { <-c.sem }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	_, ok, err := c.GeocodeAddress(ctx, AddressQuery{Street: "1 Main St", City: "Phoenix"})
	if ok || err == nil {
		t.Fatalf("expected context error, got ok=%v err=%v", ok, err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("canceled caller waited %v behind the busy limiter; acquisition must be context-aware", elapsed)
	}
}

func TestNominatimGeocodeAddress_EmptyStreetIsMissWithoutRequest(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, ok, err := c.GeocodeAddress(context.Background(), AddressQuery{Street: "  ", City: "Phoenix"})
	if err != nil || ok {
		t.Fatalf("expected clean miss, got ok=%v err=%v", ok, err)
	}
	if called {
		t.Error("no request should be made for an empty street")
	}
}

func TestNominatimGeocodeAddress_NoResultsIsMiss(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, ok, err := c.GeocodeAddress(context.Background(), AddressQuery{Street: "999 Nowhere Ln", City: "Phoenix"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected miss")
	}
}

func TestNominatimGeocodeAddress_RetriesOn429ThenSucceeds(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`[{"lat":"51.5","lon":"-0.1","osm_type":"node","category":"place","type":"house","addresstype":"place"}]`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	// Short-circuit the retry backoff by using a context deadline generous
	// enough for the 2s backoff but bounding the test.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, ok, err := c.GeocodeAddress(ctx, AddressQuery{Street: "1 Main St", City: "London"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected a hit after retry")
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("calls = %d, want 2", got)
	}
	if res.Precision != PrecisionRooftop {
		t.Errorf("address node precision = %q, want rooftop", res.Precision)
	}
}

func TestNominatimGeocodeAddress_NonRetryableStatusFailsFast(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, ok, err := c.GeocodeAddress(context.Background(), AddressQuery{Street: "1 Main St", City: "London"})
	if err == nil || ok {
		t.Fatalf("expected error, got ok=%v err=%v", ok, err)
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("calls = %d, want 1 (403 must not be retried)", got)
	}
}

func TestNominatimGeocodeAddress_OutOfRangeCoordinatesRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"lat":"91.0","lon":"0.0","osm_type":"node","category":"place","type":"house","addresstype":"place"}]`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, ok, err := c.GeocodeAddress(context.Background(), AddressQuery{Street: "1 Main St"})
	if err == nil || ok {
		t.Fatalf("expected out-of-range error, got ok=%v err=%v", ok, err)
	}
}

func TestNominatimGeocodeAddress_EnforcesMinInterval(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	c.minInterval = 120 * time.Millisecond

	start := time.Now()
	for i := 0; i < 3; i++ {
		if _, _, err := c.GeocodeAddress(context.Background(), AddressQuery{Street: "1 Main St"}); err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
	}
	// Three requests need at least two full intervals between them.
	if elapsed := time.Since(start); elapsed < 240*time.Millisecond {
		t.Errorf("3 requests completed in %v; rate limiter not enforcing %v spacing", elapsed, c.minInterval)
	}
}

func TestPrecisionForResult(t *testing.T) {
	tests := []struct {
		name string
		r    nominatimResult
		want string
	}{
		{"building", nominatimResult{OSMType: "way", Category: "building", Type: "yes", AddressType: "building"}, PrecisionRooftop},
		{"amenity POI", nominatimResult{OSMType: "node", Category: "amenity", Type: "bar", AddressType: "amenity"}, PrecisionRooftop},
		{"address node", nominatimResult{OSMType: "node", Category: "place", Type: "house", AddressType: "place"}, PrecisionRooftop},
		{"interpolated house number", nominatimResult{OSMType: "way", Category: "place", Type: "house", AddressType: "place"}, PrecisionInterpolated},
		{"road match", nominatimResult{OSMType: "way", Category: "highway", Type: "residential", AddressType: "road"}, PrecisionInterpolated},
		{"city fallback", nominatimResult{OSMType: "relation", Category: "boundary", Type: "administrative", AddressType: "city"}, PrecisionCity},
		{"suburb fallback", nominatimResult{OSMType: "node", Category: "place", Type: "suburb", AddressType: "suburb"}, PrecisionCity},
		{"postcode fallback", nominatimResult{OSMType: "node", Category: "place", Type: "postcode", AddressType: "postcode"}, PrecisionCity},
		// place_rank guard: coarse addresstypes the map doesn't enumerate must
		// fail CLOSED to city, not open to rooftop.
		{"unlisted locality via rank", nominatimResult{OSMType: "node", Category: "place", Type: "locality", AddressType: "locality", PlaceRank: 22}, PrecisionCity},
		{"unlisted farm via rank", nominatimResult{OSMType: "node", Category: "place", Type: "farm", AddressType: "farm", PlaceRank: 25}, PrecisionCity},
		{"street-rank unknown type", nominatimResult{OSMType: "way", Category: "place", Type: "square", AddressType: "square", PlaceRank: 26}, PrecisionInterpolated},
		{"house-rank POI keeps rooftop", nominatimResult{OSMType: "node", Category: "amenity", Type: "bar", AddressType: "amenity", PlaceRank: 30}, PrecisionRooftop},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := precisionForResult(tt.r); got != tt.want {
				t.Errorf("precisionForResult(%+v) = %q, want %q", tt.r, got, tt.want)
			}
		})
	}
}

// A transport failure must not leak the request URL — net/http wraps it in
// *url.Error whose message embeds the full query string, which carries the
// venue's street address (potentially a private home address that callers
// deliberately keep out of logs).
func TestNominatimDoSearch_TransportErrorRedactsURL(t *testing.T) {
	// A listener that is closed immediately gives a deterministic
	// connection-refused address with no live server.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	const street = "742 Evergreen Terrace"
	c := newTestClient("http://" + addr)
	endpoint := c.baseURL + "/search?street=" + url.QueryEscape(street)
	_, ok, _, err := c.doSearch(context.Background(), endpoint)
	if ok || err == nil {
		t.Fatalf("expected transport error, got ok=%v err=%v", ok, err)
	}
	msg := err.Error()
	if strings.Contains(msg, "Evergreen") || strings.Contains(msg, url.QueryEscape(street)) || strings.Contains(msg, "street=") {
		t.Errorf("transport error leaks the request URL/address: %q", msg)
	}
	if !strings.Contains(msg, "refused") && !strings.Contains(msg, "connect") {
		t.Errorf("sanitized error should keep the underlying cause, got %q", msg)
	}
}

// The sanitizer must preserve error-chain semantics (errors.Is on the cause)
// while dropping the URL, and must pass non-url.Error values through.
func TestSanitizeTransportErr(t *testing.T) {
	cause := context.DeadlineExceeded
	ue := &url.Error{Op: "Get", URL: "https://nominatim.example/search?street=1+Private+Rd", Err: cause}
	got := sanitizeTransportErr(ue)
	if strings.Contains(got.Error(), "Private") || strings.Contains(got.Error(), "search?") {
		t.Errorf("sanitized error still contains the URL: %q", got.Error())
	}
	if !errors.Is(got, context.DeadlineExceeded) {
		t.Errorf("sanitized error lost the wrapped cause: %v", got)
	}
	plain := errors.New("boring")
	if sanitizeTransportErr(plain) != plain {
		t.Errorf("non-url.Error must pass through unchanged")
	}
}
