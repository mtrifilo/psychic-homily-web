package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The per-show ICS download is a chi route next to Huma siblings — the same
// pattern-vs-parameter-name failure mode as the venue feed. Full rationale:
// venue_calendar_routing_test.go.

func TestShowCalendarRouteIsRegisteredOnce(t *testing.T) {
	routes := chiRoutes(t, newTestRouter(t))

	for _, method := range []string{http.MethodGet, http.MethodHead} {
		got := matching(routes, method, "/shows/{}/calendar.ics")
		if len(got) != 1 {
			t.Errorf("%s /shows/{}/calendar.ics: %d registered routes %v, want exactly 1",
				method, len(got), got)
			continue
		}
		if got[0] != "/shows/{show_id}/calendar.ics" {
			t.Errorf("%s calendar route resolved to %q, want %q — the parameter must match its "+
				"Huma siblings or chi silently renames one of them",
				method, got[0], "/shows/{show_id}/calendar.ics")
		}
	}
}

// Every show sub-resource must agree on the parameter name at the same
// position. A single disagreeing registration breaks the whole group.
func TestShowSubRoutesShareOneParameterName(t *testing.T) {
	routes := chiRoutes(t, newTestRouter(t))

	for _, shape := range []string{
		"/shows/{}",
		"/shows/{}/calendar.ics",
		"/shows/{}/also-tonight",
		"/shows/{}/timeline",
	} {
		for _, pattern := range matching(routes, http.MethodGet, shape) {
			if pattern != "/shows/{show_id}"+shape[len("/shows/{}"):] {
				t.Errorf("GET %s registered as %q — every /shows/{show_id}/… route must use "+
					"the same parameter name", shape, pattern)
			}
		}
	}
}

// The download must actually be reachable. An unregistered path 404s in chi
// before any handler runs, so anything other than 404/405 proves the route
// matched — with a nil-DB test container the service errors out, which is a 500.
func TestShowCalendarPathResolves(t *testing.T) {
	router := newTestRouter(t)

	for _, method := range []string{http.MethodGet, http.MethodHead} {
		req := httptest.NewRequest(method, "/shows/some-show/calendar.ics", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code == http.StatusNotFound || w.Code == http.StatusMethodNotAllowed {
			t.Errorf("%s /shows/some-show/calendar.ics = %d — the route is not registered",
				method, w.Code)
		}
	}
}

// Anonymous and unauthenticated, so it must stay on the ordinary public-read
// budget — never the personal-feed exemption.
func TestShowCalendarIsNotExemptFromRateLimiting(t *testing.T) {
	path := "/shows/desert-doom-night/calendar.ics"

	if token := personalFeedTokenFromPath(path); token != "" {
		t.Errorf("show calendar path %q reads as personal-feed token %q — a public unauthenticated endpoint must stay metered",
			path, token)
	}
	for _, exact := range infraPathsExemptFromRateLimit {
		if path == exact {
			t.Errorf("show calendar path %q is exempt from rate limiting", path)
		}
	}
}
