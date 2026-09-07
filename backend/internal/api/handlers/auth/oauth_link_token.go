package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The one-time proof that a link was started from this application's own
// Settings page rather than from a page an attacker controls.
//
// /auth/link/{provider} is a cookie-authenticated GET, and the auth cookie's
// SameSite is SESSION_SAME_SITE (default lax), which browsers DO send on a
// cross-site top-level navigation. Without this token any page could navigate
// a signed-in reader into the link flow, and a reader with a live provider
// session completes it with no interaction at all.
//
// The token is a signed assertion rather than a row in a map. A map is
// per-process, and the token spans two requests, so a rolling deploy or a
// second replica would refuse tokens it never minted. Everything the spend
// needs is inside the value and covered by the signature.
//
// Single use is enforced by remembering spent nonces, and THAT set is
// process-local, so single use is best-effort across replicas while validity
// is not. The ordering matters: a replay that lands on another replica is
// still bounded by the short expiry, and the alternative, refusing every token
// after a deploy, breaks the ordinary case to harden the rare one.
const (
	oauthLinkTokenTTL = 5 * time.Minute
	// oauthLinkTokenParam carries the token on the start URL.
	oauthLinkTokenParam = "t"
)

// oauthLinkTokenClaims is what the signature covers.
type oauthLinkTokenClaims struct {
	userID uint
	origin string
	expiry time.Time
	nonce  string
}

// spentOAuthLinkTokens remembers nonces already spent, so a token cannot be
// replayed on the replica that accepted it. Pruned on write; entries are
// worthless once past their expiry.
var spentOAuthLinkTokens = struct {
	sync.Mutex
	nonces map[string]time.Time
}{
	nonces: make(map[string]time.Time),
}

// mintOAuthLinkToken issues a token bound to userID AND to the origin that
// asked for it.
//
// The origin binding is what survives a CORS allowlist wider than production's.
// Outside production the allowlist admits any *.vercel.app with credentials, so
// an attacker origin can call the mint endpoint and read the answer. Binding
// the token to the origin that minted it means the token it gets back is only
// spendable from that origin, and the spend requires our own frontend.
func mintOAuthLinkToken(secret string, userID uint, origin string) (string, error) {
	nonce, err := randomHexID(16)
	if err != nil {
		return "", err
	}
	claims := oauthLinkTokenClaims{
		userID: userID,
		origin: origin,
		expiry: time.Now().Add(oauthLinkTokenTTL),
		nonce:  nonce,
	}
	payload := encodeOAuthLinkTokenPayload(claims)
	return payload + "." + signOAuthLinkToken(secret, payload), nil
}

func encodeOAuthLinkTokenPayload(claims oauthLinkTokenClaims) string {
	// The origin is escaped so a value containing the separator cannot shift
	// the field boundaries and present as a different set of claims.
	return strings.Join([]string{
		strconv.FormatUint(uint64(claims.userID), 10),
		strconv.FormatInt(claims.expiry.Unix(), 10),
		claims.nonce,
		url.QueryEscape(claims.origin),
	}, "|")
}

