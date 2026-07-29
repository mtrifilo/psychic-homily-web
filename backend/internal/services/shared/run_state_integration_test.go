package shared

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/internal/testutil"
)

// The scheduler's correctness ultimately rests on one SQL statement — the claim
// — and on the schedule being read off the DATABASE clock. Neither can be
// proved with an in-memory double: a mistake in the ON CONFLICT predicate or a
// timestamp type would still pass every unit test and then silently let two
// instances run the same sweep in production. These tests run the real
// statements against real Postgres.

// One container for the whole file: these cases are cheap SQL round trips and
// the table is reset between them, so paying container startup per test would
// dominate the package's runtime for no isolation benefit.
var (
	runStoreDBOnce sync.Once
	runStoreDB     *gorm.DB
	runStoreClean  func()
)

func setupRunStore(t *testing.T) (*gorm.DB, *GormRunStore) {
	t.Helper()
	runStoreDBOnce.Do(func() {
		tdb := testutil.SetupTestPostgres(t)
		runStoreDB = tdb.DB
		runStoreClean = tdb.Cleanup
	})
	if runStoreDB == nil {
		t.Fatal("run-store test database unavailable")
	}
	if err := runStoreDB.Exec(`DELETE FROM background_service_runs`).Error; err != nil {
		t.Fatalf("reset run-state table: %v", err)
	}
	return runStoreDB, NewGormRunStore(runStoreDB)
}

// TestMain owns the shared container's lifetime; a t.Cleanup would tear it down
// after whichever test happened to create it, leaving the rest with no database.
func TestMain(m *testing.M) {
	code := m.Run()
	if runStoreClean != nil {
		runStoreClean()
	}
	os.Exit(code)
}

// backdateCompletion moves a row's completion into the past USING THE DATABASE
// CLOCK, so the test never depends on the test process and the database agreeing
// on what time it is.
func backdateCompletion(t *testing.T, db *gorm.DB, name string, ago time.Duration) {
	t.Helper()
	err := db.Exec(`
		UPDATE background_service_runs
		SET last_completed_at = NOW() - make_interval(secs => ?),
		    last_started_at   = NOW() - make_interval(secs => ?),
		    last_outcome      = 'success'
		WHERE name = ?`, ago.Seconds(), ago.Seconds(), name).Error
	if err != nil {
		t.Fatalf("backdate %s: %v", name, err)
	}
}

func TestGormRunStore_NeverRunLoopIsOverdue(t *testing.T) {
	_, store := setupRunStore(t)
	ctx := context.Background()

	due, err := store.DueIn(ctx, "fresh-loop", 24*time.Hour)
	if err != nil {
		t.Fatalf("DueIn: %v", err)
	}
	if due > 0 {
		t.Fatalf("a loop with no row must read as overdue, got %v", due)
	}
}

func TestGormRunStore_ClaimCompleteRoundTrip(t *testing.T) {
	db, store := setupRunStore(t)
	ctx := context.Background()

	token, ok, err := store.Claim(ctx, "round-trip", 24*time.Hour, time.Hour, false)
	if err != nil || !ok {
		t.Fatalf("first claim must succeed: ok=%v err=%v", ok, err)
	}
	if token.IsZero() {
		t.Fatal("claim must return the token identifying this run")
	}

	rows := 42
	if err := store.Complete(ctx, "round-trip", token, CycleOutcome{
		Rows:     &rows,
		Duration: 1500 * time.Millisecond,
	}); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	var got BackgroundServiceRun
	if err := db.Table("background_service_runs").Where("name = ?", "round-trip").Take(&got).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got.LastOutcome == nil || *got.LastOutcome != RunOutcomeSuccess {
		t.Fatalf("outcome = %v, want success", got.LastOutcome)
	}
	if got.LastCompletedAt == nil || got.LastSuccessAt == nil {
		t.Fatal("a successful cycle must stamp both completion and success")
	}
	if got.LastRowsProcessed == nil || *got.LastRowsProcessed != 42 {
		t.Fatalf("rows processed = %v, want 42", got.LastRowsProcessed)
	}
	if got.LastDurationMs == nil || *got.LastDurationMs != 1500 {
		t.Fatalf("duration ms = %v, want 1500", got.LastDurationMs)
	}
	if got.RunCount != 1 || got.ConsecutiveFailures != 0 {
		t.Fatalf("run_count=%d failures=%d, want 1/0", got.RunCount, got.ConsecutiveFailures)
	}
	if got.IntervalSeconds != int64((24 * time.Hour).Seconds()) {
		t.Fatalf("interval_seconds = %d; overdue alerting reads this", got.IntervalSeconds)
	}

	// Immediately after completing, the loop is not due again.
	due, err := store.DueIn(ctx, "round-trip", 24*time.Hour)
	if err != nil {
		t.Fatalf("DueIn: %v", err)
	}
	if due <= 23*time.Hour {
		t.Fatalf("next due should be ~24h out, got %v", due)
	}
}

