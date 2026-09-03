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
	mw := PublicReadRateLimiter(nil, nil, nil, func(string) string { return "" })
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
	mw := PublicReadRateLimiter(nil, nil, nil, enableEnv)
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
	mw := PublicReadRateLimiter(nil, nil, nil, enableEnv)
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

// The batch follow-count endpoint is a READ that carries entity IDs in a POST
// body. It must share the anonymous read budget (PSY-1397).
func TestPublicReadRateLimiter_LimitsFollowsBatch(t *testing.T) {
	mw := PublicReadRateLimiter(nil, nil, nil, enableEnv)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < middleware.APIRequestsPerMinute; i++ {
		req := httptest.NewRequest(http.MethodPost, FollowsBatchPath, nil)
		req.RemoteAddr = "7.7.7.23:100"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("batch read %d within limit: status = %d, want 200", i, rr.Code)
		}
	}

	req := httptest.NewRequest(http.MethodPost, FollowsBatchPath, nil)
	req.RemoteAddr = "7.7.7.23:100"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("batch read past limit: status = %d, want 429 (read-via-POST must be metered)", rr.Code)
	}
	if rr.Header().Get("Retry-After") != "60" {
		t.Errorf("Retry-After = %q, want 60", rr.Header().Get("Retry-After"))
	}
}

// Every optional-auth POST is read-shaped and must appear on readViaPostPaths.
// If a new huma.Post(optionalAuthGroup, …) is added, list it here or justify
// an explicit exemption — otherwise it slips through the GET/HEAD filter unmetered.
func TestReadViaPostPaths_CoversOptionalAuthPosts(t *testing.T) {
	optionalAuthPosts := []string{
		SaveCountsBatchPath,
		ReleaseSaveCountsBatchPath,
		FollowsBatchPath,
	}
	listed := make(map[string]bool, len(readViaPostPaths))
	for _, p := range readViaPostPaths {
		listed[p] = true
	}
	for _, p := range optionalAuthPosts {
		if !listed[p] {
			t.Errorf("optional-auth POST %q not in readViaPostPaths", p)
		}
	}
}

func TestPublicReadRateLimiter_LimitsReleaseSaveBatch(t *testing.T) {
	mw := PublicReadRateLimiter(nil, nil, nil, enableEnv)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < middleware.APIRequestsPerMinute; i++ {
		req := httptest.NewRequest(http.MethodPost, ReleaseSaveCountsBatchPath, nil)
		req.RemoteAddr = "7.7.7.22:100"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("batch read %d within limit: status = %d, want 200", i, rr.Code)
		}
	}

	req := httptest.NewRequest(http.MethodPost, ReleaseSaveCountsBatchPath, nil)
	req.RemoteAddr = "7.7.7.22:100"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("read past limit: status = %d, want 429", rr.Code)
	}
}

// Genuine follow writes must bypass the read budget — only the batch read path
// is on the read-via-POST allowlist, not /{entity_type}/{entity_id}/follow.
func TestPublicReadRateLimiter_FollowWriteNotLimited(t *testing.T) {
	mw := PublicReadRateLimiter(nil, nil, nil, enableEnv)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < middleware.APIRequestsPerMinute+5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/artists/42/follow", nil)
		req.RemoteAddr = "7.7.7.24:100"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("follow write %d: status = %d, want 200 (writes bypass the read limiter)", i, rr.Code)
		}
	}
}

