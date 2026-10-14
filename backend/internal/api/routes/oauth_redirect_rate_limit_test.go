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
const oauthRedirectLimitPerMinute = 20 // middleware.OAuthRedirectRequestsPerMinute

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
	if got := loc.Query().Get("error_code"); got != authh.AuthPageRateLimitedCode {
		t.Errorf("%s error_code = %q, want %q", path, got, authh.AuthPageRateLimitedCode)
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

	for i, target := range []string{"/auth/login/google", "/auth/callback/google?state=s&code=c", "/auth/login/a%2Fb"} {
		if limitedAfterBurst(http.MethodGet, target, fmt.Sprintf("198.51.100.%d:1", 70+i)) {
			t.Errorf("GET %s was refused by the public-read limiter", target)
		}
	}

	for i, tc := range []struct{ method, target string }{
		{http.MethodHead, "/auth/login/google"},
		{http.MethodGet, "/auth/login/google/extra"},
		{http.MethodGet, "/auth/login/"},
		{http.MethodGet, "/auth/link/google"},
		{http.MethodGet, "/auth/profile"},
	} {
		if !limitedAfterBurst(tc.method, tc.target, fmt.Sprintf("198.51.100.%d:1", 80+i)) {
			t.Errorf("%s %s skipped the public-read limiter; only the OAuth redirects may", tc.method, tc.target)
		}
	}
}

// isOAuthRedirectRequest reads each template as a path ending in its one
// {provider} parameter, registered once.
func TestOAuthRedirectRoutesEndInTheirProvider(t *testing.T) {
	routes := chiRoutes(t, newTestRouter(t))
	for _, route := range oauthRedirectRoutes {
		if strings.Count(route, "{") != 1 || !strings.HasSuffix(route, "/{provider}") {
			t.Errorf("%q is not a path ending in its one {provider} parameter", route)
		}
		if got := matching(routes, http.MethodGet, strings.ReplaceAll(route, "{provider}", "{}")); len(got) != 1 || got[0] != route {
			t.Errorf("GET %s registrations = %v, want exactly [%s]", route, got, route)
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
