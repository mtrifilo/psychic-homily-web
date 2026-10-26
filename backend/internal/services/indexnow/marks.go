package indexnow

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// clockSkewMargin is added to the last successful run's duration to form the
// overlap. It covers the gap between the run row's database clock and the app
// clock lastmod is stamped on, the claim and close-out statements that the
// recorded duration leaves out, and a commit that lands after its stamp.
const clockSkewMargin = time.Minute

// runRowMarks reads the mark from the loop's background_service_runs row: the
// last successful run's completion, last_success_at, minus an overlap.
//
// The overlap must reach back to that run's start, so every row changed while
// it ran has a lastmod later than the mark. When the row's latest completion
// is that success (last_completed_at equals last_success_at), last_duration_ms
// is its duration and the overlap is that duration plus clockSkewMargin. A
// later failed or panicked run overwrites last_duration_ms with its own, and a
// row may carry no duration at all; then the overlap is runLease, which bounds
// every run because runCycle cuts a run off at runLease.
//
// The cost: a URL changed within the overlap before the last success is
// submitted again by the next run, so with the duration-sized overlap a
// changed URL is submitted at most twice; after a failure, the lease-sized
// overlap holds it for about runLease / interval + 1 runs. IndexNow accepts a
// resubmission.
//
// Advance writes nothing: RunScheduledLoop stamps last_success_at when a cycle
// reports no error, and only then.
type runRowMarks struct {
	db *gorm.DB
}

// Begin implements markStore. A row with no last_success_at, or no row, is no
// mark.
func (m runRowMarks) Begin(ctx context.Context) (time.Time, time.Time, bool, error) {
	var row struct {
		Now             time.Time
		LastSuccessAt   *time.Time
		LastCompletedAt *time.Time
		LastDurationMs  *int64
	}
	err := m.db.WithContext(ctx).Raw(
		`SELECT NOW() AS now, r.last_success_at, r.last_completed_at, r.last_duration_ms
		   FROM (SELECT 1) AS one
		   LEFT JOIN background_service_runs r ON r.name = ?`,
		LoopName,
	).Scan(&row).Error
	if err != nil {
		return time.Time{}, time.Time{}, false, fmt.Errorf("read indexnow mark: %w", err)
	}
	if row.LastSuccessAt == nil {
		return row.Now, time.Time{}, false, nil
	}
	return row.Now, row.LastSuccessAt.Add(-markOverlap(*row.LastSuccessAt, row.LastCompletedAt, row.LastDurationMs)), true, nil
}

// markOverlap is how far before lastSuccess the mark sits; see runRowMarks.
func markOverlap(lastSuccess time.Time, lastCompleted *time.Time, lastDurationMs *int64) time.Duration {
	if lastDurationMs == nil || *lastDurationMs < 0 || lastCompleted == nil || !lastCompleted.Equal(lastSuccess) {
		return runLease
	}
	return time.Duration(*lastDurationMs)*time.Millisecond + clockSkewMargin
}

// Advance implements markStore.
func (runRowMarks) Advance(context.Context, time.Time) error { return nil }
