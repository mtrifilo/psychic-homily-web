package routes

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	authh "psychic-homily-backend/internal/api/handlers/auth"
	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
)

// Written out rather than read from middleware, so a budget retuned there and
// not here fails this file instead of passing silently.
const (
	oauthRedirectLimitPerMinute = 20 // middleware.OAuthRedirectRequestsPerMinute
	appleSignInLimitPerMinute   = 20 // middleware.AppleSignInRequestsPerMinute
)

// testConfig leaves the frontend URL empty, so the handler's localhost default
// is where a refused sign-in is sent.
const testFrontendAuthPage = "http://localhost:3000/auth"

// oauthStartReached reports whether GET /auth/login/google reached its handler.
// The test binary registers no goth provider, so the handler answers 400 for
// one it cannot resolve; any other status means something in front of it
// answered instead.
func oauthStartReached(t *testing.T, router http.Handler, ip string) bool {
	t.Helper()
	return serveFrom(router, http.MethodGet, "/auth/login/google", ip).Code == http.StatusBadRequest
}

// oauthCallbackReached reports whether GET /auth/callback/google reached its
// handler, which answers a handshake it cannot complete by sending the browser
// to the auth page with its generic failure and no error_code.
func oauthCallbackReached(t *testing.T, router http.Handler, ip string) bool {
	t.Helper()
	rr := serveFrom(router, http.MethodGet, "/auth/callback/google", ip)
	if rr.Code != http.StatusTemporaryRedirect {
		return false
	}
	loc, err := url.Parse(rr.Header().Get("Location"))
	if err != nil {
		t.Fatalf("callback Location %q does not parse: %v", rr.Header().Get("Location"), err)
	}
	return loc.Query().Get("error") == "authentication failed" && !loc.Query().Has("error_code")
}

func serveFrom(router http.Handler, method, path, ip string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.RemoteAddr = ip
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr
}

// exhaustPasswordBudget spends the public auth budget on POST /auth/login and
// confirms the next attempt is refused, so a later assertion is made against a
// budget that is really spent.
func exhaustPasswordBudget(t *testing.T, router *chi.Mux, ip string) {
	t.Helper()
	for i := 0; i < authLimitPerMinute; i++ {
		send(t, router, http.MethodPost, "/auth/login", ip, nil)
	}
	if code := send(t, router, http.MethodPost, "/auth/login", ip, nil); code != http.StatusTooManyRequests {
		t.Fatalf("POST /auth/login after %d attempts = %d, want 429; the password budget is not spent", authLimitPerMinute, code)
	}
}

// assertRefusedToAuthPage requires a refused sign-in redirect: a redirect to
// the frontend auth page carrying the JSON 429's message and the RATE_LIMITED
// code, with nothing a browser would render as a JSON page.
func assertRefusedToAuthPage(t *testing.T, rr *httptest.ResponseRecorder, path string) {
	t.Helper()
	if rr.Code != http.StatusTemporaryRedirect {
		t.Fatalf("%s refused = %d, want %d", path, rr.Code, http.StatusTemporaryRedirect)
	}
	loc, err := url.Parse(rr.Header().Get("Location"))
	if err != nil {
		t.Fatalf("%s Location %q does not parse: %v", path, rr.Header().Get("Location"), err)
	}
	if got := loc.Scheme + "://" + loc.Host + loc.Path; got != testFrontendAuthPage {
		t.Errorf("%s redirects to %q, want %q", path, got, testFrontendAuthPage)
	}
	if got := loc.Query().Get("error_code"); got != authh.OAuthRedirectRateLimitedCode {
		t.Errorf("%s error_code = %q, want %q", path, got, authh.OAuthRedirectRateLimitedCode)
	}
	if got := loc.Query().Get("error"); got != rateLimitExceededMessage {
		t.Errorf("%s error = %q, want the JSON 429's message %q", path, got, rateLimitExceededMessage)
	}
	if ct := rr.Header().Get("Content-Type"); strings.Contains(ct, "json") {
		t.Errorf("%s refusal Content-Type = %q, want no JSON", path, ct)
	}
	if body := rr.Body.String(); strings.Contains(body, "too_many_requests") {
		t.Errorf("%s refusal body carries the JSON 429: %s", path, body)
	}
	if got := rr.Header().Get("Retry-After"); got != "" {
		t.Errorf("%s refusal carries Retry-After %q, which asks the browser to wait before following the redirect", path, got)
	}
}

