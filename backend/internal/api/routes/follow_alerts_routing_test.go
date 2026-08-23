package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// PSY-1893. The alert subscription hangs off the generic wildcard follow shape
// (/{entity_type}/{entity_id}/follow/alerts) while /artists/... and
// /venues/... also own static subtrees in the same tree. Handler tests call
// handlers directly and never see a routing tree, so only a router-level test
// can show that these paths are reachable at all — and that no second
// registration of the same shape silently replaced them (see
// pattern: chi keys its tree on shape, last registration wins).

func TestFollowAlertRoutesAreRegisteredOnce(t *testing.T) {
	routes := chiRoutes(t, newTestRouter(t))

	for _, want := range []struct{ method, shape, pattern string }{
		{http.MethodGet, "/{}/{}/follow/alerts", "/{entity_type}/{entity_id}/follow/alerts"},
		{http.MethodPatch, "/{}/{}/follow/alerts", "/{entity_type}/{entity_id}/follow/alerts"},
	} {
		got := matching(routes, want.method, want.shape)
		if len(got) != 1 {
			t.Errorf("%s %s: %d registered routes %v, want exactly 1",
				want.method, want.shape, len(got), got)
			continue
		}
		if got[0] != want.pattern {
			t.Errorf("%s %s resolved to %q, want %q", want.method, want.shape, got[0], want.pattern)
		}
	}
}

// A path chi never matched 404s before any Huma middleware runs, so a 401 from
// the auth gate is proof the wildcard route wins for entity types that also own
// a static subtree.
func TestFollowAlertPathsResolveForStaticSubtreeEntities(t *testing.T) {
	router := newTestRouter(t)

	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/artists/1/follow/alerts"},
		{http.MethodPatch, "/artists/1/follow/alerts"},
		{http.MethodGet, "/venues/1/follow/alerts"},
		{http.MethodPatch, "/venues/1/follow/alerts"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s = %d, want 401 (route matched, auth gate rejected); body: %s",
				tc.method, tc.path, w.Code, w.Body.String())
		}
	}
}
