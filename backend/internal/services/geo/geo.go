// Package geo resolves a venue's (city, state, country) to coordinates and an
// IANA timezone using an embedded, offline GeoNames cities dataset.
//
// Why this exists: show times must be anchored to the venue's local timezone so
// a show reads correctly for any viewer anywhere in the world. The previous
// approach guessed the zone from a US-state lookup that defaulted everything
// non-US to America/Phoenix. GeoNames gives us the IANA zone per city directly
// (cities15000 column 18), so a single offline lookup yields (lat, lng, tz) with
// no external API, key, or rate limit.
//
// Timezone resolution tolerates city-centroid precision — we only need the point
// to fall in the right tz region — so coarse matching is acceptable.
package geo

import (
	"bufio"
	"bytes"
	_ "embed"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

//go:embed data/cities.tsv
var citiesData []byte

//go:embed data/countries.tsv
var countriesData []byte

// Result is a resolved geocoding hit.
type Result struct {
	Latitude  float64
	Longitude float64
	Timezone  string // IANA name, e.g. "America/Phoenix", "Europe/London"
}

// Geocoder resolves a location to coordinates + timezone. It is an interface so
// the data source stays swappable (info hiding) and callers can stub it in tests.
type Geocoder interface {
	// Resolve returns a Result and ok=true when the location is found. A miss
	// (ok=false) is expected for obscure places; callers fall back accordingly.
	Resolve(city, state, country string) (Result, bool)
}

// cityRow is one populated place from GeoNames.
type cityRow struct {
	country string // ISO 3166-1 alpha-2
	admin1  string // for US rows this is the 2-letter state code
	pop     int64
	lat     float64
	lng     float64
	tz      string
}

type offlineGeocoder struct {
	byCity      map[string][]cityRow // key: foldKey(city name)
	nameToISO   map[string]string    // key: foldKey(country name) -> ISO2
	isoToName   map[string]string    // ISO2 -> canonical display name
	isoCodes    map[string]bool      // valid ISO2 codes
	usStates    map[string]bool      // US state/territory codes (admin1)
	caProvinces map[string]bool      // Canadian province codes (non-ISO-colliding)
}

var (
	defaultOnce sync.Once
	defaultGeo  *offlineGeocoder
)

// Default returns the process-wide offline geocoder, parsing the embedded
// datasets on first use.
func Default() Geocoder {
	defaultOnce.Do(func() { defaultGeo = newOfflineGeocoder() })
	return defaultGeo
}

// LookupPointers resolves a location and returns nullable latitude/longitude/
// timezone pointers — all nil on a miss (or a nil geocoder). This is the shared
// entry point for every venue write path (VenueService, pending-edit apply,
// data-sync import): assign the results to a model's fields or a GORM updates
// map, where a nil pointer writes SQL NULL so display falls back to the legacy
// state->tz map. (PSY-985)
func LookupPointers(g Geocoder, city, state, country string) (lat, lng *float64, tz *string) {
	if g == nil {
		return nil, nil, nil
	}
	res, ok := g.Resolve(city, state, country)
	if !ok {
		return nil, nil, nil
	}
	return &res.Latitude, &res.Longitude, &res.Timezone
}

// CountryToISO canonicalizes a country string — an ISO 3166-1 alpha-2 code, a
// full name, or a known alias ("USA") — to its ISO2 code; ok is false for an
// unrecognized value. It exists to compare country values from heterogeneous
// sources (MusicBrainz ISO codes vs Bandcamp free-text names) on a single key.
func CountryToISO(s string) (string, bool) {
	Default() // ensure the dataset is parsed
	return defaultGeo.countryToISO(s)
}

func (g *offlineGeocoder) countryToISO(s string) (string, bool) {
	t := strings.TrimSpace(s)
	if t == "" {
		return "", false
	}
	if len(t) == 2 {
		if up := strings.ToUpper(t); g.isoCodes[up] {
			return up, true
		}
	}
	if iso, ok := g.nameToISO[foldKey(stripLeadingThe(t))]; ok {
		return iso, true
	}
	return "", false
}

// CanonicalCountryName maps any recognized country string to a single canonical
// display name (the GeoNames name for its ISO code), so values from different
// sources ("US" / "USA" / "United States") store identically. ok is false for an
// unrecognized value (the caller then keeps the original string).
func CanonicalCountryName(s string) (string, bool) {
	Default()
	iso, ok := defaultGeo.countryToISO(s)
	if !ok {
		return "", false
	}
	name, ok := defaultGeo.isoToName[iso]
	return name, ok
}

func newOfflineGeocoder() *offlineGeocoder {
	g := &offlineGeocoder{
		byCity:      make(map[string][]cityRow, 40000),
		nameToISO:   make(map[string]string, 512),
		isoToName:   make(map[string]string, 256),
		isoCodes:    make(map[string]bool, 256),
		usStates:    usStateSet(),
		caProvinces: caProvinceSet(),
	}
	g.loadCountries()
	g.loadCities()
	return g
}

func (g *offlineGeocoder) loadCountries() {
	sc := bufio.NewScanner(bytes.NewReader(countriesData))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) < 2 {
			continue
		}
		iso := strings.ToUpper(strings.TrimSpace(f[0]))
		name := strings.TrimSpace(f[1])
		if iso == "" || name == "" {
			continue
		}
		g.isoCodes[iso] = true
		g.nameToISO[foldKey(name)] = iso
		// First (canonical GeoNames) name per ISO wins; later aliases don't
		// override it, so CanonicalCountryName returns a stable display form.
		if _, seen := g.isoToName[iso]; !seen {
			g.isoToName[iso] = name
		}
		// GeoNames prefixes some names with "The " (e.g. "The Netherlands").
		// Index the stripped form too so "Netherlands" resolves.
		if stripped := stripLeadingThe(name); stripped != name {
			g.nameToISO[foldKey(stripped)] = iso
		}
	}
	// Common aliases not present verbatim in GeoNames country names.
	for alias, iso := range countryAliases {
		if g.isoCodes[iso] {
			g.nameToISO[foldKey(alias)] = iso
		}
	}
}

