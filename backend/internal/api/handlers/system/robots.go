package system

import (
	"net/http"

	"psychic-homily-backend/internal/respond"
)

// RobotsTxtRoute is the path RobotsTxtHandler is registered on.
const RobotsTxtRoute = "/robots.txt"

// robotsTxtBody keeps crawlers off this host except for its subscription feeds.
//
// This host serves the JSON the public site's pages read, calendar (iCal) and
// Atom feeds for subscribers, and email unsubscribe pages. The public site's
// own robots.txt governs crawler access to pages. Here crawlers get the feeds
// only: calendar and feed services fetch a subscription as a crawler, and one
// that honours robots.txt must still reach it.
//
// One Allow line per feed route family the router serves:
//
//	/feeds/phcal_            personal saved-shows iCal and follows Atom feeds;
//	                         every route under /feeds/ is a token feed and
//	                         every feed token carries the phcal_ prefix
//	/calendar/phcal_         legacy personal iCal alias; the phcal_ prefix
//	                         keeps the session-authenticated /calendar/token
//	                         JSON out
//	/shows/*/calendar.ics    public show feed
//	/venues/*/calendar.ics   public venue feed
//	/scenes/*/calendar.ics   public scene feed
//
// The personal feed lines are plain prefixes, so a parser without `*` support
// still allows them; the public feed lines rely on `*`.
//
// Under RFC 9309 and Google's matching the longest matching rule wins, so each
// Allow outranks Disallow: /. The Allow lines come first for parsers that take
// the first match instead. No pattern ends in $, so a feed URL carrying a query
// string still matches. Because `*` also spans the query, a query string that
// itself contains /calendar.ics admits the URL under the public feed lines.
const robotsTxtBody = "User-agent: *\n" +
	"Allow: /feeds/phcal_\n" +
	"Allow: /calendar/phcal_\n" +
	"Allow: /shows/*/calendar.ics\n" +
	"Allow: /venues/*/calendar.ics\n" +
	"Allow: /scenes/*/calendar.ics\n" +
	"Disallow: /\n"

// robotsTxtCacheControl lets a crawler or a shared cache reuse the file for a
// day.
const robotsTxtCacheControl = "public, max-age=86400"

// RobotsTxtHandler serves robotsTxtBody. The answer is the same for every
// caller, whatever its user agent or credentials.
func RobotsTxtHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", robotsTxtCacheControl)
	respond.SafeWrite(r.Context(), w, []byte(robotsTxtBody))
}
