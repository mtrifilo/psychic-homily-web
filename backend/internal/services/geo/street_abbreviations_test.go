package geo

import "testing"

// The expansion exists because Nominatim resolves what OSM actually stores:
// "75 M.L.K. Jr Dr SW" returns nothing, "75 Martin Luther King Jr Dr SW"
// resolves rooftop (verified against live Nominatim, PSY-1607 investigation).
//
// The negative cases matter more than the positive one. This runs on the shared
// path used by BOTH the inline venue write and the scheduled sweep, so a wrong
// expansion mangles addresses that currently resolve — a regression affecting
// venues nobody was complaining about.
func TestExpandStreetAbbreviations(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "the observed miss — venue 128, Altar at the Masquerade",
			in:   "75 M.L.K. Jr Dr SW",
			want: "75 Martin Luther King Jr Dr SW",
		},
		{
			name: "unpunctuated form",
			in:   "75 MLK Jr Dr SW",
			want: "75 Martin Luther King Jr Dr SW",
		},
		{
			name: "case-insensitive",
			in:   "75 m.l.k. Jr Dr SW",
			want: "75 Martin Luther King Jr Dr SW",
		},
		{
			name: "trailing comma is preserved, not swallowed",
			in:   "75 M.L.K., Atlanta",
			want: "75 Martin Luther King, Atlanta",
		},

		// --- MUST NOT expand ---
		{
			// The AC requires a case that must NOT be expanded. "St" is the reason
			// the table is narrow: it means Street here, and expanding it would
			// produce "Main Saint", which resolves to nothing. A venue that
			// geocodes fine today would start missing.
			name: "trailing St stays Street, never Saint",
			in:   "123 Main St",
			want: "123 Main St",
		},
		{
			name: "St. at the head is also left alone — deliberately out of scope",
			in:   "12 St. Marks Pl",
			want: "12 St. Marks Pl",
		},
		{
			name: "substring match must not fire",
			in:   "9 Mlkovic Way",
			want: "9 Mlkovic Way",
		},
		{
			name: "address with nothing to expand is returned unchanged",
			in:   "1234 Elm Street",
			want: "1234 Elm Street",
		},
		{
			name: "empty stays empty",
			in:   "",
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ExpandStreetAbbreviations(tc.in); got != tc.want {
				t.Fatalf("ExpandStreetAbbreviations(%q)\n got: %q\nwant: %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestWithStreetLeavesScopingIntact: the fallback retries with the raw street
// while keeping city/state/zip/country, or a common street name would resolve in
// the wrong place.
func TestWithStreetLeavesScopingIntact(t *testing.T) {
	q := AddressQuery{
		Street:  "75 Martin Luther King Jr Dr SW",
		City:    "Atlanta",
		State:   "GA",
		Zipcode: "30303",
		Country: "US",
	}
	got := q.WithStreet("75 M.L.K. Jr Dr SW")

	if got.Street != "75 M.L.K. Jr Dr SW" {
		t.Fatalf("street = %q", got.Street)
	}
	if got.City != q.City || got.State != q.State || got.Zipcode != q.Zipcode || got.Country != q.Country {
		t.Fatalf("scoping fields changed: %+v", got)
	}
	if q.Street != "75 Martin Luther King Jr Dr SW" {
		t.Fatal("WithStreet must not mutate the receiver")
	}
}
