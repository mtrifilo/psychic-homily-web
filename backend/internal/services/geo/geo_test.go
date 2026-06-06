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
