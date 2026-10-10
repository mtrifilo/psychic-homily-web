package shared

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// countClaimStatements counts other backends running the claim statement whose
// wait state matches lockWaitFilter. The driver gives up on a cancelled
// statement by dropping the connection, so the server can still be running it
// after the client has returned.
func countClaimStatements(t *testing.T, db *gorm.DB, lockWaitFilter string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Raw(`
		SELECT COUNT(*) FROM pg_stat_activity
		WHERE datname = current_database()
		  AND pid <> pg_backend_pid()
		  AND state = 'active'
		  AND query LIKE '%INSERT INTO background_service_runs AS b%'
		  AND `+lockWaitFilter).Scan(&n).Error)
	return n
}

func claimStatementsWaitingOnLock(t *testing.T, db *gorm.DB) int64 {
	return countClaimStatements(t, db, `wait_event_type = 'Lock'`)
}

func claimStatementsRunning(t *testing.T, db *gorm.DB) int64 {
	return countClaimStatements(t, db, `TRUE`)
}

// beforeClaimStore runs a hook as Claim begins, then claims through the wrapped
// store with the context Claim was given.
type beforeClaimStore struct {
	RunStore
	beforeClaim func()
}

func (s beforeClaimStore) Claim(ctx context.Context, name string, interval, lease time.Duration, force bool) (time.Time, bool, error) {
	s.beforeClaim()
	return s.RunStore.Claim(ctx, name, interval, lease, force)
}

// loopBlockedInClaim is a RunAtBoot loop whose claim statement is waiting on a
// row lock that the test holds in lockTx.
type loopBlockedInClaim struct {
	lockTx *gorm.DB
	cancel context.CancelFunc
	done   chan struct{}
	calls  atomic.Int32
}

// startLoopBlockedInClaim starts a loop named name and returns once its claim
// statement is waiting on the row lock. The lock is taken after the loop has
// registered, so only the claim statement waits on it. With cancelFirst the
// loop's context ends as Claim begins, before the statement is sent. Cleanup
// rolls the lock back and waits for the server to finish the claim statement,
// so none runs into the next test's table reset.
func startLoopBlockedInClaim(t *testing.T, db *gorm.DB, store RunStore, name string, cancelFirst bool) *loopBlockedInClaim {
	t.Helper()
	l := &loopBlockedInClaim{done: make(chan struct{})}

	l.lockTx = db.Begin()
	require.NoError(t, l.lockTx.Error)
	t.Cleanup(func() {
		l.lockTx.Rollback()
		assert.Eventually(t, func() bool { return claimStatementsRunning(t, db) == 0 },
			10*time.Second, 5*time.Millisecond, "the server must finish the claim statement")
	})
	var ctx context.Context
	ctx, l.cancel = context.WithCancel(context.Background())
	t.Cleanup(l.cancel)

	lockRow := func() {
		assert.NoError(t, l.lockTx.Exec(
			`SELECT 1 FROM background_service_runs WHERE name = ? FOR UPDATE`, name).Error)
		if cancelFirst {
			l.cancel()
		}
	}

	go func() {
		defer close(l.done)
		RunScheduledLoop(ctx, LoopConfig{
			Name:      name,
			Interval:  time.Hour,
			RunAtBoot: true,
			Store:     beforeClaimStore{RunStore: store, beforeClaim: lockRow},
		}, func(context.Context) { l.calls.Add(1) })
	}()

	require.Eventually(t, func() bool { return claimStatementsWaitingOnLock(t, db) == 1 },
		10*time.Second, 5*time.Millisecond, "the claim statement must be waiting on the row lock")
	return l
}

// waitReturned fails the test unless the loop returns within d.
func (l *loopBlockedInClaim) waitReturned(t *testing.T, d time.Duration, msg string) {
	t.Helper()
	select {
	case <-l.done:
	case <-time.After(d):
		t.Fatal(msg)
	}
}

