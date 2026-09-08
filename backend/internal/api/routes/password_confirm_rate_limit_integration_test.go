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

// The throttle shared by POST /auth/change-password and POST
// /auth/account/delete, driven through the ROUTER by a caller holding a real
// session.
//
// The budget, the key and the escape hatch are pinned in
// password_confirm_rate_limit_test.go against a limiter that test builds
// itself, so nothing there fails if the middleware is never mounted on a route,
// and the tests there that do drive the router send unauthenticated requests,
// which stop at HumaJWTMiddleware before any counter. Reaching the counter
// takes a session, and a session takes a database.
//
// What this file covers is the MOUNTING of both routes, the reach of the
// mount, and that the two routes draw on ONE counter.

// The account password of the fixture user. Every request in this file submits
// something else, so no request here can delete the account or change its
// password.
const passwordConfirmAccountPassword = "fixture-account-password"

// Request bodies and the handler replies that prove the request got past the
// limiter. The change-password body repeats one value on purpose: the handler
// rejects equal passwords before it reaches the password validator, which
// queries HaveIBeenPwned over the network. Asserting the reply is what stops a
// body the operation never accepted from passing for a request that reached the
// handler.
const (
	changePasswordAttempt = `{"current_password":"same-password-value","new_password":"same-password-value"}`
	changePasswordReached = "New password must be different"
	accountDeleteAttempt  = `{"password":"not-the-account-password"}`
	accountDeleteReached  = "Password is incorrect"
)

// passwordConfirmFixture builds a user carrying a real password hash, a session
// for it, and a router from the live route table.
//
// The hash is load bearing: the delete handler answers an account with no
// password before it compares anything, so a user without one would exercise a
// branch that is not the oracle this budget exists to meter.
func passwordConfirmFixture(t *testing.T, email string) (*chi.Mux, string) {
	t.Helper()

	td := testutil.SetupTestPostgres(t)
	t.Cleanup(td.Cleanup)

	cfg := testConfig()
	sc := services.NewServiceContainer(td.DB, cfg)

	hash, err := sc.User.HashPassword(passwordConfirmAccountPassword)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	user := &authm.User{Email: &email, IsActive: true, EmailVerified: true, PasswordHash: &hash}
	if err := td.DB.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	session, err := sc.JWT.CreateToken(user)
	if err != nil {
		t.Fatalf("create session token: %v", err)
	}

	router := chi.NewRouter()
	SetupRoutes(router, sc, cfg)
	return router, session
}

func sendAuthed(t *testing.T, router *chi.Mux, method, path, body, ip, session string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if session != "" {
		req.Header.Set("Authorization", "Bearer "+session)
	}
	req.RemoteAddr = ip
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// assertPasswordConfirmAttemptLanded requires a response to be the handler's own
// reply to a rejected guess. A 200 with an empty body is the signature of a
// broken humaFromHTTP, which returns without calling next, so the operation
// never runs and every not-429 assertion still passes.
func assertPasswordConfirmAttemptLanded(t *testing.T, w *httptest.ResponseRecorder, path, reached, when string) {
	t.Helper()
	if w.Code == http.StatusUnauthorized {
		t.Fatalf("%s %s returned 401: the session was not accepted, so this test proves nothing", path, when)
	}
	if !strings.Contains(w.Body.String(), reached) {
		t.Fatalf("%s %s did not reach the handler; body: %q", path, when, w.Body.String())
	}
}

// spendBudget sends exactly the budget at one route from one IP and fails if any
// of those requests was limited or failed to reach the handler.
func spendBudget(t *testing.T, router *chi.Mux, session, path, body, reached, ip string) {
	t.Helper()
	for i := 0; i < PasswordConfirmAttemptsPerMinute; i++ {
		w := sendAuthed(t, router, "POST", path, body, ip, session)
		if w.Code == http.StatusTooManyRequests {
			t.Fatalf("%s attempt %d/%d was limited early; the budget is tighter than %d",
				path, i+1, PasswordConfirmAttemptsPerMinute, PasswordConfirmAttemptsPerMinute)
		}
		assertPasswordConfirmAttemptLanded(t, w, path, reached, "within budget")
	}
}

// assertThrottled requires the next request at path to be the limiter's answer,
// carrying the header the client renders as a wait.
func assertThrottled(t *testing.T, router *chi.Mux, session, path, body, ip, why string) {
	t.Helper()
	w := sendAuthed(t, router, "POST", path, body, ip, session)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("%s returned %d past the budget, want 429: %s", path, w.Code, why)
	}
	// Asserted here rather than only on the bare limiter: the header has to
	// survive humaFromHTTP and the protected group's response writer to reach a
	// client, and only a request through the router proves that.
	if got := w.Header().Get("Retry-After"); got != "60" {
		t.Errorf("429 from %s through the router carried Retry-After %q, want 60", path, got)
	}
}

