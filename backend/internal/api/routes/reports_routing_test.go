package routes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services"
	"psychic-homily-backend/internal/testutil"
)

// PSY-1633. The artist report pipeline shipped broken for a reason no
// handler-level test could see: handler tests call the handler function
// directly, so they pass whether or not the route that reaches it exists.
//
// Two setups claimed the same path SHAPE with different parameter names —
// `POST /artists/{artist_id}/report` and `POST /artists/{entity_id}/report`.
// chi keys its routing tree on shape, not on parameter name, so those are one
// route with two claimants; chi silently keeps the later registration instead
// of erroring. The artist-specific handler was therefore never reachable, and
// its read-back and admin queue searched a table nothing ever wrote to.
//
// These tests speak to the BUILT ROUTER, which is the only place that
// distinction is observable.

// chiRoutes returns every (method, pattern) pair in the built routing tree.
// Patterns keep their parameter names, which is the whole point: the name is
// what tells you WHICH registration survived a shape collision.
func chiRoutes(t *testing.T, router *chi.Mux) map[string][]string {
	t.Helper()
	routes := map[string][]string{}
	err := chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		routes[method] = append(routes[method], route)
		return nil
	})
	if err != nil {
		t.Fatalf("walk chi tree: %v", err)
	}
	return routes
}

// matching returns the patterns for `method` whose shape matches `shape`, i.e.
// the pattern with every {param} replaced by {}.
func matching(routes map[string][]string, method, shape string) []string {
	param := regexp.MustCompile(`\{[^}]*\}`)
	var out []string
	for _, r := range routes[method] {
		if param.ReplaceAllString(r, "{}") == shape {
			out = append(out, r)
		}
	}
	return out
}

// TestArtistReportRoutesAreRegisteredOnce walks the built tree and pins both
// halves of the fix: the artist report paths exist exactly once, and they carry
// the ENTITY handler's parameter name.
//
// If anyone re-introduces an artist-specific registration on either path, chi
// will keep whichever registered last and this test fails on the parameter
// name — the same signal that was missing the first time.
func TestArtistReportRoutesAreRegisteredOnce(t *testing.T) {
	routes := chiRoutes(t, newTestRouter(t))

	for _, want := range []struct{ method, shape, pattern string }{
		{http.MethodPost, "/artists/{}/report", "/artists/{entity_id}/report"},
		{http.MethodGet, "/artists/{}/my-report", "/artists/{entity_id}/my-report"},
	} {
		got := matching(routes, want.method, want.shape)
		if len(got) != 1 {
			t.Errorf("%s %s: %d registered routes %v, want exactly 1 — a second registration "+
				"of this shape would be silently dropped by chi", want.method, want.shape, len(got), got)
			continue
		}
		if got[0] != want.pattern {
			t.Errorf("%s %s resolved to %q, want %q — a non-entity handler claimed the path",
				want.method, want.shape, got[0], want.pattern)
		}
	}

	// The artist-specific queue is gone; entity reports are the one queue.
	for _, gone := range []struct{ method, path string }{
		{http.MethodGet, "/admin/artist-reports"},
		{http.MethodPost, "/admin/artist-reports/{report_id}/dismiss"},
		{http.MethodPost, "/admin/artist-reports/{report_id}/resolve"},
	} {
		for _, r := range routes[gone.method] {
			if r == gone.path {
				t.Errorf("%s %s is still registered — it reads artist_reports, which nothing writes",
					gone.method, gone.path)
			}
		}
	}
}

// TestArtistReportPathsResolve proves the patterns above are actually reachable
// rather than merely present: an unregistered path 404s in chi before any Huma
// middleware runs, so a 401 from the auth gate is proof the route matched.
func TestArtistReportPathsResolve(t *testing.T) {
	router := newTestRouter(t)

	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/artists/1/report"},
		{http.MethodGet, "/artists/1/my-report"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s = %d, want 401 (route matched, auth gate rejected); body: %s",
				tc.method, tc.path, w.Code, w.Body.String())
		}
	}
}