// TestCancelDuringClaimLeavesNoClaim_GormStore ends the loop's context while
// its claim statement is in flight on the server, then lets the statement
// finish. Whatever the statement's fate, no claim row may remain once the loop
// has returned and the server is done with it.
func TestCancelDuringClaimLeavesNoClaim_GormStore(t *testing.T) {
	logs := withCapturedSlog(t)
	db, store := setupRunStore(t)
	t.Cleanup(resetRegisteredLoops)

	const name = "cancel-mid-claim-pg"
	l := startLoopBlockedInClaim(t, db, store, name, false)
	l.cancel()

	// The lock stays held long enough for a claim bound to the loop's context to
	// give up and return; only then is the statement let through, which is the
	// order in which the server commits a claim the client abandoned.
	select {
	case <-l.done:
	case <-time.After(200 * time.Millisecond):
	}
	require.NoError(t, l.lockTx.Rollback().Error)

	l.waitReturned(t, 10*time.Second, "the loop did not return after its context ended")
	require.Eventually(t, func() bool { return claimStatementsRunning(t, db) == 0 },
		10*time.Second, 5*time.Millisecond, "the server must finish the claim statement")

	var run BackgroundServiceRun
	require.NoError(t, db.Where("name = ?", name).Take(&run).Error)
	assert.Zero(t, l.calls.Load(), "a loop stopping during its claim must not run the work")
	assert.Nil(t, run.LastStartedAt, "no claim may outlive a loop that stopped during its claim")
	if assert.NotNil(t, run.LastOutcome) {
		assert.NotEqual(t, RunOutcomeRunning, *run.LastOutcome)
	}
	assert.NotContains(t, logs.String(), "background run state:",
		"releasing the claim must not report a store failure")
	assert.Contains(t, logs.String(), `"claim_released":true`)
}

// TestClaimStuckOnDatabaseBoundsStop_GormStore: a claim that the database never
// answers cannot hold a stopping loop much past claimTimeout, and the loop still
// does not run the work. The context ends before the claim statement is sent,
// so the claim's timeout always lands on a stopping loop.
func TestClaimStuckOnDatabaseBoundsStop_GormStore(t *testing.T) {
	logs := withCapturedSlog(t)
	db, store := setupRunStore(t)
	t.Cleanup(resetRegisteredLoops)

	oldTimeout := claimTimeout
	claimTimeout = 300 * time.Millisecond
	t.Cleanup(func() { claimTimeout = oldTimeout })

	l := startLoopBlockedInClaim(t, db, store, "stuck-claim-pg", true)
	stoppedAt := time.Now()

	l.waitReturned(t, claimTimeout+3*time.Second,
		"a claim the database never answers must not hold the loop past claimTimeout")
	assert.Less(t, time.Since(stoppedAt), claimTimeout+2*time.Second)
	assert.Zero(t, l.calls.Load(), "a loop stopping during its claim must not run the work")
	assert.Contains(t, logs.String(), "loop stopping during claim")
	assert.Contains(t, logs.String(), `"claim_released":false`, "a claim that timed out has nothing to release")
	assert.NotContains(t, logs.String(), "background run state:",
		"a loop that stops during its claim must not report a store failure on the way out")
}

// completeFailsStore fails every Complete.
type completeFailsStore struct{ RunStore }

func (completeFailsStore) Complete(context.Context, string, time.Time, CycleOutcome) error {
	return errors.New("complete unavailable")
}

// TestFailedReleaseIsNotReportedAsReleased: when the context ends during a
// granted claim and the release then fails, the stop line says the claim was
// not released.
func TestFailedReleaseIsNotReportedAsReleased(t *testing.T) {
	logs := withCapturedSlog(t)
	t.Cleanup(resetRegisteredLoops)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	var calls atomic.Int32
	RunScheduledLoop(ctx, LoopConfig{
		Name:      "release-fails",
		Interval:  time.Hour,
		RunAtBoot: true,
		Store:     beforeClaimStore{RunStore: completeFailsStore{newMemRunStore()}, beforeClaim: cancel},
	}, func(context.Context) { calls.Add(1) })

	assert.Zero(t, calls.Load())
	assert.Contains(t, logs.String(), "background run state: complete failed")
	assert.Contains(t, logs.String(), `"claim_released":false`)
	assert.NotContains(t, logs.String(), `"claim_released":true`)
}