func (g *offlineGeocoder) loadCities() {
	sc := bufio.NewScanner(bytes.NewReader(citiesData))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		// name, asciiname, country, admin1, population, lat, lng, timezone
		f := strings.Split(sc.Text(), "\t")
		if len(f) < 8 {
			continue
		}
		tz := strings.TrimSpace(f[7])
		if tz == "" {
			continue
		}
		pop, _ := strconv.ParseInt(strings.TrimSpace(f[4]), 10, 64)
		lat, errLat := strconv.ParseFloat(strings.TrimSpace(f[5]), 64)
		lng, errLng := strconv.ParseFloat(strings.TrimSpace(f[6]), 64)
		if errLat != nil || errLng != nil {
			continue
		}
		row := cityRow{
			country: strings.ToUpper(strings.TrimSpace(f[2])),
			admin1:  strings.TrimSpace(f[3]),
			pop:     pop,
			lat:     lat,
			lng:     lng,
			tz:      tz,
		}
		// Index under both the localized name and the ASCII name so accented
		// and plain input both hit. foldKey collapses them to one key in most
		// cases, so guard against duplicate appends.
		nameKey := foldKey(f[0])
		asciiKey := foldKey(f[1])
		g.byCity[nameKey] = append(g.byCity[nameKey], row)
		if asciiKey != nameKey {
			g.byCity[asciiKey] = append(g.byCity[asciiKey], row)
		}
	}
}

func (g *offlineGeocoder) Resolve(city, state, country string) (Result, bool) {
	cityKey := foldKey(city)
	if cityKey == "" {
		return Result{}, false
	}
	candidates := g.byCity[cityKey]
	if len(candidates) == 0 {
		return Result{}, false
	}

	iso, admin1 := g.resolveCountry(state, country)

	// If we know the country, restrict to it. A confident country with no
	// in-country match returns a miss rather than a wrong-country guess.
	if iso != "" {
		var filtered []cityRow
		for _, c := range candidates {
			if c.country == iso {
				filtered = append(filtered, c)
			}
		}
		if len(filtered) == 0 {
			return Result{}, false
		}
		candidates = filtered
	}

	// Prefer an exact US-state (admin1) match when we have one.
	if admin1 != "" {
		var byAdmin []cityRow
		for _, c := range candidates {
			if c.admin1 == admin1 {
				byAdmin = append(byAdmin, c)
			}
		}
		if len(byAdmin) > 0 {
			candidates = byAdmin
		}
	}

	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.pop > best.pop {
			best = c
		}
	}
	return Result{Latitude: best.lat, Longitude: best.lng, Timezone: best.tz}, true
}

// resolveCountry determines an ISO country code (and, for the US, a state admin1
// hint) from the venue's state/country fields. The data is messy: `state` may be
// a US state code, a 2-letter country code, or empty; `country` may be a full
// name or empty.
func (g *offlineGeocoder) resolveCountry(state, country string) (iso, admin1 string) {
	st := strings.ToUpper(strings.TrimSpace(state))

	// US state code in `state` is the most reliable signal.
	if g.usStates[st] {
		return "US", st
	}

	c := strings.TrimSpace(country)
	if c != "" {
		// Already a 2-letter ISO code?
		if len(c) == 2 && g.isoCodes[strings.ToUpper(c)] {
			return strings.ToUpper(c), ""
		}
		if code, ok := g.nameToISO[foldKey(stripLeadingThe(c))]; ok {
			return code, ""
		}
	}

	// Canadian province code (no explicit country): filter to Canada so the city
	// resolves to its Canadian instance instead of a higher-population namesake
	// elsewhere (e.g. London/ON -> America/Toronto, not Europe/London). GeoNames
	// CA admin1 codes are numeric, so we filter by country and let population pick
	// the right Canadian city. NL/PE/SK/YT/NU are excluded — they collide with ISO
	// codes (Netherlands/Peru/Slovakia/Mayotte/Niue); pass country="Canada" for those.
	if g.caProvinces[st] {
		return "CA", ""
	}

	// `state` sometimes holds a country code for international venues
	// (e.g. "NL", "DE") — only trust it when it's a real ISO code and not a US state.
	if len(st) == 2 && g.isoCodes[st] {
		return st, ""
	}

	return "", ""
}

