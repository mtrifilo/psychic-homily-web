package auth

import (
	"container/heap"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/logger"
)

// The one-time proof that a link was started from this application's own
// frontend rather than from a page an attacker controls. It proves the origin,
// not which page: any page of the frontend may start a link.
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
// No origin. Storing the browser's reported origin here and comparing it at
// spend against the browser's reported origin then would compare two values
// from the same untrusted source, which proves only that they agree. The
// origin check that means something compares against the CONFIGURED frontend:
// both legs test it with namesFrontend, the mint in StartOAuthLinkHandler and
// the spend in requestIsFromOurFrontend.
type oauthLinkTokenClaims struct {
	userID uint
	expiry time.Time
	nonce  string
}

// oauthLinkSpentNonceCapacity bounds the spent-nonce set. A nonce can never
// leave the set before its token expires, since that would make the token
// spendable again, so a full set refuses the spend instead of evicting: a flood
// of link starts refuses later starts until spent tokens expire, and never
// reopens a spent token to replay.
const oauthLinkSpentNonceCapacity = 1000

// spentOAuthLinkTokens remembers nonces already spent, so a token cannot be
// replayed on the replica that accepted it.
var spentOAuthLinkTokens = newSpentNonceSet(oauthLinkSpentNonceCapacity, time.Now)

// spentNonceSet holds spent nonces, each until the expiry of the token that
// carried it. Tokens are spent in no particular order of expiry, so a min-heap
// orders them for pruning: a spend drops expired nonces from the top only, and
// no operation walks the whole set.
type spentNonceSet struct {
	mu       sync.Mutex
	capacity int
	now      func() time.Time

	// expiries and byExpiry hold the same nonces: expiries answers "spent?",
	// and byExpiry keeps the soonest expiry at its root.
	expiries map[string]time.Time
	byExpiry nonceExpiryHeap

	refusedSinceReport int
	lastReportAt       time.Time
}

// newSpentNonceSet panics on a capacity below 1: a set that can hold nothing
// refuses every spend.
func newSpentNonceSet(capacity int, now func() time.Time) *spentNonceSet {
	if capacity < 1 {
		panic("spentNonceSet capacity must be at least 1")
	}
	return &spentNonceSet{
		capacity: capacity,
		now:      now,
		expiries: make(map[string]time.Time),
	}
}

// spend records nonce as spent until expiry and reports whether this spend
// is the first. It refuses a token already past expiry, a nonce already
// spent, and any spend while the set holds capacity unexpired nonces.
//
// Pruning stays safe because of the first refusal: a nonce leaves the set
// only once the set's clock is past its expiry, and from then on the set
// refuses its token as expired.
//
// refusalsToReport is nonzero when spends were refused for capacity since the
// last report and at least registryEvictionReportInterval has passed since it;
// the count covers every such refusal since that report. The caller logs it.
func (s *spentNonceSet) spend(nonce string, expiry time.Time) (ok bool, refusalsToReport int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	if now.After(expiry) {
		return false, 0
	}
	for len(s.byExpiry) > 0 && now.After(s.byExpiry[0].expiry) {
		delete(s.expiries, heap.Pop(&s.byExpiry).(spentNonce).nonce)
	}
	if _, spent := s.expiries[nonce]; spent {
		return false, 0
	}
	if len(s.expiries) >= s.capacity {
		s.refusedSinceReport++
		if s.lastReportAt.IsZero() || now.Sub(s.lastReportAt) >= registryEvictionReportInterval {
			refusalsToReport = s.refusedSinceReport
			s.refusedSinceReport = 0
			s.lastReportAt = now
		}
		return false, refusalsToReport
	}
	s.expiries[nonce] = expiry
	heap.Push(&s.byExpiry, spentNonce{nonce: nonce, expiry: expiry})
	return true, 0
}

type spentNonce struct {
	nonce  string
	expiry time.Time
}

// nonceExpiryHeap is a container/heap min-heap of spent nonces by expiry.
type nonceExpiryHeap []spentNonce