func TestPasswordBudgetDoesNotRefuseOAuthRedirects(t *testing.T) {
	t.Setenv(DisableAuthRateLimitsEnvVar, "")
	router := newTestRouter(t)
	const ip = "203.0.113.61:1234"

	exhaustPasswordBudget(t, router, ip)

	if !oauthStartReached(t, router, ip) {
		t.Error("GET /auth/login/google did not reach its handler after the password budget was spent")
	}
	if !oauthCallbackReached(t, router, ip) {
		t.Error("GET /auth/callback/google did not reach its handler after the password budget was spent")
	}
	if !linkStartReached(t, router, ip) {
		t.Error("GET /auth/link/google did not reach the session check after the password budget was spent")
	}
	if !appleCallbackReached(t, router, ip) {
		t.Error("POST /auth/apple/callback did not reach its handler after the password budget was spent")
	}
}

func TestOAuthRedirectRefusalRedirectsToAuthPage(t *testing.T) {
	t.Setenv(DisableAuthRateLimitsEnvVar, "")
	router := newTestRouter(t)
	const ip = "203.0.113.62:1234"

	// Start and callback draw on one counter: half the budget on each, and the
	// next request to either is refused.
	for i := 0; i < oauthRedirectLimitPerMinute/2; i++ {
		if !oauthStartReached(t, router, ip) {
			t.Fatalf("start %d was refused before the %d/min budget was spent", i+1, oauthRedirectLimitPerMinute)
		}
		if !oauthCallbackReached(t, router, ip) {
			t.Fatalf("callback %d was refused before the %d/min budget was spent", i+1, oauthRedirectLimitPerMinute)
		}
	}

	for _, path := range []string{"/auth/login/google", "/auth/callback/google", "/auth/login/github", "/auth/callback/github"} {
		assertRefusedToAuthPage(t, serveFrom(router, http.MethodGet, path, ip), path)
	}

	if !oauthStartReached(t, router, "203.0.113.63:1234") {
		t.Error("a second address was refused by the first address's OAuth redirect budget")
	}
}

func TestOAuthRedirectRefusalsDoNotSpendPasswordBudget(t *testing.T) {
	t.Setenv(DisableAuthRateLimitsEnvVar, "")
	router := newTestRouter(t)
	const ip = "203.0.113.64:1234"

	for i := 0; i < oauthRedirectLimitPerMinute+authLimitPerMinute; i++ {
		serveFrom(router, http.MethodGet, "/auth/login/google", ip)
	}
	assertRefusedToAuthPage(t, serveFrom(router, http.MethodGet, "/auth/login/google", ip), "/auth/login/google")

	code, body := sendWithBody(t, router, http.MethodPost, "/auth/login", ip, nil)
	if code == http.StatusTooManyRequests {
		t.Fatal("POST /auth/login was refused after only OAuth redirects were sent; the two budgets share a counter")
	}
	assertReachedHandler(t, code, body, "/auth/login")
}

// A route that answers JSON keeps answering its refusal in JSON.
func TestAuthJSONRouteRefusalStaysJSON(t *testing.T) {
	t.Setenv(DisableAuthRateLimitsEnvVar, "")
	router := newTestRouter(t)
	const ip = "203.0.113.65:1234"

	exhaustPasswordBudget(t, router, ip)

	rr := serveFrom(router, http.MethodPost, "/auth/login", ip)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("POST /auth/login = %d, want 429", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("POST /auth/login refusal Content-Type = %q, want application/json", ct)
	}
	if got := rr.Header().Get("Retry-After"); got != "60" {
		t.Errorf("POST /auth/login refusal Retry-After = %q, want 60", got)
	}
	want := `{"success":false,"error":"too_many_requests","message":"` + rateLimitExceededMessage + `"}`
	if body := rr.Body.String(); body != want {
		t.Errorf("POST /auth/login refusal body = %s, want %s", body, want)
	}
}

