package geo

import "testing"

func TestCountryToISO(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"US", "US", true},
		{"us", "US", true},
		{"USA", "US", true},
		{"United States", "US", true},
		{"Japan", "JP", true},
		{"JP", "JP", true},
		{"Italy", "IT", true},
		{"Georgia", "GE", true}, // the COUNTRY (distinct from the US state GA)
		{"  United Kingdom  ", "GB", true},
		{"Nowhereland", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := CountryToISO(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("CountryToISO(%q) = %q,%v want %q,%v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestCanonicalCountryName(t *testing.T) {
	// Different source spellings of the same country collapse to one display form.
	for _, in := range []string{"US", "USA", "United States", "us"} {
		got, ok := CanonicalCountryName(in)
		if !ok || got != "United States" {
			t.Errorf("CanonicalCountryName(%q) = %q,%v want United States", in, got, ok)
		}
	}
	if got, ok := CanonicalCountryName("JP"); !ok || got != "Japan" {
		t.Errorf("CanonicalCountryName(JP) = %q,%v want Japan", got, ok)
	}
	// Leading article stripped: GeoNames stores "The Netherlands".
	for _, in := range []string{"Netherlands", "NL", "The Netherlands"} {
		if got, ok := CanonicalCountryName(in); !ok || got != "Netherlands" {
			t.Errorf("CanonicalCountryName(%q) = %q,%v want Netherlands", in, got, ok)
		}
	}
	if _, ok := CanonicalCountryName("Nowhereland"); ok {
		t.Error("expected miss for unrecognized country")
	}
}
