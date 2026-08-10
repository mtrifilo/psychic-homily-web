package routes

import (
	"net/http"
	"testing"
)

// PSY-1781. The named new-bands module is a new SUB-ROUTE under the scene's own
// path, so this pins what a handler test cannot see: chi routes on SHAPE, and a
// sibling registered as `/scenes/{scene_slug}/…` would share the prefix node
// under whichever spelling landed first (the reporting artifact behind
// PSY-1584, and the collision class behind the artist-report bug).
func TestSceneNewArtistsRouteIsRegisteredOnceUnderSlug(t *testing.T) {
	routes := chiRoutes(t, newTestRouter(t))

	got := matching(routes, http.MethodGet, "/scenes/{}/new-artists")
	if len(got) != 1 {
		t.Fatalf("GET /scenes/{}/new-artists registered %d times, want exactly 1: %v", len(got), got)
	}
	if got[0] != "/scenes/{slug}/new-artists" {
		t.Errorf("registered as %q, want %q — the scene sub-routes must agree on the parameter name",
			got[0], "/scenes/{slug}/new-artists")
	}

	// It must not shadow, or be shadowed by, the scene detail route or the
	// existing data sub-resources.
	for _, shape := range []string{"/scenes/{}", "/scenes/{}/artists", "/scenes/{}/shows"} {
		if len(matching(routes, http.MethodGet, shape)) == 0 {
			t.Errorf("GET %s disappeared from the tree", shape)
		}
	}
}
