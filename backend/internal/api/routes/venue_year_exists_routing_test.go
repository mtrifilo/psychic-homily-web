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

// Which method/path pairs the built router answers.
//
// "Resolves" is asserted as "not 404 and not 405", because with a nil-DB test
// container everything past the router errors out — the status below that point
// says nothing, and only the router is under test here.
func TestVenueYearArchiveRouteResolution(t *testing.T) {
	router := newTestRouter(t)

	for _, tc := range []struct {
		name     string
		method   string
		path     string
		resolves bool
		why      string
	}{
		{
			name:     "the probe is registered",
			method:   http.MethodHead,
			path:     "/venues/some-venue/shows/2024/exists",
			resolves: true,
			why:      "an unregistered path would 404 every venue year archive through the proxy",
		},
		{
			// The reverse mistake — registering GET and forgetting HEAD — is the
			// one the scene branches hit. This pins which way round this is.
			name:     "the probe is HEAD-only",
			method:   http.MethodGet,
			path:     "/venues/some-venue/shows/2024/exists",
			resolves: false,
			why:      "the status is the whole answer, so a GET would advertise a body that does not exist",
		},
		{
			// chi walks static children before parameterised ones, so `/shows/years`
			// stays the histogram rather than year "years" — but that is a property
			// of the built tree, not of the registration order these two happen to
			// have today.
			name:     "the static years sibling is not shadowed by {year}",
			method:   http.MethodGet,
			path:     "/venues/some-venue/shows/years",
			resolves: true,
			why:      "the histogram must not be shadowed by the {year} sibling",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			unrouted := w.Code == http.StatusNotFound || w.Code == http.StatusMethodNotAllowed
			if tc.resolves && unrouted {
				t.Errorf("%s %s = %d, want it to resolve — %s", tc.method, tc.path, w.Code, tc.why)
			}
			if !tc.resolves && !unrouted {
				t.Errorf("%s %s = %d, want 404/405 — %s", tc.method, tc.path, w.Code, tc.why)
			}
		})
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
