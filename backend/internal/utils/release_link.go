package utils

import (
	"fmt"
	"maps"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// MaxReleaseLinkURLLen caps release_external_links.url. The column is TEXT, so
// nothing below this layer objects to any length; the cap exists so the write
// boundary and the render gate agree about what fits. The corpus pins the same
// number on the TypeScript side.
const MaxReleaseLinkURLLen = 2048

// How the two halves of a release link are named to whoever is refused.
//
// They are format ARGUMENTS rather than literals at the head of each message
// because Go error strings are lowercase by convention while these are
// user-facing sentences that begin with a field name.
const (
	releaseLinkPlatformLabel = "Platform"
	releaseLinkURLLabel      = "URL"
)

// releaseLinkPlatformHosts is the whole contract for release_external_links:
// the platforms the product knows, and the hosts each one's URL may sit on. A
// row whose platform is absent here, or whose URL host is not anchored to that
// platform's bases, is neither storable nor rendered.
//
// The platform key is what the release page prints as a label, so an unanchored
// URL is worse than a broken link: an arbitrary host wearing a name the reader
// trusts. Anchoring is on the PARSED host, never a substring of the URL, so
// "https://evil.test/?x=bandcamp.com" and "https://bandcamp.com.evil.test/" are
// both refused (pattern: ssrf host anchor).
//
// Redirector and short-link hosts are absent; youtu.be is YouTube's own share
// host, not a general redirector. A host that cannot be shown statically to
// land on-platform defeats the anchor.
//
// Where a company runs a music service AND a forum on the same registrable
// domain, the anchor names the MUSIC host rather than the domain: apple.com
// would admit discussions.apple.com and spotify.com would admit
// community.spotify.com, both of which host writing by strangers, which is the
// surface this table exists to keep out from under a platform's name. The
// platforms whose whole catalogue is user-uploaded (bandcamp, soundcloud,
// youtube, discogs) cannot be narrowed that way, and that residual is accepted:
// there the anchor buys "on the platform", never "vouched for by it".
//
// amazon_music is anchored to music.amazon.com: Amazon's regional storefronts
// are separate registrable domains, and each one added here is a claim about
// Amazon's TLD list.
//
// What it does NOT close, stated so nobody reads more into it: an allowlisted
// host may run its own redirector (youtube.com/redirect), so a click can still
// leave the platform. Closing that needs a path rule this deliberately has not
// got; the platforms' own interstitials are the mitigation.
//
// CROSS-LANGUAGE MIRROR: RELEASE_LINK_PLATFORMS in frontend/lib/releaseLinks.ts.
// Both sides assert this table against the shared corpus in
// testdata/release_link_corpus.json, so a platform or host added to one
// language and not the other fails the other language's suite by name.
var releaseLinkPlatformHosts = map[string][]string{
	"amazon_music":  {"music.amazon.com"},
	"apple_music":   {"music.apple.com", "itunes.apple.com"},
	"bandcamp":      {"bandcamp.com"},
	"deezer":        {"deezer.com"},
	"discogs":       {"discogs.com"},
	"soundcloud":    {"soundcloud.com"},
	"spotify":       {"open.spotify.com"},
	"tidal":         {"tidal.com"},
	"youtube":       {"youtube.com", "youtu.be"},
	"youtube_music": {"music.youtube.com"},
}

// asciiHostRe is the character allowlist a host must satisfy before the suffix
// anchor is trusted.
//
// It is what makes the Go and browser answers the same answer. Go's url.Parse
// already refuses a backslash or a percent escape inside a host, but it keeps
// non-ASCII bytes verbatim while the WHATWG parser runs UTS-46 over them, and
// several code points map to something else there: U+3002 becomes a label
// separator, U+00AD is deleted outright. Restricting the host to the bytes a
// real platform host is made of removes every one of those cases instead of
// reimplementing UTS-46 in Go.
//
// It accepts both cases and is applied to the host AS PARSED, before any
// folding. Lowercasing first would defeat it: strings.ToLower uses simple case
// mapping, so "EVİL.bandcamp.com" (U+0130) folds to the pure-ASCII
// "evil.bandcamp.com" and sails through, while the browser punycodes the same
// input to "xn--evil-swc.bandcamp.com". Two different Bandcamp accounts from one
// stored string.
var asciiHostRe = regexp.MustCompile(`^[A-Za-z0-9.-]+$`)

// asciiPlatformRe is the same rule for the platform key, and for the same
// reason: every key in the registry is ASCII, and folding before checking lets
// U+0130 in "TİDAL" reach the table as "tidal" here while JavaScript's
// SpecialCasing folds it to "ti̇dal" and finds nothing. That row would be stored
// with a 201 and render no link at all.
var asciiPlatformRe = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

// ReleaseLinkPlatforms returns every platform a release link may name, sorted.
//
// Exported so the refusal message, the handlers' OpenAPI doc-tag drift test and
// the corpus test all read the same list rather than restating it.
func ReleaseLinkPlatforms() []string {
	return slices.Sorted(maps.Keys(releaseLinkPlatformHosts))
}

// ReleaseLinkHostsFor returns the host bases allowlisted for a platform,
// and whether the platform is known at all.
//
// The lookup lowercases but does NOT trim: the enrichment writer's dedup index
// is on LOWER(platform), so the system has always treated "Bandcamp" and
// "bandcamp" as one platform, while a padded value is a different string that
// would be stored padded and looked up by nothing. Non-ASCII is refused before
// the fold, so the two languages cannot fold the same key to different strings
// (see asciiPlatformRe).
//
// The returned slice is a copy: the table is a security allowlist in a leaf
// package everything imports, so a caller holding the live slice could widen it
// for the whole process.
func ReleaseLinkHostsFor(platform string) ([]string, bool) {
	if !asciiPlatformRe.MatchString(platform) {
		return nil, false
	}
	bases, ok := releaseLinkPlatformHosts[strings.ToLower(platform)]
	if !ok {
		return nil, false
	}
	return slices.Clone(bases), true
}

// ValidateReleaseLink is the write-boundary check, and the single statement of
// what a release link may be. Every writer of the column gates on it (cmd/seed
// checks its own literals; everything else goes through ReleaseService), so a
// value that is stored is a value the release page will render.
//
// It returns the refusal message every path that stores a link shows, so the
// same rejection cannot read three different ways across the contributor
// endpoint, the admin create funnel and the CLIs.
//
// CROSS-LANGUAGE MIRROR: isRenderableReleaseLink in frontend/lib/releaseLinks.ts
// is the render gate, and it is the one that decides what the page shows. The
// two are held together by the shared corpus in
// testdata/release_link_corpus.json, which both suites assert.
//
// The messages name the accepted value rather than the rule that failed: a
// submitter cannot act on "host anchor failed", but can act on seeing the
// platforms that exist and the hosts a Spotify link may sit on.
//
// The rules, each load-bearing:
//
//   - Both halves are required. Unlike the optional URL fields elsewhere in the
//     codebase there is no clear-the-field gesture here: a blank platform or URL
//     is a row that renders nothing.
//   - The platform is one of the registered keys, matched case-insensitively.
//     This also closes a 22001 from Postgres on the VARCHAR(50) column, which
//     used to surface as a 500 no submitter could act on.
//   - The URL is judged EXACTLY as it will be stored: surrounding whitespace is
//     refused rather than trimmed. Callers store the string they were handed,
//     and the browser's parser trims before resolving, so trimming here would
//     put a value in the column that only one of the two layers agrees about.
//   - http or https only, matching the scheme rule every other URL field in
//     this codebase applies. Everything else (javascript:, data:, mailto:) is
//     refused.
//   - No userinfo. No real platform URL carries any, and the release card prints
//     the stored URL as its caption, truncated from the right, so
//     "https://your-account-is-suspended.example.com@open.spotify.com/album/x"
//     reads as another domain with the real host cut off. What separates this
//     from any long hostname is that userinfo is not part of the host in ANY
//     parser: it is text a browser discards, so the caption names a domain the
//     click does not go near. The render gate refuses it for the same reason
//     rather than treating it as one of its lenient cases. A misleading
//     SUBDOMAIN is a different thing and neither gate closes it, because that is
//     where the click genuinely goes.
//   - The host is ASCII-only in the [a-z0-9.-] set, so the suffix anchor means
//     the same thing to Go and to a browser (see asciiHostRe).
//   - No label spelled in punycode. Go treats "xn--" as four ordinary bytes,
//     while the WHATWG parser decodes it and THROWS on a malformed one, so
//     "xn--a.bandcamp.com" is storable here and unparseable, and therefore
//     unrenderable, there. No platform host in the registry is an IDN, so the
//     rule costs nothing; the render gate stays lenient about the well-formed
//     ones a legacy row might hold.
//   - A port, if present, is one a browser would accept. Go's parser takes any
//     run of digits, while the WHATWG parser refuses anything above 65535
//     outright, so without this a value is storable here and unparseable, and
//     therefore unrenderable, there.
//   - The host equals one of the platform's bases or is a subdomain of it.
//
// It deliberately says nothing about the PATH. A release link is "this release
// on that platform", and each platform spells that differently; a path rule
// would refuse real links (locale-prefixed Spotify URLs, Apple Music's country
// segment) to buy nothing, because the host anchor already decides where a
// click lands. The path-strict rules are the embed validators
// (IsValidBandcampEmbedURL, parseSpotifyEmbed), which answer a narrower
// question about one URL rather than about the column.
//
// Nothing here fetches the value, so the win is keeping a hostile or foreign
// host out of an href wearing a platform's name; it is not SSRF.
func ValidateReleaseLink(platform, rawURL string) error {
	if strings.TrimSpace(platform) == "" {
		return fmt.Errorf("%s is required", releaseLinkPlatformLabel)
	}
	if strings.TrimSpace(rawURL) == "" {
		return fmt.Errorf("%s is required", releaseLinkURLLabel)
	}
	bases, known := ReleaseLinkHostsFor(platform)
	if !known {
		return fmt.Errorf(
			"%s must be one of: %s (got %q)",
			releaseLinkPlatformLabel, strings.Join(ReleaseLinkPlatforms(), ", "), platform,
		)
	}
	if len(rawURL) > MaxReleaseLinkURLLen {
		return fmt.Errorf(
			"%s must be %d characters or fewer",
			releaseLinkURLLabel, MaxReleaseLinkURLLen,
		)
	}
	// Separated from the refusal below because the host sentence misdirects when
	// the value never parsed: "must be on bandcamp.com" is unhelpful for
	// ".../album/100%-pure", which IS on bandcamp.com and is simply not a URL.
	// That sentence is what an operator reads in the enrichment sweep's error
	// list, so it has to name the real problem.
	if _, err := url.Parse(rawURL); err != nil {
		return fmt.Errorf("%s must be a valid URL (got %q)", releaseLinkURLLabel, rawURL)
	}
	if !isAnchoredPlatformURL(rawURL, bases) {
		return fmt.Errorf(
			"%s link must be an http or https URL on %s (got %q)",
			strings.ToLower(platform), strings.Join(bases, " or "), rawURL,
		)
	}
	return nil
}

// isAnchoredPlatformURL applies the scheme, whitespace and host rules named in
// ValidateReleaseLink's doc comment.
func isAnchoredPlatformURL(rawURL string, bases []string) bool {
	if rawURL != strings.TrimSpace(rawURL) {
		return false
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	if u.User != nil {
		return false
	}
	// Checked BEFORE the fold; see asciiHostRe.
	if !asciiHostRe.MatchString(u.Hostname()) {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, label := range strings.Split(host, ".") {
		if strings.HasPrefix(label, punycodePrefix) {
			return false
		}
	}
	if port := u.Port(); port != "" {
		// url.Parse has already established the port is all digits.
		n, err := strconv.Atoi(port)
		if err != nil || n > maxTCPPort {
			return false
		}
	}
	return hostMatchesAnyBase(host, bases)
}

const (
	// maxTCPPort is where the WHATWG URL parser stops accepting a port. Go's
	// parser has no such ceiling, and the difference is the whole reason the
	// check exists.
	maxTCPPort = 65535
	// punycodePrefix marks a host label as an encoded IDN. Go passes it through
	// as bytes; a browser decodes it and refuses the whole URL if it does not
	// decode.
	punycodePrefix = "xn--"
)
