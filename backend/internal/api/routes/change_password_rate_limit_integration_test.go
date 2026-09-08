package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services"
	"psychic-homily-backend/internal/testutil"
)

// The throttle on POST /auth/change-password, driven through the ROUTER by a
// caller holding a real session.
//
// The budget, the key and the escape hatch are pinned in
// change_password_rate_limit_test.go against a limiter that test builds itself,
// so nothing there fails if the middleware is never mounted on the route, and
// the one test in that file that does drive the router sends unauthenticated
// requests, which stop at HumaJWTMiddleware before any counter. Reaching the
// counter takes a session, and a session takes a database.
//
// What this covers is the MOUNTING and the reach of the mount, not the
// credential path: the requests below never get as far as comparing a
// password.
func TestChangePasswordThrottledThroughRouter(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Setenv(DisableAuthRateLimitsEnvVar, "")

	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	cfg := testConfig()
	sc := services.NewServiceContainer(td.DB, cfg)

	email := "change-password-throttle@test.com"
	user := &authm.User{Email: &email, IsActive: true, EmailVerified: true}
	if err := td.DB.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	session, err := sc.JWT.CreateToken(user)
	if err != nil {
		t.Fatalf("create session token: %v", err)
	}

	router := chi.NewRouter()
	SetupRoutes(router, sc, cfg)

	// The two passwords are equal on purpose: the handler rejects that before it
	// reaches the password validator, which queries HaveIBeenPwned over the
	// network. The reply it gives is asserted below, so a body the operation
	// never accepted cannot pass for a request that reached the handler.
	const attempt = `{"current_password":"same-password-value","new_password":"same-password-value"}`
	const reachedHandler = "New password must be different"

	send := func(t *testing.T, method, path, body, ip string, authenticate bool) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if authenticate {
			req.Header.Set("Authorization", "Bearer "+session)
		}
		req.RemoteAddr = ip
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	const ip = "198.51.100.90:4444"

	for i := 0; i < ChangePasswordAttemptsPerMinute; i++ {
		w := send(t, "POST", "/auth/change-password", attempt, ip, true)
		if w.Code == http.StatusTooManyRequests {
			t.Fatalf("attempt %d/%d was limited early; the budget is tighter than %d",
				i+1, ChangePasswordAttemptsPerMinute, ChangePasswordAttemptsPerMinute)
		}
		if w.Code == http.StatusUnauthorized {
			t.Fatalf("attempt %d returned 401: the session was not accepted, so this test proves nothing", i+1)
		}
		if !strings.Contains(w.Body.String(), reachedHandler) {
			t.Fatalf("attempt %d did not reach the handler; body: %s", i+1, w.Body.String())
		}
	}

	w := send(t, "POST", "/auth/change-password", attempt, ip, true)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("attempt %d returned %d, want 429: the limiter is not mounted on the route",
			ChangePasswordAttemptsPerMinute+1, w.Code)
	}
	// Asserted here rather than only on the bare limiter: the header has to
	// survive humaFromHTTP and the protected group's response writer to reach a
	// client, and only a request through the router proves that.
	if got := w.Header().Get("Retry-After"); got != "60" {
		t.Errorf("429 through the router carried Retry-After %q, want 60", got)
	}

	// The reach of the mount. Attaching the middleware to rc.Protected itself
	// rather than to a child group is a one-token edit that would throttle
	// authenticated routes across the app at this budget, and no assertion
	// above can see it. The probe has to be a route registered AFTER the mount
	// point, because a Huma group binds its middleware into an operation when
	// that operation is registered: routes registered earlier would keep
	// answering normally and prove nothing.
	sibling := send(t, "GET", "/auth/account/deletion-summary", "", ip, true)
	if sibling.Code == http.StatusTooManyRequests {
		t.Error("GET /auth/account/deletion-summary returned 429 with only change-password exhausted; " +
			"the limiter is mounted on the protected group rather than on the change-password group")
	}

	// The ticket's second acceptance criterion. Login is public, so it carries
	// no session here; what matters is that the counter it draws on is not the
	// one just exhausted.
	login := send(t, "POST", "/auth/login",
		`{"email":"change-password-throttle@test.com","password":"whatever"}`, ip, false)
	if login.Code == http.StatusTooManyRequests {
		t.Error("POST /auth/login returned 429 after change-password was exhausted from the same IP; " +
			"the two routes share a counter")
	}
	assertReachedHandler(t, login.Code, login.Body.String(), "/auth/login")
}
