package geo

import (
	"strings"
	"testing"
)

func TestResolve(t *testing.T) {
	g := Default()
	tests := []struct {
		name              string
		city, state, ctry string
		wantTZ            string
		wantOK            bool
	}{
		{"phoenix US state", "Phoenix", "AZ", "", "America/Phoenix", true},
		{"london by ISO code", "London", "", "GB", "Europe/London", true},
		{"london by country name", "London", "", "United Kingdom", "Europe/London", true},
		{"london ontario disambiguated by country", "London", "ON", "Canada", "America/Toronto", true},
		{"london ontario by province code, no country", "London", "ON", "", "America/Toronto", true},
		{"montreal by province code, no country", "Montreal", "QC", "", "America/Toronto", true},
		{"vancouver by province code, no country", "Vancouver", "BC", "", "America/Vancouver", true},
		{"NL means Netherlands not Newfoundland without country", "Amsterdam", "NL", "", "Europe/Amsterdam", true},
		{"portland oregon", "Portland", "OR", "", "America/Los_Angeles", true},
		{"portland maine", "Portland", "ME", "", "America/New_York", true},
		{"paris france", "Paris", "", "France", "Europe/Paris", true},
		{"paris texas", "Paris", "TX", "", "America/Chicago", true},
		// PSY-1276: an abbreviated "St." resolves the same as the full form. When
		// pinned by state the correct row wins even though the larger Russian
		// "Saint Petersburg" now shares the folded key.
		{"st. paul abbreviated → Chicago tz", "St. Paul", "MN", "", "America/Chicago", true},
		{"st. petersburg FL pinned → New York tz", "St. Petersburg", "FL", "", "America/New_York", true},
		{"berlin germany", "Berlin", "", "Germany", "Europe/Berlin", true},
		// PSY-1377: the cities1000 tier now INCLUDES these sub-15k split-zone towns,
		// so they resolve to their EXACT zone rather than missing. Sidney, NE (pop
		// ~6.9k, Mountain panhandle) and Evanston, WY (~12k) both resolve to
		// America/Denver — pinned to their own state over the same-named Eastern/
		// Central namesakes (Sidney OH, Evanston IL) by bestCity's admin1 hard filter.
		// (Under cities15000 these were misses that fell back to the state's
		// predominant zone — wrong for a minority-zone panhandle town; PSY-1012.)
		{"sub-15k split-zone town resolves to its exact zone (cities1000)", "Sidney", "NE", "", "America/Denver", true},
		{"second split-zone town resolves exactly", "Evanston", "WY", "", "America/Denver", true},
		// A third zone: Ontario, OR (pop ~11k) sits in Oregon's Mountain-time
		// panhandle, so it resolves to America/Boise — NOT the OR predominant Pacific
		// zone the cities15000 state fallback would have given.
		{"OR Mountain panhandle town resolves to Boise", "Ontario", "OR", "", "America/Boise", true},
		// PSY-1012's hard filter still guards the remaining long tail: a confident US
		// state with NO same-name row ANYWHERE in the dataset must MISS (-> NULL ->
		// state fallback), not return a wrong-state namesake. Pasadena is in MD/TX/CA
		// but not FL, even at cities1000.
		{"confident state pins no namesake → miss", "Pasadena", "FL", "", "", false},
		// No regression: a same-state namesake still resolves. Pasadena, MD returns
		// its Eastern zone (not the larger TX/CA namesakes).
		{"same-state namesake still resolves", "Pasadena", "MD", "", "America/New_York", true},
		{"amsterdam state-holds-country-code", "Amsterdam", "NL", "Netherlands", "Europe/Amsterdam", true},
		{"montreal by country name", "Montreal", "QC", "Canada", "America/Toronto", true},
		{"montreal accented input", "Montréal", "QC", "Canada", "America/Toronto", true},
		{"zurich accented input", "Zürich", "", "Switzerland", "Europe/Zurich", true},
		{"tokyo japan", "Tokyo", "", "Japan", "Asia/Tokyo", true},
		{"unknown city is a miss", "Nowherecityville", "", "", "", false},
		{"empty city is a miss", "", "AZ", "US", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := g.Resolve(tt.city, tt.state, tt.ctry)
			if ok != tt.wantOK {
				t.Fatalf("Resolve(%q,%q,%q) ok=%v, want %v", tt.city, tt.state, tt.ctry, ok, tt.wantOK)
			}
			if ok && got.Timezone != tt.wantTZ {
				t.Errorf("Resolve(%q,%q,%q) tz=%q, want %q", tt.city, tt.state, tt.ctry, got.Timezone, tt.wantTZ)
			}
		})
	}
}

