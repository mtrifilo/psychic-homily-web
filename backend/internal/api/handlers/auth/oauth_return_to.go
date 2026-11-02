package auth

import (
	"encoding/base64"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	autherrors "psychic-homily-backend/internal/errors"
)

// oauthReturnToParam names the destination on both sides of the sign-in
// round trip: the sign-in start reads it (GoogleOAuthButton in
// frontend/app/auth/_components/google-oauth-button.tsx writes it), and the
// redirects to the frontend auth page write it (frontend/app/auth/page.tsx
// reads it through sanitizeReturnTo).
const oauthReturnToParam = "returnTo"

// oauthReturnToCookieName carries the oauthReturnTo from the sign-in start to
// its callback. The callback URL belongs to the provider's redirect, so the
// destination cannot travel on it; a cookie set on the start's own response is
// the one channel the provider does not control.
const oauthReturnToCookieName = "oauth_return_to"

// oauthReturnToCookieLifetime bounds the destination cookie to one OAuth round
// trip, a consent screen and a second factor at the provider included.
const oauthReturnToCookieLifetime = 10 * time.Minute

var oauthReturnToCookie = oauthHandshakeCookie{
	name:     oauthReturnToCookieName,
	lifetime: oauthReturnToCookieLifetime,
}

// oauthReturnToFallback is where a sign-in lands when it names no destination
// this file accepts. The frontend's FALLBACK_RETURN_TO (frontend/lib/auth-href.ts)
// is the same path.
const oauthReturnToFallback = "/"

// oauthReturnToMaxLength bounds the destination so the encoded cookie stays
// well inside the 4096-byte limit browsers apply to a single cookie, past
// which the cookie is silently dropped.
const oauthReturnToMaxLength = 2048

// oauthAuthPagePath is the frontend sign-in page: where a refused sign-in is
// sent, and a destination this file refuses, since a destination on it or
// under it would return a reader who just signed in to the sign-in surface.
const oauthAuthPagePath = "/auth"

// oauthReauthReasons are the auth page's re-authentication reasons
// (REAUTH_REASONS in frontend/lib/auth-href.ts): an arrival carrying one asks a
// signed-in reader to prove the account again, and the page holds its
// signed-in redirect for it. They are the only reasons a sign-in start
// records.
var oauthReauthReasons = []string{oauthLinkErrorReauthRequired, autherrors.CodeReauthRequired}

// sanitizeOAuthReauthReason returns raw when it is one of oauthReauthReasons,
// and "" for anything else.
func sanitizeOAuthReauthReason(raw string) string {
	if slices.Contains(oauthReauthReasons, raw) {
		return raw
	}
	return ""
}

// oauthReturnTo is what a browser sign-in returns to: the destination, and the
// re-authentication reason of the auth page it started from. A refusal
// carries both back to the auth page, so a signed-in reader whose
// re-authentication was refused sees the refusal there instead of being
// redirected to the destination with nothing said. A sign-in that succeeds
// lands on the destination alone.
type oauthReturnTo struct {
	// path is a sanitizeOAuthReturnTo result.
	path string
	// reauthReason is a sanitizeOAuthReauthReason result.
	reauthReason string
}

// noOAuthReturnTo is the oauthReturnTo of a sign-in that named neither a
// destination nor a re-authentication.
var noOAuthReturnTo = oauthReturnTo{path: oauthReturnToFallback}

// oauthReturnToFromQuery is the oauthReturnTo a sign-in start's query names:
// oauthReturnToParam and authPageReasonParam, each sanitized.
func oauthReturnToFromQuery(query url.Values) oauthReturnTo {
	return oauthReturnTo{
		path:         sanitizeOAuthReturnTo(query.Get(oauthReturnToParam)),
		reauthReason: sanitizeOAuthReauthReason(query.Get(authPageReasonParam)),
	}
}

// oauthReturnToCookieSeparator divides the cookie's encoded destination from
// its reason. It is outside the base64url alphabet the destination is encoded
// in and absent from every oauthReauthReasons value, so the first one in a
// value is the division.
const oauthReturnToCookieSeparator = "."

