package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/httprate"

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
// human click rate. Halving the shared burst window leaves 5x headroom over that
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
// RateLimitMutationsByUser stashes the authenticated user id so the
// per-user limiters' key func can read it without re-parsing the token. It is
// deliberately distinct from the public-read limiter's key so the two never
// share a bucket even though both key "user:<id>".
type mutationUserIDKey struct{}

// mutationUserKeyFunc keys every per-user mutation limiter by the user id
// RateLimitMutationsByUser stashes in context. If the value is absent
// it FAILS LOUD (httprate turns a key-func error into a 428) rather than
// silently keying every request into one shared bucket — so mounting a bare
// per-user limiter without the wrapper that sets the id is a detectable
// misuse, not a single site-wide budget.
func mutationUserKeyFunc(r *http.Request) (string, error) {
	uid, ok := r.Context().Value(mutationUserIDKey{}).(uint)
	if !ok {
		return "", fmt.Errorf(
			"mutationUserKeyFunc: no authenticated user id in context; " +
				"per-user mutation limiters must be mounted via RateLimitMutationsByUser")
	}
	return "user:" + strconv.FormatUint(uint64(uid), 10), nil
}

// perUserMutationLimiter builds one per-USER window: limit requests per window,
// keyed by the user id RateLimitMutationsByUser stashes in context (NOT by IP,
// so shared-IP logged-in users each get their own bucket). Every call mints its
// OWN httprate store, so two budgets built here never draw on one counter.
func perUserMutationLimiter(limit int, window time.Duration) func(http.Handler) http.Handler {
	return httprate.Limit(
		limit,
		window,
		httprate.WithKeyFuncs(mutationUserKeyFunc),
		httprate.WithLimitHandler(RateLimitExceededHandler),
	)
}

// RateLimitEngagementMutationBurst is the shared budget's minute window. Pair
// with RateLimitMutationsByUser, which supplies the user id via context.
func RateLimitEngagementMutationBurst() func(http.Handler) http.Handler {
	return perUserMutationLimiter(EngagementMutationBurstPerMinute, time.Minute)
}

// RateLimitEngagementMutationSustained is the shared budget's hour window.
// Chained INSIDE the burst limiter (see the ORDER note on
// RateLimitMutationsByUser).
func RateLimitEngagementMutationSustained() func(http.Handler) http.Handler {
	return perUserMutationLimiter(EngagementMutationSustainedPerHour, time.Hour)
}

// RateLimitEntityRequestBatchBurst is the entity-request batch budget's minute
// window.
func RateLimitEntityRequestBatchBurst() func(http.Handler) http.Handler {
	return perUserMutationLimiter(EntityRequestBatchBurstPerMinute, time.Minute)
}

// RateLimitEntityRequestBatchSustained is the entity-request batch budget's hour
// window. Chained INSIDE the burst limiter (see the ORDER note on
// RateLimitMutationsByUser).
func RateLimitEntityRequestBatchSustained() func(http.Handler) http.Handler {
	return perUserMutationLimiter(EntityRequestBatchSustainedPerHour, time.Hour)
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
		limited := burstLimiter(sustainedLimiter(next))
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if uid, ok := sessionUserID(jwtService, r); ok {
				ctx := context.WithValue(r.Context(), mutationUserIDKey{}, uid)
				limited.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
