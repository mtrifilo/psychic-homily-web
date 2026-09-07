package routes

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/services"
	"psychic-homily-backend/internal/testutil"
)

// PSY-1884. The parameter-name half of the chi shape rule is held for /crews by
// TestSceneSubRoutesShareOneParameterName, which this route is listed in. What
// is left is what that loop cannot see: the route exists exactly once, and it
// carries no write method.
func TestSceneCrewsRouteIsPublicReadOnly(t *testing.T) {
	routes := chiRoutes(t, newTestRouter(t))

	if got := matching(routes, http.MethodGet, "/scenes/{}/crews"); len(got) != 1 {
		t.Fatalf("GET /scenes/{}/crews registered %d times, want exactly 1: %v", len(got), got)
	}

	// No auth group is attached to the read, so no authenticated method may
	// appear on this shape.
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		if extra := matching(routes, method, "/scenes/{}/crews"); len(extra) != 0 {
			t.Errorf("%s /scenes/{}/crews registered: %v", method, extra)
		}
	}
}

// Crew tags are admin-mint. TagService guards the inline-create path a
// contributor reaches; the OTHER two paths that can set a tag's category are
// kept out of reach by their route registration alone, with no service-side
// check behind them. If either slid onto the Protected group, a signed-in
// contributor could mint a crew tag through CreateTag or re-categorize any tag
// into crew through UpdateTag.
//
// The caller is AUTHENTICATED and non-admin, which is the whole point: an
// anonymous probe answers 401 on the Protected group too, so it would read as
// "the gate works" wherever these routes were registered.
func TestTagCategoryWritePathsRejectNonAdmin(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	cfg := testConfig()
	sc := services.NewServiceContainer(td.DB, cfg)
	router := chi.NewRouter()
	SetupRoutes(router, sc, cfg)

	contributor := testhelpers.CreateUserWithTier(td.DB, "trusted_contributor")
	token := mintToken(t, sc, contributor)

	for _, tc := range []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPost, "/tags", `{"name":"probe crew","category":"crew"}`},
		{http.MethodPut, "/tags/1", `{"category":"crew"}`},
	} {
		t.Run(fmt.Sprintf("%s %s", tc.method, tc.path), func(t *testing.T) {
			code, body := doRequest(t, router, tc.method, tc.path, token, []byte(tc.body))
			if code != http.StatusForbidden {
				t.Errorf("answered %d for a trusted_contributor, want 403 — the admin group is "+
					"the only thing keeping crew minting off this path; body: %s", code, body)
			}
		})
	}
}
