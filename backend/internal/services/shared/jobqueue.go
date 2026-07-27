package shared

import (
	"context"
	"log/slog"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// JobQueue is the claim/finalize/reclaim/prune mechanics shared by the
// database-backed work queues (PSY-1572). It is deliberately an EXTRACTION of the
// image-enrich outbox implementation (PSY-1247), which had all four and was
// hardened in production, rather than a fresh abstraction: the enrichment queue
// had none of them and could strand rows permanently.
//
// What the mechanics buy, and why each one is load-bearing:
//
//   - Claim uses SELECT ... FOR UPDATE SKIP LOCKED and flips rows to `processing`
//     inside the same short transaction. Without the lock, two pollers select the
//     same pending rows and both process them; without the single transaction, the
//     select and the flip are a TOCTOU window.
//   - Finalize is guarded by `status = processing`, so a late write from a worker
//     that lost its row to ReclaimStale is a no-op instead of a state clobber.
//   - ReclaimStale is the ONLY thing that recovers a row orphaned by a crash or a
//     deploy mid-batch. A queue without it leaks a permanent zombie every time a
//     finalize write fails, because the claim filter only looks at `pending`.
//   - PruneTerminal bounds table growth while keeping a short audit trail.
//
// The queue owns mechanics only. What work a row represents, how it is executed,
// and when a failure is terminal stay with the caller.
type JobQueue[T QueueRow] struct {
	db       *gorm.DB
	name     string
	statuses QueueStatuses
	logger   *slog.Logger
}

// QueueRow is the minimum a row type must expose. Everything else the mechanics
// touch (status, attempts, max_attempts, last_error, updated_at) is addressed as a
// column, so a queue can carry whatever domain fields it likes alongside.
type QueueRow interface {
	QueueRowID() uint
}

// QueueStatuses names one queue's status vocabulary. It is a parameter rather
// than a set of constants because the existing queues genuinely disagree: both
// use pending/processing/failed, but image_enrich_queue's success terminal is
// "done" while enrichment_queue's is "completed". Hardcoding either would force a
// data migration on the other for no benefit.
type QueueStatuses struct {
	Pending    string
	Processing string
	Failed     string
	// Terminal lists the statuses PruneTerminal may delete. It is explicit so a
	// queue cannot accidentally prune rows still in flight.
	Terminal []string
}

// NewJobQueue builds the mechanics for one table. name appears in log messages
// and should identify the queue, not the caller.
func NewJobQueue[T QueueRow](db *gorm.DB, name string, statuses QueueStatuses, logger *slog.Logger) *JobQueue[T] {
	if logger == nil {
		logger = slog.Default()
	}
	return &JobQueue[T]{db: db, name: name, statuses: statuses, logger: logger}
}

// Claim atomically takes up to batch pending rows that still have attempts left:
// it SELECTs them FOR UPDATE SKIP LOCKED and flips them to `processing` with
// attempts+1 in the same short transaction. Row locks release at commit, so the
// actual work runs OUTSIDE the transaction — holding locks across slow network
// I/O would be a long-transaction antipattern.
//
// Returned rows carry their PRE-increment attempts (scanned before the update),
// so a caller reasoning about the post-increment count uses attempts+1.
func (q *JobQueue[T]) Claim(ctx context.Context, batch int) ([]T, error) {
	var items []T
	err := q.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if ferr := tx.
			Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate, Options: clause.LockingOptionsSkipLocked}).
			Where("status = ? AND attempts < max_attempts", q.statuses.Pending).
			Order("created_at ASC").
			Limit(batch).
			Find(&items).Error; ferr != nil {
			return ferr
		}
		if len(items) == 0 {
			return nil
		}
		var zero T
		return tx.Model(&zero).
			Where("id IN ?", QueueRowIDs(items)).
			Updates(map[string]interface{}{
				"status":   q.statuses.Processing,
				"attempts": gorm.Expr("attempts + 1"),
			}).Error
	})
	return items, err
}

