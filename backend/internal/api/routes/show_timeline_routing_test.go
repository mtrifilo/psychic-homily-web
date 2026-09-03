package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The gig timeline is a /shows/{show_id}/… sub-route on a public, anonymous,
// id-enumerable path that fans out to four database reads. chi resolves a
// shape+method collision by silently keeping the LAST registration rather than
// erroring, which is the failure that left the artist report handler
// unreachable (see reports_routing_test.go), so the shape is pinned here.
//
// These tests speak to the BUILT ROUTER, which is the only place either
// property is observable.

func TestShowTimelineRouteIsRegisteredOnce(t *testing.T) {
	routes := chiRoutes(t, newTestRouter(t))

	got := matching(routes, http.MethodGet, "/shows/{}/timeline")
	if len(got) != 1 {
		t.Fatalf("GET /shows/{}/timeline: %d registered routes %v, want exactly 1 — chi keeps "+
			"only the LAST registration of a shape, so a second one would silently win", len(got), got)
	}
	if got[0] != "/shows/{show_id}/timeline" {
		t.Errorf("timeline route resolved to %q, want %q — every /shows/{show_id}/… route "+
			"uses the same parameter name", got[0], "/shows/{show_id}/timeline")
	}
}

// The route must be registered AND anonymous. With a nil-DB test container the
// service fails on its own database check, so a correctly wired public route
// answers exactly 500 here.
//
// Asserting the concrete status rather than "not 404/405": a negative match
// stays green if the operation is moved onto rc.Protected (401) or grows a
// required parameter (422), which is precisely the regression that would make a
// reachable route useless to the anonymous callers it exists for.
func TestShowTimelinePathResolvesAnonymously(t *testing.T) {
	router := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/shows/some-show/timeline", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("GET /shows/some-show/timeline = %d, want %d — 404/405 means the route is "+
			"not registered, and any 4xx means it is no longer reachable anonymously",
			w.Code, http.StatusInternalServerError)
	}
}

// Public, anonymous, and enumerable by show id, so it must stay on the ordinary
// public-read budget rather than any exemption lane.
func TestShowTimelineIsNotExemptFromRateLimiting(t *testing.T) {
	path := "/shows/desert-doom-night/timeline"

	if token := personalFeedTokenFromPath(path); token != "" {
		t.Errorf("timeline path %q reads as personal-feed token %q: a public unauthenticated endpoint must stay metered",
			path, token)
	}
	for _, exact := range infraPathsExemptFromRateLimit {
		if path == exact {
			t.Errorf("timeline path %q is exempt from rate limiting", path)
		}
	}
}
