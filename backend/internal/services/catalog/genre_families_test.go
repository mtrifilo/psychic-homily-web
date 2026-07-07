package catalog

import (
	"testing"

	"psychic-homily-backend/internal/services/contracts"
)

func gc(slug string, count int) contracts.GenreCount {
	return contracts.GenreCount{Slug: slug, Count: count}
}

func TestDominantGenreFamily(t *testing.T) {
	tests := []struct {
		name   string
		counts []contracts.GenreCount
		want   string
	}{
		{
			// The dev-catalog shape: punk-family slugs dominate by a wide margin.
			name:   "punk-heavy scene rolls up to punk family",
			counts: []contracts.GenreCount{gc("punk", 45), gc("garage-punk", 22), gc("post-punk", 15), gc("rock", 17)},
			want:   genreFamilyPunk,
		},
		{
			// Several slugs of one family sum: post-punk + hardcore-punk + punk all
			// roll to punk_hardcore, clearing 40% together even though "rock" leads
			// any single punk slug.
			name:   "family rollup sums sibling slugs",
			counts: []contracts.GenreCount{gc("post-punk", 3), gc("hardcore-punk", 3), gc("punk", 3), gc("rock", 8)},
			want:   genreFamilyPunk, // punk mass 9 of 17 = 53% > rock 8
		},
		{
			name:   "no family reaches 40% -> neutral",
			counts: []contracts.GenreCount{gc("punk", 3), gc("rock", 3), gc("electronic", 2), gc("folk", 2)},
			want:   "", // punk 3/10 = 30%, rock 3/10 = 30%
		},
		{
			name:   "exactly 40% share tints (inclusive)",
			counts: []contracts.GenreCount{gc("punk", 4), gc("rock", 3), gc("folk", 3)},
			want:   genreFamilyPunk, // 4/10 = 40%
		},
		{
			// Unmapped genres stay in the denominator, so a scene mostly tagged with
			// them does NOT tint even if the top MAPPED family leads.
			name:   "unmapped genres dilute the share",
			counts: []contracts.GenreCount{gc("punk", 3), gc("cumbia", 4), gc("lukthung", 3)},
			want:   "", // punk 3/10 = 30%; cumbia/lukthung are unmapped
		},
		{
			name:   "mapped family beats unmapped when it clears the bar",
			counts: []contracts.GenreCount{gc("punk", 5), gc("cumbia", 5)},
			want:   genreFamilyPunk, // 5/10 = 50%
		},
		{
			name:   "below the tagged-artist floor -> neutral even at 100%",
			counts: []contracts.GenreCount{gc("punk", 2), gc("hardcore", 2)},
			want:   "", // total 4 < dominantGenreFamilyMinTagged (5)
		},
		{
			name:   "at the tagged-artist floor with a clear leader tints",
			counts: []contracts.GenreCount{gc("punk", 5)},
			want:   genreFamilyPunk, // total 5 == floor, 100%
		},
		{
			name:   "empty distribution -> neutral",
			counts: nil,
			want:   "",
		},
		{
			name:   "zero/negative counts are ignored, not summed",
			counts: []contracts.GenreCount{gc("punk", 0), gc("rock", 0)},
			want:   "", // nothing counts -> total 0 < floor
		},
		{
			// electronic rolls up from its wave/synth slugs.
			name:   "electronic family from synth/wave slugs",
			counts: []contracts.GenreCount{gc("coldwave", 4), gc("synth-pop", 3), gc("new-wave", 2), gc("punk", 1)},
			want:   genreFamilyElectronic, // 9/10 = 90%
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dominantGenreFamily(tt.counts); got != tt.want {
				t.Errorf("dominantGenreFamily(%v) = %q, want %q", tt.counts, got, tt.want)
			}
		})
	}
}

// TestDominantGenreFamily_TieIsDeterministic pins the tie-break: two families with
// equal mass resolve to whichever comes first in genreFamilyKeys, never a
// map-iteration coin flip.
func TestDominantGenreFamily_TieIsDeterministic(t *testing.T) {
	// punk (index 0) and electronic (index 3) each hold 5 of 10.
	counts := []contracts.GenreCount{gc("punk", 5), gc("electronic", 5)}
	for i := 0; i < 50; i++ {
		if got := dominantGenreFamily(counts); got != genreFamilyPunk {
			t.Fatalf("tie must resolve to the earliest family key (%q), got %q on iteration %d",
				genreFamilyPunk, got, i)
		}
	}
}

// TestGenreFamilyMapIntegrity guards the cross-layer contract: every family a slug
// maps to must be a declared key (so it can't emit a family the frontend legend
// doesn't know), and the ordered key list must have no duplicates and cover every
// family a slug can map to.
func TestGenreFamilyMapIntegrity(t *testing.T) {
	known := make(map[string]bool, len(genreFamilyKeys))
	for _, k := range genreFamilyKeys {
		if known[k] {
			t.Errorf("duplicate family key in genreFamilyKeys: %q", k)
		}
		known[k] = true
	}
	for slug, fam := range genreFamilyBySlug {
		if !known[fam] {
			t.Errorf("slug %q maps to family %q which is not in genreFamilyKeys", slug, fam)
		}
	}
}
