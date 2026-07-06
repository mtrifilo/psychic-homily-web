package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"psychic-homily-backend/internal/api/middleware"
)

func TestIsPublicReadRateLimitDisabled(t *testing.T) {
	// Reuses the "==1" flag convention; anything else reads as enabled.
	cases := map[string]bool{"1": true, "": false, "0": false, "true": false, "2": false}
	for val, want := range cases {
		getenv := func(k string) string {
			if k == DisablePublicReadRateLimitsEnvVar {
				return val
			}
			return ""
		}
		if got := IsPublicReadRateLimitDisabled(getenv); got != want {
			t.Errorf("IsPublicReadRateLimitDisabled(%q) = %v, want %v", val, got, want)
		}
	}
}

// Kill-switch on → pass-through noop, even for anonymous traffic well past the limit.
func TestPublicReadRateLimiter_DisabledIsNoop(t *testing.T) {
	mw := PublicReadRateLimiter(nil, func(k string) string {
		if k == DisablePublicReadRateLimitsEnvVar {
			return "1"
		}
		return ""
	})
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < middleware.APIRequestsPerMinute+5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/artists/1/graph-card", nil)
		req.RemoteAddr = "7.7.7.7:100"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want 200 (kill-switch → noop)", i, rr.Code)
		}
	}
}

// /health is exempt — a load-balancer/uptime probe hammering it anonymously
// from one IP must never be 429'd (that would flap the service unhealthy).
func TestPublicReadRateLimiter_HealthPathExempt(t *testing.T) {
	mw := PublicReadRateLimiter(nil, func(string) string { return "" })
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < middleware.APIRequestsPerMinute+5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		req.RemoteAddr = "7.7.7.9:100"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("health probe %d: status = %d, want 200 (/health must be exempt)", i, rr.Code)
		}
	}
}

// Kill-switch off + nil JWT (all requests anonymous) → the limit is enforced:
// APIRequestsPerMinute pass, the next is 429 with Retry-After.
func TestPublicReadRateLimiter_EnabledLimitsAnonymous(t *testing.T) {
	mw := PublicReadRateLimiter(nil, func(string) string { return "" })
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < middleware.APIRequestsPerMinute; i++ {
		req := httptest.NewRequest(http.MethodGet, "/artists/1/graph-card", nil)
		req.RemoteAddr = "7.7.7.8:100"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("request %d within limit: status = %d, want 200", i, rr.Code)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/artists/1/graph-card", nil)
	req.RemoteAddr = "7.7.7.8:100"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("request past limit: status = %d, want 429", rr.Code)
	}
	if rr.Header().Get("Retry-After") != "60" {
		t.Errorf("Retry-After = %q, want 60", rr.Header().Get("Retry-After"))
	}
}