// TestArtistReportRoundTripThroughRouter is the end-to-end proof, driven over
// real HTTP through the registered router with a real database and a real JWT.
//
// It exercises the loop PSY-1633 fixed: submit a report, read it back, and find
// it in the admin queue. Before the fix the submission landed in entity_reports
// while both reads searched artist_reports, so steps 2 and 3 came back empty
// even though step 1 reported success.
func TestArtistReportRoundTripThroughRouter(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	cfg := testConfig()
	sc := services.NewServiceContainer(td.DB, cfg)
	router := chi.NewRouter()
	SetupRoutes(router, sc, cfg)

	reporter := &authm.User{
		Email:         strPtr("reporter@psy-1633.test"),
		IsActive:      true,
		EmailVerified: true,
	}
	if err := td.DB.Create(reporter).Error; err != nil {
		t.Fatalf("seed reporter: %v", err)
	}
	admin := &authm.User{
		Email:         strPtr("admin@psy-1633.test"),
		IsActive:      true,
		EmailVerified: true,
		IsAdmin:       true,
	}
	if err := td.DB.Create(admin).Error; err != nil {
		t.Fatalf("seed admin: %v", err)
	}

	slug := "psy-1633-artist"
	artist := &catalogm.Artist{Name: "PSY-1633 Artist", Slug: &slug}
	if err := td.DB.Create(artist).Error; err != nil {
		t.Fatalf("seed artist: %v", err)
	}

	reporterToken, err := sc.JWT.CreateToken(reporter)
	if err != nil {
		t.Fatalf("mint reporter token: %v", err)
	}
	adminToken, err := sc.JWT.CreateToken(admin)
	if err != nil {
		t.Fatalf("mint admin token: %v", err)
	}

	do := func(method, path, token, body string) (int, string) {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w.Code, w.Body.String()
	}

	// 1. Submit — the response must be the ENTITY shape, which is what both
	//    frontend callers now expect.
	code, body := do(http.MethodPost, fmt.Sprintf("/artists/%d/report", artist.ID), reporterToken,
		`{"report_type":"inaccurate","details":"wrong hometown"}`)
	if code != http.StatusOK {
		t.Fatalf("POST report = %d, want 200; body: %s", code, body)
	}

	var created struct {
		ID         uint    `json:"id"`
		EntityType string  `json:"entity_type"`
		EntityID   uint    `json:"entity_id"`
		EntitySlug *string `json:"entity_slug"`
		ReportType string  `json:"report_type"`
		Status     string  `json:"status"`
	}
	if err := json.Unmarshal([]byte(body), &created); err != nil {
		t.Fatalf("parse submit response: %v; body: %s", err, body)
	}
	if created.EntityType != "artist" || created.EntityID != artist.ID {
		t.Fatalf("submit returned entity_type=%q entity_id=%d, want artist/%d — the artist-specific "+
			"handler (artist_id/report_type shape) would have answered instead",
			created.EntityType, created.EntityID, artist.ID)
	}
	if created.EntitySlug == nil || *created.EntitySlug != slug {
		t.Errorf("entity_slug = %v, want %q so the moderation queue can deep-link", created.EntitySlug, slug)
	}
	if created.Status != "pending" {
		t.Errorf("status = %q, want pending", created.Status)
	}

	// 2. Read back — the half that silently returned nothing before.
	code, body = do(http.MethodGet, fmt.Sprintf("/artists/%d/my-report", artist.ID), reporterToken, "")
	if code != http.StatusOK {
		t.Fatalf("GET my-report = %d, want 200; body: %s", code, body)
	}
	var mine struct {
		Report *struct {
			ID         uint   `json:"id"`
			EntityType string `json:"entity_type"`
		} `json:"report"`
	}
	if err := json.Unmarshal([]byte(body), &mine); err != nil {
		t.Fatalf("parse my-report: %v; body: %s", err, body)
	}
	if mine.Report == nil {
		t.Fatalf("my-report returned no report after a successful submit — the read-back "+
			"is searching a different table again; body: %s", body)
	}
	if mine.Report.ID != created.ID || mine.Report.EntityType != "artist" {
		t.Errorf("my-report returned report %d (%s), want %d (artist)",
			mine.Report.ID, mine.Report.EntityType, created.ID)
	}

	// 3. Admin queue — the other half that silently returned nothing.
	code, body = do(http.MethodGet, "/admin/entity-reports?status=pending&entity_type=artist", adminToken, "")
	if code != http.StatusOK {
		t.Fatalf("GET /admin/entity-reports = %d, want 200; body: %s", code, body)
	}
	var queue struct {
		Reports []struct {
			ID         uint   `json:"id"`
			EntityType string `json:"entity_type"`
		} `json:"reports"`
		Total int64 `json:"total"`
	}
	if err := json.Unmarshal([]byte(body), &queue); err != nil {
		t.Fatalf("parse admin queue: %v; body: %s", err, body)
	}
	if queue.Total != 1 || len(queue.Reports) != 1 || queue.Reports[0].ID != created.ID {
		t.Fatalf("admin queue = %+v (total %d), want exactly the report just filed (%d)",
			queue.Reports, queue.Total, created.ID)
	}

	// 4. One table, not two. artist_reports still exists (dropping it is its own
	//    change) but nothing may write to it any more.
	var entityRows, artistRows int64
	td.DB.Table("entity_reports").Count(&entityRows)
	td.DB.Table("artist_reports").Count(&artistRows)
	if entityRows != 1 || artistRows != 0 {
		t.Errorf("entity_reports=%d artist_reports=%d, want 1/0 — the round trip must go through one table",
			entityRows, artistRows)
	}

	// 5. The duplicate guard and the read-back must agree: a second submit is
	//    rejected, which is exactly what my-report reporting a pending report means.
	code, body = do(http.MethodPost, fmt.Sprintf("/artists/%d/report", artist.ID), reporterToken,
		`{"report_type":"duplicate"}`)
	if code != http.StatusConflict {
		t.Errorf("second POST report = %d, want 409; body: %s", code, body)
	}

	// 6. ...and they must agree in the other direction too. Once an admin
	//    resolves the report the user may file a new one, so my-report has to
	//    stop reporting the old one — otherwise the UI blocks a submission the
	//    API accepts. This also keeps the reviewed report's admin_notes, which
	//    are moderator-internal, off a non-admin endpoint.
	code, body = do(http.MethodPost,
		fmt.Sprintf("/admin/entity-reports/%d/resolve", created.ID), adminToken,
		`{"notes":"internal moderator note"}`)
	if code != http.StatusOK {
		t.Fatalf("POST resolve = %d, want 200; body: %s", code, body)
	}

	code, body = do(http.MethodGet, fmt.Sprintf("/artists/%d/my-report", artist.ID), reporterToken, "")
	if code != http.StatusOK {
		t.Fatalf("GET my-report after resolve = %d, want 200; body: %s", code, body)
	}
	if err := json.Unmarshal([]byte(body), &mine); err != nil {
		t.Fatalf("parse my-report after resolve: %v; body: %s", err, body)
	}
	if mine.Report != nil {
		t.Errorf("my-report still returns report %d after it was resolved — pending-only is the "+
			"contract; body: %s", mine.Report.ID, body)
	}
	if strings.Contains(body, "internal moderator note") {
		t.Error("my-report leaked the admin resolution notes to the reporter")
	}
}

func strPtr(s string) *string { return &s }
