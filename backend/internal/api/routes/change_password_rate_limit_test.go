package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// POST /auth/change-password verifies the current password before setting the
// new one, so it answers "is this the password?" for whoever holds the session.
// These tests pin the budget, the escape hatch, and the two properties that
// make the budget safe to add: it is not the public auth counter, and an
// unauthenticated caller cannot spend it.

func TestChangePasswordRateLimiter_ThrottlesAfterBudget(t *testing.T) {
	t.Setenv(DisableAuthRateLimitsEnvVar, "")

	var served int
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		served++
		w.WriteHeader(http.StatusOK)
	})
	wrapped := changePasswordRateLimiter()(next)

	for i := 0; i < ChangePasswordAttemptsPerMinute; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/auth/change-password", nil)
		req.RemoteAddr = "203.0.113.61:1234"
		wrapped.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d within budget: want 200 got %d", i+1, w.Code)
		}
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/auth/change-password", nil)
	req.RemoteAddr = "203.0.113.61:1234"
	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("request past budget: want 429 got %d", w.Code)
	}
	// Retry-After is what the frontend surfaces as ApiError.retryAfter.
	if got := w.Header().Get("Retry-After"); got == "" {
		t.Error("429 must carry Retry-After so the client can render a wait")
	}
	if served != ChangePasswordAttemptsPerMinute {
		t.Errorf("handler served %d requests, want %d", served, ChangePasswordAttemptsPerMinute)
	}
}

// A second IP must arrive with a full budget, or one abusive client stops
// everyone else from changing their password.
func TestChangePasswordRateLimiter_IsPerIP(t *testing.T) {
	t.Setenv(DisableAuthRateLimitsEnvVar, "")

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	wrapped := changePasswordRateLimiter()(next)

	for i := 0; i <= ChangePasswordAttemptsPerMinute; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/auth/change-password", nil)
		req.RemoteAddr = "198.51.100.61:5000"
		wrapped.ServeHTTP(w, req)
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/auth/change-password", nil)
	req.RemoteAddr = "198.51.100.61:5000"
	wrapped.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("first client should be exhausted: want 429 got %d", w.Code)
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/auth/change-password", nil)
	req.RemoteAddr = "198.51.100.62:5000"
	wrapped.ServeHTTP(w, req)
	if w.Code == http.StatusTooManyRequests {
		t.Error("a second IP was limited by the first client's budget; the limiter is not keyed by client IP")
	}
}

// Every E2E worker shares 127.0.0.1, so a live limiter here would 429 unrelated
// shards.
func TestChangePasswordRateLimiter_HonorsDisableFlag(t *testing.T) {
	t.Setenv(DisableAuthRateLimitsEnvVar, "1")

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	wrapped := changePasswordRateLimiter()(next)

	for i := 0; i < ChangePasswordAttemptsPerMinute*3; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/auth/change-password", nil)
		req.RemoteAddr = "203.0.113.62:1234"
		wrapped.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d with limits disabled: want 200 got %d", i+1, w.Code)
		}
	}
}

// Each constructor call owns its own counter, so spending one route's budget
// leaves the other's untouched. Both routes sit behind the same session and the
// same per-IP key, which is exactly the shape in which a shared store would go
// unnoticed.
func TestChangePasswordAndVerificationResendDoNotShareABudget(t *testing.T) {
	t.Setenv(DisableAuthRateLimitsEnvVar, "")

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	changePassword := changePasswordRateLimiter()(next)
	resend := verificationResendRateLimiter()(next)

	const ip = "203.0.113.63:1234"
	for i := 0; i <= ChangePasswordAttemptsPerMinute; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/auth/change-password", nil)
		req.RemoteAddr = ip
		changePassword.ServeHTTP(w, req)
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/auth/change-password", nil)
	req.RemoteAddr = ip
	changePassword.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("change-password budget should be exhausted: want 429 got %d", w.Code)
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/auth/verify-email/send", nil)
	req.RemoteAddr = ip
	resend.ServeHTTP(w, req)
	if w.Code == http.StatusTooManyRequests {
		t.Error("verification resend was limited by the change-password budget; the two limiters share a counter")
	}
}

// The group hangs off rc.Protected, so HumaJWTMiddleware runs before the
// limiter. Requests without a session are refused at the JWT middleware and
// never reach the counter, which is what stops an anonymous flood from locking
// a real user out of their own password change. A 429 here would mean the
// limiter had been mounted ahead of authentication.
func TestChangePasswordUnauthenticatedRequestsDoNotSpendTheBudget(t *testing.T) {
	t.Setenv(DisableAuthRateLimitsEnvVar, "")
	router := newTestRouter(t)
	const ip = "203.0.113.64:1234"

	for i := 0; i < ChangePasswordAttemptsPerMinute*3; i++ {
		code := send(t, router, "POST", "/auth/change-password", ip, nil)
		if code == http.StatusTooManyRequests {
			t.Fatalf("unauthenticated request %d returned 429: the limiter runs before authentication, "+
				"so anonymous traffic can exhaust the budget of whoever shares this IP", i+1)
		}
		if code != http.StatusUnauthorized {
			t.Fatalf("unauthenticated request %d returned %d, want 401", i+1, code)
		}
	}
}

// The public auth counter is what /auth/login draws on. Change-password must
// not share it, or a run of failed attempts would lock the same person out of
// signing in.
func TestChangePasswordDoesNotSpendThePublicAuthBudget(t *testing.T) {
	t.Setenv(DisableAuthRateLimitsEnvVar, "")
	router := newTestRouter(t)
	const ip = "203.0.113.65:1234"

	for i := 0; i < authLimitPerMinute*2; i++ {
		send(t, router, "POST", "/auth/change-password", ip, nil)
	}

	code, body := sendWithBody(t, router, "POST", "/auth/login", ip, nil)
	if code == http.StatusTooManyRequests {
		t.Error("POST /auth/login was limited after change-password attempts from the same IP; " +
			"the change-password route is on the public auth counter")
	}
	assertReachedHandler(t, code, body, "/auth/login")
}

// The route keeps its place in the one published document. A group conversion
// that dropped it from the spec would leave the endpoint reachable and
// undocumented, which is the failure PSY-1598 exists to prevent.
func TestChangePasswordIsInMainSpec(t *testing.T) {
	paths := servedSpecPaths(t, newTestRouter(t))

	item, ok := paths["/auth/change-password"]
	if !ok {
		t.Fatal("/auth/change-password is missing from the served spec")
	}
	if _, ok := item["post"]; !ok {
		t.Error("expected a documented POST operation for /auth/change-password")
	}
}
