package utils

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSocialHandleBasesMatchCorpus is the drift tripwire for the tolerance's
// half of the registry: frontend/lib/socialLinks.test.ts asserts the same
// object, so a base changed in one language fails the other language's suite.
//
// The bases are not covered by the host-anchor assertions, because every base
// here is itself on an anchored host: a typo would still produce an anchored
// URL and send every legacy handle to the wrong page with nothing failing.
func TestSocialHandleBasesMatchCorpus(t *testing.T) {
	corpus := loadSocialLinkCorpus(t)

	assert.NotEmpty(t, corpus.HandleBases)
	assert.ElementsMatch(t,
		slices.Collect(maps.Keys(socialHandleBases)),
		slices.Collect(maps.Keys(corpus.HandleBases)),
		"the corpus and the Go table give a handle base to different fields")
	for field, want := range corpus.HandleBases {
		assert.Equal(t, want, socialHandleBases[field], "handle base disagrees for %q", field)
	}

	// A base that is not itself on the field's anchored host would resolve every
	// legacy handle to a value the anchor then refuses, which reads as "the
	// handle is bad" rather than "the base is wrong".
	for field, base := range socialHandleBases {
		assert.NoError(t, ValidateSocialHost(field, field, base+"someone"),
			"the handle base for %q resolves off its own anchored host", field)
	}
}

// TestValidateStoredSocialValueAgainstCorpus judges the offline writers' gate
// by the same shared corpus the HTTP boundary and the frontend read gate use.
//
// The buckets it can assert, and the one it cannot:
//   - `storable` must pass: this gate is never stricter than the HTTP boundary
//     about a value that already carries a scheme.
//   - `refusedByWriter` with rendersAnyway false must fail: those are the values
//     a click carries off-platform, and the tolerance must not open a door to
//     one.
//   - `refusedByWriter` with rendersAnyway true and NO scheme must pass: that is
//     the tolerance's whole job, and it is why the dev seed still loads.
//   - `storableButUnrenderable` is not asserted here. Those rows are places Go's
//     URL parser and the browser's disagree, which this gate inherits from
//     ValidateHTTPURL rather than decides; TestSocialLinkCorpus already pins
//     them for every write path.
func TestValidateStoredSocialValueAgainstCorpus(t *testing.T) {
	corpus := loadSocialLinkCorpus(t)

	for _, c := range corpus.Storable {
		assert.NoError(t, ValidateStoredSocialValue(c.Field, c.Field, c.Value),
			"corpus says storable (%s) but the offline gate refuses: %s %q", c.Why, c.Field, c.Value)
	}

	for _, c := range corpus.RefusedByWriter {
		if !c.RendersAnyway {
			assert.Error(t, ValidateStoredSocialValue(c.Field, c.Field, c.Value),
				"corpus says the reader refuses it (%s) but the offline gate stores it: %s %q",
				c.Why, c.Field, c.Value)
			continue
		}
		// A rendersAnyway row that already carries a scheme is a parser
		// divergence, not a legacy shape the tolerance exists for: Go refuses
		// "https://instagram.com\evil.test/x" in the authority while the browser
		// folds the backslash into the path. Failing closed there is correct.
		if hasHTTPSchemePrefix(strings.TrimSpace(c.Value)) {
			continue
		}
		assert.NoError(t, ValidateStoredSocialValue(c.Field, c.Field, c.Value),
			"corpus says the reader still renders it (%s) but the offline gate refuses: %s %q",
			c.Why, c.Field, c.Value)
	}
}

