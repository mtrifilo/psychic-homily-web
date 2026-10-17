package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/httprate"

	"psychic-homily-backend/internal/config"
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
	burst := httprate.Limit(1, time.Minute, httprate.WithKeyFuncs(mutationUserKeyFunc),
		httprate.WithLimitHandler(testRejection))
	sustained := httprate.Limit(1000, time.Hour, httprate.WithKeyFuncs(mutationUserKeyFunc),
		httprate.WithLimitHandler(testRejection))
	return RateLimitMutationsByUser(jwtService, burst, sustained)
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
// The batch budget's HOUR window: without this, wiring the burst limiter twice,
// or building the sustained one on time.Minute, ships green. The burst window is
// stubbed wide open so only the hour window can reject.
func TestRateLimitEntityRequestBatchSustained_LimitsAtItsOwnCeiling(t *testing.T) {
	jwtService := newTestJWTService()
	token := mkEngagementToken(t, jwtService, 1)
	burst := httprate.Limit(EntityRequestBatchSustainedPerHour*2, time.Minute,
		httprate.WithKeyFuncs(mutationUserKeyFunc),
		httprate.WithLimitHandler(testRejection))
	handler := RateLimitMutationsByUser(jwtService, burst, RateLimitEntityRequestBatchSustained())(okHandler())

	send := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/entity-requests/batch", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		return rr
	}

	for i := 0; i < EntityRequestBatchSustainedPerHour; i++ {
		if rr := send(); rr.Code != http.StatusOK {
			t.Fatalf("request %d within the hour ceiling: status = %d, want 200", i, rr.Code)
		}
	}
	rr := send()
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("request past the hour ceiling: status = %d, want 429", rr.Code)
	}
	// The hour window must not tell the caller to retry in a minute: that value
	// becomes the client's countdown.
	if got := rr.Header().Get("Retry-After"); got != "3600" {
		t.Errorf("Retry-After = %q, want 3600 (the window this limiter meters)", got)
	}
}

