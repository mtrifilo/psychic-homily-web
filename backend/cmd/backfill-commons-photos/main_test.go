package main

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
	got := toMBArtistCandidates(raw)
	assert.Len(t, got, 2)
	assert.Equal(t, "m1", got[0].MBID)
	assert.Equal(t, "Sleep", got[0].Name)
	assert.Equal(t, "m2", got[1].MBID)
	assert.Equal(t, "Boris", got[1].Name)
	assert.Empty(t, toMBArtistCandidates(nil))
}

func TestToURLResources(t *testing.T) {
	var r1, r2, r3 pipeline.MBURLRelation
	r1.URL.Resource = "https://open.spotify.com/artist/abc"
	r2.URL.Resource = "" // empty resource is dropped
	r3.URL.Resource = "https://www.wikidata.org/wiki/Q1"
	got := toURLResources([]pipeline.MBURLRelation{r1, r2, r3})
	assert.Equal(t, []string{"https://open.spotify.com/artist/abc", "https://www.wikidata.org/wiki/Q1"}, got)
	assert.Empty(t, toURLResources(nil))
}
