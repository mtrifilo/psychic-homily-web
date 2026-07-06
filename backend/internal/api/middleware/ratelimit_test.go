package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/httprate"

	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/auth"
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

// PSY-1173: isTrustedAPIToken recognizes the phk_ API-token prefix (and only
// that) so admin API clients bypass the limiter like show creation does.
func TestIsTrustedAPIToken(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   bool
	}{
		{"phk_ bearer token", "Bearer " + APITokenPrefix + "abc123", true},
		{"jwt bearer token", "Bearer eyJhbGciOi.foo.bar", false},
		{"non-bearer header", "Basic dXNlcjpwYXNz", false},
		{"no auth header", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/tag", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			if got := isTrustedAPIToken(req); got != tc.want {
				t.Errorf("isTrustedAPIToken() = %v, want %v", got, tc.want)
			}
		})
	}
}

// PSY-1173: a phk_ API token bypasses the limiter past the limit even with a
// nil JWTService — API tokens are admin-only and trusted (mirrors show
// creation's rateLimitUnlessAPIToken). Without this the ph CLI gets throttled
// during bulk tagging despite PSY-345's admin-bypass intent.
func TestSkipRateLimitForAdmin_APITokenBypassesLimit(t *testing.T) {
	// 1 request / minute limiter — trivially saturated without a bypass.
	base := httprate.Limit(1, time.Minute, httprate.WithKeyFuncs(httprate.KeyByIP))
	mw := SkipRateLimitForAdmin(nil, base)

	hits := 0
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
	}))

	// Five rapid same-IP requests, all carrying an API token, all pass.
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/tag", nil)
		req.Header.Set("Authorization", "Bearer "+APITokenPrefix+"deadbeef")
		req.RemoteAddr = "9.9.9.9:1000"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want 200 (phk_ token must bypass the limiter)", i+1, rr.Code)
		}
	}
	if hits != 5 {
		t.Errorf("handler hits = %d, want 5 (all phk_ requests should reach the handler)", hits)
	}
}

// --- RateLimitPublicReadsByAuthState (PSY-1373) ---

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

// authStateMW builds the router with 1-req/min anon (per-IP) and per-user limiters
// — trivially saturable so tests can assert routing + isolation.
func authStateMW(jwtService *auth.JWTService) func(http.Handler) http.Handler {
	anon := httprate.Limit(1, time.Minute, httprate.WithKeyFuncs(httprate.KeyByIP),
		httprate.WithLimitHandler(RateLimitExceededHandler))
	perUser := httprate.Limit(1, time.Minute, httprate.WithKeyFuncs(rateLimitUserKeyFunc),
		httprate.WithLimitHandler(RateLimitExceededHandler))
	return RateLimitPublicReadsByAuthState(jwtService, anon, perUser)
}

func readReq(remoteAddr, bearer string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/artists/1/graph-card", nil)
	req.RemoteAddr = remoteAddr
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	return req
}

func serve(h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// Anonymous traffic is routed to the per-IP limiter; past the limit it's 429'd.
func TestRateLimitPublicReadsByAuthState_AnonymousIsIPLimited(t *testing.T) {
	handler := authStateMW(newTestJWTService())(okHandler())

	if rr := serve(handler, readReq("9.9.9.9:1000", "")); rr.Code != http.StatusOK {
		t.Fatalf("first anonymous: status = %d, want 200", rr.Code)
	}
	rr := serve(handler, readReq("9.9.9.9:1001", "")) // same IP, different port
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("second anonymous: status = %d, want 429", rr.Code)
	}
	if rr.Header().Get("Retry-After") != "60" {
		t.Errorf("Retry-After = %q, want 60", rr.Header().Get("Retry-After"))
	}
}

// An authenticated user is METERED per-user (not bypassed): past the per-user
// limit the same user is 429'd. Closes the "full bypass" hole from PSY-1362.
func TestRateLimitPublicReadsByAuthState_AuthenticatedIsPerUserLimited(t *testing.T) {
	jwtService := newTestJWTService()
	user := &authm.User{Email: strPtr("fan@example.com")}
	user.ID = 42
	token, err := jwtService.CreateToken(user)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	handler := authStateMW(jwtService)(okHandler())

	if rr := serve(handler, readReq("9.9.9.9:1000", token)); rr.Code != http.StatusOK {
		t.Fatalf("first authenticated: status = %d, want 200", rr.Code)
	}
	rr := serve(handler, readReq("9.9.9.9:1001", token))
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("second (same user): status = %d, want 429 (authenticated users are metered, not bypassed)", rr.Code)
	}
}

// THE crux: two different users behind ONE IP each get their own bucket — a
// per-user key, not a shared per-IP one (the shared-IP false-positive fix).
func TestRateLimitPublicReadsByAuthState_UsersOnSameIPDoNotCollide(t *testing.T) {
	jwtService := newTestJWTService()
	mkToken := func(id uint) string {
		u := &authm.User{Email: strPtr("u@example.com")}
		u.ID = id
		tok, err := jwtService.CreateToken(u)
		if err != nil {
			t.Fatalf("CreateToken(%d): %v", id, err)
		}
		return tok
	}
	tokenA, tokenB := mkToken(42), mkToken(99)
	handler := authStateMW(jwtService)(okHandler())

	const sharedIP = "9.9.9.9:1000"
	// User A exhausts their own per-user bucket (limit 1).
	serve(handler, readReq(sharedIP, tokenA))
	if rr := serve(handler, readReq(sharedIP, tokenA)); rr.Code != http.StatusTooManyRequests {
		t.Fatalf("A second: status = %d, want 429", rr.Code)
	}
	// User B on the SAME IP still gets their first request through.
	if rr := serve(handler, readReq(sharedIP, tokenB)); rr.Code != http.StatusOK {
		t.Errorf("B first (same IP as exhausted A): status = %d, want 200 (per-user buckets must not collide on IP)", rr.Code)
	}
}

// SECURITY (PSY-1362 CRITICAL, preserved): a forged phk_ carries no session JWT,
// so it is routed as ANONYMOUS (per-IP) — it does NOT get the higher per-user cap.
func TestRateLimitPublicReadsByAuthState_ForgedAPITokenIsAnonymous(t *testing.T) {
	handler := authStateMW(newTestJWTService())(okHandler())

	if rr := serve(handler, readReq("9.9.9.9:1000", APITokenPrefix+"forged")); rr.Code != http.StatusOK {
		t.Fatalf("first forged: status = %d, want 200", rr.Code)
	}
	// Same IP → the anonymous per-IP bucket 429s it (no per-user bypass).
	if rr := serve(handler, readReq("9.9.9.9:1001", APITokenPrefix+"forged")); rr.Code != http.StatusTooManyRequests {
		t.Errorf("second forged (same IP): status = %d, want 429 (phk_ gets no per-user bucket)", rr.Code)
	}
}
