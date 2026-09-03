package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/httprate"

	authm "psychic-homily-backend/internal/models/auth"
	adminsvc "psychic-homily-backend/internal/services/admin"
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
	mw := SkipRateLimitForAdmin(nil, nil, base)

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

// extractJWT rejects exactly what the authenticating middleware rejects. A
// header with extra fields is not a Bearer credential to bearerTokenFromHeader,
// so both fall back to the cookie and agree on which credential the request is
// presenting.
func TestExtractJWT_MalformedBearerHeaderFallsBackToCookieLikeTheAuthenticator(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+APITokenPrefix+"live trailing")
	req.AddCookie(&http.Cookie{Name: "auth_token", Value: "cookie-token"})

	if got := extractJWT(req); got != "cookie-token" {
		t.Errorf("extractJWT = %q, want %q (a three-field header is not a Bearer credential)", got, "cookie-token")
	}
}

// validatedAPIToken calls the validator only for a phk_-prefixed Authorization
// header, so an ordinary browser request costs no database round trip, and it
// reports the validator's answer rather than the prefix.
func TestValidatedAPIToken(t *testing.T) {
	const live = APITokenPrefix + "live"

	tests := []struct {
		name        string
		header      string
		cookie      string
		want        bool
		wantQueried bool
	}{
		{name: "live phk_ token", header: "Bearer " + live, want: true, wantQueried: true},
		{name: "unknown phk_ token", header: "Bearer " + APITokenPrefix + "forged", wantQueried: true},
		{name: "jwt bearer token", header: "Bearer eyJhbGciOi.foo.bar"},
		{name: "non-bearer header", header: "Basic dXNlcjpwYXNz"},
		{name: "no credential at all"},
		{name: "session cookie only", cookie: "session-jwt"},
		// An API token delivered in the cookie authenticates downstream but is
		// not a bypass here: reading it would let any request carrying a
		// phk_-shaped cookie value spend a hash and a query ahead of the
		// limiter about to reject it.
		{name: "phk_ in the cookie is not an API-token credential", cookie: live},
		// The limiter and the authenticator must read the same header. A
		// three-field header is not a Bearer credential to either of them, so
		// the phk_ inside it buys nothing.
		{name: "phk_ inside a malformed header", header: "Bearer " + live + " trailing"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			queried := ""
			validate := func(token string) bool {
				queried = token
				return token == live
			}
			req := httptest.NewRequest(http.MethodPost, "/tag", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			if tc.cookie != "" {
				req.AddCookie(&http.Cookie{Name: "auth_token", Value: tc.cookie})
			}
			if got := validatedAPIToken(validate, req); got != tc.want {
				t.Errorf("validatedAPIToken() = %v, want %v", got, tc.want)
			}
			if gotQueried := queried != ""; gotQueried != tc.wantQueried {
				t.Errorf("validator queried = %v, want %v (queried %q)", gotQueried, tc.wantQueried, queried)
			}
		})
	}
}

// A nil validator is "no usable token", so every request is metered.
func TestValidatedAPIToken_NilValidatorFailsClosed(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/tag", nil)
	req.Header.Set("Authorization", "Bearer "+APITokenPrefix+"whatever")
	if validatedAPIToken(nil, req) {
		t.Error("validatedAPIToken(nil, ...) = true, want false: a missing validator must not exempt anything")
	}
}

// acceptOnly is a token validator that accepts exactly one token.
func acceptOnly(live string) func(string) bool {
	return func(token string) bool { return token == live }
}

// skipAdminMW builds SkipRateLimitForAdmin over a 1-request/minute limiter,
// trivially saturated, and returns it with a counter of the requests that
// reached the handler.
func skipAdminMW(t *testing.T, jwtService *auth.JWTService, validate func(string) bool) (http.Handler, *int) {
	t.Helper()
	base := httprate.Limit(1, time.Minute,
		httprate.WithKeyFuncs(httprate.KeyByIP),
		httprate.WithLimitHandler(RateLimitExceededHandler))
	hits := 0
	handler := SkipRateLimitForAdmin(jwtService, validate, base)(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			hits++
			w.WriteHeader(http.StatusOK)
		}))
	return handler, &hits
}

