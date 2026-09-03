package utils

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// MaxReleaseLinkURLLen caps release_external_links.url. The column is TEXT, so
// nothing below this layer objects to any length; the cap exists so one
// boundary cannot store a value another boundary would refuse, and it matches
// MaxBandcampEmbedURLLen because both hold the same kind of platform URL.
const MaxReleaseLinkURLLen = 2048

// The two halves of a release link, as they are named to whoever is refused.
// They live beside the rule so the refusal wording is decided once rather than
// at each of the handler, service and CLI boundaries that show it.
const (
	ReleaseLinkPlatformLabel = "Platform"
	ReleaseLinkURLLabel      = "URL"
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
// A host matches when it equals a base or is a subdomain of it, which is what
// covers open.spotify.com, <artist>.bandcamp.com, music.apple.com,
// itunes.apple.com, music.youtube.com, music.amazon.com and www.discogs.com
// without listing each.
//
// Redirector and short-link hosts are deliberately absent (youtu.be is the one
// exception, and it is YouTube's own canonical share host, not a general
// redirector). A host that cannot be statically shown to land on-platform
// defeats the anchor.
//
// amazon_music is anchored to amazon.com alone: Amazon's regional storefronts
// (amazon.co.uk, amazon.de, ...) are separate registrable domains and each one
// added here is a claim about Amazon's TLD list. Nothing in this codebase
// writes amazon_music today, so the narrow anchor costs nothing measurable and
// widening it later is one line.
//
// CROSS-LANGUAGE MIRROR: RELEASE_LINK_PLATFORMS in frontend/lib/releaseLinks.ts.
// Both sides assert this table against the shared corpus in
// testdata/release_link_corpus.json, so a platform or host added to one
// language and not the other fails the other language's suite by name.
var releaseLinkPlatformHosts = map[string][]string{
	"amazon_music":  {"amazon.com"},
	"apple_music":   {"apple.com"},
	"bandcamp":      {"bandcamp.com"},
	"deezer":        {"deezer.com"},
	"discogs":       {"discogs.com"},
	"soundcloud":    {"soundcloud.com"},
	"spotify":       {"spotify.com"},
	"tidal":         {"tidal.com"},
	"youtube":       {"youtube.com", "youtu.be"},
	"youtube_music": {"youtube.com"},
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
var asciiHostRe = regexp.MustCompile(`^[a-z0-9.-]+$`)

// ReleaseLinkPlatforms returns every platform a release link may name, sorted.
//
// Exported so the refusal message, the handler registry and the corpus test all
// read the same list rather than restating it.
func ReleaseLinkPlatforms() []string {
	platforms := make([]string, 0, len(releaseLinkPlatformHosts))
	for platform := range releaseLinkPlatformHosts {
		platforms = append(platforms, platform)
	}
	sort.Strings(platforms)
	return platforms
}

// ReleaseLinkPlatformHosts returns the host bases allowlisted for a platform,
// and whether the platform is known at all. The returned slice is a copy: the
// table is a security allowlist and must not be reachable for mutation.
func ReleaseLinkPlatformHosts(platform string) ([]string, bool) {
	bases, ok := releaseLinkPlatformHosts[strings.ToLower(strings.TrimSpace(platform))]
	if !ok {
		return nil, false
	}
	return append([]string(nil), bases...), true
}

// IsRenderableReleaseLink is the ONE named predicate for what a
// release_external_links row is allowed to be. Every write path gates on it and
// the release page renders only rows that satisfy it, so a value that is stored
// always produces a link and a value that would not produce a link is never
// stored.
//
// Platform is matched case-insensitively because the enrichment writer's dedup
// index is on LOWER(platform), so the system has always treated "Bandcamp" and
// "bandcamp" as the same platform; the label lookup on the render side lowers
// too.
//
// Four rules, each load-bearing:
//
//   - The value is judged EXACTLY as it will be stored: surrounding whitespace
//     is refused rather than trimmed. Callers store the string they were handed,
//     and the browser's parser trims before resolving, so trimming here would
//     put a value in the column that only one of the two layers agrees about.
//   - http or https only, matching the scheme rule every other URL field in
//     this codebase applies. Everything else (javascript:, data:, mailto:) is
//     refused.
//   - The host is ASCII-only in the [a-z0-9.-] set, so the suffix anchor below
//     means the same thing to Go and to a browser.
//   - The host equals one of the platform's bases or is a subdomain of it. The
//     leading dot in the suffix test is load-bearing: it rejects
//     "notbandcamp.com" and "bandcamp.com.evil.test" while accepting
//     "<artist>.bandcamp.com".
//
// It deliberately says nothing about the PATH. A release link is "this release
// on that platform", and each platform spells that differently; a path rule
// would refuse real links (locale-prefixed Spotify URLs, Apple Music's
// country segment) to buy nothing, because the host anchor already decides
// where a click lands. The path-strict rules are the embed validators
// (IsValidBandcampEmbedURL, parseSpotifyEmbed), which answer a narrower
// question about one URL rather than about the column.
//
// Nothing here fetches the value, so the win is keeping a hostile or foreign
// host out of an href wearing a platform's name; it is not SSRF.
func IsRenderableReleaseLink(platform, rawURL string) bool {
	bases, ok := ReleaseLinkPlatformHosts(platform)
	if !ok || platform != strings.TrimSpace(platform) {
		return false
	}
	if rawURL != strings.TrimSpace(rawURL) || rawURL == "" {
		return false
	}
	if len(rawURL) > MaxReleaseLinkURLLen {
		return false
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if !asciiHostRe.MatchString(host) {
		return false
	}
	for _, base := range bases {
		if host == base || strings.HasSuffix(host, "."+base) {
			return true
		}
	}
	return false
}

// ValidateReleaseLink is IsRenderableReleaseLink as a write-boundary check,
// returning the refusal message every path that stores a release link shows.
// Keeping the wording here rather than at each caller is what stops the same
// rejection reading three different ways across the contributor endpoint, the
// admin create funnel and the enrichment writer.
//
// The two messages name the accepted value rather than the rule that failed: a
// submitter cannot act on "host anchor failed", but they can act on seeing the
// platforms that exist and the hosts a Spotify link may sit on.
//
// Unlike the optional URL fields elsewhere in the codebase, an empty value is
// refused, not passed: a link row has no clear-the-field gesture, and a blank
// platform or URL is a row that renders nothing.
func ValidateReleaseLink(platform, rawURL string) error {
	if strings.TrimSpace(platform) == "" {
		return fmt.Errorf("%s is required", ReleaseLinkPlatformLabel)
	}
	if strings.TrimSpace(rawURL) == "" {
		return fmt.Errorf("%s is required", ReleaseLinkURLLabel)
	}
	bases, known := ReleaseLinkPlatformHosts(platform)
	if !known || platform != strings.TrimSpace(platform) {
		return fmt.Errorf(
			"%s must be one of: %s (got %q)",
			ReleaseLinkPlatformLabel, strings.Join(ReleaseLinkPlatforms(), ", "), platform,
		)
	}
	if len(rawURL) > MaxReleaseLinkURLLen {
		return fmt.Errorf(
			"%s must be %d characters or fewer",
			ReleaseLinkURLLabel, MaxReleaseLinkURLLen,
		)
	}
	if !IsRenderableReleaseLink(platform, rawURL) {
		return fmt.Errorf(
			"%s link must be an http or https URL on %s (got %q)",
			strings.ToLower(strings.TrimSpace(platform)), strings.Join(bases, " or "), rawURL,
		)
	}
	return nil
}
