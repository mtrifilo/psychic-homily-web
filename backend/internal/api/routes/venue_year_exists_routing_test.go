package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// PSY-1770. `HEAD /venues/{venue_id}/shows/{year}/exists` is the frontend
// proxy's year-archive probe, and it is registered as a PARAMETER sibling of the
// static `/venues/{venue_id}/shows/years`. Everything worth pinning about it is
// router-level, which is exactly what a handler test cannot see.
//
// It matters more than the usual registration check because the caller fails
// CLOSED on a missing route: chi 404s an unregistered path, the proxy cannot
// tell that from "this year has no shows", and every venue year archive on the
// site would 404 rather than degrade.

// The static sibling must keep winning. chi walks static children before
// parameterised ones, so `/shows/years` is the histogram and not year "years" —
// but that is a property of the built tree, not of the registration order these
// two happen to have today.
func TestVenueShowYearsStillResolvesToTheHistogram(t *testing.T) {
	router := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/venues/some-venue/shows/years", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code == http.StatusNotFound || w.Code == http.StatusMethodNotAllowed {
		t.Errorf("GET /venues/some-venue/shows/years = %d — the histogram must not be "+
			"shadowed by the {year} sibling", w.Code)
	}
}

// The probe itself resolves. Anything other than a 404/405 proves the route
// matched: with a nil-DB test container the service errors out below it.
func TestVenueYearArchiveExistsPathResolves(t *testing.T) {
	router := newTestRouter(t)

	req := httptest.NewRequest(http.MethodHead, "/venues/some-venue/shows/2024/exists", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code == http.StatusNotFound || w.Code == http.StatusMethodNotAllowed {
		t.Errorf("HEAD /venues/some-venue/shows/2024/exists = %d — the route is not registered, "+
			"which would 404 every venue year archive through the proxy", w.Code)
	}
}

// HEAD-only, deliberately: the status is the whole answer, so a GET would
// advertise a body that does not exist. Asserted because the reverse mistake
// (registering GET and forgetting HEAD) is the one the scene branches hit, and
// this pins which way round this endpoint is.
func TestVenueYearArchiveExistsIsHeadOnly(t *testing.T) {
	router := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/venues/some-venue/shows/2024/exists", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound && w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /venues/some-venue/shows/2024/exists = %d, want 404/405 — the probe is "+
			"HEAD-only", w.Code)
	}
}

// The parameter name at position 2 has to match every other venue sub-route, or
// chi reports the shared prefix node under whichever spelling registered first
// (the reporting artifact behind PSY-1584).
func TestVenueYearArchiveExistsSharesTheVenueParameterName(t *testing.T) {
	routes := chiRoutes(t, newTestRouter(t))

	got := matching(routes, http.MethodHead, "/venues/{}/shows/{}/exists")
	if len(got) == 0 {
		t.Fatal("HEAD /venues/{}/shows/{}/exists is not registered")
	}
	for _, pattern := range got {
		if pattern != "/venues/{venue_id}/shows/{year}/exists" {
			t.Errorf("registered as %q, want %q — every /venues/{venue_id}/… route must agree "+
				"on the parameter name", pattern, "/venues/{venue_id}/shows/{year}/exists")
		}
	}
}
