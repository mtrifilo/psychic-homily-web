package utils

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// ValidateHTTPURL returns an error if s is not a parseable absolute URL with
// an http or https scheme. Empty input returns nil — callers should normalize
// empty strings (e.g. via nilIfEmpty) before deciding what an empty value
// means in their domain.
//
// fieldName is interpolated into the error message so curators can fix the
// offending field without guessing which input was rejected.
//
// PSY-525: defense-in-depth at the API boundary. Validate-on-write only —
// existing rows that may already contain non-conforming URLs stay readable.
// Accepted schemes are http and https; everything else (data:, javascript:,
// mailto:, ftp:, file:, etc.) is rejected.
//
// PSY-599: the rejection message surfaces the user's original (trimmed) input,
// not the post-parse scheme. `url.Parse("not-a-real-url")` succeeds with an
// empty Scheme — surfacing `(got "")` is misleading because the diff preview
// shows the actual typed value next to it. Use the trimmed input so the two
// surfaces agree on what was submitted.
func ValidateHTTPURL(s, fieldName string) error {
	if s == "" {
		return nil
	}
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return nil
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return fmt.Errorf("%s must be a valid URL (got %q): %w", fieldName, trimmed, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%s must use http or https scheme (got %q)", fieldName, trimmed)
	}
	if u.Host == "" {
		return fmt.Errorf("%s must include a host (got %q)", fieldName, trimmed)
	}
	return nil
}

// instagramHandleRe matches a bare Instagram username: 1–30 characters of
// letters, digits, periods, and underscores. This mirrors Instagram's own
// handle grammar and, crucially, contains no ':' or '/', so any URL-shaped or
// scheme-bearing input ("https://evil.test", "instagram.com/x",
// "javascript:…") fails to match and is rejected.
var instagramHandleRe = regexp.MustCompile(`^[A-Za-z0-9._]{1,30}$`)

// NormalizeInstagramHandle validates a user-supplied Instagram handle and
// returns the canonical storage form, "https://instagram.com/<handle>".
//
// PSY-1118: the show-create/update artist `instagram_handle` was length-only
// validated and stored verbatim into social.instagram — the same slot PSY-1113
// host-anchors on every other write path. A value like "https://evil.test"
// therefore bypassed the host anchor and rendered as an off-platform
// SocialLinks href. This is the deliberate "it's a handle, not a URL" fix:
// reject anything URL-shaped, then normalize the accepted handle to the same
// full-instagram.com URL form the artist/venue/label edit paths (and the seed
// exemplars) already store. Normalizing — rather than storing the bare handle —
// also closes a second hole: the frontend SocialLinks.normalizeUrl renders a
// bare ".com"/".org" value as a raw https host, so a handle like "evil.com"
// would otherwise still escape on-platform. Stored as instagram.com/<handle>,
// it can only ever resolve under instagram.com.
//
// A leading "@" is stripped (the AI extraction pipeline emits "@username").
// Whitespace-only input normalizes to "" with no error so callers can treat it
// as "no handle provided"; callers should skip storing an empty result.
// Validate-on-write only — pre-PSY-1118 rows that hold a bare handle stay
// readable via the SocialLinks tolerance layer.
func NormalizeInstagramHandle(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	handle := strings.TrimPrefix(trimmed, "@")
	if handle == "" {
		return "", nil
	}
	if !instagramHandleRe.MatchString(handle) {
		return "", fmt.Errorf(
			"instagram handle %q must be a bare username (letters, digits, '.', '_'; max 30), not a URL",
			trimmed,
		)
	}
	return "https://instagram.com/" + handle, nil
}
