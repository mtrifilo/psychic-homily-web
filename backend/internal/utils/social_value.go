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
// It is for the writers that carry values this system stored BEFORE the anchor
// existed: the admin data import (whose body is normally an export of this
// database) and cmd/seed (whose YAML holds bare handles). The HTTP boundary's
// ValidateSocialURLs refuses a handle, correctly, because a contributor form
// has no legacy rows; cmd/festival-entry uses that stricter spelling for the
// same reason, so it does not call this.
//
// The value is judged, never rewritten: the caller stores what it was given, so
// a row that survives this reads back exactly as it was written.
//
// What this does NOT reproduce is the browser's parser. Go accepts an
// out-of-range port and a punycode label the browser refuses; those are the
// storableButUnrenderable rows in the shared corpus, and they are storable
// through every path, not only this one.
func ValidateStoredSocialValue(field, fieldName, value string) error {
	raw := stripURLEdgeWhitespace(value)
	if strings.TrimSpace(raw) == "" {
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
// Every branch produces an http(s) URL or nothing, and every result goes
// through the anchor in the caller, so widening the tolerance can only ever
// produce a URL for the anchor to judge, never a link off the platform and
// never a non-http scheme.
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
	//
	// The dot is a floor, not a proof of a domain: a dotted numeric value is
	// still read as an IPv4 address by a browser. The anchor below refuses every
	// such value on the seven anchored fields; on the unanchored ones it is a
	// link to an address on the reader's own network, which nothing here can
	// tell apart from a deliberate one.
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

// stripURLEdgeWhitespace removes the surrounding characters a URL parser itself
// removes: C0 controls and space, and nothing else.
//
// strings.TrimSpace is the wrong tool for deciding whether a stored value will
// resolve, because it also strips U+00A0 and the other Unicode spaces, which the
// browser's URL parser KEEPS. A leading U+00A0 (what you get pasting a handle
// out of a rendered page) makes the whole value unparseable to a reader, so a
// gate that trimmed it would certify a value that renders no link at all: the
// one direction the shared corpus forbids.
//
// CROSS-LANGUAGE MIRROR: stripUrlWhitespace in frontend/lib/urlAnchor.ts.
func stripURLEdgeWhitespace(raw string) string {
	return strings.TrimFunc(raw, func(r rune) bool { return r <= 0x20 })
}

// hasHTTPSchemePrefix reports whether a value already carries its own http or
// https scheme. Case-insensitive, because Go lowercases the scheme it parses,
// so "HTTPS://x" clears the write boundary and is a value this side reads back.
func hasHTTPSchemePrefix(raw string) bool {
	lower := strings.ToLower(raw)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

// SocialFieldLabels names each social column as a refusal reports it.
//
// It is the source the other two label tables are held to:
// TestSocialLabelsAgreeAcrossLayers in internal/services/admin asserts that the
// HTTP boundary's urlFieldSpecs and the apply gate's rollbackURLFields spell these
// eight the same way, so an operator sees one wording whichever writer refused
// the value. It lives here because utils is the only one of the three packages
// the other two can both import.
var SocialFieldLabels = map[string]string{
	"instagram":  "Instagram URL",
	"facebook":   "Facebook URL",
	"twitter":    "Twitter URL",
	"youtube":    "YouTube URL",
	"spotify":    "Spotify URL",
	"soundcloud": "SoundCloud URL",
	"bandcamp":   "Bandcamp URL",
	"website":    "Website URL",
}

// SocialColumns is the eight social values of one row. A nil field is a column
// the caller does not write; an empty string is the clear-the-field gesture.
//
// Named fields rather than eight positional *string arguments: every one has
// the same type, so a transposition would compile and judge a value against
// another platform's anchor.
type SocialColumns struct {
	Instagram  *string
	Facebook   *string
	Twitter    *string
	YouTube    *string
	Spotify    *string
	SoundCloud *string
	Bandcamp   *string
	Website    *string
}

// ValidateStoredSocialColumns runs ValidateStoredSocialValue over one row's
// eight social columns and reports the first that would not render as the link
// its column claims.
func ValidateStoredSocialColumns(columns SocialColumns) error {
	pairs := [...]struct {
		field string
		value *string
	}{
		{"instagram", columns.Instagram},
		{"facebook", columns.Facebook},
		{"twitter", columns.Twitter},
		{"youtube", columns.YouTube},
		{"spotify", columns.Spotify},
		{"soundcloud", columns.SoundCloud},
		{"bandcamp", columns.Bandcamp},
		{"website", columns.Website},
	}
	for _, p := range pairs {
		if p.value == nil {
			continue
		}
		if err := ValidateStoredSocialValue(p.field, SocialFieldLabels[p.field], *p.value); err != nil {
			return err
		}
	}
	return nil
}

// DropUnrenderableSocialColumns clears every social column that would not render
// as the link its column claims, and returns the field names it cleared.
//
// It is for a BULK writer moving rows it did not author, where refusing the
// whole row costs more than the bad value does. The admin import is that case:
// a refused artist there is not merely skipped, because the show pass recreates
// it by name with no initializer, losing its location, its verified flag and the
// seven columns that were fine. Clearing one column keeps the rest of the row
// and still stores nothing a reader would refuse.
//
// The caller MUST report the returned names. Dropping a value silently is the
// one outcome neither this nor a refusal may produce: the operator has to learn
// which value did not survive, so they can fix it at the source.
//
// A single-row writer calls ValidateStoredSocialColumns and refuses instead:
// there is no rest-of-the-row to save, and the operator is right there.
func DropUnrenderableSocialColumns(columns *SocialColumns) []string {
	targets := [...]struct {
		field string
		value **string
	}{
		{"instagram", &columns.Instagram},
		{"facebook", &columns.Facebook},
		{"twitter", &columns.Twitter},
		{"youtube", &columns.YouTube},
		{"spotify", &columns.Spotify},
		{"soundcloud", &columns.SoundCloud},
		{"bandcamp", &columns.Bandcamp},
		{"website", &columns.Website},
	}
	var dropped []string
	for _, t := range targets {
		if *t.value == nil {
			continue
		}
		if err := ValidateStoredSocialValue(t.field, SocialFieldLabels[t.field], **t.value); err != nil {
			dropped = append(dropped, t.field)
			*t.value = nil
		}
	}
	return dropped
}
