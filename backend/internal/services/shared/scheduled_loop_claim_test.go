package shared

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// claimStatementsOnServer counts other backends running the claim statement,
// optionally only those waiting on a lock. The driver gives up on a cancelled
// statement by dropping the connection, so the server can still be running it
// after the client has returned.
func claimStatementsOnServer(t *testing.T, db *gorm.DB, waitingOnLock bool) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Raw(`
		SELECT COUNT(*) FROM pg_stat_activity
		WHERE datname = current_database()
		  AND pid <> pg_backend_pid()
		  AND state = 'active'
		  AND query LIKE '%INSERT INTO background_service_runs AS b%'
		  AND (NOT ?::boolean OR wait_event_type = 'Lock')
	`, waitingOnLock).Scan(&n).Error)
	return n
}

// beforeClaimStore runs a hook as Claim begins, then claims through the wrapped
// store with the context it was given.
type beforeClaimStore struct {
	RunStore
	beforeClaim func()
}

func (s beforeClaimStore) Claim(ctx context.Context, name string, interval, lease time.Duration, force bool) (time.Time, bool, error) {
	s.beforeClaim()
	return s.RunStore.Claim(ctx, name, interval, lease, force)
}

// TestCancelDuringClaimLeavesNoClaim_GormStore ends the loop's context while
// its claim statement is in flight on the server, then lets the statement
// finish. Another transaction holds the loop's row lock so the statement is
// still running when the context ends. Whatever the statement's fate, no claim
// row may remain once the loop has returned and the server is done with it.
func TestCancelDuringClaimLeavesNoClaim_GormStore(t *testing.T) {
	logs := withCapturedSlog(t)
	db, store := setupRunStore(t)
	t.Cleanup(resetRegisteredLoops)

	const name = "cancel-mid-claim-pg"

	// The row lock is taken once the loop has registered, so only the claim
	// statement waits on it.
	lockTx := db.Begin()
	require.NoError(t, lockTx.Error)
	t.Cleanup(func() { lockTx.Rollback() })
	lockRow := func() {
		assert.NoError(t, lockTx.Exec(
			`SELECT 1 FROM background_service_runs WHERE name = ? FOR UPDATE`, name).Error)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	var calls atomic.Int32
	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		RunScheduledLoop(ctx, LoopConfig{
			Name:      name,
			Interval:  time.Hour,
			RunAtBoot: true,
			Store:     beforeClaimStore{RunStore: store, beforeClaim: lockRow},
		}, func(context.Context) { calls.Add(1) })
	}()

	require.Eventually(t, func() bool { return claimStatementsOnServer(t, db, true) == 1 },
		10*time.Second, 5*time.Millisecond, "the claim statement must be waiting on the row lock")
	cancel()

	// A loop whose claim gave up with the context has returned by now; a loop
	// whose claim is still waiting is released by the lock going away.
	select {
	case <-loopDone:
	case <-time.After(200 * time.Millisecond):
	}
	require.NoError(t, lockTx.Rollback().Error)

	select {
	case <-loopDone:
	case <-time.After(10 * time.Second):
		t.Fatal("the loop did not return after its context ended")
	}
	require.Eventually(t, func() bool { return claimStatementsOnServer(t, db, false) == 0 },
		10*time.Second, 5*time.Millisecond, "the server must finish the claim statement")

	var run BackgroundServiceRun
	require.NoError(t, db.Where("name = ?", name).Take(&run).Error)
	assert.Zero(t, calls.Load(), "a loop stopping during its claim must not run the work")
	assert.Nil(t, run.LastStartedAt, "no claim may outlive a loop that stopped during its claim")
	if assert.NotNil(t, run.LastOutcome) {
		assert.NotEqual(t, RunOutcomeRunning, *run.LastOutcome)
	}
	assert.NotContains(t, logs.String(), "background run state:",
		"releasing the claim must not report a store failure")
}

