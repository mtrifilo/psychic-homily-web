package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services"
	"psychic-homily-backend/internal/testutil"

	"github.com/go-chi/chi/v5"
)

// Venue merge deletes shows and a venue row, so two things about its routes are
// worth pinning at the ROUTER level rather than the handler level:
//
//  1. chi keys its routing tree on path SHAPE, not parameter name. Two
//     registrations of the same shape are one route, last-wins, silently — and
//     a handler test cannot see it because it calls the handler directly.
//  2. The endpoint must sit behind rc.Admin. A handler test proves nothing
//     about which middleware chain the route was registered on.

// TestAdminVenueMergeRoutesAreRegisteredOnce walks the built tree and pins the
// merge paths. They use static segments precisely so they cannot collide with
// /admin/venues/{venue_id}/verify, and this test fails if that ever changes.
func TestAdminVenueMergeRoutesAreRegisteredOnce(t *testing.T) {
	routes := chiRoutes(t, newTestRouter(t))

	for _, want := range []string{
		"/admin/venues/merge",
		"/admin/venues/merge/preview",
	} {
		var n int
		for _, r := range routes[http.MethodPost] {
			if r == want {
				n++
			}
		}
		if n != 1 {
			t.Errorf("POST %s registered %d times, want exactly 1 — a second registration "+
				"of this shape would be silently dropped by chi; all POST routes: %v",
				want, n, routes[http.MethodPost])
		}
	}

	// The merge paths must not have been swallowed by the {venue_id} shape.
	// /admin/venues/merge/preview is 4 segments and {venue_id}/verify is 4 too,
	// so a careless refactor to /admin/venues/{loser_id}/merge would land on
	// the SAME shape as {venue_id}/verify. Assert the surviving parameter name.
	for _, r := range matching(routes, http.MethodPost, "/admin/venues/{}/verify") {
		if r != "/admin/venues/{venue_id}/verify" {
			t.Errorf("POST /admin/venues/{}/verify resolved to %q, want the venue_id "+
				"registration — something else claimed the shape", r)
		}
	}
}

// TestAdminVenueMergeRoutesRequireAdmin proves the routes are on rc.Admin, not
// rc.Protected. An unregistered path 404s in chi before any middleware runs, so
// a 401 here is simultaneously proof that the route matched AND that the auth
// gate stands in front of it.
func TestAdminVenueMergeRoutesRequireAdmin(t *testing.T) {
	router := newTestRouter(t)

	for _, path := range []string{"/admin/venues/merge", "/admin/venues/merge/preview"} {
		req := httptest.NewRequest(http.MethodPost, path,
			strings.NewReader(`{"canonical_venue_id":1,"merge_from_venue_id":2}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("POST %s unauthenticated = %d, want 401; body: %s", path, w.Code, w.Body.String())
		}
	}
}

// TestVenueMergeRejectsNonAdmin is the other half of the gate: a valid,
// authenticated, NON-admin token must be refused. Without this, a route
// accidentally registered on rc.Protected would still pass the 401 test above.
func TestVenueMergeRejectsNonAdmin(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	cfg := testConfig()
	sc := services.NewServiceContainer(td.DB, cfg)
	router := chi.NewRouter()
	SetupRoutes(router, sc, cfg)

	regular := &authm.User{
		Email:         strPtr("regular@psy-1597.test"),
		IsActive:      true,
		EmailVerified: true,
	}
	if err := td.DB.Create(regular).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	token, err := sc.JWT.CreateToken(regular)
	if err != nil {
		t.Fatalf("mint token: %v", err)
	}

	for _, path := range []string{"/admin/venues/merge", "/admin/venues/merge/preview"} {
		req := httptest.NewRequest(http.MethodPost, path,
			strings.NewReader(`{"canonical_venue_id":1,"merge_from_venue_id":2}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Errorf("POST %s as a non-admin = %d, want 403 — a destructive merge must be "+
				"admin-gated; body: %s", path, w.Code, w.Body.String())
		}
	}
}
