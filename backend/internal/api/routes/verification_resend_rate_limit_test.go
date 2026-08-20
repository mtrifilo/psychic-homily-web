package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestVerificationResendRateLimiter_ThrottlesAfterBudget pins the PSY-1871
// throttle on POST /auth/verify-email/send. Signup emails the verification link
// automatically and the /shows/submit gate puts a resend button in front of
// every blocked user, so an unthrottled endpoint is an inbox-bombing and
// Resend-quota vector for any authenticated caller.
func TestVerificationResendRateLimiter_ThrottlesAfterBudget(t *testing.T) {
	t.Setenv("DISABLE_AUTH_RATE_LIMITS", "")

	var served int
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		served++
		w.WriteHeader(http.StatusOK)
	})
	wrapped := verificationResendRateLimiter()(next)

	for i := 0; i < VerificationResendPerMinute; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/auth/verify-email/send", nil)
		req.RemoteAddr = "203.0.113.7:1234"
		wrapped.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d within budget: want 200 got %d", i+1, w.Code)
		}
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/auth/verify-email/send", nil)
	req.RemoteAddr = "203.0.113.7:1234"
	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("request past budget: want 429 got %d", w.Code)
	}
	// Retry-After is what the frontend surfaces as ApiError.retryAfter.
	if got := w.Header().Get("Retry-After"); got == "" {
		t.Error("429 must carry Retry-After so the client can render a wait")
	}
	if served != VerificationResendPerMinute {
		t.Errorf("handler served %d requests, want %d", served, VerificationResendPerMinute)
	}
}

// TestVerificationResendRateLimiter_HonorsDisableFlag guards the E2E path: all
// workers share 127.0.0.1, so a live limiter here would 429 unrelated shards.
func TestVerificationResendRateLimiter_HonorsDisableFlag(t *testing.T) {
	t.Setenv("DISABLE_AUTH_RATE_LIMITS", "1")

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	wrapped := verificationResendRateLimiter()(next)

	for i := 0; i < VerificationResendPerMinute*3; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/auth/verify-email/send", nil)
		req.RemoteAddr = "203.0.113.8:1234"
		wrapped.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d with limits disabled: want 200 got %d", i+1, w.Code)
		}
	}
}
