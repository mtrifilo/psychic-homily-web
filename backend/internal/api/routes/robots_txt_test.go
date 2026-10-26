package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	systemh "psychic-homily-backend/internal/api/handlers/system"
)

// The built router answers /robots.txt with the same disallow-all file for
// every user agent, on GET and HEAD. Retrieval and training crawlers get the
// same answer as a search crawler and a browser: this host has no allow group.
func TestRobotsTxtDisallowsEveryCrawler(t *testing.T) {
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
			if got := rr.Body.String(); got != "User-agent: *\nDisallow: /\n" {
				t.Errorf("%s as %q: body = %q", method, ua, got)
			}
		}
	}
}
