package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/httprate"
)

func TestRateLimitExceededHandler_StatusCode(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	rr := httptest.NewRecorder()

	RateLimitExceededHandler(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("status code = %d, want %d", rr.Code, http.StatusTooManyRequests)
	}
}

func TestRateLimitExceededHandler_ContentType(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	rr := httptest.NewRecorder()

	RateLimitExceededHandler(rr, req)

	if got := rr.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
}

func TestRateLimitExceededHandler_RetryAfter(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	rr := httptest.NewRecorder()

	RateLimitExceededHandler(rr, req)

	if got := rr.Header().Get("Retry-After"); got != "60" {
		t.Errorf("Retry-After = %q, want 60", got)
	}
}

func TestRateLimitExceededHandler_Body(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	rr := httptest.NewRecorder()

	RateLimitExceededHandler(rr, req)

	var body map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse response body as JSON: %v", err)
	}

	if body["success"] != false {
		t.Errorf("success = %v, want false", body["success"])
	}
	if body["error"] != "too_many_requests" {
		t.Errorf("error = %v, want too_many_requests", body["error"])
	}
	msg, ok := body["message"].(string)
	if !ok || msg == "" {
		t.Error("expected non-empty message field")
	}
}

func TestRateLimitAuthEndpoints_ReturnsMiddleware(t *testing.T) {
	mw := RateLimitAuthEndpoints()
	if mw == nil {
		t.Fatal("RateLimitAuthEndpoints() returned nil")
	}
}

func TestRateLimitPasskeyEndpoints_ReturnsMiddleware(t *testing.T) {
	mw := RateLimitPasskeyEndpoints()
	if mw == nil {
		t.Fatal("RateLimitPasskeyEndpoints() returned nil")
	}
}

func TestRateLimitAPIEndpoints_ReturnsMiddleware(t *testing.T) {
	mw := RateLimitAPIEndpoints()
	if mw == nil {
		t.Fatal("RateLimitAPIEndpoints() returned nil")
	}
}

func TestRateLimitTagCreateEndpoints_ReturnsMiddleware(t *testing.T) {
	mw := RateLimitTagCreateEndpoints()
	if mw == nil {
		t.Fatal("RateLimitTagCreateEndpoints() returned nil")
	}
}

func TestRateLimitTagVoteEndpoints_ReturnsMiddleware(t *testing.T) {
	mw := RateLimitTagVoteEndpoints()
	if mw == nil {
		t.Fatal("RateLimitTagVoteEndpoints() returned nil")
	}
}

// PSY-345: nil JWTService should fall through to the underlying limiter for
// every request. Non-admin/unauthenticated paths get rate-limited as before.
func TestSkipRateLimitForAdmin_NilJWTServiceLimitsEveryRequest(t *testing.T) {
	// 1 request / minute limiter, easy to saturate within the test.
	base := httprate.Limit(1, time.Minute, httprate.WithKeyFuncs(httprate.KeyByIP))
	mw := SkipRateLimitForAdmin(nil, base)

	hits := 0
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
	}))

	// First request passes
	req1 := httptest.NewRequest(http.MethodPost, "/tag", nil)
	req1.RemoteAddr = "1.2.3.4:1000"
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Fatalf("first request: status = %d, want 200", rr1.Code)
	}

	// Second request from the same IP hits the limiter
	req2 := httptest.NewRequest(http.MethodPost, "/tag", nil)
	req2.RemoteAddr = "1.2.3.4:1001"
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusTooManyRequests {
		t.Errorf("second request: status = %d, want 429 (limiter should apply when JWTService is nil)", rr2.Code)
	}
	if hits != 1 {
		t.Errorf("handler hits = %d, want 1 (second call should be short-circuited by limiter)", hits)
	}
}

// PSY-345: extractJWT picks up the Bearer header when present, falls back to
// the auth_token cookie, and returns empty string otherwise.
func TestExtractJWT_PrefersAuthorizationHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer header-token")
	req.AddCookie(&http.Cookie{Name: "auth_token", Value: "cookie-token"})

	got := extractJWT(req)
	if got != "header-token" {
		t.Errorf("extractJWT = %q, want %q (header takes precedence)", got, "header-token")
	}
}

func TestExtractJWT_FallsBackToCookie(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "auth_token", Value: "cookie-token"})

	got := extractJWT(req)
	if got != "cookie-token" {
		t.Errorf("extractJWT = %q, want %q", got, "cookie-token")
	}
}

func TestExtractJWT_EmptyWhenNoToken(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if got := extractJWT(req); got != "" {
		t.Errorf("extractJWT = %q, want empty string", got)
	}
}

func TestExtractJWT_IgnoresNonBearerAuthHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	// Basic auth, not Bearer — should not be treated as a JWT.
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")

	if got := extractJWT(req); got != "" {
		t.Errorf("extractJWT = %q, want empty string for non-Bearer header", got)
	}
}
