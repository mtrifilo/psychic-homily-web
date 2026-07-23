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

func enableEngagementEnv(k string) string {
	if k == EnableEngagementMutationRateLimitsEnvVar {
		return "1"
	}
	return ""
}

func newEngagementJWTService() *auth.JWTService {
	cfg := &config.Config{JWT: config.JWTConfig{SecretKey: "test-secret-key-for-routes-unit-32c", Expiry: 24}}
	return auth.NewJWTService(nil, cfg, usersvc.NewUserService(nil))
}

func engagementToken(t *testing.T, jwtService *auth.JWTService, id uint) string {
	t.Helper()
	user := &authm.User{ID: id}
	token, err := jwtService.CreateToken(user)
	if err != nil {
		t.Fatalf("CreateToken(%d): %v", id, err)
	}
	return token
}

func TestIsEngagementMutationRateLimitEnabled(t *testing.T) {
	// Opt-in: only "1" enables; anything else (incl. unset) stays off.
	cases := map[string]bool{"1": true, "": false, "0": false, "true": false, "2": false}
	for val, want := range cases {
		getenv := func(k string) string {
			if k == EnableEngagementMutationRateLimitsEnvVar {
				return val
			}
			return ""
		}
		if got := IsEngagementMutationRateLimitEnabled(getenv); got != want {
			t.Errorf("IsEngagementMutationRateLimitEnabled(%q) = %v, want %v", val, got, want)
		}
	}
}

func TestIsEngagementMutationRequest(t *testing.T) {
	cases := []struct {
		method string
		path   string
		want   bool
	}{
		{http.MethodPost, "/saved-shows/42", true},
		{http.MethodDelete, "/saved-shows/42", true},
		{http.MethodPost, "/saved-releases/7", true},
		{http.MethodDelete, "/saved-releases/7", true},
		{http.MethodPost, "/artists/1/follow", true},
		{http.MethodDelete, "/venues/9/follow", true},
		{http.MethodPost, "/scenes/phoenix-az/follow", true},
		{http.MethodDelete, "/scenes/phoenix-az/follow", true},
		// Reads and read-shaped helpers are NOT mutations.
		{http.MethodGet, "/saved-shows/42", false},
		{http.MethodGet, "/saved-shows", false},
		{http.MethodGet, "/saved-shows/42/check", false},
		{http.MethodGet, "/artists/1/followers", false},
		{http.MethodPost, FollowsBatchPath, false},
		{http.MethodPost, SaveCountsBatchPath, false},
		{http.MethodPost, "/me/following", false},
		{http.MethodPost, "/auth/login", false},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		if got := isEngagementMutationRequest(req); got != tc.want {
			t.Errorf("isEngagementMutationRequest(%s %s) = %v, want %v", tc.method, tc.path, got, tc.want)
		}
	}
}

// Default (flag unset) → pass-through noop, even for mutations past the limit.
// Keeps CI/E2E and fresh deploys unthrottled until deliberately enabled.
func TestEngagementMutationRateLimiter_NotEnabledIsNoop(t *testing.T) {
	jwtService := newEngagementJWTService()
	token := engagementToken(t, jwtService, 1)
	mw := EngagementMutationRateLimiter(jwtService, func(string) string { return "" })
	handler := mw(okRoutesHandler())

	for i := 0; i < middleware.EngagementMutationBurstPerMinute+5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/saved-shows/1", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		req.RemoteAddr = "7.7.7.7:100"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want 200 (disabled → noop)", i, rr.Code)
		}
	}
}

// Enabled: the burst-cap-th mutation passes, the next (61st) is 429 with
// Retry-After — the headline acceptance criterion.
func TestEngagementMutationRateLimiter_EnabledLimits61stMutation(t *testing.T) {
	jwtService := newEngagementJWTService()
	token := engagementToken(t, jwtService, 1)
	mw := EngagementMutationRateLimiter(jwtService, enableEngagementEnv)
	handler := mw(okRoutesHandler())

	for i := 0; i < middleware.EngagementMutationBurstPerMinute; i++ {
		req := httptest.NewRequest(http.MethodPost, "/saved-shows/1", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		req.RemoteAddr = "7.7.7.8:100"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("mutation %d within cap: status = %d, want 200", i, rr.Code)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/saved-shows/1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.RemoteAddr = "7.7.7.8:100"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("61st mutation: status = %d, want 429", rr.Code)
	}
	if rr.Header().Get("Retry-After") != "60" {
		t.Errorf("Retry-After = %q, want 60", rr.Header().Get("Retry-After"))
	}
}

// Save and follow share ONE per-user budget across DISTINCT endpoints: after a
// user fills the burst window with saves, a follow on a different route is 429'd.
func TestEngagementMutationRateLimiter_SaveAndFollowShareCounter(t *testing.T) {
	jwtService := newEngagementJWTService()
	token := engagementToken(t, jwtService, 1)
	mw := EngagementMutationRateLimiter(jwtService, enableEngagementEnv)
	handler := mw(okRoutesHandler())

	for i := 0; i < middleware.EngagementMutationBurstPerMinute; i++ {
		req := httptest.NewRequest(http.MethodPost, "/saved-shows/1", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		req.RemoteAddr = "7.7.7.9:100"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("save %d within cap: status = %d, want 200", i, rr.Code)
		}
	}

	// A follow on a DIFFERENT path from the same user is rejected: shared budget.
	req := httptest.NewRequest(http.MethodPost, "/artists/5/follow", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.RemoteAddr = "7.7.7.9:100"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("follow after save budget exhausted: status = %d, want 429 (save+follow share one counter)", rr.Code)
	}
}

// A trusted phk_ API token bypasses the limiter well past the cap.
func TestEngagementMutationRateLimiter_APITokenBypasses(t *testing.T) {
	mw := EngagementMutationRateLimiter(newEngagementJWTService(), enableEngagementEnv)
	handler := mw(okRoutesHandler())

	for i := 0; i < middleware.EngagementMutationBurstPerMinute+5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/saved-shows/1", nil)
		req.Header.Set("Authorization", "Bearer phk_deadbeef")
		req.RemoteAddr = "7.7.7.10:100"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("phk_ request %d: status = %d, want 200 (API token must bypass)", i, rr.Code)
		}
	}
}

// Unrelated writes (a login, a tag vote) are never touched by the engagement
// budget — a shared engagement counter must not 429 an unrelated endpoint.
func TestEngagementMutationRateLimiter_UnrelatedWritesNotLimited(t *testing.T) {
	jwtService := newEngagementJWTService()
	token := engagementToken(t, jwtService, 1)
	mw := EngagementMutationRateLimiter(jwtService, enableEngagementEnv)
	handler := mw(okRoutesHandler())

	for i := 0; i < middleware.EngagementMutationBurstPerMinute+5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		req.RemoteAddr = "7.7.7.11:100"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("unrelated write %d: status = %d, want 200 (not an engagement mutation)", i, rr.Code)
		}
	}
}

func okRoutesHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}
