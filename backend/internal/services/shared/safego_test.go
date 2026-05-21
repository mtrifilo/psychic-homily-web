package shared

import (
	"bytes"
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// lockedBuffer is a concurrency-safe io.Writer wrapping a bytes.Buffer.
// GoSafe's slog.Error fires on the spawned goroutine while the test polls the
// captured output from the main goroutine, so the shared withCapturedSlog
// helper (which exposes a raw *bytes.Buffer) would race. This wrapper
// serializes the writes and reads.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// withSafeCapturedSlog is the concurrency-safe analogue of withCapturedSlog,
// used by the GoSafe tests because the panic log is emitted off-goroutine.
func withSafeCapturedSlog(t *testing.T) *lockedBuffer {
	t.Helper()
	buf := &lockedBuffer{}
	original := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(original) })
	return buf
}

// TestGoSafe_RecoversPanic is the load-bearing assertion of PSY-751: a
// fire-and-forget work func that panics is recovered inside the goroutine —
// the process survives — and both the slog path and the registered
// PanicHandler (the Sentry wiring point) fire with the goroutine name, panic
// value, and a stack trace.
//
// If GoSafe's recover regressed, the panicking goroutine would crash the test
// binary (Go makes an unrecovered goroutine panic fatal), so the test reaching
// its assertions at all already proves recovery.
func TestGoSafe_RecoversPanic(t *testing.T) {
	logs := withSafeCapturedSlog(t)
	snapshot := installRecordingPanicHandler(t)

	done := make(chan struct{})
	GoSafe(context.Background(), "test-fire-and-forget", func() {
		defer close(done)
		panic("boom in fire-and-forget")
	})

	select {
	case <-done:
		// work ran to its panic; the defer closed the channel before
		// unwinding into GoSafe's recover.
	case <-time.After(2 * time.Second):
		t.Fatal("GoSafe work func did not run")
	}

	// The PanicHandler runs after recover, so poll briefly for it to land.
	var got []capturedPanic
	require.Eventually(t, func() bool {
		got = snapshot()
		return len(got) == 1
	}, 2*time.Second, 5*time.Millisecond, "panic handler should be invoked exactly once")

	assert.Equal(t, "test-fire-and-forget", got[0].service)
	assert.Equal(t, "boom in fire-and-forget", got[0].panicValue)
	assert.NotEmpty(t, got[0].stack, "stack trace should be populated")
	assert.Contains(t, string(got[0].stack), "safego.go", "stack should reference the recover site")

	logged := logs.String()
	assert.Contains(t, logged, "fire-and-forget goroutine panic — recovered", "panic should be logged")
	assert.Contains(t, logged, `"goroutine":"test-fire-and-forget"`, "goroutine name should be in log")
	assert.Contains(t, logged, "boom in fire-and-forget", "panic value should be in log")
}

// TestGoSafe_RunsWorkOnHappyPath sanity-checks the non-panicking path: work
// runs exactly once and no panic handler fires.
func TestGoSafe_RunsWorkOnHappyPath(t *testing.T) {
	_ = withSafeCapturedSlog(t)
	snapshot := installRecordingPanicHandler(t)

	var calls atomic.Int32
	done := make(chan struct{})
	GoSafe(context.Background(), "happy-fire-and-forget", func() {
		calls.Add(1)
		close(done)
	})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("GoSafe work func did not run")
	}

	assert.Equal(t, int32(1), calls.Load(), "work should run exactly once")
	assert.Empty(t, snapshot(), "no panic handler should fire on the happy path")
}

// TestGoSafe_NilPanicHandlerIsNoop guards that with no handler installed (the
// package default — e.g. CLIs and tests that don't wire Sentry) a panicking
// work func is still recovered without crashing and the slog path still fires.
func TestGoSafe_NilPanicHandlerIsNoop(t *testing.T) {
	logs := withSafeCapturedSlog(t)
	SetPanicHandler(nil)
	t.Cleanup(func() { SetPanicHandler(nil) })

	done := make(chan struct{})
	GoSafe(context.Background(), "no-handler-fire-and-forget", func() {
		defer close(done)
		panic("boom-no-handler")
	})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("GoSafe work func did not run")
	}

	// Give the recover/log a moment to flush after the work func unwinds.
	require.Eventually(t, func() bool {
		return len(logs.String()) > 0
	}, 2*time.Second, 5*time.Millisecond, "slog path should fire even with a nil handler")
	assert.Contains(t, logs.String(), "boom-no-handler")
}

// TestGoSafe_ConcurrentLaunchesAllRecover exercises many concurrent GoSafe
// launches that each panic — none escape, the process survives, and the
// handler records one capture per launch. Run under -race this also guards the
// handler's locking.
func TestGoSafe_ConcurrentLaunchesAllRecover(t *testing.T) {
	_ = withSafeCapturedSlog(t)
	snapshot := installRecordingPanicHandler(t)

	const n = 50
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		GoSafe(context.Background(), "concurrent-fire-and-forget", func() {
			defer wg.Done()
			panic("boom-concurrent")
		})
	}
	wg.Wait()

	require.Eventually(t, func() bool {
		return len(snapshot()) == n
	}, 3*time.Second, 10*time.Millisecond, "every launch should record exactly one panic")
}
