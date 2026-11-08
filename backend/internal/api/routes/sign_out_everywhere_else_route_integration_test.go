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
	authh "psychic-homily-backend/internal/api/handlers/auth"
	"psychic-homily-backend/internal/config"
	autherrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services"
	"psychic-homily-backend/internal/testutil"
)

// POST /auth/sessions/sign-out-everywhere-else driven through the ROUTER by a
// browser holding a real session cookie: every other session is refused
// afterwards, the caller goes on with the renewal it was handed, the
// calendar-feed URL is refused, and the API token keeps working.

type signOutEverywhereElseFixture struct {
	router *chi.Mux
	sc     *services.ServiceContainer
	user   *authm.User
}

func newSignOutEverywhereElseFixture(t *testing.T, email string) *signOutEverywhereElseFixture {
	t.Helper()
	td := testutil.SetupTestPostgres(t)
	t.Cleanup(td.Cleanup)

	cfg := testConfig()
	sc := services.NewServiceContainer(td.DB, cfg)

	// An admin, so the account can hold an API token.
	user := &authm.User{Email: &email, IsActive: true, EmailVerified: true, IsAdmin: true}
	if err := td.DB.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	// HumaJWTMiddleware validates API tokens on the process-wide handle it
	// reads when the router is built, so it is pointed at this database first.
	prev := db.DB
	db.DB = td.DB
	t.Cleanup(func() { db.DB = prev })

	router := chi.NewRouter()
	SetupRoutes(router, sc, cfg)
	return &signOutEverywhereElseFixture{router: router, sc: sc, user: user}
}

// session mints a session for the fixture's account, as it stands in the
// database, whose factor completed at authAt.
func (f *signOutEverywhereElseFixture) session(t *testing.T, authAt time.Time) string {
	t.Helper()
	account, err := f.sc.User.GetUserByID(f.user.ID)
	if err != nil {
		t.Fatalf("reload user: %v", err)
	}
	token, err := f.sc.JWT.RenewSessionToken(account, authAt)
	if err != nil {
		t.Fatalf("mint session: %v", err)
	}
	return token
}

// signOutEverywhereElse posts the control with session as the auth cookie, as
// the browser does.
func signOutEverywhereElse(t *testing.T, router *chi.Mux, ip, session string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, authh.SignOutEverywhereElseRoute, nil)
	req.AddCookie(&http.Cookie{Name: config.AuthCookieName, Value: session})
	req.RemoteAddr = ip
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// renewedSession returns the session cookie a response set, failing when it
// set none.
func renewedSession(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	for _, c := range w.Result().Cookies() {
		if c.Name == config.AuthCookieName && c.Value != "" {
			return c.Value
		}
	}
	t.Fatalf("the response set no session cookie; Set-Cookie %q", w.Header().Values("Set-Cookie"))
	return ""
}

// profileStatus asks for the profile carrying credential as a Bearer token.
func profileStatus(t *testing.T, router *chi.Mux, credential string) int {
	t.Helper()
	return sendAuthed(t, router, http.MethodGet, "/auth/profile", "", "203.0.113.199:1234", credential).Code
}

// feedStatus fetches the saved-shows iCal feed whose URL carries token, as a
// calendar client does, with no session.
func feedStatus(t *testing.T, router *chi.Mux, token string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, strings.Replace(savedShowsFeedRoute, "{token}", token, 1), nil)
	req.RemoteAddr = "203.0.113.198:1234"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w.Code
}

