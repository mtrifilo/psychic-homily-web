package routes

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	systemh "psychic-homily-backend/internal/api/handlers/system"
)

// A deliberate copy of the served file, not a reference to the handler's
// constant: a change to the crawler policy has to be re-approved here.
const wantRobotsTxtBody = "User-agent: *\n" +
	"Allow: /feeds/phcal_\n" +
	"Allow: /calendar/phcal_\n" +
	"Allow: /shows/*/calendar.ics\n" +
	"Allow: /venues/*/calendar.ics\n" +
	"Allow: /scenes/*/calendar.ics\n" +
	"Disallow: /\n"

// The built router answers /robots.txt with the same file for every user
// agent, on GET and HEAD. Search, retrieval and training crawlers and a browser
// all get one answer: this host names no agent.
func TestRobotsTxtIsTheSameForEveryAgent(t *testing.T) {
	router := newTestRouter(t)

	userAgents := []string{
		"Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)",
		"Mozilla/5.0 (compatible; bingbot/2.0; +http://www.bing.com/bingbot.htm)",
		"Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko); compatible; GPTBot/1.1; +https://openai.com/gptbot",
		"Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko; compatible; ClaudeBot/1.0; +claudebot@anthropic.com)",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36",
	}
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		for _, ua := range userAgents {
			req := httptest.NewRequest(method, systemh.RobotsTxtRoute, nil)
			req.Header.Set("User-Agent", ua)
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("%s %s as %q = %d, want 200", method, systemh.RobotsTxtRoute, ua, rr.Code)
			}
			if got := rr.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
				t.Errorf("%s as %q: Content-Type = %q", method, ua, got)
			}
			if got := rr.Header().Get("Cache-Control"); got != "public, max-age=86400" {
				t.Errorf("%s as %q: Cache-Control = %q", method, ua, got)
			}
			// httptest.ResponseRecorder keeps the body for HEAD too; net/http
			// drops it on the wire.
			if got := rr.Body.String(); got != wantRobotsTxtBody {
				t.Errorf("%s as %q: body = %q", method, ua, got)
			}
		}
	}
}

// robotsRule is one Allow or Disallow line, its pattern compiled to the
// RFC 9309 match: anchored at the start of the path and query, `*` matching
// any run of characters, a trailing `$` anchoring the end.
type robotsRule struct {
	allow   bool
	pattern string
	match   *regexp.Regexp
}

func parseRobotsRules(t *testing.T, body string) []robotsRule {
	t.Helper()
	var rules []robotsRule
	for _, line := range strings.Split(strings.TrimSpace(body), "\n") {
		field, value, ok := strings.Cut(line, ":")
		if !ok {
			t.Fatalf("robots.txt line %q has no field", line)
		}
		value = strings.TrimSpace(value)
		var allow bool
		switch strings.ToLower(field) {
		case "user-agent":
			continue
		case "allow":
			allow = true
		case "disallow":
			allow = false
		default:
			t.Fatalf("robots.txt line %q has an unexpected field", line)
		}
		expr := regexp.QuoteMeta(strings.TrimSuffix(value, "$"))
		expr = "^" + strings.ReplaceAll(expr, `\*`, ".*")
		if strings.HasSuffix(value, "$") {
			expr += "$"
		}
		rules = append(rules, robotsRule{allow: allow, pattern: value, match: regexp.MustCompile(expr)})
	}
	return rules
}

// robotsAllows applies RFC 9309 precedence, which Google follows too: the
// matching rule with the longest pattern wins, Allow wins a tie, and a path no
// rule matches is allowed.
func robotsAllows(rules []robotsRule, target string) bool {
	allowed, longest := true, -1
	for _, rule := range rules {
		if !rule.match.MatchString(target) {
			continue
		}
		if n := len(rule.pattern); n > longest || (n == longest && rule.allow) {
			allowed, longest = rule.allow, n
		}
	}
	return allowed
}

func servedRobotsRules(t *testing.T, router http.Handler) []robotsRule {
	t.Helper()
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, systemh.RobotsTxtRoute, nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("GET %s = %d, want 200", systemh.RobotsTxtRoute, rr.Code)
	}
	return parseRobotsRules(t, rr.Body.String())
}

