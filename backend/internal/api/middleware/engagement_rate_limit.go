package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"psychic-homily-backend/internal/services/auth"
)

// Engagement-mutation rate-limit ceilings (PSY-1482; policy locked in PSY-1460).
// ONE shared per-user budget covers show/release save+unsave, entity/scene
// follow+unfollow, venue confirm, and single entity-request file+withdraw: the
// same inline UX burst on Broadsheet chart rows, so a separate budget per action
// would just invite "rotate which button to spam." A request must clear BOTH
// windows (stricter wins).
const (
	// EngagementMutationBurstPerMinute caps short bursts at ~1/sec — enough to
	// walk a Top-100 drill-down with inline saves/follows, far below scraper
	// churn.
	EngagementMutationBurstPerMinute = 60

	// EngagementMutationSustainedPerHour caps flip-flop save/unsave loops and
	// overnight scripted churn without biting a genuine power session
	// (~10/min average).
	EngagementMutationSustainedPerHour = 600
)

// Entity-request BATCH ceilings (PSY-1991). Its own per-user budget, separate
// from the shared one above, because one batch request carries up to
// community.maxEntityRequestSubmissions submissions while every mutation on the
// shared budget carries exactly one. Metering is per REQUEST, so these numbers
// bound requests and not filed rows; the row bound is the endpoint's own
// per-request item cap plus its per-item dedup.
//
// Both numbers are read off the paste flow that drives the endpoint. The picker
// splits a paste into chunks of its QUEUE_BATCH_MAX_ITEMS (200, the endpoint's
// cap), so a 600-line paste is 3 back-to-back requests and a full retry of it is
// 3 more; AICollectionFiller files one row per click from the same session, at
// human click rate. Halving the shared burst LIMIT leaves 5x headroom over that
// 6-request paste while keeping the 1:10 burst-to-sustained ratio the shared
// budget uses.
const (
	// EntityRequestBatchBurstPerMinute caps back-to-back batch requests from one
	// user. A paste larger than 30 chunks (6000 zero-result lines) 429s its tail,
	// which the picker reports on the rows it did not file.
	EntityRequestBatchBurstPerMinute = 30

	// EntityRequestBatchSustainedPerHour caps scripted paste churn without biting
	// a contributor working through a long AI-extracted list one row at a time.
	EntityRequestBatchSustainedPerHour = 300
)

// mutationUserIDKey is the context key under which
// meterSessionUser stashes the authenticated user id so the
// per-user limiters' key func can read it without re-parsing the token. It is
// deliberately distinct from the public-read limiter's key so the two never
// share a bucket even though both key "user:<id>".
type mutationUserIDKey struct{}

// mutationUserKeyFunc keys every per-user mutation limiter by the user id
// meterSessionUser stashes in context. If the value is absent
// it FAILS LOUD (httprate turns a key-func error into a 428) rather than
// silently keying every request into one shared bucket — so mounting a bare
// per-user limiter without the wrapper that sets the id is a detectable
// misuse, not a single site-wide budget.
func mutationUserKeyFunc(r *http.Request) (string, error) {
	uid, ok := r.Context().Value(mutationUserIDKey{}).(uint)
	if !ok {
		return "", fmt.Errorf(
			"mutationUserKeyFunc: no authenticated user id in context; " +
				"per-user mutation limiters must be mounted via RateLimitMutationsByUser or RateLimitBySessionUser")
	}
	return "user:" + strconv.FormatUint(uint64(uid), 10), nil
}

// perUserMutationLimiter builds one per-USER window: limit requests per window,
// keyed by the user id meterSessionUser stashes in context (NOT by IP,
// so shared-IP logged-in users each get their own bucket). Every call mints its
// OWN httprate store, so two budgets built here never draw on one counter.
//
// The 429 names this window's own length in Retry-After, so an hour bucket does
// not tell a caller to come back in a minute. That value reaches the picker's
// countdown copy through ApiError.retryAfter.
func perUserMutationLimiter(name LimiterName, limit int, window time.Duration) func(http.Handler) http.Handler {
	return limiterSpec{name: name, limit: limit, window: window, key: mutationUserKeyFunc}.handler()
}

// RateLimitEngagementMutationBurst is the shared budget's minute window. Pair
// with RateLimitMutationsByUser, which supplies the user id via context.
func RateLimitEngagementMutationBurst() func(http.Handler) http.Handler {
	return perUserMutationLimiter(LimiterEngagementMutationBurst, EngagementMutationBurstPerMinute, time.Minute)
}

// RateLimitEngagementMutationSustained is the shared budget's hour window.
// Chained INSIDE the burst limiter (see the ORDER note on
// RateLimitMutationsByUser).
func RateLimitEngagementMutationSustained() func(http.Handler) http.Handler {
	return perUserMutationLimiter(LimiterEngagementMutationSustained, EngagementMutationSustainedPerHour, time.Hour)
}

// RateLimitEntityRequestBatchBurst is the entity-request batch budget's minute
// window.
func RateLimitEntityRequestBatchBurst() func(http.Handler) http.Handler {
	return perUserMutationLimiter(LimiterEntityRequestBatchBurst, EntityRequestBatchBurstPerMinute, time.Minute)
}

