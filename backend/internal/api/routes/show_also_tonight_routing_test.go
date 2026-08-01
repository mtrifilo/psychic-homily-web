package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The "also tonight" rail is a /shows/{show_id}/… sub-route registered from a
// DIFFERENT handler than its siblings (the scene handler answers it), and from a
// different route file, so nothing local to either file would notice a second
// claimant on the same path. chi resolves a shape+method collision by silently
// keeping the LAST registration rather than erroring, which is the failure that
// left the artist report handler unreachable (see reports_routing_test.go).
//
// Parameter naming is a readability convention rather than a correctness one:
// chi stores parameter names per endpoint pattern, so /shows/{entity_id}/…
// coexists fine. The name is still asserted, because the group's shared-name
// test is only meaningful while every member keeps it.
//
// These tests speak to the BUILT ROUTER, which is the only place either
// property is observable.

func TestShowAlsoTonightRouteIsRegisteredOnce(t *testing.T) {
	routes := chiRoutes(t, newTestRouter(t))

	got := matching(routes, http.MethodGet, "/shows/{}/also-tonight")
	if len(got) != 1 {
		t.Fatalf("GET /shows/{}/also-tonight: %d registered routes %v, want exactly 1 — chi keeps "+
			"only the LAST registration of a shape, so a second one would silently win", len(got), got)
	}
	if got[0] != "/shows/{show_id}/also-tonight" {
		t.Errorf("also-tonight route resolved to %q, want %q — every /shows/{show_id}/… route "+
			"uses the same parameter name", got[0], "/shows/{show_id}/also-tonight")
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
