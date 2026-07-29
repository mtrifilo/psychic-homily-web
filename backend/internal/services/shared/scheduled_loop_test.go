package shared

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withCapturedSlog swaps the slog default logger for a JSON handler that
// writes to a buffer for the duration of the test, restoring the original
// logger on cleanup. Use to assert that panic recoveries actually log.
func withCapturedSlog(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	original := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(original) })
	return buf
}

// TestScheduledLoop_PanicInWorkContinuesLoop is the canonical demonstration:
// a tick that panics is recovered, logged, and the loop fires the next tick.
// This is the load-bearing assertion of PSY-615.
func TestScheduledLoop_PanicInWorkContinuesLoop(t *testing.T) {
	logs := withCapturedSlog(t)

	var calls atomic.Int32
	work := func(_ context.Context) {
		n := calls.Add(1)
		if n == 1 {
			panic("boom on tick 1")
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		RunScheduledLoop(ctx, LoopConfig{
			Name:     "test-service",
			Interval: 10 * time.Millisecond,
		}, work)
		close(done)
	}()

	<-done

	got := calls.Load()
	require.Greater(t, int(got), 1, "loop should have ticked at least twice; got %d", got)

	logged := logs.String()
	assert.Contains(t, logged, "background service tick panic — continuing", "panic should be logged")
	assert.Contains(t, logged, `"service":"test-service"`, "service name should be in log")
	assert.Contains(t, logged, "boom on tick 1", "panic value should be in log")
	assert.Contains(t, logged, `"stack"`, "stack trace should be in log")
}

// TestScheduledLoop_NormalWorkRunsRepeatedly sanity-checks the helper
// on the happy path: a non-panicking work function runs once per tick.
func TestScheduledLoop_NormalWorkRunsRepeatedly(t *testing.T) {
	var calls atomic.Int32
	work := func(_ context.Context) {
		calls.Add(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		RunScheduledLoop(ctx, LoopConfig{
			Name:     "happy-service",
			Interval: 10 * time.Millisecond,
		}, work)
		close(done)
	}()

	<-done

	got := calls.Load()
	assert.GreaterOrEqual(t, int(got), 3, "expected several ticks in 100ms with 10ms interval; got %d", got)
}

// TestScheduledLoop_RunImmediately fires the work function once before
// entering the scheduled loop. Used by services that want a boot cycle.
func TestScheduledLoop_RunImmediately(t *testing.T) {
	var calls atomic.Int32
	work := func(_ context.Context) {
		calls.Add(1)
	}

	// Long interval — only the immediate startup call should land before
	// ctx times out.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		RunScheduledLoop(ctx, LoopConfig{
			Name:      "startup-service",
			Interval:  1 * time.Hour,
			RunAtBoot: true,
		}, work)
		close(done)
	}()

	<-done
	assert.Equal(t, int32(1), calls.Load(), "exactly one cycle should have run (the startup cycle)")
}

// TestScheduledLoop_StartupPanicDoesNotKillLoop covers the case where
// the startup cycle panics — the loop must still fire a regular tick after.
func TestScheduledLoop_StartupPanicDoesNotKillLoop(t *testing.T) {
	logs := withCapturedSlog(t)

	var calls atomic.Int32
	work := func(_ context.Context) {
		n := calls.Add(1)
		if n == 1 {
			panic("boom on startup")
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		RunScheduledLoop(ctx, LoopConfig{
			Name:      "startup-panic-service",
			Interval:  10 * time.Millisecond,
			RunAtBoot: true,
		}, work)
		close(done)
	}()

	<-done

	got := calls.Load()
	require.Greater(t, int(got), 1, "loop should keep running after startup panic; got %d calls", got)
	assert.Contains(t, logs.String(), "boom on startup")
}

// TestScheduledLoop_StopChannel covers the explicit `close(stopCh)`
// shutdown path. Each existing service uses both `<-ctx.Done()` and
// `<-stopCh`; the helper must honor stopCh too.
func TestScheduledLoop_StopChannel(t *testing.T) {
	var calls atomic.Int32
	work := func(_ context.Context) {
		calls.Add(1)
	}

	stopCh := make(chan struct{})
	ctx := context.Background()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		RunScheduledLoop(ctx, LoopConfig{
			Name:     "stop-ch-service",
			Interval: 10 * time.Millisecond,
			StopCh:   stopCh,
		}, work)
	}()

	// Let it tick a few times, then close stopCh.
	time.Sleep(50 * time.Millisecond)
	close(stopCh)

	// wg.Wait should return once the loop sees the closed channel.
	waitDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(waitDone)
	}()

	select {
	case <-waitDone:
		// expected
	case <-time.After(200 * time.Millisecond):
		t.Fatal("RunScheduledLoop did not return after stopCh was closed")
	}
}

