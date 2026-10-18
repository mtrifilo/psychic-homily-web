package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/config"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

const handshakeTestSecret = "handshake-test-secret-at-least-32-characters"

// handshakeCookieHandler is an OAuth handler whose sign-in completion fails,
// so a callback runs every cookie read and clear and then redirects, with
// cookies marked Secure or not.
func handshakeCookieHandler(secure bool) *OAuthHTTPHandler {
	authService := &testhelpers.MockAuthService{
		OAuthCallbackWithConsentFn: func(
			http.ResponseWriter, *http.Request, string, *contracts.OAuthSignupConsent,
		) (*authm.User, string, error) {
			return nil, "", errors.New("provider refused")
		},
	}
	return NewOAuthHTTPHandler(authService, nil, &config.Config{
		JWT:     config.JWTConfig{SecretKey: handshakeTestSecret},
		Session: config.SessionConfig{Path: "/", HttpOnly: true, SameSite: "lax", Secure: secure},
		Email:   config.EmailConfig{FrontendURL: testFrontendOrigin},
	})
}

// handshakeCallback runs the callback with cookies on the request and returns
// the response.
func handshakeCallback(h *OAuthHTTPHandler, target string, cookies ...*http.Cookie) *http.Response {
	w := httptest.NewRecorder()
	h.OAuthCallbackHTTPHandler(w, googleRequest(target, cookies...))
	return w.Result()
}

// startLinkFromSettings runs a link start that passes every gate, the way one
// arrives from Settings.
func startLinkFromSettings(t *testing.T, h *OAuthHTTPHandler) *http.Response {
	t.Helper()
	user := &authm.User{ID: 7}
	token, err := mintOAuthLinkToken(handshakeTestSecret, user.ID)
	if err != nil {
		t.Fatalf("mintOAuthLinkToken: %v", err)
	}
	req := httptest.NewRequest("GET", "/auth/link/google?"+url.Values{oauthLinkTokenParam: {token}}.Encode(), nil)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Origin", testFrontendOrigin)
	ctx := testhelpers.CtxWithUser(user)
	ctx = context.WithValue(ctx, middleware.SessionAuthTimeContextKey, time.Now())
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("provider", "google")
	ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)

	w := httptest.NewRecorder()
	h.OAuthLinkHTTPHandler(w, req.WithContext(ctx))
	return w.Result()
}

func startSignInWith(h *OAuthHTTPHandler, query string) *http.Response {
	w := httptest.NewRecorder()
	h.OAuthLoginHTTPHandler(w, googleRequest("/auth/login/google?"+query))
	return w.Result()
}

// handshakeCookieCases are the four cookies that carry state from an OAuth
// start to its callback, each with the start that sets it and the callback
// that clears it.
func handshakeCookieCases() []struct {
	name   string
	maxAge int
	set    func(t *testing.T, h *OAuthHTTPHandler) *http.Response
	clear  func(h *OAuthHTTPHandler, sent *http.Cookie) *http.Response
} {
	signUp := "signup_intent=1&terms_accepted=true&age_confirmed=true&min_age_attested=16&" + currentConsentVersionsQuery
	return []struct {
		name   string
		maxAge int
		set    func(t *testing.T, h *OAuthHTTPHandler) *http.Response
		clear  func(h *OAuthHTTPHandler, sent *http.Cookie) *http.Response
	}{
		{
			name:   oauthSignupConsentCookieName,
			maxAge: oauthSignupConsentCookieMaxAge,
			set: func(_ *testing.T, h *OAuthHTTPHandler) *http.Response {
				return startSignInWith(h, signUp)
			},
			clear: func(h *OAuthHTTPHandler, sent *http.Cookie) *http.Response {
				return handshakeCallback(h, "/auth/callback/google", sent)
			},
		},
		{
			name:   cliCallbackIDCookieName,
			maxAge: int(cliCallbackTTL / time.Second),
			set: func(_ *testing.T, h *OAuthHTTPHandler) *http.Response {
				return startSignInWith(h, "cli_callback="+url.QueryEscape("http://localhost:8888/cb"))
			},
			clear: func(h *OAuthHTTPHandler, sent *http.Cookie) *http.Response {
				return handshakeCallback(h, "/auth/callback/google", sent)
			},
		},
		{
			name:   oauthReturnToCookieName,
			maxAge: oauthReturnToCookieMaxAge,
			set: func(_ *testing.T, h *OAuthHTTPHandler) *http.Response {
				return startSignInWith(h, "returnTo="+url.QueryEscape("/shows"))
			},
			clear: func(h *OAuthHTTPHandler, sent *http.Cookie) *http.Response {
				return handshakeCallback(h, "/auth/callback/google", sent)
			},
		},
		{
			name:   oauthLinkIntentCookieName,
			maxAge: int(oauthLinkIntentTTL / time.Second),
			set:    startLinkFromSettings,
			clear: func(h *OAuthHTTPHandler, sent *http.Cookie) *http.Response {
				// An intent this process no longer holds is the callback branch
				// that clears without completing a link.
				consumeOAuthLinkIntent(sent.Value)
				return handshakeCallback(h, "/auth/callback/google", sent)
			},
		},
	}
}

