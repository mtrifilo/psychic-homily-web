package auth

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/config"
	autherrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

func TestSanitizeOAuthReturnTo(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"on-site path", "/shows", "/shows"},
		{"path with query", "/profile?tab=settings", "/profile?tab=settings"},
		{"path with query and fragment", "/profile?tab=settings#alerts", "/profile?tab=settings#alerts"},
		{"percent-encoded path", "/artists/bj%C3%B6rk", "/artists/bj%C3%B6rk"},
		{"query carrying separators", "/shows?a=1;b=2,3", "/shows?a=1;b=2,3"},
		{"site root", "/", "/"},
		{"empty", "", "/"},
		{"absolute URL", "https://evil.example", "/"},
		{"absolute URL with path", "https://evil.example/shows", "/"},
		{"protocol-relative", "//evil.example", "/"},
		{"protocol-relative with path", "//evil.example/shows", "/"},
		{"backslash after slash", "/\\evil.example", "/"},
		{"backslash later in the path", "/shows\\..\\x", "/"},
		{"backslash in the query", "/search?q=a\\b", "/search?q=a\\b"},
		{"backslash in the fragment", "/shows#a\\b", "/shows#a\\b"},
		{"encoded slash inside a segment", "/a/%2E%2E%2Fb", "/a/%2E%2E%2Fb"},
		{"undecodable segment", "/shows/%zz", "/"},
		{"dot-dot in the query only", "/search?q=..", "/search?q=.."},
		{"tab inside", "/\t/evil.example", "/"},
		{"newline inside", "/shows\r\nSet-Cookie: x=1", "/"},
		{"space inside", "/shows here", "/"},
		{"leading space", " /shows", "/"},
		{"non-ASCII byte", "/artists/björk", "/"},
		{"relative path", "shows", "/"},
		{"scheme only", "javascript:alert(1)", "/"},
		{"dot-dot segment", "/shows/../auth", "/"},
		{"dot-dot into protocol-relative", "/..//evil.example", "/"},
		{"encoded dot-dot segment", "/shows/%2e%2e/auth", "/"},
		{"dot segment", "/./shows", "/"},
		{"auth page", "/auth", "/"},
		{"auth page with query", "/auth?returnTo=%2Fshows", "/"},
		{"under the auth page", "/auth/magic-link", "/"},
		{"encoded spelling of the auth page", "/%61uth", "/"},
		{"a path that only starts with auth", "/authors", "/authors"},
		{"too long", "/" + strings.Repeat("a", oauthReturnToMaxLength), "/"},
		{"at the length limit", "/" + strings.Repeat("a", oauthReturnToMaxLength-1), "/" + strings.Repeat("a", oauthReturnToMaxLength-1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sanitizeOAuthReturnTo(tc.raw); got != tc.want {
				t.Errorf("sanitizeOAuthReturnTo(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

// --- start: the cookie the sign-in start writes ---

// googleRequest is a GET to target as chi routes it: the provider path
// parameter set to google.
func googleRequest(target string, cookies ...*http.Cookie) *http.Request {
	req := httptest.NewRequest("GET", target, nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("provider", "google")
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func startSignIn(query string, cookies ...*http.Cookie) *http.Response {
	target := "/auth/login/google"
	if query != "" {
		target += "?" + query
	}
	w := httptest.NewRecorder()
	NewOAuthHTTPHandler(nil, nil, nil).OAuthLoginHTTPHandler(w, googleRequest(target, cookies...))
	return w.Result()
}

func findCookie(resp *http.Response, name string) *http.Cookie {
	for _, c := range resp.Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func encodedReturnTo(path string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(path))
}

func TestOAuthLogin_RecordsAnOnSiteReturnTo(t *testing.T) {
	resp := startSignIn("returnTo=" + url.QueryEscape("/profile?tab=settings"))

	cookie := findCookie(resp, oauthReturnToCookieName)
	if cookie == nil {
		t.Fatal("expected the return-to cookie to be set")
	}
	if cookie.Value != encodedReturnTo("/profile?tab=settings") {
		t.Errorf("cookie value = %q, want the encoded destination", cookie.Value)
	}
	if cookie.MaxAge != oauthReturnToCookieMaxAge || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" {
		t.Errorf("cookie attributes = %+v, want HttpOnly, Lax, path /, MaxAge %d", cookie, oauthReturnToCookieMaxAge)
	}
}

// TestSanitizeOAuthReturnTo owns the refusal matrix; this pins only that the
// start consults it.
func TestOAuthLogin_RecordsNoRefusedReturnTo(t *testing.T) {
	for _, query := range []string{"returnTo=" + url.QueryEscape("//evil.example"), ""} {
		if cookie := findCookie(startSignIn(query), oauthReturnToCookieName); cookie != nil {
			t.Errorf("start query %q: expected no return-to cookie, got %+v", query, cookie)
		}
	}
}

// An abandoned start leaves its cookie behind; a later start that names no
// destination must not inherit it.
func TestOAuthLogin_ClearsAnEarlierStartsReturnTo(t *testing.T) {
	stale := &http.Cookie{Name: oauthReturnToCookieName, Value: encodedReturnTo("/shows")}

	assertReturnToCleared(t, startSignIn("", stale))
}

// The consent gate refuses before anything is recorded.
func TestOAuthLogin_RefusedSignupRecordsNoReturnTo(t *testing.T) {
	resp := startSignIn("signup_intent=1&returnTo=%2Fshows")

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if cookie := findCookie(resp, oauthReturnToCookieName); cookie != nil {
		t.Errorf("expected no return-to cookie on a refused start, got %+v", cookie)
	}
}

// --- callback: where a successful sign-in lands ---

func signInCallbackHandler() *OAuthHTTPHandler {
	authService := &testhelpers.MockAuthService{
		OAuthCallbackWithConsentFn: func(
			http.ResponseWriter, *http.Request, string, *contracts.OAuthSignupConsent,
		) (*authm.User, string, error) {
			return &authm.User{ID: 42}, "session-token", nil
		},
	}
	return NewOAuthHTTPHandler(authService, nil, &config.Config{
		Session: config.SessionConfig{Path: "/", HttpOnly: true, SameSite: "lax"},
		Email:   config.EmailConfig{FrontendURL: errorRedirectFrontendURL},
	})
}

func runCallback(t *testing.T, handler *OAuthHTTPHandler, target string, cookies ...*http.Cookie) (*http.Response, string) {
	t.Helper()
	w := httptest.NewRecorder()
	handler.OAuthCallbackHTTPHandler(w, googleRequest(target, cookies...))
	if w.Code != http.StatusTemporaryRedirect {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusTemporaryRedirect)
	}
	return w.Result(), w.Header().Get("Location")
}

func assertReturnToCleared(t *testing.T, resp *http.Response) {
	t.Helper()
	cookie := findCookie(resp, oauthReturnToCookieName)
	if cookie == nil || cookie.MaxAge >= 0 || cookie.Value != "" {
		t.Errorf("expected the return-to cookie to be cleared, got %+v", cookie)
	}
}

func TestOAuthCallback_SuccessLandsOnTheRecordedDestination(t *testing.T) {
	cases := []struct {
		name   string
		cookie string
		want   string
	}{
		{"on-site path", encodedReturnTo("/shows"), errorRedirectFrontendURL + "/shows"},
		{"settings tab", encodedReturnTo("/profile?tab=settings"), errorRedirectFrontendURL + "/profile?tab=settings"},
		// A cookie the start never wrote is checked again, not trusted.
		{"off-site destination", encodedReturnTo("https://evil.example"), errorRedirectFrontendURL},
		{"protocol-relative destination", encodedReturnTo("//evil.example"), errorRedirectFrontendURL},
		{"auth page", encodedReturnTo("/auth"), errorRedirectFrontendURL},
		{"undecodable value", "%%%not-base64", errorRedirectFrontendURL},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, location := runCallback(t, signInCallbackHandler(), "/auth/callback/google",
				&http.Cookie{Name: oauthReturnToCookieName, Value: tc.cookie})

			if location != tc.want {
				t.Errorf("Location = %q, want %q", location, tc.want)
			}
			if findCookie(resp, "auth_token") == nil {
				t.Error("expected the session cookie on a successful sign-in")
			}
			assertReturnToCleared(t, resp)
		})
	}
}

func TestOAuthCallback_SuccessWithoutADestinationLandsOnTheRoot(t *testing.T) {
	resp, location := runCallback(t, signInCallbackHandler(), "/auth/callback/google")

	if location != errorRedirectFrontendURL {
		t.Errorf("Location = %q, want %q", location, errorRedirectFrontendURL)
	}
	if cookie := findCookie(resp, oauthReturnToCookieName); cookie != nil {
		t.Errorf("expected no return-to cookie write when none was sent, got %+v", cookie)
	}
}

// The provider builds the callback URL, so a destination on it is ignored.
func TestOAuthCallback_IgnoresADestinationOnItsOwnQuery(t *testing.T) {
	_, location := runCallback(t, signInCallbackHandler(),
		"/auth/callback/google?returnTo="+url.QueryEscape("/shows"))
	if location != errorRedirectFrontendURL {
		t.Errorf("Location = %q, want %q", location, errorRedirectFrontendURL)
	}

	_, location = runCallback(t, signInCallbackHandler(),
		"/auth/callback/google?returnTo="+url.QueryEscape("/shows"),
		&http.Cookie{Name: oauthReturnToCookieName, Value: encodedReturnTo("/library")})
	if location != errorRedirectFrontendURL+"/library" {
		t.Errorf("Location = %q, want the cookie's destination %q", location, errorRedirectFrontendURL+"/library")
	}
}

// A refused sign-in goes to the auth page with its refusal and the re-checked
// destination, so a retry from there (the consent handoff to Create account
// included) still lands where the reader started. The cookie is spent.
func TestOAuthCallback_RefusalCarriesTheDestinationToTheAuthPage(t *testing.T) {
	termsRefusal := autherrors.ErrTermsAcceptanceRequired("no consent")
	cases := []struct {
		name         string
		cookie       string
		wantReturnTo string
	}{
		{"on-site destination", encodedReturnTo("/profile?tab=settings"), "/profile?tab=settings"},
		{"off-site destination", encodedReturnTo("https://evil.example"), ""},
		{"protocol-relative destination", encodedReturnTo("//evil.example"), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, raw := runCallback(t, failingCallbackHandler(termsRefusal), "/auth/callback/google",
				&http.Cookie{Name: oauthReturnToCookieName, Value: tc.cookie})

			location, err := url.Parse(raw)
			if err != nil {
				t.Fatalf("unparseable Location %q: %v", raw, err)
			}
			if got := location.Scheme + "://" + location.Host + location.Path; got != errorRedirectFrontendURL+"/auth" {
				t.Errorf("redirect target = %q, want the auth page", got)
			}
			query := location.Query()
			if got := query.Get("error_code"); got != autherrors.CodeTermsAcceptanceRequired {
				t.Errorf("error_code = %q, want %q", got, autherrors.CodeTermsAcceptanceRequired)
			}
			if tc.wantReturnTo == "" {
				if query.Has("returnTo") {
					t.Errorf("returnTo = %q, want the parameter absent", query.Get("returnTo"))
				}
			} else if got := query.Get("returnTo"); got != tc.wantReturnTo {
				t.Errorf("returnTo = %q, want %q", got, tc.wantReturnTo)
			}
			assertReturnToCleared(t, resp)
		})
	}
}

// The CLI loopback keeps its token redirect; the cookie is spent regardless.
func TestOAuthCallback_CLILoopbackIgnoresTheDestination(t *testing.T) {
	t.Cleanup(cleanCLICallbackStore)
	const callbackID = "return-to-cli-id"
	storeCLICallback(callbackID, "http://localhost:8888/cli-cb")

	resp, location := runCallback(t, signInCallbackHandler(), "/auth/callback/google",
		&http.Cookie{Name: "cli_callback_id", Value: callbackID},
		&http.Cookie{Name: oauthReturnToCookieName, Value: encodedReturnTo("/shows")})

	if !strings.HasPrefix(location, "http://localhost:8888/cli-cb?token=") {
		t.Errorf("Location = %q, want the CLI loopback with a token", location)
	}
	assertReturnToCleared(t, resp)
}

// A link callback returns to Settings whatever the cookie names, and spends it.
func TestOAuthCallback_LinkResultIgnoresTheDestination(t *testing.T) {
	const intentID = "return-to-link-intent"
	const state = "return-to-link-state"
	storeOAuthLinkIntent(intentID, oauthLinkIntent{
		userID:    42,
		provider:  "google",
		state:     state,
		expiresAt: time.Now().Add(-time.Minute),
	})
	t.Cleanup(func() { consumeOAuthLinkIntent(intentID) })

	resp, location := runCallback(t, signInCallbackHandler(), "/auth/callback/google?state="+state,
		&http.Cookie{Name: oauthLinkIntentCookieName, Value: intentID},
		&http.Cookie{Name: oauthReturnToCookieName, Value: encodedReturnTo("/shows")})

	if !strings.HasPrefix(location, errorRedirectFrontendURL+oauthLinkSettingsPath+"?") {
		t.Errorf("Location = %q, want the Settings link result", location)
	}
	assertReturnToCleared(t, resp)
}
