package utils

import (
	"fmt"
	"strings"
)

// socialHandleBases is where a bare handle stored in a social column resolves.
//
// It exists because the columns hold three shapes, not one: a full URL, a
// scheme-less domain, and a bare handle ("calexico", "@calexico"). Only the
// first is storable through an HTTP write path, and the other two are what the
// dev seed's YAML and any legacy row carry, so a gate that judged them as URLs
// would refuse values that resolve onto the platform and render correctly.
//
// A field absent here accepts no handle: bandcamp's account URL is a subdomain
// rather than a path, and website anchors no host, so in both cases there is
// nothing a handle could be appended to.
//
// spotify's base is the web player rather than the apex, while its ANCHOR is
// spotify.com: the base is a subdomain of the anchor, so a resolved handle
// clears the anchor like any other value.
//
// CROSS-LANGUAGE MIRROR: handleBase in frontend/lib/socialLinks.ts. Both sides
// assert this table against testdata/social_link_corpus.json, so a base changed
// in one language and not the other fails the other language's suite.
var socialHandleBases = map[string]string{
	"instagram":  "https://instagram.com/",
	"facebook":   "https://facebook.com/",
	"twitter":    "https://twitter.com/",
	"youtube":    "https://youtube.com/",
	"spotify":    "https://open.spotify.com/",
	"soundcloud": "https://soundcloud.com/",
}

// ValidateStoredSocialValue applies the write boundary's two rules (the http
// scheme rule and the per-platform host anchor) to the URL a stored social
// value RESOLVES to, so a legacy handle is judged by where it lands rather than
// by the fact that it is not a URL.
//
// It is for the writers that do not sit behind an HTTP request struct: the
// admin data import, cmd/seed and cmd/festival-entry. Those carry values from
// an operator's YAML, JSON export or prompt, where the handle shape is normal
// and predates every gate; ValidateSocialURLs at the HTTP boundary refuses it,
// which is correct there because a contributor form has no legacy rows.
//
// The value is judged, never rewritten: the caller stores what it was given, so
// a row that survives this reads back exactly as it was written.
//
// What this does NOT reproduce is the browser's parser. Go accepts an
// out-of-range port and a punycode label the browser refuses; those are the
// storableButUnrenderable rows in the shared corpus, and they are storable
// through every path, not only this one.
func ValidateStoredSocialValue(field, fieldName, value string) error {
	raw := strings.TrimSpace(value)
	if raw == "" {
		return nil
	}
	candidate, ok := resolveStoredSocialValue(field, raw)
	if !ok {
		return fmt.Errorf("%s must be a link or a handle (got %q)", fieldName, raw)
	}
	if err := ValidateHTTPURL(candidate, fieldName); err != nil {
		return err
	}
	return ValidateSocialHost(field, fieldName, candidate)
}

// resolveStoredSocialValue turns a stored value into the URL a reader resolves
// it to, and reports false for a value that resolves to nothing.
//
// Every branch produces an https URL or nothing, so widening the tolerance can
// only ever produce a URL for the anchor to judge, never a link off the
// platform and never a non-http scheme.
//
// CROSS-LANGUAGE MIRROR: normalizeSocialValue in frontend/lib/socialLinks.ts.
func resolveStoredSocialValue(field, raw string) (string, bool) {
	if hasHTTPSchemePrefix(raw) {
		return raw, true
	}
	if base, hasBase := socialHandleBases[field]; hasBase && isSocialHandleShaped(raw) {
		return base + strings.TrimPrefix(raw, "@"), true
	}
	// A scheme-less value is repaired only when it is domain-shaped. The dot is
	// what separates a domain from a value that is neither URL nor handle:
	// without it a stored "123" repairs to https://123, which every browser
	// resolves to the host 0.0.0.123.
	if strings.Contains(raw, ".") {
		return "https://" + raw, true
	}
	return "", false
}

// isSocialHandleShaped reports whether a scheme-less value is a handle rather
// than a URL somebody left the scheme off.
//
// A dot alone does not disqualify a handle: "fashion.club.la" and "jia._.pet"
// are real Instagram handles, and the dev seed carries both. What disqualifies
// one is a dot together with a path separator or a common TLD, which is what
// "instagram.com/calexico" has and a handle does not.
//
// A colon is either a scheme or a port, so a value carrying one is never a
// handle: without that rule "javascript:alert(1)" in the instagram column
// resolves to a real on-platform 404 and "spotify:artist:x" resolves to a page
// on the web player.
//
// Lowercased first, because a host is case-insensitive and "EVIL.COM" must take
// the same branch as "evil.com" rather than being pasted onto a platform base.
//
// CROSS-LANGUAGE MIRROR: isHandleShaped in frontend/lib/socialLinks.ts.
func isSocialHandleShaped(raw string) bool {
	if strings.Contains(raw, ":") {
		return false
	}
	value := strings.ToLower(raw)
	if !strings.Contains(value, ".") {
		return true
	}
	return !strings.Contains(value, "/") &&
		!strings.Contains(value, ".com") &&
		!strings.Contains(value, ".org")
}

// hasHTTPSchemePrefix reports whether a value already carries its own http or
// https scheme. Case-insensitive, because Go lowercases the scheme it parses,
// so "HTTPS://x" clears the write boundary and is a value this side reads back.
func hasHTTPSchemePrefix(raw string) bool {
	lower := strings.ToLower(raw)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

// SocialColumnFields is the field name of each social column, in the order the
// eight positional arguments of the HTTP boundary's ValidateSocialURLs take
// them.
//
// Exported so a writer outside the handler layer can drive one loop over a
// row's social values instead of restating the field names, which is how a
// column added to the model reaches every gate at once.
var SocialColumnFields = [8]string{
	"instagram", "facebook", "twitter", "youtube",
	"spotify", "soundcloud", "bandcamp", "website",
}

// SocialColumnLabels names each social column as a refusal reports it. The
// labels match the HTTP boundary's urlFieldSpecs, so one operator sees one
// wording whichever writer refused the value.
var SocialColumnLabels = map[string]string{
	"instagram":  "Instagram URL",
	"facebook":   "Facebook URL",
	"twitter":    "Twitter URL",
	"youtube":    "YouTube URL",
	"spotify":    "Spotify URL",
	"soundcloud": "SoundCloud URL",
	"bandcamp":   "Bandcamp URL",
	"website":    "Website URL",
}

// ValidateStoredSocialColumns runs ValidateStoredSocialValue over the eight
// social columns of one row, taking them in the same positional order as the
// HTTP boundary's ValidateSocialURLs so the two cannot be read as different
// field sets. A nil pointer is a column the caller does not write.
func ValidateStoredSocialColumns(instagram, facebook, twitter, youtube, spotify, soundcloud, bandcamp, website *string) error {
	values := [8]*string{instagram, facebook, twitter, youtube, spotify, soundcloud, bandcamp, website}
	for i, field := range SocialColumnFields {
		if values[i] == nil {
			continue
		}
		if err := ValidateStoredSocialValue(field, SocialColumnLabels[field], *values[i]); err != nil {
			return err
		}
	}
	return nil
}
