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

// IsBandcampArtistHost reports whether host is an artist subdomain of
// bandcamp.com (<artist>.bandcamp.com) — NOT the bare apex. This is the single
// host-anchor every Bandcamp URL check shares: a parsed-host suffix match, never
// a substring of the whole URL, so hostile values like "169.254.169.254" (raw IP),
// "bandcamp.com.evil.test", or "evilbandcamp.com" are rejected. Real album/track
// pages and artist profiles always live on a subdomain; the apex (bandcamp.com)
// is the storefront, not an artist, so it is intentionally excluded.
//
// Callers pass u.Hostname() (already free of port/userinfo). PSY-1190 lifted this
// out of the three inline copies (the embed validator, the profile-root
// classifier, the resolver's SSRF fetch-anchor) so the volatile "what counts as a
// Bandcamp artist host" rule lives in one place.
func IsBandcampArtistHost(host string) bool {
	return strings.HasSuffix(strings.ToLower(host), ".bandcamp.com")
}

// IsValidBandcampEmbedURL reports whether rawURL is a strict Bandcamp
// album/track embed URL: an http/https URL whose host is an artist subdomain
// (<artist>.bandcamp.com) and whose path is /album/… or /track/….
//
// It parses the URL and anchors on the host (not a substring match), so a
// hostile value like "http://169.254.169.254/album/x?bandcamp.com" is rejected.
// This value is later rendered in an iframe src — it is STORED, not fetched —
// so the win is keeping a hostile/foreign host out of the column, not SSRF.
//
// This is the STRICT embed gate (it requires the /album|/track path), distinct
// from the looser host-anchor that utils.ValidateHTTPURL + the social-link
// validators apply. PSY-1188 lifted it out of the catalog handler into utils so
// the service-layer release-derived backfill can reuse the SAME rule that the
// admin bandcamp endpoint already enforces.
func IsValidBandcampEmbedURL(rawURL string) bool {
	trimmed := strings.TrimSpace(rawURL)
	u, err := url.Parse(trimmed)
	if err != nil {
		return false
	}
	// Accept http and https (matches the codebase's URL convention); the host
	// anchor below, not the scheme, is what rejects hostile values.
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	// Real album/track pages always live on an artist subdomain
	// (<artist>.bandcamp.com); the bare apex is not a release URL.
	if !IsBandcampArtistHost(u.Hostname()) {
		return false
	}
	// Album or track page, not a bare profile.
	return strings.HasPrefix(u.Path, "/album/") || strings.HasPrefix(u.Path, "/track/")
}

// IsBandcampAlbumURL reports whether rawURL's PATH is a Bandcamp /album/… page
// (as opposed to a /track/… page). It anchors on the parsed path so a /track/
// URL with "/album/" elsewhere (e.g. in a query string) is NOT misclassified.
// Callers should gate on IsValidBandcampEmbedURL first; this is the album-vs-
// track discriminator used to prefer the richer album embed (PSY-1188).
func IsBandcampAlbumURL(rawURL string) bool {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}
	return strings.HasPrefix(u.Path, "/album/")
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
