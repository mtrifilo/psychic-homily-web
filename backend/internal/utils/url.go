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
// allowed to hold. Every write path that stores the column gates on it.
//
// CROSS-LANGUAGE MIRROR: isBandcampReleaseUrl in frontend/lib/bandcamp.ts. The
// names differ because each reads naturally in its own file, so grep for the
// other one from here, and change neither without the shared corpus in
// internal/utils/testdata/bandcamp_url_corpus.json, which both sides assert.
//
// THE CONTRACT between them is one-directional: everything STORABLE here must be
// RENDERABLE there. Nothing may be stored that the renderer would refuse, or a
// curator's save silently produces no link and no one can see why.
//
// The reverse does NOT hold, and do not "simplify" one gate into the other on
// the assumption that it does. This predicate is strictly the tighter of the two
// in several ways beyond the bandcamp.com apex: the browser's parser resolves
// "/foo/../album/y" to /album/y, tolerates a leading space, a trailing tab and a
// stray "%zz", and would render every one of them. Each is refused here on
// purpose. The apex is simply the delta that matters for legacy rows, because it
// is the one where the READ gate is the lenient one.
//
// Six rules, each load-bearing:
//
//   - The value is judged EXACTLY as it will be stored, and surrounding
//     whitespace is refused outright rather than trimmed. Every caller stores the
//     string it handed in, so a validator that trimmed would pass
//     " https://…/album/x" and put the space in the column. A TRAILING space is
//     the one that bites quietly: it survives url.Parse, escapes to "/album/y%20"
//     and would validate, but no longer equals the trimmed URL that
//     selectBandcampEmbedFromReleases derives, so the release-derived recompute
//     would rewrite the row forever. A whitespace-only value is refused for the
//     same family of reasons: stored blank-but-not-null it reads as "has an
//     embed" to every IS NULL backfill and renders as nothing.
//   - https only. The value's whole purpose is to be rendered, and both
//     renderers refuse http: the resolver that turns it into an iframe fetches
//     it through isAllowedBandcampUrl (https), and the link fallback gates on
//     isBandcampReleaseUrl (https). Storing http yields a row that shows
//     nothing at all, so it is refused where someone can still fix it.
//   - Host anchored on the parsed hostname, never a substring of the URL, so
//     "http://169.254.169.254/album/x?bandcamp.com", "bandcamp.com.evil.test"
//     and "evilbandcamp.com" are all rejected.
//   - Path prefix on the path AS WRITTEN in the input, never on anything Go
//     derived from it. Go's u.Path is percent-DECODED, which would accept
//     "/%61lbum/y" and "/album%2Fx": spellings the browser's `pathname`, the
//     thing the read gate reads, keeps encoded and refuses. u.EscapedPath() is
//     not a fix either: it returns RawPath only while RawPath is a valid
//     encoding of Path, and ANY space or non-ASCII byte in the path invalidates
//     that, at which point Go silently re-escapes the DECODED path and hands
//     back "/album/..." for both. A Bandcamp slug with an accent or a CJK
//     character is entirely ordinary, so that fallback is the common case, not
//     an exotic one. rawPath below reads the bytes between the authority and
//     the query, which is what the browser reads too.
//   - No "." or ".." path segment, and no backslash anywhere in the path. Go
//     leaves dot segments in place and treats "\\" as an ordinary character; the
//     browser resolves the first away and treats the second as a segment
//     delimiter. So "/album/../../evil" and "/album/..\\evil" are release pages
//     to one layer and "/evil" to the other. A real release page has neither.
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
	// Surrounding whitespace is refused, not trimmed: see rule 1 above. A leading
	// space happens to make url.Parse fail already; a trailing one does not, so
	// this is the check that actually covers both.
	if rawURL != strings.TrimSpace(rawURL) {
		return false
	}
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
	path := rawPath(rawURL)
	if hasTraversalSegment(path) {
		return false
	}
	// Album or track page, not a bare profile.
	return strings.HasPrefix(path, "/album/") || strings.HasPrefix(path, "/track/")
}

