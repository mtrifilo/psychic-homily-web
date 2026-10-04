package geo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// PlaceQuery is a free-form Nominatim search for a NAMED place (a venue), as
// opposed to AddressQuery, which resolves a known street address. The text sent
// is "<name>, <city>, <state>"; CountryCode (ISO 3166-1 alpha-2) restricts the
// search to one country when set.
type PlaceQuery struct {
	Name        string
	City        string
	State       string
	CountryCode string
}

// Text is the free-form q parameter: the non-empty parts of name, city, and
// state, comma-joined.
func (q PlaceQuery) Text() string {
	parts := make([]string, 0, 3)
	for _, p := range []string{q.Name, q.City, q.State} {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, ", ")
}

// Key is the canonical "what was searched" string, used to memoize a miss so
// an unchanged venue is not searched again: the query text plus the country
// restriction.
func (q PlaceQuery) Key() string {
	text := q.Text()
	if cc := strings.ToUpper(strings.TrimSpace(q.CountryCode)); cc != "" {
		return text + " [" + cc + "]"
	}
	return text
}

// PlaceCandidate is one search row with the address components the caller
// needs to decide whether the row is the venue it searched for.
type PlaceCandidate struct {
	OSMType     string
	Category    string // OSM key: amenity, building, highway, ...
	Type        string // OSM value: bar, theatre, yes, ...
	AddressType string
	PlaceRank   int
	Name        string
	DisplayName string
	Latitude    float64
	Longitude   float64
	HouseNumber string
	Road        string
	// Localities holds every locality-level component the row carries (city,
	// town, village, borough, suburb, ...), so a venue stored under a borough
	// name ("Brooklyn") still matches a row whose city is "New York".
	Localities  []string
	State       string
	StateCode   string // ISO 3166-2 subdivision code, e.g. "US-IL"; empty when absent
	Postcode    string
	CountryCode string // lowercase ISO 3166-1 alpha-2, as Nominatim returns it
}

// PlaceSearcher finds candidate places for a free-form query. A nil error with
// no candidates is a clean miss; a non-nil error is a transport or service
// failure a later run may succeed on.
type PlaceSearcher interface {
	SearchPlaces(ctx context.Context, q PlaceQuery) ([]PlaceCandidate, error)
}

// placeSearchLimit bounds the rows one search returns. The caller takes the
// first row that passes its acceptance rules, so a handful is enough to step
// past a same-named street or neighbourhood ranked above the venue.
const placeSearchLimit = 5

// placeRow is the jsonv2 row shape with addressdetails=1.
type placeRow struct {
	nominatimResult
	Name        string            `json:"name"`
	DisplayName string            `json:"display_name"`
	Address     map[string]string `json:"address"`
}

// placeLocalityKeys are the addressdetails keys that name a locality, in the
// order Nominatim documents them from coarse to fine.
var placeLocalityKeys = []string{
	"city", "town", "village", "hamlet", "municipality",
	"borough", "city_district", "district", "suburb", "quarter", "neighbourhood",
}

// SearchPlaces runs a rate-limited free-form search and returns up to
// placeSearchLimit candidates in Nominatim's ranking order. It shares the
// client's limiter and retry policy with GeocodeAddress, so the 1 request per
// second budget holds across both kinds of lookup. Rows whose coordinates do
// not parse are dropped rather than failing the search.
func (c *NominatimClient) SearchPlaces(ctx context.Context, q PlaceQuery) ([]PlaceCandidate, error) {
	text := q.Text()
	if strings.TrimSpace(q.Name) == "" {
		return nil, nil
	}

	params := url.Values{}
	params.Set("format", "jsonv2")
	params.Set("limit", fmt.Sprintf("%d", placeSearchLimit))
	params.Set("addressdetails", "1")
	params.Set("q", text)
	if cc := strings.ToLower(strings.TrimSpace(q.CountryCode)); cc != "" {
		params.Set("countrycodes", cc)
	}
	endpoint := c.baseURL + "/search?" + params.Encode()

	var body []byte
	err := c.withRetries(ctx, func() (searchOutcome, error) {
		b, outcome, err := c.doGet(ctx, endpoint)
		body = b
		return outcome, err
	})
	if err != nil {
		return nil, err
	}

	var rows []placeRow
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("nominatim: parse body: %w", err)
	}
	candidates := make([]PlaceCandidate, 0, len(rows))
	for _, r := range rows {
		lat, lng, err := parseCoordinates(r.Lat, r.Lon)
		if err != nil {
			continue
		}
		cand := PlaceCandidate{
			OSMType:     r.OSMType,
			Category:    r.Category,
			Type:        r.Type,
			AddressType: r.AddressType,
			PlaceRank:   r.PlaceRank,
			Name:        r.Name,
			DisplayName: r.DisplayName,
			Latitude:    lat,
			Longitude:   lng,
			HouseNumber: strings.TrimSpace(r.Address["house_number"]),
			Road:        strings.TrimSpace(r.Address["road"]),
			State:       r.Address["state"],
			StateCode:   r.Address["ISO3166-2-lvl4"],
			Postcode:    r.Address["postcode"],
			CountryCode: strings.ToLower(r.Address["country_code"]),
		}
		for _, k := range placeLocalityKeys {
			if v := strings.TrimSpace(r.Address[k]); v != "" {
				cand.Localities = append(cand.Localities, v)
			}
		}
		candidates = append(candidates, cand)
	}
	return candidates, nil
}
