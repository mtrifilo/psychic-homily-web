package routes

import (
	"net/http"
	"testing"
)

// PSY-1751. The artist show-year histogram is registered UNDER the show list's
// own path (`/artists/{artist_id}/shows/years`), so this pins two things a
// handler-level test cannot see, because handler tests call the handler function
// directly rather than going through the router.
//
// 1. The deeper path is registered at all. chi 404s an unregistered path before
//    any handler runs, so a dropped registration surfaces as a 404 on the year
//    picker's fetch.
// 2. The routes listed below agree on the parameter NAME. chi v5 stores param
//    keys per leaf, so a sibling spelled `/artists/{slug}/…` does resolve
//    correctly at request time — this is not a correctness guard, it is a
//    consistency one. What it protects is `chi.Walk` and anything built on the
//    reported RoutePattern, which surfaces a shared prefix node under whichever
//    spelling registered first (the reporting artifact behind PSY-1584).
//
// The list is deliberately NOT every artist sub-route. `/artists/{slug}/radio-plays`
// (routes/radio.go) already uses a different name, so an exhaustive sweep would
// fail on a pre-existing divergence this ticket has no mandate to settle. These
// are the reads that share the {artist_id} contract today.
func TestArtistShowsSubRoutesShareOneParameterName(t *testing.T) {
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
				t.Errorf("GET %s registered as %q, want %q — these /artists/{artist_id}/… routes "+
					"must agree on the parameter name", shape, pattern, want)
			}
		}
	}
}
