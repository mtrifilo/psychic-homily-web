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
// channel (override via NOMINATIM_CONTACT); no API key. The limiter is
// per-process — do NOT run the backfill CLI against the public endpoint while
// the live server is taking venue-write traffic, or the aggregate exceeds the
// budget (run the backfill off-hours or point it at a self-hosted instance
// via NOMINATIM_BASE_URL).
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

	// EnvNominatimContact overrides the contact channel embedded in the
	// User-Agent (email address or URL the OSM operators can actually reach —
	// set this to a monitored mailbox in production).
	EnvNominatimContact = "NOMINATIM_CONTACT"

	// nominatimDefaultContact is the fallback contact channel. The site URL is
	// itself a reachable channel per the policy; noreply@ is the app's existing
	// outbound sender identity. Prefer overriding via NOMINATIM_CONTACT.
	nominatimDefaultContact = "https://psychichomily.com; noreply@psychichomily.com"

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

	// sem (capacity 1) serializes requests — including the pre-request wait —
	// so the 1-request/second budget holds across all goroutines sharing the
	// client. A channel rather than a mutex so a waiter can give up when its
	// context expires instead of queueing uncancellably behind slow round
	// trips (the inline write path's timeout must stay honest).
	sem      chan struct{}
	lastCall time.Time // guarded by sem
}

// nominatimUserAgent builds the identifying User-Agent required by the OSM
// usage policy (stock library User-Agents are rejected), embedding the
// contact channel from NOMINATIM_CONTACT when set.
func nominatimUserAgent() string {
	contact := strings.TrimSpace(os.Getenv(EnvNominatimContact))
	if contact == "" {
		contact = nominatimDefaultContact
	}
	return fmt.Sprintf("PsychicHomily/1.0 (%s)", contact)
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
		userAgent:   nominatimUserAgent(),
		minInterval: nominatimMinInterval,
		sem:         make(chan struct{}, 1),
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
	var retryAfter time.Duration
	for attempt := 1; attempt <= nominatimMaxAttempts; attempt++ {
		if attempt > 1 {
			// Extra backoff on top of the per-request spacing: 2s, then 4s —
			// or the server's own Retry-After on a 429, whichever is longer
			// (re-hitting inside the penalty window extends the throttle).
			backoff := time.Duration(attempt-1) * 2 * time.Second
			if retryAfter > backoff {
				backoff = retryAfter
			}
			select {
			case <-ctx.Done():
				return AddressResult{}, false, ctx.Err()
			case <-time.After(backoff):
			}
		}
		res, ok, outcome, err := c.doSearch(ctx, endpoint)
		if err == nil {
			return res, ok, nil
		}
		lastErr = err
		if !outcome.retryable {
			break
		}
		retryAfter = outcome.retryAfter
	}
	return AddressResult{}, false, lastErr
}

// searchOutcome carries retry metadata for a failed doSearch attempt.
type searchOutcome struct {
	retryable  bool
	retryAfter time.Duration // server-requested wait (429 Retry-After), 0 if none
}

// nominatimMaxRetryAfter caps how long a server-requested Retry-After is
// honored; anything longer is bounded by the caller's context anyway.
const nominatimMaxRetryAfter = 30 * time.Second

// parseRetryAfter reads a Retry-After header in delay-seconds form (the form
// Nominatim uses); HTTP-date or garbage yields 0.
func parseRetryAfter(h string) time.Duration {
	secs, err := strconv.Atoi(strings.TrimSpace(h))
	if err != nil || secs <= 0 {
		return 0
	}
	d := time.Duration(secs) * time.Second
	if d > nominatimMaxRetryAfter {
		d = nominatimMaxRetryAfter
	}
	return d
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
	PlaceRank   int    `json:"place_rank"` // Nominatim rank: <=25 settlement or coarser, 26-27 street, 28-30 house/POI
}

// doSearch performs one rate-limited request. The semaphore is held across
// the pre-request wait AND the round trip so concurrent callers cannot exceed
// the request budget; acquisition itself is context-aware so a caller whose
// deadline expires while queued gives up instead of piling on. outcome
// reports whether the failure class is worth another attempt (429/5xx/
// transport) and any server-requested Retry-After.
func (c *NominatimClient) doSearch(ctx context.Context, endpoint string) (res AddressResult, ok bool, outcome searchOutcome, err error) {
	select {
	case c.sem <- struct{}{}:
	case <-ctx.Done():
		return AddressResult{}, false, searchOutcome{}, ctx.Err()
	}
	defer func() { <-c.sem }()

	if wait := c.minInterval - time.Since(c.lastCall); wait > 0 {
		select {
		case <-ctx.Done():
			return AddressResult{}, false, searchOutcome{}, ctx.Err()
		case <-time.After(wait):
		}
	}
	c.lastCall = time.Now()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return AddressResult{}, false, searchOutcome{}, fmt.Errorf("nominatim: build request: %w", err)
	}
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return AddressResult{}, false, searchOutcome{}, ctx.Err()
		}
		return AddressResult{}, false, searchOutcome{retryable: true}, fmt.Errorf("nominatim: request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch {
	case resp.StatusCode == http.StatusOK:
		// fall through to parse
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		return AddressResult{}, false,
			searchOutcome{retryable: true, retryAfter: parseRetryAfter(resp.Header.Get("Retry-After"))},
			fmt.Errorf("nominatim: status %d", resp.StatusCode)
	default:
		return AddressResult{}, false, searchOutcome{}, fmt.Errorf("nominatim: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return AddressResult{}, false, searchOutcome{retryable: true}, fmt.Errorf("nominatim: read body: %w", err)
	}
	var rows []nominatimResult
	if err := json.Unmarshal(body, &rows); err != nil {
		return AddressResult{}, false, searchOutcome{}, fmt.Errorf("nominatim: parse body: %w", err)
	}
	if len(rows) == 0 {
		return AddressResult{}, false, searchOutcome{}, nil // clean miss
	}

	r := rows[0]
	lat, latErr := strconv.ParseFloat(r.Lat, 64)
	lng, lngErr := strconv.ParseFloat(r.Lon, 64)
	if latErr != nil || lngErr != nil {
		return AddressResult{}, false, searchOutcome{}, fmt.Errorf("nominatim: unparseable coordinates %q,%q", r.Lat, r.Lon)
	}
	// Defensive bound check at the trust boundary — an out-of-range value
	// would also violate the numeric(9,6) columns.
	if lat < -90 || lat > 90 || lng < -180 || lng > 180 {
		return AddressResult{}, false, searchOutcome{}, fmt.Errorf("nominatim: coordinates out of range %f,%f", lat, lng)
	}

	return AddressResult{Latitude: lat, Longitude: lng, Precision: precisionForResult(r)}, true, searchOutcome{}, nil
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
//   - a locality-level addresstype, or ANY result whose place_rank says
//     settlement-or-coarser (<=25) → city; the rank guard fails CLOSED for
//     coarse addresstypes the map doesn't enumerate (locality, farm,
//     administrative, ...) — a locality centroid must never be labeled a
//     street-level hit;
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
	case r.PlaceRank > 0 && r.PlaceRank <= 25:
		return PrecisionCity
	case r.PlaceRank > 0 && r.PlaceRank <= 27:
		return PrecisionInterpolated
	default:
		return PrecisionRooftop
	}
}