func TestGormRunStore_ClaimRefusedWhenNotDue(t *testing.T) {
	db, store := setupRunStore(t)
	ctx := context.Background()

	token, _, err := store.Claim(ctx, "not-due", 24*time.Hour, time.Hour, false)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if err := store.Complete(ctx, "not-due", token, CycleOutcome{}); err != nil {
		t.Fatalf("complete: %v", err)
	}

	_, ok, err := store.Claim(ctx, "not-due", 24*time.Hour, time.Hour, false)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if ok {
		t.Fatal("a loop that just completed must not be claimable again")
	}

	// ... until it is due. Backdating through the database keeps the assertion
	// about the database's own clock.
	backdateCompletion(t, db, "not-due", 25*time.Hour)
	_, ok, err = store.Claim(ctx, "not-due", 24*time.Hour, time.Hour, false)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if !ok {
		t.Fatal("an overdue loop must be claimable")
	}
}

// TestGormRunStore_ForcedClaimIgnoresDueButNotLease pins the exact asymmetry the
// boot cycle depends on.
func TestGormRunStore_ForcedClaimIgnoresDueButNotLease(t *testing.T) {
	_, store := setupRunStore(t)
	ctx := context.Background()

	token, _, err := store.Claim(ctx, "forced", 24*time.Hour, time.Hour, false)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if err := store.Complete(ctx, "forced", token, CycleOutcome{}); err != nil {
		t.Fatalf("complete: %v", err)
	}

	forcedToken, ok, err := store.Claim(ctx, "forced", 24*time.Hour, time.Hour, true)
	if err != nil || !ok {
		t.Fatalf("a forced claim must ignore the due check: ok=%v err=%v", ok, err)
	}

	// The forced claim is now live; another forced claim must still be refused.
	_, ok, err = store.Claim(ctx, "forced", 24*time.Hour, time.Hour, true)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if ok {
		t.Fatal("force must not override the concurrency check — that is the deploy-overlap guard")
	}
	_ = forcedToken
}

// TestGormRunStore_ConcurrentClaimsElectExactlyOneWinner is the multi-instance
// assertion. Twenty goroutines on separate connections race for the same overdue
// loop; the ON CONFLICT predicate must let exactly one through.
func TestGormRunStore_ConcurrentClaimsElectExactlyOneWinner(t *testing.T) {
	_, store := setupRunStore(t)
	ctx := context.Background()

	const racers = 20
	var (
		mu       sync.Mutex
		winners  int
		firstErr error
		wg       sync.WaitGroup
		gate     = make(chan struct{})
	)
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-gate
			_, ok, err := store.Claim(ctx, "contested", 24*time.Hour, time.Hour, false)
			mu.Lock()
			defer mu.Unlock()
			if err != nil && firstErr == nil {
				firstErr = err
			}
			if ok {
				winners++
			}
		}()
	}
	close(gate)
	wg.Wait()

	if firstErr != nil {
		t.Fatalf("claim error during race: %v", firstErr)
	}
	if winners != 1 {
		t.Fatalf("exactly one instance may claim an overdue loop; %d did", winners)
	}
}