// encodeOAuthReturnTo is the cookie value for returnTo: the destination
// base64url-encoded, because http.SetCookie drops ";", `"` and `\` from a
// cookie value and quotes a value holding "," or a space, and a destination's
// query string can carry any of them; then, when there is a reason, the
// separator and the reason.
func encodeOAuthReturnTo(returnTo oauthReturnTo) string {
	encoded := base64.RawURLEncoding.EncodeToString([]byte(returnTo.path))
	if returnTo.reauthReason == "" {
		return encoded
	}
	return encoded + oauthReturnToCookieSeparator + returnTo.reauthReason
}

// decodeOAuthReturnTo reads a cookie value encodeOAuthReturnTo wrote, checking
// both parts again, since the browser can send any value. A destination that
// does not decode makes the whole value noOAuthReturnTo.
func decodeOAuthReturnTo(value string) oauthReturnTo {
	encodedPath, reason, _ := strings.Cut(value, oauthReturnToCookieSeparator)
	decoded, err := base64.RawURLEncoding.DecodeString(encodedPath)
	if err != nil {
		return noOAuthReturnTo
	}
	return oauthReturnTo{
		path:         sanitizeOAuthReturnTo(string(decoded)),
		reauthReason: sanitizeOAuthReauthReason(reason),
	}
}

// sanitizeOAuthReturnTo returns raw when it is a path on the frontend's own
// origin that a signed-in reader may be sent to, less any OAuth link result
// in its query string, and oauthReturnToFallback for anything else. It checks
// a value that has not been through a browser URL parser:
//
//   - at most oauthReturnToMaxLength bytes
//   - one leading slash: "//host" is protocol-relative
//   - printable ASCII only, so no whitespace or control byte anywhere: a
//     browser URL parser strips tabs and newlines, and the frontend's own
//     parser percent-encodes spaces and non-ASCII before sending
//   - no backslash in the path: a browser URL parser reads it as "/", so it
//     can turn a value that passes the slash test into "//host"
//   - no "." or ".." path segment, percent-encoded or not: a browser resolves
//     them, so the path checked here would not be the path visited
//   - no malformed percent escape in the path or the fragment ("/shows/%zz",
//     "/shows#50%"), which Go's URL parser refuses
//   - not the sign-in page or anything under it, judged on the decoded path,
//     so "/%61uth" is the sign-in page too
//
// A destination that passes them all loses its oauthLinkResultParams pairs:
// Settings reads them once, on the redirect that delivers them, and a
// destination still carrying one would replay that result after this sign-in.
// Every other parameter, and the fragment, keep the spelling and order they
// arrived with (see withoutOAuthLinkResult).
//
// These rules are the source of truth for the frontend's sanitizeReturnTo
// (frontend/app/auth/auth-redirect-utils.ts), which applies them in the same
// order, so the auth page never keeps a destination this refuses.
// testdata/return_to_corpus.json is the statement of that parity: both
// sanitizers' tests read it and assert one expected output per input.
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
	return withoutOAuthLinkResult(raw)
}

// oauthLinkResultParams are the query keys the OAuth link redirect delivers
// its result under, which no destination keeps.
var oauthLinkResultParams = []string{oauthLinkResultParam, oauthLinkErrorParam}

// withoutOAuthLinkResult is returnTo without the oauthLinkResultParams pairs
// of its query string, or returnTo itself, byte for byte, when its query
// carries none. A name is compared query-unescaped, the way Settings reads
// it, so an encoded spelling of a link parameter is dropped too. When a pair
// is dropped, empty pairs go with it, and a query left empty loses its "?".
// The frontend's sanitizeReturnTo applies the same rewrite, so both
// sanitizers return the same string.
func withoutOAuthLinkResult(returnTo string) string {
	queryStart := strings.IndexByte(returnTo, '?')
	fragmentStart := strings.IndexByte(returnTo, '#')
	if queryStart < 0 || (fragmentStart >= 0 && fragmentStart < queryStart) {
		return returnTo
	}
	queryEnd := len(returnTo)
	if fragmentStart >= 0 {
		queryEnd = fragmentStart
	}

	pairs := strings.Split(returnTo[queryStart+1:queryEnd], "&")
	kept := make([]string, 0, len(pairs))
	dropped := false
	for _, pair := range pairs {
		if isOAuthLinkResultPair(pair) {
			dropped = true
		} else if pair != "" {
			kept = append(kept, pair)
		}
	}
	if !dropped {
		return returnTo
	}

	query := ""
	if len(kept) > 0 {
		query = "?" + strings.Join(kept, "&")
	}
	return returnTo[:queryStart] + query + returnTo[queryEnd:]
}

