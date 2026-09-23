package middleware

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	mathrand "math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/httprate"

	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/respond"
)

// Values of the `event` attribute on rate-limit log lines. Log queries count
// rejections and buckets by grouping on these, so renaming one silently breaks
// every query built on it.
const (
	rateLimitRejectedEvent      = "ratelimit_rejected"
	rateLimitAllowedSampleEvent = "ratelimit_allowed_sample"
)

// Values of the `limiter` attribute: which limiter wrote the line.
const (
	limiterAuth                        = "auth"
	limiterPasskey                     = "passkey"
	limiterTagCreate                   = "tag_create"
	limiterTagVote                     = "tag_vote"
	limiterPublicReadAnonymous         = "public_read_anonymous"
	limiterPublicReadUser              = "public_read_user"
	limiterPublicReadIPCeiling         = "public_read_ip_ceiling"
	limiterEngagementMutationBurst     = "engagement_mutation_burst"
	limiterEngagementMutationSustained = "engagement_mutation_sustained"
	limiterEntityRequestBatchBurst     = "entity_request_batch_burst"
	limiterEntityRequestBatchSustained = "entity_request_batch_sustained"
)

// Values of the `auth_state` attribute on sampled lines.
const (
	authStateAnonymous     = "anonymous"
	authStateAuthenticated = "authenticated"
)

// allowedReadSampleRate is the fraction of allowed requests a sampled limiter
// logs. A bucket that allows n requests during a query's time range appears in
// it with probability 1-(1-rate)^n: 79% at n=15, 96% at n=30.
const allowedReadSampleRate = 0.1

// rateLimitRemainingHeader is the response header httprate writes the bucket's
// remaining budget to on every request it meters. limiterSpec.handler keeps
// httprate's default header names, which is what lets an allowed-request sample
// read the bucket's state back off the response.
const rateLimitRemainingHeader = "X-RateLimit-Remaining"

// limiterSpec is one rate limiter: its budget, the key that picks a bucket, and
// the name its log lines carry.
type limiterSpec struct {
	name   string
	limit  int
	window time.Duration
	key    httprate.KeyFunc
}

// handler is the limiter as middleware. A rejected request gets rejection's
// 429; an allowed one reaches the next handler.
func (s limiterSpec) handler() func(http.Handler) http.Handler {
	return httprate.Limit(
		s.limit,
		s.window,
		httprate.WithKeyFuncs(s.key),
		httprate.WithLimitHandler(s.rejection()),
	)
}

// sampledHandler is handler plus a log line for each allowed request sample
// selects. The sampler sits directly inside the limiter, so the remaining
// budget it reads is this limiter's and not that of a limiter nested further in
// or out.
func (s limiterSpec) sampledHandler(authState string, sample func() bool) func(http.Handler) http.Handler {
	limit := s.handler()
	return func(next http.Handler) http.Handler {
		return limit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if sample() {
				s.logAllowed(w, r, authState)
			}
			next.ServeHTTP(w, r)
		}))
	}
}

// logAttrs are the attributes every rate-limit line from this limiter carries.
// They name the request by path family and the bucket by fingerprint, so a line
// built on them carries no client address, slug, or credential; request_id,
// which the context logger attaches, joins it to the request line when the raw
// path is needed.
func (s limiterSpec) logAttrs(event string, r *http.Request) []any {
	return []any{
		"event", event,
		"limiter", s.name,
		"window_seconds", int(s.window.Seconds()),
		"path_family", RateLimitPathFamily(r.URL.Path),
		"key_fingerprint", requestKeyFingerprint(s.key, r),
	}
}

// logAllowed writes one ratelimit_allowed_sample line. A response without the
// remaining-budget header yields no line rather than one with an invented
// number.
func (s limiterSpec) logAllowed(w http.ResponseWriter, r *http.Request, authState string) {
	remaining, err := strconv.Atoi(w.Header().Get(rateLimitRemainingHeader))
	if err != nil {
		return
	}
	logger.FromContext(r.Context()).Info("rate limit sample", append(s.logAttrs(rateLimitAllowedSampleEvent, r),
		"auth_state", authState,
		"limit", s.limit,
		"remaining", remaining,
	)...)
}

// sampleAt returns a sampler that selects each call independently with
// probability rate. Independent draws keep a page's burst of requests from
// aliasing against a fixed every-Nth stride.
func sampleAt(rate float64) func() bool {
	return func() bool { return mathrand.Float64() < rate }
}