// TestValidateStoredSocialValue covers the shapes the corpus deliberately
// carries no row for, and the seed data's own handles.
func TestValidateStoredSocialValue(t *testing.T) {
	t.Run("blank is the clear-the-field gesture", func(t *testing.T) {
		assert.NoError(t, ValidateStoredSocialValue("instagram", "Instagram URL", ""))
		assert.NoError(t, ValidateStoredSocialValue("instagram", "Instagram URL", "   "))
	})

	// data/bands.yaml carries these three verbatim. A dot alone must not make a
	// handle look like a domain, or the dev seed stops loading.
	t.Run("a dotted handle is still a handle", func(t *testing.T) {
		for _, handle := range []string{"fashion.club.la", "johnny.dynamite", "jia._.pet"} {
			assert.NoError(t, ValidateStoredSocialValue("instagram", "Instagram URL", handle), handle)
		}
	})

	t.Run("a colon is never a handle", func(t *testing.T) {
		for _, value := range []string{"javascript:alert(1)", "spotify:artist:x", "data:text/html,x"} {
			assert.Error(t, ValidateStoredSocialValue("instagram", "Instagram URL", value), value)
		}
	})

	t.Run("an unknown field anchors no host but still needs a URL", func(t *testing.T) {
		// A field the anchor table does not know behaves as website does: any
		// host, but it must resolve to an absolute http URL.
		assert.NoError(t, ValidateStoredSocialValue("mastodon", "Mastodon URL", "https://mas.to/@x"))
		assert.Error(t, ValidateStoredSocialValue("mastodon", "Mastodon URL", "javascript:alert(1)"))
	})

	t.Run("the value is judged, not rewritten", func(t *testing.T) {
		// Nothing here returns a normalized string, so no caller can be tempted
		// to store the resolved URL in place of what the operator wrote.
		resolved, ok := resolveStoredSocialValue("instagram", "@calexico")
		assert.True(t, ok)
		assert.Equal(t, "https://instagram.com/calexico", resolved)
	})
}

// TestValidateStoredSocialColumns pins that each field of the struct is judged
// against its OWN platform, and that every anchored field is reachable through
// it: a column dropped from the pairs table inside would be invisible to the
// compiler and to any test that only passes valid values.
func TestValidateStoredSocialColumns(t *testing.T) {
	spotify := "https://open.spotify.com/artist/x"
	assert.NoError(t, ValidateStoredSocialColumns(SocialColumns{Spotify: &spotify}))
	assert.Error(t, ValidateStoredSocialColumns(SocialColumns{Instagram: &spotify}),
		"a Spotify URL in the instagram column must be refused")

	hostile := "https://spotify-account-verify.evil.test/"
	assert.Error(t, ValidateStoredSocialColumns(SocialColumns{Spotify: &hostile}))

	// Drive the struct one field at a time from a value that is on NO platform,
	// so a field the loop forgot passes where it should fail.
	offPlatform := "https://not-a-platform.evil.test/x"
	perField := map[string]SocialColumns{
		"instagram":  {Instagram: &offPlatform},
		"facebook":   {Facebook: &offPlatform},
		"twitter":    {Twitter: &offPlatform},
		"youtube":    {YouTube: &offPlatform},
		"spotify":    {Spotify: &offPlatform},
		"soundcloud": {SoundCloud: &offPlatform},
		"bandcamp":   {Bandcamp: &offPlatform},
	}
	assert.ElementsMatch(t, slices.Collect(maps.Keys(socialHostSuffixes)), slices.Collect(maps.Keys(perField)),
		"the anchor table and this test cover different fields")
	for field, columns := range perField {
		assert.Error(t, ValidateStoredSocialColumns(columns), "%q must be judged against its own anchor", field)
	}

	// website is the escape hatch: any host, but it must still be a URL.
	website := "https://anything.example.test/x"
	assert.NoError(t, ValidateStoredSocialColumns(SocialColumns{Website: &website}))
	junk := "javascript:alert(1)"
	assert.Error(t, ValidateStoredSocialColumns(SocialColumns{Website: &junk}))

	for field := range perField {
		assert.NotEmpty(t, SocialFieldLabels[field], "field %q has no refusal label", field)
	}
	assert.NotEmpty(t, SocialFieldLabels["website"])
}
