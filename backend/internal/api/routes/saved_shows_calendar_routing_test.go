package routes

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/services"
	"psychic-homily-backend/internal/testutil"
)

// The one-shot saved-shows download is a chi route whose path shares the
// /saved-shows/ prefix with the Huma {show_id} operations. chi ranks a static
// segment above a parameter, so the download must win for GET and the Huma
// operations must keep every other method and shape. Handler tests call the
// handler directly and cannot see any of this.
func TestSavedShowsCalendarDownloadRouteResolution(t *testing.T) {
	router := newTestRouter(t)

	for _, c := range []struct {
		method, path, want string
	}{
		{http.MethodGet, "/saved-shows/calendar.ics", savedShowsCalendarDownloadRoute},
		{http.MethodGet, "/saved-shows/123/check", "/saved-shows/{show_id}/check"},
		{http.MethodGet, "/saved-shows", "/saved-shows"},
		// The static segment carries no POST or DELETE, so those still fall
		// through to the save and unsave operations, exactly as before.
		{http.MethodPost, "/saved-shows/calendar.ics", "/saved-shows/{show_id}"},
		{http.MethodDelete, "/saved-shows/calendar.ics", "/saved-shows/{show_id}"},
		{http.MethodPost, "/saved-shows/123", "/saved-shows/{show_id}"},
	} {
		if got := router.Find(chi.NewRouteContext(), c.method, c.path); got != c.want {
			t.Errorf("%s %s routes to %q, want %q", c.method, c.path, got, c.want)
		}
	}
}

// The download needs a session. A request with none, with a credential that is
// not a session JWT, or with an API token is refused by JWTMiddleware before
// the handler runs.
func TestSavedShowsCalendarDownloadRefusesWithoutSession(t *testing.T) {
	router := newTestRouter(t)

	for name, decorate := range map[string]func(*http.Request){
		"no credential":  func(*http.Request) {},
		"garbage bearer": func(r *http.Request) { r.Header.Set("Authorization", "Bearer not-a-jwt") },
		"garbage cookie": func(r *http.Request) { r.AddCookie(&http.Cookie{Name: config.AuthCookieName, Value: "not-a-jwt"}) },
		"api token":      func(r *http.Request) { r.Header.Set("Authorization", "Bearer phk_0123456789abcdef") },
	} {
		req := httptest.NewRequest(http.MethodGet, savedShowsCalendarDownloadRoute, nil)
		decorate(req)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s: GET %s = %d, want 401", name, savedShowsCalendarDownloadRoute, w.Code)
		}
		if strings.Contains(w.Body.String(), "BEGIN:VCALENDAR") {
			t.Errorf("%s: a refused request carried calendar bytes", name)
		}
	}
}

// Session-authenticated, so it is not a personal feed: the token-feed exemption
// must not read a token out of its path, which would take it off the per-user
// public-read budget.
func TestSavedShowsCalendarDownloadIsNotATokenFeed(t *testing.T) {
	if token := personalFeedTokenFromPath(savedShowsCalendarDownloadRoute); token != "" {
		t.Errorf("%s reads as personal-feed token %q", savedShowsCalendarDownloadRoute, token)
	}
}

