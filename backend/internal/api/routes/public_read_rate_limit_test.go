package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/config"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/auth"
	usersvc "psychic-homily-backend/internal/services/user"
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

// The batch save-count endpoint is a READ that carries its show IDs in a POST
// body. It must share the anonymous read budget rather than slip through the
// GET/HEAD filter — otherwise it is an unmetered aggregate query over
// user_bookmarks for any anonymous caller.
func TestPublicReadRateLimiter_LimitsReadViaPostBatch(t *testing.T) {
	mw := PublicReadRateLimiter(nil, enableEnv)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < middleware.APIRequestsPerMinute; i++ {
		req := httptest.NewRequest(http.MethodPost, SaveCountsBatchPath, nil)
		req.RemoteAddr = "7.7.7.20:100"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("batch read %d within limit: status = %d, want 200", i, rr.Code)
		}
	}

	req := httptest.NewRequest(http.MethodPost, SaveCountsBatchPath, nil)
	req.RemoteAddr = "7.7.7.20:100"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("batch read past limit: status = %d, want 429 (read-via-POST must be metered)", rr.Code)
	}
}

// A genuine write on a path that merely LOOKS adjacent must still bypass the
// read budget — the allowlist is exact-match, not a prefix.
func TestPublicReadRateLimiter_SaveWriteNotLimited(t *testing.T) {
	mw := PublicReadRateLimiter(nil, enableEnv)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < middleware.APIRequestsPerMinute+5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/saved-shows/42", nil)
		req.RemoteAddr = "7.7.7.21:100"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("save write %d: status = %d, want 200 (writes bypass the read limiter)", i, rr.Code)
		}
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

// PSY-1373: an authenticated user is routed to the per-USER cap
// (PublicReadUserRequestsPerMinute), which is higher than the anonymous per-IP
// cap — so it passes well past the anonymous limit instead of 429-ing at it.
func TestPublicReadRateLimiter_AuthenticatedUsesPerUserCap(t *testing.T) {
	cfg := &config.Config{JWT: config.JWTConfig{SecretKey: "test-secret-key-for-routes-unit-32c", Expiry: 24}}
	jwtService := auth.NewJWTService(nil, cfg, usersvc.NewUserService(nil))
	user := &authm.User{ID: 7}
	token, err := jwtService.CreateToken(user)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	mw := PublicReadRateLimiter(jwtService, enableEnv)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Send more requests than the anonymous cap would allow; all pass because the
	// authenticated user is on the higher per-user bucket, not the per-IP one.
	for i := 0; i < middleware.APIRequestsPerMinute+50; i++ {
		req := httptest.NewRequest(http.MethodGet, "/artists/1/graph-card", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		req.RemoteAddr = "7.7.7.11:100"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("authenticated request %d (past anon cap): status = %d, want 200 (per-user cap is higher)", i, rr.Code)
		}
	}
}