// TestGormRunStore_StaleClaimIsReclaimableAfterLease covers the crashed-process
// case: a claim left behind by a process that died must not wedge the loop
// forever, but must hold for the lease so a live run is not doubled.
func TestGormRunStore_StaleClaimIsReclaimableAfterLease(t *testing.T) {
	db, store := setupRunStore(t)
	ctx := context.Background()

	if _, ok, err := store.Claim(ctx, "crashed", 24*time.Hour, time.Hour, false); err != nil || !ok {
		t.Fatalf("initial claim: ok=%v err=%v", ok, err)
	}

	if _, ok, err := store.Claim(ctx, "crashed", 24*time.Hour, time.Hour, false); err != nil || ok {
		t.Fatalf("a live claim must block a second one: ok=%v err=%v", ok, err)
	}

	// Age the claim past its lease, again on the database clock.
	if err := db.Exec(`
		UPDATE background_service_runs
		SET last_started_at = NOW() - make_interval(secs => ?)
		WHERE name = 'crashed'`, (2 * time.Hour).Seconds()).Error; err != nil {
		t.Fatalf("age claim: %v", err)
	}

	if _, ok, err := store.Claim(ctx, "crashed", 24*time.Hour, time.Hour, false); err != nil || !ok {
		t.Fatalf("an abandoned claim must be reclaimable after the lease: ok=%v err=%v", ok, err)
	}
}

// TestGormRunStore_SupersededCompleteIsRejected stops a run that overran its
// lease from clobbering the state of whichever run replaced it.
func TestGormRunStore_SupersededCompleteIsRejected(t *testing.T) {
	db, store := setupRunStore(t)
	ctx := context.Background()

	staleToken, _, err := store.Claim(ctx, "superseded", 24*time.Hour, time.Hour, false)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}

	if err := db.Exec(`
		UPDATE background_service_runs
		SET last_started_at = NOW() - make_interval(secs => ?)
		WHERE name = 'superseded'`, (2 * time.Hour).Seconds()).Error; err != nil {
		t.Fatalf("age claim: %v", err)
	}
	if _, ok, err := store.Claim(ctx, "superseded", 24*time.Hour, time.Hour, false); err != nil || !ok {
		t.Fatalf("reclaim: ok=%v err=%v", ok, err)
	}

	err = store.Complete(ctx, "superseded", staleToken, CycleOutcome{})
	if !errors.Is(err, ErrClaimSuperseded) {
		t.Fatalf("stale Complete must be rejected, got %v", err)
	}
}

// TestGormRunStore_FailureRecordsHealthButKeepsCadence is the anti-starvation
// property: a failing sweep must be visible AND must not fire on every deploy.
func TestGormRunStore_FailureRecordsHealthButKeepsCadence(t *testing.T) {
	db, store := setupRunStore(t)
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		token, ok, err := store.Claim(ctx, "flaky", 24*time.Hour, time.Hour, true)
		if err != nil || !ok {
			t.Fatalf("claim %d: ok=%v err=%v", i, ok, err)
		}
		if err := store.Complete(ctx, "flaky", token, CycleOutcome{Err: errors.New("provider down")}); err != nil {
			t.Fatalf("complete %d: %v", i, err)
		}
	}

	var got BackgroundServiceRun
	if err := db.Table("background_service_runs").Where("name = ?", "flaky").Take(&got).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got.LastOutcome == nil || *got.LastOutcome != RunOutcomeError {
		t.Fatalf("outcome = %v, want error", got.LastOutcome)
	}
	if got.ConsecutiveFailures != 2 {
		t.Fatalf("consecutive_failures = %d, want 2", got.ConsecutiveFailures)
	}
	if got.LastSuccessAt != nil {
		t.Fatal("a failing loop must never advance last_success_at")
	}
	if got.LastCompletedAt == nil {
		t.Fatal("a failure must still stamp completion so the cadence holds")
	}

	due, err := store.DueIn(ctx, "flaky", 24*time.Hour)
	if err != nil {
		t.Fatalf("DueIn: %v", err)
	}
	if due <= 0 {
		t.Fatal("a failed cycle must not leave the loop permanently overdue")
	}
}

