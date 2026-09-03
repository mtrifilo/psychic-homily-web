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

// The route must be registered AND anonymous. With a nil-DB test container the
// service fails on its own database check, so a correctly wired public route
// answers exactly 500 here.
//
// Asserting the concrete status rather than "not 404/405": a negative match
// stays green if the operation is moved onto rc.Protected (401) or grows a
// required parameter (422), which is precisely the regression that would make a
// reachable route useless to the anonymous callers it exists for.
func TestShowAlsoTonightPathResolvesAnonymously(t *testing.T) {
	router := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/shows/some-show/also-tonight", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("GET /shows/some-show/also-tonight = %d, want %d — 404/405 means the route is "+
			"not registered, and any 4xx means it is no longer reachable anonymously",
			w.Code, http.StatusInternalServerError)
	}
}

// Public, anonymous, and enumerable by show id, so it must stay on the ordinary
// public-read budget rather than any exemption lane.
func TestShowAlsoTonightIsNotExemptFromRateLimiting(t *testing.T) {
	path := "/shows/desert-doom-night/also-tonight"

	if token := personalFeedTokenFromPath(path); token != "" {
		t.Errorf("also-tonight path %q reads as personal-feed token %q: a public unauthenticated endpoint must stay metered",
			path, token)
	}
	for _, exact := range infraPathsExemptFromRateLimit {
		if path == exact {
			t.Errorf("also-tonight path %q is exempt from rate limiting", path)
		}
	}
}