func TestPasswordConfirmRoutesThrottledThroughRouter(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Setenv(DisableAuthRateLimitsEnvVar, "")

	router, session := passwordConfirmFixture(t, "password-confirm-throttle@test.com")

	// Spending the budget at change-password and finding the delete route
	// already throttled is the whole contract in one pass: the delete route is
	// mounted, since an unmounted one would answer with its handler's reply,
	// and the two mounts were handed one counter.
	const changeFirstIP = "198.51.100.90:4444"
	spendBudget(t, router, session, "/auth/change-password", changePasswordAttempt, changePasswordReached, changeFirstIP)
	assertThrottled(t, router, session, "/auth/change-password", changePasswordAttempt, changeFirstIP,
		"the limiter is not mounted on the change-password route")
	assertThrottled(t, router, session, "/auth/account/delete", accountDeleteAttempt, changeFirstIP,
		"the delete route is not on the shared password-confirm counter, so a caller out of "+
			"change-password guesses still gets a full budget of delete guesses")

	// The same contract from the other side, on a fresh counter. Without this
	// direction, dropping change-password from the group would still pass:
	// every assertion above is satisfied by the delete route alone once the
	// change-password requests stop being counted.
	const deleteFirstIP = "198.51.100.92:4444"
	spendBudget(t, router, session, "/auth/account/delete", accountDeleteAttempt, accountDeleteReached, deleteFirstIP)
	assertThrottled(t, router, session, "/auth/account/delete", accountDeleteAttempt, deleteFirstIP,
		"the limiter is not mounted on the account-delete route")
	assertThrottled(t, router, session, "/auth/change-password", changePasswordAttempt, deleteFirstIP,
		"the change-password route is not on the shared password-confirm counter")

	// The reach of the mount. Attaching the middleware to rc.Protected itself
	// rather than to a child group is a one-token edit that would throttle
	// authenticated routes across the app at this budget, and no assertion
	// above can see it. The probe has to be a route registered AFTER the mount
	// point, because a Huma group binds its middleware into an operation when
	// that operation is registered: routes registered earlier would keep
	// answering normally and prove nothing. This one is registered last in
	// setupProtectedAuthRoutes, so reordering the file cannot quietly move it
	// above the mount.
	sibling := sendAuthed(t, router, "GET", "/auth/preferences/alerts", "", changeFirstIP, session)
	if sibling.Code != http.StatusOK {
		t.Errorf("GET /auth/preferences/alerts answered %d with only the password-confirm budget exhausted, want 200: "+
			"a 429 means the limiter is mounted on the protected group rather than on the "+
			"password-confirm group, and any other code means this probe stopped proving that",
			sibling.Code)
	}

	// The neighbouring budget of the same size. Hoisting the
	// authScopedRateLimiter(5) calls in auth.go into one shared variable is the
	// same DRY move this file's own constructor came from, and it would mean
	// five resend clicks throttle a password change. Nothing but a request
	// through the router can see which limiter each mount was handed.
	resend := sendAuthed(t, router, "POST", "/auth/verify-email/send", "{}", changeFirstIP, session)
	if resend.Code == http.StatusTooManyRequests {
		t.Error("POST /auth/verify-email/send returned 429 with only the password-confirm budget exhausted; " +
			"the two mounts were handed one limiter")
	}

	// The ticket's second acceptance criterion. Login is public, so it carries
	// no session here. One request cannot tell a separate counter from a shared
	// one with budget left, so this spends the WHOLE public auth budget from the
	// same IP: on a shared counter the attempts above would have eaten into it
	// and one of these would 429.
	for i := 0; i < authLimitPerMinute; i++ {
		login := sendAuthed(t, router, "POST", "/auth/login",
			`{"email":"password-confirm-throttle@test.com","password":"whatever"}`, changeFirstIP, "")
		if login.Code == http.StatusTooManyRequests {
			t.Fatalf("POST /auth/login %d/%d returned 429 after the password-confirm budget was exhausted from "+
				"the same IP; the password routes share a counter with login", i+1, authLimitPerMinute)
		}
		assertReachedHandler(t, login.Code, login.Body.String(), "/auth/login")
	}
}

// The escape hatch, at the mounts rather than at the constructor. Re-inlining
// httprate.Limit on the password-confirm group would leave every
// constructor-level test green and 429 the E2E shards that share 127.0.0.1,
// which is the regression DISABLE_AUTH_RATE_LIMITS exists to prevent.
func TestPasswordConfirmDisableFlagIsHonoredAtTheRoutes(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Setenv(DisableAuthRateLimitsEnvVar, "1")

	router, session := passwordConfirmFixture(t, "password-confirm-flag@test.com")

	for _, route := range []struct {
		path    string
		body    string
		reached string
		ip      string
	}{
		{"/auth/change-password", changePasswordAttempt, changePasswordReached, "198.51.100.91:4444"},
		{"/auth/account/delete", accountDeleteAttempt, accountDeleteReached, "198.51.100.94:4444"},
	} {
		for i := 0; i < PasswordConfirmAttemptsPerMinute*3; i++ {
			w := sendAuthed(t, router, "POST", route.path, route.body, route.ip, session)
			if w.Code == http.StatusTooManyRequests {
				t.Fatalf("%s request %d returned 429 with %s=1: the route carries a limiter the flag does not reach",
					route.path, i+1, DisableAuthRateLimitsEnvVar)
			}
			assertPasswordConfirmAttemptLanded(t, w, route.path, route.reached, "with limits disabled")
		}
	}
}