// A genuine write on a path that merely LOOKS adjacent must still bypass the
// read budget — the allowlist is exact-match, not a prefix.
func TestPublicReadRateLimiter_SaveWriteNotLimited(t *testing.T) {
	mw := PublicReadRateLimiter(nil, nil, nil, enableEnv)
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
	mw := PublicReadRateLimiter(nil, nil, nil, enableEnv)
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

// Both health endpoints are exempt — a load-balancer/uptime probe hammering one
// anonymously from a single IP must never be 429'd (that would flap the service
// unhealthy). /health/ready is the one alerting watches, so throttling it pages
// a human about a service that is fine. The exemption is exact-match, so each
// path needs its own entry and its own assertion here.
func TestPublicReadRateLimiter_HealthPathsExempt(t *testing.T) {
	probes := map[string]string{
		"/health":       "7.7.7.9:100",
		"/health/ready": "7.7.7.11:100",
	}

	for path, remoteAddr := range probes {
		t.Run(path, func(t *testing.T) {
			mw := PublicReadRateLimiter(nil, nil, nil, enableEnv)
			handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			for i := 0; i < middleware.APIRequestsPerMinute+5; i++ {
				req := httptest.NewRequest(http.MethodGet, path, nil)
				req.RemoteAddr = remoteAddr
				rr := httptest.NewRecorder()
				handler.ServeHTTP(rr, req)
				if rr.Code != http.StatusOK {
					t.Fatalf("health probe %d: status = %d, want 200 (%s must be exempt)", i, rr.Code, path)
				}
			}
		})
	}
}

// PSY-1430 / PSY-1505: personal iCal + Atom feeds are token-auth'd URL polls
// from Google/Apple Calendar / RSS-reader cloud IPs. They must not share the
// anonymous public-read per-IP bucket (PSY-1418) or feed fetchers get unfairly
// 429'd. PSY-2017: the exemption is the VALIDATED token, so this fixture
// resolves exactly the one token the paths below carry.
func TestPublicReadRateLimiter_PersonalFeedPathsExempt(t *testing.T) {
	const liveToken = "phcal_abc123"
	mw := PublicReadRateLimiter(nil, nil, func(tok string) bool { return tok == liveToken }, enableEnv)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	paths := []string{
		"/feeds/phcal_abc123/saved-shows.ics",
		"/feeds/phcal_abc123/follows.atom",
		"/calendar/phcal_abc123",
	}
	for _, path := range paths {
		for i := 0; i < middleware.APIRequestsPerMinute+5; i++ {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.RemoteAddr = "7.7.7.30:100"
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			if rr.Code != http.StatusOK {
				t.Fatalf("%s request %d: status = %d, want 200 (personal feeds exempt)", path, i, rr.Code)
			}
		}
	}

	// /calendar/token must NOT inherit the feed exemption (CRUD is JWT-gated;
	// unauthenticated probes still consume the anonymous public-read budget).
	for i := 0; i < middleware.APIRequestsPerMinute; i++ {
		req := httptest.NewRequest(http.MethodGet, "/calendar/token", nil)
		req.RemoteAddr = "7.7.7.31:100"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("calendar/token probe %d within limit: status = %d, want 200", i, rr.Code)
		}
	}
	reqToken := httptest.NewRequest(http.MethodGet, "/calendar/token", nil)
	reqToken.RemoteAddr = "7.7.7.31:100"
	rrToken := httptest.NewRecorder()
	handler.ServeHTTP(rrToken, reqToken)
	if rrToken.Code != http.StatusTooManyRequests {
		t.Errorf("calendar/token past limit: status = %d, want 429 (must not share feed exemption)", rrToken.Code)
	}

	// Adjacent catalog reads on the same IP still hit the anonymous bucket.
	for i := 0; i < middleware.APIRequestsPerMinute; i++ {
		req := httptest.NewRequest(http.MethodGet, "/artists/1/graph-card", nil)
		req.RemoteAddr = "7.7.7.30:100"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("catalog read %d within limit: status = %d, want 200", i, rr.Code)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/artists/1/graph-card", nil)
	req.RemoteAddr = "7.7.7.30:100"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("catalog read past limit: status = %d, want 429 (feeds exemption must not leak)", rr.Code)
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

	mw := PublicReadRateLimiter(jwtService, nil, nil, enableEnv)
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

// PSY-1814: a validated phk_ token is exempt from the anonymous per-IP budget.
func TestPublicReadRateLimiter_ValidatedAPITokenBypassesAnonCap(t *testing.T) {
	const valid = "phk_validated-ingest"
	mw := PublicReadRateLimiter(nil, func(tok string) bool { return tok == valid }, nil, enableEnv)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	const sharedIP = "7.7.7.40:100"
	for i := 0; i < middleware.APIRequestsPerMinute+1; i++ {
		req := httptest.NewRequest(http.MethodGet, "/artists/search", nil)
		req.Header.Set("Authorization", "Bearer "+valid)
		req.RemoteAddr = sharedIP
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("validated ingest %d: status = %d, want 200 (must not 429 at anon cap+1)", i, rr.Code)
		}
	}

	// Same-IP anonymous visitor still has a full anonymous budget.
	for i := 0; i < middleware.APIRequestsPerMinute; i++ {
		req := httptest.NewRequest(http.MethodGet, "/scenes", nil)
		req.RemoteAddr = sharedIP
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("visitor read %d after ingest burst: status = %d, want 200", i, rr.Code)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/scenes", nil)
	req.RemoteAddr = sharedIP
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("visitor past own cap: status = %d, want 429 (visitor still metered independently)", rr.Code)
	}
}

// PSY-1814 / PSY-1362: a forged phk_ prefix still 429s at the anonymous cap.
func TestPublicReadRateLimiter_ForgedAPITokenHitsAnonCap(t *testing.T) {
	mw := PublicReadRateLimiter(nil, func(string) bool { return false }, nil, enableEnv)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	const sharedIP = "7.7.7.41:100"
	for i := 0; i < middleware.APIRequestsPerMinute; i++ {
		req := httptest.NewRequest(http.MethodGet, "/artists/search", nil)
		req.Header.Set("Authorization", "Bearer phk_forged")
		req.RemoteAddr = sharedIP
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("forged %d within limit: status = %d, want 200", i, rr.Code)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/artists/search", nil)
	req.Header.Set("Authorization", "Bearer phk_forged")
	req.RemoteAddr = sharedIP
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("forged past limit: status = %d, want 429 (prefix-only must fail closed)", rr.Code)
	}
}

func TestPublicReadRateLimiter_CapsUnchanged(t *testing.T) {
	if middleware.APIRequestsPerMinute != 100 {
		t.Errorf("APIRequestsPerMinute = %d, want 100", middleware.APIRequestsPerMinute)
	}
	if middleware.PublicReadUserRequestsPerMinute != 300 {
		t.Errorf("PublicReadUserRequestsPerMinute = %d, want 300", middleware.PublicReadUserRequestsPerMinute)
	}
	if middleware.PublicReadAuthenticatedIPCeilingPerMinute != 1000 {
		t.Errorf("PublicReadAuthenticatedIPCeilingPerMinute = %d, want 1000", middleware.PublicReadAuthenticatedIPCeilingPerMinute)
	}
}

// PSY-1814: visitor GETs with no Authorization must not invoke ValidateToken.
func TestPublicReadRateLimiter_NoAuthDoesNotHitValidateCallback(t *testing.T) {
	called := false
	mw := PublicReadRateLimiter(nil, func(string) bool {
		called = true
		return false
	}, nil, enableEnv)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/venues/search", nil)
	req.RemoteAddr = "7.7.7.42:100"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("anonymous GET: status = %d, want 200", rr.Code)
	}
	if called {
		t.Error("validateAPIToken was called with no Authorization header (must stay DB-free)")
	}
}

