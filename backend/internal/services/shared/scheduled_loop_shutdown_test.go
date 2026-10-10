package shared

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The tests in this file pin how a loop behaves when its context is done at the
// moment a cycle would start. A select over two ready channels picks one at
// random, so each "both ready" case is repeated tieTrials times: a loop that let
// the timer win even half the time fails with probability 1 - 2^-tieTrials.
const tieTrials = 64

// expiredContext returns a context whose deadline has already passed, the state
// a process's context is in when shutdown lands on the instant a cycle is due.
// runFor with a zero lifetime runs a loop on the same kind of context.
func expiredContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithDeadline(context.Background(), time.Now())
	t.Cleanup(cancel)
	return ctx
}

// TestSleepOrStop_ElapsedTimerLosesToDoneContext: a zero wait on an expired
// context makes the timer and the done channel ready together, and the answer
// must be "stop" every time.
func TestSleepOrStop_ElapsedTimerLosesToDoneContext(t *testing.T) {
	ctx := expiredContext(t)
	for i := 0; i < tieTrials; i++ {
		require.False(t, sleepOrStop(ctx, nil, 0),
			"trial %d: an elapsed timer must never start a cycle on a done context", i)
	}
}

// TestExpiredContextStartsNoCycle_MemStore covers both places a cycle starts,
// the wait between cycles and the boot cycle, against a store that does not
// look at the context. Here the claim would succeed, so only the loop itself can
// keep the work from running.
func TestExpiredContextStartsNoCycle_MemStore(t *testing.T) {
	_ = withCapturedSlog(t)
	t.Cleanup(resetRegisteredLoops)

	t.Run("wait between cycles", func(t *testing.T) {
		// A zero catch-up delay on an overdue loop puts the first wait's timer
		// and the expired context in the same select.
		compressCatchUp(t, 0, 0)
		store := newMemRunStore()
		store.seedCompleted("expired-wait", 2*time.Hour)

		var calls atomic.Int32
		for i := 0; i < tieTrials; i++ {
			runFor(LoopConfig{
				Name:     "expired-wait",
				Interval: time.Hour,
				Store:    store,
			}, 0, func(context.Context) { calls.Add(1) })
		}

		assert.Zero(t, calls.Load(), "no cycle may start on an expired context")
		assert.Zero(t, store.claimed.Load(), "no claim may be taken on an expired context")
	})

	t.Run("boot cycle", func(t *testing.T) {
		store := newMemRunStore()

		var calls atomic.Int32
		runFor(LoopConfig{
			Name:      "expired-boot",
			Interval:  time.Hour,
			RunAtBoot: true,
			Store:     store,
		}, 0, func(context.Context) { calls.Add(1) })

		assert.Zero(t, calls.Load(), "RunAtBoot must not start a cycle on an expired context")
		assert.Zero(t, store.claimed.Load(), "no claim may be taken on an expired context")
	})

	t.Run("boot cycle without a store", func(t *testing.T) {
		// Below the persistence threshold the loop has no store, so nothing but
		// the loop's own check stands between an expired context and the work.
		var calls atomic.Int32
		runFor(LoopConfig{
			Name:      "expired-boot-storeless",
			Interval:  time.Minute,
			RunAtBoot: true,
		}, 0, func(context.Context) { calls.Add(1) })

		assert.Zero(t, calls.Load(), "a storeless RunAtBoot loop must not start a cycle on an expired context")
	})

	t.Run("context error from the store on a live context still fails open", func(t *testing.T) {
		// The exclusion keys on the loop's own context, not on the error's type:
		// a store that times out internally while the loop is live is an outage,
		// and an outage runs the work.
		store := newMemRunStore()
		store.failClaim = context.DeadlineExceeded

		var calls atomic.Int32
		runFor(LoopConfig{
			Name:      "live-context-claim-timeout",
			Interval:  time.Hour,
			RunAtBoot: true,
			Store:     store,
		}, 100*time.Millisecond, func(context.Context) { calls.Add(1) })

		assert.Equal(t, int32(1), calls.Load(),
			"a claim failing with a context error while the loop is live must still fail open")
	})
}

// claimCancelsStore cancels the loop's context as Claim begins and then claims
// through the wrapped store with the context Claim was given.
type claimCancelsStore struct {
	RunStore
	cancel context.CancelFunc
}

