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
