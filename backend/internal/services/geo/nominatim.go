// Street-address geocoding via the public Nominatim/OpenStreetMap API.
//
// This is a NEW sibling of the offline GeoNames geocoder in geo.go, not a
// replacement: the offline geocoder resolves (city, state, country) to a city
// centroid + timezone + metro with no network, and scenes/timezone derivation
// depend on it. Nominatim resolves a full STREET address to street-level
// coordinates, which requires a network call — so it is rate-limited, retried,
// and always optional for callers (a venue write must never block on it).
//
// Usage-policy compliance (https://operations.osmfoundation.org/policies/nominatim/):
// at most one request per second, enforced process-wide by the shared client
// from DefaultNominatim; a custom identifying User-Agent with a contact
// address; no API key.
package geo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Street-geocode precision labels stored in venues.geocode_precision.
const (
	// PrecisionRooftop: the hit is a concrete OSM feature at the address — a
	// building, an address node, an amenity/shop/venue POI.
	PrecisionRooftop = "rooftop"
	// PrecisionInterpolated: the point was interpolated along a road from
	// house-number ranges, or the match is road-level.
	PrecisionInterpolated = "interpolated"
	// PrecisionCity: only a locality-level match — no more precise than the
	// existing offline city-centroid columns.
	PrecisionCity = "city"
)

// AddressQuery is a structured street-address lookup. Street carries the house
// number + street name exactly as stored in venues.address; the remaining
// fields scope the search so a common street name resolves in the right place.
type AddressQuery struct {
	Street  string
	City    string
	State   string
	Zipcode string
	Country string
}

// Key is the canonical "what was geocoded" string: the non-empty components
// joined in a fixed order. It is stored in venues.geocoded_address so an
// unchanged address is skipped on backfill re-runs, and a stale geocode (any
// component changed since it was produced) is detectable by comparison.
func (q AddressQuery) Key() string {
	parts := make([]string, 0, 5)
	for _, p := range []string{q.Street, q.City, q.State, q.Zipcode, q.Country} {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, ", ")
}

// AddressResult is a resolved street-address hit.
type AddressResult struct {
	Latitude  float64
	Longitude float64
	Precision string // one of the Precision* constants
}

// AddressGeocoder resolves a street address to coordinates. It is a narrow
// interface so callers can stub it in tests and a future provider swap touches
// one implementation. ok=false with a nil error is a MISS (the address simply
// doesn't resolve); a non-nil error is a transport/service failure that a later
// retry (e.g. the backfill CLI) may succeed on.
type AddressGeocoder interface {
	GeocodeAddress(ctx context.Context, q AddressQuery) (AddressResult, bool, error)
}

const (
	nominatimDefaultBaseURL = "https://nominatim.openstreetmap.org"

	// EnvNominatimBaseURL overrides the Nominatim endpoint — a self-hosted
	// instance, or a local stub in tests/manual verification.
	EnvNominatimBaseURL = "NOMINATIM_BASE_URL"

	// nominatimUserAgent identifies the application per the OSM usage policy
	// (stock library User-Agents are rejected); the address is the contact
	// channel the policy asks for.
	nominatimUserAgent = "PsychicHomily/1.0 (https://psychichomily.com; noreply@psychichomily.com)"

	// nominatimMinInterval spaces requests to stay under the policy's absolute
	// maximum of 1 request/second, with margin for clock skew.
	nominatimMinInterval = 1100 * time.Millisecond

	nominatimMaxAttempts = 3
)

// NominatimClient implements AddressGeocoder against a Nominatim server.
// The zero value is not usable; construct via NewNominatimClient or share the
// process-wide DefaultNominatim.
type NominatimClient struct {
	httpClient  *http.Client
	baseURL     string
	userAgent   string
	minInterval time.Duration

	// mu serializes requests (including the pre-request wait) so the
	// 1-request/second budget holds across all goroutines sharing the client.
	mu       sync.Mutex
	lastCall time.Time
}

// NewNominatimClient returns a client for the given base URL (the public
// endpoint when empty). Callers inside the server should prefer
// DefaultNominatim so the rate limit is enforced process-wide.
func NewNominatimClient(baseURL string) *NominatimClient {
	if baseURL == "" {
		baseURL = nominatimDefaultBaseURL
	}
	return &NominatimClient{
		httpClient:  &http.Client{Timeout: 10 * time.Second},
		baseURL:     strings.TrimRight(baseURL, "/"),
		userAgent:   nominatimUserAgent,
		minInterval: nominatimMinInterval,
	}
}

var (
	nominatimOnce    sync.Once
	nominatimDefault *NominatimClient
)

// DefaultNominatim returns the process-wide shared client, honoring
// NOMINATIM_BASE_URL on first use. All in-process callers MUST share this
// instance: the usage policy's 1 req/s budget is per application, and the
// limiter lives on the client.
func DefaultNominatim() *NominatimClient {
	nominatimOnce.Do(func() {
		nominatimDefault = NewNominatimClient(os.Getenv(EnvNominatimBaseURL))
	})
	return nominatimDefault
}

