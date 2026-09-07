package routes

import (
	"net/http"
	"testing"
)

// PSY-1884. The crews chip row is a new SUB-ROUTE under the scene's own path,
// so this pins what a handler test cannot see: chi routes on SHAPE, and a
// sibling registered as `/scenes/{scene_slug}/…` would share the prefix node
// under whichever spelling landed first rather than erroring.
func TestSceneCrewsRouteIsRegisteredOnceUnderSlug(t *testing.T) {
	routes := chiRoutes(t, newTestRouter(t))

	got := matching(routes, http.MethodGet, "/scenes/{}/crews")
	if len(got) != 1 {
		t.Fatalf("GET /scenes/{}/crews registered %d times, want exactly 1: %v", len(got), got)
	}
	if got[0] != "/scenes/{slug}/crews" {
		t.Errorf("registered as %q, want %q — the scene sub-routes must agree on the parameter name",
			got[0], "/scenes/{slug}/crews")
	}

	// It must not shadow, or be shadowed by, the scene detail route or the
	// sibling data sub-resources on the same page.
	for _, shape := range []string{"/scenes/{}", "/scenes/{}/gaps", "/scenes/{}/collections", "/scenes/{}/new-artists"} {
		if len(matching(routes, http.MethodGet, shape)) == 0 {
			t.Errorf("GET %s disappeared from the tree", shape)
		}
	}

	// Public read: no auth group is attached, so no authenticated method may
	// appear on this shape.
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		if extra := matching(routes, method, "/scenes/{}/crews"); len(extra) != 0 {
			t.Errorf("%s /scenes/{}/crews registered: %v", method, extra)
		}
	}
}
