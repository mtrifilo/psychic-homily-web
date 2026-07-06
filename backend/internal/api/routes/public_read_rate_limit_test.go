package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"psychic-homily-backend/internal/api/middleware"
)

func enableEnv(k string) string {
	if k == EnablePublicReadRateLimitsEnvVar {
		return "1"
	}
	return ""
}

func TestIsPublicReadRateLimitEnabled(t *testing.T) {
	// Opt-in: only "1" enables; anything else (incl. unset) stays off.
	cases := map[string]bool{"1": true, "": false, "0": false, "true": false, "2": false}
	for val, want := range cases {
		getenv := func(k string) string {
			if k == EnablePublicReadRateLimitsEnvVar {
				return val
			}
			return ""
		}
		if got := IsPublicReadRateLimitEnabled(getenv); got != want {
			t.Errorf("IsPublicReadRateLimitEnabled(%q) = %v, want %v", val, got, want)
		}
	}
}

// Default (flag unset) → pass-through noop, even for anonymous reads past the
// limit. This is what keeps CI/E2E and a fresh prod deploy unthrottled until the
// limiter is deliberately enabled per environment (stage-first rollout).
func TestPublicReadRateLimiter_NotEnabledIsNoop(t *testing.T) {
	mw := PublicReadRateLimiter(nil, func(string) string { return "" })
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < middleware.APIRequestsPerMinute+5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/artists/1/graph-card", nil)
		req.RemoteAddr = "7.7.7.7:100"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want 200 (disabled → noop)", i, rr.Code)
		}
	}
}

// Enabled + nil JWT (all requests anonymous) → anonymous READS are limited:
// APIRequestsPerMinute pass, the next is 429 with Retry-After.
func TestPublicReadRateLimiter_EnabledLimitsAnonymousReads(t *testing.T) {
	mw := PublicReadRateLimiter(nil, enableEnv)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < middleware.APIRequestsPerMinute; i++ {
		req := httptest.NewRequest(http.MethodGet, "/artists/1/graph-card", nil)
		req.RemoteAddr = "7.7.7.8:100"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("read %d within limit: status = %d, want 200", i, rr.Code)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/artists/1/graph-card", nil)
	req.RemoteAddr = "7.7.7.8:100"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("read past limit: status = %d, want 429", rr.Code)
	}
	if rr.Header().Get("Retry-After") != "60" {
		t.Errorf("Retry-After = %q, want 60", rr.Header().Get("Retry-After"))
	}
}

// Writes (non-GET/HEAD) are NOT limited here — they keep their own dedicated
// limiters, so a shared read budget can't 429 an anonymous write.
func TestPublicReadRateLimiter_WritesNotLimited(t *testing.T) {
	mw := PublicReadRateLimiter(nil, enableEnv)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < middleware.APIRequestsPerMinute+5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
		req.RemoteAddr = "7.7.7.10:100"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("anonymous write %d: status = %d, want 200 (writes bypass the read limiter)", i, rr.Code)
		}
	}
}

// /health is exempt — a load-balancer/uptime probe hammering it anonymously
// from one IP must never be 429'd (that would flap the service unhealthy).
func TestPublicReadRateLimiter_HealthPathExempt(t *testing.T) {
	mw := PublicReadRateLimiter(nil, enableEnv)
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
