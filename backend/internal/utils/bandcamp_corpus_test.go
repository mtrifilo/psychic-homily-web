package utils

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bandcampURLCorpus mirrors testdata/bandcamp_url_corpus.json, the file this
// test shares with frontend/lib/bandcamp.test.ts.
type bandcampURLCorpus struct {
	Storable []string `json:"storable"`
	Rejected []struct {
		URL                  string `json:"url"`
		Why                  string `json:"why"`
		AlsoRejectedByReader bool   `json:"alsoRejectedByReader"`
	} `json:"rejected"`
}

// TestBandcampURLCorpus is the Go half of the cross-language contract.
//
// The corpus lives in a JSON file rather than in either language's test because
// a hand-copied list can only fail for shapes someone already thought of — the
// backslash divergence sat in one for a full review round before a panel found
// it. With one shared file, a change to either predicate that breaks the
// contract fails on that side and names the value, and adding a case obliges
// both languages to agree about it.
//
// This half asserts the Go classification. frontend/lib/bandcamp.test.ts reads
// the same file and asserts the reader's, including that every `storable` entry
// renders — which is the contract direction.
func TestBandcampURLCorpus(t *testing.T) {
	raw, err := os.ReadFile("testdata/bandcamp_url_corpus.json")
	require.NoError(t, err, "the corpus is shared with the frontend test; do not move it without updating both")

	var corpus bandcampURLCorpus
	require.NoError(t, json.Unmarshal(raw, &corpus))
	require.NotEmpty(t, corpus.Storable)
	require.NotEmpty(t, corpus.Rejected)

	for _, url := range corpus.Storable {
		assert.True(t, IsValidBandcampEmbedURL(url),
			"corpus says storable but the write gate refuses: %q", url)
		// Storable implies resolvable, or the row would be written and then show
		// no player and no dot.
		assert.True(t, IsResolvableBandcampURL(url),
			"a storable value must also be resolvable: %q", url)
	}

	for _, c := range corpus.Rejected {
		assert.False(t, IsValidBandcampEmbedURL(c.URL),
			"corpus says rejected (%s) but the write gate accepts: %q", c.Why, c.URL)
	}
}
