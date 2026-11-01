package auth

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
	"time"

	authm "psychic-homily-backend/internal/models/auth"
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
	return startLinkFromSettingsWith(t, h)
}

// startLinkFromSettingsWith is startLinkFromSettings from a browser carrying
// cookies.
func startLinkFromSettingsWith(t *testing.T, h *OAuthHTTPHandler, cookies ...*http.Cookie) *http.Response {
	t.Helper()
	user := &authm.User{ID: 7}
	token, err := mintOAuthLinkToken(handshakeTestSecret, user.ID)
	if err != nil {
		t.Fatalf("mintOAuthLinkToken: %v", err)
	}
	w, req := newLinkStartRequest("google", user, time.Now())
	req.URL.RawQuery = url.Values{oauthLinkTokenParam: {token}}.Encode()
	for _, c := range cookies {
		req.AddCookie(c)
	}
	h.OAuthLinkHTTPHandler(w, req)
	return w.Result()
}

// handshakeCookieCase is one of the three handshake cookies a sign-in or link
// start sets, with that start. The callback clears every one of them; beforeClear arranges
// the state that makes it do so.
type handshakeCookieCase struct {
	cookie      oauthHandshakeCookie
	set         func(t *testing.T, h *OAuthHTTPHandler) *http.Response
	beforeClear func(sent *http.Cookie)
}

func handshakeCookieCases() []handshakeCookieCase {
	startSignInWith := func(query string) func(*testing.T, *OAuthHTTPHandler) *http.Response {
		return func(_ *testing.T, h *OAuthHTTPHandler) *http.Response { return startSignInOn(h, query) }
	}
	return []handshakeCookieCase{
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

// assertHandshakeCookieCleared checks that resp expires the handshake cookie
// wantName with the attributes every handshake set carries.
func assertHandshakeCookieCleared(t *testing.T, resp *http.Response, wantName string, secure bool) {
	t.Helper()
	cleared := assertHandshakeCookie(t, resp, wantName, secure)
	if cleared.Value != "" || cleared.MaxAge >= 0 {
		t.Errorf("%s: want an empty value and an expired Max-Age, got value %q Max-Age %d", wantName, cleared.Value, cleared.MaxAge)
	}
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
				assertHandshakeCookieCleared(t, clearResp, wantName, secure)
			})
		}
	}
}

func TestOAuthHandshakeCookies_LifetimesFitOneSignIn(t *testing.T) {
	every := []oauthHandshakeCookie{
		cliCallbackIDCookie,
		oauthReturnToCookie,
		oauthLinkIntentCookie,
		oauthPendingSignupCookie,
		oauthPendingLinkCookie,
	}
	for _, c := range every {
		if c.lifetime <= 0 || c.lifetime > oauthHandshakeCookieMaxLifetime {
			t.Errorf("%s lives %v, want (0, %v]", c.name, c.lifetime, oauthHandshakeCookieMaxLifetime)
		}
	}
}

// A callback reads a handshake cookie under the one name the deployment sets
// it under: the prefixed name when Secure, the bare name over plain http,
// where a browser never stores a "__Host-" cookie. A request carrying only
// the other name has no such cookie, and that name gets no Set-Cookie.
func TestOAuthHandshakeCookies_CallbackReadsOnlyTheNameTheDeploymentSets(t *testing.T) {
	prefixed := oauthHostCookiePrefix + oauthReturnToCookieName
	cases := []struct {
		secure   bool
		sent     string
		wantRead bool
	}{
		{secure: true, sent: prefixed, wantRead: true},
		{secure: true, sent: oauthReturnToCookieName, wantRead: false},
		{secure: false, sent: oauthReturnToCookieName, wantRead: true},
		{secure: false, sent: prefixed, wantRead: false},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("secure=%v/%s", tc.secure, tc.sent), func(t *testing.T) {
			resp, loc := runCallback(t, handshakeCookieHandler(tc.secure), "/auth/callback/google",
				&http.Cookie{Name: tc.sent, Value: encodedReturnTo("/library")})

			parsed, err := url.Parse(loc)
			if err != nil {
				t.Fatalf("unparseable Location %q: %v", loc, err)
			}
			want := ""
			if tc.wantRead {
				want = "/library"
			}
			if got := parsed.Query().Get(oauthReturnToParam); got != want {
				t.Errorf("destination = %q, want %q (Location %q)", got, want, loc)
			}
			if tc.wantRead {
				assertHandshakeCookieCleared(t, resp, tc.sent, tc.secure)
			} else if findCookie(resp, tc.sent) != nil {
				t.Errorf("want no Set-Cookie for %s, got %q", tc.sent, resp.Header.Values("Set-Cookie"))
			}
		})
	}
}

