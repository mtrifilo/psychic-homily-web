package routes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	autherrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/services"
	"psychic-homily-backend/internal/testutil"
)

// The feed status and forget routes sit beside the legacy /calendar/{token}
// alias, which chi registers on the same tree. Each path must reach its own
// route, and a two-segment path that is not one of the literals must still
// reach the alias.
func TestCalendarTokenRoutesResolution(t *testing.T) {
	router := newTestRouter(t)

	for _, c := range []struct {
		method, path, want string
	}{
		{http.MethodGet, "/calendar/token", "/calendar/token"},
		{http.MethodPost, "/calendar/token", "/calendar/token"},
		{http.MethodDelete, "/calendar/token", "/calendar/token"},
		{http.MethodGet, "/calendar/token/status", calendarFeedStatusRoute},
		{http.MethodDelete, "/calendar/token/reads", calendarFeedReadsRoute},
		{http.MethodGet, "/calendar/phcal_abc123", legacyCalendarFeedRoute},
	} {
		if got := router.Find(chi.NewRouteContext(), c.method, c.path); got != c.want {
			t.Errorf("%s %s routes to %q, want %q", c.method, c.path, got, c.want)
		}
	}
}

// statusBody is a decoded JSON response body.
type statusBody = map[string]any

// End to end through the built router against a real database: the legacy
// alias serves a real token and records the read; the address-free status
// lists it and carries no address for a session or an API token; the token
// status still gives a session its URLs; forgetting the client removes it
// until the client reads the link again; an API token cannot forget, and an
// unknown client is refused.
func TestCalendarFeedStatusAndForgetThroughRouter(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	cfg := testConfig()
	sc := services.NewServiceContainer(td.DB, cfg)
	// HumaJWTMiddleware validates API tokens on the process-wide handle it
	// reads when the router is built, so it is pointed at this database first.
	prev := db.DB
	db.DB = td.DB
	t.Cleanup(func() { db.DB = prev })
	router := chi.NewRouter()
	SetupRoutes(router, sc, cfg)

	// API tokens validate only for an admin owner, so the account under test
	// is an admin and holds both credentials.
	user := testhelpers.CreateAdminUser(td.DB)
	session, err := sc.JWT.CreateToken(user)
	if err != nil {
		t.Fatalf("mint session: %v", err)
	}
	apiToken, err := sc.APIToken.CreateToken(user.ID, nil, 1)
	if err != nil {
		t.Fatalf("mint API token: %v", err)
	}
	if _, _, err := sc.APIToken.ValidateToken(apiToken.Token); err != nil {
		t.Fatalf("the API token fixture is not valid, so its refusals prove nothing: %v", err)
	}

	send := func(method, path, bearer, userAgent string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		if userAgent != "" {
			req.Header.Set("User-Agent", userAgent)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	decode := func(w *httptest.ResponseRecorder) statusBody {
		t.Helper()
		var body statusBody
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode %q: %v", w.Body.String(), err)
		}
		return body
	}
	readClients := func(body statusBody) []string {
		reads, _ := body["reads"].([]any)
		var clients []string
		for _, r := range reads {
			read, _ := r.(map[string]any)
			client, _ := read["client"].(string)
			clients = append(clients, client)
		}
		return clients
	}
	feedStatus := func(bearer string) (statusBody, string) {
		t.Helper()
		w := send(http.MethodGet, calendarFeedStatusRoute, bearer, "")
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s = %d; body: %s", calendarFeedStatusRoute, w.Code, w.Body.String())
		}
		if got := w.Header().Get("Cache-Control"); got != "no-store" {
			t.Errorf("GET %s Cache-Control = %q, want no-store", calendarFeedStatusRoute, got)
		}
		return decode(w), w.Body.String()
	}
	// The feed handler records a read after its response, on a background
	// writer, so the status is polled until the read lands.
	awaitReads := func(want ...string) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for {
			body, raw := feedStatus(session)
			if strings.Join(readClients(body), ",") == strings.Join(want, ",") {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("status reads = %s, want clients %v", raw, want)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}

	w := send(http.MethodPost, "/calendar/token", session, "")
	if w.Code != http.StatusOK {
		t.Fatalf("POST /calendar/token = %d; body: %s", w.Code, w.Body.String())
	}
	feedToken, _ := decode(w)["token"].(string)
	if !strings.HasPrefix(feedToken, "phcal_") {
		t.Fatalf("minted token = %q", feedToken)
	}

	// The legacy alias serves the real token, and a Google poll of it is a read.
	feed := send(http.MethodGet, "/calendar/"+feedToken, "", "Google-Calendar-Importer")
	if feed.Code != http.StatusOK || !strings.Contains(feed.Body.String(), "BEGIN:VCALENDAR") {
		t.Fatalf("GET /calendar/{token} = %d %q, want 200 with a calendar", feed.Code, feed.Body.String())
	}
	awaitReads("google")

	// No feed address in the address-free status, for either credential.
	for name, bearer := range map[string]string{"session": session, "API token": apiToken.Token} {
		body, raw := feedStatus(bearer)
		for _, key := range []string{"feed_url", "follows_feed_url", "url_recoverable"} {
			if _, present := body[key]; present {
				t.Errorf("%s: %s carries %s: %s", name, calendarFeedStatusRoute, key, raw)
			}
		}
		if strings.Contains(raw, feedToken) || strings.Contains(raw, "phcal_") {
			t.Errorf("%s: %s carries the feed token: %s", name, calendarFeedStatusRoute, raw)
		}
		if body["has_token"] != true || body["created_at"] == nil {
			t.Errorf("%s: %s = %s, want has_token and created_at", name, calendarFeedStatusRoute, raw)
		}
	}
	// An API token learns only that a token exists and when it was minted.
	if body, raw := feedStatus(apiToken.Token); len(body) != 2 {
		t.Errorf("API token: %s = %s, want has_token and created_at only", calendarFeedStatusRoute, raw)
	}

	// The token status still gives a session the URLs.
	w = send(http.MethodGet, "/calendar/token", session, "")
	if body := decode(w); w.Code != http.StatusOK ||
		!strings.Contains(body["feed_url"].(string), feedToken) || !strings.Contains(body["follows_feed_url"].(string), feedToken) {
		t.Errorf("GET /calendar/token = %d %s, want both feed URLs for a session", w.Code, w.Body.String())
	}

	// An API token cannot forget, and nothing is forgotten.
	w = send(http.MethodDelete, calendarFeedReadsRoute+"?client=google", apiToken.Token, "")
	if w.Code != http.StatusForbidden || decode(w)["error_code"] != autherrors.CodeSessionCredentialRequired {
		t.Errorf("API token: DELETE %s = %d %s, want 403 %s",
			calendarFeedReadsRoute, w.Code, w.Body.String(), autherrors.CodeSessionCredentialRequired)
	}
	awaitReads("google")

	// An unknown or missing client is refused, not forgotten as a no-op.
	for _, query := range []string{"?client=outlook", "?client=", ""} {
		if w := send(http.MethodDelete, calendarFeedReadsRoute+query, session, ""); w.Code != http.StatusUnprocessableEntity {
			t.Errorf("DELETE %s%s = %d %s, want 422", calendarFeedReadsRoute, query, w.Code, w.Body.String())
		}
	}
	awaitReads("google")

	// A session forgets the client; forgetting it again still succeeds.
	for i := 0; i < 2; i++ {
		w := send(http.MethodDelete, calendarFeedReadsRoute+"?client=google", session, "")
		if w.Code != http.StatusOK || decode(w)["success"] != true {
			t.Fatalf("DELETE %s?client=google #%d = %d %s, want 200", calendarFeedReadsRoute, i+1, w.Code, w.Body.String())
		}
		if body, raw := feedStatus(session); body["reads"] != nil {
			t.Errorf("after forgetting: %s = %s, want no reads", calendarFeedStatusRoute, raw)
		}
	}

	// The client's next real read brings it back.
	if feed := send(http.MethodGet, "/feeds/"+feedToken+"/saved-shows.ics", "", "Google-Calendar-Importer"); feed.Code != http.StatusOK {
		t.Fatalf("GET saved-shows feed = %d", feed.Code)
	}
	awaitReads("google")
}
