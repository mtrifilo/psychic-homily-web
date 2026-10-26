package indexnow

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// runRowMarks reads the mark from the loop's background_service_runs row: the
// last successful run's completion, last_success_at, minus runLease.
//
// The overlap equals the lease because runCycle cuts a run off at runLease, so
// no successful run lasted longer: its start is at most runLease before its
// completion, and every row changed during it has a lastmod later than the
// mark. The same overlap absorbs app-to-database clock skew, since lastmod is
// stamped on the app clock and last_success_at on the database clock. The cost is that a URL changed in the overlap is submitted again by
// the next run; IndexNow accepts a resubmission.
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
		Now           time.Time
		LastSuccessAt *time.Time
	}
	err := m.db.WithContext(ctx).Raw(
		`SELECT NOW() AS now,
		        (SELECT last_success_at FROM background_service_runs WHERE name = ?) AS last_success_at`,
		LoopName,
	).Scan(&row).Error
	if err != nil {
		return time.Time{}, time.Time{}, false, fmt.Errorf("read indexnow mark: %w", err)
	}
	if row.LastSuccessAt == nil {
		return row.Now, time.Time{}, false, nil
	}
	return row.Now, row.LastSuccessAt.Add(-runLease), true, nil
}

// Advance implements markStore.
func (runRowMarks) Advance(context.Context, time.Time) error { return nil }
