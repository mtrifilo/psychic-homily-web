package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The radio and collection existence probes are the frontend proxy's answer to
// whether a radio or collection page names anything. What is worth pinning about
// them is router-level, which a handler test cannot see: that each is
// registered for HEAD, that none shadows a GET sibling under the same prefix,
// that the parameter names agree with their siblings', and that an unrouted
// path beside them does not answer the way the API answers a missing entity.

// "Resolves" is asserted as "not 404 and not 405": with a nil-DB test container
// everything past the router errors out, so only the router is under test.
func TestFamilyExistsRouteResolution(t *testing.T) {
	router := newTestRouter(t)

	for _, tc := range []struct {
		method   string
		path     string
		resolves bool
		why      string
	}{
		{http.MethodHead, "/radio-stations/kexp/exists", true, "the station probe is registered"},
		{http.MethodHead, "/radio-stations/kexp/radio-shows/the-morning-show/exists", true, "the show probe is registered"},
		{http.MethodHead, "/radio-stations/kexp/radio-shows/the-morning-show/episodes/2025-01-15/exists", true, "the episode probe is registered"},
		{http.MethodHead, "/collections/my-list/exists", true, "the collection probe is registered"},
		{http.MethodGet, "/radio-stations/kexp/exists", false, "the status is the whole answer, so no GET"},
		{http.MethodGet, "/collections/my-list/exists", false, "the status is the whole answer, so no GET"},
		{http.MethodGet, "/radio-stations/kexp", true, "the station detail is not shadowed"},
		{http.MethodGet, "/radio-stations/kexp/episodes", true, "a static station sibling is not shadowed"},
		{http.MethodGet, "/collections/my-list", true, "the collection detail is not shadowed"},
		{http.MethodGet, "/collections/my-list/stats", true, "a static collection sibling is not shadowed"},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			unrouted := w.Code == http.StatusNotFound || w.Code == http.StatusMethodNotAllowed
			if tc.resolves && unrouted {
				t.Errorf("%s %s = %d, want it to resolve: %s", tc.method, tc.path, w.Code, tc.why)
			}
			if !tc.resolves && !unrouted {
				t.Errorf("%s %s = %d, want 404/405: %s", tc.method, tc.path, w.Code, tc.why)
			}
		})
	}
}

// An unrouted path beside each probe answers 404 WITHOUT the API's error
// content type. The proxy probes these routes with that guard on, so a route
// the running API does not carry reads as "no answer" and the page renders,
// where an API-authored 404 would read as "missing" and 404 the page.
func TestUnroutedFamilyExistsSiblingsDoNotLookLikeApiErrors(t *testing.T) {
	router := newTestRouter(t)

	for _, path := range []string{
		"/radio-stations/kexp/no-such-leaf",
		"/radio-stations/kexp/radio-shows/the-morning-show/no-such-leaf",
		"/radio-stations/kexp/radio-shows/the-morning-show/episodes/2025-01-15/no-such-leaf",
		"/collections/my-list/no-such-leaf",
	} {
		req := httptest.NewRequest(http.MethodHead, path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("HEAD %s = %d, want 404: this test's premise is gone", path, w.Code)
			continue
		}
		if got := w.Header().Get("Content-Type"); strings.Contains(got, "application/problem+json") {
			t.Errorf("HEAD %s answers 404 with Content-Type %q, which the proxy reads as an "+
				"entity the API says is missing", path, got)
		}
	}
}

// The other half: an error each probe authors carries the API's error content
// type, so the proxy's guard has something to match on. With the nil-DB test
// container each probe answers the service's error, written by the same huma
// writer as its 404; whatever would strip the header from one strips it from
// both.
func TestFamilyExistsProbeErrorsCarryTheApiErrorContentType(t *testing.T) {
	router := newTestRouter(t)

	for _, path := range []string{
		"/radio-stations/kexp/exists",
		"/radio-stations/kexp/radio-shows/the-morning-show/exists",
		"/radio-stations/kexp/radio-shows/the-morning-show/episodes/2025-01-15/exists",
		"/collections/my-list/exists",
	} {
		req := httptest.NewRequest(http.MethodHead, path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code < 400 || w.Code == http.StatusNotFound || w.Code == http.StatusMethodNotAllowed {
			t.Errorf("HEAD %s = %d, want the probe's own error: this test's premise is gone", path, w.Code)
			continue
		}
		if got := w.Header().Get("Content-Type"); !strings.Contains(got, "application/problem+json") {
			t.Errorf("HEAD %s answers %d with Content-Type %q; without application/problem+json "+
				"the proxy reads every missing page behind this probe as no answer", path, w.Code, got)
		}
	}
}

// The station parameter is spelled as on every other /radio-stations/{slug}
// route, so chi reports the shared prefix node under one name.
func TestFamilyExistsRoutesShareTheirSiblingsParameterNames(t *testing.T) {
	routes := chiRoutes(t, newTestRouter(t))

	for shape, want := range map[string]string{
		"/radio-stations/{}/exists":                            "/radio-stations/{slug}/exists",
		"/radio-stations/{}/radio-shows/{}/exists":             "/radio-stations/{slug}/radio-shows/{show_slug}/exists",
		"/radio-stations/{}/radio-shows/{}/episodes/{}/exists": "/radio-stations/{slug}/radio-shows/{show_slug}/episodes/{date}/exists",
		"/collections/{}/exists":                               "/collections/{slug}/exists",
	} {
		got := matching(routes, http.MethodHead, shape)
		if len(got) != 1 || got[0] != want {
			t.Errorf("HEAD %s registered as %v, want [%s]", shape, got, want)
		}
	}
}
