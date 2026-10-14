package routes

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	authh "psychic-homily-backend/internal/api/handlers/auth"
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
