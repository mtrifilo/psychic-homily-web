package routes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services"
	"psychic-homily-backend/internal/testutil"
)

// TestSitemapEntriesEndToEnd drives a real HTTP request through the registered
// router, so route registration, handler wiring and JSON serialisation are all
// covered — not just the service in isolation.
//
// The generator is an unattended consumer: nobody eyeballs its output, which is
// how the previous one went stale unnoticed (see contracts.SitemapEntry). The
// contract it depends on gets a test that speaks HTTP.
func TestSitemapEntriesEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	slug := "e2e-approved-show"
	show := &catalogm.Show{
		Title:     "End To End Show",
		Slug:      &slug,
		EventDate: time.Now().Add(48 * time.Hour),
		Status:    catalogm.ShowStatusApproved,
	}
	if err := td.DB.Create(show).Error; err != nil {
		t.Fatalf("seed show: %v", err)
	}

	cfg := testConfig()
	router := chi.NewRouter()
	SetupRoutes(router, services.NewServiceContainer(td.DB, cfg), cfg)

	req := httptest.NewRequest(http.MethodGet, "/sitemap/entries", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /sitemap/entries = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	var body struct {
		Shows []struct {
			Slug      string    `json:"slug"`
			UpdatedAt time.Time `json:"updated_at"`
		} `json:"shows"`
		Artists    []json.RawMessage `json:"artists"`
		Venues     []json.RawMessage `json:"venues"`
		VenueYears []json.RawMessage `json:"venue_years"`
		Scenes     []json.RawMessage `json:"scenes"`
		SceneWeeks []json.RawMessage `json:"scene_weeks"`
		Labels     []json.RawMessage `json:"labels"`
		Releases   []json.RawMessage `json:"releases"`
		Festivals  []json.RawMessage `json:"festivals"`
		Tags       []json.RawMessage `json:"tags"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("parse response: %v; body: %s", err, w.Body.String())
	}

	if len(body.Shows) != 1 || body.Shows[0].Slug != slug {
		t.Fatalf("shows = %+v, want exactly one entry for %q", body.Shows, slug)
	}
	if body.Shows[0].UpdatedAt.IsZero() {
		t.Error("updated_at did not survive serialisation — <lastmod> would be empty")
	}

	// Empty families must serialise as [] rather than null: the generator
	// iterates each list, and a null would need a nil check that is easy to omit
	// and silent when forgotten.
	for name, raw := range map[string][]json.RawMessage{
		"artists": body.Artists, "venues": body.Venues, "venue_years": body.VenueYears,
		"scenes":      body.Scenes,
		"scene_weeks": body.SceneWeeks, "labels": body.Labels, "releases": body.Releases,
		"festivals": body.Festivals, "tags": body.Tags,
	} {
		if raw == nil {
			t.Errorf("%s serialised as null, want []; body: %s", name, w.Body.String())
		}
	}
}

// TestSitemapEntriesRejectsAnUnknownFamilyOverHTTP pins the one status the
// whole sub-shard rollout design rests on.
//
// A shard id rides in `family` precisely because a backend that predates it
// answers 422, which the generator degrades to an empty document for one deploy
// window (UNKNOWN_FAMILY_STATUSES in frontend/app/sitemap.ts), the prerender
// gate excuses, and the freshness monitor reports as an unserved shard. All
// three read the STATUS, and huma produces it from the enum tag rather than
// from any code a service test exercises, so it needs a test that speaks HTTP.
//
// The `shows-2026-01` case is a retired id: an old shard id must be rejected
// the same way an invented one is, or a stale frontend would be served a
// silently wrong answer instead of a degradable one.
func TestSitemapEntriesRejectsAnUnknownFamilyOverHTTP(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	cfg := testConfig()
	router := chi.NewRouter()
	SetupRoutes(router, services.NewServiceContainer(td.DB, cfg), cfg)

	for _, family := range []string{"collections", "shows-b99", "shows-2026-01", "releases-a-e", "shows-"} {
		req := httptest.NewRequest(http.MethodGet, "/sitemap/entries?family="+family, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusUnprocessableEntity {
			t.Errorf("GET /sitemap/entries?family=%s = %d, want 422; body: %s", family, w.Code, w.Body.String())
		}
	}

	// A served id answers 200 through the same path, so the check above is
	// about the value and not about the route being broken.
	req := httptest.NewRequest(http.MethodGet, "/sitemap/entries?family=shows-b0", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("GET /sitemap/entries?family=shows-b0 = %d, want 200; body: %s", w.Code, w.Body.String())
	}
}