func TestResolveReturnsCoordinates(t *testing.T) {
	got, ok := Default().Resolve("Phoenix", "AZ", "")
	if !ok {
		t.Fatal("expected Phoenix to resolve")
	}
	if got.Latitude == 0 || got.Longitude == 0 {
		t.Errorf("expected non-zero coordinates, got lat=%v lng=%v", got.Latitude, got.Longitude)
	}
}

// TestLookupPointers covers the shared seam every venue write path relies on:
// nil pointers on a miss or nil geocoder (→ SQL NULL → legacy fallback), and
// non-nil pointers on a hit.
func TestLookupPointers(t *testing.T) {
	g := Default()

	t.Run("hit returns non-nil pointers", func(t *testing.T) {
		lat, lng, tz := LookupPointers(g, "Phoenix", "AZ", "")
		if lat == nil || lng == nil || tz == nil {
			t.Fatalf("expected non-nil pointers, got lat=%v lng=%v tz=%v", lat, lng, tz)
		}
		if *tz != "America/Phoenix" {
			t.Errorf("tz = %q, want America/Phoenix", *tz)
		}
	})

	t.Run("miss returns all nil", func(t *testing.T) {
		lat, lng, tz := LookupPointers(g, "Nowherecityville", "ZZ", "")
		if lat != nil || lng != nil || tz != nil {
			t.Errorf("expected all nil on miss, got lat=%v lng=%v tz=%v", lat, lng, tz)
		}
	})

	t.Run("nil geocoder returns all nil", func(t *testing.T) {
		lat, lng, tz := LookupPointers(nil, "Phoenix", "AZ", "")
		if lat != nil || lng != nil || tz != nil {
			t.Errorf("expected all nil with nil geocoder, got lat=%v lng=%v tz=%v", lat, lng, tz)
		}
	})

	// PSY-1377: a sub-15k split-zone town now IN the cities1000 tier resolves to its
	// exact zone through the write seam — Sidney, NE -> America/Denver (was a miss ->
	// NULL -> state fallback under cities15000).
	t.Run("cities1000 sub-15k town returns its exact zone", func(t *testing.T) {
		lat, lng, tz := LookupPointers(g, "Sidney", "NE", "")
		if lat == nil || lng == nil || tz == nil {
			t.Fatalf("expected non-nil pointers for Sidney,NE, got lat=%v lng=%v tz=%v", lat, lng, tz)
		}
		if *tz != "America/Denver" {
			t.Errorf("tz = %q, want America/Denver", *tz)
		}
	})

	// PSY-1012's miss seam still holds for a genuinely absent town: a confident state
	// with no same-name row anywhere -> all nil -> SQL NULL -> state fallback.
	t.Run("absent town under a confident state returns all nil", func(t *testing.T) {
		lat, lng, tz := LookupPointers(g, "Pasadena", "FL", "")
		if lat != nil || lng != nil || tz != nil {
			t.Errorf("expected all nil for Pasadena,FL, got lat=%v lng=%v tz=%v", lat, lng, tz)
		}
	})

	// No regression at the write seam: a same-state namesake among several (Pasadena
	// MD, vs the larger TX/CA) still returns non-nil pointers with its own zone.
	t.Run("same-state namesake returns non-nil pointers", func(t *testing.T) {
		lat, lng, tz := LookupPointers(g, "Pasadena", "MD", "")
		if lat == nil || lng == nil || tz == nil {
			t.Fatalf("expected non-nil pointers for Pasadena,MD, got lat=%v lng=%v tz=%v", lat, lng, tz)
		}
		if *tz != "America/New_York" {
			t.Errorf("tz = %q, want America/New_York", *tz)
		}
	})
}

