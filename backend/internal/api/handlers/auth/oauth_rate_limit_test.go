package auth

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"psychic-homily-backend/internal/config"
)

const rateLimitedTestMessage = "Rate limit exceeded. Please try again in 60 seconds."

// refuseRecorded runs RedirectRateLimited for target with chi's route pattern
// and provider parameter set, as the router sets them, and returns what it
// wrote after checking that it is a redirect.
func refuseRecorded(t *testing.T, route, target, provider string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
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
	return rr
}

// refuse is refuseRecorded's redirect target.
func refuse(t *testing.T, route, target, provider string, cookies ...*http.Cookie) *url.URL {
	t.Helper()
	rr := refuseRecorded(t, route, target, provider, cookies...)
	loc, err := url.Parse(rr.Header().Get("Location"))
	if err != nil {
		t.Fatalf("Location %q does not parse: %v", rr.Header().Get("Location"), err)
	}
	return loc
}

// assertAuthPage requires the refused sign-in redirect, carrying returnTo when
// it is not empty and no destination when it is.
func assertAuthPage(t *testing.T, loc *url.URL, returnTo string) {
	t.Helper()
	if got, want := loc.Scheme+"://"+loc.Host+loc.Path, "https://psychichomily.com/auth"; got != want {
		t.Errorf("redirects to %q, want %q", got, want)
	}
	want := url.Values{
		authPageErrorParam:     {rateLimitedTestMessage},
		authPageErrorCodeParam: {OAuthRedirectRateLimitedCode},
	}
	if returnTo != "" {
		want.Set(oauthReturnToParam, returnTo)
	}
	if got := loc.Query(); got.Encode() != want.Encode() {
		t.Errorf("query = %q, want exactly %q", got.Encode(), want.Encode())
	}
}

// assertSettingsPage requires the refused link redirect: the Settings tab with
// the code and nothing else.
func assertSettingsPage(t *testing.T, loc *url.URL) {
	t.Helper()
	if got, want := loc.Scheme+"://"+loc.Host+loc.Path, "https://psychichomily.com"+oauthLinkSettingsPath; got != want {
		t.Errorf("redirects to %q, want %q", got, want)
	}
	want := url.Values{
		oauthLinkSettingsTabKey: {oauthLinkSettingsTabValue},
		oauthLinkErrorParam:     {OAuthRedirectRateLimitedCode},
	}
	if got := loc.Query(); got.Encode() != want.Encode() {
		t.Errorf("query = %q, want exactly %q", got.Encode(), want.Encode())
	}
}

func returnToCookie(returnTo string) *http.Cookie {
	return &http.Cookie{Name: oauthReturnToCookieName, Value: base64.RawURLEncoding.EncodeToString([]byte(returnTo))}
}

func TestRedirectRateLimitedSendsTheBrowserToTheConfiguredAuthPage(t *testing.T) {
	assertAuthPage(t, refuse(t, OAuthCallbackRoute, "/auth/callback/google?code=spent", "google"), "")
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

	assertSettingsPage(t, refuse(t, OAuthCallbackRoute, "/auth/callback/google?state=link-state&code=spent", "google", intentCookie))
	if _, found := peekOAuthLinkIntent(intentID); !found {
		t.Error("the refusal consumed the link intent; only the callback handler may")
	}

	// The intent is about one handshake. A sign-in that merely still carries the
	// cookie is refused to the auth page like any other.
	assertAuthPage(t, refuse(t, OAuthCallbackRoute, "/auth/callback/google?state=other-state&code=spent", "google", intentCookie), "")
	assertAuthPage(t, refuse(t, OAuthCallbackRoute, "/auth/callback/github?state=link-state&code=spent", "github", intentCookie), "")

	// A cookie naming an intent this process does not hold is no link either.
	assertAuthPage(t, refuse(t, OAuthCallbackRoute, "/auth/callback/google?state=link-state&code=spent", "google",
		&http.Cookie{Name: oauthLinkIntentCookieName, Value: "no-such-intent"}), "")
}

func TestRedirectRateLimitedReturnsALinkStartToSettings(t *testing.T) {
	assertSettingsPage(t, refuse(t, OAuthLinkRoute, "/auth/link/google?t=one-time-token", "google"))

	// A link start names no destination; one in its query is not read.
	assertSettingsPage(t, refuse(t, OAuthLinkRoute, "/auth/link/google?t=one-time-token&returnTo=%2Fshows", "google"))
}

// Only the link route is a link start. A sign-in start, or a request whose
// route pattern chi never set, goes to the auth page.
func TestRedirectRateLimitedSendsOtherRoutesToTheAuthPage(t *testing.T) {
	assertAuthPage(t, refuse(t, OAuthLoginRoute, "/auth/login/google", "google"), "")
	assertAuthPage(t, refuse(t, "", "/auth/link/google", "google"), "")
}

// A refused sign-in start carries the destination its query names, checked
// as the start handler checks it.
func TestRedirectRateLimitedCarriesTheStartDestination(t *testing.T) {
	assertAuthPage(t, refuse(t, OAuthLoginRoute, "/auth/login/google?returnTo=%2Fprofile%3Ftab%3Dsettings", "google"), "/profile?tab=settings")
	assertAuthPage(t, refuse(t, OAuthLoginRoute, "/auth/login/google?returnTo=https%3A%2F%2Fevil.example", "google"), "")
	assertAuthPage(t, refuse(t, OAuthLoginRoute, "/auth/login/google?returnTo=%2Fauth", "google"), "")

	// A start never reads the cookie: it belongs to an earlier start.
	assertAuthPage(t, refuse(t, OAuthLoginRoute, "/auth/login/google", "google", returnToCookie("/shows")), "")
}

// A refused callback carries the destination its start recorded and clears
// the cookie, as the callback handler does. Its query is never read: the
// provider builds the callback URL.
func TestRedirectRateLimitedCarriesTheCallbackDestination(t *testing.T) {
	rr := refuseRecorded(t, OAuthCallbackRoute, "/auth/callback/google?state=s&code=spent&returnTo=%2Fadmin", "google", returnToCookie("/shows?city=phoenix"))
	loc, err := url.Parse(rr.Header().Get("Location"))
	if err != nil {
		t.Fatalf("Location %q does not parse: %v", rr.Header().Get("Location"), err)
	}
	assertAuthPage(t, loc, "/shows?city=phoenix")

	cleared := false
	for _, c := range rr.Result().Cookies() {
		if c.Name == oauthReturnToCookieName && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Error("the refused callback left the destination cookie set; it is single use")
	}

	// A cookie that does not survive the check carries nothing.
	assertAuthPage(t, refuse(t, OAuthCallbackRoute, "/auth/callback/google?code=spent", "google", returnToCookie("//evil.example")), "")
	assertAuthPage(t, refuse(t, OAuthCallbackRoute, "/auth/callback/google?code=spent", "google",
		&http.Cookie{Name: oauthReturnToCookieName, Value: "not base64!"}), "")
}
