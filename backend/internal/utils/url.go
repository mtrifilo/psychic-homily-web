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

// IsValidBandcampEmbedURL reports whether rawURL is a Bandcamp release page:
// an https URL whose host is an artist subdomain (<artist>.bandcamp.com) and
// whose path is /album/… or /track/….
//
// This is the ONE named predicate for the shape artists.bandcamp_embed_url is
// allowed to hold. Every write path that stores the column gates on it, and the
// frontend read gate (isBandcampReleaseUrl in frontend/lib/bandcamp.ts) mirrors
// it, so what can be stored is a subset of what can be rendered. The read gate
// is the more lenient of the two only in accepting the bandcamp.com apex, which
// exists to keep a legacy row renderable; nothing may be STORED that the
// renderer would refuse, or a curator's save would silently produce no link.
//
// Five rules, each load-bearing:
//
//   - The value is judged EXACTLY as it will be stored. Nothing is trimmed or
//     normalized first, because every caller stores the string it handed in: a
//     validator that trims passes " https://…/album/x" and then a leading space
//     goes into the column, where the browser's parser refuses it. Same reason a
//     whitespace-only value is refused rather than treated as empty — it would
//     otherwise be STORED as blank-but-not-null, which reads as "has an embed"
//     to every IS NULL backfill and renders as nothing.
//   - https only. The value's whole purpose is to be rendered, and both
//     renderers refuse http: the resolver that turns it into an iframe fetches
//     it through isAllowedBandcampUrl (https), and the link fallback gates on
//     isBandcampReleaseUrl (https). Storing http yields a row that shows
//     nothing at all, so it is refused where someone can still fix it.
//   - Host anchored on the parsed hostname, never a substring of the URL, so
//     "http://169.254.169.254/album/x?bandcamp.com", "bandcamp.com.evil.test"
//     and "evilbandcamp.com" are all rejected.
//   - Path prefix on the ESCAPED path, never on the decoded one and never on a
//     substring of the URL. Decoded would accept "/%61lbum/y" and "/album%2Fx",
//     which the browser's `pathname` — the thing the read gate reads — spells
//     differently and refuses. Escaped is what the two layers agree on.
//   - No "." or ".." path segment. Go leaves them in place and the browser
//     resolves them away, so "/album/../../evil" is a release page to one layer
//     and an arbitrary path to the other. A real release page has neither.
//
// The column is not fetched by us at write time, so the win here is keeping a
// hostile or foreign host out of a value that later renders BOTH as an iframe
// and as an outbound link wearing a Bandcamp label (PSY-1966); it is not SSRF.
//
// What it does NOT prove, stated so nobody reads more into it: bandcamp.com
// subdomains are handed out on signup, so this establishes "a Bandcamp release
// page", never "a page this artist controls". The surface is narrowed to
// Bandcamp's own abuse handling, not closed.
//
// This is the STRICT embed gate (it requires the /album|/track path), distinct
// from the looser per-platform host floor the social-link validators apply to
// social.bandcamp, which holds a profile root rather than a release.
func IsValidBandcampEmbedURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	if u.Scheme != "https" {
		return false
	}
	// Real album/track pages always live on an artist subdomain
	// (<artist>.bandcamp.com); the bare apex is not a release URL.
	if !IsBandcampArtistHost(u.Hostname()) {
		return false
	}
	path := u.EscapedPath()
	if hasDotSegment(path) {
		return false
	}
	// Album or track page, not a bare profile.
	return strings.HasPrefix(path, "/album/") || strings.HasPrefix(path, "/track/")
}

// hasDotSegment reports whether path contains a "." or ".." segment, in either
// its literal or percent-encoded spelling. Those are the segments a browser
// resolves away before it decides what the path is, so a check that ignored them
// would answer a different question than the renderer does.
func hasDotSegment(path string) bool {
	for _, segment := range strings.Split(path, "/") {
		decoded, err := url.PathUnescape(segment)
		if err != nil {
			// Unescapable input is not a segment we can reason about; treat it
			// as suspect, which the caller turns into a rejection.
			return true
		}
		if decoded == "." || decoded == ".." {
			return true
		}
	}
	return false
}

// IsResolvableBandcampURL reports whether rawURL is a Bandcamp page the embed
// resolver will even attempt: https on bandcamp.com or one of its subdomains.
//
// Looser than IsValidBandcampEmbedURL, and for a different question. That one
// decides what may be STORED; this one answers "can this row produce anything
// on screen", which is the question the backend's playable-audio flags and the
// scene representative-embed picker have to answer to avoid promising a player
// that never appears.
//
// It is the Go mirror of isAllowedBandcampUrl in frontend/lib/bandcamp.ts, which
// is the precondition /api/bandcamp/album-id enforces before it fetches. It
// accepts the apex because that predicate does: the two must agree, and being
// the more lenient of the two costs nothing here (a page that is on Bandcamp but
// carries no player still renders nothing; it just is not predicted).
func IsResolvableBandcampURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "bandcamp.com" || IsBandcampArtistHost(host)
}

// BandcampEmbedURLField is the artists column that holds a Bandcamp release
// URL, and BandcampEmbedURLLabel is how it is named to whoever is refused.
//
// They live here, beside the rule they belong to, because three layers need the
// same two strings and utils is the leaf all three already import: the handler
// field registry, the pending-edit apply gate in internal/services/admin (which
// must not import a handler package), and the entity-request payload validator.
// A service spelling the field name itself is how an apply-side gate silently
// stops covering the field a submit-side gate renamed.
const (
	BandcampEmbedURLField = "bandcamp_embed_url"
	BandcampEmbedURLLabel = "Bandcamp embed URL"
	// MaxBandcampEmbedURLLen sizes the column-shaped cap the queues already
	// applied, so every boundary onto this TEXT column agrees on what fits.
	MaxBandcampEmbedURLLen = 2048
)

// ValidateBandcampEmbedURL is IsValidBandcampEmbedURL as a write-boundary
// check, returning the one refusal message every path that stores
// artists.bandcamp_embed_url shows. Keeping the message here rather than at each
// caller is what stops the same rejection reading three different ways across
// the admin endpoint, the suggest-edit queue and the entity-request queue.
//
// The message states the accepted shape by example instead of naming the rule
// that failed. A submitter cannot act on "host anchor failed"; they can act on
// seeing the form of URL that works, and one message covers every way the value
// can be wrong.
//
// ONLY the empty string passes as the clear-the-field gesture. A whitespace-only
// value is refused, because it would otherwise be stored blank-but-not-null:
// "has an embed" to every IS NULL backfill and the playable-audio flags, and
// nothing at all to a reader.
//
// The length cap is here rather than at each boundary because this is the only
// check the direct admin endpoint runs (its request struct carries no maxLength,
// and huma's validate tags are inert in this codebase), while both queues cap at
// 2048. Without it the three write paths disagree about what fits.
//
// fieldName is the user-facing label. It is a parameter rather than a constant
// because the entity-request queue names the field as it appears in the stored
// payload, while the direct endpoints name it as the form labels it.
func ValidateBandcampEmbedURL(value, fieldName string) error {
	if value == "" {
		return nil
	}
	if len(value) > MaxBandcampEmbedURLLen {
		return fmt.Errorf("%s must be %d characters or fewer", fieldName, MaxBandcampEmbedURLLen)
	}
	if !IsValidBandcampEmbedURL(value) {
		return fmt.Errorf(
			"%s must be an https link to a Bandcamp album or track page "+
				"(for example https://artist.bandcamp.com/album/title)",
			fieldName,
		)
	}
	return nil
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
