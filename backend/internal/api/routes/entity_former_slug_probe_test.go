package routes

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services"
	"psychic-homily-backend/internal/testutil"
)

// statementRecorder is a GORM logger whose Trace runs once per executed SQL
// statement, recording its text.
type statementRecorder struct {
	gormlogger.Interface
	mu   *sync.Mutex
	stmt *[]string
}

func (r statementRecorder) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()
	r.mu.Lock()
	*r.stmt = append(*r.stmt, sql)
	r.mu.Unlock()
}

// probeStatements runs one HEAD through the built router and returns the
// response and every SQL statement the request executed.
func probeStatements(t *testing.T, router *chi.Mux, rec statementRecorder, path string) (*httptest.ResponseRecorder, []string) {
	t.Helper()
	rec.mu.Lock()
	*rec.stmt = nil
	rec.mu.Unlock()
	req := httptest.NewRequest(http.MethodHead, path, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	rec.mu.Lock()
	defer rec.mu.Unlock()
	return w, append([]string(nil), *rec.stmt...)
}

func readsFormerSlugs(statements []string) bool {
	for _, sql := range statements {
		if strings.Contains(sql, "entity_former_slugs") {
			return true
		}
	}
	return false
}

// The existence probes the frontend proxy calls answer a live slug exactly as
// they always have, with no header and without reading entity_former_slugs, and
// spend one lookup more only on a miss. Measured on the built router over a
// real database, so the handler, the service and the SQL are all in the count.
func TestFormerSlugProbesCostNothingOnALiveSlug(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	suffix := time.Now().UnixNano()
	seed := func(table, oldSlug, newSlug string, row any) {
		t.Helper()
		if err := td.DB.Create(row).Error; err != nil {
			t.Fatalf("seed %s: %v", table, err)
		}
		if err := td.DB.Exec(fmt.Sprintf("UPDATE %s SET slug = ? WHERE slug = ?", table), newSlug, oldSlug).Error; err != nil {
			t.Fatalf("rename %s: %v", table, err)
		}
	}
	names := func(family string) (string, string) {
		return fmt.Sprintf("%s-old-%d", family, suffix), fmt.Sprintf("%s-new-%d", family, suffix)
	}

	venueOld, venueNew := names("venue")
	venue := &catalogm.Venue{Name: "Probe Room", Slug: &venueOld, City: "Phoenix", State: "AZ"}
	seed("venues", venueOld, venueNew, venue)
	artistOld, artistNew := names("artist")
	seed("artists", artistOld, artistNew, &catalogm.Artist{Name: "Probe Band", Slug: &artistOld})
	showOld, showNew := names("show")
	pastShow := &catalogm.Show{
		Title: "Probe Show", Slug: &showOld, Status: catalogm.ShowStatusApproved,
		EventDate: time.Date(2024, 6, 1, 3, 0, 0, 0, time.UTC),
	}
	seed("shows", showOld, showNew, pastShow)
	if err := td.DB.Create(&catalogm.ShowVenue{ShowID: pastShow.ID, VenueID: venue.ID}).Error; err != nil {
		t.Fatalf("seed show_venue: %v", err)
	}

	var mu sync.Mutex
	var statements []string
	rec := statementRecorder{Interface: gormlogger.Discard, mu: &mu, stmt: &statements}
	counted := td.DB.Session(&gorm.Session{Logger: rec})
	cfg := testConfig()
	router := chi.NewRouter()
	SetupRoutes(router, services.NewServiceContainer(counted, cfg), cfg)

	for _, tc := range []struct{ prefix, live, former string }{
		{"shows", showNew, showOld},
		{"venues", venueNew, venueOld},
		{"artists", artistNew, artistOld},
	} {
		w, stmts := probeStatements(t, router, rec, "/entities/"+tc.prefix+"/"+tc.live+"/exists")
		if w.Code != http.StatusOK || len(w.Header().Values("Canonical-Slug")) != 0 {
			t.Errorf("%s live slug = %d Canonical-Slug %q, want 200 and no header", tc.prefix, w.Code, w.Header().Values("Canonical-Slug"))
		}
		if len(stmts) != 1 || readsFormerSlugs(stmts) {
			t.Errorf("%s live slug ran %d statements %q, want the one live lookup", tc.prefix, len(stmts), stmts)
		}
		t.Logf("%s live slug: %d statement(s)", tc.prefix, len(stmts))

		w, stmts = probeStatements(t, router, rec, "/entities/"+tc.prefix+"/"+tc.former+"/exists")
		if w.Code != http.StatusOK || w.Header().Get("Canonical-Slug") != tc.live {
			t.Errorf("%s former slug = %d Canonical-Slug %q, want 200 and %q", tc.prefix, w.Code, w.Header().Get("Canonical-Slug"), tc.live)
		}
		if len(stmts) != 2 || !readsFormerSlugs(stmts) {
			t.Errorf("%s former slug ran %d statements %q, want the live lookup plus one former slug lookup", tc.prefix, len(stmts), stmts)
		}
		t.Logf("%s former slug: %d statement(s)", tc.prefix, len(stmts))

		w, stmts = probeStatements(t, router, rec, "/entities/"+tc.prefix+"/never-existed-"+tc.prefix+"/exists")
		if w.Code != http.StatusNotFound || len(stmts) != 2 {
			t.Errorf("%s never-existed slug = %d after %d statements, want 404 after 2", tc.prefix, w.Code, len(stmts))
		}
		t.Logf("%s never-existed slug: %d statement(s)", tc.prefix, len(stmts))
	}

	// A family without former slugs pays nothing extra on a miss either.
	w, stmts := probeStatements(t, router, rec, "/entities/tags/never-existed-tag/exists")
	if w.Code != http.StatusNotFound || len(stmts) != 1 {
		t.Errorf("tag miss = %d after %d statements, want 404 after 1", w.Code, len(stmts))
	}

	// The venue year archive's probe: a live slug runs what it always has and
	// never reads entity_former_slugs; a former slug answers for the venue it
	// now stands for.
	w, liveStmts := probeStatements(t, router, rec, "/venues/"+venueNew+"/shows/2024/exists")
	if w.Code != http.StatusOK || len(w.Header().Values("Canonical-Slug")) != 0 || readsFormerSlugs(liveStmts) {
		t.Errorf("venue year live slug = %d Canonical-Slug %q after %q, want 200, no header, no former slug read",
			w.Code, w.Header().Values("Canonical-Slug"), liveStmts)
	}
	t.Logf("venue year live slug: %d statement(s)", len(liveStmts))
	w, formerStmts := probeStatements(t, router, rec, "/venues/"+venueOld+"/shows/2024/exists")
	if w.Code != http.StatusOK || w.Header().Get("Canonical-Slug") != venueNew {
		t.Errorf("venue year former slug = %d Canonical-Slug %q, want 200 and %q", w.Code, w.Header().Get("Canonical-Slug"), venueNew)
	}
	// The former slug costs the live path's lookups plus the one former slug
	// lookup: the venue read misses, so the year probe still runs once.
	if len(formerStmts) != len(liveStmts)+1 || !readsFormerSlugs(formerStmts) {
		t.Errorf("venue year former slug ran %d statements %q, want the live path's %d plus one former slug lookup",
			len(formerStmts), formerStmts, len(liveStmts))
	}
	t.Logf("venue year former slug: %d statement(s)", len(formerStmts))
	w, neverStmts := probeStatements(t, router, rec, "/venues/never-existed-venue/shows/2024/exists")
	if w.Code != http.StatusNotFound || len(neverStmts) != 2 || !readsFormerSlugs(neverStmts) {
		t.Errorf("venue year never-existed slug = %d after %d statements %q, want 404 after the venue read and the former slug lookup",
			w.Code, len(neverStmts), neverStmts)
	}
	t.Logf("venue year never-existed slug: %d statement(s)", len(neverStmts))
	w, _ = probeStatements(t, router, rec, "/venues/"+venueOld+"/shows/2019/exists")
	if w.Code != http.StatusNotFound {
		t.Errorf("venue year former slug, empty year = %d, want 404", w.Code)
	}
}