// The two budgets are distinct instances, so spending one leaves the other. The
// burst pair is covered in routes; this pins the sustained pair, which is what a
// copy-paste of the burst constructor would break.
func TestEntityRequestBatchSustained_IsNotTheEngagementSustained(t *testing.T) {
	jwtService := newTestJWTService()
	token := mkEngagementToken(t, jwtService, 1)
	wideBurst := func() func(http.Handler) http.Handler {
		return httprate.Limit(EngagementMutationSustainedPerHour*2, time.Minute,
			httprate.WithKeyFuncs(mutationUserKeyFunc),
			httprate.WithLimitHandler(testRejection))
	}
	batch := RateLimitMutationsByUser(jwtService, wideBurst(), RateLimitEntityRequestBatchSustained())(okHandler())
	engagement := RateLimitMutationsByUser(jwtService, wideBurst(), RateLimitEngagementMutationSustained())(okHandler())

	send := func(h http.Handler) int {
		req := httptest.NewRequest(http.MethodPost, "/entity-requests/batch", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		return rr.Code
	}

	for i := 0; i < EntityRequestBatchSustainedPerHour; i++ {
		if code := send(batch); code != http.StatusOK {
			t.Fatalf("batch request %d: status = %d, want 200", i, code)
		}
	}
	if code := send(batch); code != http.StatusTooManyRequests {
		t.Fatalf("batch past its hour ceiling: status = %d, want 429", code)
	}
	if code := send(engagement); code != http.StatusOK {
		t.Errorf("engagement hour window after the batch one is spent: status = %d, want 200", code)
	}
}

func TestRateLimitMutationsByUser_AuthenticatedIsPerUserLimited(t *testing.T) {
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
func TestRateLimitMutationsByUser_SustainedWindowEnforced(t *testing.T) {
	jwtService := newTestJWTService()
	token := mkEngagementToken(t, jwtService, 42)
	burst := httprate.Limit(1000, time.Minute, httprate.WithKeyFuncs(mutationUserKeyFunc),
		httprate.WithLimitHandler(testRejection))
	sustained := httprate.Limit(1, time.Hour, httprate.WithKeyFuncs(mutationUserKeyFunc),
		httprate.WithLimitHandler(testRejection))
	handler := RateLimitMutationsByUser(jwtService, burst, sustained)(okHandler())

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
func TestRateLimitMutationsByUser_SaveAndFollowShareCounter(t *testing.T) {
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
func TestRateLimitMutationsByUser_UsersDoNotCollide(t *testing.T) {
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
func TestRateLimitMutationsByUser_UnauthenticatedPassesThrough(t *testing.T) {
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
func TestRateLimitMutationsByUser_StandaloneLimiterFailsLoud(t *testing.T) {
	handler := RateLimitEngagementMutationBurst()(okHandler())
	rr := serve(handler, mutationReq("9.9.9.9:1000", ""))
	if rr.Code != http.StatusPreconditionRequired {
		t.Errorf("standalone burst limiter: status = %d, want 428 (misuse must fail loud)", rr.Code)
	}
}

// sessionUserMW builds RateLimitBySessionUser over a per-user budget of
// perUser and a per-IP ceiling of ceiling, both a minute long and logged under
// the register begin limiter names.
func sessionUserMW(jwtService *auth.JWTService, perUser, ceiling int) func(http.Handler) http.Handler {
	return RateLimitBySessionUser(jwtService,
		perUserMutationLimiter(LimiterPasskeyRegisterBegin, perUser, time.Minute),
		limiterSpec{name: LimiterPasskeyRegisterBeginIPCeiling, limit: ceiling, window: time.Minute, key: KeyByClientIP}.handler())
}

// A session-user budget is perUser deep for each account wherever it connects
// from, a second account on the same address has its own, and a request with no
// session passes through to the auth gate downstream.
func TestRateLimitBySessionUser_OneBucketPerAccount(t *testing.T) {
	jwtService := newTestJWTService()
	tokenA := mkEngagementToken(t, jwtService, 42)
	tokenB := mkEngagementToken(t, jwtService, 99)
	const budget = 2
	handler := sessionUserMW(jwtService, budget, 1000)(okHandler())

	for i, addr := range []string{"9.9.9.9:1000", "8.8.8.8:1000"} {
		if rr := serve(handler, mutationReq(addr, tokenA)); rr.Code != http.StatusOK {
			t.Fatalf("A request %d from %s: status = %d, want 200", i+1, addr, rr.Code)
		}
	}
	rr := serve(handler, mutationReq("7.7.7.7:1000", tokenA))
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("A past its budget from a third address: status = %d, want 429", rr.Code)
	}
	if got := rr.Header().Get("Retry-After"); got != "60" {
		t.Errorf("Retry-After = %q, want 60", got)
	}

	if rr := serve(handler, mutationReq("9.9.9.9:1000", tokenB)); rr.Code != http.StatusOK {
		t.Errorf("B on A's address after A's budget was spent: status = %d, want 200", rr.Code)
	}

	// A browser session rides the auth cookie, not the Authorization header,
	// and spends the same account's bucket.
	cookieReq := mutationReq("6.6.6.6:1000", "")
	cookieReq.AddCookie(&http.Cookie{Name: config.AuthCookieName, Value: tokenA})
	if rr := serve(handler, cookieReq); rr.Code != http.StatusTooManyRequests {
		t.Errorf("A's spent budget through the auth cookie: status = %d, want 429", rr.Code)
	}
	for i := 0; i < budget*2; i++ {
		if rr := serve(handler, mutationReq("9.9.9.9:1000", "")); rr.Code != http.StatusOK {
			t.Fatalf("request %d with no session: status = %d, want 200 (passed to the auth gate)", i+1, rr.Code)
		}
	}
}

// serveSessionUserLogged serves one request through handler with a capturing
// logger and returns the response and the limiters its rejection lines name.
func serveSessionUserLogged(t *testing.T, handler http.Handler, remoteAddr, bearer string) (*httptest.ResponseRecorder, []any) {
	t.Helper()
	logs := newLogCapture(t)
	rr := serve(handler, logs.attach(mutationReq(remoteAddr, bearer)))
	var limiters []any
	for _, line := range logs.events(rateLimitRejectedEvent) {
		limiters = append(limiters, line["limiter"])
	}
	return rr, limiters
}

// Accounts behind one address each keep their own budget, and together they
// are bounded by the address's ceiling: past it a fresh account, its own budget
// untouched, is refused by the ceiling with the same 429 and Retry-After.
func TestRateLimitBySessionUser_IPCeilingBoundsAccountsTogether(t *testing.T) {
	jwtService := newTestJWTService()
	const perUser, ceiling = 2, 3
	handler := sessionUserMW(jwtService, perUser, ceiling)(okHandler())
	const sharedIP = "5.5.5.5:1000"
	tokenA := mkEngagementToken(t, jwtService, 1)
	tokenB := mkEngagementToken(t, jwtService, 2)
	tokenC := mkEngagementToken(t, jwtService, 3)

	for i := 0; i < perUser; i++ {
		if rr := serve(handler, mutationReq(sharedIP, tokenA)); rr.Code != http.StatusOK {
			t.Fatalf("A request %d: status = %d, want 200", i+1, rr.Code)
		}
	}
	rr, limiters := serveSessionUserLogged(t, handler, sharedIP, tokenA)
	if rr.Code != http.StatusTooManyRequests || len(limiters) != 1 || limiters[0] != string(LimiterPasskeyRegisterBegin) {
		t.Fatalf("A past its own budget: status = %d, rejection lines name %v, want 429 from [%s]", rr.Code, limiters, LimiterPasskeyRegisterBegin)
	}

	if rr := serve(handler, mutationReq(sharedIP, tokenB)); rr.Code != http.StatusOK {
		t.Fatalf("B first, filling the ceiling: status = %d, want 200", rr.Code)
	}
	rr, limiters = serveSessionUserLogged(t, handler, sharedIP, tokenC)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("C first on a full ceiling: status = %d, want 429", rr.Code)
	}
	if len(limiters) != 1 || limiters[0] != string(LimiterPasskeyRegisterBeginIPCeiling) {
		t.Errorf("C's refusal lines name %v, want exactly [%s]", limiters, LimiterPasskeyRegisterBeginIPCeiling)
	}
	if got := rr.Header().Get("Retry-After"); got != "60" {
		t.Errorf("ceiling refusal Retry-After = %q, want 60", got)
	}

	// The ceiling is per address, not one bucket for the site.
	if rr := serve(handler, mutationReq("4.4.4.4:1000", tokenC)); rr.Code != http.StatusOK {
		t.Errorf("C from another address: status = %d, want 200", rr.Code)
	}
}

// An account retrying past its own budget is refused by the user limiter and
// never reaches the shared ceiling, so it cannot spend the address's ceiling
// and refuse the other accounts behind it. This fails if the ceiling is the
// outer limiter.
func TestRateLimitBySessionUser_OwnBudgetRefusalsDoNotSpendTheCeiling(t *testing.T) {
	jwtService := newTestJWTService()
	handler := sessionUserMW(jwtService, 1, 2)(okHandler())
	const sharedIP = "3.3.3.3:400"
	tokenA := mkEngagementToken(t, jwtService, 1)
	tokenB := mkEngagementToken(t, jwtService, 2)

	if rr := serve(handler, mutationReq(sharedIP, tokenA)); rr.Code != http.StatusOK {
		t.Fatalf("A first: status = %d, want 200", rr.Code)
	}
	for i := 0; i < 20; i++ {
		if rr := serve(handler, mutationReq(sharedIP, tokenA)); rr.Code != http.StatusTooManyRequests {
			t.Fatalf("A retry %d: status = %d, want 429", i+1, rr.Code)
		}
	}
	if rr := serve(handler, mutationReq(sharedIP, tokenB)); rr.Code != http.StatusOK {
		t.Errorf("B on A's address after A's refused retries: status = %d, want 200", rr.Code)
	}
}