// GeocodeAddress resolves the query via Nominatim's structured search.
// Rate-limited, and retried (with growing backoff) on 429/5xx/transport
// errors. An empty Street is a no-op miss — there is nothing street-level to
// resolve.
func (c *NominatimClient) GeocodeAddress(ctx context.Context, q AddressQuery) (AddressResult, bool, error) {
	street := strings.TrimSpace(q.Street)
	if street == "" {
		return AddressResult{}, false, nil
	}

	params := url.Values{}
	params.Set("format", "jsonv2")
	params.Set("limit", "1")
	params.Set("addressdetails", "0")
	params.Set("street", street)
	for name, val := range map[string]string{
		"city":       q.City,
		"state":      q.State,
		"postalcode": q.Zipcode,
		"country":    q.Country,
	} {
		if v := strings.TrimSpace(val); v != "" {
			params.Set(name, v)
		}
	}
	endpoint := c.baseURL + "/search?" + params.Encode()

	var lastErr error
	for attempt := 1; attempt <= nominatimMaxAttempts; attempt++ {
		if attempt > 1 {
			// Extra backoff on top of the per-request spacing: 2s, then 4s.
			backoff := time.Duration(attempt-1) * 2 * time.Second
			select {
			case <-ctx.Done():
				return AddressResult{}, false, ctx.Err()
			case <-time.After(backoff):
			}
		}
		res, ok, retryable, err := c.doSearch(ctx, endpoint)
		if err == nil {
			return res, ok, nil
		}
		lastErr = err
		if !retryable {
			break
		}
	}
	return AddressResult{}, false, lastErr
}

// nominatimResult is the subset of a jsonv2 search row the precision mapping
// needs. lat/lon are strings in the Nominatim wire format.
type nominatimResult struct {
	Lat         string `json:"lat"`
	Lon         string `json:"lon"`
	OSMType     string `json:"osm_type"`
	Category    string `json:"category"` // jsonv2 name for v1's "class"
	Type        string `json:"type"`
	AddressType string `json:"addresstype"`
}

// doSearch performs one rate-limited request. The mutex is held across the
// pre-request wait AND the round trip so concurrent callers cannot exceed the
// request budget. retryable reports whether the failure class is worth another
// attempt (429/5xx/transport), as opposed to a caller/parse problem.
func (c *NominatimClient) doSearch(ctx context.Context, endpoint string) (res AddressResult, ok bool, retryable bool, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if wait := c.minInterval - time.Since(c.lastCall); wait > 0 {
		select {
		case <-ctx.Done():
			return AddressResult{}, false, false, ctx.Err()
		case <-time.After(wait):
		}
	}
	c.lastCall = time.Now()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return AddressResult{}, false, false, fmt.Errorf("nominatim: build request: %w", err)
	}
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return AddressResult{}, false, false, ctx.Err()
		}
		return AddressResult{}, false, true, fmt.Errorf("nominatim: request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch {
	case resp.StatusCode == http.StatusOK:
		// fall through to parse
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		return AddressResult{}, false, true, fmt.Errorf("nominatim: status %d", resp.StatusCode)
	default:
		return AddressResult{}, false, false, fmt.Errorf("nominatim: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return AddressResult{}, false, true, fmt.Errorf("nominatim: read body: %w", err)
	}
	var rows []nominatimResult
	if err := json.Unmarshal(body, &rows); err != nil {
		return AddressResult{}, false, false, fmt.Errorf("nominatim: parse body: %w", err)
	}
	if len(rows) == 0 {
		return AddressResult{}, false, false, nil // clean miss
	}

	r := rows[0]
	lat, latErr := strconv.ParseFloat(r.Lat, 64)
	lng, lngErr := strconv.ParseFloat(r.Lon, 64)
	if latErr != nil || lngErr != nil {
		return AddressResult{}, false, false, fmt.Errorf("nominatim: unparseable coordinates %q,%q", r.Lat, r.Lon)
	}
	// Defensive bound check at the trust boundary — an out-of-range value
	// would also violate the numeric(9,6) columns.
	if lat < -90 || lat > 90 || lng < -180 || lng > 180 {
		return AddressResult{}, false, false, fmt.Errorf("nominatim: coordinates out of range %f,%f", lat, lng)
	}

	return AddressResult{Latitude: lat, Longitude: lng, Precision: precisionForResult(r)}, true, false, nil
}

// nominatimCityLevelTypes are addresstype values that locate no better than a
// locality — the offline city-centroid columns already give that precision, so
// such a hit is labeled PrecisionCity.
var nominatimCityLevelTypes = map[string]bool{
	"city": true, "town": true, "village": true, "hamlet": true,
	"municipality": true, "suburb": true, "neighbourhood": true,
	"quarter": true, "borough": true, "city_district": true, "district": true,
	"county": true, "state": true, "state_district": true, "region": true,
	"province": true, "postcode": true, "country": true,
}

// precisionForResult maps a Nominatim hit to the stored precision label:
//   - a locality-level addresstype → city;
//   - a road-level match, or Nominatim's house-number interpolation (category
//     "place", type "house" on a synthetic WAY — a real address point is a
//     node) → interpolated;
//   - anything else is a concrete feature at the address (building, address
//     node, POI) → rooftop.
func precisionForResult(r nominatimResult) string {
	switch {
	case nominatimCityLevelTypes[r.AddressType]:
		return PrecisionCity
	case r.AddressType == "road" || r.Category == "highway":
		return PrecisionInterpolated
	case r.Category == "place" && r.Type == "house" && r.OSMType != "node":
		return PrecisionInterpolated
	default:
		return PrecisionRooftop
	}
}
