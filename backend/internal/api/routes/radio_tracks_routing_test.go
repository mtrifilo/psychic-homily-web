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

// TestAdminRadioTracksRoute pins GET /admin/radio/tracks at the router: it sits
// behind rc.Admin, and huma enforces the sort enum and the limit bounds before
// the handler runs, which a direct handler test cannot see.
func TestAdminRadioTracksRoute(t *testing.T) {
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

	cases := []struct {
		name, path, token string
		want              int
	}{
		{"unauthenticated", "/admin/radio/tracks", "", http.StatusUnauthorized},
		{"non-admin", "/admin/radio/tracks", regular, http.StatusForbidden},
		{"admin, defaults", "/admin/radio/tracks", admin, http.StatusOK},
		{"admin, family sort", "/admin/radio/tracks?sort=station_families&limit=5", admin, http.StatusOK},
		{"unknown sort", "/admin/radio/tracks?sort=bogus", admin, http.StatusUnprocessableEntity},
		{"limit below range", "/admin/radio/tracks?limit=0", admin, http.StatusUnprocessableEntity},
		{"limit above range", "/admin/radio/tracks?limit=501", admin, http.StatusUnprocessableEntity},
	}
	for _, c := range cases {
		w := get(c.path, c.token)
		if w.Code != c.want {
			t.Errorf("%s: GET %s = %d, want %d; body: %s", c.name, c.path, w.Code, c.want, w.Body.String())
		}
	}

	w := get("/admin/radio/tracks", admin)
	var listing contracts.RadioTrackListing
	if err := json.Unmarshal(w.Body.Bytes(), &listing); err != nil {
		t.Fatalf("decode listing: %v; body: %s", err, w.Body.String())
	}
	if listing.Tracks == nil || len(listing.Tracks) != 0 || listing.TotalTracks != 0 {
		t.Errorf("empty table: got %+v, want an empty tracks array and zero totals", listing)
	}
}