// TestScheduledLoop_ContextCancellationStopsLoop covers the `<-ctx.Done()`
// path independently from stopCh.
func TestScheduledLoop_ContextCancellationStopsLoop(t *testing.T) {
	var calls atomic.Int32
	work := func(_ context.Context) {
		calls.Add(1)
	}

	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		RunScheduledLoop(ctx, LoopConfig{
			Name:     "ctx-cancel-service",
			Interval: 10 * time.Millisecond,
		}, work)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	waitDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(waitDone)
	}()

	select {
	case <-waitDone:
		// expected
	case <-time.After(200 * time.Millisecond):
		t.Fatal("RunScheduledLoop did not return after context cancel")
	}
}

// capturedPanic records the args invokePanicHandler passes through so
// PSY-617's Sentry-wiring tests can assert against them without depending
// on the live Sentry SDK or a transport mock.
type capturedPanic struct {
	service    string
	panicValue any
	stack      []byte
}

// installRecordingPanicHandler installs a thread-safe recorder for the
// duration of the test, restoring the previous (nil) handler on cleanup.
// Returns a function that snapshots the captured calls.
func installRecordingPanicHandler(t *testing.T) func() []capturedPanic {
	t.Helper()
	var (
		mu       sync.Mutex
		captured []capturedPanic
	)
	SetPanicHandler(func(service string, panicValue any, stack []byte) {
		mu.Lock()
		defer mu.Unlock()
		captured = append(captured, capturedPanic{service: service, panicValue: panicValue, stack: stack})
	})
	t.Cleanup(func() { SetPanicHandler(nil) })
	return func() []capturedPanic {
		mu.Lock()
		defer mu.Unlock()
		out := make([]capturedPanic, len(captured))
		copy(out, captured)
		return out
	}
}

// TestScheduledLoop_PanicHandlerInvokedOnTickPanic is the load-bearing
// PSY-617 assertion: a panic inside per-tick work invokes the registered
// PanicHandler with the service name, panic value, and stack trace —
// in addition to the slog.Error already exercised by the PSY-615 test.
// The handler is the wiring point cmd/server/main.go uses to escalate
// to Sentry.
func TestScheduledLoop_PanicHandlerInvokedOnTickPanic(t *testing.T) {
	_ = withCapturedSlog(t) // suppress noisy stack trace from slog.Error
	snapshot := installRecordingPanicHandler(t)

	var calls atomic.Int32
	work := func(_ context.Context) {
		if calls.Add(1) == 1 {
			panic("boom-tick")
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		RunScheduledLoop(ctx, LoopConfig{
			Name:     "panic-handler-test-service",
			Interval: 10 * time.Millisecond,
		}, work)
		close(done)
	}()
	<-done

	got := snapshot()
	require.Len(t, got, 1, "panic handler should be invoked exactly once for the single tick panic")
	assert.Equal(t, "panic-handler-test-service", got[0].service)
	assert.Equal(t, "boom-tick", got[0].panicValue)
	assert.NotEmpty(t, got[0].stack, "stack trace should be populated")
	assert.Contains(t, string(got[0].stack), "scheduled_loop.go", "stack should reference the recover site")
}

// TestScheduledLoop_NonPositiveIntervalDegradesInsteadOfDying covers a
// misconfigured interval. The old loop handed 0 to time.NewTicker, which
// panicked and stopped the service permanently — a config typo silently
// disabled a background job. The scheduler now clamps to a sane default, says
// so, and keeps running.
func TestScheduledLoop_NonPositiveIntervalDegradesInsteadOfDying(t *testing.T) {
	logs := withCapturedSlog(t)
	snapshot := installRecordingPanicHandler(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("a non-positive interval must not panic; got: %v", r)
		}
	}()

	var calls atomic.Int32
	RunScheduledLoop(ctx, LoopConfig{
		Name:     "zero-interval-service",
		Interval: 0,
	}, func(_ context.Context) { calls.Add(1) })

	assert.Empty(t, snapshot(), "a config error is not a panic and must not reach the panic handler")
	assert.Contains(t, logs.String(), "non-positive interval", "the clamp must be reported, not silent")
	assert.Equal(t, int32(0), calls.Load(),
		"the clamped 1h interval means no cycle inside the test window, but the loop is alive")
}

