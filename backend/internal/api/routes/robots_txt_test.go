package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	systemh "psychic-homily-backend/internal/api/handlers/system"
	"psychic-homily-backend/internal/api/middleware"
)

const wantRobotsTxtBody = "User-agent: *\nDisallow: /\n"

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
			if got := rr.Body.String(); got != wantRobotsTxtBody {
				t.Errorf("%s as %q: body = %q", method, ua, got)
			}
		}
	}
}

// With the public-read limiter mounted in front of the built router, an
// anonymous burst from one IP past the per-IP cap still gets robots.txt every
// time, while the same burst against a metered anonymous GET is refused. The
// control proves the burst is large enough to trip the limiter.
func TestRobotsTxtSurvivesAnAnonymousBurst(t *testing.T) {
	handler := PublicReadRateLimiter(nil, nil, nil, enableEnv)(newTestRouter(t))
	burst := middleware.APIRequestsPerMinute + 5

	for i := 0; i < burst; i++ {
		req := httptest.NewRequest(http.MethodGet, systemh.RobotsTxtRoute, nil)
		req.RemoteAddr = "7.7.7.13:100"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK || rr.Body.String() != wantRobotsTxtBody {
			t.Fatalf("robots.txt fetch %d: status = %d, body = %q; want 200 and the file", i, rr.Code, rr.Body.String())
		}
	}

	limited := 0
	for i := 0; i < burst; i++ {
		req := httptest.NewRequest(http.MethodGet, systemh.LegalVersionsRoute, nil)
		req.RemoteAddr = "7.7.7.14:100"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code == http.StatusTooManyRequests {
			limited++
		}
	}
	if limited == 0 {
		t.Fatalf("control: %d anonymous GET %s from one IP were never limited, so the burst proves nothing", burst, systemh.LegalVersionsRoute)
	}
}
