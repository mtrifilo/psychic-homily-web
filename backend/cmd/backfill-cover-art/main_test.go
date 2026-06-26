package main

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"psychic-homily-backend/internal/services/pipeline"
)

// credit builds an MBArtistCredit with the given credited + canonical names.
func credit(creditedName, canonicalName string) pipeline.MBArtistCredit {
	var ac pipeline.MBArtistCredit
	ac.Name = creditedName
	ac.Artist.Name = canonicalName
	return ac
}

func TestFlattenArtistNames(t *testing.T) {
	// Credited == canonical → a single name (no duplicate).
	assert.Equal(t, []string{"Sleep"},
		flattenArtistNames([]pipeline.MBArtistCredit{credit("Sleep", "Sleep")}))

	// Credited != canonical → both, credited first.
	assert.Equal(t, []string{"Sleep feat. Guest", "Sleep"},
		flattenArtistNames([]pipeline.MBArtistCredit{credit("Sleep feat. Guest", "Sleep")}))

	// Empty credited name is skipped; the canonical is still emitted.
	assert.Equal(t, []string{"Sleep"},
		flattenArtistNames([]pipeline.MBArtistCredit{credit("", "Sleep")}))

	// Multiple credits are flattened in order.
	assert.Equal(t, []string{"A", "B credited", "B"},
		flattenArtistNames([]pipeline.MBArtistCredit{credit("A", "A"), credit("B credited", "B")}))

	// A fully empty credit contributes no names.
	assert.Empty(t, flattenArtistNames([]pipeline.MBArtistCredit{credit("", "")}))
}
