package utils

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateHTTPURL(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantErr   bool
		errSubstr string
	}{
		// --- Accepted ---
		{
			name:    "http URL",
			input:   "http://example.com",
			wantErr: false,
		},
		{
			name:    "https URL",
			input:   "https://example.com",
			wantErr: false,
		},
		{
			name:    "https with path and query",
			input:   "https://example.com/path?q=1",
			wantErr: false,
		},
		{
			name:    "https with subdomain and port",
			input:   "https://sub.example.com:8080",
			wantErr: false,
		},
		{
			name:    "https with trailing slash",
			input:   "https://example.com/",
			wantErr: false,
		},
		{
			name:    "https with auth",
			input:   "https://user:pass@example.com",
			wantErr: false,
		},
		{
			name:    "https with fragment",
			input:   "https://example.com/page#section",
			wantErr: false,
		},
		{
			name:    "leading and trailing whitespace is trimmed",
			input:   "  https://example.com  ",
			wantErr: false,
		},

		// --- Rejected: dangerous schemes ---
		{
			name:      "javascript scheme",
			input:     "javascript:alert(1)",
			wantErr:   true,
			errSubstr: "http or https",
		},
		{
			name:      "data scheme",
			input:     "data:text/html,foo",
			wantErr:   true,
			errSubstr: "http or https",
		},
		{
			name:      "mailto scheme",
			input:     "mailto:user@example.com",
			wantErr:   true,
			errSubstr: "http or https",
		},
		{
			name:      "file scheme",
			input:     "file:///etc/passwd",
			wantErr:   true,
			errSubstr: "http or https",
		},

		// --- Rejected: other schemes ---
		{
			name:      "ftp scheme",
			input:     "ftp://example.com",
			wantErr:   true,
			errSubstr: "http or https",
		},
		{
			name:      "ws scheme",
			input:     "ws://example.com",
			wantErr:   true,
			errSubstr: "http or https",
		},

		// --- Rejected: malformed ---
		{
			name:      "no scheme, no host",
			input:     "not-a-url",
			wantErr:   true,
			errSubstr: "http or https",
		},
		{
			name:      "https with no host",
			input:     "https://",
			wantErr:   true,
			errSubstr: "host",
		},
		{
			name:    "empty scheme with host-like value",
			input:   "://example.com",
			wantErr: true,
		},
		{
			name:      "scheme-relative URL",
			input:     "//example.com",
			wantErr:   true,
			errSubstr: "http or https",
		},
		{
			name:      "path only",
			input:     "/path/only",
			wantErr:   true,
			errSubstr: "http or https",
		},

		// --- Edge: empty input is valid (caller decides what empty means) ---
		{
			name:    "empty string",
			input:   "",
			wantErr: false,
		},
		{
			name:    "whitespace only",
			input:   "   ",
			wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateHTTPURL(tc.input, "Image URL")
			if tc.wantErr {
				assert.Error(t, err, "expected error for %q", tc.input)
				if tc.errSubstr != "" && err != nil {
					assert.Contains(t, err.Error(), tc.errSubstr,
						"error message should mention %q for input %q", tc.errSubstr, tc.input)
				}
			} else {
				assert.NoError(t, err, "expected no error for %q", tc.input)
			}
		})
	}
}

func TestValidateHTTPURL_FieldNameInError(t *testing.T) {
	// The error message must name the field so curators can fix the right input.
	err := ValidateHTTPURL("javascript:alert(1)", "Instagram URL")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Instagram URL",
		"error should mention the field name")
}

func TestValidateHTTPURL_AcceptedSchemesInError(t *testing.T) {
	// The error message must enumerate the accepted schemes so callers know
	// what's allowed without reading the source.
	err := ValidateHTTPURL("ftp://example.com", "Website")
	assert.Error(t, err)
	msg := err.Error()
	assert.True(t,
		strings.Contains(msg, "http") && strings.Contains(msg, "https"),
		"error %q should name both http and https", msg)
}