// End to end through the built router against a real database: a show saved
// through the API a moment before the download is in the file, while the
// subscription feed still serves the bytes it cached before that save. Pending,
// cancelled and past saves stay out.
func TestSavedShowsCalendarDownloadThroughRouter(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	cfg := testConfig()
	sc := services.NewServiceContainer(td.DB, cfg)
	router := chi.NewRouter()
	SetupRoutes(router, sc, cfg)

	user := testhelpers.CreateTestUser(td.DB)
	token, err := sc.JWT.CreateToken(user)
	if err != nil {
		t.Fatalf("mint token: %v", err)
	}
	send := func(method, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	uid := func(id uint) string { return fmt.Sprintf("UID:show-%d@psychichomily.com\r\n", id) }

	early := testhelpers.CreateFutureApprovedShow(td.DB, user.ID, "Early Save", 5)
	pending := testhelpers.CreatePendingShow(td.DB, user.ID, "Pending Save")
	cancelled := testhelpers.CreateFutureApprovedShow(td.DB, user.ID, "Cancelled Save", 6)
	if err := td.DB.Model(cancelled).Update("is_cancelled", true).Error; err != nil {
		t.Fatalf("cancel fixture: %v", err)
	}
	past := testhelpers.CreatePastApprovedShow(td.DB, user.ID, "Past Save", 3)
	for _, id := range []uint{early.ID, pending.ID, cancelled.ID, past.ID} {
		if err := sc.SavedShow.SaveShow(user.ID, id); err != nil {
			t.Fatalf("save fixture %d: %v", id, err)
		}
	}

	// The subscription feed renders and caches before the next save.
	frontendURL := "http://localhost:3000"
	cachedFeed, err := sc.Calendar.GenerateICSFeed(user.ID, frontendURL)
	if err != nil {
		t.Fatalf("prime feed cache: %v", err)
	}

	late := testhelpers.CreateFutureApprovedShow(td.DB, user.ID, "Late Save", 8)
	if w := send(http.MethodPost, fmt.Sprintf("/saved-shows/%d", late.ID)); w.Code != http.StatusOK {
		t.Fatalf("POST /saved-shows/%d = %d; body: %s", late.ID, w.Code, w.Body.String())
	}

	w := send(http.MethodGet, savedShowsCalendarDownloadRoute)
	if w.Code != http.StatusOK {
		t.Fatalf("GET %s = %d; body: %s", savedShowsCalendarDownloadRoute, w.Code, w.Body.String())
	}
	for header, want := range map[string]string{
		"Content-Type":        "text/calendar; charset=utf-8",
		"Content-Disposition": `attachment; filename="psychic-homily-saved-shows.ics"`,
		"Cache-Control":       "private, no-store",
		"ETag":                "",
	} {
		if got := w.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}

	// The wire itself is pinned by the service's RFC 5545 tests; this checks
	// only that the response carries those bytes: CRLF line endings and a
	// DTSTAMP and SEQUENCE for each VEVENT.
	body := w.Body.Bytes()
	if n := bytes.Count(body, []byte("BEGIN:VEVENT\r\n")); n != 2 {
		t.Errorf("download carries %d VEVENTs, want 2 (the early and late saves)", n)
	}
	if lf, crlf := bytes.Count(body, []byte("\n")), bytes.Count(body, []byte("\r\n")); lf != crlf {
		t.Errorf("download has %d bare LF line endings", lf-crlf)
	}
	for _, prop := range []string{"\r\nDTSTAMP:", "\r\nSEQUENCE:"} {
		if n := bytes.Count(body, []byte(prop)); n != 2 {
			t.Errorf("download has %d %q lines, want one per VEVENT", n, strings.TrimSpace(prop))
		}
	}
	for _, id := range []uint{early.ID, late.ID} {
		if !bytes.Contains(body, []byte(uid(id))) {
			t.Errorf("download is missing saved show %d", id)
		}
	}
	for name, id := range map[string]uint{"pending": pending.ID, "cancelled": cancelled.ID, "past": past.ID} {
		if bytes.Contains(body, []byte(uid(id))) {
			t.Errorf("download carries the %s save %d", name, id)
		}
	}

	stillCached, err := sc.Calendar.GenerateICSFeed(user.ID, frontendURL)
	if err != nil {
		t.Fatalf("re-read feed: %v", err)
	}
	if !bytes.Equal(stillCached, cachedFeed) {
		t.Error("the subscription feed changed; this test needs it cached to show the download bypasses that cache")
	}
	if bytes.Contains(stillCached, []byte(uid(late.ID))) {
		t.Error("the cached subscription feed already carries the late save, so the freshness check proves nothing")
	}

	// The {show_id} sibling still reaches its own handler for a real show id.
	if w := send(http.MethodGet, fmt.Sprintf("/saved-shows/%d/check", late.ID)); w.Code != http.StatusOK ||
		!strings.Contains(w.Body.String(), `"is_saved":true`) {
		t.Errorf("GET /saved-shows/%d/check = %d %s, want 200 with is_saved true", late.ID, w.Code, w.Body.String())
	}
}