func TestOAuthRedirectRateLimitDisabledFlagIsHonored(t *testing.T) {
	t.Setenv(DisableAuthRateLimitsEnvVar, "1")
	router := newTestRouter(t)
	const ip = "203.0.113.66:1234"

	for i := 0; i < oauthRedirectLimitPerMinute*3; i++ {
		if !oauthStartReached(t, router, ip) {
			t.Fatalf("start %d did not reach its handler with %s=1", i+1, DisableAuthRateLimitsEnvVar)
		}
		if !oauthCallbackReached(t, router, ip) {
			t.Fatalf("callback %d did not reach its handler with %s=1", i+1, DisableAuthRateLimitsEnvVar)
		}
	}
}

// The public-read limiter is mounted globally, outside the route group, so it
// would refuse an OAuth redirect first and with JSON. Only a GET that routes to
// one of oauthRedirectRoutes skips it.
func TestPublicReadLimiterSkipsOnlyTheOAuthRedirects(t *testing.T) {
	mw := PublicReadRateLimiter(nil, nil, nil, enableEnv)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	limitedAfterBurst := func(method, target, ip string) bool {
		for i := 0; i < middleware.APIRequestsPerMinute; i++ {
			serveFrom(handler, method, target, ip)
		}
		return serveFrom(handler, method, target, ip).Code == http.StatusTooManyRequests
	}

	for i, target := range []string{"/auth/login/google", "/auth/callback/google?state=s&code=c", "/auth/login/a%2Fb", "/auth/link/google?t=x"} {
		if limitedAfterBurst(http.MethodGet, target, fmt.Sprintf("198.51.100.%d:1", 70+i)) {
			t.Errorf("GET %s was refused by the public-read limiter", target)
		}
	}

	for i, tc := range []struct{ method, target string }{
		{http.MethodHead, "/auth/login/google"},
		{http.MethodGet, "/auth/login/google/extra"},
		{http.MethodGet, "/auth/login/"},
		{http.MethodHead, "/auth/link/google"},
		{http.MethodGet, "/auth/link/google/extra"},
		// RawPath is not a valid encoding of Path in these two, so EscapedPath
		// re-encodes Path and reads as an OAuth route while chi routes the raw
		// spelling to no OAuth handler.
		{http.MethodGet, "/auth/log%69n/goo|gle"},
		{http.MethodGet, "/auth%2Flogin/x|"},
		{http.MethodGet, "/auth/profile"},
	} {
		if !limitedAfterBurst(tc.method, tc.target, fmt.Sprintf("198.51.100.%d:1", 80+i)) {
			t.Errorf("%s %s skipped the public-read limiter; only the OAuth redirects may", tc.method, tc.target)
		}
	}
}

// isOAuthRedirectRequest reads each template as a path ending in its one
// {provider} parameter, and exempts every GET one segment under the template's
// prefix. That is only the OAuth route while no other GET is registered under
// the prefix: a literal sibling would win over {provider} in chi and skip the
// public-read limiter without being on the OAuth one.
func TestOAuthRedirectRoutesEndInTheirProvider(t *testing.T) {
	routes := chiRoutes(t, newTestRouter(t))
	for _, route := range oauthRedirectRoutes {
		if strings.Count(route, "{") != 1 || !strings.HasSuffix(route, "/{provider}") {
			t.Errorf("%q is not a path ending in its one {provider} parameter", route)
		}
		prefix := strings.TrimSuffix(route, "{provider}")
		var under []string
		for _, pattern := range routes[http.MethodGet] {
			if strings.HasPrefix(pattern, prefix) {
				under = append(under, pattern)
			}
		}
		if len(under) != 1 || under[0] != route {
			t.Errorf("GET routes under %s = %v, want exactly [%s]", prefix, under, route)
		}
	}
}

// The rejection line names this limiter, which is what tells these refusals
// apart from the password budget's in the logs.
func TestOAuthRedirectRefusalLogsItsOwnLimiter(t *testing.T) {
	t.Setenv(DisableAuthRateLimitsEnvVar, "")
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	limited := oauthRedirectRateLimiter(func(w http.ResponseWriter, _ *http.Request, _ string) {
		w.WriteHeader(http.StatusTemporaryRedirect)
	})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i <= oauthRedirectLimitPerMinute; i++ {
		req := httptest.NewRequest(http.MethodGet, "/auth/login/google", nil)
		req.RemoteAddr = "203.0.113.67:1234"
		limited.ServeHTTP(httptest.NewRecorder(), req.WithContext(logger.NewContext(req.Context(), log)))
	}

	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatalf("want one JSON rejection line, got %q: %v", buf.String(), err)
	}
	if line["event"] != "ratelimit_rejected" || line["limiter"] != string(middleware.LimiterOAuthRedirect) {
		t.Errorf("rejection line event=%v limiter=%v, want ratelimit_rejected and %s",
			line["event"], line["limiter"], middleware.LimiterOAuthRedirect)
	}
}

