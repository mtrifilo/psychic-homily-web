package auth

import (
	"net/http"
	"time"
)

// oauthHostCookiePrefix binds a cookie to the host that set it: a browser
// stores a "__Host-" cookie only when it is Secure, has Path "/" and names no
// Domain, so a sibling subdomain cannot plant or shadow one. Browsers refuse
// the prefix on an insecure origin, so it is applied only when the cookie is
// Secure, and local development over plain http keeps the bare name.
const oauthHostCookiePrefix = "__Host-"

// oauthHandshakeCookieMaxLifetime bounds every oauthHandshakeCookie's
// lifetime, and so how long a browser can hold one a previous deploy set.
const oauthHandshakeCookieMaxLifetime = 10 * time.Minute

// oauthHandshakeCookie is a cookie that carries state from an OAuth start to
// its callback: host-only, script-inaccessible, sent on the provider's
// redirect back, and gone after lifetime. Its methods are the only code that
// writes or reads one, so every set and clear carries the same attributes.
type oauthHandshakeCookie struct {
	name     string
	lifetime time.Duration
}

// setName is the name this deployment sets the cookie under.
func (c oauthHandshakeCookie) setName(secure bool) string {
	if secure {
		return oauthHostCookiePrefix + c.name
	}
	return c.name
}

// readNames are the names a request may carry the cookie under, in the order
// they are read.
//
// The bare name after the prefixed one is the rollover read: a browser that
// started a handshake before the prefix was deployed still carries the bare
// name for up to oauthHandshakeCookieMaxLifetime. Remove that entry on or
// after 2026-10-24, once a production deploy that sets the prefixed names has
// been live for longer than that.
func (c oauthHandshakeCookie) readNames(secure bool) []string {
	if !secure {
		return []string{c.name}
	}
	return []string{c.setName(true), c.name}
}

func (c oauthHandshakeCookie) set(w http.ResponseWriter, value string, secure bool) {
	http.SetCookie(w, newOAuthHandshakeCookie(c.setName(secure), value, int(c.lifetime/time.Second), secure))
}

// read returns the cookie under the first of readNames the request carries.
func (c oauthHandshakeCookie) read(r *http.Request, secure bool) (*http.Cookie, bool) {
	for _, name := range c.readNames(secure) {
		if cookie, err := r.Cookie(name); err == nil {
			return cookie, true
		}
	}
	return nil, false
}

// clear expires the cookie under every name the request carries it under, so
// a value under a name read would fall back to cannot outlive the one read.
func (c oauthHandshakeCookie) clear(w http.ResponseWriter, r *http.Request, secure bool) {
	for _, name := range c.readNames(secure) {
		if _, err := r.Cookie(name); err == nil {
			http.SetCookie(w, newOAuthHandshakeCookie(name, "", -1, secure))
		}
	}
}

// take reads a single-use cookie and clears it.
func (c oauthHandshakeCookie) take(w http.ResponseWriter, r *http.Request, secure bool) (*http.Cookie, bool) {
	cookie, ok := c.read(r, secure)
	if ok {
		c.clear(w, r, secure)
	}
	return cookie, ok
}

func newOAuthHandshakeCookie(name, value string, maxAge int, secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		// Lax, not Strict: the provider returns the browser to the callback by
		// a cross-site top-level navigation, which Strict would strip.
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
	}
}
