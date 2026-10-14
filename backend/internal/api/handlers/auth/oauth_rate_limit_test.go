package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"psychic-homily-backend/internal/config"
)

const rateLimitedTestMessage = "Rate limit exceeded. Please try again in 60 seconds."

// refuse runs RedirectRateLimited for target with chi's route pattern and
// provider parameter set, as the router sets them, and returns the redirect it
// answered with.
func refuse(t *testing.T, route, target, provider string, cookies ...*http.Cookie) *url.URL {
	t.Helper()
	h := NewOAuthHTTPHandler(nil, nil, &config.Config{Email: config.EmailConfig{FrontendURL: "https://psychichomily.com"}})
	req := httptest.NewRequest(http.MethodGet, target, nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rctx := chi.NewRouteContext()
	rctx.RoutePatterns = []string{route}
	rctx.URLParams.Add("provider", provider)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	h.RedirectRateLimited(rr, req, rateLimitedTestMessage)

	if rr.Code != http.StatusTemporaryRedirect {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusTemporaryRedirect)
	}
	loc, err := url.Parse(rr.Header().Get("Location"))
	if err != nil {
		t.Fatalf("Location %q does not parse: %v", rr.Header().Get("Location"), err)
	}
	return loc
}

func assertAuthPage(t *testing.T, loc *url.URL) {
	t.Helper()
	if got, want := loc.Scheme+"://"+loc.Host+loc.Path, "https://psychichomily.com/auth"; got != want {
		t.Errorf("redirects to %q, want %q", got, want)
	}
	want := url.Values{
		authPageErrorParam:     {rateLimitedTestMessage},
		authPageErrorCodeParam: {OAuthRedirectRateLimitedCode},
	}
	if got := loc.Query(); got.Encode() != want.Encode() {
		t.Errorf("query = %q, want exactly %q", got.Encode(), want.Encode())
	}
}

func TestRedirectRateLimitedSendsTheBrowserToTheConfiguredAuthPage(t *testing.T) {
	assertAuthPage(t, refuse(t, OAuthCallbackRoute, "/auth/callback/google?code=spent", "google"))
}

func TestRedirectRateLimitedReturnsAPendingLinkToSettings(t *testing.T) {
	const intentID = "rate-limit-test-intent"
	storeOAuthLinkIntent(intentID, oauthLinkIntent{
		userID:    7,
		provider:  "google",
		state:     "link-state",
		expiresAt: time.Now().Add(time.Minute),
	})
	t.Cleanup(func() { consumeOAuthLinkIntent(intentID) })
	intentCookie := &http.Cookie{Name: oauthLinkIntentCookieName, Value: intentID}

	loc := refuse(t, OAuthCallbackRoute, "/auth/callback/google?state=link-state&code=spent", "google", intentCookie)
	if got, want := loc.Scheme+"://"+loc.Host+loc.Path, "https://psychichomily.com"+oauthLinkSettingsPath; got != want {
		t.Errorf("a refused link callback redirects to %q, want %q", got, want)
	}
	if got := loc.Query().Get(oauthLinkErrorParam); got != OAuthRedirectRateLimitedCode {
		t.Errorf("%s = %q, want %q", oauthLinkErrorParam, got, OAuthRedirectRateLimitedCode)
	}
	if _, found := peekOAuthLinkIntent(intentID); !found {
		t.Error("the refusal consumed the link intent; only the callback handler may")
	}

	// The intent is about one handshake. A sign-in that merely still carries the
	// cookie is refused to the auth page like any other.
	assertAuthPage(t, refuse(t, OAuthCallbackRoute, "/auth/callback/google?state=other-state&code=spent", "google", intentCookie))
	assertAuthPage(t, refuse(t, OAuthCallbackRoute, "/auth/callback/github?state=link-state&code=spent", "github", intentCookie))

	// A cookie naming an intent this process does not hold is no link either.
	assertAuthPage(t, refuse(t, OAuthCallbackRoute, "/auth/callback/google?state=link-state&code=spent", "google",
		&http.Cookie{Name: oauthLinkIntentCookieName, Value: "no-such-intent"}))
}

func TestRedirectRateLimitedReturnsALinkStartToSettings(t *testing.T) {
	loc := refuse(t, OAuthLinkRoute, "/auth/link/google?t=one-time-token", "google")
	if got, want := loc.Scheme+"://"+loc.Host+loc.Path, "https://psychichomily.com"+oauthLinkSettingsPath; got != want {
		t.Errorf("a refused link start redirects to %q, want %q", got, want)
	}
	want := url.Values{
		oauthLinkSettingsTabKey: {oauthLinkSettingsTabValue},
		oauthLinkErrorParam:     {OAuthRedirectRateLimitedCode},
	}
	if got := loc.Query(); got.Encode() != want.Encode() {
		t.Errorf("query = %q, want exactly %q", got.Encode(), want.Encode())
	}
}

// Only the link route is a link start. A sign-in start, or a request whose
// route pattern chi never set, goes to the auth page.
func TestRedirectRateLimitedSendsOtherRoutesToTheAuthPage(t *testing.T) {
	assertAuthPage(t, refuse(t, OAuthLoginRoute, "/auth/login/google", "google"))
	assertAuthPage(t, refuse(t, "", "/auth/link/google", "google"))
}
