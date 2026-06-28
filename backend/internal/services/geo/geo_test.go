package geo

import "testing"

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
		{"berlin germany", "Berlin", "", "Germany", "Europe/Berlin", true},
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
		{"oakland → SF", "Oakland", "CA", "", "41860", true},
		{"chicago → Chicago CBSA", "Chicago", "IL", "", "16980", true},
		{"phoenix → Phoenix CBSA", "Phoenix", "AZ", "", "38060", true},
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

func TestFoldKey(t *testing.T) {
	cases := map[string]string{
		"  Saint-Louis ": "saint louis",
		"Zürich":         "zurich",
		"São Paulo":      "sao paulo",
		"Montréal":       "montreal",
		"St. John's":     "st john s",
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
