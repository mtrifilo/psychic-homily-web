package routes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// TestAdminRadioArtistCandidatesRoute pins GET /admin/radio/candidates/artists
// at the router: it sits behind rc.Admin, and huma enforces the lane and sort
// enums and the limit bounds before the handler runs.
func TestAdminRadioArtistCandidatesRoute(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	cfg := testConfig()
	sc := services.NewServiceContainer(td.DB, cfg)
	router := chi.NewRouter()
	SetupRoutes(router, sc, cfg)

	mint := func(u *authm.User) string {
		if u.ID == 0 {
			t.Fatal("seed user was not created")
		}
		token, err := sc.JWT.CreateToken(u)
		if err != nil {
			t.Fatalf("mint token: %v", err)
		}
		return token
	}
	regular := mint(testhelpers.CreateTestUser(td.DB))
	admin := mint(testhelpers.CreateAdminUser(td.DB))

	get := func(path, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	const path = "/admin/radio/candidates/artists"
	cases := []struct {
		name, path, token string
		want              int
	}{
		{"unauthenticated", path, "", http.StatusUnauthorized},
		{"non-admin", path, regular, http.StatusForbidden},
		{"admin, defaults", path, admin, http.StatusOK},
		{"admin, every param", path + "?lane=auto&sort=stations&limit=500", admin, http.StatusOK},
		{"admin, human lane by plays", path + "?lane=human&sort=plays&limit=1", admin, http.StatusOK},
		{"unknown lane", path + "?lane=bogus", admin, http.StatusUnprocessableEntity},
		{"unknown sort", path + "?sort=bogus", admin, http.StatusUnprocessableEntity},
		{"limit below range", path + "?limit=0", admin, http.StatusUnprocessableEntity},
		{"limit above range", path + "?limit=501", admin, http.StatusUnprocessableEntity},
	}
	for _, c := range cases {
		w := get(c.path, c.token)
		if w.Code != c.want {
			t.Errorf("%s: GET %s = %d, want %d; body: %s", c.name, c.path, w.Code, c.want, w.Body.String())
		}
	}

	w := get(path, admin)
	var listing contracts.RadioArtistCandidateListing
	if err := json.Unmarshal(w.Body.Bytes(), &listing); err != nil {
		t.Fatalf("decode listing: %v; body: %s", err, w.Body.String())
	}
	if listing.Candidates == nil || len(listing.Candidates) != 0 || listing.Total != 0 {
		t.Errorf("no plays: got %+v, want an empty candidates array and a zero total", listing)
	}
	if listing.WindowDays != 180 || listing.MinStationRows != 2 || listing.MinPlays != 3 || listing.WindowStart == "" {
		t.Errorf("no plays: got window %q/%d and floors %d/%d, want the window and floors reported", listing.WindowStart, listing.WindowDays, listing.MinStationRows, listing.MinPlays)
	}
}

// TestAdminRadioTrackCandidatesRoute pins GET /admin/radio/candidates/tracks
// at the router: it sits behind rc.Admin, and huma enforces the required view,
// the view, lane and sort enums and the limit bounds before the handler runs.
func TestAdminRadioTrackCandidatesRoute(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	cfg := testConfig()
	sc := services.NewServiceContainer(td.DB, cfg)
	router := chi.NewRouter()
	SetupRoutes(router, sc, cfg)

	mint := func(u *authm.User) string {
		if u.ID == 0 {
			t.Fatal("seed user was not created")
		}
		token, err := sc.JWT.CreateToken(u)
		if err != nil {
			t.Fatalf("mint token: %v", err)
		}
		return token
	}
	regular := mint(testhelpers.CreateTestUser(td.DB))
	admin := mint(testhelpers.CreateAdminUser(td.DB))

	get := func(path, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	const path = "/admin/radio/candidates/tracks"
	const unlinked = path + "?view=unlinked"
	cases := []struct {
		name, path, token string
		want              int
	}{
		{"unauthenticated", unlinked, "", http.StatusUnauthorized},
		{"non-admin", unlinked, regular, http.StatusForbidden},
		{"admin, unlinked view", unlinked, admin, http.StatusOK},
		{"admin, every param", path + "?view=unmatched&lane=auto&sort=stations&limit=500", admin, http.StatusOK},
		{"admin, human lane by plays", path + "?view=unlinked&lane=human&sort=plays&limit=1", admin, http.StatusOK},
		{"no view", path, admin, http.StatusUnprocessableEntity},
		{"unknown view", path + "?view=bogus", admin, http.StatusUnprocessableEntity},
		{"unknown lane", unlinked + "&lane=bogus", admin, http.StatusUnprocessableEntity},
		{"unknown sort", unlinked + "&sort=bogus", admin, http.StatusUnprocessableEntity},
		{"limit below range", unlinked + "&limit=0", admin, http.StatusUnprocessableEntity},
		{"limit above range", unlinked + "&limit=501", admin, http.StatusUnprocessableEntity},
	}
	for _, c := range cases {
		w := get(c.path, c.token)
		if w.Code != c.want {
			t.Errorf("%s: GET %s = %d, want %d; body: %s", c.name, c.path, w.Code, c.want, w.Body.String())
		}
	}

	w := get(path+"?view=unmatched", admin)
	var listing contracts.RadioTrackCandidateListing
	if err := json.Unmarshal(w.Body.Bytes(), &listing); err != nil {
		t.Fatalf("decode listing: %v; body: %s", err, w.Body.String())
	}
	if listing.View != contracts.RadioTrackCandidateViewUnmatched || listing.Candidates == nil || len(listing.Candidates) != 0 || listing.Total != 0 {
		t.Errorf("no plays: got %+v, want the unmatched view, an empty candidates array and a zero total", listing)
	}
	if listing.WindowDays != 180 || listing.MinStationRows != 2 || listing.MinPlays != 3 || listing.WindowStart == "" {
		t.Errorf("no plays: got window %q/%d and floors %d/%d, want the window and floors reported", listing.WindowStart, listing.WindowDays, listing.MinStationRows, listing.MinPlays)
	}
}