// TestResolveUSState pins the unambiguous-vs-ambiguous contract that keeps the
// PSY-1244 bug dead: a multi-state namesake must NOT resolve to any state.
func TestResolveUSState(t *testing.T) {
	g := Default()
	tests := []struct {
		name       string
		city       string
		wantState  string
		wantStatus USStateStatus
	}{
		{"chicago is unambiguously IL", "Chicago", "IL", USStateUnambiguous},
		{"los angeles is unambiguously CA", "Los Angeles", "CA", USStateUnambiguous},
		{"case and space insensitive", "  chicago ", "IL", USStateUnambiguous},
		// Portland spans OR and ME in the dataset (see TestResolve) — the exact
		// namesake collision PSY-1244 mis-resolved to the bigger city.
		{"portland is ambiguous (OR/ME)", "Portland", "", USStateAmbiguous},
		// Cambridge is US-internally only MA, but the UK Cambridge is the bigger
		// namesake — a band tagged just "Cambridge" must NOT be stamped MA (the
		// empty-country international-city corruption an earlier draft allowed).
		{"cambridge is internationally dominant (UK) → not found", "Cambridge", "", USStateNotFound},
		// Paris resolves to one US state (TX) but is dominantly Paris, France.
		{"paris is internationally dominant (FR) → not found", "Paris", "", USStateNotFound},
		// PSY-1276: once "St. Petersburg" folds together with the larger Russian
		// "Saint Petersburg", a bare query is internationally dominant (like
		// Cambridge/Paris) and must NOT be stamped FL — the fix removed an
		// accidental pre-fix exception where the abbreviation isolated the FL row.
		{"st. petersburg is internationally dominant (RU) → not found", "St. Petersburg", "", USStateNotFound},
		// A non-US city has no US state, even though GeoNames gives it an admin1.
		{"tokyo is not a US place", "Tokyo", "", USStateNotFound},
		{"unknown city is not found", "Nowherecityville", "", USStateNotFound},
		{"empty city is not found", "", "", USStateNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, status := g.ResolveUSState(tt.city)
			if status != tt.wantStatus {
				t.Fatalf("ResolveUSState(%q) status=%d, want %d", tt.city, status, tt.wantStatus)
			}
			if state != tt.wantState {
				t.Errorf("ResolveUSState(%q) state=%q, want %q", tt.city, state, tt.wantState)
			}
		})
	}
}