// TestGormRunStore_InterruptedReleasesWithoutCompleting proves the deploy-during
// -cycle path at the SQL level.
func TestGormRunStore_InterruptedReleasesWithoutCompleting(t *testing.T) {
	db, store := setupRunStore(t)
	ctx := context.Background()

	token, _, err := store.Claim(ctx, "interrupted", 24*time.Hour, time.Hour, false)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if err := store.Complete(ctx, "interrupted", token, CycleOutcome{Interrupted: true}); err != nil {
		t.Fatalf("release: %v", err)
	}

	var got BackgroundServiceRun
	if err := db.Table("background_service_runs").Where("name = ?", "interrupted").Take(&got).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got.LastCompletedAt != nil {
		t.Fatal("an interrupted cycle must not stamp a completion")
	}
	if got.LastStartedAt != nil {
		t.Fatal("an interrupted cycle must release its claim")
	}
	if got.ConsecutiveFailures != 0 {
		t.Fatal("a shutdown is not a fault and must not count as a failure")
	}

	// Still overdue, and immediately claimable by the next boot.
	if _, ok, err := store.Claim(ctx, "interrupted", 24*time.Hour, time.Hour, false); err != nil || !ok {
		t.Fatalf("next boot must be able to pick the work straight back up: ok=%v err=%v", ok, err)
	}
}

// TestGormRunStore_LongErrorIsTruncated keeps a chatty provider from turning a
// status row into a blob.
func TestGormRunStore_LongErrorIsTruncated(t *testing.T) {
	db, store := setupRunStore(t)
	ctx := context.Background()

	token, _, err := store.Claim(ctx, "chatty", 24*time.Hour, time.Hour, false)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if err := store.Complete(ctx, "chatty", token, CycleOutcome{
		Err: errors.New(strings.Repeat("x", maxRunErrorLength*3)),
	}); err != nil {
		t.Fatalf("complete: %v", err)
	}

	var got BackgroundServiceRun
	if err := db.Table("background_service_runs").Where("name = ?", "chatty").Take(&got).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got.LastError == nil || len(*got.LastError) != maxRunErrorLength {
		t.Fatalf("error should be truncated to %d chars, got %v", maxRunErrorLength, got.LastError)
	}
}

// TestGormRunStore_DueSlackAbsorbsTimerJitter: a cycle arriving a hair before
// the interval elapses must still be allowed. Without slack, a millisecond of
// drift between the process timer and the database clock would push a daily
// sweep out by a whole day.
func TestGormRunStore_DueSlackAbsorbsTimerJitter(t *testing.T) {
	db, store := setupRunStore(t)
	ctx := context.Background()

	token, _, err := store.Claim(ctx, "jittery", time.Hour, time.Hour, false)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if err := store.Complete(ctx, "jittery", token, CycleOutcome{}); err != nil {
		t.Fatalf("complete: %v", err)
	}

	// 59m50s after completion: 10s short of the hour, inside the 1m slack.
	backdateCompletion(t, db, "jittery", time.Hour-10*time.Second)
	if _, ok, err := store.Claim(ctx, "jittery", time.Hour, time.Hour, false); err != nil || !ok {
		t.Fatalf("a slightly-early cycle must be allowed: ok=%v err=%v", ok, err)
	}
}

// TestGormRunStore_FutureCompletionIsClamped: a clock jump or a database restored
// from another environment must not park a loop for an unbounded time.
func TestGormRunStore_FutureCompletionIsClamped(t *testing.T) {
	db, store := setupRunStore(t)
	ctx := context.Background()

	if _, _, err := store.Claim(ctx, "time-traveller", time.Hour, time.Hour, false); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if err := db.Exec(`
		UPDATE background_service_runs
		SET last_completed_at = NOW() + interval '30 days'
		WHERE name = 'time-traveller'`).Error; err != nil {
		t.Fatalf("set future completion: %v", err)
	}

	due, err := store.DueIn(ctx, "time-traveller", time.Hour)
	if err != nil {
		t.Fatalf("DueIn: %v", err)
	}
	if due > time.Hour {
		t.Fatalf("a future completion must be clamped to one interval, got %v", due)
	}
}
