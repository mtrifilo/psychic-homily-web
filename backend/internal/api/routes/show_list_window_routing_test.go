package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The date-addressed show list and its month strip are STATIC siblings of
// /shows/{show_id}. chi resolves a static segment before a parameter node, so
// these two paths must reach their own handlers rather than the show detail
// route, and the show detail route must still answer everything else.
//
// These tests speak to the BUILT ROUTER, which is the only place either property
// is observable: handler tests call handlers directly and bypass chi entirely.

func TestShowListWindowRoutesAreRegisteredOnceAsStaticPaths(t *testing.T) {
	routes := chiRoutes(t, newTestRouter(t))

	for _, path := range []string{"/shows/calendar", "/shows/months", "/shows/calendar/range"} {
		got := matching(routes, http.MethodGet, path)
		if len(got) != 1 {
			t.Fatalf("GET %s: %d registered routes %v, want exactly 1. chi keeps only the "+
				"LAST registration of a shape, so a second one would silently win", path, len(got), got)
		}
		if got[0] != path {
			t.Errorf("%s resolved to %q, want the static path", path, got[0])
		}
	}
}

// Registered AND anonymous. With a nil-DB test router the service fails its own
// database check, so a correctly wired public route answers exactly 500 here.
//
// Asserting the concrete status rather than "not 404/405": a negative match
// stays green if the operation is moved onto rc.Protected (401) or grows a
// required parameter (422), which is precisely the regression that would make a
// reachable route useless to the anonymous callers it exists for.
func TestShowListWindowPathsResolveAnonymously(t *testing.T) {
	for _, path := range []string{
		"/shows/calendar",
		"/shows/calendar?year=2026&month=11",
		"/shows/calendar?year=2026&month=11&day=14&offset=50",
		"/shows/calendar?year=2026&month=11&day=14&days=7",
		"/shows/months",
		"/shows/months?cities=Phoenix,AZ",
		// The proxy reads this one anonymously on behalf of every visitor, and a
		// 404 or a 405 here is indistinguishable from a backend that does not
		// carry the route yet, which is the deploy-skew case it fails open on.
		"/shows/calendar/range",
	} {
		router := newTestRouter(t)
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("GET %s = %d, want %d. 404/405 means the route is not registered, and "+
				"any 4xx means it is no longer reachable anonymously", path, w.Code, http.StatusInternalServerError)
		}
	}
}

// A half-stated or impossible window is refused before it reaches the service,
// because the service cannot tell one from "no window" and would answer with the
// whole upcoming catalog under a URL promising one day of it.
//
// Two guards produce these 422s and this test does not distinguish them: the
// request schema's own `maximum` bounds reject an out-of-range month or day
// first, and ShowCalendarWindow.Validate rejects the combinations a schema
// cannot express. What this pins is that the STATUS is the same either way, so a
// client sees one answer for one class of mistake. Validate's own arms are
// pinned directly by TestGetUpcomingShowsPage_RefusesMalformedWindows in
// services/catalog.
func TestShowListWindowRefusesIncoherentWindows(t *testing.T) {
	for _, path := range []string{
		// Refused by ShowCalendarWindow.Validate; no schema bound can express these.
		"/shows/calendar?day=14",
		"/shows/calendar?month=11&day=14",
		"/shows/calendar?month=11",
		"/shows/calendar?year=2026",
		"/shows/calendar?year=2027&month=2&day=31",
		"/shows/calendar?days=3",
		"/shows/calendar?year=2026&month=11&days=3",
		// Out of range. Two guards produce these, the request schema's bounds and
		// Validate; this pins only that the STATUS is one answer either way.
		"/shows/calendar?year=2026&month=13",
		"/shows/calendar?year=2026&month=11&day=32",
		"/shows/calendar?year=2026&month=11&day=14&days=15",
	} {
		router := newTestRouter(t)
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusUnprocessableEntity {
			t.Errorf("GET %s = %d, want %d", path, w.Code, http.StatusUnprocessableEntity)
		}
	}
}

// Public, anonymous and unauthenticated, so both stay on the ordinary
// public-read budget rather than any exemption lane.
func TestShowListWindowRoutesAreNotExemptFromRateLimiting(t *testing.T) {
	for _, path := range []string{"/shows/calendar", "/shows/months", "/shows/calendar/range"} {
		if token := personalFeedTokenFromPath(path); token != "" {
			t.Errorf("path %q reads as personal-feed token %q: a public unauthenticated endpoint must stay metered",
				path, token)
		}
		for _, exempt := range infraPathsExemptFromRateLimit {
			if path == exempt {
				t.Errorf("path %q is listed as infra-exempt", path)
			}
		}
	}
}