// linkStartReached reports whether GET /auth/link/google got past the limiter.
// The request carries no session, so JWTMiddleware, which runs behind the
// limiter, answers 401; a refusal by the limiter is a redirect instead.
func linkStartReached(t *testing.T, router http.Handler, ip string) bool {
	t.Helper()
	return serveFrom(router, http.MethodGet, "/auth/link/google", ip).Code == http.StatusUnauthorized
}

// appleCallbackReached reports whether POST /auth/apple/callback reached its
// operation. The body omits the required identity token, so the operation
// answers its own validation error; the limiter answers 429.
func appleCallbackReached(t *testing.T, router *chi.Mux, ip string) bool {
	t.Helper()
	code, body := sendWithBody(t, router, http.MethodPost, "/auth/apple/callback", ip, nil)
	if code == http.StatusTooManyRequests {
		return false
	}
	assertReachedHandler(t, code, body, "/auth/apple/callback")
	return true
}

// assertRefusedToSettings requires a refused link start: a redirect to the
// Settings tab carrying oauth_link_error=RATE_LIMITED, where every other link
// result is read, with nothing a browser would render as a JSON page.
func assertRefusedToSettings(t *testing.T, rr *httptest.ResponseRecorder, path string) {
	t.Helper()
	if rr.Code != http.StatusTemporaryRedirect {
		t.Fatalf("%s refused = %d, want %d", path, rr.Code, http.StatusTemporaryRedirect)
	}
	loc, err := url.Parse(rr.Header().Get("Location"))
	if err != nil {
		t.Fatalf("%s Location %q does not parse: %v", path, rr.Header().Get("Location"), err)
	}
	if got, want := loc.Scheme+"://"+loc.Host+loc.Path, "http://localhost:3000/profile"; got != want {
		t.Errorf("%s redirects to %q, want %q", path, got, want)
	}
	want := url.Values{"tab": {"settings"}, "oauth_link_error": {authh.OAuthRedirectRateLimitedCode}}
	if got := loc.Query(); got.Encode() != want.Encode() {
		t.Errorf("%s query = %q, want exactly %q", path, got.Encode(), want.Encode())
	}
	if ct := rr.Header().Get("Content-Type"); strings.Contains(ct, "json") {
		t.Errorf("%s refusal Content-Type = %q, want no JSON", path, ct)
	}
	if body := rr.Body.String(); strings.Contains(body, "too_many_requests") {
		t.Errorf("%s refusal body carries the JSON 429: %s", path, body)
	}
	if got := rr.Header().Get("Retry-After"); got != "" {
		t.Errorf("%s refusal carries Retry-After %q, which asks the browser to wait before following the redirect", path, got)
	}
}

// The link start draws on the OAuth redirect counter: half the budget on the
// sign-in start and half on the link start, and the next request to either is
// refused, the link start back to Settings and the sign-in start to the auth
// page.
func TestLinkStartSharesTheOAuthRedirectBudgetAndRefusesToSettings(t *testing.T) {
	t.Setenv(DisableAuthRateLimitsEnvVar, "")
	router := newTestRouter(t)
	const ip = "203.0.113.68:1234"

	for i := 0; i < oauthRedirectLimitPerMinute/2; i++ {
		if !oauthStartReached(t, router, ip) {
			t.Fatalf("start %d was refused before the %d/min budget was spent", i+1, oauthRedirectLimitPerMinute)
		}
		if !linkStartReached(t, router, ip) {
			t.Fatalf("link start %d was refused before the %d/min budget was spent", i+1, oauthRedirectLimitPerMinute)
		}
	}

	for _, path := range []string{"/auth/link/google", "/auth/link/github?t=token"} {
		assertRefusedToSettings(t, serveFrom(router, http.MethodGet, path, ip), path)
	}
	assertRefusedToAuthPage(t, serveFrom(router, http.MethodGet, "/auth/login/google", ip), "/auth/login/google")

	if !linkStartReached(t, router, "203.0.113.69:1234") {
		t.Error("a second address was refused by the first address's OAuth redirect budget")
	}
}