// A validated phk_ token bypasses the limiter past the cap: the ph CLI's bulk
// imports must not be throttled.
func TestSkipRateLimitForAdmin_ValidatedAPITokenBypassesLimit(t *testing.T) {
	const live = APITokenPrefix + "live"
	handler, hits := skipAdminMW(t, nil, acceptOnly(live))

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/tag", nil)
		req.Header.Set("Authorization", "Bearer "+live)
		req.RemoteAddr = "9.9.9.9:1000"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want 200 (a live API token must bypass the limiter)", i+1, rr.Code)
		}
	}
	if *hits != 5 {
		t.Errorf("handler hits = %d, want 5 (all validated requests should reach the handler)", *hits)
	}
}

// The headline regression. A cookie-authenticated caller that names a phk_
// token it does not hold is metered like the session it actually rides on.
func TestSkipRateLimitForAdmin_ForgedAPITokenOverCookieSessionIsLimited(t *testing.T) {
	// Both shapes must be metered. Which credential the limiter READ is pinned
	// by TestCredentialReadersAgree; here the assertion is the outcome, so
	// these stay green for either reason.
	headers := []string{
		"Bearer " + APITokenPrefix + "forged",
		"Bearer " + APITokenPrefix + "live trailing",
	}
	for _, header := range headers {
		t.Run(header, func(t *testing.T) {
			// A fresh limiter per case: httprate keys by IP, so a shared one
			// would let the first case spend the second case's budget.
			handler, hits := skipAdminMW(t, nil, acceptOnly(APITokenPrefix+"live"))

			first := httptest.NewRequest(http.MethodPost, "/tag", nil)
			first.Header.Set("Authorization", header)
			first.AddCookie(&http.Cookie{Name: "auth_token", Value: "session-jwt"})
			first.RemoteAddr = "9.9.9.8:1000"
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, first)
			if rr.Code != http.StatusOK {
				t.Fatalf("first request: status = %d, want 200", rr.Code)
			}

			second := httptest.NewRequest(http.MethodPost, "/tag", nil)
			second.Header.Set("Authorization", header)
			second.AddCookie(&http.Cookie{Name: "auth_token", Value: "session-jwt"})
			second.RemoteAddr = "9.9.9.8:1001"
			rr = httptest.NewRecorder()
			handler.ServeHTTP(rr, second)
			if rr.Code != http.StatusTooManyRequests {
				t.Fatalf("second request: status = %d, want 429 (a forged phk_ must not exempt a cookie session)", rr.Code)
			}
			if got := rr.Header().Get("Retry-After"); got != "60" {
				t.Errorf("Retry-After = %q, want %q", got, "60")
			}
			if *hits != 1 {
				t.Errorf("handler hits = %d, want 1 (only the first request is under the cap)", *hits)
			}
		})
	}
}

// No Authorization header at all: unchanged, the limiter applies.
func TestSkipRateLimitForAdmin_NoCredentialIsLimited(t *testing.T) {
	handler, _ := skipAdminMW(t, nil, acceptOnly(APITokenPrefix+"live"))

	first := httptest.NewRequest(http.MethodPost, "/tag", nil)
	first.RemoteAddr = "9.9.9.6:1000"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, first)
	if rr.Code != http.StatusOK {
		t.Fatalf("first request: status = %d, want 200", rr.Code)
	}

	second := httptest.NewRequest(http.MethodPost, "/tag", nil)
	second.RemoteAddr = "9.9.9.6:1001"
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, second)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("second request: status = %d, want 429", rr.Code)
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
	return authStateMWValidate(jwtService, nil)
}