// rejection builds this limiter's 429 handler, which writes one
// ratelimit_rejected line per rejection. Retry-After and the message both name
// the limiter's OWN window: the header is what ApiError.retryAfter carries into
// client countdown copy, so a limiter that reports a minute on an hour bucket
// tells the caller to retry 59 times before the budget can possibly refill.
func (s limiterSpec) rejection() http.HandlerFunc {
	seconds := int(s.window.Seconds())
	retryAfter := strconv.Itoa(seconds)
	body := []byte(fmt.Sprintf(
		`{"success":false,"error":"too_many_requests","message":"Rate limit exceeded. Please try again in %d seconds."}`,
		seconds))
	return func(w http.ResponseWriter, r *http.Request) {
		logger.FromContext(r.Context()).Warn("rate limit exceeded",
			append(s.logAttrs(rateLimitRejectedEvent, r), "method", r.Method)...)

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", retryAfter)
		w.WriteHeader(http.StatusTooManyRequests)
		respond.SafeWrite(r.Context(), w, body)
	}
}

// requestKeyFingerprint fingerprints the bucket key the limiter derived for r.
// httprate runs the key func before it rejects or admits a request, and every
// key func here is a pure function of the request, so this re-derivation lands
// on the same key.
func requestKeyFingerprint(key httprate.KeyFunc, r *http.Request) string {
	k, err := key(r)
	if err != nil {
		return "unavailable"
	}
	return fingerprint(k)
}

// ClientIPKeyFingerprint is the fingerprint of the KeyByClientIP bucket r lands
// in, for 429 handlers outside this package that log which bucket fired.
func ClientIPKeyFingerprint(r *http.Request) string {
	return requestKeyFingerprint(KeyByClientIP, r)
}

// keyFingerprintHexLen is the fingerprint length in hex characters: 48 bits,
// which keeps the chance that two of 100,000 distinct buckets share a
// fingerprint below 1 in 10,000.
const keyFingerprintHexLen = 12

// fingerprintSalt keys every fingerprint this process writes. It is drawn once
// per process and never logged. An unkeyed hash of an IPv4 address is reversed
// by hashing all 2^32 candidates; a keyed one cannot be without the key. The
// cost is that fingerprints compare only between lines from one process.
var fingerprintSalt = newFingerprintSalt()

func newFingerprintSalt() []byte {
	salt := make([]byte, sha256.Size)
	if _, err := rand.Read(salt); err != nil {
		panic("rate-limit fingerprint salt: " + err.Error())
	}
	return salt
}

// fingerprint reduces a bucket key to a short tag that is stable within this
// process and not reversible from logs.
func fingerprint(key string) string {
	return fingerprintWithSalt(fingerprintSalt, key)
}

func fingerprintWithSalt(salt []byte, key string) string {
	mac := hmac.New(sha256.New, salt)
	mac.Write([]byte(key))
	return hex.EncodeToString(mac.Sum(nil)[:keyFingerprintHexLen/2])
}

// Values of the `path_family` attribute. A family names a route shape and never
// the slug or id in the path.
const (
	pathFamilyArtist = "/artists/{slug}"
	pathFamilyShow   = "/shows/{slug}"
	pathFamilyVenue  = "/venues/{slug}"
	pathFamilyScene  = "/scenes/{slug}"
	pathFamilySearch = "/search"
	pathFamilyOther  = "other"
)

// entityCollection is a collection whose entities have their own path family.
type entityCollection struct {
	// family is the family of every path under one entity of the collection,
	// sub-resources included.
	family string
	// routeSegments are the second path segments that name a collection-level
	// route rather than an entity. chi routes a static segment ahead of a
	// parameter, so a path whose second segment is listed here never reaches an
	// entity handler. TestRateLimitPathFamilyAgreesWithRouter walks the built
	// router and fails on a static segment missing from this list.
	routeSegments map[string]bool
}

// entityCollections are keyed by the collection's first path segment.
var entityCollections = map[string]entityCollection{
	"artists": {pathFamilyArtist, map[string]bool{"cities": true, "listing": true, "relationships": true, "search": true}},
	"shows": {pathFamilyShow, map[string]bool{
		"ai-process": true, "calendar": true, "cities": true, "months": true,
		"my-submissions": true, "saves": true, "search": true, "upcoming": true,
	}},
	"venues": {pathFamilyVenue, map[string]bool{"cities": true, "listing": true, "search": true}},
	"scenes": {pathFamilyScene, nil},
}

// RateLimitPathFamily maps a request path to a low-cardinality family for
// rate-limit logs: the entity families in entityCollections, the search family
// for /search and any /{collection}/search, and pathFamilyOther for the rest.
func RateLimitPathFamily(path string) string {
	segments := strings.Split(strings.Trim(path, "/"), "/")
	if len(segments) <= 2 && segments[len(segments)-1] == "search" {
		return pathFamilySearch
	}
	collection, ok := entityCollections[segments[0]]
	if !ok || len(segments) < 2 || segments[1] == "" || collection.routeSegments[segments[1]] {
		return pathFamilyOther
	}
	return collection.family
}