// PSY-2017: the exemption is the token, not the URL shape. A path that spells a
// feed route with a token nothing resolves is metered exactly like any other
// anonymous read, which is what closes the "name the prefix, skip the limiter"
// hole the phk_ bypass had.
func TestPublicReadRateLimiter_UnvalidatedFeedTokenIsLimited(t *testing.T) {
	paths := []string{
		"/feeds/phcal_junk/saved-shows.ics",
		"/feeds/phcal_junk/follows.atom",
		"/calendar/phcal_junk",
		"/feeds/junk/saved-shows.ics",
		"/feeds/anything-at-all",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			mw := PublicReadRateLimiter(nil, nil, func(string) bool { return false }, enableEnv)
			handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			for i := 0; i < middleware.APIRequestsPerMinute; i++ {
				req := httptest.NewRequest(http.MethodGet, path, nil)
				req.RemoteAddr = "7.7.9.10:100"
				rr := httptest.NewRecorder()
				handler.ServeHTTP(rr, req)
				if rr.Code != http.StatusOK {
					t.Fatalf("request %d within limit: status = %d, want 200", i, rr.Code)
				}
			}
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.RemoteAddr = "7.7.9.10:100"
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			if rr.Code != http.StatusTooManyRequests {
				t.Errorf("request past limit: status = %d, want 429 (an unvalidated feed token earns no exemption)", rr.Code)
			}
		})
	}
}

// A feed path whose token carries no phcal_ prefix must not spend a lookup: the
// prefix is the pre-filter that keeps a visitor GET off the database, exactly as
// validatedAPIToken keeps one off api_tokens.
func TestPublicReadRateLimiter_NonPrefixedFeedPathDoesNotHitValidateCallback(t *testing.T) {
	called := false
	mw := PublicReadRateLimiter(nil, nil, func(string) bool {
		called = true
		return false
	}, enableEnv)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for _, path := range []string{"/feeds/junk/saved-shows.ics", "/calendar/token", "/artists/1/graph-card"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.RemoteAddr = "7.7.9.11:100"
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}
	if called {
		t.Error("validateFeedToken was called for a path carrying no phcal_ token (must stay DB-free)")
	}
}

// personalFeedTokenFromPath is what decides whether a request is even a
// candidate for the exemption, so its shape rules are pinned directly: only the
// registered feed routes yield a token, and a token never spans a path segment.
func TestPersonalFeedTokenFromPath(t *testing.T) {
	cases := map[string]string{
		"/feeds/phcal_abc/saved-shows.ics": "phcal_abc",
		"/feeds/phcal_abc/follows.atom":    "phcal_abc",
		"/calendar/phcal_abc":              "phcal_abc",
		"/calendar/token":                  "token",
		"/feeds/phcal_abc/other.ics":       "",
		"/feeds//saved-shows.ics":          "",
		"/saved-shows.ics":                 "",
		"/feeds/a/b/saved-shows.ics":       "",
		"/calendar/phcal_abc/saved-shows":  "",
		"/calendar/":                       "",
		"/feeds/":                          "",
	}
	for path, want := range cases {
		if got := personalFeedTokenFromPath(path); got != want {
			t.Errorf("personalFeedTokenFromPath(%q) = %q, want %q", path, got, want)
		}
	}
}
