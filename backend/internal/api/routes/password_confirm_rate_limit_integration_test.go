package routes

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services"
	"psychic-homily-backend/internal/testutil"
)

// The throttle shared by the routes on passwordConfirmGroup, driven through the
// ROUTER by a caller holding a real session.
//
// The budget, the key and the escape hatch are pinned in
// password_confirm_rate_limit_test.go against a limiter that test builds
// itself, so nothing there fails if the middleware is never mounted on a route,
// and the tests there that do drive the router send unauthenticated requests,
// which stop at HumaJWTMiddleware before any counter. Reaching the counter takes
// a session, and a session takes a database.
//
// What this file covers is the MOUNTING of every member, the reach of the
// mount, and that the members draw on ONE counter.

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

	router := chi.NewRouter()
	SetupRoutes(router, sc, cfg)
	return router, mintToken(t, sc, user)
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
func assertPasswordConfirmAttemptLanded(t *testing.T, w *httptest.ResponseRecorder, path, reached string) {
	t.Helper()
	if w.Code == http.StatusUnauthorized {
		t.Fatalf("%s returned 401: the session was not accepted, so this test proves nothing", path)
	}
	if !strings.Contains(w.Body.String(), reached) {
		t.Fatalf("%s did not reach the handler; body: %q", path, w.Body.String())
	}
}

// spendBudget sends exactly the budget at one route from one IP and fails if any
// of those requests was limited or failed to reach the handler.
func spendBudget(t *testing.T, router *chi.Mux, session, ip string, route passwordConfirmRoute) {
	t.Helper()
	for i := 0; i < PasswordConfirmAttemptsPerMinute; i++ {
		w := sendAuthed(t, router, "POST", route.path, route.body, ip, session)
		if w.Code == http.StatusTooManyRequests {
			t.Fatalf("%s attempt %d/%d was limited early; the budget is tighter than %d",
				route.path, i+1, PasswordConfirmAttemptsPerMinute, PasswordConfirmAttemptsPerMinute)
		}
		assertPasswordConfirmAttemptLanded(t, w, route.path, route.reached)
	}
}

// assertThrottled requires the next request at a route to be the limiter's
// answer, carrying the header the client renders as a wait.
func assertThrottled(t *testing.T, router *chi.Mux, session, ip, why string, route passwordConfirmRoute) {
	t.Helper()
	w := sendAuthed(t, router, "POST", route.path, route.body, ip, session)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("%s returned %d past the budget, want 429: %s", route.path, w.Code, why)
	}
	// Asserted here rather than only on the bare limiter: the header has to
	// survive humaFromHTTP and the protected group's response writer to reach a
	// client, and only a request through the router proves that.
	if got := w.Header().Get("Retry-After"); got != "60" {
		t.Errorf("429 from %s through the router carried Retry-After %q, want 60", route.path, got)
	}
}

func TestPasswordConfirmRoutesThrottledThroughRouter(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Setenv(DisableAuthRateLimitsEnvVar, "")

	router, session := passwordConfirmFixture(t, "password-confirm-throttle@test.com")

	// Every member spends the budget once, from a counter of its own, and every
	// OTHER member must then already be throttled. Run per member rather than
	// once, because a single direction is satisfied by whichever route is still
	// mounted: dropping any one route from the group leaves the pass on the
	// pass it did not drive.
	for i, spender := range passwordConfirmBudgetRoutes {
		ip := fmt.Sprintf("198.51.100.%d:4444", 90+i)

		spendBudget(t, router, session, ip, spender)
		assertThrottled(t, router, session, ip,
			"the limiter is not mounted on this route", spender)

		for _, sibling := range passwordConfirmBudgetRoutes {
			if sibling.path == spender.path {
				continue
			}
			assertThrottled(t, router, session, ip,
				"this route is not on the shared password-confirm counter, so a caller out of "+
					spender.path+" guesses still gets a full budget here", sibling)
		}
	}

	// The reach of the mount. Attaching the middleware to rc.Protected itself
	// rather than to a child group is a one-token edit that would throttle
	// authenticated routes across the app at this budget, and no assertion
	// above can see it. The probe has to be a route registered AFTER the mount
	// point, because a Huma group binds its middleware into an operation when
	// that operation is registered: routes registered earlier would keep
	// answering normally and prove nothing. That ordering is the premise this
	// assertion rests on, and TestPasswordConfirmOverReachProbeIsRegisteredAfterTheGroup
	// fails when an edit to auth.go breaks it.
	const probeIP = "198.51.100.90:4444"
	sibling := sendAuthed(t, router, "GET", "/auth/preferences/alerts", "", probeIP, session)
	if sibling.Code != http.StatusOK {
		t.Errorf("GET /auth/preferences/alerts answered %d with only the password-confirm budget exhausted, want 200: "+
			"a 429 means the limiter is mounted on the protected group rather than on the "+
			"password-confirm group, and any other code means this probe stopped proving that",
			sibling.Code)
	}

	// The read the dialog makes when it opens. It takes no password, so it is on
	// rc.Protected rather than on the group; moving it onto the group would mean
	// merely opening the deletion dialog spends a password guess, and the
	// inventory sweep cannot see it because it sweeps mutating methods only.
	summary := sendAuthed(t, router, "GET", "/auth/account/deletion-summary", "", probeIP, session)
	if summary.Code != http.StatusOK {
		t.Errorf("GET /auth/account/deletion-summary answered %d with the password-confirm budget exhausted, "+
			"want 200: the summary read is on the password-confirm group, so opening the deletion dialog "+
			"now costs a password guess", summary.Code)
	}

	// The neighbouring budget of the same size. Hoisting the
	// authScopedRateLimiter(5) calls in auth.go into one shared variable is the
	// same DRY move this file's own constructor came from, and it would mean
	// five resend clicks throttle a password change. Nothing but a request
	// through the router can see which limiter each mount was handed.
	resend := sendAuthed(t, router, "POST", "/auth/verify-email/send", "{}", probeIP, session)
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
			`{"email":"password-confirm-throttle@test.com","password":"whatever"}`, probeIP, "")
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

	for i, route := range passwordConfirmBudgetRoutes {
		ip := fmt.Sprintf("198.51.100.%d:4444", 120+i)
		// One past the budget is the whole assertion: that request is the first
		// one a live limiter would refuse. Each further attempt costs a bcrypt
		// comparison and proves nothing more.
		for attempt := 0; attempt <= PasswordConfirmAttemptsPerMinute; attempt++ {
			w := sendAuthed(t, router, "POST", route.path, route.body, ip, session)
			if w.Code == http.StatusTooManyRequests {
				t.Fatalf("%s request %d returned 429 with %s=1: the route carries a limiter the flag does not reach",
					route.path, attempt+1, DisableAuthRateLimitsEnvVar)
			}
			assertPasswordConfirmAttemptLanded(t, w, route.path, route.reached)
		}
	}
}