func authStateMWValidate(jwtService *auth.JWTService, validateAPIToken func(string) bool) func(http.Handler) http.Handler {
	anon := httprate.Limit(1, time.Minute, httprate.WithKeyFuncs(httprate.KeyByIP),
		httprate.WithLimitHandler(RateLimitExceededHandler))
	perUser := httprate.Limit(1, time.Minute, httprate.WithKeyFuncs(rateLimitUserKeyFunc),
		httprate.WithLimitHandler(RateLimitExceededHandler))
	ipCeiling := httprate.Limit(1000, time.Minute, httprate.WithKeyFuncs(httprate.KeyByIP),
		httprate.WithLimitHandler(RateLimitExceededHandler))
	return RateLimitPublicReadsByAuthState(jwtService, validateAPIToken, anon, perUser, ipCeiling)
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
	return RateLimitPublicReadsByAuthState(jwtService, nil, anon, perUser, ipCeiling)
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

// SECURITY (PSY-1362 CRITICAL, preserved): a forged phk_ carries no session JWT
// and fails ValidateToken, so it is routed as ANONYMOUS (per-IP) — it does NOT
// get the higher per-user cap or the validated-token bypass (PSY-1814).
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

// PSY-1814: a callback that rejects the token (forged / revoked / expired) still
// lands on the anonymous per-IP bucket — prefix alone is not enough.
func TestRateLimitPublicReadsByAuthState_RejectedAPITokenIsAnonymous(t *testing.T) {
	handler := authStateMWValidate(newTestJWTService(), func(string) bool { return false })(okHandler())

	if rr := serve(handler, readReq("8.8.8.8:1000", APITokenPrefix+"forged")); rr.Code != http.StatusOK {
		t.Fatalf("first rejected: status = %d, want 200", rr.Code)
	}
	if rr := serve(handler, readReq("8.8.8.8:1001", APITokenPrefix+"forged")); rr.Code != http.StatusTooManyRequests {
		t.Errorf("second rejected (same IP): status = %d, want 429 (failed ValidateToken stays anonymous)", rr.Code)
	}
}

// PSY-1814: a validated phk_ token skips the public-read limiter entirely — it
// does not 429 past the anonymous cap and does not increment that bucket.
func TestRateLimitPublicReadsByAuthState_ValidatedAPITokenBypasses(t *testing.T) {
	const valid = APITokenPrefix + "real"
	handler := authStateMWValidate(newTestJWTService(), func(tok string) bool { return tok == valid })(okHandler())

	const sharedIP = "6.6.6.6:1000"
	for i := 0; i < 5; i++ {
		if rr := serve(handler, readReq(sharedIP, valid)); rr.Code != http.StatusOK {
			t.Fatalf("validated request %d: status = %d, want 200 (validated phk_ must bypass)", i, rr.Code)
		}
	}
	// Anonymous visitor on the same IP still gets the first anonymous slot —
	// the ingest burst must not have incremented the per-IP bucket.
	if rr := serve(handler, readReq(sharedIP, "")); rr.Code != http.StatusOK {
		t.Errorf("anonymous after validated burst: status = %d, want 200 (validated ingest must not starve visitors)", rr.Code)
	}
}

// PSY-1814: visitor GETs with no Authorization must not invoke the DB callback.
func TestRateLimitPublicReadsByAuthState_NoAuthDoesNotValidateAPIToken(t *testing.T) {
	called := false
	handler := authStateMWValidate(newTestJWTService(), func(string) bool {
		called = true
		return false
	})(okHandler())

	if rr := serve(handler, readReq("5.5.5.5:1000", "")); rr.Code != http.StatusOK {
		t.Fatalf("anonymous: status = %d, want 200", rr.Code)
	}
	if called {
		t.Error("validateAPIToken was called for a request with no Authorization (must stay DB-free)")
	}
}

// PSY-1814: admin session JWTs stay on the per-user bucket even when a validate
// callback is wired — they must not take the full API-token bypass.
func TestRateLimitPublicReadsByAuthState_SessionJWTDoesNotUseAPITokenBypass(t *testing.T) {
	jwtService := newTestJWTService()
	user := &authm.User{Email: strPtr("admin@example.com")}
	user.ID = 7
	token, err := jwtService.CreateToken(user)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	called := false
	handler := authStateMWValidate(jwtService, func(string) bool {
		called = true
		return true
	})(okHandler())

	if rr := serve(handler, readReq("4.4.4.4:1000", token)); rr.Code != http.StatusOK {
		t.Fatalf("first JWT: status = %d, want 200", rr.Code)
	}
	if rr := serve(handler, readReq("4.4.4.4:1001", token)); rr.Code != http.StatusTooManyRequests {
		t.Errorf("second JWT (same user): status = %d, want 429 (session JWTs stay on the per-user cap)", rr.Code)
	}
	if called {
		t.Error("validateAPIToken was called for a session JWT (SessionUserID must win)")
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
	handler := RateLimitPublicReadsByAuthState(jwtService, nil, anon, perUser, ipCeiling)(okHandler())

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

// PSY-2017: skipping a limiter is a SEPARATE grant from authenticating. A scope
// that adminsvc later learns to validate stays metered until it is named in the
// bypass allowlist, which is what "a new scope is limited by default" means.
func TestAPITokenBypassScopes(t *testing.T) {
	cases := map[string]bool{
		adminsvc.TokenScopeAdmin: true,
		"readonly":               false,
		"ingest":                 false,
		"":                       false,
	}
	for scope, want := range cases {
		if got := apiTokenBypassScopes[scope]; got != want {
			t.Errorf("apiTokenBypassScopes[%q] = %v, want %v", scope, got, want)
		}
	}
}
