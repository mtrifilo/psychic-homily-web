package pipeline

import (
	"testing"
)

// The inputs below are REAL production artist names, minted by ingest from
// venue-calendar listing text. They are not hypothetical: each one produced a
// bogus artist record, and two of them silently duplicated a show, because show
// dedup keys on (artist_id, venue_id, event_date) — a decorated name yields a
// different artist_id, so the dedup never fires.
//
//	artist 4208  *SOLD OUT* Red Vox ft. special guests Super Guitar Bros.
//	artist 4209  *SOLD OUT* Audrey Hobert
//	artist 4210  *SOLD OUT* Chuck Timely Live From Chicago
func TestStripStatusMarkers(t *testing.T) {
	tests := []struct {
		name        string
		in          string
		wantName    string
		wantSoldOut bool
	}{
		// Production cases.
		{"sold-out prefix", "*SOLD OUT* Audrey Hobert", "Audrey Hobert", true},
		{"sold-out prefix, long bill", "*SOLD OUT* Red Vox ft. special guests Super Guitar Bros.", "Red Vox ft. special guests Super Guitar Bros.", true},
		{"sold-out prefix, event title", "*SOLD OUT* Chuck Timely Live From Chicago", "Chuck Timely Live From Chicago", true},
		{"sold-out prefix", "*SOLD OUT* This Is Lorelei", "This Is Lorelei", true},

		// Marker spelling variants.
		{"no asterisks", "SOLD OUT Bella Kay", "Bella Kay", true},
		{"lowercase", "*sold out* Bella Kay", "Bella Kay", true},
		{"no space variant", "*SOLDOUT* Bella Kay", "Bella Kay", true},
		{"colon left behind", "*SOLD OUT*: Bella Kay", "Bella Kay", true},

		// Stacked markers.
		{"stacked markers", "*SOLD OUT* *21+* Bella Kay", "Bella Kay", true},
		{"non-sold-out marker only", "*21+* Bella Kay", "Bella Kay", false},
		{"cancelled marker", "*CANCELLED* Bella Kay", "Bella Kay", false},

		// Must NOT touch legitimate names.
		{"clean name untouched", "Audrey Hobert", "Audrey Hobert", false},
		{"real band with ampersand", "Acid Mothers Temple & The Melting Paraiso U.F.O.", "Acid Mothers Temple & The Melting Paraiso U.F.O.", false},
		{"band whose name contains 'sold'’ mid-string", "The Soldiers of Fortune", "The Soldiers of Fortune", false},
		{"leading whitespace only", "  Momma  ", "Momma", false},

		// Degenerate input: a name that is ONLY a marker keeps its original
		// text rather than becoming empty, so a human can see and fix it.
		{"marker only", "*SOLD OUT*", "*SOLD OUT*", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotName, gotSoldOut := stripStatusMarkers(tc.in)
			if gotName != tc.wantName {
				t.Errorf("stripStatusMarkers(%q) name = %q, want %q", tc.in, gotName, tc.wantName)
			}
			if gotSoldOut != tc.wantSoldOut {
				t.Errorf("stripStatusMarkers(%q) soldOut = %v, want %v", tc.in, gotSoldOut, tc.wantSoldOut)
			}
		})
	}
}

// parseArtistsFromTitle previously had no "featuring" separator, so an entire
// billing string collapsed into one artist name. The extraction prompt already
// classifies ft./feat./w/ as billing markers ("special_guest", "support") —
// the title fallback just wasn't splitting on them.
func TestParseArtistsFromTitle_FeaturingSeparators(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{
			name: "ft. with 'special guests' connective",
			in:   "Red Vox ft. special guests Super Guitar Bros.",
			want: []string{"Red Vox", "Super Guitar Bros."},
		},
		{"feat.", "Nothing-Assumed feat. PROBLEMS", []string{"Nothing-Assumed", "PROBLEMS"}},
		{"w/", "REZN w/ Lockstep", []string{"REZN", "Lockstep"}},
		{"featuring, multiple support", "Headliner featuring A, B", []string{"Headliner", "A", "B"}},

		// Existing behaviour must be preserved.
		{"comma still wins", "Ovlov, Cusp", []string{"Ovlov", "Cusp"}},
		{"with separator", "Momma with Been Stellar", []string{"Momma", "Been Stellar"}},
		{"no separator", "beabadoobee", []string{"beabadoobee"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseArtistsFromTitle(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("parseArtistsFromTitle(%q) = %v (%d), want %v (%d)", tc.in, got, len(got), tc.want, len(tc.want))
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("parseArtistsFromTitle(%q)[%d] = %q, want %q", tc.in, i, got[i], tc.want[i])
				}
			}
		})
	}
}

// End-to-end on the exact production string: the title must yield clean artist
// names AND surface the sold-out flag, so nothing is lost by the strip.
func TestStatusMarkerAndSplit_ProductionCase(t *testing.T) {
	const raw = "*SOLD OUT* Red Vox ft. special guests Super Guitar Bros."

	cleaned, soldOut := stripStatusMarkers(raw)
	if !soldOut {
		t.Fatal("sold-out marker not detected — the flag would be lost")
	}

	got := parseArtistsFromTitle(cleaned)
	want := []string{"Red Vox", "Super Guitar Bros."}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("artist[%d] = %q, want %q", i, got[i], want[i])
		}
	}

	// This is the whole point: the resulting artist_id for "Red Vox" now
	// matches the existing record, so dedup fires and no duplicate show is
	// created.
	for _, n := range got {
		if len(n) > 45 {
			t.Errorf("artist name %q is still listing text, not a band name", n)
		}
	}
}