// isOAuthLinkResultPair reports whether one name=value pair of a query
// string names an oauthLinkResultParams key. A name that does not unescape
// keeps a literal "%" in the browser's reading too, so it names neither key.
func isOAuthLinkResultPair(pair string) bool {
	name, _, _ := strings.Cut(pair, "=")
	decoded, err := url.QueryUnescape(name)
	return err == nil && slices.Contains(oauthLinkResultParams, decoded)
}

// setOAuthReturnToCookie records the sign-in start's oauthReturnTo for its
// callback, or, when the start names neither a destination nor a
// re-authentication, clears one an earlier, abandoned start left behind so
// that it cannot redirect this sign-in.
func setOAuthReturnToCookie(w http.ResponseWriter, r *http.Request, secure bool) {
	returnTo := oauthReturnToFromQuery(r.URL.Query())
	if returnTo == noOAuthReturnTo {
		clearOAuthReturnToCookie(w, r, secure)
		return
	}
	oauthReturnToCookie.set(w, encodeOAuthReturnTo(returnTo), secure)
}

// clearOAuthReturnToCookie removes an oauthReturnTo an earlier, abandoned
// start left behind. Every start that hands the browser to the provider
// without one of its own calls it, so a callback that resolves as a sign-in
// never lands on another start's destination or reason.
func clearOAuthReturnToCookie(w http.ResponseWriter, r *http.Request, secure bool) {
	oauthReturnToCookie.clear(w, r, secure)
}

// takeOAuthReturnTo reads the oauthReturnTo the sign-in start recorded, clears
// the cookie, and returns it re-checked, or noOAuthReturnTo when there is
// none. Single use: every callback consumes it, whatever the callback then
// does.
func takeOAuthReturnTo(w http.ResponseWriter, r *http.Request, secure bool) oauthReturnTo {
	cookie, ok := oauthReturnToCookie.take(w, r, secure)
	if !ok {
		return noOAuthReturnTo
	}
	return decodeOAuthReturnTo(cookie.Value)
}

// oauthSignInDestination is the frontend URL a successful browser sign-in
// redirects to: the destination, without the reason, since a sign-in that
// succeeded has satisfied it. The fallback is the bare frontend URL.
func oauthSignInDestination(frontendURL string, returnTo oauthReturnTo) string {
	if returnTo.path == oauthReturnToFallback {
		return frontendURL
	}
	return frontendURL + returnTo.path
}

// authPageRefusalURL is where a refused browser sign-in sends the reader: the
// auth page with the code of a refusal authRefusalCarriesItsOwnCopy names, and
// CodeUnknown for everything else, so a backend fault is never described to
// the page in its own terms.
func authPageRefusalURL(frontendURL string, err error, returnTo oauthReturnTo) string {
	return authPageCodeURL(frontendURL, refusalCode(err, autherrors.CodeUnknown), returnTo)
}

// authPageCodeURL is the frontend auth page with code as its only message,
// plus the re-authentication reason and the destination when there are
// ones, so that the page reopens as the re-authentication it was and a retry
// from it still lands on the destination. The page maps the code to fixed
// copy.
func authPageCodeURL(frontendURL, code string, returnTo oauthReturnTo) string {
	query := url.Values{authPageErrorCodeParam: {code}}
	if returnTo.reauthReason != "" {
		query.Set(authPageReasonParam, returnTo.reauthReason)
	}
	return authPageURLWithReturnTo(frontendURL, query, returnTo.path)
}

// authPageURLWithReturnTo is the frontend auth page with query, plus the
// destination when there is one.
func authPageURLWithReturnTo(frontendURL string, query url.Values, returnTo string) string {
	if returnTo != oauthReturnToFallback {
		query.Set(oauthReturnToParam, returnTo)
	}
	return frontendURL + oauthAuthPagePath + "?" + query.Encode()
}
