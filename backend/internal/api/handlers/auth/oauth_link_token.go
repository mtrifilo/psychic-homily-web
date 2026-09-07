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
//
// No origin. An earlier cut stored the browser's reported origin here and
// compared it at spend against the browser's reported origin then: two values
// from the same untrusted source, which proves only that they agree. The
// origin check that means something compares against the CONFIGURED frontend,
// and it lives in requestIsFromOurFrontend, which both legs already run.
type oauthLinkTokenClaims struct {
	userID uint
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

// mintOAuthLinkToken issues a token bound to userID.
//
// What keeps an attacker origin from obtaining one is the caller: the mint
// endpoint refuses a request whose origin is not the configured frontend. That
// matters outside production, where the CORS allowlist admits any
// *.vercel.app with credentials and so would let such an origin read the
// answer.
func mintOAuthLinkToken(secret string, userID uint) (string, error) {
	nonce, err := randomHexID(16)
	if err != nil {
		return "", err
	}
	claims := oauthLinkTokenClaims{
		userID: userID,
		expiry: time.Now().Add(oauthLinkTokenTTL),
		nonce:  nonce,
	}
	payload := encodeOAuthLinkTokenPayload(claims)
	return payload + "." + signOAuthLinkToken(secret, payload), nil
}

func encodeOAuthLinkTokenPayload(claims oauthLinkTokenClaims) string {
	return strings.Join([]string{
		strconv.FormatUint(uint64(claims.userID), 10),
		strconv.FormatInt(claims.expiry.Unix(), 10),
		claims.nonce,
	}, "|")
}

// oauthLinkTokenDomain separates this MAC from every other use of the JWT
// secret, so a value signed for some other purpose can never be presented here
// as a link token.
const oauthLinkTokenDomain = "oauthlink|"

func signOAuthLinkToken(secret, payload string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(oauthLinkTokenDomain))
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// consumeOAuthLinkToken spends a token and reports whether it was valid for
// userID. Where the request came from is requestIsFromOurFrontend's job, and
// the caller runs it first.
//
// Every check is a refusal, and none of them tell the caller which one failed.
func consumeOAuthLinkToken(secret, token string, userID uint) bool {
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
	return spendOAuthLinkNonce(claims.nonce, claims.expiry)
}

func decodeOAuthLinkTokenPayload(payload string) (oauthLinkTokenClaims, error) {
	parts := strings.Split(payload, "|")
	if len(parts) != 3 {
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
	return oauthLinkTokenClaims{
		userID: uint(userID),
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

// requestIsFromOurFrontend reports whether this request came from our own
// frontend, for both legs of the link: the mint (an XHR, which sends Origin)
// and the spend (a top-level GET, which does not, and so relies on Referer).
//
// The CONFIGURED frontend is the primary test, not fetch metadata. Metadata
// answers "same site as this API", and on stage and every preview the frontend
// is on *.vercel.app while the API is on Railway, a different registrable
// domain: genuinely cross-site, so a check that stopped at the metadata would
// refuse every Connect there. Only production happens to be same-site.
//
//	Origin or Referer matches frontendURL  -> accept, whatever metadata says
//	otherwise, metadata says cross-site    -> refuse
//	otherwise, no Origin and no Referer    -> refuse
//
// Referer is present on the spend because next.config.ts:203 sets
// Referrer-Policy: strict-origin-when-cross-origin, which keeps the origin on
// a cross-origin navigation. A stricter policy there, or a browser that strips
// it, drops the spend to the metadata branch: a same-site deployment still
// works, a cross-site one would not. That coupling is why the policy is named
// here.
func requestIsFromOurFrontend(r *http.Request, frontendURL string) bool {
	if origin := requestOrigin(r); origin != "" && sameOrigin(origin, frontendURL) {
		return true
	}

	switch strings.ToLower(r.Header.Get("Sec-Fetch-Site")) {
	case "cross-site", "cross-origin":
		// Another site drove this, and it did not name ours.
		return false
	case "same-origin", "same-site":
		// This API's own origin, or its registrable domain. In a same-site
		// deployment this is the ordinary path when Referer is stripped.
		return true
	case "none":
		// User-initiated: typed, bookmarked, or opened from outside a page.
		return true
	}

	// Nothing says where this came from, so it does not get to start a link.
	return false
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
