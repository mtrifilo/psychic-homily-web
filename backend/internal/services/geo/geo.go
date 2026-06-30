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
	// State is the matched place's GeoNames admin1 code. For a US hit this is the
	// 2-letter state code ("IL"); for a non-US hit it is GeoNames' admin1 (often
	// numeric) and must NOT be treated as a US state — pair it with Country.
	State string
	// Country is the matched place's ISO 3166-1 alpha-2 code ("US").
	Country string
}

// USStateStatus is the outcome of resolving a bare city name to a US state via
// ResolveUSState. It distinguishes "one unambiguous US state" from the two cases
// a caller must NOT guess past: a multi-state namesake, and a city that is not a
// known US place at all.
type USStateStatus int

const (
	// USStateNotFound: the city name matches no US place in the dataset (it is
	// non-US, or absent entirely). The caller has no US state to write.
	USStateNotFound USStateStatus = iota
	// USStateUnambiguous: every US namesake of this city resolves to the SAME
	// state — safe to write that state from the bare city name.
	USStateUnambiguous
	// USStateAmbiguous: the city name spans two or more US states (Pasadena CA/TX,
	// Springfield, Bloomington). A bare-city guess would silently corrupt data
	// (PSY-1244), so the caller must disambiguate from a stronger source instead.
	USStateAmbiguous
)

// Metro is the US Census CBSA (Core-Based Statistical Area) a place rolls up to —
// the canonical, OMB-maintained metro/micro definition. CBSACode is the stable
// 5-digit OMB code (the durable key to store/match on); Name is the friendly
// title for display ("Los Angeles-Long Beach-Anaheim, CA").
type Metro struct {
	CBSACode string
	Name     string
}

// MetroPrincipal describes a CBSA metro for display + keying: its principal
// (highest-population) member city, that city's state, and the city's
// coordinates — the representative name + point a scene is shown under once
// scenes are keyed by metro (PSY-1255 step C). Found is the reverse of
// ResolveMetro: ResolveMetro maps a place -> its CBSA; this maps a CBSA -> its
// principal city.
type MetroPrincipal struct {
	CBSACode  string  // echo of the input code, e.g. "16980"
	Name      string  // CBSA friendly title, e.g. "Chicago-Naperville-Elgin, IL-IN-WI"
	City      string  // principal city name, e.g. "Chicago"
	State     string  // principal city's 2-letter state, e.g. "IL"
	Latitude  float64 // principal city centroid
	Longitude float64
}

// Geocoder resolves a location to coordinates + timezone. It is an interface so
// the data source stays swappable (info hiding) and callers can stub it in tests.
type Geocoder interface {
	// Resolve returns a Result and ok=true when the location is found. A miss
	// (ok=false) is expected for obscure places; callers fall back accordingly.
	Resolve(city, state, country string) (Result, bool)
	// ResolveMetro returns the CBSA a (city, state, country) rolls up to, so
	// suburbs/boroughs share one metro key. ok is false for non-US places, US
	// places not in any CBSA, or a miss. The state disambiguates same-named cities
	// ("Pasadena, CA" → Los Angeles vs "Pasadena, TX" → Houston).
	ResolveMetro(city, state, country string) (Metro, bool)
	// ResolveUSState maps a bare city name to its US state, but ONLY when the
	// mapping is safe: the name's most-populous namesake worldwide is in the US,
	// and all US namesakes share one state. It returns the 2-letter state with
	// USStateUnambiguous; "" with USStateAmbiguous for a multi-state US namesake;
	// or "" with USStateNotFound for an internationally dominant name (Cambridge,
	// Paris) or an unknown city. It never falls back to a population guess — that
	// guess was the PSY-1244 corruption bug — so the caller must disambiguate a
	// non-Unambiguous result from a stronger source.
	ResolveUSState(city string) (state string, status USStateStatus)
}

// cityRow is one populated place from GeoNames.
type cityRow struct {
	name    string // localized display name (e.g. "Los Angeles"); the principal-city label for a metro
	country string // ISO 3166-1 alpha-2
	admin1  string // for US rows this is the 2-letter state code
	pop     int64
	lat     float64
	lng     float64
	tz      string
	// cbsaCode/cbsaName are the US Census CBSA (metro/micro area) the place's
	// county rolls up to — set for US rows only (joined offline via county FIPS,
	// see data/README.md). Empty for non-US places and the few US places not in
	// any CBSA. The CBSA is what makes boroughs/suburbs share one metro key
	// (Brooklyn + Manhattan → New York-Newark-Jersey City; Pasadena/Santa Monica
	// + LA → Los Angeles-Long Beach-Anaheim).
	cbsaCode string
	cbsaName string
}

