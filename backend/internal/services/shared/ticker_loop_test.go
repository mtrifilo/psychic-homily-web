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

// TestRunTickerLoop_PanicInWorkContinuesLoop is the canonical demonstration:
// a tick that panics is recovered, logged, and the loop fires the next tick.
// This is the load-bearing assertion of PSY-615.
func TestRunTickerLoop_PanicInWorkContinuesLoop(t *testing.T) {
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
		RunTickerLoop(ctx, "test-service", 10*time.Millisecond, nil, false, work)
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

// TestRunTickerLoop_NormalWorkRunsRepeatedly sanity-checks the helper
// on the happy path: a non-panicking work function runs once per tick.
func TestRunTickerLoop_NormalWorkRunsRepeatedly(t *testing.T) {
	var calls atomic.Int32
	work := func(_ context.Context) {
		calls.Add(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		RunTickerLoop(ctx, "happy-service", 10*time.Millisecond, nil, false, work)
		close(done)
	}()

	<-done

	got := calls.Load()
	assert.GreaterOrEqual(t, int(got), 3, "expected several ticks in 100ms with 10ms interval; got %d", got)
}

// TestRunTickerLoop_RunImmediately fires the work function once before
// entering the ticker loop. Used by services that want a startup cycle.
func TestRunTickerLoop_RunImmediately(t *testing.T) {
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
		RunTickerLoop(ctx, "startup-service", 1*time.Hour, nil, true, work)
		close(done)
	}()

	<-done
	assert.Equal(t, int32(1), calls.Load(), "exactly one cycle should have run (the startup cycle)")
}

// TestRunTickerLoop_StartupPanicDoesNotKillLoop covers the case where
// the startup cycle panics — the loop must still fire a regular tick after.
func TestRunTickerLoop_StartupPanicDoesNotKillLoop(t *testing.T) {
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
		RunTickerLoop(ctx, "startup-panic-service", 10*time.Millisecond, nil, true, work)
		close(done)
	}()

	<-done

	got := calls.Load()
	require.Greater(t, int(got), 1, "loop should keep running after startup panic; got %d calls", got)
	assert.Contains(t, logs.String(), "boom on startup")
}

// TestRunTickerLoop_StopChannel covers the explicit `close(stopCh)`
// shutdown path. Each existing service uses both `<-ctx.Done()` and
// `<-stopCh`; the helper must honor stopCh too.
func TestRunTickerLoop_StopChannel(t *testing.T) {
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
		RunTickerLoop(ctx, "stop-ch-service", 10*time.Millisecond, stopCh, false, work)
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
		t.Fatal("RunTickerLoop did not return after stopCh was closed")
	}
}

// TestRunTickerLoop_ContextCancellationStopsLoop covers the `<-ctx.Done()`
// path independently from stopCh.
func TestRunTickerLoop_ContextCancellationStopsLoop(t *testing.T) {
	var calls atomic.Int32
	work := func(_ context.Context) {
		calls.Add(1)
	}

	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		RunTickerLoop(ctx, "ctx-cancel-service", 10*time.Millisecond, nil, false, work)
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
		t.Fatal("RunTickerLoop did not return after context cancel")
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

// TestRunTickerLoop_PanicHandlerInvokedOnTickPanic is the load-bearing
// PSY-617 assertion: a panic inside per-tick work invokes the registered
// PanicHandler with the service name, panic value, and stack trace —
// in addition to the slog.Error already exercised by the PSY-615 test.
// The handler is the wiring point cmd/server/main.go uses to escalate
// to Sentry.
func TestRunTickerLoop_PanicHandlerInvokedOnTickPanic(t *testing.T) {
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
		RunTickerLoop(ctx, "panic-handler-test-service", 10*time.Millisecond, nil, false, work)
		close(done)
	}()
	<-done

	got := snapshot()
	require.Len(t, got, 1, "panic handler should be invoked exactly once for the single tick panic")
	assert.Equal(t, "panic-handler-test-service", got[0].service)
	assert.Equal(t, "boom-tick", got[0].panicValue)
	assert.NotEmpty(t, got[0].stack, "stack trace should be populated")
	assert.Contains(t, string(got[0].stack), "ticker_loop.go", "stack should reference the recover site")
}

// TestRunTickerLoop_PanicHandlerInvokedOnSetupPanic exercises the outer
// recover path: time.NewTicker(0) panics during loop setup. The handler
// must be invoked exactly once, with the configured service name, before
// the loop returns.
func TestRunTickerLoop_PanicHandlerInvokedOnSetupPanic(t *testing.T) {
	_ = withCapturedSlog(t)
	snapshot := installRecordingPanicHandler(t)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("outer recover failed; panic escaped: %v", r)
		}
	}()

	RunTickerLoop(ctx, "setup-panic-handler-service", 0, nil, false, func(_ context.Context) {})

	got := snapshot()
	require.Len(t, got, 1, "panic handler should be invoked exactly once for the setup panic")
	assert.Equal(t, "setup-panic-handler-service", got[0].service)
	assert.NotNil(t, got[0].panicValue, "panic value should be propagated")
	assert.NotEmpty(t, got[0].stack)
}

// TestRunTickerLoop_NilPanicHandlerIsNoop guards the contract that an
// unset handler (the package default) doesn't panic and doesn't change
// the existing slog-only behaviour. Defensive clear in case a sibling
// test in the package leaked a handler past its t.Cleanup.
func TestRunTickerLoop_NilPanicHandlerIsNoop(t *testing.T) {
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
		RunTickerLoop(ctx, "no-handler-service", 10*time.Millisecond, nil, false, work)
		close(done)
	}()
	<-done

	assert.Greater(t, int(calls.Load()), 1, "loop should still continue past the panic")
	assert.Contains(t, logs.String(), "boom-no-handler", "slog path still fires when handler is nil")
}

// TestRunTickerLoop_PanicHandlerOwnPanicDoesNotKillLoop guards against a
// buggy handler taking down the loop it was meant to observe. If the
// handler panics, the recover inside invokePanicHandler must swallow it
// and the loop must continue ticking.
func TestRunTickerLoop_PanicHandlerOwnPanicDoesNotKillLoop(t *testing.T) {
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
		RunTickerLoop(ctx, "buggy-handler-service", 10*time.Millisecond, nil, false, work)
		close(done)
	}()
	<-done

	require.Greater(t, int(handlerCalls.Load()), 0, "handler should have been called at least once")
	assert.Greater(t, int(calls.Load()), 1, "loop should keep ticking after handler panic")
}

// TestRunTickerLoop_OuterRecoverCatchesSetupPanic covers the outer
// recover. `time.NewTicker(0)` panics with a duration <= 0; without the
// outer recover that panic would bubble out and kill the supervising
// goroutine. The loop returns early but the process keeps running.
func TestRunTickerLoop_OuterRecoverCatchesSetupPanic(t *testing.T) {
	logs := withCapturedSlog(t)

	work := func(_ context.Context) {}

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

	RunTickerLoop(ctx, "setup-panic-service", 0, nil, false, work)

	logged := logs.String()
	require.True(t,
		strings.Contains(logged, "background service panic — service stopping"),
		"outer recover should have logged the setup panic; got: %s", logged,
	)
	assert.Contains(t, logged, `"service":"setup-panic-service"`)
}
