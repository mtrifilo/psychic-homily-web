package shared

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"sync/atomic"
)

// auditWriteWorkers is how many audit-log writes may be in the database at
// once. Each one is a single-row INSERT, so the useful number is small: it
// exists to keep a burst from claiming the whole connection pool, not to make
// audit writing fast.
//
// It must stay well under config.DefaultDBMaxOpenConns, since every worker in
// flight holds one of those connections and the requests that produced the
// writes need the rest.
const auditWriteWorkers = 4

// auditWriteQueueDepth is how many writes may be waiting. It absorbs the INSTANT
// of a burst, not its sustained rate: a batch is capped at 200 items
// (maxEntityRequestSubmissions), each filing one row, and this holds five of
// those arriving together. Four workers doing single-row inserts clear far more
// than the per-user batch limiter (EntityRequestBatchBurstPerMinute) delivers,
// so reaching this depth means the database is not draining.
const auditWriteQueueDepth = 1024

// backgroundWrite is one queued unit of work and the label it reports under.
type backgroundWrite struct {
	name string
	work func()
}

// boundedWriter runs fire-and-forget writes on a fixed set of workers.
//
// It is the bounded counterpart to GoSafe: same panic containment and same
// caller contract (submit and move on), but a submission joins a queue instead
// of minting a goroutine, so N callers arriving at once produce at most
// `workers` concurrent database writes rather than N.
//
// LOSS POLICY, in the two places a write can be lost:
//
//   - A full queue drops the submission and logs it at Error with the queue
//     depth. Dropping rather than blocking is deliberate: the caller is a
//     request handler that has already done the durable work, and a full queue
//     means the database is far enough behind that blocking would convert an
//     audit backlog into a site-wide stall.
//   - shutdown drains what is queued within the caller's context deadline.
//     Whatever is still queued when that deadline passes is abandoned, and
//     shutdown reports how many. Submissions after shutdown are refused.
//
// Both are why a write that must not be lost belongs in the request's own
// transaction, not here.
type boundedWriter struct {
	name         string
	queue        chan backgroundWrite
	wg           sync.WaitGroup
	droppedCount atomic.Int64

	// mu guards closed and every send on queue, so a send can never race the
	// close in shutdown. It is held only for a non-blocking send.
	mu     sync.Mutex
	closed bool
}

// newBoundedWriter starts a writer with `workers` workers and a queue of
// `queueDepth`. Both are floored at 1: a writer with no workers accepts writes
// that never run.
func newBoundedWriter(name string, workers, queueDepth int) *boundedWriter {
	if workers < 1 {
		workers = 1
	}
	if queueDepth < 1 {
		queueDepth = 1
	}
	w := &boundedWriter{
		name:  name,
		queue: make(chan backgroundWrite, queueDepth),
	}
	w.wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer w.wg.Done()
			for job := range w.queue {
				w.run(job)
			}
		}()
	}
	return w
}

// submit queues work and returns whether it was accepted. It never blocks.
func (w *boundedWriter) submit(name string, work func()) bool {
	if work == nil {
		return false
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		w.drop(name, "writer is shut down")
		return false
	}
	select {
	case w.queue <- backgroundWrite{name: name, work: work}:
		return true
	default:
		w.drop(name, "queue is full")
		return false
	}
}

// shutdown stops accepting writes and waits for the queued ones to run.
// It returns an error naming what was abandoned if ctx expires first.
// Calling it more than once is safe; later calls wait on the same drain.
func (w *boundedWriter) shutdown(ctx context.Context) error {
	w.mu.Lock()
	if !w.closed {
		w.closed = true
		close(w.queue)
	}
	w.mu.Unlock()

	drained := make(chan struct{})
	go func() {
		w.wg.Wait()
		close(drained)
	}()

	select {
	case <-drained:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("%s writer drain did not finish: %d writes still queued", w.name, len(w.queue))
	}
}

// dropped is the running count of submissions this writer refused.
func (w *boundedWriter) dropped() int64 {
	return w.droppedCount.Load()
}

func (w *boundedWriter) drop(name, reason string) {
	w.droppedCount.Add(1)
	slog.Default().Error("background write dropped",
		"writer", w.name,
		"write", name,
		"reason", reason,
		"queue_depth", cap(w.queue),
		"dropped_total", w.droppedCount.Load(),
	)
}

// run mirrors GoSafe's recover so a panicking write is contained to its worker
// and escalated through the same process-wide handler. Without it a panic in
// one write would kill a worker permanently, and the pool would shrink silently
// until nothing wrote at all.
func (w *boundedWriter) run(job backgroundWrite) {
	defer func() {
		if r := recover(); r != nil {
			stack := debug.Stack()
			slog.Default().Error("bounded background write panic recovered",
				"writer", w.name,
				"write", job.name,
				"panic", r,
				"stack", string(stack),
			)
			invokePanicHandler(job.name, r, stack)
		}
	}()
	job.work()
}

// auditWriter is the process-wide writer every audit-log write goes through.
//
// One instance rather than one per service because the bound being enforced is
// on a shared resource, the connection pool: per-service pools would each be
// bounded and together be unbounded again. It is package-level for the same
// reason the panic handler is.
//
// Built on first use, so a binary that links this package for other reasons and
// never writes an audit row parks no idle workers.
var auditWriter = sync.OnceValue(func() *boundedWriter {
	return newBoundedWriter("audit_log", auditWriteWorkers, auditWriteQueueDepth)
})

// SubmitAuditWrite queues an audit-log write on the bounded writer.
//
// This is the audit-log replacement for GoSafe. Use it for a write that records
// what happened but must not fail or slow the operation it describes; `name`
// labels it in logs and in the Sentry tag a panic carries.
//
// GoSafe remains right for one-off background work whose call site cannot fan
// out. This exists for the writes that CAN: one per item of a 200-item batch.
func SubmitAuditWrite(name string, work func()) {
	auditWriter().submit(name, work)
}

// ShutdownAuditWrites drains the queued audit writes. Call it once the HTTP
// server has stopped, so no new writes are arriving; a write submitted
// concurrently with this is refused rather than queued.
func ShutdownAuditWrites(ctx context.Context) error {
	return auditWriter().shutdown(ctx)
}

// DroppedAuditWrites is the running count of audit writes refused since boot.
func DroppedAuditWrites() int64 {
	return auditWriter().dropped()
}