type offlineGeocoder struct {
	byCity      map[string][]cityRow // key: foldKey(city name)
	byMetro     map[string][]cityRow // key: CBSA code -> the metro's member cities (US only)
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

// MetroPointer resolves a (city, state, country) to its US Census CBSA code as a
// nullable pointer — nil on a miss, a refused ambiguous name, or a nil geocoder,
// so assigning it to a model field / GORM updates map writes SQL NULL. The shared
// write-path seam for an entity's denormalized metro key (PSY-1255), mirroring
// LookupPointers for coordinates/timezone. ResolveMetro already refuses to guess
// an unpinned multi-state name, so a wrong-namesake metro is never stored.
func MetroPointer(g Geocoder, city, state, country string) *string {
	if g == nil {
		return nil
	}
	m, ok := g.ResolveMetro(city, state, country)
	if !ok {
		return nil
	}
	return &m.CBSACode
}

// MetroPrincipalByCBSA returns the principal (highest-population) city of a CBSA
// metro — its name, state, and coordinates — for displaying/keying a scene by
// metro (PSY-1255 step C). ok is false for an unknown code (non-US, or a code
// absent from the dataset). It uses the process-wide geocoder, mirroring
// CountryToISO/CanonicalCountryName, because the principal-city pick needs the
// full embedded population data, not a caller-stubbable subset.
func MetroPrincipalByCBSA(cbsa string) (MetroPrincipal, bool) {
	Default() // ensure the dataset is parsed
	return defaultGeo.metroPrincipal(cbsa)
}

// metroPrincipal selects the highest-population member city of a CBSA. The
// Census principal city of a metro is (by construction) its largest, so max
// population is the right pick — and it sidesteps parsing the multi-city
// cbsaName (where "Winston-Salem, NC" is one hyphenated city, not two).
func (g *offlineGeocoder) metroPrincipal(cbsa string) (MetroPrincipal, bool) {
	rows := g.byMetro[strings.TrimSpace(cbsa)]
	if len(rows) == 0 {
		return MetroPrincipal{}, false
	}
	best := rows[0]
	for _, r := range rows[1:] {
		if r.pop > best.pop {
			best = r
		}
	}
	return MetroPrincipal{
		CBSACode:  best.cbsaCode,
		Name:      best.cbsaName,
		City:      best.name,
		State:     best.admin1,
		Latitude:  best.lat,
		Longitude: best.lng,
	}, true
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
		byMetro:     make(map[string][]cityRow, 1000),
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
		// override it, so CanonicalCountryName returns a stable display form. Strip
		// a leading article ("The Netherlands" -> "Netherlands") so the canonical
		// form matches what users type and other records store.
		if _, seen := g.isoToName[iso]; !seen {
			g.isoToName[iso] = stripLeadingThe(name)
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
		// name, asciiname, country, admin1, population, lat, lng, timezone,
		// cbsaCode, cbsaName (the last two US-only, may be empty)
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
			name:    strings.TrimSpace(f[0]),
			country: strings.ToUpper(strings.TrimSpace(f[2])),
			admin1:  strings.TrimSpace(f[3]),
			pop:     pop,
			lat:     lat,
			lng:     lng,
			tz:      tz,
		}
		// CBSA columns are optional so an older extract without them (8 or 9
		// fields) still loads (metro empty). Present only for US rows.
		if len(f) >= 10 {
			row.cbsaCode = strings.TrimSpace(f[8])
			row.cbsaName = strings.TrimSpace(f[9])
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
		// Reverse index: every CBSA member city, keyed by its code, so a metro
		// can be resolved to its principal (highest-population) city for scene
		// display/keying (PSY-1255 step C). Append once per row (not per name key).
		if row.cbsaCode != "" {
			g.byMetro[row.cbsaCode] = append(g.byMetro[row.cbsaCode], row)
		}
	}
}

func (g *offlineGeocoder) Resolve(city, state, country string) (Result, bool) {
	best, ok := g.bestCity(city, state, country)
	if !ok {
		return Result{}, false
	}
	return Result{
		Latitude:  best.lat,
		Longitude: best.lng,
		Timezone:  best.tz,
		State:     best.admin1,
		Country:   best.country,
	}, true
}

// ResolveMetro returns the US Census CBSA (metro/micro area) a (city, state,
// country) belongs to, so suburbs and boroughs roll up to one metro key
// (Brooklyn, NY → New York-Newark-Jersey City; Pasadena, CA → Los Angeles-Long
// Beach-Anaheim). ok is false for a non-US place, a US place not in any CBSA, a
// miss, OR an unpinned ambiguous name (see below).
//
// It REFUSES the population guess that sibling ResolveUSState refuses (the
// PSY-1244 corruption class): a metro is returned only when the same-named place
// is unambiguously pinned —
//   - an explicit US state was given AND it matches the selected place's state
//     (a non-matching state means the highest-population fallback fired → refuse);
//   - or no state was given but the name is US-unambiguous (one US state, US-
//     dominant) per ResolveUSState.
//
// So "Pasadena, CA" → Los Angeles and "Pasadena, TX" → Houston, but a bare
// "Pasadena" (or a bogus state) → ok=false rather than a guessed metro. Callers
// must pass the confident state derived through the artist-state pipeline, never
// a raw user-entered city alone.
func (g *offlineGeocoder) ResolveMetro(city, state, country string) (Metro, bool) {
	best, ok := g.bestCity(city, state, country)
	if !ok || best.cbsaCode == "" {
		return Metro{}, false // miss, non-US, or US place not in any CBSA
	}
	if _, admin1 := g.resolveCountry(state, country); admin1 != "" {
		// A US state was given — trust the metro only if it actually pinned THIS
		// place. A mismatch means bestCity fell back to the highest-population
		// namesake (admin1 is a preference, not a hard filter), so refuse.
		if best.admin1 != admin1 {
			return Metro{}, false
		}
	} else if _, status := g.ResolveUSState(city); status != USStateUnambiguous {
		// No state to pin the namesake and the name isn't unambiguously one US
		// state — refuse rather than guess the wrong same-named city's metro.
		return Metro{}, false
	}
	return Metro{CBSACode: best.cbsaCode, Name: best.cbsaName}, true
}

// bestCity selects the populated place a (city, state, country) refers to: it
// filters the name's candidates by a confident country, prefers an exact US-state
// (admin1) match, then takes the highest-population row. Shared by Resolve and
// ResolveMetro so coordinates/timezone and metro always describe the SAME place.
func (g *offlineGeocoder) bestCity(city, state, country string) (cityRow, bool) {
	cityKey := foldKey(city)
	if cityKey == "" {
		return cityRow{}, false
	}
	candidates := g.byCity[cityKey]
	if len(candidates) == 0 {
		return cityRow{}, false
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
			return cityRow{}, false
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
	return best, true
}

// ResolveUSState maps a bare city name to its US state, but only when that
// mapping is safe on two counts: the name's most-populous namesake WORLDWIDE is
// in the US, and all US namesakes share ONE state. See the Geocoder interface for
// the contract and USStateStatus for the cases.
//
// Two refusals, both deliberate:
//   - Internationally dominant name (Cambridge → UK, Paris → FR): a band tagged
//     only by city, with no country, is more likely the bigger international
//     place than the smaller US namesake — so return NotFound and let a stronger
//     source (an explicit country, or MusicBrainz) decide, rather than stamping
//     a US state on a probably-foreign band.
//   - Multi-state US namesake (Portland OR/ME, Springfield): return Ambiguous,
//     never the biggest one — refusing the highest-population guess is the whole
//     point (it corrupted data in PSY-1244).
//
// "Unambiguous" is scoped to the embedded, population-filtered dataset; a tiny
// same-name town it omits won't shift the answer, which is acceptable for the
// scene use case (real metros).
func (g *offlineGeocoder) ResolveUSState(city string) (string, USStateStatus) {
	cityKey := foldKey(city)
	if cityKey == "" {
		return "", USStateNotFound
	}
	rows := g.byCity[cityKey]
	if len(rows) == 0 {
		return "", USStateNotFound
	}

	var dominant cityRow
	usState := ""
	multiState := false
	for i, r := range rows {
		// Most-populous namesake wins; a population TIE resolves toward the non-US
		// row so the result is NotFound (defer to a stronger source) rather than a
		// coin-flip on dataset order — safer at the exact boundary this guards.
		switch {
		case i == 0 || r.pop > dominant.pop:
			dominant = r
		case r.pop == dominant.pop && dominant.country == "US" && r.country != "US":
			dominant = r
		}
		if r.country != "US" || r.admin1 == "" {
			continue
		}
		switch {
		case usState == "":
			usState = r.admin1
		case r.admin1 != usState:
			multiState = true
		}
	}

	if dominant.country != "US" {
		return "", USStateNotFound // a bigger international namesake → don't assume US
	}
	if multiState {
		return "", USStateAmbiguous // same name, two US states → don't guess
	}
	if usState == "" {
		return "", USStateNotFound // dominant is US but carries no admin1 (rare)
	}
	return usState, USStateUnambiguous
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
// (ß, œ, ø…) to ASCII, drop punctuation, collapse whitespace, and finally expand
// the place abbreviations St./Ft./Mt. (see expandPlaceAbbreviations) — single
// pass. Mirrors the NFKD+mark-strip approach radio matching uses in normalizeName.
//
// Used for BOTH city and country keys (byCity, nameToISO), at load time and at
// query time — so the abbreviation expansion is symmetric across the dataset and
// the query, which is the whole point (see expandPlaceAbbreviations). It is
// collision-free for country names too (no country folds to a bare st/ft/mt
// token; "St. Lucia"/"Kitts"/"Vincent" already store "Saint").
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
	return expandPlaceAbbreviations(out)
}

// placeAbbreviations expands the standard US/CA/UK place-name abbreviations to
// their full forms so a contributor's "St. Paul" / "Ft. Worth" / "Mt. Pleasant"
// folds to the SAME key as the dataset entry.
//
// This MUST run inside foldKey rather than a query-side helper: foldKey keys both
// the query AND the embedded city names (at load time), and the dataset stores
// the two forms INCONSISTENTLY — "Saint Paul" is full but "St. Louis" is
// abbreviated. Folding both sides through the same expansion is what makes either
// input form resolve regardless of how a given row happens to be stored. Without
// it, "St. Paul, MN" missed its metro and orphaned into a phantom 0-roster scene
// (PSY-1276).
//
// Collision-free: every leading St/Ft/Mt token in the city-NAME column of
// cities.tsv is a genuine Saint/Fort/Mount (verified across all rows — no
// Street/State false-friends, no "Ste"/Sainte rows). CBSA *display* names like
// "St. Louis, MO-IL" keep their abbreviation — they're returned verbatim, never
// folded. One intended consequence: a bare, unpinned query whose name also
// matches a far larger international "Saint X" (e.g. "St. Petersburg" → Russia)
// now defers to that dominant namesake, consistent with the Cambridge/Paris
// policy in ResolveUSState (a pinned state still resolves the US city correctly).
var placeAbbreviations = map[string]string{
	"st": "saint",
	"ft": "fort",
	"mt": "mount",
}

// expandPlaceAbbreviations rewrites any WHOLE token that is a known place
// abbreviation. Whole-token only, so a prefix like "Stockton" / "Fortuna" /
// "Montgomery" — one token — never matches. Allocates only when a token matches.
//
// Precondition: key is foldKey's normalized output (lowercase, single-space
// separated, no leading/trailing space) — it's called only at the end of foldKey.
func expandPlaceAbbreviations(key string) string {
	if key == "" {
		return key
	}
	fields := strings.Split(key, " ")
	changed := false
	for i, f := range fields {
		if rep, ok := placeAbbreviations[f]; ok {
			fields[i] = rep
			changed = true
		}
	}
	if !changed {
		return key
	}
	return strings.Join(fields, " ")
}

func stripLeadingThe(s string) string {
	if len(s) >= 4 && strings.EqualFold(s[:4], "the ") {
		return s[4:]
	}
	return s
}

// SamePlaceName reports whether two place names refer to the same place under the
// geocoder's own folding (diacritics stripped, lowercased, punctuation/space
// collapsed). Callers cross-checking a place name against another source — e.g.
// confirming a MusicBrainz city equals an artist's stored city before trusting
// its parent state — should use this so the comparison matches how Resolve keys
// the dataset ("Montréal" == "Montreal", "St. Louis" == "saint louis"). Two empty
// or blank names are NOT considered a match.
func SamePlaceName(a, b string) bool {
	ka, kb := foldKey(a), foldKey(b)
	return ka != "" && ka == kb
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