// assertHandshakeCookie checks the attributes every handshake set and clear
// carries, and that a prefixed name meets the rules a browser holds the
// "__Host-" prefix to.
func assertHandshakeCookie(t *testing.T, resp *http.Response, wantName string, secure bool) *http.Cookie {
	t.Helper()
	var raw string
	for _, header := range resp.Header.Values("Set-Cookie") {
		if strings.HasPrefix(header, wantName+"=") {
			if raw != "" {
				t.Fatalf("%s is written twice: %q and %q", wantName, raw, header)
			}
			raw = header
		}
	}
	if raw == "" {
		t.Fatalf("no Set-Cookie for %s in %q", wantName, resp.Header.Values("Set-Cookie"))
	}
	cookie := findCookie(resp, wantName)
	if cookie.Path != "/" || cookie.Domain != "" || strings.Contains(strings.ToLower(raw), "domain=") {
		t.Errorf("%s: want Path=/ and no Domain, got %q", wantName, raw)
	}
	if !cookie.HttpOnly {
		t.Errorf("%s: want HttpOnly, got %q", wantName, raw)
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("%s: want SameSite=Lax, got %q", wantName, raw)
	}
	if cookie.Secure != secure {
		t.Errorf("%s: want Secure=%v, got %q", wantName, secure, raw)
	}
	if strings.HasPrefix(wantName, oauthHostCookiePrefix) && !cookie.Secure {
		t.Errorf("%s: a __Host- cookie must be Secure, got %q", wantName, raw)
	}
	return cookie
}

func TestOAuthHandshakeCookies_SetAndClearCarryTheSameAttributes(t *testing.T) {
	defer cleanCLICallbackStore()

	for _, secure := range []bool{false, true} {
		for _, tc := range handshakeCookieCases() {
			wantName := tc.name
			if secure {
				wantName = oauthHostCookiePrefix + tc.name
			}
			t.Run(wantName, func(t *testing.T) {
				h := handshakeCookieHandler(secure)

				setResp := tc.set(t, h)
				set := assertHandshakeCookie(t, setResp, wantName, secure)
				if set.Value == "" || set.MaxAge != tc.maxAge {
					t.Errorf("set: want a value and Max-Age %d, got value %q Max-Age %d", tc.maxAge, set.Value, set.MaxAge)
				}
				if secure && findCookie(setResp, tc.name) != nil {
					t.Errorf("set: a Secure deployment also wrote the bare name %s", tc.name)
				}

				clearResp := tc.clear(h, &http.Cookie{Name: set.Name, Value: set.Value})
				cleared := assertHandshakeCookie(t, clearResp, wantName, secure)
				if cleared.Value != "" || cleared.MaxAge >= 0 {
					t.Errorf("clear: want an empty value and an expired Max-Age, got value %q Max-Age %d", cleared.Value, cleared.MaxAge)
				}
			})
		}
	}
}