func (s claimCancelsStore) Claim(ctx context.Context, name string, interval, lease time.Duration, force bool) (time.Time, bool, error) {
	s.cancel()
	return s.RunStore.Claim(ctx, name, interval, lease, force)
}

// TestExpiredContextRunsNothingUnclaimed_GormStore is the Postgres half: a claim
// on a done context fails, and that failure must never take the fail-open path
// that runs the work without a claim.
//
// "context ends during the claim" is the only subtest that reaches Claim. The
// claim runs detached from the loop's context, so it succeeds there, and the
// subtest pins runCycle releasing a claim granted after the context ended. The
// other two pin the outcome end to end: on an expired context the loop stops
// before Claim.
func TestExpiredContextRunsNothingUnclaimed_GormStore(t *testing.T) {
	_ = withCapturedSlog(t)
	db, store := setupRunStore(t)
	t.Cleanup(resetRegisteredLoops)

	claimRows := func(t *testing.T, name string) int64 {
		t.Helper()
		var n int64
		require.NoError(t, db.Raw(
			`SELECT COUNT(*) FROM background_service_runs WHERE name = ? AND last_started_at IS NOT NULL`,
			name).Scan(&n).Error)
		return n
	}

	// The premise the loop's guard exists for: the real claim statement fails on
	// a done context instead of claiming.
	_, claimed, err := store.Claim(expiredContext(t), "expired-premise", time.Hour, time.Hour, false)
	require.ErrorIs(t, err, context.DeadlineExceeded,
		"premise: a Postgres claim on an expired context must fail with the context's error")
	require.False(t, claimed)

	t.Run("wait between cycles", func(t *testing.T) {
		// Register and DueIn fail on the expired context too, so the loop falls
		// back to StartDelay. A nanosecond timer has normally fired by the
		// select, which makes this a tie most trials.
		var calls atomic.Int32
		for i := 0; i < tieTrials; i++ {
			runFor(LoopConfig{
				Name:       "expired-wait-pg",
				Interval:   time.Hour,
				StartDelay: time.Nanosecond,
				Store:      store,
			}, 0, func(context.Context) { calls.Add(1) })
		}

		assert.Zero(t, calls.Load(), "an expired context must not run the work unclaimed")
		assert.Zero(t, claimRows(t, "expired-wait-pg"), "an expired context must not leave a claim")
	})

	t.Run("boot cycle", func(t *testing.T) {
		var calls atomic.Int32
		runFor(LoopConfig{
			Name:      "expired-boot-pg",
			Interval:  time.Hour,
			RunAtBoot: true,
			Store:     store,
		}, 0, func(context.Context) { calls.Add(1) })

		assert.Zero(t, calls.Load(), "an expired context must not run the boot cycle unclaimed")
		assert.Zero(t, claimRows(t, "expired-boot-pg"), "an expired context must not leave a claim")
	})

	t.Run("context ends during the claim", func(t *testing.T) {
		// The context is live when the cycle starts and ends inside Claim, so the
		// check before the cycle passes and only runCycle's handling of the
		// claim is left to stop the work.
		logs := withCapturedSlog(t)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		var calls atomic.Int32
		RunScheduledLoop(ctx, LoopConfig{
			Name:      "expired-mid-claim-pg",
			Interval:  time.Hour,
			RunAtBoot: true,
			Store:     claimCancelsStore{RunStore: store, cancel: cancel},
		}, func(context.Context) { calls.Add(1) })

		assert.Zero(t, calls.Load(), "a context that ends during the claim must not start the cycle")
		assert.Zero(t, claimRows(t, "expired-mid-claim-pg"), "a claim granted after the context ended must be released")
		// logStoreError prefixes every store failure it reports this way.
		assert.NotContains(t, logs.String(), "background run state:",
			"a loop that stops during its claim must not report a store failure on the way out")
		var stopLine string
		for _, line := range strings.Split(logs.String(), "\n") {
			if strings.Contains(line, "loop stopping during claim") {
				stopLine = line
			}
		}
		require.NotEmpty(t, stopLine, "a claim abandoned on shutdown must still be traceable in the logs")
		assert.Contains(t, stopLine, `"level":"INFO"`, "shutdown is not a store fault, so the line is Info")
		assert.Contains(t, stopLine, `"service":"expired-mid-claim-pg"`)
		assert.Contains(t, stopLine, "context canceled", "the line must carry the claim's error")
	})
}
