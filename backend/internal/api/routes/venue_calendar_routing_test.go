package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// PSY-1584. The venue ICS feed is a chi route registered next to Huma-registered
// siblings on the same path prefix, which is the exact shape chi resolves by
// PATTERN rather than by parameter NAME. Registering it as `/venues/{slug}/…`
// would not create a second route — it would silently rename the parameter on
// whichever registration lost, and no handler-level test could observe it,
// because handler tests call the handler function directly.
//
// These tests speak to the BUILT ROUTER, the only place that is visible.

func TestVenueCalendarFeedRouteIsRegisteredOnce(t *testing.T) {
	routes := chiRoutes(t, newTestRouter(t))

	// GET and HEAD both: calendar clients and link checkers probe with HEAD,
	// and an unregistered HEAD returns 405 rather than the feed's headers.
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		got := matching(routes, method, "/venues/{}/calendar.ics")
		if len(got) != 1 {
			t.Errorf("%s /venues/{}/calendar.ics: %d registered routes %v, want exactly 1",
				method, len(got), got)
			continue
		}
		if got[0] != "/venues/{venue_id}/calendar.ics" {
			t.Errorf("%s feed route resolved to %q, want %q — the parameter must match its "+
				"Huma siblings or chi silently renames one of them",
				method, got[0], "/venues/{venue_id}/calendar.ics")
		}
	}
}

// PSY-1590: the venue field-note rollup. Asserted separately from
// TestVenueSubRoutesShareOneParameterName below because that test iterates
// whatever it FINDS — a route that vanished entirely would pass it vacuously,
// and this one is registered in comments.go, a file nothing else in the venue
// group would make you open.
func TestVenueFieldNotesRouteIsRegisteredOnce(t *testing.T) {
	routes := chiRoutes(t, newTestRouter(t))

	got := matching(routes, http.MethodGet, "/venues/{}/field-notes")
	if len(got) != 1 {
		t.Fatalf("GET /venues/{}/field-notes: %d registered routes %v, want exactly 1",
			len(got), got)
	}
	if got[0] != "/venues/{venue_id}/field-notes" {
		t.Errorf("rollup route resolved to %q, want %q — the parameter must match its "+
			"venue siblings or chi silently renames one of them",
			got[0], "/venues/{venue_id}/field-notes")
	}
}

// Every venue sub-resource must agree on the parameter name at the same
// position. A single disagreeing registration is what breaks the whole group.
func TestVenueSubRoutesShareOneParameterName(t *testing.T) {
	routes := chiRoutes(t, newTestRouter(t))

	for _, shape := range []string{
		"/venues/{}",
		"/venues/{}/shows",
		"/venues/{}/shows/years",
		"/venues/{}/shows/months",
		"/venues/{}/genres",
		"/venues/{}/bill-network",
		"/venues/{}/calendar.ics",
		// PSY-1590. Registered in comments.go rather than venues.go — it is a
		// field-note read that happens to hang off a venue — which is exactly
		// the case this guard exists for: the file that owns the shape and the
		// file that adds to it are different, so nothing but the built router
		// can catch a disagreeing parameter name.
		"/venues/{}/field-notes",
	} {
		for _, pattern := range matching(routes, http.MethodGet, shape) {
			if pattern != "/venues/{venue_id}"+shape[len("/venues/{}"):] {
				t.Errorf("GET %s registered as %q — every /venues/{venue_id}/… route must use "+
					"the same parameter name", shape, pattern)
			}
		}
	}
}

// The feed must actually be reachable. An unregistered path 404s in chi before
// any handler runs, so anything other than a 404 proves the route matched — and
// with a nil-DB test container the service errors out, which is a 500.
func TestVenueCalendarFeedPathResolves(t *testing.T) {
	router := newTestRouter(t)

	for _, method := range []string{http.MethodGet, http.MethodHead} {
		req := httptest.NewRequest(method, "/venues/some-venue/calendar.ics", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code == http.StatusNotFound || w.Code == http.StatusMethodNotAllowed {
			t.Errorf("%s /venues/some-venue/calendar.ics = %d — the route is not registered",
				method, w.Code)
		}
	}
}

// The venue feed is anonymous and unauthenticated, unlike the token-bearing
// personal feeds. It must therefore stay on the ordinary public-read budget:
// adding it to the personal-feed exemption would hand an unmetered lane to a
// public endpoint that scrapers can enumerate.
func TestVenueCalendarFeedIsNotExemptFromRateLimiting(t *testing.T) {
	path := "/venues/the-rebel-lounge/calendar.ics"

	for _, prefix := range personalFeedPathPrefixesExemptFromRateLimit {
		if len(path) >= len(prefix) && path[:len(prefix)] == prefix {
			t.Errorf("venue feed path %q matches rate-limit exemption prefix %q — a public "+
				"unauthenticated feed must stay metered", path, prefix)
		}
	}
	for _, exact := range infraPathsExemptFromRateLimit {
		if path == exact {
			t.Errorf("venue feed path %q is exempt from rate limiting", path)
		}
	}
}

// ...and the limiter it lands on must actually cover the method the feed is
// fetched with. limitReadMethodsOnly passes non-read methods straight through,
// so a feed served over an uncovered method would be silently unmetered.
func TestVenueCalendarFeedMethodsAreRateLimited(t *testing.T) {
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
		req := httptest.NewRequest(method, "/venues/the-rebel-lounge/calendar.ics", nil)
		handler.ServeHTTP(httptest.NewRecorder(), req)

		if len(limited) != 1 {
			t.Errorf("%s on the venue feed bypassed the public-read limiter", method)
		}
	}
}
