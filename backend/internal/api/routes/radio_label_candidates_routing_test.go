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

// TestAdminRadioLabelCandidatesRoute pins GET /admin/radio/candidates/labels
// at the router: it sits behind rc.Admin, and huma enforces the lane and sort
// enums and the limit bounds before the handler runs.
func TestAdminRadioLabelCandidatesRoute(t *testing.T) {
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

	const path = "/admin/radio/candidates/labels"
	cases := []struct {
		name, path, token string
		want              int
	}{
		{"unauthenticated", path, "", http.StatusUnauthorized},
		{"non-admin", path, regular, http.StatusForbidden},
		{"admin, defaults", path, admin, http.StatusOK},
		{"admin, every param", path + "?lane=auto&sort=velocity&limit=500", admin, http.StatusOK},
		{"admin, human lane by plays", path + "?lane=human&sort=plays&limit=1", admin, http.StatusOK},
		{"admin, by stations", path + "?sort=stations", admin, http.StatusOK},
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
	var listing contracts.RadioLabelCandidateListing
	if err := json.Unmarshal(w.Body.Bytes(), &listing); err != nil {
		t.Fatalf("decode listing: %v; body: %s", err, w.Body.String())
	}
	if listing.Candidates == nil || len(listing.Candidates) != 0 || listing.Total != 0 {
		t.Errorf("no plays: got %+v, want an empty candidates array and a zero total", listing)
	}
	if listing.WindowDays != 180 || listing.VelocityDays != 30 || listing.MinStationRows != 2 ||
		listing.MinVelocity != 1.5 || listing.MinNewPlays != 3 || listing.RecentStart == "" || listing.PriorStart == "" {
		t.Errorf("no plays: got %+v, want the windows and floors reported", listing)
	}
}
