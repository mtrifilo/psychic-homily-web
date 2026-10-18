package auth

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

const handshakeTestSecret = "handshake-test-secret-at-least-32-characters"

// handshakeCookieHandler is an OAuth handler whose sign-in completion fails,
// so a callback runs every cookie read and clear and then redirects, with
// cookies marked Secure or not.
func handshakeCookieHandler(secure bool) *OAuthHTTPHandler {
	h := failingCallbackHandler(errors.New("provider refused"))
	h.config.JWT.SecretKey = handshakeTestSecret
	h.config.Session.Secure = secure
	h.config.Email.FrontendURL = testFrontendOrigin
	return h
}

// startLinkFromSettings runs a link start that passes every gate.
func startLinkFromSettings(t *testing.T, h *OAuthHTTPHandler) *http.Response {
	t.Helper()
	user := &authm.User{ID: 7}
	token, err := mintOAuthLinkToken(handshakeTestSecret, user.ID)
	if err != nil {
		t.Fatalf("mintOAuthLinkToken: %v", err)
	}
	w, req := newLinkStartRequest("google", user, time.Now())
	req.URL.RawQuery = url.Values{oauthLinkTokenParam: {token}}.Encode()
	h.OAuthLinkHTTPHandler(w, req)
	return w.Result()
}

// handshakeCookieCase is one of the four handshake cookies, with the start
// that sets it. The callback clears every one of them; beforeClear arranges
// the state that makes it do so.
type handshakeCookieCase struct {
	cookie      oauthHandshakeCookie
	set         func(t *testing.T, h *OAuthHTTPHandler) *http.Response
	beforeClear func(sent *http.Cookie)
}

func handshakeCookieCases() []handshakeCookieCase {
	signUp := "signup_intent=1&terms_accepted=true&age_confirmed=true&min_age_attested=16&" + currentConsentVersionsQuery
	startSignInWith := func(query string) func(*testing.T, *OAuthHTTPHandler) *http.Response {
		return func(_ *testing.T, h *OAuthHTTPHandler) *http.Response { return startSignInOn(h, query) }
	}
	return []handshakeCookieCase{
		{cookie: oauthSignupConsentCookie, set: startSignInWith(signUp)},
		{cookie: cliCallbackIDCookie, set: startSignInWith("cli_callback=" + url.QueryEscape("http://localhost:8888/cb"))},
		{cookie: oauthReturnToCookie, set: startSignInWith("returnTo=" + url.QueryEscape("/shows"))},
		{
			cookie: oauthLinkIntentCookie,
			set:    startLinkFromSettings,
			// An intent this process no longer holds is the callback branch
			// that clears without completing a link.
			beforeClear: func(sent *http.Cookie) { consumeOAuthLinkIntent(sent.Value) },
		},
	}
}

// assertHandshakeCookie checks the attributes every handshake set and clear
// carries, and that a prefixed name meets the rules a browser holds the
// "__Host-" prefix to.
func assertHandshakeCookie(t *testing.T, resp *http.Response, wantName string, secure bool) *http.Cookie {
	t.Helper()
	var matches []string
	for _, header := range resp.Header.Values("Set-Cookie") {
		if strings.HasPrefix(header, wantName+"=") {
			matches = append(matches, header)
		}
	}
	if len(matches) != 1 {
		t.Fatalf("want one Set-Cookie for %s, got %q", wantName, resp.Header.Values("Set-Cookie"))
	}
	raw := matches[0]
	cookie := findCookie(resp, wantName)
	if cookie.Path != "/" || cookie.Domain != "" {
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
			wantName := tc.cookie.name
			if secure {
				wantName = oauthHostCookiePrefix + tc.cookie.name
			}
			t.Run(wantName, func(t *testing.T) {
				h := handshakeCookieHandler(secure)

				setResp := tc.set(t, h)
				set := assertHandshakeCookie(t, setResp, wantName, secure)
				if wantMaxAge := int(tc.cookie.lifetime / time.Second); set.Value == "" || set.MaxAge != wantMaxAge {
					t.Errorf("set: want a value and Max-Age %d, got value %q Max-Age %d", wantMaxAge, set.Value, set.MaxAge)
				}
				if secure && findCookie(setResp, tc.cookie.name) != nil {
					t.Errorf("set: a Secure deployment also wrote the bare name %s", tc.cookie.name)
				}

				sent := &http.Cookie{Name: set.Name, Value: set.Value}
				if tc.beforeClear != nil {
					tc.beforeClear(sent)
				}
				clearResp, _ := runCallback(t, h, "/auth/callback/google", sent)
				cleared := assertHandshakeCookie(t, clearResp, wantName, secure)
				if cleared.Value != "" || cleared.MaxAge >= 0 {
					t.Errorf("clear: want an empty value and an expired Max-Age, got value %q Max-Age %d", cleared.Value, cleared.MaxAge)
				}
			})
		}
	}
}

// The rollover read of the bare name lasts as long as oauthHandshakeCookieMaxLifetime,
// so no handshake cookie may outlive it.
func TestOAuthHandshakeCookies_LifetimesFitTheRolloverWindow(t *testing.T) {
	for _, tc := range handshakeCookieCases() {
		if tc.cookie.lifetime <= 0 || tc.cookie.lifetime > oauthHandshakeCookieMaxLifetime {
			t.Errorf("%s lives %v, want (0, %v]", tc.cookie.name, tc.cookie.lifetime, oauthHandshakeCookieMaxLifetime)
		}
	}
}