// TestResolveMetro pins the CBSA rollup that the Atlas scenes need: suburbs and
// boroughs share one metro key, and the state disambiguates same-named cities.
func TestResolveMetro(t *testing.T) {
	g := Default()
	tests := []struct {
		name              string
		city, state, ctry string
		wantCode          string
		wantOK            bool
	}{
		{"pasadena CA rolls up to LA", "Pasadena", "CA", "", "31080", true},
		{"pasadena TX rolls up to Houston", "Pasadena", "TX", "", "26420", true},
		{"santa monica (suburb) → LA", "Santa Monica", "CA", "", "31080", true},
		{"brooklyn (borough) → NYC", "Brooklyn", "NY", "", "35620", true},
		// NYC's consolidated GeoNames entry has no county FIPS; gen_cities.py maps
		// it (and the common "New York" form) to the NYC metro — the largest scene.
		{"new york city → NYC metro", "New York City", "NY", "", "35620", true},
		{"new york (common form) → NYC metro", "New York", "NY", "", "35620", true},
		// PSY-1276: "St. Paul" (abbreviated) must roll up to the Twin Cities CBSA
		// like the dataset's full "Saint Paul" — before the foldKey fix it missed
		// and orphaned into a phantom 0-roster scene.
		{"st. paul (abbrev) → Twin Cities", "St. Paul", "MN", "", "33460", true},
		{"saint paul (full) → Twin Cities", "Saint Paul", "MN", "", "33460", true},
		// Other ticket-named abbreviated cities, pinned by state. Mixed storage:
		// "St. Louis" is abbreviated in the dataset, "Fort Worth"/"Fort Lauderdale"
		// are full — both forms resolve through the symmetric fold.
		{"ft. worth → Dallas–Fort Worth", "Ft. Worth", "TX", "", "19100", true},
		{"st. louis → St. Louis CBSA", "St. Louis", "MO", "", "41180", true},
		{"ft. lauderdale → Miami CBSA", "Ft. Lauderdale", "FL", "", "33100", true},
		{"st. petersburg FL (pinned) → Tampa CBSA", "St. Petersburg", "FL", "", "45300", true},
		// Bare "St. Petersburg" (no state) now folds together with the far larger
		// Russian "Saint Petersburg", so it's internationally dominant and REFUSES a
		// US metro — consistent with the Cambridge/Paris policy below. (Pre-fix it
		// accidentally resolved to FL because the abbreviation isolated it; PSY-1276.)
		{"bare st. petersburg → refuse (Russia dominant)", "St. Petersburg", "", "", "", false},
		{"oakland → SF", "Oakland", "CA", "", "41860", true},
		{"chicago → Chicago CBSA", "Chicago", "IL", "", "16980", true},
		{"phoenix → Phoenix CBSA", "Phoenix", "AZ", "", "38060", true},
		// Unambiguous + US-dominant name resolves WITHOUT an explicit state.
		{"chicago resolves without a state", "Chicago", "", "", "16980", true},
		// REFUSE the population guess for a multi-state name with no pinning state
		// (the PSY-1244 trap) — must return false, not "Houston".
		{"bare pasadena (ambiguous) → refuse", "Pasadena", "", "", "", false},
		{"bare springfield (ambiguous) → refuse", "Springfield", "", "", "", false},
		// A state that pins NO namesake of the city must refuse, not fall back. FL is
		// a valid state with no Pasadena row → bestCity's hard filter misses; ZZ isn't
		// a US state → admin1 is empty, so the bare-name ResolveUSState ambiguity
		// check refuses instead.
		{"valid state pins no namesake → miss", "Pasadena", "FL", "", "", false},
		{"bogus state → refuse", "Pasadena", "ZZ", "", "", false},
		// Non-US place: CBSA is US-only, so no metro (caller falls back to city).
		{"london GB has no CBSA", "London", "", "GB", "", false},
		{"unknown city → no metro", "Nowherecityville", "", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, ok := g.ResolveMetro(tt.city, tt.state, tt.ctry)
			if ok != tt.wantOK {
				t.Fatalf("ResolveMetro(%q,%q,%q) ok=%v, want %v", tt.city, tt.state, tt.ctry, ok, tt.wantOK)
			}
			if ok {
				if m.CBSACode != tt.wantCode {
					t.Errorf("CBSACode = %q, want %q (name=%q)", m.CBSACode, tt.wantCode, m.Name)
				}
				if m.Name == "" {
					t.Errorf("a resolved metro must have a friendly name, got empty for %q", tt.city)
				}
			}
		})
	}
}