// TestValidateHTTPURL_SurfacesUserInput covers PSY-599: the rejection message
// must echo the user's actual typed value, not the post-parse normalized
// fragment. The canonical failure mode is `url.Parse("not-a-real-url")`
// succeeding with `Scheme == ""`, leading the old message to claim
// `(got "")` while the diff preview alongside the banner showed the real
// input. Now both surfaces must agree.
func TestValidateHTTPURL_SurfacesUserInput(t *testing.T) {
	cases := []struct {
		name  string
		input string
		// The substring the error message MUST contain — i.e. the trimmed
		// user input, never the post-parse derivative.
		mustContain string
	}{
		{
			name:        "scheme-less input parses with empty Scheme",
			input:       "not-a-real-url",
			mustContain: `"not-a-real-url"`,
		},
		{
			name:        "scheme-less input that resembles a slug",
			input:       "amylandthesniffers",
			mustContain: `"amylandthesniffers"`,
		},
		{
			name:        "leading whitespace is trimmed before echoing",
			input:       "   not-a-real-url   ",
			mustContain: `"not-a-real-url"`,
		},
		{
			name:        "non-http scheme echoes the full input, not just the scheme",
			input:       "ftp://example.com/file.zip",
			mustContain: `"ftp://example.com/file.zip"`,
		},
		{
			name:        "dangerous javascript scheme echoes the full input",
			input:       "javascript:alert(1)",
			mustContain: `"javascript:alert(1)"`,
		},
		{
			name:        "scheme-relative url echoes the full input",
			input:       "//example.com/path",
			mustContain: `"//example.com/path"`,
		},
		{
			name:        "https with no host echoes the full input",
			input:       "https://",
			mustContain: `"https://"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateHTTPURL(tc.input, "Instagram URL")
			assert.Error(t, err, "expected an error for %q", tc.input)
			if err == nil {
				return
			}
			msg := err.Error()
			assert.Contains(t, msg, tc.mustContain,
				"error %q must surface the original user input %q", msg, tc.mustContain)
			// Belt-and-suspenders: explicitly forbid the misleading
			// post-normalize empty-string render that PSY-599 was filed for.
			assert.NotContains(t, msg, `(got "")`,
				"error must not surface a post-normalize empty value (PSY-599)")
		})
	}
}

// TestNormalizeInstagramHandle covers PSY-1118: the show-create/update
// instagram_handle must be a bare handle, and is normalized to the canonical
// instagram.com URL so it can never escape the platform host (the slot is
// host-anchored everywhere else by PSY-1113).
func TestNormalizeInstagramHandle(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		// --- Accepted: bare handles normalized to canonical URL ---
		{name: "plain handle", input: "bandname", want: "https://instagram.com/bandname"},
		{name: "leading @ stripped", input: "@new_ig", want: "https://instagram.com/new_ig"},
		{name: "underscores and digits", input: "the_band_99", want: "https://instagram.com/the_band_99"},
		{name: "period in handle", input: "band.name", want: "https://instagram.com/band.name"},
		{name: "surrounding whitespace trimmed", input: "  @new_ig  ", want: "https://instagram.com/new_ig"},
		{name: "max length 30", input: strings.Repeat("a", 30), want: "https://instagram.com/" + strings.Repeat("a", 30)},
		// A bare ".com" value is a syntactically valid handle and, normalized,
		// resolves under instagram.com — defusing the SocialLinks ".com → raw
		// https host" rendering path it would otherwise have hit.
		{name: "domain-like bare value stays on-platform", input: "evil.com", want: "https://instagram.com/evil.com"},

		// --- Empty: treated as "no handle", no error ---
		{name: "empty string", input: "", want: ""},
		{name: "whitespace only", input: "   ", want: ""},
		{name: "bare @ only", input: "@", want: ""},

		// --- Rejected: URL-shaped / scheme-bearing input (the PSY-1118 vector) ---
		{name: "full https URL", input: "https://evil.test", wantErr: true},
		{name: "full instagram URL", input: "https://instagram.com/realband", wantErr: true},
		{name: "partial URL with slash", input: "instagram.com/realband", wantErr: true},
		{name: "javascript scheme", input: "javascript:alert(1)", wantErr: true},
		{name: "contains slash", input: "band/name", wantErr: true},
		{name: "contains space", input: "band name", wantErr: true},
		{name: "too long (31)", input: strings.Repeat("a", 31), wantErr: true},
		{name: "disallowed char", input: "band!name", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeInstagramHandle(tc.input)
			if tc.wantErr {
				assert.Error(t, err, "expected an error for %q", tc.input)
				assert.Empty(t, got, "no normalized value should be returned on error")
				return
			}
			assert.NoError(t, err, "unexpected error for %q", tc.input)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestIsValidBandcampEmbedURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		// Valid album/track pages on an artist subdomain.
		{"album page", "https://artificialgo.bandcamp.com/album/triple-ones", true},
		{"track page", "https://artificialgo.bandcamp.com/track/one-song", true},
		// http is refused even on a real release page: both renderers require
		// https, so an http row shows neither an embed nor a link.
		{"http scheme rejected", "http://artificialgo.bandcamp.com/album/x", false},
		// NOT trimmed. Callers store the string they hand in, so a validator that
		// trimmed would pass this and put a leading space in the column, where the
		// browser's parser refuses it.
		{"leading/trailing whitespace rejected", "  https://x.bandcamp.com/album/y  ", false},

		// Rejected: not an album/track path.
		{"bare profile root", "https://artificialgo.bandcamp.com", false},
		{"profile root with trailing slash", "https://artificialgo.bandcamp.com/", false},
		{"some other path", "https://artificialgo.bandcamp.com/music", false},

		// Rejected: wrong / hostile host (host-anchored, not substring).
		{"apex bandcamp.com no subdomain", "https://bandcamp.com/album/x", false},
		{"foreign host with bandcamp in query", "http://169.254.169.254/album/x?bandcamp.com", false},
		{"lookalike host suffix", "https://evil-bandcamp.com/album/x", false},
		{"bandcamp in path of other host", "https://evil.test/bandcamp.com/album/x", false},

		// Rejected: bad scheme / unparseable.
		{"javascript scheme", "javascript:alert(1)", false},
		{"empty string", "", false},
		{"not a url", "not a url", false},

		// Rejected: the shapes PSY-1966 exists for. Each is a plausible-looking
		// value that would otherwise render as an outbound link labelled Bandcamp.
		{"open redirect carrying a real release URL", "https://evil.test/?next=https://x.bandcamp.com/album/y", false},
		{"userinfo spoofing the host", "https://x.bandcamp.com@evil.test/album/y", false},
		{"release path on a subdomain of a lookalike", "https://x.bandcamp.com.evil.test/album/y", false},
		{"album segment only in the query of a foreign host", "https://evil.test/checkout?ref=/album/x", false},

		// Rejected because the BROWSER reads these paths differently than Go's
		// decoded u.Path does. Each would otherwise be storable and unrenderable,
		// which is the one direction the store-subset-of-render contract forbids.
		{"percent-encoded first segment", "https://x.bandcamp.com/%61lbum/y", false},
		{"encoded slash hiding the real segment", "https://x.bandcamp.com/album%2Fy", false},
		{"dot segments the browser resolves away", "https://x.bandcamp.com/album/../../evil", false},
		{"single dot segment", "https://x.bandcamp.com/album/./y", false},
		{"encoded dot-dot segment", "https://x.bandcamp.com/album/%2E%2E/evil", false},
		// A percent-encoded character LATER in the slug is ordinary and stays valid.
		{"encoded character inside the slug", "https://x.bandcamp.com/album/caf%C3%A9", true},

		// Whitespace-only would otherwise be stored blank-but-not-null: "has an
		// embed" to every IS NULL backfill, nothing at all to a reader.
		{"whitespace only", "   ", false},
		{"non-breaking space prefix", " https://x.bandcamp.com/album/y", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, IsValidBandcampEmbedURL(tc.input))
		})
	}
}

func TestValidateBandcampEmbedURL(t *testing.T) {
	t.Run("accepts a release page", func(t *testing.T) {
		assert.NoError(t, ValidateBandcampEmbedURL("https://x.bandcamp.com/album/y", BandcampEmbedURLLabel))
	})

	t.Run("only the empty string clears the field", func(t *testing.T) {
		assert.NoError(t, ValidateBandcampEmbedURL("", BandcampEmbedURLLabel))
		// Whitespace-only is NOT a clear gesture. Accepting it would store a
		// blank-but-not-null value: excluded from every IS NULL backfill and the
		// link sweep forever, counted as "has an embed" by the playable-audio
		// flags, and rendered as nothing.
		assert.Error(t, ValidateBandcampEmbedURL("   ", BandcampEmbedURLLabel))
		assert.Error(t, ValidateBandcampEmbedURL("\t\n", BandcampEmbedURLLabel))
	})

	t.Run("caps length so every boundary agrees on what fits", func(t *testing.T) {
		long := "https://x.bandcamp.com/album/" + strings.Repeat("a", MaxBandcampEmbedURLLen)
		err := ValidateBandcampEmbedURL(long, BandcampEmbedURLLabel)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "characters or fewer")
	})

	t.Run("refusal names the field and shows an accepted example", func(t *testing.T) {
		err := ValidateBandcampEmbedURL("https://evil.test/album/x", BandcampEmbedURLLabel)
		require.Error(t, err)
		// The message has to be actionable on its own: a submitter cannot fix
		// "host anchor failed", but can fix by copying the shape shown.
		assert.Contains(t, err.Error(), BandcampEmbedURLLabel)
		assert.Contains(t, err.Error(), "https://artist.bandcamp.com/album/title")
	})

	t.Run("field name is the caller's, not a constant", func(t *testing.T) {
		err := ValidateBandcampEmbedURL("https://evil.test/album/x", "bandcamp_embed_url")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "bandcamp_embed_url")
	})
}

// IsResolvableBandcampURL answers a DIFFERENT question from the write gate:
// "can this row produce anything on screen", which is what the backend's
// playable-audio flags and the scene representative-embed picker need so they do
// not promise a player that never appears. It is the Go mirror of
// isAllowedBandcampUrl in frontend/lib/bandcamp.ts.
func TestIsResolvableBandcampURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		// Looser than the write gate on purpose: a non-release page on Bandcamp
		// can still resolve to a playable id, and the apex is accepted because
		// the frontend predicate it mirrors accepts it.
		{"release page", "https://x.bandcamp.com/album/y", true},
		{"profile root", "https://x.bandcamp.com", true},
		{"non-release on-platform page", "https://x.bandcamp.com/music", true},
		{"apex", "https://bandcamp.com/album/x", true},

		// Anything the resolver would refuse to fetch.
		{"http", "http://x.bandcamp.com/album/y", false},
		{"foreign host", "https://evil.test/album/y", false},
		{"lookalike suffix", "https://bandcamp.com.evil.test/album/y", false},
		{"lookalike prefix", "https://evilbandcamp.com/album/y", false},
		{"userinfo spoofing the host", "https://x.bandcamp.com@evil.test/album/y", false},
		{"empty", "", false},
		{"whitespace only", "   ", false},
		{"not a url", "not a url", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, IsResolvableBandcampURL(tc.input))
		})
	}
}

// The two GO predicates form a ladder: anything STORABLE must also be
// RESOLVABLE, or a value the write gate accepts would light no playable dot and
// be skipped by the scene picker.
//
// This is NOT the store-subset-of-render contract — that one spans languages and
// is pinned by the shared corpus (TestBandcampURLCorpus here,
// frontend/lib/bandcamp.test.ts there). This is the weaker in-language ladder,
// and it is close to true by construction; it earns its place by failing loudly
// if someone ever makes the write gate accept a host the read-side predicate
// does not.
func TestStorableImpliesResolvable(t *testing.T) {
	for _, value := range []string{
		"https://x.bandcamp.com/album/y",
		"https://x.bandcamp.com/track/y",
		"https://x.bandcamp.com/album/caf%C3%A9",
		"https://x.bandcamp.com/track/t?ref=/album/a",
	} {
		require.True(t, IsValidBandcampEmbedURL(value), "fixture must be storable: %s", value)
		assert.True(t, IsResolvableBandcampURL(value),
			"a storable value must be resolvable, or it renders nothing: %s", value)
	}
}

func TestIsBandcampAlbumURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"album path", "https://x.bandcamp.com/album/y", true},
		{"track path", "https://x.bandcamp.com/track/y", false},
		{"track with /album/ in query is not an album", "https://x.bandcamp.com/track/y?from=/album/z", false},
		{"unparseable", "://bad", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, IsBandcampAlbumURL(tc.input))
		})
	}
}

func TestIsBandcampArtistHost(t *testing.T) {
	tests := []struct {
		name string
		host string
		want bool
	}{
		{"artist subdomain", "boris.bandcamp.com", true},
		{"deep subdomain", "a.b.bandcamp.com", true},
		{"mixed case", "Boris.BandCamp.Com", true},
		{"bare apex excluded", "bandcamp.com", false},
		{"substring suffix attack", "bandcamp.com.evil.test", false},
		{"substring prefix attack", "evilbandcamp.com", false},
		{"not-bandcamp", "notbandcamp.com", false},
		{"empty", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, IsBandcampArtistHost(tc.host))
		})
	}
}
