package routes

import (
	"bytes"
	"encoding/json"
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

// The download needs a session. A request with none, or with a credential that
// is not a valid session JWT, is refused by JWTMiddleware itself (its error
// code, not the handler's plain-text floor), and a rejected cookie is cleared.
func TestSavedShowsCalendarDownloadRefusesWithoutSession(t *testing.T) {
	router := newTestRouter(t)

	for _, c := range []struct {
		name        string
		decorate    func(*http.Request)
		wantCode    string
		wantCleared bool
	}{
		{"no credential", func(*http.Request) {}, "TOKEN_MISSING", false},
		{"garbage bearer", func(r *http.Request) { r.Header.Set("Authorization", "Bearer not-a-jwt") }, "TOKEN_INVALID", true},
		{"garbage cookie", func(r *http.Request) {
			r.AddCookie(&http.Cookie{Name: config.AuthCookieName, Value: "not-a-jwt"})
		}, "TOKEN_INVALID", true},
	} {
		req := httptest.NewRequest(http.MethodGet, savedShowsCalendarDownloadRoute, nil)
		c.decorate(req)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s: GET %s = %d, want 401", c.name, savedShowsCalendarDownloadRoute, w.Code)
		}
		var body struct {
			ErrorCode string `json:"error_code"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.ErrorCode != c.wantCode {
			t.Errorf("%s: body %q, want a JWTMiddleware refusal with error_code %s", c.name, w.Body.String(), c.wantCode)
		}
		if cleared := clearsAuthCookie(w); cleared != c.wantCleared {
			t.Errorf("%s: auth cookie cleared = %v, want %v", c.name, cleared, c.wantCleared)
		}
	}
}

// clearsAuthCookie reports whether the response expires the auth cookie.
func clearsAuthCookie(w *httptest.ResponseRecorder) bool {
	for _, c := range w.Result().Cookies() {
		if c.Name == config.AuthCookieName && c.Value == "" && c.MaxAge < 0 {
			return true
		}
	}
	return false
}

// Session-authenticated, so it is not a personal feed: the token-feed exemption
// reads no candidate token out of its path, and the request stays on the
// per-user public-read budget.
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
	sendWith := func(method, path string, authorize func(*http.Request)) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		authorize(req)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	bearer := func(tok string) func(*http.Request) {
		return func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+tok) }
	}
	send := func(method, path string) *httptest.ResponseRecorder {
		return sendWith(method, path, bearer(token))
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

	// A browser link sends the session as the auth cookie, not a header.
	viaCookie := sendWith(http.MethodGet, savedShowsCalendarDownloadRoute, func(r *http.Request) {
		r.AddCookie(&http.Cookie{Name: config.AuthCookieName, Value: token})
	})
	if viaCookie.Code != http.StatusOK || !bytes.Equal(viaCookie.Body.Bytes(), body) {
		t.Errorf("cookie session: GET %s = %d, want 200 with the same file", savedShowsCalendarDownloadRoute, viaCookie.Code)
	}

	// A real, valid API token is not a session.
	apiToken, err := sc.APIToken.CreateToken(user.ID, nil, 1)
	if err != nil {
		t.Fatalf("mint API token: %v", err)
	}
	if w := sendWith(http.MethodGet, savedShowsCalendarDownloadRoute, bearer(apiToken.Token)); w.Code != http.StatusUnauthorized ||
		strings.Contains(w.Body.String(), "BEGIN:VCALENDAR") {
		t.Errorf("API token: GET %s = %d, want 401 with no calendar", savedShowsCalendarDownloadRoute, w.Code)
	}

	// The {show_id} sibling still reaches its own handler for a real show id.
	if w := send(http.MethodGet, fmt.Sprintf("/saved-shows/%d/check", late.ID)); w.Code != http.StatusOK ||
		!strings.Contains(w.Body.String(), `"is_saved":true`) {
		t.Errorf("GET /saved-shows/%d/check = %d %s, want 200 with is_saved true", late.ID, w.Code, w.Body.String())
	}

	// The late save being in the download is the property under test. That it
	// also beats a cached feed holds only while saving leaves the feed cache
	// alone, which is the feed's behaviour, not the download's; if a save ever
	// starts clearing that cache, the comparison has nothing left to prove.
	stillCached, err := sc.Calendar.GenerateICSFeed(user.ID, frontendURL)
	if err != nil {
		t.Fatalf("re-read feed: %v", err)
	}
	if !bytes.Equal(stillCached, cachedFeed) || bytes.Contains(stillCached, []byte(uid(late.ID))) {
		t.Skip("the subscription feed no longer serves bytes cached before the save, so there is no stale feed to compare against")
	}
}