// A browser that started a handshake before the prefix was deployed sends the
// bare name to a callback that sets the prefixed one. Consent and return-to
// hold their state in the cookie, so the callback still reads them.
func TestOAuthHandshakeCookies_SecureCallbackReadsAndClearsABareStateCookie(t *testing.T) {
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

		resp, _ := runCallback(t, h, "/auth/callback/google", &http.Cookie{Name: oauthSignupConsentCookieName, Value: encoded})

		if received == nil || !received.TermsAccepted {
			t.Errorf("want the consent the bare cookie carried, got %+v", received)
		}
		assertHandshakeCookie(t, resp, oauthSignupConsentCookieName, true)
	})

	t.Run(oauthReturnToCookieName, func(t *testing.T) {
		resp, loc := runCallback(t, handshakeCookieHandler(true), "/auth/callback/google",
			&http.Cookie{Name: oauthReturnToCookieName, Value: encodedReturnTo("/shows")})

		if parsed, err := url.Parse(loc); err != nil || parsed.Query().Get(oauthReturnToParam) != "/shows" {
			t.Errorf("want the bare cookie's destination on the refusal, got %q", loc)
		}
		assertHandshakeCookie(t, resp, oauthReturnToCookieName, true)
	})
}

// cli_callback_id and the link intent hold ids into process-local stores, so a
// bare name a previous deploy set can name nothing this process holds. When
// Secure, the only bare id a callback can find is one a sibling subdomain
// planted, and the callback must not act on it: a planted CLI id would send
// the reader's session token to the planter's loopback.
func TestOAuthHandshakeCookies_SecureCallbackIgnoresABareStoreID(t *testing.T) {
	defer cleanCLICallbackStore()

	t.Run(cliCallbackIDCookieName, func(t *testing.T) {
		callbackID, err := randomHexID(16)
		if err != nil {
			t.Fatal(err)
		}
		storeCLICallback(callbackID, "http://localhost:8888/planted")

		resp, loc := runCallback(t, handshakeCookieHandler(true), "/auth/callback/google",
			&http.Cookie{Name: cliCallbackIDCookieName, Value: callbackID})

		if !strings.HasPrefix(loc, testFrontendOrigin+oauthAuthPagePath+"?") {
			t.Errorf("want the web flow's auth page, got %q", loc)
		}
		if _, still := getCLICallback(callbackID); !still {
			t.Error("the bare id was consumed")
		}
		if findCookie(resp, cliCallbackIDCookieName) != nil {
			t.Errorf("want no Set-Cookie for a bare id that was not read, got %q", resp.Header.Values("Set-Cookie"))
		}
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
		target := "/auth/callback/google?state=" + intent.state
		bare := &http.Cookie{Name: oauthLinkIntentCookieName, Value: set.Value}

		if completesPendingLink(googleRequest(target, bare), true) {
			t.Error("the rate limiter treats a bare intent id as a pending link")
		}
		_, loc := runCallback(t, h, target, bare)
		if !strings.HasPrefix(loc, testFrontendOrigin+oauthAuthPagePath+"?") {
			t.Errorf("want the sign-in refusal, not a link result, got %q", loc)
		}
		if _, _, still := peekOAuthLinkIntent(set.Value); !still {
			t.Error("the bare id consumed the intent")
		}
	})

	t.Run("a bare id beside the prefixed one is cleared", func(t *testing.T) {
		realID, plantedID := "real-callback-id", "planted-callback-id"
		storeCLICallback(realID, "http://localhost:8888/real")
		storeCLICallback(plantedID, "http://localhost:8888/planted")

		resp, loc := runCallback(t, handshakeCookieHandler(true), "/auth/callback/google",
			&http.Cookie{Name: cliCallbackIDCookieName, Value: plantedID},
			&http.Cookie{Name: oauthHostCookiePrefix + cliCallbackIDCookieName, Value: realID})

		if !strings.HasPrefix(loc, "http://localhost:8888/real?") {
			t.Errorf("want the prefixed id's destination, got %q", loc)
		}
		assertHandshakeCookie(t, resp, oauthHostCookiePrefix+cliCallbackIDCookieName, true)
		assertHandshakeCookie(t, resp, cliCallbackIDCookieName, true)
	})
}

// A request carrying both names is read under the prefixed one, which a
// sibling subdomain cannot plant, and loses both.
func TestOAuthHandshakeCookies_PrefixedNameWinsAndBothAreCleared(t *testing.T) {
	resp, loc := runCallback(t, handshakeCookieHandler(true), "/auth/callback/google",
		&http.Cookie{Name: oauthReturnToCookieName, Value: encodedReturnTo("/shows")},
		&http.Cookie{Name: oauthHostCookiePrefix + oauthReturnToCookieName, Value: encodedReturnTo("/library")})

	if parsed, err := url.Parse(loc); err != nil || parsed.Query().Get(oauthReturnToParam) != "/library" {
		t.Errorf("want the prefixed cookie's destination, got %q", loc)
	}
	assertHandshakeCookie(t, resp, oauthHostCookiePrefix+oauthReturnToCookieName, true)
	assertHandshakeCookie(t, resp, oauthReturnToCookieName, true)
}

// Over plain http a browser never stores a "__Host-" cookie, so a deployment
// that does not mark cookies Secure reads and clears only the bare name.
func TestOAuthHandshakeCookies_InsecureDeploymentIgnoresThePrefixedName(t *testing.T) {
	prefixed := &http.Cookie{Name: oauthHostCookiePrefix + oauthReturnToCookieName, Value: encodedReturnTo("/library")}

	resp, loc := runCallback(t, handshakeCookieHandler(false), "/auth/callback/google", prefixed)

	if strings.Contains(loc, oauthReturnToParam+"=") {
		t.Errorf("want no destination from the prefixed name, got %q", loc)
	}
	if findCookie(resp, prefixed.Name) != nil {
		t.Errorf("want no Set-Cookie for %s, got %q", prefixed.Name, resp.Header.Values("Set-Cookie"))
	}
}
