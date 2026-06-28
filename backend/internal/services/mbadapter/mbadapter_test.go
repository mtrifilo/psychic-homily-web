package mbadapter

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"psychic-homily-backend/internal/services/pipeline"
)

func TestToMBArtistCandidates(t *testing.T) {
	raw := []pipeline.MBArtistResult{
		{ID: "m1", Name: "Sleep"},
		{ID: "m2", Name: "Boris"},
	}
	got := ToMBArtistCandidates(raw)
	assert.Len(t, got, 2)
	assert.Equal(t, "m1", got[0].MBID)
	assert.Equal(t, "Sleep", got[0].Name)
	assert.Equal(t, "m2", got[1].MBID)
	assert.Equal(t, "Boris", got[1].Name)
	assert.Empty(t, ToMBArtistCandidates(nil))
}

func TestToURLResources(t *testing.T) {
	var r1, r2, r3 pipeline.MBURLRelation
	r1.URL.Resource = "https://open.spotify.com/artist/abc"
	r2.URL.Resource = "" // empty resource is dropped
	r3.URL.Resource = "https://www.wikidata.org/wiki/Q1"
	got := ToURLResources([]pipeline.MBURLRelation{r1, r2, r3})
	assert.Equal(t, []string{"https://open.spotify.com/artist/abc", "https://www.wikidata.org/wiki/Q1"}, got)
	assert.Empty(t, ToURLResources(nil))
}

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
		FlattenArtistNames([]pipeline.MBArtistCredit{credit("Sleep", "Sleep")}))

	// Credited != canonical → both, credited first.
	assert.Equal(t, []string{"Sleep feat. Guest", "Sleep"},
		FlattenArtistNames([]pipeline.MBArtistCredit{credit("Sleep feat. Guest", "Sleep")}))

	// Empty credited name is skipped; the canonical is still emitted.
	assert.Equal(t, []string{"Sleep"},
		FlattenArtistNames([]pipeline.MBArtistCredit{credit("", "Sleep")}))

	// Multiple credits are flattened in order.
	assert.Equal(t, []string{"A", "B credited", "B"},
		FlattenArtistNames([]pipeline.MBArtistCredit{credit("A", "A"), credit("B credited", "B")}))

	// A fully empty credit contributes no names.
	assert.Empty(t, FlattenArtistNames([]pipeline.MBArtistCredit{credit("", "")}))
}
