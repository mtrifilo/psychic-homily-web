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
	// answering normally and prove nothing. This one is registered last in
	// setupProtectedAuthRoutes, so reordering the file cannot quietly move it
	// above the mount.
	sibling := send(t, "GET", "/auth/preferences/alerts", "", ip, true)
	if sibling.Code != http.StatusOK {
		t.Errorf("GET /auth/preferences/alerts answered %d with only change-password exhausted, want 200: "+
			"a 429 means the limiter is mounted on the protected group rather than on the "+
			"change-password group, and any other code means this probe stopped proving that",
			sibling.Code)
	}

	// The neighbouring budget of the same size. Hoisting the three
	// authScopedRateLimiter(5) calls in auth.go into one shared variable is the
	// same DRY move this file's own constructor came from, and it would mean
	// five resend clicks throttle a password change. Nothing but a request
	// through the router can see which limiter each mount was handed.
	resend := send(t, "POST", "/auth/verify-email/send", "{}", ip, true)
	if resend.Code == http.StatusTooManyRequests {
		t.Error("POST /auth/verify-email/send returned 429 with only change-password exhausted; " +
			"the two mounts were handed one limiter")
	}

	// The ticket's second acceptance criterion. Login is public, so it carries
	// no session here. One request cannot tell a separate counter from a
	// shared one with budget left, so this spends the WHOLE public auth budget
	// from the same IP: on a shared counter the change-password attempts above
	// would have eaten into it and one of these would 429.
	for i := 0; i < authLimitPerMinute; i++ {
		login := send(t, "POST", "/auth/login",
			`{"email":"change-password-throttle@test.com","password":"whatever"}`, ip, false)
		if login.Code == http.StatusTooManyRequests {
			t.Fatalf("POST /auth/login %d/%d returned 429 after change-password was exhausted from "+
				"the same IP; the two routes share a counter", i+1, authLimitPerMinute)
		}
		assertReachedHandler(t, login.Code, login.Body.String(), "/auth/login")
	}
}

// The escape hatch, at the mount rather than at the constructor. Re-inlining
// httprate.Limit on changePasswordGroup would leave every constructor-level
// test green and 429 the E2E shards that share 127.0.0.1, which is the
// regression DISABLE_AUTH_RATE_LIMITS exists to prevent.
//
// The empty-body check matters as much as the absence of a 429: humaFromHTTP
// signals "allowed" by watching a sentinel handler run, and if that detection
// breaks it returns without calling next, so Huma emits a bare 200 and the
// password is silently never changed.
func TestChangePasswordDisableFlagIsHonoredAtTheRoute(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Setenv(DisableAuthRateLimitsEnvVar, "1")

	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	cfg := testConfig()
	sc := services.NewServiceContainer(td.DB, cfg)

	email := "change-password-flag@test.com"
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

	for i := 0; i < ChangePasswordAttemptsPerMinute*3; i++ {
		req := httptest.NewRequest("POST", "/auth/change-password",
			strings.NewReader(`{"current_password":"same-password-value","new_password":"same-password-value"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+session)
		req.RemoteAddr = "198.51.100.91:4444"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code == http.StatusTooManyRequests {
			t.Fatalf("request %d returned 429 with %s=1: the route carries a limiter the flag does not reach",
				i+1, DisableAuthRateLimitsEnvVar)
		}
		assertReachedHandler(t, w.Code, w.Body.String(), "/auth/change-password")
	}
}
