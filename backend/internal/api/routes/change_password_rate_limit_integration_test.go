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
// The limiter-only tests in change_password_rate_limit_test.go prove the
// budget, the key and the escape hatch, but they build the limiter themselves,
// so they stay green if the middleware is never mounted on the route. Only an
// authenticated request reaches it: HumaJWTMiddleware runs first and answers
// every anonymous request with 401, so no unauthenticated test can see the
// counter at all. That takes a session, which takes a database.
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

	// The two passwords are equal on purpose: the handler rejects that before
	// it reaches the password validator, which queries HaveIBeenPwned over the
	// network. What matters here is that the request reached the handler at
	// all, not what the handler decided.
	const attempt = `{"current_password":"same-password-value","new_password":"same-password-value"}`

	post := func(t *testing.T, path, body, ip string) (int, string) {
		t.Helper()
		req := httptest.NewRequest("POST", path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+session)
		req.RemoteAddr = ip
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w.Code, w.Body.String()
	}

	const ip = "198.51.100.90:4444"

	for i := 0; i < ChangePasswordAttemptsPerMinute; i++ {
		code, body := post(t, "/auth/change-password", attempt, ip)
		if code == http.StatusTooManyRequests {
			t.Fatalf("attempt %d/%d was limited early; the budget is tighter than %d",
				i+1, ChangePasswordAttemptsPerMinute, ChangePasswordAttemptsPerMinute)
		}
		if code == http.StatusUnauthorized {
			t.Fatalf("attempt %d returned 401: the session was not accepted, so this test proves nothing", i+1)
		}
		assertReachedHandler(t, code, body, "/auth/change-password")
	}

	code, body := post(t, "/auth/change-password", attempt, ip)
	if code != http.StatusTooManyRequests {
		t.Fatalf("attempt %d returned %d, want 429: the limiter is not mounted on the route",
			ChangePasswordAttemptsPerMinute+1, code)
	}
	if got := w429RetryAfter(body); got == "" {
		t.Errorf("the 429 body does not name a wait: %s", body)
	}

	// The same session, the same IP, a different budget. This is the ticket's
	// second acceptance criterion, and only the router can answer it: the two
	// limiters are separate objects by construction, but nothing else proves
	// login is not ALSO behind the change-password one.
	loginCode, loginBody := post(t, "/auth/login",
		`{"email":"change-password-throttle@test.com","password":"whatever"}`, ip)
	if loginCode == http.StatusTooManyRequests {
		t.Error("POST /auth/login returned 429 after change-password was exhausted from the same IP; " +
			"the two routes share a counter")
	}
	assertReachedHandler(t, loginCode, loginBody, "/auth/login")
}

// w429RetryAfter returns the wait named in a rate-limit body, or "" if the body
// does not carry one. The header is asserted in the limiter test; this checks
// the body a client actually reads when CORS hides the header.
func w429RetryAfter(body string) string {
	const marker = "try again in "
	i := strings.Index(strings.ToLower(body), marker)
	if i < 0 {
		return ""
	}
	return body[i+len(marker):]
}
