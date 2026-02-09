package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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
