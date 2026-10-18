package auth

import "net/http"

// oauthHostCookiePrefix binds a cookie to the host that set it: a browser
// stores a "__Host-" cookie only when it is Secure, has Path "/" and names no
// Domain, so a sibling subdomain cannot plant or shadow one. Browsers refuse
// the prefix on an insecure origin, so it is applied only when the cookie is
// Secure, and local development over plain http keeps the bare name.
const oauthHostCookiePrefix = "__Host-"

// oauthHandshakeCookieNames are the names a request may carry the handshake
// cookie name under, in the order they are read: the name this deployment
// sets first.
//
// The bare name after the prefixed one is the rollover read: a browser that
// started a handshake before the prefix was deployed still carries the bare
// name for up to one cookie lifetime (600 s at most). Remove that entry on or
// after 2026-10-24, once a production deploy that sets the prefixed names has
// been live for longer than that lifetime.
func oauthHandshakeCookieNames(name string, secure bool) []string {
	if !secure {
		return []string{name}
	}
	return []string{oauthHostCookiePrefix + name, name}
}

// setOAuthHandshakeCookie writes a cookie that carries state from an OAuth
// start to its callback, under the name this deployment sets.
func setOAuthHandshakeCookie(w http.ResponseWriter, name, value string, maxAge int, secure bool) {
	http.SetCookie(w, newOAuthHandshakeCookie(oauthHandshakeCookieNames(name, secure)[0], value, maxAge, secure))
}

// readOAuthHandshakeCookie returns the handshake cookie name the request
// carries, under the first of oauthHandshakeCookieNames it is found under.
func readOAuthHandshakeCookie(r *http.Request, name string, secure bool) (*http.Cookie, bool) {
	for _, candidate := range oauthHandshakeCookieNames(name, secure) {
		if cookie, err := r.Cookie(candidate); err == nil {
			return cookie, true
		}
	}
	return nil, false
}

// clearOAuthHandshakeCookie expires the handshake cookie name under every name
// the request carries it under, so a value under a name the reader would fall
// back to cannot outlive the one that was read.
func clearOAuthHandshakeCookie(w http.ResponseWriter, r *http.Request, name string, secure bool) {
	for _, candidate := range oauthHandshakeCookieNames(name, secure) {
		if _, err := r.Cookie(candidate); err == nil {
			http.SetCookie(w, newOAuthHandshakeCookie(candidate, "", -1, secure))
		}
	}
}

// newOAuthHandshakeCookie builds a handshake cookie under exactly setName:
// host-only, script-inaccessible, and sent on the provider's redirect back.
// Every set and every clear goes through it, so a clear matches the cookie it
// expires in name, path and attributes.
func newOAuthHandshakeCookie(setName, value string, maxAge int, secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     setName,
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