// A browser that started a handshake before the prefix was deployed sends the
// bare name to a callback that sets the prefixed one.
func TestOAuthHandshakeCookies_SecureCallbackReadsAndClearsTheBareName(t *testing.T) {
	defer cleanCLICallbackStore()

	t.Run(cliCallbackIDCookieName, func(t *testing.T) {
		callbackID, err := randomHexID(16)
		if err != nil {
			t.Fatal(err)
		}
		storeCLICallback(callbackID, "http://localhost:8888/cb")
		h := handshakeCookieHandler(true)

		resp := handshakeCallback(h, "/auth/callback/google", &http.Cookie{Name: cliCallbackIDCookieName, Value: callbackID})

		if loc := resp.Header.Get("Location"); !strings.HasPrefix(loc, "http://localhost:8888/cb?") {
			t.Errorf("want the stored loopback destination, got %q", loc)
		}
		assertHandshakeCookie(t, resp, cliCallbackIDCookieName, true)
	})

	t.Run(oauthSignupConsentCookieName, func(t *testing.T) {
		encoded, err := encodeOAuthSignupConsent(contracts.OAuthSignupConsent{TermsAccepted: true, AcceptedAt: time.Now()})
		if err != nil {
			t.Fatal(err)
		}
		var received *contracts.OAuthSignupConsent
		h := handshakeCookieHandler(true)
		h.authService = &testhelpers.MockAuthService{
			OAuthCallbackWithConsentFn: func(
				_ http.ResponseWriter, _ *http.Request, _ string, consent *contracts.OAuthSignupConsent,
			) (*authm.User, string, error) {
				received = consent
				return nil, "", errors.New("provider refused")
			},
		}

		resp := handshakeCallback(h, "/auth/callback/google", &http.Cookie{Name: oauthSignupConsentCookieName, Value: encoded})

		if received == nil || !received.TermsAccepted {
			t.Errorf("want the consent the bare cookie carried, got %+v", received)
		}
		assertHandshakeCookie(t, resp, oauthSignupConsentCookieName, true)
	})

	t.Run(oauthReturnToCookieName, func(t *testing.T) {
		resp := handshakeCallback(handshakeCookieHandler(true), "/auth/callback/google",
			&http.Cookie{Name: oauthReturnToCookieName, Value: encodedReturnTo("/shows")})

		loc, err := url.Parse(resp.Header.Get("Location"))
		if err != nil || loc.Query().Get(oauthReturnToParam) != "/shows" {
			t.Errorf("want the bare cookie's destination on the refusal, got %q", resp.Header.Get("Location"))
		}
		assertHandshakeCookie(t, resp, oauthReturnToCookieName, true)
	})

	t.Run(oauthLinkIntentCookieName, func(t *testing.T) {
		h := handshakeCookieHandler(true)
		set := findCookie(startLinkFromSettings(t, h), oauthHostCookiePrefix+oauthLinkIntentCookieName)
		if set == nil {
			t.Fatal("expected the link start to arm a prefixed intent")
		}
		defer consumeOAuthLinkIntent(set.Value)
		intent, _, found := peekOAuthLinkIntent(set.Value)
		if !found {
			t.Fatal("expected the armed intent in the store")
		}
		bare := &http.Cookie{Name: oauthLinkIntentCookieName, Value: set.Value}

		if !completesPendingLink(googleRequest("/auth/callback/google?state="+intent.state, bare), true) {
			t.Error("the rate limiter does not see the pending link the bare name carries")
		}

		consumeOAuthLinkIntent(set.Value)
		assertHandshakeCookie(t, handshakeCallback(h, "/auth/callback/google", bare), oauthLinkIntentCookieName, true)
	})
}

// A request carrying both names is read under the prefixed one, which a
// sibling subdomain cannot plant, and loses both.
func TestOAuthHandshakeCookies_PrefixedNameWinsAndBothAreCleared(t *testing.T) {
	resp := handshakeCallback(handshakeCookieHandler(true), "/auth/callback/google",
		&http.Cookie{Name: oauthReturnToCookieName, Value: encodedReturnTo("/shows")},
		&http.Cookie{Name: oauthHostCookiePrefix + oauthReturnToCookieName, Value: encodedReturnTo("/library")})

	loc, err := url.Parse(resp.Header.Get("Location"))
	if err != nil || loc.Query().Get(oauthReturnToParam) != "/library" {
		t.Errorf("want the prefixed cookie's destination, got %q", resp.Header.Get("Location"))
	}
	assertHandshakeCookie(t, resp, oauthHostCookiePrefix+oauthReturnToCookieName, true)
	assertHandshakeCookie(t, resp, oauthReturnToCookieName, true)
}

// Over plain http a browser never stores a "__Host-" cookie, so a deployment
// that does not mark cookies Secure reads and clears only the bare name.
func TestOAuthHandshakeCookies_InsecureDeploymentIgnoresThePrefixedName(t *testing.T) {
	prefixed := &http.Cookie{Name: oauthHostCookiePrefix + oauthReturnToCookieName, Value: encodedReturnTo("/library")}

	resp := handshakeCallback(handshakeCookieHandler(false), "/auth/callback/google", prefixed)

	if loc := resp.Header.Get("Location"); strings.Contains(loc, oauthReturnToParam+"=") {
		t.Errorf("want no destination from the prefixed name, got %q", loc)
	}
	if findCookie(resp, prefixed.Name) != nil {
		t.Errorf("want no Set-Cookie for %s, got %q", prefixed.Name, resp.Header.Values("Set-Cookie"))
	}
}