// Under the standard's matching, the served file lets a crawler fetch the
// calendar and Atom feed URLs subscribers hand to their calendar or reader, and
// nothing else: the JSON the site's pages read, the session-authenticated
// token endpoint beside the legacy feed alias, and the unsubscribe pages stay
// disallowed.
func TestRobotsTxtAllowsOnlyTheFeeds(t *testing.T) {
	rules := servedRobotsRules(t, newTestRouter(t))

	allowed := []string{
		"/feeds/phcal_0123abcd/saved-shows.ics",
		"/feeds/phcal_0123abcd/follows.atom",
		"/calendar/phcal_0123abcd",
		"/shows/some-show-slug/calendar.ics",
		"/venues/42/calendar.ics",
		"/scenes/phoenix-az/calendar.ics",
		"/scenes/phoenix-az/calendar.ics?refresh=1",
	}
	disallowed := []string{
		"/",
		"/shows/some-show-slug",
		"/shows/upcoming?limit=50",
		"/artists/42/aliases",
		"/entities/artist/42/tags",
		"/charts/rank?entity=artist",
		"/calendar/token",
		"/unsubscribe/scene-digest?token=abc",
		"/openapi.json",
	}
	for _, target := range allowed {
		if !robotsAllows(rules, target) {
			t.Errorf("robots.txt disallows feed URL %s", target)
		}
	}
	for _, target := range disallowed {
		if robotsAllows(rules, target) {
			t.Errorf("robots.txt allows non-feed URL %s", target)
		}
	}
}

// A parser without wildcard support reads `*` as a literal character, so only
// plain-prefix Allow lines work for it. The personal feeds, which subscribers
// poll for the life of their calendar, are allowed under that reading too.
func TestRobotsTxtPersonalFeedsNeedNoWildcardSupport(t *testing.T) {
	rules := servedRobotsRules(t, newTestRouter(t))

	prefixOnlyAllows := func(target string) bool {
		allowed, longest := true, -1
		for _, rule := range rules {
			if !strings.HasPrefix(target, rule.pattern) {
				continue
			}
			if n := len(rule.pattern); n > longest || (n == longest && rule.allow) {
				allowed, longest = rule.allow, n
			}
		}
		return allowed
	}
	for _, target := range []string{
		"/feeds/phcal_0123abcd/saved-shows.ics",
		"/feeds/phcal_0123abcd/follows.atom",
		"/calendar/phcal_0123abcd",
	} {
		if !prefixOnlyAllows(target) {
			t.Errorf("a parser without wildcard support disallows personal feed %s", target)
		}
	}
}

// Every GET route the built router serves is a feed exactly when robots.txt
// allows its path, sampled with no query string. A new .ics or .atom route that
// is not allowed, or an Allow line that starts admitting a JSON route's path,
// fails here.
func TestRobotsTxtAllowsExactlyTheServedFeedRoutes(t *testing.T) {
	router := newTestRouter(t)
	rules := servedRobotsRules(t, router)

	param := regexp.MustCompile(`\{[^}]*\}`)
	sample := func(pattern string) string {
		path := param.ReplaceAllStringFunc(pattern, func(p string) string {
			if strings.HasPrefix(p, "{token") {
				return "phcal_0123abcd"
			}
			return "x"
		})
		return strings.ReplaceAll(path, "*", "x")
	}
	isFeed := func(pattern string) bool {
		return strings.HasSuffix(pattern, ".ics") || strings.HasSuffix(pattern, ".atom") ||
			pattern == legacyCalendarFeedRoute
	}

	feeds := 0
	for _, pattern := range chiRoutes(t, router)[http.MethodGet] {
		if pattern == systemh.RobotsTxtRoute {
			continue
		}
		target := sample(pattern)
		switch got := robotsAllows(rules, target); {
		case isFeed(pattern) && !got:
			t.Errorf("feed route %s (%s) is disallowed", pattern, target)
		case !isFeed(pattern) && got:
			t.Errorf("non-feed route %s (%s) is allowed", pattern, target)
		}
		if isFeed(pattern) {
			feeds++
		}
	}
	if feeds == 0 {
		t.Fatal("the router serves no feed route, so this guard checks nothing")
	}
}