// rawPath returns the path exactly as written in rawURL: the bytes after the
// authority, up to the query or fragment. Empty when there is no path.
//
// It exists because neither of Go's two answers is the browser's. u.Path is
// percent-decoded, so "/%61lbum/y" reads as "/album/y". u.EscapedPath() looks
// like the right answer and is not: it returns the original RawPath only while
// RawPath round-trips through Go's own escaping rules, and a space or any
// non-ASCII byte breaks that, whereupon it re-escapes the DECODED path, so
// "/%61lbum/caf\u00e9" comes back "/album/caf%C3%A9" and passes a prefix test
// the browser fails. Reading the input directly has no such fallback.
//
// The caller compares only an ASCII prefix ("/album/", "/track/"), so the one
// remaining difference from the browser's `pathname`, which percent-encodes
// non-ASCII bytes it was handed literally, cannot change the answer. Dot
// segments and backslashes, the other things a browser resolves before deciding
// what a path is, are rejected outright by hasTraversalSegment.
//
// Safe on input url.Parse already accepted: the scheme is http or https, so the
// "://" split below cannot land inside a path or a query.
func rawPath(rawURL string) string {
	rest := rawURL
	if i := strings.Index(rest, "://"); i >= 0 {
		rest = rest[i+3:]
	}
	// A "?" or "#" before the first "/" means the URL has no path at all.
	if i := strings.IndexAny(rest, "?#"); i >= 0 {
		rest = rest[:i]
	}
	i := strings.Index(rest, "/")
	if i < 0 {
		return ""
	}
	return rest[i:]
}

