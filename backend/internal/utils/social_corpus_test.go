package utils

import (
	"encoding/json"
	"maps"
	"os"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// socialCorpusCase is one field/value pair the corpus makes a claim about.
type socialCorpusCase struct {
	Field string `json:"field"`
	Value string `json:"value"`
	Why   string `json:"why"`
}

// socialLinkCorpus mirrors testdata/social_link_corpus.json, the file this test
// shares with frontend/lib/socialLinks.test.ts.
type socialLinkCorpus struct {
	Platforms               map[string][]string `json:"platforms"`
	Unanchored              []string            `json:"unanchored"`
	Storable                []socialCorpusCase  `json:"storable"`
	StorableButUnrenderable []socialCorpusCase  `json:"storableButUnrenderable"`
	RefusedByWriter         []struct {
		socialCorpusCase
		RendersAnyway bool `json:"rendersAnyway"`
	} `json:"refusedByWriter"`
}

func loadSocialLinkCorpus(t *testing.T) socialLinkCorpus {
	t.Helper()
	raw, err := os.ReadFile("testdata/social_link_corpus.json")
	require.NoError(t, err, "the corpus is shared with the frontend test; do not move it without updating both")

	var corpus socialLinkCorpus
	require.NoError(t, json.Unmarshal(raw, &corpus))
	require.NotEmpty(t, corpus.Platforms)
	require.NotEmpty(t, corpus.Storable)
	require.NotEmpty(t, corpus.RefusedByWriter)
	return corpus
}

// socialWriteBoundary is the pair of rules every write path applies to a social
// field, as one answer. They are separate functions in production because the
// scheme rule covers fields the host anchor does not; a corpus row is judged by
// both because a value refused by either is a value no path stores.
func socialWriteBoundary(field, value string) error {
	if err := ValidateHTTPURL(value, field); err != nil {
		return err
	}
	return ValidateSocialHost(field, field, value)
}

// TestSocialLinkCorpus is the Go half of the cross-language contract. The
// TypeScript half reads the same file and asserts the reader's classification,
// including that every `storable` entry renders, which is the direction that
// keeps a stored row from silently producing no link.
//
// `storableButUnrenderable` is judged here exactly as `storable` is, because
// the two differ only in what the OTHER language does with them.
func TestSocialLinkCorpus(t *testing.T) {
	corpus := loadSocialLinkCorpus(t)

	for _, c := range slices.Concat(corpus.Storable, corpus.StorableButUnrenderable) {
		assert.NoError(t, socialWriteBoundary(c.Field, c.Value),
			"corpus says storable (%s) but the write boundary refuses: %s %q", c.Why, c.Field, c.Value)
	}

	for _, c := range corpus.RefusedByWriter {
		assert.Error(t, socialWriteBoundary(c.Field, c.Value),
			"corpus says refused (%s) but the write boundary accepts: %s %q", c.Why, c.Field, c.Value)
	}
}

// TestSocialLinkCorpusPinsTheTable is what stops the Go and TypeScript tables
// drifting: each language asserts its own registry against the corpus, so a
// field or a host changed in one and not the other fails the other side.
func TestSocialLinkCorpusPinsTheTable(t *testing.T) {
	corpus := loadSocialLinkCorpus(t)

	assert.ElementsMatch(t, slices.Collect(maps.Keys(socialHostSuffixes)), slices.Collect(maps.Keys(corpus.Platforms)),
		"the corpus and the Go table anchor different fields")
	for field, wantBases := range corpus.Platforms {
		gotBases, ok := socialHostSuffixes[field]
		if assert.True(t, ok, "corpus field %q is missing from the Go table", field) {
			assert.ElementsMatch(t, wantBases, gotBases, "host anchors disagree for %q", field)
		}
	}

	// The unanchored fields are a claim in the other direction: absent from the
	// table means "any host", so a field listed there that later gains bases
	// would silently change what the corpus is asserting.
	for _, field := range corpus.Unanchored {
		_, anchored := socialHostSuffixes[field]
		assert.False(t, anchored, "corpus calls %q unanchored but the Go table anchors it", field)
	}

	// Table membership alone is not enough. A field with no storable case has
	// bases neither parser is ever run against, so its anchor is unpoliced and a
	// later removal is silent in both languages.
	exercised := map[string]bool{}
	for _, c := range corpus.Storable {
		exercised[c.Field] = true
	}
	for field := range socialHostSuffixes {
		assert.True(t, exercised[field],
			"field %q has no storable corpus case, so its hosts are never exercised", field)
	}
}

// TestSocialWriteBoundaryTreatsBlankAsNoValue pins the clear-the-field gesture
// the corpus deliberately carries no case for: both halves of the boundary pass
// a blank value, and the reader renders nothing for it, so neither side treats
// it as a link.
func TestSocialWriteBoundaryTreatsBlankAsNoValue(t *testing.T) {
	assert.NoError(t, socialWriteBoundary("instagram", ""))
	assert.NoError(t, socialWriteBoundary("instagram", "   "))
}
