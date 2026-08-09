package routes

import (
	"net/http"
	"testing"
)

// PSY-1751. The artist show-year histogram is registered UNDER the show list's
// own path (`/artists/{artist_id}/shows/years`), which is the shape chi resolves
// by pattern rather than by parameter name. A registration that spelled the
// parameter differently would not create a second route: it would silently
// rename the parameter on whichever registration lost, and no handler-level test
// could observe it, because handler tests call the handler function directly.
//
// It also pins that the deeper path is registered at all. A missing registration
// does not fail loudly here either — `/artists/{artist_id}` would answer for
// `/artists/shows` shaped requests and the picker would just come back empty.
func TestArtistSubRoutesShareOneParameterName(t *testing.T) {
	routes := chiRoutes(t, newTestRouter(t))

	for _, shape := range []string{
		"/artists/{}",
		"/artists/{}/shows",
		"/artists/{}/shows/years",
		"/artists/{}/labels",
		"/artists/{}/aliases",
	} {
		got := matching(routes, http.MethodGet, shape)
		if len(got) == 0 {
			t.Errorf("GET %s is not registered", shape)
			continue
		}
		want := "/artists/{artist_id}" + shape[len("/artists/{}"):]
		for _, pattern := range got {
			if pattern != want {
				t.Errorf("GET %s registered as %q, want %q — every /artists/{artist_id}/… route "+
					"must use the same parameter name", shape, pattern, want)
			}
		}
	}
}