// hasTraversalSegment reports whether path contains anything a browser would
// resolve away before deciding what the path IS: a "." or ".." segment in either
// spelling, or a backslash.
//
// Both halves are needed and neither is theoretical. Go keeps dot segments
// verbatim while the browser collapses them, so "/album/../../evil" reads as a
// release page here and as "/evil" there. Backslash is worse, because Go treats
// it as an ordinary path character while the WHATWG parser treats it as a
// SEGMENT DELIMITER for special schemes: "/album/..\evil" is one Go segment and
// two browser segments resolving to "/evil". Splitting on "/" alone therefore
// misses it, which is how the first version of this check let it through.
//
// Backslash is rejected outright rather than split on, because no real Bandcamp
// release path contains one, so there is nothing to weigh against refusing it.
// The same rule, for the same reason, already guards the discovery importer's
// platform-URL canonicalizer (pipeline.discover_music); this one additionally
// refuses the percent-encoded spelling, because it reads the escaped path.
//
// It takes an ESCAPED path, so url.PathUnescape below cannot fail: url.Parse has
// already rejected a malformed escape. The error is folded into the reject
// branch anyway so the helper stays correct if a caller ever hands it raw input.
func hasTraversalSegment(path string) bool {
	if strings.Contains(path, `\`) || strings.Contains(strings.ToLower(path), "%5c") {
		return true
	}
	for _, segment := range strings.Split(path, "/") {
		decoded, err := url.PathUnescape(segment)
		if err != nil || decoded == "." || decoded == ".." {
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
// It is the Go mirror of isAllowedBandcampUrl in frontend/lib/bandcamp.ts, the
// precondition /api/bandcamp/album-id enforces before it fetches, and it accepts
// the apex because that predicate does.
//
// Being a READ-side predicate it models the BROWSER, which is why it trims where
// IsValidBandcampEmbedURL refuses. The WHATWG parser strips surrounding ASCII
// whitespace, so a legacy row holding " https://x.bandcamp.com/album/y" renders
// a working player today; refusing it here would switch that band's playable dot
// off and drop it from the scene picker while the page still plays. An
// affordance vanishing from a row that works is the inverse of what this exists
// to prevent.
//
// It still cannot model the browser exactly: percent-encoded and IDNA host
// spellings that WHATWG folds are refused here, and the residue only ever
// UNDER-reports: a row that plays but shows no dot. That is the safe direction
// for a decorative marker, and closing it would mean reimplementing WHATWG host
// parsing in Go.
func IsResolvableBandcampURL(rawURL string) bool {
	u, err := url.Parse(strings.TrimSpace(rawURL))
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
// ONLY the empty string passes as the clear-the-field gesture, and a caller that
// accepts it MUST normalize it to NULL before storing (utils.NilIfBlank): see
// BlankBandcampEmbedToNil. A whitespace-only value is refused outright. Both
// rules serve the same invariant: the column is either NULL or a renderable URL,
// never blank-but-not-null, which would read as "has an embed" to every IS NULL
// backfill and the playable-audio flags while rendering nothing.
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

// socialHostSuffixes anchors each platform social field to its known hosts, so a
// hostile value (https://evil.test/artist/x in the spotify field, which renders
// as a SocialLinks href) cannot be stored (PSY-1113). A field absent here
// (website) accepts any host. A host matches when it equals a base or is a
// subdomain of it: covering open.spotify.com, <artist>.bandcamp.com,
// m.facebook.com, music.youtube.com, www.*, etc.
//
// Redirector / short-link hosts (fb.me, t.co, youtube-nocookie.com) are
// intentionally excluded: they cannot be statically verified to land
// on-platform, which is the point of the anchor. `website` is the escape hatch.
//
// It lives in utils, not beside the handler validator that has always used it,
// because PSY-1966 gave it a SECOND consumer that cannot import a handler
// package: the rollback gate in internal/services/admin. Copying it there would
// have made a third spelling of a security allowlist. One table, two callers.
//
// UNEXPORTED on purpose. Only ValidateSocialHost below is exported, so a
// security allowlist in a leaf package everything imports cannot be reached, or
// mutated from some package's init, by anything but the rule that owns it.
var socialHostSuffixes = map[string][]string{
	"instagram":  {"instagram.com"},
	"facebook":   {"facebook.com", "fb.com"},
	"twitter":    {"twitter.com", "x.com"},
	"youtube":    {"youtube.com", "youtu.be"},
	"spotify":    {"spotify.com"},
	"soundcloud": {"soundcloud.com"},
	"bandcamp":   {"bandcamp.com"},
}

// ValidateSocialHost reports whether value sits on the platform allowlisted for
// field, returning a plain error naming the accepted hosts. Fields absent from
// SocialHostSuffixes are unrestricted and always pass.
//
// A parse failure is a PASS, not a rejection: callers run the scheme check
// first, which is what rejects unparseable input, and answering "must be a link
// on instagram.com" for a value that is not a URL at all would report the wrong
// problem.
func ValidateSocialHost(field, fieldName, value string) error {
	bases, restricted := socialHostSuffixes[field]
	if !restricted || strings.TrimSpace(value) == "" {
		return nil
	}
	u, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return nil
	}
	host := strings.ToLower(u.Hostname())
	for _, base := range bases {
		if host == base || strings.HasSuffix(host, "."+base) {
			return nil
		}
	}
	return fmt.Errorf("%s must be a link on %s", fieldName, strings.Join(bases, " or "))
}

// BlankBandcampEmbedToNil converts the clear-the-field gesture into the value the
// column must actually hold for it.
//
// ValidateBandcampEmbedURL passes the empty string, so without this an approve
// writes ” and the row becomes blank-but-not-null: invisible to every
// `bandcamp_embed_url IS NULL` gate: the profile resolver, the release-derived
// fill, cmd/backfill-artist-bandcamp-embeds, cmd/sweep-link-suggestions, which
// means the artist can never be repaired by any automated path again, while
// still rendering nothing. That is the exact state the whitespace refusal exists
// to prevent, reached by the one input the validator has to allow.
//
// It takes and returns `any` because its caller is the untyped pending-edit
// updates map. A non-string is returned untouched: it is not this function's job
// to judge, and revalidateShapedURLs has already refused it.
func BlankBandcampEmbedToNil(value any) any {
	s, ok := value.(string)
	if !ok || strings.TrimSpace(s) != "" {
		return value
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
