package shared

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"sync/atomic"
)

// AuditWriteWorkers is how many audit-log writes may be in the database at
// once. Each one is a single-row INSERT, so the useful number is small: it
// exists to keep a burst from claiming the whole connection pool, not to make
// audit writing fast.
//
// Every worker in flight holds one connection from the pool, and the requests
// that produced the writes need the rest, so this has to stay well below the
// pool's own ceiling. It is exported so the composition root can compare the two
// at boot and say so when they cross, since the pool ceiling is an env var and
// nothing here can see its value.
const AuditWriteWorkers = 4

// auditWriteQueueDepth is how many writes may be waiting. It absorbs the INSTANT
// of a burst rather than a sustained rate: a batch is capped at 200 items
// (maxEntityRequestSubmissions), each filing one row, and this holds five of
// those arriving together.
//
// It is a per-process depth against a PER-USER rate limit
// (EntityRequestBatchBurstPerMinute), so enough concurrent batching users can
// reach it with a perfectly healthy database. Reaching it means writes are
// arriving faster than AuditWriteWorkers clear them, which is a slow database or
// a lot of users, and the drop policy below is what happens either way.
const auditWriteQueueDepth = 1024

// auditWriteDropLogInterval is how many drops pass between log lines after the
// first. The drop path is reached exactly when the process is already saturated,
// so a line per dropped write is a log flood at the worst moment; the count is
// exact regardless, and DroppedAuditWrites reads it.
const auditWriteDropLogInterval = 100

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
// LOSS POLICY, in the three places a write can be lost:
//
//   - A full queue drops the submission. Dropping rather than blocking is
//     deliberate: the caller is a request handler that has already done the
//     durable work, and a full queue means the writes are arriving faster than
//     the workers clear them, where blocking would convert an audit backlog into
//     a site-wide stall.
//   - shutdown drains what is queued within the caller's context deadline.
//     Whatever is still queued when that deadline passes is abandoned, and
//     shutdown reports how many. Submissions after shutdown are refused.
//   - A worker is occupied for as long as its write runs. Work submitted here
//     must therefore bound its own duration; an unbounded write parks a worker,
//     and enough of them park the pool. AuditLogService gives its inserts a
//     deadline for that reason.
//
// All three are why a write that must not be lost belongs in the request's own
// transaction, not here. Every drop is counted (see dropped) whether or not it
// is logged.
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
//
// Nothing slow runs under the lock, logging included. The lock is global to the
// process and the drop path is reached exactly when the writer is already
// saturated, so a log write inside it would turn every audit-submitting request
// into a queue behind one blocking stdout write: the stall the bound exists to
// prevent, arriving through the bound itself.
func (w *boundedWriter) submit(name string, work func()) bool {
	if work == nil {
		w.recordDrop(name, "no work supplied")
		return false
	}

	w.mu.Lock()
	accepted, reason := w.offer(name, work)
	w.mu.Unlock()

	if !accepted {
		w.recordDrop(name, reason)
	}
	return accepted
}

// offer is the whole of submit's critical section. Caller holds w.mu.
func (w *boundedWriter) offer(name string, work func()) (bool, string) {
	if w.closed {
		return false, "writer is shut down"
	}
	select {
	case w.queue <- backgroundWrite{name: name, work: work}:
		return true, ""
	default:
		return false, "queue is full"
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

// dropped is the running count of submissions this writer refused. It is exact:
// only the LOGGING of a drop is sampled.
func (w *boundedWriter) dropped() int64 {
	return w.droppedCount.Load()
}

// recordDrop counts a refused submission and logs the first, then every
// auditWriteDropLogInterval-th. Never called with w.mu held: it writes to the
// process log, and a saturated writer would otherwise serialize every submitting
// request behind that write.
func (w *boundedWriter) recordDrop(name, reason string) {
	total := w.droppedCount.Add(1)
	if total != 1 && total%auditWriteDropLogInterval != 0 {
		return
	}
	slog.Default().Error("background write dropped",
		"writer", w.name,
		"write", name,
		"reason", reason,
		"queue_depth", cap(w.queue),
		"dropped_total", total,
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
// never writes an audit row parks no idle workers. auditWriterStarted records
// whether that happened, so shutting down is free for those binaries too.
var (
	auditWriterStarted atomic.Bool
	auditWriter        = sync.OnceValue(func() *boundedWriter {
		auditWriterStarted.Store(true)
		return newBoundedWriter("audit_log", AuditWriteWorkers, auditWriteQueueDepth)
	})
)

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
//
// ctx bounds the drain, so give it a budget of its own rather than one already
// spent elsewhere: the case with a backlog to drain is the same case that made
// the earlier work slow.
//
// A process that never submitted an audit write has nothing to drain and does
// not start the writer just to close it.
func ShutdownAuditWrites(ctx context.Context) error {
	if !auditWriterStarted.Load() {
		return nil
	}
	return auditWriter().shutdown(ctx)
}

// DroppedAuditWrites is the running count of audit writes refused since boot.
func DroppedAuditWrites() int64 {
	return auditWriter().dropped()
}
