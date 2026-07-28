package utils

import "testing"

func TestNormalizeVenueName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		// The production case. These two rows coexisted in Minneapolis and
		// silently duplicated ~90 shows.
		{"abbreviated street", "7th St Entry", "7th street entry"},
		{"expanded street", "7th Street Entry", "7th street entry"},

		{"case only", "EMPTY BOTTLE", "empty bottle"},
		{"leading article", "The Empty Bottle", "empty bottle"},
		{"punctuation", "Schubas Tavern.", "schubas tavern"},
		{"apostrophe", "Mo's Irish Pub", "mos irish pub"},
		{"ampersand word", "Haute & Freddy", "haute and freddy"},
		{"ampersand spelled", "Haute and Freddy", "haute and freddy"},
		{"theatre spelling", "The Nile Theatre", "nile theater"},
		{"theater spelling", "The Nile Theater", "nile theater"},
		{"bros", "The Hoyle Bros", "hoyle brothers"},
		{"brothers", "Hoyle Brothers", "hoyle brothers"},
		{"whitespace collapse", "  Salt   Shed  ", "salt shed"},
		{"empty", "", ""},
		{"whitespace only", "   ", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeVenueName(tc.in); got != tc.want {
				t.Errorf("NormalizeVenueName(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// The whole point of the function: variants of one room collapse together,
// while genuinely distinct rooms stay apart. The must-NOT-merge cases are the
// load-bearing half — a false merge destroys data, a missed merge only leaves a
// duplicate for a human to find.
func TestNormalizeVenueName_MergesAndSeparates(t *testing.T) {
	sameRoom := [][2]string{
		{"7th St Entry", "7th Street Entry"},
		{"The Empty Bottle", "Empty Bottle"},
		{"Schubas Tavern", "Schuba's Tavern"},
		{"The Nile Theatre", "Nile Theater"},
		{"Turner Hall Ballroom", "turner hall ballroom"},
	}
	for _, p := range sameRoom {
		t.Run("same/"+p[0], func(t *testing.T) {
			a, b := NormalizeVenueName(p[0]), NormalizeVenueName(p[1])
			if a != b {
				t.Errorf("expected %q and %q to normalize alike, got %q vs %q", p[0], p[1], a, b)
			}
		})
	}

	differentRooms := [][2]string{
		// Same operator, genuinely separate rooms — the canonical must-not-merge.
		{"Salt Shed", "Salt Shed Fairgrounds"},
		{"Elsewhere Hall", "Elsewhere Zone One"},
		{"Elsewhere Zone One", "Elsewhere Rooftop"},
		{"Middle East Downstairs", "Middle East Upstairs"},
		{"Metro Gallery", "Metro Baltimore"},
		{"7th Street Entry", "First Avenue"},
		{"Thalia Hall", "Turner Hall Ballroom"},
	}
	for _, p := range differentRooms {
		t.Run("different/"+p[0]+" vs "+p[1], func(t *testing.T) {
			a, b := NormalizeVenueName(p[0]), NormalizeVenueName(p[1])
			if a == b {
				t.Errorf("MERGED DISTINCT VENUES: %q and %q both normalize to %q", p[0], p[1], a)
			}
		})
	}
}

// Ambiguous abbreviations are excluded on purpose; pin that so nobody adds them
// back without reading why.
func TestNormalizeVenueName_LeavesAmbiguousAbbreviationsAlone(t *testing.T) {
	cases := map[string]string{
		"Rock N Roll Hotel": "rock n roll hotel", // n → and/north
		"Dr Feelgoods":      "dr feelgoods",      // dr → drive/doctor
		"Brewing Co":        "brewing co",        // co → company/county
		"Lincoln Pk Lodge":  "lincoln pk lodge",  // pk → park/peak
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			if got := NormalizeVenueName(in); got != want {
				t.Errorf("NormalizeVenueName(%q) = %q, want %q (ambiguous abbreviations must pass through)", in, got, want)
			}
		})
	}
}
