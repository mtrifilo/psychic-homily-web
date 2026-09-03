package utils

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// releaseLinkCorpus mirrors testdata/release_link_corpus.json, the file this
// test shares with frontend/lib/releaseLinks.test.ts.
type releaseLinkCorpus struct {
	Platforms  map[string][]string `json:"platforms"`
	Renderable []struct {
		Platform string `json:"platform"`
		URL      string `json:"url"`
	} `json:"renderable"`
	Refused []struct {
		Platform            string `json:"platform"`
		URL                 string `json:"url"`
		Why                 string `json:"why"`
		AlsoRefusedByReader bool   `json:"alsoRefusedByReader"`
	} `json:"refused"`
}

func loadReleaseLinkCorpus(t *testing.T) releaseLinkCorpus {
	t.Helper()
	raw, err := os.ReadFile("testdata/release_link_corpus.json")
	require.NoError(t, err, "the corpus is shared with the frontend test; do not move it without updating both")

	var corpus releaseLinkCorpus
	require.NoError(t, json.Unmarshal(raw, &corpus))
	require.NotEmpty(t, corpus.Platforms)
	require.NotEmpty(t, corpus.Renderable)
	require.NotEmpty(t, corpus.Refused)
	return corpus
}

// TestReleaseLinkCorpus is the Go half of the cross-language contract. The
// TypeScript half reads the same file and asserts the reader's classification,
// including that every `renderable` entry renders, which is the direction that
// keeps a stored row from silently producing no link.
func TestReleaseLinkCorpus(t *testing.T) {
	corpus := loadReleaseLinkCorpus(t)

	for _, c := range corpus.Renderable {
		assert.True(t, IsRenderableReleaseLink(c.Platform, c.URL),
			"corpus says renderable but the write gate refuses: %s %q", c.Platform, c.URL)
		assert.NoError(t, ValidateReleaseLink(c.Platform, c.URL),
			"the predicate and the validator disagree: %s %q", c.Platform, c.URL)
	}

	for _, c := range corpus.Refused {
		assert.False(t, IsRenderableReleaseLink(c.Platform, c.URL),
			"corpus says refused (%s) but the write gate accepts: %s %q", c.Why, c.Platform, c.URL)
		assert.Error(t, ValidateReleaseLink(c.Platform, c.URL),
			"the predicate and the validator disagree: %s %q", c.Platform, c.URL)
	}
}

// TestReleaseLinkCorpusPinsThePlatformTable is what stops the Go and TypeScript
// registries drifting: each language asserts its own table against the corpus,
// so adding a platform or a host to one and not the other fails the other side.
func TestReleaseLinkCorpusPinsThePlatformTable(t *testing.T) {
	corpus := loadReleaseLinkCorpus(t)

	assert.ElementsMatch(t, ReleaseLinkPlatforms(), keysOf(corpus.Platforms),
		"the corpus and the Go registry name different platforms")
	for platform, wantHosts := range corpus.Platforms {
		gotHosts, ok := ReleaseLinkPlatformHosts(platform)
		if assert.True(t, ok, "corpus platform %q is missing from the Go registry", platform) {
			assert.ElementsMatch(t, wantHosts, gotHosts,
				"host anchors disagree for %q", platform)
		}
	}
}

func keysOf(m map[string][]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestReleaseLinkPlatformHostsIsNotMutable pins the copy in the accessor. The
// table is a security allowlist in a leaf package everything imports, so a
// caller holding the live slice could widen it for the whole process.
func TestReleaseLinkPlatformHostsIsNotMutable(t *testing.T) {
	hosts, ok := ReleaseLinkPlatformHosts("bandcamp")
	require.True(t, ok)
	hosts[0] = "evil.test"

	assert.False(t, IsRenderableReleaseLink("bandcamp", "https://evil.test/album/x"))
	again, _ := ReleaseLinkPlatformHosts("bandcamp")
	assert.Equal(t, []string{"bandcamp.com"}, again)
}

func TestValidateReleaseLinkRefusesEmptyValues(t *testing.T) {
	assert.ErrorContains(t, ValidateReleaseLink("", "https://bandcamp.com/album/x"), "Platform is required")
	assert.ErrorContains(t, ValidateReleaseLink("bandcamp", ""), "URL is required")
	assert.ErrorContains(t, ValidateReleaseLink("bandcamp", "   "), "URL is required")
	assert.False(t, IsRenderableReleaseLink("bandcamp", ""))
}

func TestValidateReleaseLinkRefusesOverlongURL(t *testing.T) {
	long := "https://kingbuffalo.bandcamp.com/album/" + strings.Repeat("a", MaxReleaseLinkURLLen)
	require.Greater(t, len(long), MaxReleaseLinkURLLen)

	assert.False(t, IsRenderableReleaseLink("bandcamp", long))
	assert.ErrorContains(t, ValidateReleaseLink("bandcamp", long), "characters or fewer")
}

// TestValidateReleaseLinkNamesTheAcceptedValue pins the refusal copy: a
// submitter has to be able to act on it without reading this file.
func TestValidateReleaseLinkNamesTheAcceptedValue(t *testing.T) {
	err := ValidateReleaseLink("napster", "https://us.napster.com/album/x")
	require.Error(t, err)
	for _, platform := range ReleaseLinkPlatforms() {
		assert.Contains(t, err.Error(), platform)
	}

	err = ValidateReleaseLink("spotify", "https://evil.test/album/x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spotify.com")
	assert.Contains(t, err.Error(), "http or https")
}

// TestReleaseLinkPlatformMatchIsCaseInsensitive pins the one place the gate is
// lenient: the enrichment writer's dedup index is on LOWER(platform), so the
// system has always treated these as the same platform.
func TestReleaseLinkPlatformMatchIsCaseInsensitive(t *testing.T) {
	for _, platform := range []string{"bandcamp", "Bandcamp", "BANDCAMP"} {
		assert.True(t, IsRenderableReleaseLink(platform, "https://kingbuffalo.bandcamp.com/album/regenerator"),
			"platform %q", platform)
	}
}

// TestReleaseLinkAcceptsNoPathRule states the deliberate scope of the gate: it
// anchors the HOST, so real platform URLs whose paths differ per platform are
// not refused. A path rule here would refuse these and buy nothing, because the
// host already decides where a click lands.
func TestReleaseLinkAcceptsNoPathRule(t *testing.T) {
	assert.True(t, IsRenderableReleaseLink("spotify", "https://open.spotify.com/intl-pt/album/x"))
	assert.True(t, IsRenderableReleaseLink("apple_music", "https://music.apple.com/gb/album/x/1"))
	assert.True(t, IsRenderableReleaseLink("bandcamp", "https://kingbuffalo.bandcamp.com/merch/vinyl"))
}