// When Secure, a bare cli_callback_id or link intent id is one a sibling
// subdomain can plant, and the callback must not act on it: a planted CLI id
// would send the reader's session token to the planter's loopback.
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
		assertHandshakeCookieCleared(t, resp, oauthHostCookiePrefix+cliCallbackIDCookieName, true)
		assertHandshakeCookieCleared(t, resp, cliCallbackIDCookieName, true)
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
	assertHandshakeCookieCleared(t, resp, oauthHostCookiePrefix+oauthReturnToCookieName, true)
	assertHandshakeCookieCleared(t, resp, oauthReturnToCookieName, true)
}

// browserSession is one browser's cookie jar against the API origin, which is
// https when the deployment marks cookies Secure: the jar keeps and sends a
// Secure cookie only over https, as a browser does.
type browserSession struct {
	jar    *cookiejar.Jar
	origin *url.URL
}

func newBrowserSession(t *testing.T, secure bool) browserSession {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	origin := &url.URL{Scheme: "http", Host: "api.test"}
	if secure {
		origin.Scheme = "https"
	}
	return browserSession{jar: jar, origin: origin}
}

func (b browserSession) keep(resp *http.Response) { b.jar.SetCookies(b.origin, resp.Cookies()) }

func (b browserSession) cookies() []*http.Cookie { return b.jar.Cookies(b.origin) }

// abandonedCLIStart is a sign-in handler and a browser that ran a CLI sign-in
// start whose callback never arrived: the browser holds its id, returned, and
// the store its destination.
func abandonedCLIStart(t *testing.T, secure bool) (*OAuthHTTPHandler, browserSession, string) {
	t.Helper()
	h := signInCallbackHandler()
	h.config.Session.Secure = secure
	browser := newBrowserSession(t, secure)
	resp := startSignInOn(h, "cli_callback="+url.QueryEscape("http://localhost:8888/abandoned"))
	set := findCookie(resp, cliCallbackIDCookie.setName(secure))
	if set == nil {
		t.Fatal("expected the CLI start to set its callback id")
	}
	browser.keep(resp)
	return h, browser, set.Value
}

// A web start in a browser still holding an abandoned CLI start's id spends
// it, so the web sign-in's callback lands on the site and the session token
// never reaches that start's loopback.
func TestOAuthLogin_WebStartDiscardsAnAbandonedCLIStart(t *testing.T) {
	defer cleanCLICallbackStore()

	for _, secure := range []bool{false, true} {
		name := cliCallbackIDCookie.setName(secure)
		t.Run(name, func(t *testing.T) {
			h, browser, callbackID := abandonedCLIStart(t, secure)

			webStart := startSignInOn(h, "returnTo="+url.QueryEscape("/shows"), browser.cookies()...)
			browser.keep(webStart)

			_, loc := runCallback(t, h, "/auth/callback/google", browser.cookies()...)
			if loc != errorRedirectFrontendURL+"/shows" {
				t.Errorf("want the web sign-in's destination on the site, got %q", loc)
			}
			assertHandshakeCookieCleared(t, webStart, name, secure)
			if _, still := getCLICallback(callbackID); still {
				t.Error("the abandoned start's destination is still stored")
			}
		})
	}
}