func TestMetroPrincipalByCBSA(t *testing.T) {
	tests := []struct {
		name, cbsa, wantCity, wantState string
		wantOK                          bool
	}{
		{"NYC principal", "35620", "New York City", "NY", true},
		{"LA principal", "31080", "Los Angeles", "CA", true},
		{"Chicago principal", "16980", "Chicago", "IL", true},
		{"Phoenix principal", "38060", "Phoenix", "AZ", true},
		// Twin Cities: the highest-population member (Minneapolis) is the principal,
		// even though the CBSA title also names St. Paul + Bloomington.
		{"Twin Cities → Minneapolis", "33460", "Minneapolis", "MN", true},
		{"unknown code → not found", "00000", "", "", false},
		{"empty code → not found", "", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mp, ok := MetroPrincipalByCBSA(tt.cbsa)
			if ok != tt.wantOK {
				t.Fatalf("MetroPrincipalByCBSA(%q) ok=%v, want %v", tt.cbsa, ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if mp.City != tt.wantCity || mp.State != tt.wantState {
				t.Errorf("got %q,%q want %q,%q", mp.City, mp.State, tt.wantCity, tt.wantState)
			}
			if mp.CBSACode != tt.cbsa {
				t.Errorf("CBSACode = %q, want %q", mp.CBSACode, tt.cbsa)
			}
			if mp.Name == "" {
				t.Errorf("a resolved metro must carry its friendly name")
			}
			if mp.Latitude == 0 || mp.Longitude == 0 {
				t.Errorf("principal city must have coordinates, got (%v,%v)", mp.Latitude, mp.Longitude)
			}
		})
	}
}

func TestMetroMemberPlaces(t *testing.T) {
	t.Run("NYC includes Brooklyn and New York alias", func(t *testing.T) {
		places, ok := MetroMemberPlaces("35620")
		if !ok {
			t.Fatal("expected NYC metro members")
		}
		if len(places) < 10 {
			t.Fatalf("expected a substantial member list, got %d", len(places))
		}
		has := func(city, state string) bool {
			for _, p := range places {
				if strings.EqualFold(p.City, city) && strings.EqualFold(p.State, state) {
					return true
				}
			}
			return false
		}
		if !has("Brooklyn", "NY") {
			t.Error("Brooklyn, NY missing from NYC metro members")
		}
		if !has("New York", "NY") {
			t.Error("New York, NY alias missing from NYC metro members")
		}
		if !has("New York City", "NY") {
			t.Error("New York City, NY missing from NYC metro members")
		}
	})
	t.Run("unknown CBSA", func(t *testing.T) {
		_, ok := MetroMemberPlaces("00000")
		if ok {
			t.Fatal("expected not found")
		}
	})
}

func TestPlaceMatchBindVariants(t *testing.T) {
	variants := placeMatchBindVariants("Saint Paul")
	has := func(want string) bool {
		for _, v := range variants {
			if v == want {
				return true
			}
		}
		return false
	}
	if !has("Saint Paul") || !has("St. Paul") {
		t.Fatalf("expected Saint/St. variants, got %v", variants)
	}
}

func TestFoldKey(t *testing.T) {
	cases := map[string]string{
		"  Saint-Louis ": "saint louis",
		"Zürich":         "zurich",
		"São Paulo":      "sao paulo",
		"Montréal":       "montreal",
		// PSY-1276: St./Ft./Mt. expand to Saint/Fort/Mount so a contributor's
		// abbreviation folds to the same key as the dataset's full form.
		// (St. John's: apostrophe → space, so "John's" → "john s"; the trailing
		// "s" is not an abbreviation and stays — matches the dataset's "Saint
		// John's" folding the same way.)
		"St. John's":   "saint john s",
		"St. Paul":     "saint paul",
		"Ft. Worth":    "fort worth",
		"Mt. Pleasant": "mount pleasant",
		"St Paul":      "saint paul", // period optional — foldKey drops it either way
		// Whole-token only: a prefix that merely starts with the abbreviation is
		// one token and must NOT expand.
		"Stockton":   "stockton",
		"Fortuna":    "fortuna",
		"Montgomery": "montgomery",
		// Decomposable diacritics NFKD splits into base+combining-mark, which the
		// mark stripper removes (refutes the "ç/ñ get dropped" review concern).
		"Korçë": "korce",
		"Ñuñoa": "nunoa",
		// Non-decomposable Latin letters handled by the specialLetters map.
		"Łódź":   "lodz",
		"Tromsø": "tromso",
		"Gießen": "giessen",
	}
	for in, want := range cases {
		if got := foldKey(in); got != want {
			t.Errorf("foldKey(%q)=%q, want %q", in, got, want)
		}
	}
}
