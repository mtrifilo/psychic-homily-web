package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The "also tonight" rail is a /shows/{show_id}/… sub-route registered from a
// DIFFERENT handler than its siblings (the scene handler answers it), which is
// exactly the setup that produces a chi shape collision: chi keys its tree on
// pattern shape, not on parameter name, so a registration naming the segment
// anything but `show_id` would not add a route — it would silently rename the
// parameter on whichever registration lost, and no handler-level test could see
// it. These tests speak to the BUILT ROUTER.

func TestShowAlsoTonightRouteIsRegisteredOnce(t *testing.T) {
	routes := chiRoutes(t, newTestRouter(t))

	got := matching(routes, http.MethodGet, "/shows/{}/also-tonight")
	if len(got) != 1 {
		t.Fatalf("GET /shows/{}/also-tonight: %d registered routes %v, want exactly 1", len(got), got)
	}
	if got[0] != "/shows/{show_id}/also-tonight" {
		t.Errorf("also-tonight route resolved to %q, want %q — the parameter must match its "+
			"/shows/{show_id}/… siblings or chi silently renames one of them",
			got[0], "/shows/{show_id}/also-tonight")
	}
}

// An unregistered path 404s in chi before any handler runs, so anything other
// than 404/405 proves the route matched — with a nil-DB test container the
// service errors out, which is a 500.
func TestShowAlsoTonightPathResolves(t *testing.T) {
	router := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/shows/some-show/also-tonight", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code == http.StatusNotFound || w.Code == http.StatusMethodNotAllowed {
		t.Errorf("GET /shows/some-show/also-tonight = %d — the route is not registered", w.Code)
	}
}

// Public, anonymous, and enumerable by show id, so it must stay on the ordinary
// public-read budget rather than any exemption lane.
func TestShowAlsoTonightIsNotExemptFromRateLimiting(t *testing.T) {
	path := "/shows/desert-doom-night/also-tonight"

	for _, prefix := range personalFeedPathPrefixesExemptFromRateLimit {
		if len(path) >= len(prefix) && path[:len(prefix)] == prefix {
			t.Errorf("also-tonight path %q matches rate-limit exemption prefix %q — a public "+
				"unauthenticated endpoint must stay metered", path, prefix)
		}
	}
	for _, exact := range infraPathsExemptFromRateLimit {
		if path == exact {
			t.Errorf("also-tonight path %q is exempt from rate limiting", path)
		}
	}
}
