package auth

import (
	"context"
	"encoding/base64"
	"errors"
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
	"psychic-homily-backend/internal/testcorpus"
)

// returnToCorpus is testdata/return_to_corpus.json, which the frontend's
// sanitizeReturnTo test reads too. Its _comment states the contract the two
// sanitizers share.
type returnToCorpus struct {
	Fallback  string `json:"fallback"`
	MaxLength int    `json:"maxLength"`
	Cases     []struct {
		Name  string `json:"name"`
		Input string `json:"input"`
		Want  string `json:"want"`
		Rule  string `json:"rule"`
	} `json:"cases"`
}

func loadReturnToCorpus(t *testing.T) returnToCorpus {
	t.Helper()
	corpus := testcorpus.Load[returnToCorpus](t, "internal/api/handlers/auth/testdata/return_to_corpus.json")
	if len(corpus.Cases) == 0 {
		t.Fatal("the corpus has no cases")
	}
	return corpus
}

func TestSanitizeOAuthReturnTo(t *testing.T) {
	corpus := loadReturnToCorpus(t)
	if corpus.Fallback != oauthReturnToFallback {
		t.Errorf("corpus fallback = %q, oauthReturnToFallback = %q", corpus.Fallback, oauthReturnToFallback)
	}
	if corpus.MaxLength != oauthReturnToMaxLength {
		t.Errorf("corpus maxLength = %d, oauthReturnToMaxLength = %d", corpus.MaxLength, oauthReturnToMaxLength)
	}
	for _, tc := range corpus.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			if got := sanitizeOAuthReturnTo(tc.Input); got != tc.Want {
				t.Errorf("sanitizeOAuthReturnTo(%q) = %q, want %q (rule: %s)", tc.Input, got, tc.Want, tc.Rule)
			}
		})
	}
}

// Every case names the rule it exercises, and the length cap has a kept case
// on it and a refused case one past it.
func TestReturnToCorpusHoldsItsContract(t *testing.T) {
	corpus := loadReturnToCorpus(t)
	keptAtTheCap, refusedPastTheCap := false, false
	for _, tc := range corpus.Cases {
		keptAtTheCap = keptAtTheCap || (len(tc.Input) == corpus.MaxLength && tc.Want == tc.Input)
		refusedPastTheCap = refusedPastTheCap || (len(tc.Input) == corpus.MaxLength+1 && tc.Want == corpus.Fallback)
		if tc.Rule == "" {
			t.Errorf("%s: rule is empty", tc.Name)
		}
	}
	if !keptAtTheCap || !refusedPastTheCap {
		t.Errorf("the corpus needs a case of exactly maxLength (%d) that is kept (found: %t) and one a byte past it that is refused (found: %t)",
			corpus.MaxLength, keptAtTheCap, refusedPastTheCap)
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
	return startSignInOn(NewOAuthHTTPHandler(nil, nil, nil), query, cookies...)
}

func startSignInOn(h *OAuthHTTPHandler, query string, cookies ...*http.Cookie) *http.Response {
	target := "/auth/login/google"
	if query != "" {
		target += "?" + query
	}
	w := httptest.NewRecorder()
	h.OAuthLoginHTTPHandler(w, googleRequest(target, cookies...))
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
	if cookie.MaxAge != int(oauthReturnToCookie.lifetime/time.Second) || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" {
		t.Errorf("cookie attributes = %+v, want HttpOnly, Lax, path /, MaxAge %d", cookie, int(oauthReturnToCookie.lifetime/time.Second))
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

// A sign-up start naming stale consent versions goes back to the auth page
// with the destination, records nothing, and never reaches the provider.
func TestOAuthLogin_StaleConsentRefusalCarriesTheDestination(t *testing.T) {
	resp := startSignIn("signup_intent=1&terms_accepted=true&terms_version=stale&privacy_version=stale" +
		"&age_confirmed=true&min_age_attested=16&returnTo=" + url.QueryEscape("/profile?tab=settings"))

	if resp.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusTemporaryRedirect)
	}
	location, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		t.Fatalf("unparseable Location: %v", err)
	}
	if location.Path != "/auth" {
		t.Errorf("redirect path = %q, want /auth", location.Path)
	}
	query := location.Query()
	if got := query.Get("error_code"); got != autherrors.CodeTermsAcceptanceRequired {
		t.Errorf("error_code = %q, want %q", got, autherrors.CodeTermsAcceptanceRequired)
	}
	if got := query.Get("returnTo"); got != "/profile?tab=settings" {
		t.Errorf("returnTo = %q, want /profile?tab=settings", got)
	}
	if cookie := findCookie(resp, oauthReturnToCookieName); cookie != nil {
		t.Errorf("expected no return-to cookie on a refused start, got %+v", cookie)
	}
}

// A link start records no destination of its own, so it clears one an
// abandoned sign-in start left behind: a link callback that resolves as a
// sign-in must not land on it.
func (s *OAuthHandlerIntegrationSuite) TestLink_ClearsAnAbandonedSignInsDestination() {
	user := &authm.User{Email: strPtr("link-return-to@test.com"), IsActive: true, EmailVerified: true}
	s.Require().NoError(s.deps.DB.Create(user).Error)

	handler := s.newHandler(&mockOAuthCompleter{})
	w, req := s.oauthLinkRequest("google", user)
	req.AddCookie(&http.Cookie{Name: oauthReturnToCookieName, Value: encodedReturnTo("/shows")})
	handler.OAuthLinkHTTPHandler(w, req)

	s.Require().NotNil(linkIntentCookie(w), "the link start armed its intent")
	assertReturnToCleared(s.T(), w.Result())
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

// A failure that is not a refusal the reader can act on (a provider error, a
// cancelled consent screen) carries the generic message, no code, and the
// destination.
func TestOAuthCallback_GenericFailureCarriesTheDestination(t *testing.T) {
	resp, raw := runCallback(t, failingCallbackHandler(errors.New("oauth2: cannot fetch token")), "/auth/callback/google",
		&http.Cookie{Name: oauthReturnToCookieName, Value: encodedReturnTo("/shows")})

	location, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("unparseable Location %q: %v", raw, err)
	}
	query := location.Query()
	if location.Path != "/auth" || query.Get("error") != "authentication failed" || query.Has("error_code") {
		t.Errorf("Location = %q, want the auth page with the generic message and no code", raw)
	}
	if got := query.Get("returnTo"); got != "/shows" {
		t.Errorf("returnTo = %q, want /shows", got)
	}
	assertReturnToCleared(t, resp)
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
// The intent is expired on purpose: that link branch answers without the auth
// service, and it shares the redirect every link result uses.
func TestOAuthCallback_LinkResultIgnoresTheDestination(t *testing.T) {
	const intentID = "return-to-link-intent"
	const state = "return-to-link-state"
	storeExpiredOAuthLinkIntent(intentID, oauthLinkIntent{
		userID:   42,
		provider: "google",
		state:    state,
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