// Finalize applies a terminal/requeue update to the given ids, guarded by
// `status = processing` so a row that ReclaimStale (or another worker) has since
// moved is left untouched.
//
// The error is logged, not returned: callers finalize on their way out of a tick
// and have nothing useful to do with it. Logging is what makes the failure
// visible, and ReclaimStale is what makes it recoverable — the enrichment queue
// previously had neither, which is how a failed write became a permanent zombie.
//
// The guard does NOT cover the ABA case (reclaim re-pends the row and another
// worker re-claims it to `processing` before this write lands). Correctness there
// rests on the work being idempotent, and on the reclaim window exceeding the
// worst-case batch wall-clock.
func (q *JobQueue[T]) Finalize(ctx context.Context, ids []uint, updates map[string]interface{}) {
	if len(ids) == 0 {
		return
	}
	var zero T
	if err := q.db.WithContext(ctx).Model(&zero).
		Where("id IN ? AND status = ?", ids, q.statuses.Processing).
		Updates(updates).Error; err != nil {
		q.logger.Error(q.name+": finalize failed", "error", err, "ids", len(ids))
	}
}

// ReclaimStale recovers rows orphaned by a crash, a deploy mid-batch, or a failed
// finalize. A row stuck in `processing` past olderThan is returned to `pending` if
// it still has attempts left, or marked `failed` with strandedError once it has
// exhausted them.
//
// Failing the exhausted ones is essential, not tidy-up: leaving an exhausted row
// `pending` zombies it, because the claim filter is `attempts < max_attempts` — it
// would never re-claim, never finalize, and hold its slot forever.
//
// olderThan MUST exceed the worst-case batch wall-clock, or this will reclaim rows
// that are still legitimately in flight and burn an attempt on work that is
// succeeding. Measure it; do not assume it. (PSY-1569 measured 9-14 minutes for a
// 20-item image-enrich batch against a documented estimate of 1-2 minutes.)
func (q *JobQueue[T]) ReclaimStale(ctx context.Context, olderThan time.Duration, strandedError string) {
	cutoff := time.Now().Add(-olderThan)
	var zero T

	// Two statements, not one: their attempts predicates are disjoint AND the first
	// moves its matched rows out of `processing`, so no row is hit by both and none
	// escapes both. Do NOT merge them into one UPDATE — the split is what keeps the
	// retry/fail partition clean.
	retry := q.db.WithContext(ctx).Model(&zero).
		Where("status = ? AND updated_at < ? AND attempts < max_attempts", q.statuses.Processing, cutoff).
		Update("status", q.statuses.Pending)
	if retry.Error != nil {
		q.logger.Error(q.name+": reclaim (retry) failed", "error", retry.Error)
	}

	failed := q.db.WithContext(ctx).Model(&zero).
		Where("status = ? AND updated_at < ? AND attempts >= max_attempts", q.statuses.Processing, cutoff).
		Updates(map[string]interface{}{
			"status":     q.statuses.Failed,
			"last_error": strandedError,
		})
	if failed.Error != nil {
		q.logger.Error(q.name+": reclaim (fail) failed", "error", failed.Error)
	}

	if n := retry.RowsAffected + failed.RowsAffected; n > 0 {
		q.logger.Warn(q.name+": reclaimed stale processing rows",
			"requeued", retry.RowsAffected, "failed", failed.RowsAffected)
	}
}

// PruneTerminal deletes rows in a terminal status older than retention, keeping a
// short audit trail while bounding table growth. Returns rows deleted.
func (q *JobQueue[T]) PruneTerminal(ctx context.Context, retention time.Duration) int64 {
	if len(q.statuses.Terminal) == 0 {
		return 0
	}
	cutoff := time.Now().Add(-retention)
	var zero T
	res := q.db.WithContext(ctx).
		Where("status IN ? AND updated_at < ?", q.statuses.Terminal, cutoff).
		Delete(&zero)
	if res.Error != nil {
		q.logger.Error(q.name+": prune failed", "error", res.Error)
		return 0
	}
	return res.RowsAffected
}

// QueueRowIDs extracts the ids of a claimed batch.
func QueueRowIDs[T QueueRow](items []T) []uint {
	ids := make([]uint, 0, len(items))
	for _, it := range items {
		ids = append(ids, it.QueueRowID())
	}
	return ids
}