func signOAuthLinkToken(secret, payload string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// consumeOAuthLinkToken spends a token and reports whether it was valid for
// userID and for the origin this request actually came from.
//
// Every check is a refusal, and none of them tell the caller which one failed.
func consumeOAuthLinkToken(secret, token string, userID uint, requestOrigin string) bool {
	// Split at the LAST dot, not the first: the payload carries a
	// percent-escaped origin, and "." is unreserved so it survives escaping.
	// base64url has no dots, so the final one is always the separator.
	dot := strings.LastIndex(token, ".")
	if dot <= 0 || dot == len(token)-1 {
		return false
	}
	payload, signature := token[:dot], token[dot+1:]

	// Constant time: the signature is the only thing standing between a
	// guessed payload and a spendable token.
	if subtle.ConstantTimeCompare([]byte(signature), []byte(signOAuthLinkToken(secret, payload))) != 1 {
		return false
	}

	claims, err := decodeOAuthLinkTokenPayload(payload)
	if err != nil {
		return false
	}
	if claims.userID != userID || time.Now().After(claims.expiry) {
		return false
	}
	if claims.origin != requestOrigin {
		return false
	}
	return spendOAuthLinkNonce(claims.nonce, claims.expiry)
}

func decodeOAuthLinkTokenPayload(payload string) (oauthLinkTokenClaims, error) {
	parts := strings.Split(payload, "|")
	if len(parts) != 4 {
		return oauthLinkTokenClaims{}, fmt.Errorf("malformed link token payload")
	}
	userID, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil || userID == 0 {
		return oauthLinkTokenClaims{}, fmt.Errorf("malformed link token subject")
	}
	expiry, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return oauthLinkTokenClaims{}, fmt.Errorf("malformed link token expiry")
	}
	origin, err := url.QueryUnescape(parts[3])
	if err != nil {
		return oauthLinkTokenClaims{}, fmt.Errorf("malformed link token origin")
	}
	return oauthLinkTokenClaims{
		userID: uint(userID),
		origin: origin,
		expiry: time.Unix(expiry, 0),
		nonce:  parts[2],
	}, nil
}

// spendOAuthLinkNonce records a nonce and reports whether it was unspent.
func spendOAuthLinkNonce(nonce string, expiry time.Time) bool {
	spentOAuthLinkTokens.Lock()
	defer spentOAuthLinkTokens.Unlock()

	now := time.Now()
	for k, v := range spentOAuthLinkTokens.nonces {
		if now.After(v) {
			delete(spentOAuthLinkTokens.nonces, k)
		}
	}
	if _, spent := spentOAuthLinkTokens.nonces[nonce]; spent {
		return false
	}
	spentOAuthLinkTokens.nonces[nonce] = expiry
	return true
}

// requestOrigin is the origin a browser says this request came from, as
// scheme://host, or "" when it says nothing.
//
// Origin first: it is set by the browser on every cross-origin request and on
// same-origin navigations that matter here. Referer is the fallback, trimmed to
// its origin because the path is neither needed nor trustworthy to compare.
func requestOrigin(r *http.Request) string {
	if origin := r.Header.Get("Origin"); origin != "" && origin != "null" {
		return origin
	}
	return originOfURL(r.Header.Get("Referer"))
}

// originOfURL trims a URL to scheme://host, or "" when it is not one.
func originOfURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}

// requestIsFromOurFrontend reports whether this request may start a link.
//
// Fail CLOSED, which is the difference from a check that only refuses a
// declared "cross-site": fetch metadata is absent on older browsers and on
// some navigation shapes, and treating absence as permission leaves the whole
// defence to a header the attacker's victim may simply not send.
//
//   - Sec-Fetch-Site present: it must say same-origin or same-site. Page
//     script cannot set it, so a present value is trustworthy.
//   - absent: fall back to Origin, then Referer, which must match the
//     configured frontend's origin.
//   - neither: refused.
func requestIsFromOurFrontend(r *http.Request, frontendURL string) bool {
	switch strings.ToLower(r.Header.Get("Sec-Fetch-Site")) {
	case "same-origin", "same-site", "none":
		// "none" is a user-initiated navigation: typed, bookmarked, or opened
		// from outside a page. No other site drove it.
		return true
	case "cross-site", "cross-origin":
		return false
	}

	origin := requestOrigin(r)
	if origin == "" {
		return false
	}
	return sameOrigin(origin, frontendURL)
}

// sameOrigin compares two URLs by scheme and host.
func sameOrigin(a, b string) bool {
	parsedA, errA := url.Parse(a)
	parsedB, errB := url.Parse(b)
	if errA != nil || errB != nil {
		return false
	}
	if parsedA.Host == "" || parsedB.Host == "" {
		return false
	}
	return strings.EqualFold(parsedA.Scheme, parsedB.Scheme) &&
		strings.EqualFold(parsedA.Host, parsedB.Host)
}