// A link start spends an abandoned CLI start's id the same way.
func TestOAuthLink_StartDiscardsAnAbandonedCLIStart(t *testing.T) {
	defer cleanCLICallbackStore()

	for _, secure := range []bool{false, true} {
		name := cliCallbackIDCookie.setName(secure)
		t.Run(name, func(t *testing.T) {
			const callbackID = "abandoned-before-link"
			storeCLICallback(callbackID, "http://localhost:8888/abandoned")

			resp := startLinkFromSettingsWith(t, handshakeCookieHandler(secure), &http.Cookie{Name: name, Value: callbackID})
			if set := findCookie(resp, oauthLinkIntentCookie.setName(secure)); set == nil {
				t.Fatalf("expected the link start to arm an intent, got %q", resp.Header.Values("Set-Cookie"))
			} else {
				defer consumeOAuthLinkIntent(set.Value)
			}
			assertHandshakeCookieCleared(t, resp, name, secure)
			if _, still := getCLICallback(callbackID); still {
				t.Error("the abandoned start's destination is still stored")
			}
		})
	}
}

// A web start sets nothing under the CLI id's names when the browser holds no
// id read would find: none at all, or, when Secure, only a bare one, which the
// callback ignores too.
func TestOAuthLogin_WebStartLeavesAnUnreadCLIIDAlone(t *testing.T) {
	defer cleanCLICallbackStore()

	const bareID = "bare-callback-id"
	storeCLICallback(bareID, "http://localhost:8888/planted")
	cases := []struct {
		name    string
		secure  bool
		cookies []*http.Cookie
	}{
		{name: "no id, plain", secure: false},
		{name: "no id, Secure", secure: true},
		{name: "bare id only, Secure", secure: true, cookies: []*http.Cookie{{Name: cliCallbackIDCookieName, Value: bareID}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := signInCallbackHandler()
			h.config.Session.Secure = tc.secure
			resp := startSignInOn(h, "returnTo="+url.QueryEscape("/shows"), tc.cookies...)
			for _, name := range cliCallbackIDCookie.clearNames(tc.secure) {
				if findCookie(resp, name) != nil {
					t.Errorf("want no Set-Cookie for %s, got %q", name, resp.Header.Values("Set-Cookie"))
				}
			}
		})
	}
	if _, still := getCLICallback(bareID); !still {
		t.Error("a bare id consumed its stored destination")
	}
}

// A CLI start after an abandoned one replaces the id, so its callback reaches
// its own loopback.
func TestOAuthLogin_CLIStartAfterAnAbandonedOneReachesItsOwnLoopback(t *testing.T) {
	defer cleanCLICallbackStore()

	for _, secure := range []bool{false, true} {
		t.Run(cliCallbackIDCookie.setName(secure), func(t *testing.T) {
			h, browser, _ := abandonedCLIStart(t, secure)

			const loopback = "http://localhost:8888/current"
			cliStart := startSignInOn(h, "cli_callback="+url.QueryEscape(loopback), browser.cookies()...)
			assertHandshakeCookie(t, cliStart, cliCallbackIDCookie.setName(secure), secure)
			browser.keep(cliStart)

			_, loc := runCallback(t, h, "/auth/callback/google", browser.cookies()...)
			if !strings.HasPrefix(loc, loopback+"?token=") {
				t.Errorf("want the current CLI start's loopback with a token, got %q", loc)
			}
		})
	}
}

// A CLI id that names nothing in the store (a restart, or an id this process
// never minted) is spent and ignored: the callback is the web sign-in it
// looks like.
func TestOAuthCallback_CLIIDWithNoStoredDestinationSignsInOnTheSite(t *testing.T) {
	resp, loc := runCallback(t, signInCallbackHandler(), "/auth/callback/google",
		&http.Cookie{Name: cliCallbackIDCookieName, Value: "never-stored"})

	if loc != errorRedirectFrontendURL {
		t.Errorf("want the site, got %q", loc)
	}
	assertHandshakeCookieCleared(t, resp, cliCallbackIDCookieName, false)
}