func TestSignOutEverywhereElseRouteThroughRouter(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Setenv(DisableAuthRateLimitsEnvVar, "1")
	f := newSignOutEverywhereElseFixture(t, "sign-out-everywhere-else@test.com")

	authAt := time.Now().Add(-3 * time.Hour).Truncate(time.Second)
	thisDevice := f.session(t, authAt)
	otherBrowser := f.session(t, time.Now().Add(-time.Hour))
	cliToken := f.session(t, time.Now().Add(-5*time.Minute))

	apiToken, err := f.sc.APIToken.CreateToken(f.user.ID, nil, 30)
	if err != nil {
		t.Fatalf("create API token: %v", err)
	}
	calendarToken, err := f.sc.Calendar.CreateToken(f.user.ID, nil, "http://localhost:8080")
	if err != nil {
		t.Fatalf("create calendar token: %v", err)
	}

	// With no credential the JWT middleware refuses before the handler. An
	// API token authenticates but holds no session to keep, so it is refused
	// with nothing minted. The checks that follow show every session still
	// honored after both.
	if w := sendAuthed(t, f.router, http.MethodPost, authh.SignOutEverywhereElseRoute, "", "203.0.113.190:1234", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("a call with no session answered %d, want 401", w.Code)
	}
	if code := profileStatus(t, f.router, apiToken.Token); code != http.StatusOK {
		t.Fatalf("the API token answered %d on the profile, want 200: the router cannot see it", code)
	}
	forbidden := sendAuthed(t, f.router, http.MethodPost, authh.SignOutEverywhereElseRoute, "", "203.0.113.189:1234", apiToken.Token)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("a call with an API token answered %d, want 403; body %s", forbidden.Code, forbidden.Body.String())
	}
	for _, c := range forbidden.Result().Cookies() {
		if c.Name == config.AuthCookieName && c.Value != "" {
			t.Fatal("an API-token caller was handed a session cookie")
		}
	}
	if account, err := f.sc.User.GetUserByID(f.user.ID); err != nil || account.SessionsValidAfter != nil {
		t.Fatalf("an API-token call revoked sessions (stamp %v, err %v)", account.SessionsValidAfter, err)
	}

	for name, credential := range map[string]string{"this device": thisDevice, "another browser": otherBrowser, "a CLI token": cliToken} {
		if code := profileStatus(t, f.router, credential); code != http.StatusOK {
			t.Fatalf("before the call, %s answered %d, want 200", name, code)
		}
	}
	if code := feedStatus(t, f.router, calendarToken.Token); code != http.StatusOK {
		t.Fatalf("before the call, the calendar feed answered %d, want 200", code)
	}

	w := signOutEverywhereElse(t, f.router, "203.0.113.191:1234", thisDevice)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", w.Code, w.Body.String())
	}
	var reply struct {
		Success bool `json:"success"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &reply); err != nil || !reply.Success {
		t.Fatalf("body %s, want success", w.Body.String())
	}
	renewed := renewedSession(t, w)

	for name, credential := range map[string]string{"this device's old token": thisDevice, "another browser": otherBrowser, "a CLI token": cliToken} {
		if code := profileStatus(t, f.router, credential); code != http.StatusUnauthorized {
			t.Errorf("after the call, %s answered %d, want 401", name, code)
		}
	}

	if code := profileStatus(t, f.router, renewed); code != http.StatusOK {
		t.Fatalf("the renewed session answered %d, want 200", code)
	}
	_, renewedAuthAt, err := f.sc.JWT.ValidateSession(renewed)
	if err != nil {
		t.Fatalf("validate renewed session: %v", err)
	}
	if !renewedAuthAt.Equal(authAt) {
		t.Errorf("renewed auth_at = %v, want the caller's %v carried through", renewedAuthAt, authAt)
	}

	if code := profileStatus(t, f.router, apiToken.Token); code != http.StatusOK {
		t.Errorf("the API token answered %d after the call, want 200", code)
	}
	if code := feedStatus(t, f.router, calendarToken.Token); code != http.StatusUnauthorized {
		t.Errorf("after the call, the calendar feed answered %d, want 401", code)
	}

	t.Run("a revoked session cannot use it to mint itself a current one", func(t *testing.T) {
		w := signOutEverywhereElse(t, f.router, "203.0.113.192:1234", otherBrowser)
		if w.Code != http.StatusUnauthorized || !strings.Contains(w.Body.String(), autherrors.CodeTokenInvalid) {
			t.Fatalf("got %d %s, want the 401 TOKEN_INVALID refusal", w.Code, w.Body.String())
		}
		if code := profileStatus(t, f.router, renewed); code != http.StatusOK {
			t.Errorf("the refused call signed out the session that made the last one: %d", code)
		}
	})

	t.Run("the renewed session can sign out the rest again", func(t *testing.T) {
		w := signOutEverywhereElse(t, f.router, "203.0.113.193:1234", renewed)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body %s", w.Code, w.Body.String())
		}
		again := renewedSession(t, w)
		if code := profileStatus(t, f.router, renewed); code != http.StatusUnauthorized {
			t.Errorf("the session that pressed it before answered %d, want 401", code)
		}
		if code := profileStatus(t, f.router, again); code != http.StatusOK {
			t.Errorf("the new renewal answered %d, want 200", code)
		}
	})
}

// The route meters on its own counter at the verification-resend size: the
// request past it is refused, and spending it throttles no other route.
func TestSignOutEverywhereElseRouteHasItsOwnBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Setenv(DisableAuthRateLimitsEnvVar, "")
	f := newSignOutEverywhereElseFixture(t, "sign-out-everywhere-else-budget@test.com")

	const ip = "203.0.113.195:1234"
	session := f.session(t, time.Now())
	for i := 0; i < VerificationResendPerMinute; i++ {
		w := signOutEverywhereElse(t, f.router, ip, session)
		if w.Code != http.StatusOK {
			t.Fatalf("call %d returned %d, want 200; body %s", i+1, w.Code, w.Body.String())
		}
		// Each call revokes the session that made it, so the next goes on
		// with the renewal, as the browser does.
		session = renewedSession(t, w)
	}

	w := signOutEverywhereElse(t, f.router, ip, session)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("call past the budget returned %d, want 429", w.Code)
	}
	if got := w.Header().Get("Retry-After"); got != "60" {
		t.Errorf("429 carried Retry-After %q, want 60", got)
	}

	// Registered after the control's group on rc.Protected: a limiter mounted
	// on rc.Protected rather than on the child group would throttle it here.
	if w := sendAuthed(t, f.router, http.MethodGet, "/auth/preferences/alerts", "", ip, session); w.Code == http.StatusTooManyRequests {
		t.Error("the sign-out budget throttled another protected route; the limiter is not on its own group")
	}
	if code := profileStatus(t, f.router, session); code != http.StatusOK {
		t.Errorf("the refused call changed the caller's session: profile answered %d", code)
	}
}
