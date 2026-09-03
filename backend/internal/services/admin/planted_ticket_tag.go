package admin

import (
	"net/url"
	"strings"
)

// A tag found in a STORED ticket URL was planted by whoever submitted the row:
// this app never writes an affiliate parameter into the database, it appends
// one at render time. The row is therefore monetized for somebody else, on a
// page the site publishes.

// plantedTagPattern is the POSIX regular expression the SQL predicate binds. It
// matches a known affiliate parameter carrying a non-empty value, preceded by a
// query separator so `xirmp=` is not a match. Case-sensitive, because query
// keys are: `IRMP` is a spelling no vendor reads.
//
// Narrower than the render-side matcher in two ways, both in the
// under-reporting direction: a parameter name spelled with percent escapes
// (`%69rmp`) is not matched, and neither is one whose value is entirely
// percent-encoded separators. Widening it would need a decode step SQL has no
// cheap spelling for.
var plantedTagPattern = "[?&;](" + strings.Join(knownAffiliateParams, "|") + ")=[^&;#]"

// plantedTagColumnSQL builds the predicate over one ticket_url column.
//
// The fragment is dropped first: text after `#` never reaches the vendor's
// server, so a parameter spelled there credits nobody and is not a planted tag.
func plantedTagColumnSQL(column string) string {
	return column + " IS NOT NULL AND split_part(" + column + ", '#', 1) ~ ?"
}

// plantedAffiliateTag reports the affiliate parameter a stored ticket URL
// credits and the host it credits it on.
//
// Returns the parameter NAME and the HOST only. The value is a third party's
// account identifier and the rest of the URL is contributor text; neither
// belongs in an admin list that is rendered as prose.
func plantedAffiliateTag(rawURL string) (param, host string, ok bool) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return "", "", false
	}
	beforeFragment, _, _ := strings.Cut(trimmed, "#")

	_, query, hasQuery := strings.Cut(beforeFragment, "?")
	if !hasQuery {
		return "", "", false
	}

	param, ok = firstAffiliateParam(query)
	if !ok {
		return "", "", false
	}
	return param, ticketURLHost(beforeFragment), true
}

// firstAffiliateParam reads the query the way a vendor's server does: `&` and
// `;` both separate pairs, and a key is percent-decoded with `+` meaning space.
// A valueless parameter credits nobody and is not a tag.
func firstAffiliateParam(query string) (string, bool) {
	for _, pair := range strings.FieldsFunc(query, func(r rune) bool {
		return r == '&' || r == ';'
	}) {
		key, value, hasValue := strings.Cut(pair, "=")
		if !hasValue || value == "" {
			continue
		}
		decoded, err := url.QueryUnescape(key)
		if err != nil {
			// An undecodable key is not a parameter name a vendor reads.
			continue
		}
		if isKnownAffiliateParam(decoded) {
			return decoded, true
		}
	}
	return "", false
}

func isKnownAffiliateParam(key string) bool {
	for _, known := range knownAffiliateParams {
		if key == known {
			return true
		}
	}
	return false
}

// ticketURLHost reads the host out of a stored ticket URL, supplying the scheme
// a contributor omitted so `ticketweb.com/e/1` still names a host. Returns the
// empty string when the value names none.
func ticketURLHost(rawURL string) string {
	if parsed, err := url.Parse(rawURL); err == nil && parsed.Host != "" {
		return parsed.Hostname()
	}
	if parsed, err := url.Parse("https://" + rawURL); err == nil {
		return parsed.Hostname()
	}
	return ""
}

// plantedTagReason names the parameter and the host, and nothing else.
func plantedTagReason(rawURL string) string {
	param, host, ok := plantedAffiliateTag(rawURL)
	if !ok || host == "" {
		return "Affiliate parameter in the stored ticket URL"
	}
	return param + " on " + host
}