// TestSleepOrStop_ElapsedTimerLosesToClosedStopCh: a zero wait with StopCh
// closed makes the timer and the stop channel ready together on a live context,
// and the answer must be "stop" every time.
func TestSleepOrStop_ElapsedTimerLosesToClosedStopCh(t *testing.T) {
	stopCh := make(chan struct{})
	close(stopCh)
	for i := 0; i < tieTrials; i++ {
		require.False(t, sleepOrStop(context.Background(), stopCh, 0),
			"trial %d: an elapsed timer must never start a cycle after StopCh closed", i)
	}
}

// TestClosedStopChStartsNoCycle_MemStore stops a loop through StopCh alone,
// with its context live, at the moment its first cycle is due.
func TestClosedStopChStartsNoCycle_MemStore(t *testing.T) {
	_ = withCapturedSlog(t)
	t.Cleanup(resetRegisteredLoops)
	compressCatchUp(t, 0, 0)

	stopCh := make(chan struct{})
	close(stopCh)

	// A fresh overdue store per trial: a trial that started a cycle would
	// otherwise leave the next one not due, and no tie.
	var calls, claims atomic.Int32
	for i := 0; i < tieTrials; i++ {
		store := newMemRunStore()
		store.seedCompleted("stopped-wait", 2*time.Hour)
		RunScheduledLoop(context.Background(), LoopConfig{
			Name:     "stopped-wait",
			Interval: time.Hour,
			StopCh:   stopCh,
			Store:    store,
		}, func(context.Context) { calls.Add(1) })
		claims.Add(store.claimed.Load())
	}

	assert.Zero(t, calls.Load(), "no cycle may start after StopCh closed")
	assert.Zero(t, claims.Load(), "no claim may be taken after StopCh closed")
}

// TestClaimStuckOnDatabaseBoundsStop_GormStore: a claim that the database never
// answers holds a stopping loop for claimTimeout and no longer, and the loop
// still does not run the work.
func TestClaimStuckOnDatabaseBoundsStop_GormStore(t *testing.T) {
	logs := withCapturedSlog(t)
	db, store := setupRunStore(t)
	t.Cleanup(resetRegisteredLoops)

	oldTimeout := claimTimeout
	claimTimeout = 300 * time.Millisecond
	t.Cleanup(func() { claimTimeout = oldTimeout })

	const name = "stuck-claim-pg"
	lockTx := db.Begin()
	require.NoError(t, lockTx.Error)
	t.Cleanup(func() {
		lockTx.Rollback()
		// Leave no claim statement running into the next test's table reset.
		assert.Eventually(t, func() bool { return claimStatementsOnServer(t, db, false) == 0 },
			10*time.Second, 5*time.Millisecond)
	})
	lockRow := func() {
		assert.NoError(t, lockTx.Exec(
			`SELECT 1 FROM background_service_runs WHERE name = ? FOR UPDATE`, name).Error)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	var calls atomic.Int32
	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		RunScheduledLoop(ctx, LoopConfig{
			Name:      name,
			Interval:  time.Hour,
			RunAtBoot: true,
			Store:     beforeClaimStore{RunStore: store, beforeClaim: lockRow},
		}, func(context.Context) { calls.Add(1) })
	}()

	require.Eventually(t, func() bool { return claimStatementsOnServer(t, db, true) == 1 },
		10*time.Second, 5*time.Millisecond, "the claim statement must be waiting on the row lock")
	cancel()
	stoppedAt := time.Now()

	select {
	case <-loopDone:
	case <-time.After(claimTimeout + 3*time.Second):
		t.Fatal("a claim the database never answers must not hold the loop past claimTimeout")
	}
	assert.Less(t, time.Since(stoppedAt), claimTimeout+2*time.Second)
	assert.Zero(t, calls.Load(), "a loop stopping during its claim must not run the work")
	assert.Contains(t, logs.String(), "loop stopping during claim")
	assert.NotContains(t, logs.String(), "background run state:",
		"a loop that stops during its claim must not report a store failure on the way out")
}
