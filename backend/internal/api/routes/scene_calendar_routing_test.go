package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// PSY-1782. The scene ICS feed is a chi route registered next to Huma-registered
// siblings on the same path prefix, which is the exact shape chi resolves by
// PATTERN rather than by parameter NAME. Registering it as `/scenes/{scene_slug}/…`
// would not create a second route — it would silently rename the parameter on
// whichever registration lost, and no handler-level test could observe it,
// because handler tests call the handler function directly.
//
// These tests speak to the BUILT ROUTER, the only place that is visible.

func TestSceneCalendarFeedRouteIsRegisteredOnce(t *testing.T) {
	routes := chiRoutes(t, newTestRouter(t))

	// GET and HEAD both: calendar clients and link checkers probe with HEAD, and
	// an unregistered HEAD returns 405 rather than the feed's headers.
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		got := matching(routes, method, "/scenes/{}/calendar.ics")
		if len(got) != 1 {
			t.Errorf("%s /scenes/{}/calendar.ics: %d registered routes %v, want exactly 1",
				method, len(got), got)
			continue
		}
		if got[0] != "/scenes/{slug}/calendar.ics" {
			t.Errorf("%s feed route resolved to %q, want %q — the parameter must match its "+
				"Huma siblings or chi silently renames one of them",
				method, got[0], "/scenes/{slug}/calendar.ics")
		}
	}
}

// Every scene sub-resource must agree on the parameter name at the same
// position. A single disagreeing registration is what breaks the whole group.
func TestSceneSubRoutesShareOneParameterName(t *testing.T) {
	routes := chiRoutes(t, newTestRouter(t))

	for _, shape := range []string{
		"/scenes/{}",
		"/scenes/{}/artists",
		"/scenes/{}/shows",
		"/scenes/{}/genres",
		"/scenes/{}/graph",
		"/scenes/{}/week",
		"/scenes/{}/day",
		"/scenes/{}/calendar.ics",
	} {
		for _, pattern := range matching(routes, http.MethodGet, shape) {
			if pattern != "/scenes/{slug}"+shape[len("/scenes/{}"):] {
				t.Errorf("GET %s registered as %q — every /scenes/{slug}/… route must use "+
					"the same parameter name", shape, pattern)
			}
		}
	}
}

// The feed must actually be reachable. An unregistered path 404s in chi before
// any handler runs, so anything other than a 404 proves the route matched — and
// with a nil-DB test container the service errors out, which is a 500.
func TestSceneCalendarFeedPathResolves(t *testing.T) {
	router := newTestRouter(t)

	for _, method := range []string{http.MethodGet, http.MethodHead} {
		req := httptest.NewRequest(method, "/scenes/some-scene/calendar.ics", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code == http.StatusNotFound || w.Code == http.StatusMethodNotAllowed {
			t.Errorf("%s /scenes/some-scene/calendar.ics = %d — the route is not registered",
				method, w.Code)
		}
	}
}

// The scene feed is anonymous and unauthenticated, exactly like the venue feed
// and unlike the token-bearing personal feeds. It must therefore stay on the
// ordinary public-read budget: adding it to the personal-feed exemption would
// hand an unmetered lane to a public endpoint that scrapers can enumerate.
func TestSceneCalendarFeedIsNotExemptFromRateLimiting(t *testing.T) {
	path := "/scenes/phoenix-az/calendar.ics"

	for _, prefix := range personalFeedPathPrefixesExemptFromRateLimit {
		if len(path) >= len(prefix) && path[:len(prefix)] == prefix {
			t.Errorf("scene feed path %q matches rate-limit exemption prefix %q — a public "+
				"unauthenticated feed must stay metered", path, prefix)
		}
	}
	for _, exact := range infraPathsExemptFromRateLimit {
		if path == exact {
			t.Errorf("scene feed path %q is exempt from rate limiting", path)
		}
	}
}

// ...and the limiter it lands on must cover BOTH methods the feed is fetched
// with. limitReadMethodsOnly dispatches on r.Method alone and passes non-read
// methods straight through, so a feed served over an uncovered method would be
// silently unmetered.
//
// This pins the METHOD coverage only — the wrapper never sees a route, so the
// path below is illustrative. That the feed's path lands on this limiter rather
// than on an exemption is what the test above establishes.
func TestPublicReadLimiterCoversSceneFeedMethods(t *testing.T) {
	var limited []string
	limiter := limitReadMethodsOnly(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			limited = append(limited, r.Method)
			next.ServeHTTP(w, r)
		})
	})
	handler := limiter(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	for _, method := range []string{http.MethodGet, http.MethodHead} {
		limited = nil
		req := httptest.NewRequest(method, "/scenes/phoenix-az/calendar.ics", nil)
		handler.ServeHTTP(httptest.NewRecorder(), req)

		if len(limited) != 1 {
			t.Errorf("%s on the scene feed bypassed the public-read limiter", method)
		}
	}
}
