package shared

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/internal/testutil"
)

// jobQueueTestRow is a self-contained fixture: the shared mechanics address rows
// by column, so these tests do not depend on either real caller's schema and stay
// valid if image_enrich_queue or enrichment_queue changes shape.
type jobQueueTestRow struct {
	ID          uint `gorm:"primaryKey"`
	Status      string
	Attempts    int
	MaxAttempts int
	LastError   *string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (jobQueueTestRow) TableName() string { return "job_queue_test_rows" }

func (r jobQueueTestRow) QueueRowID() uint { return r.ID }

func testStatuses() QueueStatuses {
	return QueueStatuses{
		Pending:    "pending",
		Processing: "processing",
		Failed:     "failed",
		Terminal:   []string{"done", "failed"},
	}
}

func setupJobQueue(t *testing.T) (*gorm.DB, *JobQueue[jobQueueTestRow]) {
	t.Helper()
	tdb := testutil.SetupTestPostgres(t)
	t.Cleanup(tdb.Cleanup)
	if err := tdb.DB.Exec(`CREATE TABLE job_queue_test_rows (
		id BIGSERIAL PRIMARY KEY,
		status TEXT NOT NULL DEFAULT 'pending',
		attempts INT NOT NULL DEFAULT 0,
		max_attempts INT NOT NULL DEFAULT 3,
		last_error TEXT,
		created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT now())`).Error; err != nil {
		t.Fatalf("create fixture table: %v", err)
	}
	return tdb.DB, NewJobQueue[jobQueueTestRow](tdb.DB, "test queue", testStatuses(), nil)
}

func seedRows(t *testing.T, db *gorm.DB, n int, status string, attempts int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if err := db.Exec(
			`INSERT INTO job_queue_test_rows (status, attempts, max_attempts) VALUES (?, ?, 3)`,
			status, attempts).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
}

// TestClaim_FlipsAtomicallyAndIncrements pins the claim contract: rows come back
// with PRE-increment attempts, while the row itself is already `processing` with
// attempts+1 — both done in one transaction.
func TestClaim_FlipsAtomicallyAndIncrements(t *testing.T) {
	db, q := setupJobQueue(t)
	seedRows(t, db, 3, "pending", 0)

	items, err := q.Claim(context.Background(), 10)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("claimed %d rows, want 3", len(items))
	}
	for _, it := range items {
		if it.Attempts != 0 {
			t.Errorf("returned row carries attempts=%d, want the PRE-increment 0", it.Attempts)
		}
	}

	var processing int64
	db.Model(&jobQueueTestRow{}).Where("status = ? AND attempts = 1", "processing").Count(&processing)
	if processing != 3 {
		t.Errorf("rows flipped to processing with attempts=1: %d, want 3", processing)
	}
}

// TestClaim_SkipsRowsLockedByAnotherTransaction is the deterministic proof of
// FOR UPDATE SKIP LOCKED. An open transaction holds a row lock; a concurrent Claim
// must step over that row rather than block on it or double-claim it.
//
// This is deliberately not a goroutine race: a timing-based test can pass against
// an unlocked implementation by luck, which is exactly what happened when the
// caller-level concurrency test was run against the pre-PSY-1572 code.
func TestClaim_SkipsRowsLockedByAnotherTransaction(t *testing.T) {
	db, q := setupJobQueue(t)
	seedRows(t, db, 2, "pending", 0)

	var first jobQueueTestRow
	if err := db.Order("id ASC").First(&first).Error; err != nil {
		t.Fatalf("load first: %v", err)
	}

	// Hold a lock on the first row for the duration of the Claim below.
	locked := make(chan struct{})
	release := make(chan struct{})
	go func() {
		_ = db.Transaction(func(tx *gorm.DB) error {
			var row jobQueueTestRow
			if err := tx.Raw(
				`SELECT * FROM job_queue_test_rows WHERE id = ? FOR UPDATE`, first.ID).
				Scan(&row).Error; err != nil {
				return err
			}
			close(locked)
			<-release
			return nil
		})
	}()
	<-locked

	items, err := q.Claim(context.Background(), 10)
	close(release)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}

	if len(items) != 1 {
		t.Fatalf("claimed %d rows, want 1 (the locked row must be skipped, not blocked on)", len(items))
	}
	if items[0].ID == first.ID {
		t.Errorf("claimed the locked row %d; SKIP LOCKED is not in effect", first.ID)
	}
}

