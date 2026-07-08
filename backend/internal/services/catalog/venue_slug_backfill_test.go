package catalog

import "testing"

// venueSlugTarget is the pure decision core of BackfillVenueSlugs — no DB. The
// `slugTaken` closure lets us simulate an already-used candidate slug.
func TestVenueSlugTarget(t *testing.T) {
	none := func(string) bool { return false }

	cases := []struct {
		name        string
		vName       string
		city        string
		state       string
		current     string
		slugTaken   func(string) bool
		wantTarget  string
		wantChanged bool
	}{
		{
			// The real Valley Bar corruption: first char of each word dropped +
			// missing state suffix.
			name:        "drop-first-char corruption is rewritten",
			vName:       "Valley Bar", city: "Phoenix", state: "AZ",
			current:     "alley-ar-hoenix",
			slugTaken:   none,
			wantTarget:  "valley-bar-phoenix-az",
			wantChanged: true,
		},
		{
			name:        "empty stored slug is rewritten",
			vName:       "Palo Verde Lounge", city: "Tempe", state: "AZ",
			current:     "",
			slugTaken:   none,
			wantTarget:  "palo-verde-lounge-tempe-az",
			wantChanged: true,
		},
		{
			name:        "already-canonical slug is left unchanged (fast path, no probe)",
			vName:       "Empty Bottle", city: "Chicago", state: "IL",
			current:     "empty-bottle-chicago-il",
			slugTaken:   func(string) bool { t.Fatal("slugTaken must not be probed on the fast path"); return false },
			wantTarget:  "empty-bottle-chicago-il",
			wantChanged: false,
		},
		{
			// A legitimate "-2" uniqueness suffix (the canonical base is taken by a
			// different venue). Must be treated as already-correct, not rewritten.
			name:        "legit -2 uniqueness suffix is idempotent",
			vName:       "The Venue", city: "Phoenix", state: "AZ",
			current:     "the-venue-phoenix-az-2",
			slugTaken:   func(c string) bool { return c == "the-venue-phoenix-az" }, // base taken by another venue
			wantTarget:  "the-venue-phoenix-az-2",
			wantChanged: false,
		},
		{
			// Corrupted slug on a venue whose canonical base is taken → gets the
			// next free suffix.
			name:        "corrupted slug resolves around a taken canonical base",
			vName:       "The Venue", city: "Phoenix", state: "AZ",
			current:     "he-venue-hoenix",
			slugTaken:   func(c string) bool { return c == "the-venue-phoenix-az" },
			wantTarget:  "the-venue-phoenix-az-2",
			wantChanged: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			target, changed := venueSlugTarget(c.vName, c.city, c.state, c.current, c.slugTaken)
			if target != c.wantTarget {
				t.Errorf("target = %q, want %q", target, c.wantTarget)
			}
			if changed != c.wantChanged {
				t.Errorf("needsUpdate = %v, want %v", changed, c.wantChanged)
			}
		})
	}
}
