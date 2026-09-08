package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"psychic-homily-backend/internal/api/middleware"
)

// POST /auth/change-password verifies the current password before setting the
// new one and POST /auth/account/delete verifies it before deactivating the
// account, so each answers "is this the password?" for whoever holds the
// session. These tests pin the budget they share, the escape hatch, and the two
// properties that make the budget safe to add: it is not the public auth
// counter, and an unauthenticated caller cannot spend it.

// limiterAttempt sends one request through a bare limiter chain, with no router
// or handler behind it, so the recorded code is the limiter's own answer.
func limiterAttempt(t *testing.T, limited http.Handler, ip string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/auth/change-password", nil)
	req.RemoteAddr = ip
	limited.ServeHTTP(w, req)
	return w
}

func okHandler(served *int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if served != nil {
			*served++
		}
		w.WriteHeader(http.StatusOK)
	})
}

func TestPasswordConfirmRateLimiter_ThrottlesAfterBudget(t *testing.T) {
	t.Setenv(DisableAuthRateLimitsEnvVar, "")

	var served int
	limited := authScopedRateLimiter(PasswordConfirmAttemptsPerMinute)(okHandler(&served))
	const ip = "203.0.113.61:1234"

	for i := 0; i < PasswordConfirmAttemptsPerMinute; i++ {
		if code := limiterAttempt(t, limited, ip).Code; code != http.StatusOK {
			t.Fatalf("request %d within budget: want 200 got %d", i+1, code)
		}
	}

	w := limiterAttempt(t, limited, ip)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("request past budget: want 429 got %d", w.Code)
	}
	// Retry-After is what the frontend surfaces as ApiError.retryAfter, and the
	// value it renders. Pinned here as well as through the router so a machine
	// without docker still covers it.
	if got := w.Header().Get("Retry-After"); got != "60" {
		t.Errorf("429 carried Retry-After %q, want 60: the client renders this as the wait", got)
	}
	if served != PasswordConfirmAttemptsPerMinute {
		t.Errorf("handler served %d requests, want %d", served, PasswordConfirmAttemptsPerMinute)
	}
}

// The key carries no path component, which is what lets one limiter meter two
// routes: the budget is spent by the pair, not by each. A key that included the
// route would silently give each of them a full budget while every other
// assertion in this file still passed.
func TestPasswordConfirmKeyIgnoresTheRoute(t *testing.T) {
	const ip = "203.0.113.65:1234"

	key := func(path string) string {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, path, nil)
		req.RemoteAddr = ip
		k, err := middleware.KeyByClientIP(req)
		if err != nil {
			t.Fatalf("key for %s: %v", path, err)
		}
		return k
	}

	if change, del := key("/auth/change-password"), key("/auth/account/delete"); change != del {
		t.Fatalf("the two routes key differently from one client (%q vs %q); one limiter cannot meter both", change, del)
	}
}

// A second IP must arrive with a full budget, or one abusive client stops
// everyone else from changing their password or closing their account.
func TestPasswordConfirmRateLimiter_IsPerIP(t *testing.T) {
	t.Setenv(DisableAuthRateLimitsEnvVar, "")

	limited := authScopedRateLimiter(PasswordConfirmAttemptsPerMinute)(okHandler(nil))

	for i := 0; i < PasswordConfirmAttemptsPerMinute; i++ {
		limiterAttempt(t, limited, "198.51.100.61:5000")
	}
	if code := limiterAttempt(t, limited, "198.51.100.61:5000").Code; code != http.StatusTooManyRequests {
		t.Fatalf("first client should be exhausted: want 429 got %d", code)
	}
	if code := limiterAttempt(t, limited, "198.51.100.62:5000").Code; code == http.StatusTooManyRequests {
		t.Error("a second IP was limited by the first client's budget; the limiter is not keyed by client IP")
	}
}

// Every E2E worker shares 127.0.0.1, so a live limiter here would 429 unrelated
// shards.
func TestPasswordConfirmRateLimiter_HonorsDisableFlag(t *testing.T) {
	t.Setenv(DisableAuthRateLimitsEnvVar, "1")

	limited := authScopedRateLimiter(PasswordConfirmAttemptsPerMinute)(okHandler(nil))

	for i := 0; i < PasswordConfirmAttemptsPerMinute*3; i++ {
		if code := limiterAttempt(t, limited, "203.0.113.62:1234").Code; code != http.StatusOK {
			t.Fatalf("request %d with limits disabled: want 200 got %d", i+1, code)
		}
	}
}

// Each authScopedRateLimiter call owns its own counter, so two budgets of the
// same size on the same per-IP key do not drain each other. This is the
// constructor's contract, not the routes': what each mount site was handed is
// checked through the router.
func TestAuthScopedLimitersOfTheSameSizeDoNotShareACounter(t *testing.T) {
	t.Setenv(DisableAuthRateLimitsEnvVar, "")

	passwordConfirm := authScopedRateLimiter(PasswordConfirmAttemptsPerMinute)(okHandler(nil))
	resend := authScopedRateLimiter(VerificationResendPerMinute)(okHandler(nil))
	const ip = "203.0.113.63:1234"

	for i := 0; i < PasswordConfirmAttemptsPerMinute; i++ {
		limiterAttempt(t, passwordConfirm, ip)
	}
	if code := limiterAttempt(t, passwordConfirm, ip).Code; code != http.StatusTooManyRequests {
		t.Fatalf("password-confirm budget should be exhausted: want 429 got %d", code)
	}

	if code := limiterAttempt(t, resend, ip).Code; code == http.StatusTooManyRequests {
		t.Error("verification resend was limited by the password-confirm budget; the two limiters share a counter")
	}
}

// The group hangs off rc.Protected, so HumaJWTMiddleware runs before the
// limiter. Requests without a session are refused at the JWT middleware and
// never reach the counter, which is what stops an anonymous flood from locking
// a real user out of their own password change or account deletion. A 429 here
// would mean the limiter had been mounted ahead of authentication.
func TestPasswordConfirmUnauthenticatedRequestsDoNotSpendTheBudget(t *testing.T) {
	t.Setenv(DisableAuthRateLimitsEnvVar, "")
	router := newTestRouter(t)

	for _, tc := range []struct{ path, ip string }{
		{"/auth/change-password", "203.0.113.64:1234"},
		{"/auth/account/delete", "203.0.113.66:1234"},
	} {
		for i := 0; i < PasswordConfirmAttemptsPerMinute*3; i++ {
			code := send(t, router, "POST", tc.path, tc.ip, nil)
			if code == http.StatusTooManyRequests {
				t.Fatalf("unauthenticated %s request %d returned 429: the limiter runs before authentication, "+
					"so anonymous traffic can exhaust the budget of whoever shares this IP", tc.path, i+1)
			}
			if code != http.StatusUnauthorized {
				t.Fatalf("unauthenticated %s request %d returned %d, want 401", tc.path, i+1, code)
			}
		}
	}
}