// TestFinalize_OnlyTouchesProcessingRows pins the status='processing' guard: a
// late write from a worker that already lost its row must be a no-op, not a clobber.
func TestFinalize_OnlyTouchesProcessingRows(t *testing.T) {
	db, q := setupJobQueue(t)
	seedRows(t, db, 1, "pending", 0)

	var row jobQueueTestRow
	if err := db.First(&row).Error; err != nil {
		t.Fatalf("load: %v", err)
	}

	q.Finalize(context.Background(), []uint{row.ID}, map[string]interface{}{"status": "done"})

	var got jobQueueTestRow
	if err := db.First(&got, row.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.Status != "pending" {
		t.Errorf("finalize clobbered a non-processing row: status=%q, want pending", got.Status)
	}
}

// TestReclaimStale_PartitionsRetryAndFail pins the split that keeps a stranded row
// from zombieing: attempts left -> back to pending; exhausted -> failed with the
// sentinel. Fresh rows are untouched so reclaim never burns an attempt on work
// that is still running.
func TestReclaimStale_PartitionsRetryAndFail(t *testing.T) {
	db, q := setupJobQueue(t)
	const sentinel = "stranded"

	seedRows(t, db, 1, "processing", 1) // stale, attempts left  -> pending
	seedRows(t, db, 1, "processing", 3) // stale, exhausted      -> failed
	if err := db.Exec(`UPDATE job_queue_test_rows SET updated_at = now() - interval '1 hour'`).Error; err != nil {
		t.Fatalf("age rows: %v", err)
	}
	seedRows(t, db, 1, "processing", 1) // fresh, still in flight -> untouched

	q.ReclaimStale(context.Background(), 30*time.Minute, sentinel)

	counts := map[string]int64{}
	for _, st := range []string{"pending", "failed", "processing"} {
		var n int64
		db.Model(&jobQueueTestRow{}).Where("status = ?", st).Count(&n)
		counts[st] = n
	}
	if counts["pending"] != 1 {
		t.Errorf("stale row with attempts left -> pending: got %d, want 1", counts["pending"])
	}
	if counts["failed"] != 1 {
		t.Errorf("stale exhausted row -> failed: got %d, want 1", counts["failed"])
	}
	if counts["processing"] != 1 {
		t.Errorf("fresh in-flight row must be untouched: processing=%d, want 1", counts["processing"])
	}

	var failed jobQueueTestRow
	if err := db.Where("status = ?", "failed").First(&failed).Error; err != nil {
		t.Fatalf("load failed row: %v", err)
	}
	if failed.LastError == nil || *failed.LastError != sentinel {
		t.Errorf("failed row must carry the sentinel, got %v", failed.LastError)
	}
}

// TestPruneTerminal_DeletesOnlyAgedTerminalRows: prune must bound growth without
// touching anything still in flight.
func TestPruneTerminal_DeletesOnlyAgedTerminalRows(t *testing.T) {
	db, q := setupJobQueue(t)

	seedRows(t, db, 2, "done", 1)
	seedRows(t, db, 1, "failed", 3)
	seedRows(t, db, 1, "pending", 0)
	if err := db.Exec(`UPDATE job_queue_test_rows SET updated_at = now() - interval '30 days'`).Error; err != nil {
		t.Fatalf("age rows: %v", err)
	}
	seedRows(t, db, 1, "done", 1) // fresh terminal row, inside retention

	deleted := q.PruneTerminal(context.Background(), 7*24*time.Hour)
	if deleted != 3 {
		t.Errorf("pruned %d rows, want 3 (2 done + 1 failed, all aged)", deleted)
	}

	var remaining int64
	db.Model(&jobQueueTestRow{}).Count(&remaining)
	if remaining != 2 {
		t.Errorf("remaining rows: %d, want 2 (the pending one and the fresh done one)", remaining)
	}
}
