// Package ratelimit holds the Postgres-backed per-user rate limiter for the AI
// extraction routes (extract-show + extract-collection, PSY-855).
//
// The two Next.js BFF extract routes proxy to this backend before doing any
// (paid) Anthropic work. The counter MUST be Postgres-backed — not in-memory —
// because the routes run on Vercel serverless instances that don't share
// process memory, so an in-memory counter would let a user multiply the limit
// by the number of warm instances.
package ratelimit

import (
	"fmt"
	"math"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
)

const (
	// AIExtractionLimit is the number of AI extractions a single non-admin user
	// may run per rolling window. Applies identically to both extract routes —
	// they share one counter so a user can't get 10 of each (PSY-855 spec'd a
	// single 10/hr budget).
	AIExtractionLimit = 10

	// AIExtractionWindow is the rolling window the limit applies over.
	AIExtractionWindow = time.Hour
)

// ThrottleDecision is the result of an increment-and-check. Allowed is false
// once the user has exceeded AIExtractionLimit within the current window;
// RetryAfterSeconds is then the whole-second wait until the window resets
// (always >= 1 when Allowed is false).
type ThrottleDecision struct {
	Allowed           bool
	RetryAfterSeconds int
}

// AIExtractionThrottleService gates the AI extraction routes behind a per-user
// rolling-window counter.
type AIExtractionThrottleService struct {
	db *gorm.DB
}

// NewAIExtractionThrottleService creates the throttle service.
func NewAIExtractionThrottleService(database *gorm.DB) *AIExtractionThrottleService {
	if database == nil {
		database = db.GetDB()
	}
	return &AIExtractionThrottleService{db: database}
}

// scanRow is the post-write window state returned by the atomic upsert.
type scanRow struct {
	WindowStart  time.Time `gorm:"column:window_start"`
	RequestCount int       `gorm:"column:request_count"`
}

// CheckAndIncrement atomically records one extraction attempt for the user and
// reports whether it is allowed under the limit.
//
// Atomicity matters: two concurrent serverless instances must not both read
// count=9, both decide "allowed", and both write count=10 — that would let 11
// through. The whole reset-or-increment decision happens in a SINGLE upsert
// statement (the CASE in the ON CONFLICT clause), so Postgres row locking
// serializes concurrent callers and RETURNING hands back the authoritative
// post-write state in the same round-trip.
//
// The window resets in-place when it has elapsed: if NOW() >= window_start +
// window, the row's window_start becomes NOW() and request_count becomes 1.
// Otherwise request_count is incremented. The caller is allowed iff the
// resulting count is within AIExtractionLimit.
//
// Note: a blocked attempt still increments the count. That's harmless —
// RetryAfterSeconds is derived from window_start (not the count), so a user
// hammering the route past the limit doesn't extend their own cooldown.
func (s *AIExtractionThrottleService) CheckAndIncrement(userID uint) (ThrottleDecision, error) {
	if s.db == nil {
		return ThrottleDecision{}, fmt.Errorf("database not initialized")
	}
	if userID == 0 {
		return ThrottleDecision{}, fmt.Errorf("userID is required")
	}

	windowSeconds := int(AIExtractionWindow / time.Second)

	var row scanRow
	// The interval is built from windowSeconds (a constant int) via make_interval,
	// not string-interpolated, so there's no injection surface; userID is bound.
	err := s.db.Raw(`
		INSERT INTO ai_extraction_throttle (user_id, window_start, request_count, created_at, updated_at)
		VALUES (?, NOW(), 1, NOW(), NOW())
		ON CONFLICT (user_id) DO UPDATE SET
			window_start = CASE
				WHEN ai_extraction_throttle.window_start + make_interval(secs => ?) <= NOW()
					THEN NOW()
				ELSE ai_extraction_throttle.window_start
			END,
			request_count = CASE
				WHEN ai_extraction_throttle.window_start + make_interval(secs => ?) <= NOW()
					THEN 1
				ELSE ai_extraction_throttle.request_count + 1
			END,
			updated_at = NOW()
		RETURNING window_start, request_count
	`, userID, windowSeconds, windowSeconds).Scan(&row).Error
	if err != nil {
		return ThrottleDecision{}, fmt.Errorf("failed to record extraction attempt: %w", err)
	}

	if row.RequestCount <= AIExtractionLimit {
		return ThrottleDecision{Allowed: true}, nil
	}

	// Over the limit — compute the whole-second wait until the window resets.
	// Clamp to >= 1 so the Retry-After header / "try again in N" copy never
	// reads "0 seconds" due to sub-second rounding at the window boundary.
	resetAt := row.WindowStart.Add(AIExtractionWindow)
	retryAfter := int(math.Ceil(time.Until(resetAt).Seconds()))
	if retryAfter < 1 {
		retryAfter = 1
	}

	return ThrottleDecision{Allowed: false, RetryAfterSeconds: retryAfter}, nil
}