func (h nonceExpiryHeap) Len() int           { return len(h) }
func (h nonceExpiryHeap) Less(i, j int) bool { return h[i].expiry.Before(h[j].expiry) }
func (h nonceExpiryHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *nonceExpiryHeap) Push(x any)        { *h = append(*h, x.(spentNonce)) }
func (h *nonceExpiryHeap) Pop() any {
	old := *h
	last := old[len(old)-1]
	*h = old[:len(old)-1]
	return last
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
// A spend refused because the spent-nonce set is full is logged here, at most
// once per registryEvictionReportInterval, since the caller cannot tell it apart.
func consumeOAuthLinkToken(ctx context.Context, secret, token string, userID uint) bool {
	// The payload (decimal fields and a hex nonce joined by "|") and the
	// base64url signature hold no dot, so the last dot is the separator.
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
	ok, refusalsToReport := spentOAuthLinkTokens.spend(claims.nonce, claims.expiry)
	if refusalsToReport > 0 {
		logger.AuthWarn(ctx, "oauth_link_spent_nonce_set_full",
			"refused", refusalsToReport,
			"capacity", spentOAuthLinkTokens.capacity,
		)
	}
	return ok
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

// namesFrontend reports whether a request's Origin and Referer header values
// name the frontend at frontendURL: originFromHeaders is non-empty and the
// same origin as one of config.FrontendOrigins(frontendURL), so every
// deployed form of the frontend (apex and www) is ours. The mint refuses a
// request that does not; the spend accepts one that does whatever fetch
// metadata says.
func namesFrontend(origin, referer, frontendURL string) bool {
	from := originFromHeaders(origin, referer)
	if from == "" {
		return false
	}
	return slices.ContainsFunc(config.FrontendOrigins(frontendURL), func(ours string) bool {
		return sameOrigin(from, ours)
	})
}

// originFromHeaders is the origin a browser says a request came from, given
// its Origin and Referer header values: scheme://host, or "" when it says
// nothing.
//
// Origin first: a browser sends it on every CORS request and every POST, so
// the mint (a POST) carries it; a top-level GET navigation, which the spend
// is, carries none. An absent or opaque ("null", in any case) Origin falls
// back to Referer, trimmed to its origin because the path is neither needed
// nor trustworthy to compare. A present Origin is returned as sent, so a
// foreign Origin is never rescued by a Referer naming us.
func originFromHeaders(origin, referer string) string {
	if origin != "" && !strings.EqualFold(origin, "null") {
		return origin
	}
	refererOrigin, _ := config.URLOrigin(referer)
	return refererOrigin
}

// requestIsFromOurFrontend reports whether the spend of a link token came from
// our own frontend. The spend is a top-level GET, which sends no Origin and so
// relies on Referer. The mint (an XHR, which sends Origin) does not run this:
// StartOAuthLinkHandler refuses any origin other than the configured frontend,
// with no fetch-metadata fallback.
//
// The CONFIGURED frontend is the primary test, not fetch metadata. Metadata
// answers "same site as this API", and on stage and every preview the frontend
// is on *.vercel.app while the API is on Railway, a different registrable
// domain: genuinely cross-site, so a check that stopped at the metadata would
// refuse every Connect there. Only production happens to be same-site.
//
//	Origin or Referer names the frontend   -> accept, whatever metadata says
//	otherwise, metadata says cross-site    -> refuse
//	otherwise, no Origin and no Referer    -> refuse
//
// Referer is present on the spend because the site-wide headers in
// frontend/next.config.ts set Referrer-Policy: strict-origin-when-cross-origin,
// which keeps the origin on a cross-origin navigation. A stricter policy
// there, or a browser that strips it, drops the spend to the metadata branch:
// a same-site deployment still works, a cross-site one would not. That
// coupling is why the policy is named here.
func requestIsFromOurFrontend(r *http.Request, frontendURL string) bool {
	if namesFrontend(r.Header.Get("Origin"), r.Header.Get("Referer"), frontendURL) {
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
