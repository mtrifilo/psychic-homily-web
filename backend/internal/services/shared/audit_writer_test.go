package shared

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A 200-item entity-request batch files one audit row per item. Before the
// bounded writer that was 200 goroutines racing for the connection pool at
// once; the whole point of the writer is that the same burst is served by a
// fixed number of workers and still loses nothing.
func TestBoundedWriter_BurstIsBoundedAndLosesNothing(t *testing.T) {
	const (
		workers = 4
		burst   = 200
	)
	w := newBoundedWriter("test_audit", workers, burst)

	var (
		mu      sync.Mutex
		live    int
		maxLive int
		ran     atomic.Int64
	)

	// A barrier so the burst is genuinely concurrent: without it the submits
	// could be serialized by the test's own timing and prove nothing.
	release := make(chan struct{})
	for i := 0; i < burst; i++ {
		accepted := w.submit("audit_log", func() {
			mu.Lock()
			live++
			if live > maxLive {
				maxLive = live
			}
			mu.Unlock()

			<-release

			mu.Lock()
			live--
			mu.Unlock()
			ran.Add(1)
		})
		require.True(t, accepted, "a burst inside the queue depth must not be dropped")
	}
	close(release)

	require.NoError(t, w.shutdown(context.Background()))

	assert.Equal(t, int64(burst), ran.Load(), "every queued audit write must run")
	assert.LessOrEqual(t, maxLive, workers,
		"a burst must not put more than the worker count in the database at once")
	assert.Zero(t, w.dropped())
}

// Shutdown is what makes "queued" different from "lost": everything accepted
// before it must still run.
func TestBoundedWriter_ShutdownDrainsQueuedWrites(t *testing.T) {
	w := newBoundedWriter("test_audit", 1, 64)

	var ran atomic.Int64
	for i := 0; i < 50; i++ {
		require.True(t, w.submit("audit_log", func() { ran.Add(1) }))
	}

	require.NoError(t, w.shutdown(context.Background()))
	assert.Equal(t, int64(50), ran.Load())
}

func TestBoundedWriter_SubmitAfterShutdownIsRefused(t *testing.T) {
	w := newBoundedWriter("test_audit", 1, 4)
	require.NoError(t, w.shutdown(context.Background()))

	var ran atomic.Int64
	assert.False(t, w.submit("audit_log", func() { ran.Add(1) }))
	assert.Equal(t, int64(1), w.dropped())
	assert.Zero(t, ran.Load(), "a refused write must not run")
}

// Shutdown twice happens on any path that both defers and calls it; it must not
// panic on a closed channel.
func TestBoundedWriter_ShutdownIsIdempotent(t *testing.T) {
	w := newBoundedWriter("test_audit", 2, 4)
	require.NoError(t, w.shutdown(context.Background()))
	require.NoError(t, w.shutdown(context.Background()))
}

// The documented loss policy: a full queue drops rather than blocking the
// caller, and says so.
func TestBoundedWriter_FullQueueDropsRatherThanBlocking(t *testing.T) {
	w := newBoundedWriter("test_audit", 1, 1)

	block := make(chan struct{})
	require.True(t, w.submit("audit_log", func() { <-block }))

	// The single worker is now parked, so the queue fills and then refuses.
	// Submit must return either way and never block the caller.
	accepted := 0
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 32; i++ {
			if w.submit("audit_log", func() {}) {
				accepted++
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Submit blocked on a full queue")
	}

	assert.LessOrEqual(t, accepted, 1, "a queue of one must not accept more than one waiting write")
	assert.Positive(t, w.dropped(), "drops must be counted, not silent")

	close(block)
	require.NoError(t, w.shutdown(context.Background()))
}

// A panicking write must not take its worker with it, or the pool shrinks
// silently until nothing writes at all.
func TestBoundedWriter_PanicIsContainedToOneWrite(t *testing.T) {
	w := newBoundedWriter("test_audit", 1, 8)

	var ran atomic.Int64
	require.True(t, w.submit("audit_log", func() { panic("audit write blew up") }))
	for i := 0; i < 5; i++ {
		require.True(t, w.submit("audit_log", func() { ran.Add(1) }))
	}

	require.NoError(t, w.shutdown(context.Background()))
	assert.Equal(t, int64(5), ran.Load(), "the worker must survive a panicking write")
}

func TestBoundedWriter_NilWorkIsRefused(t *testing.T) {
	w := newBoundedWriter("test_audit", 1, 4)
	defer func() { _ = w.shutdown(context.Background()) }()

	assert.False(t, w.submit("audit_log", nil))
	assert.Equal(t, int64(1), w.dropped(), "every refusal is counted, including this one")
}

// The drop count is exact even though the LOGGING of drops is sampled: the count
// is the only signal that says how much of the audit trail is missing.
func TestBoundedWriter_EveryDropIsCountedNotJustTheLoggedOnes(t *testing.T) {
	const attempts = auditWriteDropLogInterval * 2

	w := newBoundedWriter("test_audit", 1, 1)
	block := make(chan struct{})
	require.True(t, w.submit("audit_log", func() { <-block }))

	refused := 0
	for i := 0; i < attempts; i++ {
		if !w.submit("audit_log", func() {}) {
			refused++
		}
	}

	assert.Equal(t, int64(refused), w.dropped(),
		"the counter must not be sampled the way the log line is")
	assert.Greater(t, refused, auditWriteDropLogInterval,
		"the burst must be big enough that sampling would show up")

	close(block)
	require.NoError(t, w.shutdown(context.Background()))
}

// A process that never wrote an audit row must not start the writer just to
// close it.
func TestShutdownAuditWritesIsFreeWhenNothingWasSubmitted(t *testing.T) {
	if auditWriterStarted.Load() {
		t.Skip("another test in this package already started the process-wide writer")
	}
	require.NoError(t, ShutdownAuditWrites(context.Background()))
	assert.False(t, auditWriterStarted.Load(), "shutdown must not construct the writer")
}

// A drain that cannot finish reports what it abandoned instead of returning
// nil and letting the process exit as though nothing was lost.
func TestBoundedWriter_ShutdownReportsAnIncompleteDrain(t *testing.T) {
	w := newBoundedWriter("test_audit", 1, 8)

	block := make(chan struct{})
	require.True(t, w.submit("audit_log", func() { <-block }))
	require.True(t, w.submit("audit_log", func() {}))

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := w.shutdown(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "still queued")

	close(block)
}
