package utils

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
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
			name:      "empty scheme with host-like value",
			input:     "://example.com",
			wantErr:   true,
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