// TestScheduledLoop_NilPanicHandlerIsNoop guards the contract that an
// unset handler (the package default) doesn't panic and doesn't change
// the existing slog-only behaviour. Defensive clear in case a sibling
// test in the package leaked a handler past its t.Cleanup.
func TestScheduledLoop_NilPanicHandlerIsNoop(t *testing.T) {
	logs := withCapturedSlog(t)
	SetPanicHandler(nil)
	t.Cleanup(func() { SetPanicHandler(nil) })

	var calls atomic.Int32
	work := func(_ context.Context) {
		if calls.Add(1) == 1 {
			panic("boom-no-handler")
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		RunScheduledLoop(ctx, LoopConfig{
			Name:     "no-handler-service",
			Interval: 10 * time.Millisecond,
		}, work)
		close(done)
	}()
	<-done

	assert.Greater(t, int(calls.Load()), 1, "loop should still continue past the panic")
	assert.Contains(t, logs.String(), "boom-no-handler", "slog path still fires when handler is nil")
}

// TestScheduledLoop_PanicHandlerOwnPanicDoesNotKillLoop guards against a
// buggy handler taking down the loop it was meant to observe. If the
// handler panics, the recover inside invokePanicHandler must swallow it
// and the loop must continue ticking.
func TestScheduledLoop_PanicHandlerOwnPanicDoesNotKillLoop(t *testing.T) {
	_ = withCapturedSlog(t)

	var handlerCalls atomic.Int32
	SetPanicHandler(func(_ string, _ any, _ []byte) {
		handlerCalls.Add(1)
		panic("handler is buggy")
	})
	t.Cleanup(func() { SetPanicHandler(nil) })

	var calls atomic.Int32
	work := func(_ context.Context) {
		if calls.Add(1) == 1 {
			panic("trigger-handler")
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		RunScheduledLoop(ctx, LoopConfig{
			Name:     "buggy-handler-service",
			Interval: 10 * time.Millisecond,
		}, work)
		close(done)
	}()
	<-done

	require.Greater(t, int(handlerCalls.Load()), 0, "handler should have been called at least once")
	assert.Greater(t, int(calls.Load()), 1, "loop should keep ticking after handler panic")
}

// TestScheduledLoop_OuterRecoverCatchesSchedulingPanic covers the outer
// recover: a panic raised by the scheduling machinery itself (here, a store
// that misbehaves) must be logged and contained rather than escaping into the
// supervising goroutine and taking the process down.
func TestScheduledLoop_OuterRecoverCatchesSchedulingPanic(t *testing.T) {
	logs := withCapturedSlog(t)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Recover here too — if the helper's recover failed, the goroutine
	// would crash and we want the test to fail with a clear message
	// rather than a process-level panic.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("outer recover failed; panic escaped: %v", r)
		}
	}()

	RunScheduledLoop(ctx, LoopConfig{
		Name:     "scheduling-panic-service",
		Interval: time.Hour,
		Store:    panickingRunStore{},
	}, func(_ context.Context) {})

	logged := logs.String()
	require.True(t,
		strings.Contains(logged, "background service panic — service stopping"),
		"outer recover should have logged the scheduling panic; got: %s", logged,
	)
	assert.Contains(t, logged, `"service":"scheduling-panic-service"`)
}

// panickingRunStore models the worst case for the scheduler's own code path.
type panickingRunStore struct{}

func (panickingRunStore) Register(context.Context, string, time.Duration) error {
	panic("store exploded")
}

func (panickingRunStore) DueIn(context.Context, string, time.Duration) (time.Duration, error) {
	panic("store exploded")
}

func (panickingRunStore) Claim(context.Context, string, time.Duration, time.Duration, bool) (time.Time, bool, error) {
	panic("store exploded")
}

func (panickingRunStore) Complete(context.Context, string, time.Time, CycleOutcome) error {
	panic("store exploded")
}

// =============================================================================
// Start delay — the first cycle of a loop with no boot cycle and no persisted
// state must be anchored to a bounded delay, NEVER to the interval. A
// long-interval service otherwise makes no progress at all on a platform that
// restarts the process more often than the interval.
// =============================================================================

