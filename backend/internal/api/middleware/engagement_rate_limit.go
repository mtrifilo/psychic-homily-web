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
// ONE shared per-user budget covers show/release save+unsave AND entity/scene
// follow+unfollow — the same inline UX burst on Broadsheet chart rows, so a
// separate budget per action would just invite "rotate which button to spam."
// A request must clear BOTH windows (stricter wins).
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

// engagementUserIDKey is the context key under which
// RateLimitEngagementMutationsByUser stashes the authenticated user id so the
// per-user limiters' key func can read it without re-parsing the token. It is
// deliberately distinct from the public-read limiter's key so the two never
// share a bucket even though both key "user:<id>".
type engagementUserIDKey struct{}

// engagementUserKeyFunc keys the engagement limiters by the user id
// RateLimitEngagementMutationsByUser stashes in context. If the value is absent
// it FAILS LOUD (httprate turns a key-func error into a 428) rather than
// silently keying every request into one shared bucket — so mounting a bare
// engagement limiter without the wrapper that sets the id is a detectable
// misuse, not a single site-wide budget.
func engagementUserKeyFunc(r *http.Request) (string, error) {
	uid, ok := r.Context().Value(engagementUserIDKey{}).(uint)
	if !ok {
		return "", fmt.Errorf(
			"engagementUserKeyFunc: no authenticated user id in context; " +
				"engagement limiters must be mounted via RateLimitEngagementMutationsByUser")
	}
	return "user:" + strconv.FormatUint(uint64(uid), 10), nil
}

// RateLimitEngagementMutationBurst is the per-USER burst limiter:
// EngagementMutationBurstPerMinute per user id (NOT per IP), so shared-IP
// logged-in users each get their own bucket. Pair with
// RateLimitEngagementMutationsByUser, which supplies the user id via context.
func RateLimitEngagementMutationBurst() func(http.Handler) http.Handler {
	return httprate.Limit(
		EngagementMutationBurstPerMinute,
		time.Minute,
		httprate.WithKeyFuncs(engagementUserKeyFunc),
		httprate.WithLimitHandler(RateLimitExceededHandler),
	)
}

// RateLimitEngagementMutationSustained is the per-USER sustained limiter:
// EngagementMutationSustainedPerHour per user id. Chained INSIDE the burst
// limiter (see the ORDER note on RateLimitEngagementMutationsByUser).
func RateLimitEngagementMutationSustained() func(http.Handler) http.Handler {
	return httprate.Limit(
		EngagementMutationSustainedPerHour,
		time.Hour,
		httprate.WithKeyFuncs(engagementUserKeyFunc),
		httprate.WithLimitHandler(RateLimitExceededHandler),
	)
}

// RateLimitEngagementMutationsByUser meters authenticated engagement-toggle
// mutations (save/follow) against a SHARED per-user budget: it stashes the user
// id from the verified session JWT into context and routes the request through
// burstLimiter (minute) OUTER and sustainedLimiter (hour) INNER — a request
// must clear both.
//
// BYPASS: admin JWTs and trusted phk_ API tokens skip the limiter entirely,
// matching SkipRateLimitForAdmin — bulk import / dogfood sessions must not fight
// the ceiling. Reuses the same isTrustedAPIToken / isAdminTokenRequest helpers.
//
// UNAUTHENTICATED requests PASS THROUGH untouched: these endpoints sit behind a
// JWT middleware that 401s anonymous callers anyway, and the policy meters per
// authenticated user only (no anonymous/IP budget in v1). Passing through
// avoids keying a "user:0" bucket for requests that can never mutate.
//
// ORDER — burst OUTER, sustained INNER: httprate increments a limiter's counter
// only when the request clears that limiter's own limit. With burst OUTER, a
// user hammering past the minute burst is 429'd by the OUTER limiter and never
// reaches (nor increments) the hour window — a bad minute cannot drain the
// hour budget. Both windows are keyed by the SAME user, so unlike the
// public-read per-IP ceiling there is no cross-user collateral either way; this
// order just matches the policy doc (outer = minute burst).
func RateLimitEngagementMutationsByUser(jwtService *auth.JWTService, burstLimiter, sustainedLimiter func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		limited := burstLimiter(sustainedLimiter(next))
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isTrustedAPIToken(r) || isAdminTokenRequest(jwtService, r) {
				next.ServeHTTP(w, r)
				return
			}
			if uid, ok := sessionUserID(jwtService, r); ok {
				ctx := context.WithValue(r.Context(), engagementUserIDKey{}, uid)
				limited.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
