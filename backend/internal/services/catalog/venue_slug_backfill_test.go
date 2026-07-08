package catalog

import "testing"

// slugLooksCorrupt is the corruption-signature gate: it must flag the historical
// empty / dropped-location-tail slugs while leaving every well-formed slug
// alone — including a renamed venue's deliberately-stable slug and a "-N"
// uniqueness suffix.
func TestSlugLooksCorrupt(t *testing.T) {
	cases := []struct {
		name  string
		slug  string
		city  string
		state string
		want  bool
	}{
		{"empty slug is corrupt", "", "Phoenix", "AZ", true},
		{"dropped-char + missing state is corrupt", "alley-ar-hoenix", "Phoenix", "AZ", true},
		{"dropped-char missing state (crescent)", "rescent-allroom-hoenix", "Phoenix", "AZ", true},
		{"canonical slug is fine", "valley-bar-phoenix-az", "Phoenix", "AZ", false},
		{"renamed venue keeps a valid tail — not corrupt", "the-rogue-bar-phoenix-az", "Phoenix", "AZ", false},
		{"legit -2 uniqueness suffix is not corrupt", "the-venue-phoenix-az-2", "Phoenix", "AZ", false},
		{"multi-digit dedup suffix is not corrupt", "venue-phoenix-az-42", "Phoenix", "AZ", false},
		{"exactly the location tail is not corrupt", "phoenix-az", "Phoenix", "AZ", false},
		{"non-numeric trailing segment does not count as tail", "valley-bar-phoenix-az-annex", "Phoenix", "AZ", true},
		{"empty state falls back to city tail", "berlin-club-berlin", "Berlin", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := slugLooksCorrupt(c.slug, c.city, c.state); got != c.want {
				t.Errorf("slugLooksCorrupt(%q, %q, %q) = %v, want %v", c.slug, c.city, c.state, got, c.want)
			}
		})
	}
}

// venueSlugTarget is the pure decision core of BackfillVenueSlugs — no DB. The
// `slugTaken` closure lets us simulate an already-used candidate slug.
func TestVenueSlugTarget(t *testing.T) {
	none := func(string) bool { return false }
	mustNotProbe := func(t *testing.T) func(string) bool {
		return func(string) bool { t.Fatal("slugTaken must not be probed for a well-formed slug"); return false }
	}

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
			name: "drop-first-char corruption is rewritten",
			vName: "Valley Bar", city: "Phoenix", state: "AZ",
			current:     "alley-ar-hoenix",
			slugTaken:   none,
			wantTarget:  "valley-bar-phoenix-az",
			wantChanged: true,
		},
		{
			name: "empty stored slug is rewritten",
			vName: "Palo Verde Lounge", city: "Tempe", state: "AZ",
			current:     "",
			slugTaken:   none,
			wantTarget:  "palo-verde-lounge-tempe-az",
			wantChanged: true,
		},
		{
			name: "already-canonical slug is left unchanged (no probe)",
			vName: "Empty Bottle", city: "Chicago", state: "IL",
			current:     "empty-bottle-chicago-il",
			slugTaken:   mustNotProbe(t),
			wantTarget:  "empty-bottle-chicago-il",
			wantChanged: false,
		},
		{
			// Finding #2 regression: UpdateVenue never regenerates the slug on
			// rename, so a renamed venue's stable slug must NOT be rewritten even
			// though it no longer matches the current name.
			name: "renamed venue's stable slug is left untouched",
			vName: "The Rebel Lounge", city: "Phoenix", state: "AZ",
			current:     "the-rogue-bar-phoenix-az",
			slugTaken:   mustNotProbe(t),
			wantTarget:  "the-rogue-bar-phoenix-az",
			wantChanged: false,
		},
		{
			// A legitimate "-2" uniqueness suffix must be treated as well-formed.
			name: "legit -2 uniqueness suffix is idempotent",
			vName: "The Venue", city: "Phoenix", state: "AZ",
			current:     "the-venue-phoenix-az-2",
			slugTaken:   mustNotProbe(t),
			wantTarget:  "the-venue-phoenix-az-2",
			wantChanged: false,
		},
		{
			// Corrupted slug whose canonical base is taken → gets the next suffix.
			name: "corrupted slug resolves around a taken canonical base",
			vName: "The Venue", city: "Phoenix", state: "AZ",
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