// TestStopDuringClaimWithoutGrant: a loop that stops while a claim is refused
// or failing starts no cycle, attempts no release, and names the claim's own
// error when there is one, whether the stop came through the context or StopCh.
func TestStopDuringClaimWithoutGrant(t *testing.T) {
	t.Cleanup(resetRegisteredLoops)
	storeFailure := errors.New("distinct store failure")

	cases := []struct {
		name       string
		viaStopCh  bool
		failClaim  error
		wantReason string
	}{
		{"refused, context ends", false, nil, "context canceled"},
		{"refused, StopCh closes", true, nil, "loop stop requested"},
		{"store error, context ends", false, storeFailure, storeFailure.Error()},
		{"store error, StopCh closes", true, storeFailure, storeFailure.Error()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logs := withCapturedSlog(t)
			const name = "stop-without-grant"
			store := newMemRunStore()
			store.failClaim = tc.failClaim
			// Another instance holds a live claim, so even a forced boot claim
			// is refused.
			store.rows[name] = &memRunRow{outcome: RunOutcomeRunning, startedAt: time.Now()}

			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			stopCh := make(chan struct{})
			stop := cancel
			if tc.viaStopCh {
				stop = func() { close(stopCh) }
			}

			var calls atomic.Int32
			RunScheduledLoop(ctx, LoopConfig{
				Name:      name,
				Interval:  time.Hour,
				RunAtBoot: true,
				StopCh:    stopCh,
				Store:     beforeClaimStore{RunStore: store, beforeClaim: stop},
			}, func(context.Context) { calls.Add(1) })

			assert.Zero(t, calls.Load(), "a loop stopping during its claim must not run the work")
			assert.Zero(t, store.released.Load(), "there is no granted claim to release")
			var stopLine string
			for _, line := range strings.Split(logs.String(), "\n") {
				if strings.Contains(line, "loop stopping during claim") {
					stopLine = line
				}
			}
			require.NotEmpty(t, stopLine)
			assert.Contains(t, stopLine, `"claim_released":false`)
			assert.Contains(t, stopLine, tc.wantReason)
			assert.NotContains(t, logs.String(), "background run state:",
				"a stop during the claim is not reported as a store failure")
			assert.NotContains(t, logs.String(), "cycle skipped")
		})
	}
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
// with its context live, at each place a cycle starts.
func TestClosedStopChStartsNoCycle_MemStore(t *testing.T) {
	_ = withCapturedSlog(t)
	t.Cleanup(resetRegisteredLoops)

	closedStopCh := func() chan struct{} {
		stopCh := make(chan struct{})
		close(stopCh)
		return stopCh
	}

	t.Run("wait between cycles", func(t *testing.T) {
		compressCatchUp(t, 0, 0)
		stopCh := closedStopCh()

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
	})

	t.Run("boot cycle", func(t *testing.T) {
		store := newMemRunStore()
		var calls atomic.Int32
		RunScheduledLoop(context.Background(), LoopConfig{
			Name:      "stopped-boot",
			Interval:  time.Hour,
			RunAtBoot: true,
			StopCh:    closedStopCh(),
			Store:     store,
		}, func(context.Context) { calls.Add(1) })

		assert.Zero(t, calls.Load(), "RunAtBoot must not start a cycle after StopCh closed")
		assert.Zero(t, store.claimed.Load(), "no claim may be taken after StopCh closed")
	})

	t.Run("StopCh closes during the claim", func(t *testing.T) {
		logs := withCapturedSlog(t)
		store := newMemRunStore()
		stopCh := make(chan struct{})
		var calls atomic.Int32
		RunScheduledLoop(context.Background(), LoopConfig{
			Name:      "stopped-mid-claim",
			Interval:  time.Hour,
			RunAtBoot: true,
			StopCh:    stopCh,
			Store:     beforeClaimStore{RunStore: store, beforeClaim: func() { close(stopCh) }},
		}, func(context.Context) { calls.Add(1) })

		assert.Zero(t, calls.Load(), "a claim granted after StopCh closed must not start the cycle")
		assert.Equal(t, int32(1), store.claimed.Load())
		assert.Equal(t, int32(1), store.released.Load(), "the claim granted after the stop must be released")
		assert.Contains(t, logs.String(), `"claim_released":true`)
		assert.Contains(t, logs.String(), "loop stop requested")
	})
}

// TestLiveSlowClaimWaits_GormStore: a claim that takes longer than claimTimeout
// while the loop's context is live keeps waiting and is granted, so the work
// runs claimed and its completion is recorded. It never fails open.
func TestLiveSlowClaimWaits_GormStore(t *testing.T) {
	logs := withCapturedSlog(t)
	db, store := setupRunStore(t)
	t.Cleanup(resetRegisteredLoops)

	oldTimeout := claimTimeout
	claimTimeout = 200 * time.Millisecond
	t.Cleanup(func() { claimTimeout = oldTimeout })

	const name = "live-slow-claim-pg"
	l := startLoopBlockedInClaim(t, db, store, name, false)
	time.Sleep(claimTimeout + 300*time.Millisecond)
	require.NoError(t, l.lockTx.Rollback().Error)

	require.Eventually(t, func() bool { return l.calls.Load() == 1 },
		10*time.Second, 5*time.Millisecond, "the slow claim must be granted and the cycle run")
	l.cancel()
	l.waitReturned(t, 10*time.Second, "the loop did not return after its context ended")

	var run BackgroundServiceRun
	require.NoError(t, db.Where("name = ?", name).Take(&run).Error)
	assert.NotNil(t, run.LastCompletedAt, "the cycle ran under its claim and completed")
	if assert.NotNil(t, run.LastOutcome) {
		assert.Equal(t, RunOutcomeSuccess, *run.LastOutcome)
	}
	assert.NotContains(t, logs.String(), "background run state: claim failed",
		"a slow claim on a live loop must not fail open")
}
