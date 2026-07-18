package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/httprate"

	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/auth"
)

func TestRateLimitEngagementMutationBurst_ReturnsMiddleware(t *testing.T) {
	if RateLimitEngagementMutationBurst() == nil {
		t.Fatal("RateLimitEngagementMutationBurst() returned nil")
	}
}

func TestRateLimitEngagementMutationSustained_ReturnsMiddleware(t *testing.T) {
	if RateLimitEngagementMutationSustained() == nil {
		t.Fatal("RateLimitEngagementMutationSustained() returned nil")
	}
}

// engagementMW builds the wrapper with tiny burst/sustained limiters (1/min,
// 1000/hr) so tests can saturate the burst window and assert routing/isolation
// without sending 60 requests. The sustained window stays generous so only the
// burst limiter trips here.
func engagementMW(jwtService *auth.JWTService) func(http.Handler) http.Handler {
	burst := httprate.Limit(1, time.Minute, httprate.WithKeyFuncs(engagementUserKeyFunc),
		httprate.WithLimitHandler(RateLimitExceededHandler))
	sustained := httprate.Limit(1000, time.Hour, httprate.WithKeyFuncs(engagementUserKeyFunc),
		httprate.WithLimitHandler(RateLimitExceededHandler))
	return RateLimitEngagementMutationsByUser(jwtService, burst, sustained)
}

func mutationReq(remoteAddr, bearer string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/saved-shows/1", nil)
	req.RemoteAddr = remoteAddr
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	return req
}

func mkEngagementToken(t *testing.T, jwtService *auth.JWTService, id uint) string {
	t.Helper()
	u := &authm.User{Email: strPtr("fan@example.com")}
	u.ID = id
	tok, err := jwtService.CreateToken(u)
	if err != nil {
		t.Fatalf("CreateToken(%d): %v", id, err)
	}
	return tok
}

// An authenticated user is metered per-user: past the burst cap the same user
// is 429'd with a Retry-After header (the AC's "61st mutation → 429", scaled).
func TestRateLimitEngagementMutationsByUser_AuthenticatedIsPerUserLimited(t *testing.T) {
	jwtService := newTestJWTService()
	token := mkEngagementToken(t, jwtService, 42)
	handler := engagementMW(jwtService)(okHandler())

	if rr := serve(handler, mutationReq("9.9.9.9:1000", token)); rr.Code != http.StatusOK {
		t.Fatalf("first mutation: status = %d, want 200", rr.Code)
	}
	rr := serve(handler, mutationReq("9.9.9.9:1001", token))
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("second mutation (same user): status = %d, want 429", rr.Code)
	}
	if rr.Header().Get("Retry-After") != "60" {
		t.Errorf("Retry-After = %q, want 60", rr.Header().Get("Retry-After"))
	}
}

// BOTH windows are enforced: with a generous burst but a tiny sustained cap, the
// second mutation trips the SUSTAINED (hour) limiter — proving it is actually
// chained, not just the burst window (policy: stricter of the two wins).
func TestRateLimitEngagementMutationsByUser_SustainedWindowEnforced(t *testing.T) {
	jwtService := newTestJWTService()
	token := mkEngagementToken(t, jwtService, 42)
	burst := httprate.Limit(1000, time.Minute, httprate.WithKeyFuncs(engagementUserKeyFunc),
		httprate.WithLimitHandler(RateLimitExceededHandler))
	sustained := httprate.Limit(1, time.Hour, httprate.WithKeyFuncs(engagementUserKeyFunc),
		httprate.WithLimitHandler(RateLimitExceededHandler))
	handler := RateLimitEngagementMutationsByUser(jwtService, burst, sustained)(okHandler())

	if rr := serve(handler, mutationReq("9.9.9.9:1000", token)); rr.Code != http.StatusOK {
		t.Fatalf("first mutation: status = %d, want 200", rr.Code)
	}
	if rr := serve(handler, mutationReq("9.9.9.9:1001", token)); rr.Code != http.StatusTooManyRequests {
		t.Errorf("second mutation: status = %d, want 429 (sustained hour window must be enforced)", rr.Code)
	}
}

// Save and follow share ONE counter: after a user exhausts the shared budget on
// a save path, a follow on a DIFFERENT path from the same user is still 429'd.
// (Routing to distinct handlers is proven at the routes level; here the wrapper
// keys purely on user id, so any two in-scope requests share the bucket.)
func TestRateLimitEngagementMutationsByUser_SaveAndFollowShareCounter(t *testing.T) {
	jwtService := newTestJWTService()
	token := mkEngagementToken(t, jwtService, 7)
	handler := engagementMW(jwtService)(okHandler())

	save := mutationReq("9.9.9.9:1000", token)
	if rr := serve(handler, save); rr.Code != http.StatusOK {
		t.Fatalf("first (save): status = %d, want 200", rr.Code)
	}
	follow := httptest.NewRequest(http.MethodPost, "/artists/1/follow", nil)
	follow.Header.Set("Authorization", "Bearer "+token)
	if rr := serve(handler, follow); rr.Code != http.StatusTooManyRequests {
		t.Errorf("follow after save exhausted budget: status = %d, want 429 (shared counter)", rr.Code)
	}
}

// Two different users each get their own bucket — a per-user key, not a shared
// one — even on the same IP.
func TestRateLimitEngagementMutationsByUser_UsersDoNotCollide(t *testing.T) {
	jwtService := newTestJWTService()
	tokenA := mkEngagementToken(t, jwtService, 42)
	tokenB := mkEngagementToken(t, jwtService, 99)
	handler := engagementMW(jwtService)(okHandler())

	const sharedIP = "9.9.9.9:1000"
	serve(handler, mutationReq(sharedIP, tokenA))
	if rr := serve(handler, mutationReq(sharedIP, tokenA)); rr.Code != http.StatusTooManyRequests {
		t.Fatalf("A second: status = %d, want 429", rr.Code)
	}
	if rr := serve(handler, mutationReq(sharedIP, tokenB)); rr.Code != http.StatusOK {
		t.Errorf("B first (same IP as exhausted A): status = %d, want 200 (per-user buckets must not collide)", rr.Code)
	}
}

// Unauthenticated requests pass through untouched (no user id to key on; the
// downstream JWT middleware 401s them). They are NOT collapsed into a shared
// bucket, so an anonymous flood can't 429 a legitimate authenticated user.
func TestRateLimitEngagementMutationsByUser_UnauthenticatedPassesThrough(t *testing.T) {
	handler := engagementMW(newTestJWTService())(okHandler())

	for i := 0; i < 5; i++ {
		if rr := serve(handler, mutationReq("9.9.9.9:1000", "")); rr.Code != http.StatusOK {
			t.Fatalf("anonymous request %d: status = %d, want 200 (pass through to downstream auth)", i+1, rr.Code)
		}
	}
}

// Mounting a bare per-user limiter (no wrapper to stash the user id) FAILS LOUD
// — httprate turns the key-func error into a 428 — rather than silently keying
// every request into one shared bucket. A detectable misuse, not a site-wide
// budget.
func TestRateLimitEngagementMutationsByUser_StandaloneLimiterFailsLoud(t *testing.T) {
	handler := RateLimitEngagementMutationBurst()(okHandler())
	rr := serve(handler, mutationReq("9.9.9.9:1000", ""))
	if rr.Code != http.StatusPreconditionRequired {
		t.Errorf("standalone burst limiter: status = %d, want 428 (misuse must fail loud)", rr.Code)
	}
}