// TestStartDelay_FirstCycleBeatsInterval is the regression test for the whole
// bug class: with a 1h interval, an interval-anchored first cycle never runs in
// a short-lived process, which is exactly why the production sweeps wrote
// nothing. The bounded delay must still produce a cycle.
func TestStartDelay_FirstCycleBeatsInterval(t *testing.T) {
	var calls atomic.Int32
	work := func(_ context.Context) { calls.Add(1) }

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		RunScheduledLoop(ctx, LoopConfig{
			Name:       "delayed-service",
			Interval:   time.Hour,
			StartDelay: 20 * time.Millisecond,
		}, work)
		close(done)
	}()

	<-done
	assert.Equal(t, int32(1), calls.Load(),
		"the start delay must produce exactly one cycle even though the interval never elapses")
}

// TestStartDelay_NoCycleOnBootPath guards the property the original design was
// protecting: the first cycle must NOT run on the boot path. A process that
// dies inside the delay window makes zero third-party calls — which is what
// keeps a burst of rapid redeploys free of traffic.
func TestStartDelay_NoCycleOnBootPath(t *testing.T) {
	var calls atomic.Int32
	work := func(_ context.Context) { calls.Add(1) }

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		RunScheduledLoop(ctx, LoopConfig{
			Name:       "boot-path-service",
			Interval:   time.Hour,
			StartDelay: 5 * time.Second,
		}, work)
		close(done)
	}()

	<-done
	assert.Equal(t, int32(0), calls.Load(), "no cycle may run before the start delay elapses")
}

// TestStartDelay_ShutdownDuringDelayIsPrompt asserts the delay is a cancellable
// timer, not a sleep: a stop signal during the delay window returns immediately
// rather than riding out the full delay.
func TestStartDelay_ShutdownDuringDelayIsPrompt(t *testing.T) {
	stopCh := make(chan struct{})
	var calls atomic.Int32
	work := func(_ context.Context) { calls.Add(1) }

	done := make(chan struct{})
	go func() {
		RunScheduledLoop(context.Background(), LoopConfig{
			Name:       "stop-service",
			Interval:   time.Hour,
			StopCh:     stopCh,
			StartDelay: 10 * time.Second,
		}, work)
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	close(stopCh)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stop during the start-delay window must return promptly, not wait out the delay")
	}
	assert.Equal(t, int32(0), calls.Load(), "no cycle should have run")
}

// TestStartDelay_TicksAfterFirstCycle confirms the loop keeps its periodic
// behaviour once the delayed first cycle has run.
func TestStartDelay_TicksAfterFirstCycle(t *testing.T) {
	var calls atomic.Int32
	work := func(_ context.Context) { calls.Add(1) }

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		RunScheduledLoop(ctx, LoopConfig{
			Name:       "periodic-service",
			Interval:   20 * time.Millisecond,
			StartDelay: 10 * time.Millisecond,
		}, work)
		close(done)
	}()

	<-done
	assert.GreaterOrEqual(t, int(calls.Load()), 3,
		"the delayed first cycle must be followed by regular interval cycles; got %d", calls.Load())
}

// TestStartDelay_CappedAtInterval keeps the delay from REGRESSING a short loop:
// a 30s worker must not be pushed out to the 15m default. The cap is what makes
// one scheduling mechanism safe for both a 30s queue drain and a daily sweep.
func TestStartDelay_CappedAtInterval(t *testing.T) {
	var calls atomic.Int32
	work := func(_ context.Context) { calls.Add(1) }

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		RunScheduledLoop(ctx, LoopConfig{
			Name:       "short-interval-service",
			Interval:   10 * time.Millisecond,
			StartDelay: time.Hour, // absurd on purpose; must be clamped to Interval
		}, work)
		close(done)
	}()

	<-done
	assert.GreaterOrEqual(t, int(calls.Load()), 3,
		"a start delay longer than the interval must be capped at the interval; got %d", calls.Load())
}

// TestStartDelay_PanicInFirstCycleDoesNotKillLoop mirrors the boot-cycle panic
// guarantee for the delayed first cycle.
func TestStartDelay_PanicInFirstCycleDoesNotKillLoop(t *testing.T) {
	logs := withCapturedSlog(t)

	var calls atomic.Int32
	work := func(_ context.Context) {
		if calls.Add(1) == 1 {
			panic("boom on delayed first cycle")
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		RunScheduledLoop(ctx, LoopConfig{
			Name:       "delayed-panic-service",
			Interval:   10 * time.Millisecond,
			StartDelay: 10 * time.Millisecond,
		}, work)
		close(done)
	}()

	<-done
	require.Greater(t, int(calls.Load()), 1, "loop must survive a panic in the delayed first cycle")
	assert.Contains(t, logs.String(), "boom on delayed first cycle")
}