func TestLinkStartRefusalsDoNotSpendPasswordBudget(t *testing.T) {
	t.Setenv(DisableAuthRateLimitsEnvVar, "")
	router := newTestRouter(t)
	const ip = "203.0.113.70:1234"

	for i := 0; i < oauthRedirectLimitPerMinute+authLimitPerMinute; i++ {
		serveFrom(router, http.MethodGet, "/auth/link/google", ip)
	}
	assertRefusedToSettings(t, serveFrom(router, http.MethodGet, "/auth/link/google", ip), "/auth/link/google")

	code, body := sendWithBody(t, router, http.MethodPost, "/auth/login", ip, nil)
	if code == http.StatusTooManyRequests {
		t.Fatal("POST /auth/login was refused after only link starts were sent; the two budgets share a counter")
	}
	assertReachedHandler(t, code, body, "/auth/login")
}

// The Apple callback is posted by a native client, so its refusal is the JSON
// 429 with Retry-After, on a counter of its own: password attempts and OAuth
// redirects cannot spend it, and it spends neither.
func TestAppleSignInHasItsOwnJSONBudget(t *testing.T) {
	t.Setenv(DisableAuthRateLimitsEnvVar, "")
	router := newTestRouter(t)
	const ip = "203.0.113.71:1234"

	exhaustPasswordBudget(t, router, ip)
	for i := 0; i < oauthRedirectLimitPerMinute; i++ {
		serveFrom(router, http.MethodGet, "/auth/login/google", ip)
	}
	assertRefusedToAuthPage(t, serveFrom(router, http.MethodGet, "/auth/login/google", ip), "/auth/login/google")

	for i := 0; i < appleSignInLimitPerMinute; i++ {
		if !appleCallbackReached(t, router, ip) {
			t.Fatalf("Apple callback %d was refused before its %d/min budget was spent", i+1, appleSignInLimitPerMinute)
		}
	}

	rr := serveFrom(router, http.MethodPost, "/auth/apple/callback", ip)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("POST /auth/apple/callback past its budget = %d, want 429", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Apple callback refusal Content-Type = %q, want application/json", ct)
	}
	if got := rr.Header().Get("Retry-After"); got != "60" {
		t.Errorf("Apple callback refusal Retry-After = %q, want 60", got)
	}
	if loc := rr.Header().Get("Location"); loc != "" {
		t.Errorf("Apple callback refusal redirects to %q; its caller reads JSON", loc)
	}
	want := `{"success":false,"error":"too_many_requests","message":"` + rateLimitExceededMessage + `"}`
	if body := rr.Body.String(); body != want {
		t.Errorf("Apple callback refusal body = %s, want %s", body, want)
	}

	const fresh = "203.0.113.72:1234"
	for i := 0; i <= appleSignInLimitPerMinute; i++ {
		serveFrom(router, http.MethodPost, "/auth/apple/callback", fresh)
	}
	if !oauthStartReached(t, router, fresh) {
		t.Error("GET /auth/login/google was refused after only Apple callbacks were sent; the two budgets share a counter")
	}
	code, body := sendWithBody(t, router, http.MethodPost, "/auth/login", fresh, nil)
	if code == http.StatusTooManyRequests {
		t.Fatal("POST /auth/login was refused after only Apple callbacks were sent; the two budgets share a counter")
	}
	assertReachedHandler(t, code, body, "/auth/login")
}

func TestLinkStartAndAppleRateLimitDisabledFlagIsHonored(t *testing.T) {
	t.Setenv(DisableAuthRateLimitsEnvVar, "1")
	router := newTestRouter(t)
	const ip = "203.0.113.73:1234"

	for i := 0; i < oauthRedirectLimitPerMinute*3; i++ {
		if !linkStartReached(t, router, ip) {
			t.Fatalf("link start %d did not reach the session check with %s=1", i+1, DisableAuthRateLimitsEnvVar)
		}
		if !appleCallbackReached(t, router, ip) {
			t.Fatalf("Apple callback %d did not reach its handler with %s=1", i+1, DisableAuthRateLimitsEnvVar)
		}
	}
}