// RateLimitEntityRequestBatchSustained is the entity-request batch budget's hour
// window. Chained INSIDE the burst limiter (see the ORDER note on
// RateLimitMutationsByUser).
func RateLimitEntityRequestBatchSustained() func(http.Handler) http.Handler {
	return perUserMutationLimiter(LimiterEntityRequestBatchSustained, EntityRequestBatchSustainedPerHour, time.Hour)
}

// RateLimitBySessionUser meters each signed-in request against two minute
// budgets: userLimiter, keyed by the session user the way
// RateLimitMutationsByUser keys the mutation budgets, and ipCeilingLimiter, keyed
// by the client address. Accounts behind one address do not share a user bucket,
// and one account does not get a fresh one by changing address; the ceiling
// bounds what every account behind one address spends together. A request with
// no session token passes through unmetered, as it does on the mutation budgets.
//
// ORDER: user OUTER, ceiling INNER, the order RateLimitPublicReadsByAuthState
// uses and for the same reason. httprate counts a request against a limiter once
// it clears that limiter's own limit, whether or not a limiter further in refuses
// it. With the ceiling inside, an account retrying past its own budget is refused
// by the user limiter and never reaches the shared ceiling, so it cannot spend
// the address's ceiling and refuse every other account behind that address. The
// cost of this order is the other direction: a request the ceiling refuses has
// already spent one from its account's own budget.
func RateLimitBySessionUser(jwtService *auth.JWTService, userLimiter, ipCeilingLimiter func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return meterSessionUser(jwtService, userLimiter(ipCeilingLimiter(next)), next)
	}
}

// RateLimitPasskeyRegisterBeginUser is the per-user register begin budget:
// PasskeyRegisterBeginRequestsPerMinute per session user. Pair with
// RateLimitBySessionUser, which supplies the user id via context.
func RateLimitPasskeyRegisterBeginUser() func(http.Handler) http.Handler {
	return perUserMutationLimiter(LimiterPasskeyRegisterBegin, PasskeyRegisterBeginRequestsPerMinute, time.Minute)
}

// RateLimitPasskeyRegisterBeginIPCeiling is the per-IP ceiling over every
// account's register begin budget: PasskeyRegisterBeginIPCeilingPerMinute per
// KeyByClientIP bucket. Chained INSIDE RateLimitPasskeyRegisterBeginUser by
// RateLimitBySessionUser (see the ORDER note there). Its log lines fingerprint
// the address bucket, and the user limiter's lines fingerprint the account
// bucket, so a refusal line names the bucket that refused.
func RateLimitPasskeyRegisterBeginIPCeiling() func(http.Handler) http.Handler {
	return limiterSpec{
		name:   LimiterPasskeyRegisterBeginIPCeiling,
		limit:  PasskeyRegisterBeginIPCeilingPerMinute,
		window: time.Minute,
		key:    KeyByClientIP,
	}.handler()
}

// meterSessionUser routes a request carrying a verified session token through
// metered with the session user's id in context, where mutationUserKeyFunc reads
// it, and any other request straight to next.
func meterSessionUser(jwtService *auth.JWTService, metered, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if uid, ok := sessionUserID(jwtService, r); ok {
			ctx := context.WithValue(r.Context(), mutationUserIDKey{}, uid)
			metered.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RateLimitMutationsByUser meters an authenticated mutation against a per-user
// budget: it stashes the user id from the verified session JWT into context and
// routes the request through burstLimiter (minute) OUTER and sustainedLimiter
// (hour) INNER, so a request must clear both. Which budget that is comes from the
// limiters passed in: callers sharing one pair of limiters share one bucket,
// callers passing their own pair get their own.
//
// UNAUTHENTICATED requests PASS THROUGH untouched: these endpoints sit behind a
// JWT middleware that 401s anonymous callers anyway, and the policy meters per
// authenticated user only (no anonymous/IP budget in v1). Passing through
// avoids keying a "user:0" bucket for requests that can never mutate.
//
// This is the per-user CORE only. Admin / validated-API-token BYPASS is layered by
// wrapping this in SkipRateLimitForAdmin (see routes.EngagementMutationRateLimiter
// and routes.EntityRequestBatchRateLimiter), reusing that audited helper rather
// than re-deriving the bypass condition here.
//
// ORDER — burst OUTER, sustained INNER: httprate increments a limiter's counter
// only when the request clears that limiter's own limit. With burst OUTER, a
// user hammering past the minute burst is 429'd by the OUTER limiter and never
// reaches (nor increments) the hour window — a bad minute cannot drain the
// hour budget. Both windows are keyed by the SAME user, so unlike the
// public-read per-IP ceiling there is no cross-user collateral either way; this
// order just matches the policy doc (outer = minute burst).
func RateLimitMutationsByUser(jwtService *auth.JWTService, burstLimiter, sustainedLimiter func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return meterSessionUser(jwtService, burstLimiter(sustainedLimiter(next)), next)
	}
}
