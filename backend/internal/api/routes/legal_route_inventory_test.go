package routes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	systemh "psychic-homily-backend/internal/api/handlers/system"
)

// legalRouteDispositions disposes of every route the router serves under
// /legal/, keyed "METHOD /path". The sweep compares keys against the built
// router, so a route added under /legal/ fails until its author writes down who
// may call it and what meters it. Like the /auth/ sweep, the value is a
// sentence, not a reading of the middleware chain; what pins this row's claim
// is TestLegalVersionsIsPublicThroughRouter.
var legalRouteDispositions = map[string]string{
	"GET " + systemh.LegalVersionsRoute: "rc.API, no session required, identical for every caller; " +
		"metered only by the global public-read limiter when ENABLE_PUBLIC_READ_RATE_LIMITS is on",
}

func TestEveryLegalRouteHasADisposition(t *testing.T) {
	served := map[string]bool{}
	for method, patterns := range chiRoutes(t, newTestRouter(t)) {
		for _, pattern := range patterns {
			if strings.HasPrefix(pattern, "/legal/") {
				served[method+" "+pattern] = true
			}
		}
	}

	for route := range served {
		if _, ok := legalRouteDispositions[route]; !ok {
			t.Errorf("router serves %q but legalRouteDispositions does not dispose of it", route)
		}
	}
	for route := range legalRouteDispositions {
		if !served[route] {
			t.Errorf("legalRouteDispositions names %q but the router does not serve it", route)
		}
	}
}

// The versions are what a signed-out visitor needs before an account exists,
// so the route must answer with no session through the full middleware stack,
// and appear in the published spec the frontend's types are generated from.
func TestLegalVersionsIsPublicThroughRouter(t *testing.T) {
	router := newTestRouter(t)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, systemh.LegalVersionsRoute, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("GET %s with no session = %d, want 200: %s", systemh.LegalVersionsRoute, w.Code, w.Body.String())
	}
	if got := w.Header().Get("Cache-Control"); got != "public, max-age=300" {
		t.Errorf("Cache-Control = %q, want %q", got, "public, max-age=300")
	}
	if got := w.Header().Get("Set-Cookie"); got != "" {
		t.Errorf("a public cacheable response sets a cookie: %q", got)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	for _, field := range []string{"terms_version", "privacy_version", "min_signup_age", "grace_until"} {
		if _, ok := body[field]; !ok {
			t.Errorf("response is missing %q: %s", field, w.Body.String())
		}
	}

	operation, ok := servedSpecPaths(t, router)[systemh.LegalVersionsRoute]["get"].(map[string]any)
	if !ok {
		t.Fatalf("served spec has no GET %s", systemh.LegalVersionsRoute)
	}
	if security, present := operation["security"]; present {
		t.Errorf("GET %s declares security %v; it must be callable signed out", systemh.LegalVersionsRoute, security)
	}
}
