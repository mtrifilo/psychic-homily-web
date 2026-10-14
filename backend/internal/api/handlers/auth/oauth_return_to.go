package auth

import (
	"encoding/base64"
	"net/http"
	"net/url"
	"strings"
)

// oauthReturnToParam is the query parameter the sign-in start reads the
// destination from. The frontend auth page names the same parameter
// (frontend/app/auth/page.tsx reads it through sanitizeReturnTo).
const oauthReturnToParam = "returnTo"

// oauthReturnToCookieName carries the destination from the sign-in start to
// its callback. The callback URL belongs to the provider's redirect, so the
// destination cannot travel on it; a cookie set on the start's own response is
// the one channel the provider does not control.
const oauthReturnToCookieName = "oauth_return_to"

// oauthReturnToCookieMaxAge matches the signup consent cookie: long enough
// for a consent screen and a second factor at the provider.
const oauthReturnToCookieMaxAge = 600

// oauthReturnToFallback is where a sign-in lands when it names no destination
// this file accepts. The frontend's FALLBACK_RETURN_TO (frontend/lib/auth-href.ts)
// is the same path.
const oauthReturnToFallback = "/"

// oauthReturnToMaxLength bounds the destination so the encoded cookie stays
// well inside the 4096-byte limit browsers apply to a single cookie, past
// which the cookie is silently dropped.
const oauthReturnToMaxLength = 2048

// oauthAuthPagePath is the frontend sign-in page. A destination on it, or
// under it, would return a reader who just signed in to the sign-in surface.
const oauthAuthPagePath = "/auth"

// sanitizeOAuthReturnTo returns raw when it is a path on the frontend's own
// origin that a signed-in reader may be sent to, and oauthReturnToFallback
// for anything else. It applies the rules of the frontend's sanitizeReturnTo
// (frontend/app/auth/auth-redirect-utils.ts) to a value that has not been
// through a browser URL parser:
//
//   - one leading slash: "//host" is protocol-relative
//   - printable ASCII only, so no whitespace or control byte anywhere: a
//     browser URL parser strips tabs and newlines, and the frontend's own
//     parser percent-encodes spaces and non-ASCII before sending
//   - no backslash in the path: a browser URL parser reads it as "/", so it
//     can turn a value that passes the slash test into "//host"
//   - no "." or ".." path segment, percent-encoded or not: a browser resolves
//     them, so the path checked here would not be the path visited
//   - not the sign-in page or anything under it, judged on the decoded path
//
// The last rule is stricter than the frontend's, which compares the encoded
// path: a percent-encoded spelling of the sign-in path that the frontend
// passes through falls back here.
//
// The callback prefixes the result with the configured frontend URL, so the
// redirect's host is never taken from raw.
func sanitizeOAuthReturnTo(raw string) string {
	if raw == "" || len(raw) > oauthReturnToMaxLength {
		return oauthReturnToFallback
	}
	if !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") {
		return oauthReturnToFallback
	}
	for i := 0; i < len(raw); i++ {
		if c := raw[i]; c <= ' ' || c > '~' {
			return oauthReturnToFallback
		}
	}

	rawPath := raw
	if end := strings.IndexAny(raw, "?#"); end >= 0 {
		rawPath = raw[:end]
	}
	if strings.Contains(rawPath, `\`) {
		return oauthReturnToFallback
	}
	for _, segment := range strings.Split(rawPath, "/") {
		decoded, err := url.PathUnescape(segment)
		if err != nil || decoded == "." || decoded == ".." {
			return oauthReturnToFallback
		}
	}

	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "" || parsed.Host != "" || parsed.User != nil {
		return oauthReturnToFallback
	}
	if parsed.Path == oauthAuthPagePath || strings.HasPrefix(parsed.Path, oauthAuthPagePath+"/") {
		return oauthReturnToFallback
	}
	return raw
}

// setOAuthReturnToCookie records the sign-in start's destination for its
// callback, or clears one an earlier, abandoned start left behind so that it
// cannot redirect a sign-in that named no destination.
func setOAuthReturnToCookie(w http.ResponseWriter, r *http.Request, secure bool) {
	returnTo := sanitizeOAuthReturnTo(r.URL.Query().Get(oauthReturnToParam))
	if returnTo == oauthReturnToFallback {
		if _, err := r.Cookie(oauthReturnToCookieName); err == nil {
			http.SetCookie(w, newOAuthReturnToCookie("", -1, secure))
		}
		return
	}
	// Encoded because http.SetCookie drops ";", `"` and `\` from a cookie
	// value and quotes a value holding "," or a space, and a destination's
	// query string can carry any of them.
	encoded := base64.RawURLEncoding.EncodeToString([]byte(returnTo))
	http.SetCookie(w, newOAuthReturnToCookie(encoded, oauthReturnToCookieMaxAge, secure))
}

// takeOAuthReturnTo reads the destination the sign-in start recorded, clears
// the cookie, and returns the destination re-checked by sanitizeOAuthReturnTo,
// or oauthReturnToFallback when there is none. Single use: every callback
// consumes it, whatever the callback then does.
func takeOAuthReturnTo(w http.ResponseWriter, r *http.Request, secure bool) string {
	cookie, err := r.Cookie(oauthReturnToCookieName)
	if err != nil {
		return oauthReturnToFallback
	}
	http.SetCookie(w, newOAuthReturnToCookie("", -1, secure))

	decoded, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil {
		return oauthReturnToFallback
	}
	return sanitizeOAuthReturnTo(string(decoded))
}

// oauthSignInDestination is the frontend URL a successful browser sign-in
// redirects to. The fallback is the bare frontend URL.
func oauthSignInDestination(frontendURL, returnTo string) string {
	if returnTo == oauthReturnToFallback {
		return frontendURL
	}
	return frontendURL + returnTo
}

func newOAuthReturnToCookie(value string, maxAge int, secure bool) *http.Cookie {
	return newOAuthHandshakeCookie(oauthReturnToCookieName, value, maxAge, secure)
}

// newOAuthHandshakeCookie builds a cookie that carries state from an OAuth
// start to its callback: host-only, script-inaccessible, and sent on the
// provider's redirect back.
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