// foldKey normalizes a place/country name for matching: strip diacritics via
// canonical decomposition (so "Zürich"/"São Paulo"/"Kraków"/Turkish/Vietnamese
// names fold to ASCII), lowercase, map the few non-decomposable Latin letters
// (ß, œ, ø…) to ASCII, drop punctuation, and collapse whitespace — single pass.
// Mirrors the NFKD+mark-strip approach radio matching uses in normalizeName.
func foldKey(s string) string {
	// Per-call transformer: transform.Transformer is stateful and not safe for
	// concurrent reuse (Resolve may run on many goroutines).
	stripper := transform.Chain(norm.NFKD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	folded, _, err := transform.String(stripper, s)
	if err != nil {
		// transform only errors if the chain errors, which norm/runes cannot on
		// valid UTF-8; fall back to the raw string rather than corrupt the key.
		folded = s
	}

	var b strings.Builder
	b.Grow(len(folded))
	prevSpace := false
	for _, r := range folded {
		r = unicode.ToLower(r)
		if rep, ok := specialLetters[r]; ok {
			b.WriteString(rep)
			prevSpace = false
			continue
		}
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevSpace = false
		case r == '-' || r == '\'' || r == '.' || r == '/' || unicode.IsSpace(r):
			if !prevSpace && b.Len() > 0 {
				b.WriteByte(' ')
				prevSpace = true
			}
		}
		// All other runes (residual marks, punctuation) are dropped.
	}
	out := b.String()
	if n := len(out); n > 0 && out[n-1] == ' ' {
		out = out[:n-1] // trim a trailing separator-space
	}
	return out
}

func stripLeadingThe(s string) string {
	if len(s) >= 4 && strings.EqualFold(s[:4], "the ") {
		return s[4:]
	}
	return s
}

// specialLetters maps the lowercase Latin letters that canonical decomposition
// (NFKD) does NOT split into base+mark — they're distinct letters, not accented
// forms — to their conventional ASCII spelling.
var specialLetters = map[rune]string{
	'ß': "ss", 'œ': "oe", 'æ': "ae", 'ø': "o", 'ł': "l", 'đ': "d", 'ð': "d", 'þ': "th",
}

// countryAliases maps common informal country names to ISO2 codes when GeoNames'
// canonical name differs from everyday usage.
var countryAliases = map[string]string{
	"usa":            "US",
	"u.s.a.":         "US",
	"u.s.":           "US",
	"united states":  "US",
	"america":        "US",
	"uk":             "GB",
	"u.k.":           "GB",
	"great britain":  "GB",
	"england":        "GB",
	"scotland":       "GB",
	"wales":          "GB",
	"south korea":    "KR",
	"north korea":    "KP",
	"russia":         "RU",
	"czech republic": "CZ",
	"czechia":        "CZ",
}

func usStateSet() map[string]bool {
	codes := []string{
		"AL", "AK", "AZ", "AR", "CA", "CO", "CT", "DE", "DC", "FL", "GA",
		"HI", "ID", "IL", "IN", "IA", "KS", "KY", "LA", "ME", "MD", "MA",
		"MI", "MN", "MS", "MO", "MT", "NE", "NV", "NH", "NJ", "NM", "NY",
		"NC", "ND", "OH", "OK", "OR", "PA", "RI", "SC", "SD", "TN", "TX",
		"UT", "VT", "VA", "WA", "WV", "WI", "WY",
	}
	m := make(map[string]bool, len(codes))
	for _, c := range codes {
		m[c] = true
	}
	return m
}

// caProvinceSet returns Canadian province/territory codes that do NOT collide
// with ISO 3166-1 alpha-2 country codes. NL (Netherlands), PE (Peru), SK
// (Slovakia), YT (Mayotte), and NU (Niue) are deliberately omitted — for venues
// in those provinces, pass country="Canada" so the country field disambiguates.
func caProvinceSet() map[string]bool {
	codes := []string{"ON", "QC", "BC", "AB", "MB", "NB", "NS", "NT"}
	m := make(map[string]bool, len(codes))
	for _, c := range codes {
		m[c] = true
	}
	return m
}
