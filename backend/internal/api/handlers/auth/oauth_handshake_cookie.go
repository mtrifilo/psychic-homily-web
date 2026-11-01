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
// lifetime to one sign-in: a provider round trip, and the sign-up or
// collision step after it.
const oauthHandshakeCookieMaxLifetime = 10 * time.Minute

// oauthHandshakeCookie is a cookie that carries state across an OAuth sign-in:
// from its start to its callback, or, for oauthPendingSignupCookie, from the
// callback to the sign-up step. It is host-only, script-inaccessible, sent on
// a top-level navigation back from the provider, and gone after lifetime.
// Every set and clear goes through newOAuthHandshakeCookie, so each carries
// the same attributes.
type oauthHandshakeCookie struct {
	name     string
	lifetime time.Duration
}

// setName is the name this deployment sets the cookie under, and the one name
// read finds it under. When Secure that is the prefixed name, so a bare copy,
// which a sibling subdomain can plant, is never read.
func (c oauthHandshakeCookie) setName(secure bool) string {
	if secure {
		return oauthHostCookiePrefix + c.name
	}
	return c.name
}

// clearNames are the names clear expires the cookie under: when Secure, the
// bare name as well as the prefixed one, so a bare copy this host set expires
// with the cookie. A bare copy set under a Domain is a different cookie to the
// browser and survives the clear, unread.
func (c oauthHandshakeCookie) clearNames(secure bool) []string {
	if !secure {
		return []string{c.name}
	}
	return []string{c.setName(true), c.name}
}

func (c oauthHandshakeCookie) set(w http.ResponseWriter, value string, secure bool) {
	http.SetCookie(w, newOAuthHandshakeCookie(c.setName(secure), value, int(c.lifetime/time.Second), secure))
}

// read returns the cookie under setName, when the request carries it.
func (c oauthHandshakeCookie) read(r *http.Request, secure bool) (*http.Cookie, bool) {
	cookie, err := r.Cookie(c.setName(secure))
	return cookie, err == nil
}

// clear expires the cookie under every one of clearNames the request carries
// it under.
func (c oauthHandshakeCookie) clear(w http.ResponseWriter, r *http.Request, secure bool) {
	for _, name := range c.clearNames(secure) {
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
