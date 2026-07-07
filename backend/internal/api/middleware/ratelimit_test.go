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
// — trivially saturable so tests can assert routing + isolation. The per-IP
// authenticated ceiling is set generously high (1000) so it never interferes with
// the routing/isolation assertions; ceiling behavior has its own helper below.
func authStateMW(jwtService *auth.JWTService) func(http.Handler) http.Handler {
	anon := httprate.Limit(1, time.Minute, httprate.WithKeyFuncs(httprate.KeyByIP),
		httprate.WithLimitHandler(RateLimitExceededHandler))
	perUser := httprate.Limit(1, time.Minute, httprate.WithKeyFuncs(rateLimitUserKeyFunc),
		httprate.WithLimitHandler(RateLimitExceededHandler))
	ipCeiling := httprate.Limit(1000, time.Minute, httprate.WithKeyFuncs(httprate.KeyByIP),
		httprate.WithLimitHandler(RateLimitExceededHandler))
	return RateLimitPublicReadsByAuthState(jwtService, anon, perUser, ipCeiling)
}

// authStateMWWithCeiling saturates the per-IP authenticated ceiling directly for
// the PSY-1378 aggregate-bound tests: the per-user limiter stays generous (1000) so
// the ceiling — not the per-user cap — is what trips, isolating the behavior under
// test. anon stays at 1 (unused on the authenticated path).
func authStateMWWithCeiling(jwtService *auth.JWTService, ceiling int) func(http.Handler) http.Handler {
	anon := httprate.Limit(1, time.Minute, httprate.WithKeyFuncs(httprate.KeyByIP),
		httprate.WithLimitHandler(RateLimitExceededHandler))
	perUser := httprate.Limit(1000, time.Minute, httprate.WithKeyFuncs(rateLimitUserKeyFunc),
		httprate.WithLimitHandler(RateLimitExceededHandler))
	ipCeiling := httprate.Limit(ceiling, time.Minute, httprate.WithKeyFuncs(httprate.KeyByIP),
		httprate.WithLimitHandler(RateLimitExceededHandler))
	return RateLimitPublicReadsByAuthState(jwtService, anon, perUser, ipCeiling)
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

// Mounting the per-user limiter standalone (no RateLimitPublicReadsByAuthState to
// stash the user id) FAILS LOUD — httprate turns the key-func error into a 428 —
// rather than silently collapsing every request into one shared "user:0" bucket
// (adversarial-review MEDIUM).
func TestRateLimitPublicReadUserEndpoints_StandaloneFailsLoud(t *testing.T) {
	handler := RateLimitPublicReadUserEndpoints()(okHandler())
	rr := serve(handler, readReq("9.9.9.9:1000", ""))
	if rr.Code != http.StatusPreconditionRequired {
		t.Errorf("standalone per-user limiter: status = %d, want 428 (misuse must fail loud, not key user:0)", rr.Code)
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

// --- Per-IP authenticated ceiling (PSY-1378) ---

// THE crux of PSY-1378: the per-user cap alone lets one IP multiply throughput by
// spinning up many accounts. The per-IP ceiling bounds that aggregate — distinct
// users behind ONE IP share the ceiling bucket, so past the ceiling a fresh account
// (whose own generous per-user cap is untouched) is still 429'd.
func TestRateLimitPublicReadsByAuthState_AuthenticatedIPCeilingBoundsMultipleAccounts(t *testing.T) {
	jwtService := newTestJWTService()
	mkToken := func(id uint) string {
		u := &authm.User{Email: strPtr("scraper@example.com")}
		u.ID = id
		tok, err := jwtService.CreateToken(u)
		if err != nil {
			t.Fatalf("CreateToken(%d): %v", id, err)
		}
		return tok
	}
	// Ceiling of 2 authenticated reads/min per IP; per-user cap is generous (1000),
	// so only the ceiling can trip here.
	handler := authStateMWWithCeiling(jwtService, 2)(okHandler())

	const sharedIP = "7.7.7.7:2000"
	// Two DIFFERENT accounts from the same IP each get one request — fills the ceiling.
	if rr := serve(handler, readReq(sharedIP, mkToken(1))); rr.Code != http.StatusOK {
		t.Fatalf("account 1 first: status = %d, want 200", rr.Code)
	}
	if rr := serve(handler, readReq(sharedIP, mkToken(2))); rr.Code != http.StatusOK {
		t.Fatalf("account 2 first: status = %d, want 200", rr.Code)
	}
	// A THIRD fresh account from the same IP is 429'd by the aggregate ceiling,
	// even though its own per-user bucket is untouched — one origin can't multiply
	// throughput by adding accounts.
	if rr := serve(handler, readReq(sharedIP, mkToken(3))); rr.Code != http.StatusTooManyRequests {
		t.Errorf("account 3 (same IP, ceiling exhausted): status = %d, want 429", rr.Code)
	}
}

// The ceiling is PER-IP, not a single global bucket: exhausting one IP's ceiling
// must not throttle authenticated users on a DIFFERENT IP (else the fix would DoS
// the whole site the moment one scraper is active).
func TestRateLimitPublicReadsByAuthState_AuthenticatedIPCeilingIsPerIP(t *testing.T) {
	jwtService := newTestJWTService()
	u := &authm.User{Email: strPtr("fan@example.com")}
	u.ID = 55
	token, err := jwtService.CreateToken(u)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	handler := authStateMWWithCeiling(jwtService, 1)(okHandler())

	// Exhaust IP-A's ceiling (limit 1).
	if rr := serve(handler, readReq("1.1.1.1:100", token)); rr.Code != http.StatusOK {
		t.Fatalf("IP-A first: status = %d, want 200", rr.Code)
	}
	if rr := serve(handler, readReq("1.1.1.1:101", token)); rr.Code != http.StatusTooManyRequests {
		t.Fatalf("IP-A second: status = %d, want 429 (ceiling exhausted)", rr.Code)
	}
	// Same user (generous per-user cap) on a DIFFERENT IP still gets through — the
	// ceiling did not collapse into one global bucket.
	if rr := serve(handler, readReq("2.2.2.2:100", token)); rr.Code != http.StatusOK {
		t.Errorf("IP-B first (different IP): status = %d, want 200 (ceiling is per-IP, not global)", rr.Code)
	}
}

// ORDER regression (adversarial review): a single account spamming PAST ITS OWN
// per-user cap must NOT deplete the shared per-IP ceiling and collaterally 429 a
// DIFFERENT user on the same IP. This only holds when the per-user limiter is OUTER
// (rejected retries never reach/increment the ceiling); with the ceiling outer,
// httprate increments it on every attempt that clears 1000/min regardless of the
// per-user rejection, re-creating the shared-IP collision per-user keying prevents.
func TestRateLimitPublicReadsByAuthState_OwnCapRejectionsDoNotDrainSharedCeiling(t *testing.T) {
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
	// Tight per-user cap (1), small ceiling (2). Per-user is OUTER, ceiling INNER.
	anon := httprate.Limit(1, time.Minute, httprate.WithKeyFuncs(httprate.KeyByIP),
		httprate.WithLimitHandler(RateLimitExceededHandler))
	perUser := httprate.Limit(1, time.Minute, httprate.WithKeyFuncs(rateLimitUserKeyFunc),
		httprate.WithLimitHandler(RateLimitExceededHandler))
	ipCeiling := httprate.Limit(2, time.Minute, httprate.WithKeyFuncs(httprate.KeyByIP),
		httprate.WithLimitHandler(RateLimitExceededHandler))
	handler := RateLimitPublicReadsByAuthState(jwtService, anon, perUser, ipCeiling)(okHandler())

	const sharedIP = "3.3.3.3:400"
	tokenA, tokenB := mkToken(1), mkToken(2)

	// A's first read passes (per-user 1/1, ceiling 1/2).
	if rr := serve(handler, readReq(sharedIP, tokenA)); rr.Code != http.StatusOK {
		t.Fatalf("A first: status = %d, want 200", rr.Code)
	}
	// A now hammers past its OWN per-user cap 20×. Each is 429'd by the OUTER
	// per-user limiter and must NOT increment the shared ceiling (still 1/2).
	for i := 0; i < 20; i++ {
		if rr := serve(handler, readReq(sharedIP, tokenA)); rr.Code != http.StatusTooManyRequests {
			t.Fatalf("A retry %d: status = %d, want 429 (own per-user cap)", i, rr.Code)
		}
	}
	// B (fresh per-user bucket) on the SAME IP still gets through: the ceiling has
	// budget left (2/2) because A's rejected retries never reached it.
	if rr := serve(handler, readReq(sharedIP, tokenB)); rr.Code != http.StatusOK {
		t.Errorf("B first (same IP as spamming A): status = %d, want 200 (A's own-cap rejections must not drain the shared ceiling)", rr.Code)
	}
}
