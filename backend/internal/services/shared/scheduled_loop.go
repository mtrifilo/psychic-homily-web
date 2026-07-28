// Package shared provides cross-cutting helpers for background services
// (panic-safe scheduled loops, durable run state, etc.). Per-service business
// logic stays in the per-domain service packages — this package is
// intentionally tiny.
package shared

import (
	"context"
	"log/slog"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
)

// PanicHandler is invoked when a panic is recovered inside RunScheduledLoop.
// `service` is the loop name, `panicValue` is the recovered value (whatever
// the work passed to `panic`), and `stack` is the stack trace as a string.
//
// Wiring point for observability: cmd/server/main.go installs a Sentry-
// capturing handler at startup. Tests install their own handler. When no
// handler is set, panics are logged via slog and otherwise swallowed —
// matching pre-PSY-617 behaviour.
//
// The handler runs on the goroutine that recovered the panic; it should
// return promptly (Sentry's CaptureException is non-blocking, so the
// canonical handler is fine).
type PanicHandler func(service string, panicValue any, stack []byte)

var (
	panicHandlerMu sync.RWMutex
	panicHandler   PanicHandler
)

// SetPanicHandler installs a process-wide handler for scheduled-loop panics.
// Pass nil to clear (used by tests via t.Cleanup).
//
// Intended to be called once at startup from cmd/server/main.go after
// Sentry is initialised. Safe to call concurrently with RunScheduledLoop.
func SetPanicHandler(h PanicHandler) {
	panicHandlerMu.Lock()
	panicHandler = h
	panicHandlerMu.Unlock()
}

// invokePanicHandler runs the registered handler under the read lock so it
// can't race with SetPanicHandler. Recovers any panic the handler itself
// raises so a buggy handler can't take down the loop it was meant to
// observe.
func invokePanicHandler(service string, panicValue any, stack []byte) {
	panicHandlerMu.RLock()
	h := panicHandler
	panicHandlerMu.RUnlock()
	if h == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			slog.Default().Error("scheduled-loop panic handler itself panicked",
				"service", service,
				"panic", r,
			)
		}
	}()
	h(service, panicValue, stack)
}

// Scheduling policy. Fixed for the process — a caller that needs different
// values sets them per loop on LoopConfig.
const (
	// defaultFirstCycleDelay bounds the first cycle of a loop that does not run
	// at boot and has no persisted state to schedule from. Long enough to keep
	// third-party traffic off the boot path, short enough to fit inside any
	// uptime the platform realistically gives us.
	defaultFirstCycleDelay = 15 * time.Minute

	// runStatePersistenceThreshold is the interval at or above which a loop gets
	// durable run state. Below it, persistence is waste: a sub-hour loop reaches
	// its first cycle inside any plausible uptime (the sub-hour loops were the
	// control group that proved this empirically), so a row write every 30s buys
	// nothing.
	runStatePersistenceThreshold = time.Hour

	// defaultRunLease is how long a claim stays valid before another instance may
	// treat it as abandoned. It only has to cover a normal cycle; the bounded
	// batches these loops use run in minutes.
	defaultRunLease = time.Hour
)

// Tunable at runtime: the env-configured knobs, plus the retry floor that tests
// compress to keep their runtime sane.
var (
	// claimRetryDelay is how soon a loop retries after a claim was refused
	// because another instance held a live one. Without a retry the loop would
	// wait a full interval, so one crashed process could cost a daily sweep a
	// whole day — the shape of failure this ticket exists to make impossible.
	claimRetryDelay = 5 * time.Minute

	// Catch-up stagger. Seven sweeps discovering at the same boot that they are
	// overdue would otherwise fire together, and the MusicBrainz / Nominatim /
	// Spotify rate limits they share are ToS obligations rather than best-effort.
	// Successive overdue loops take successive slots.
	catchUpBaseDelay    = EnvPositiveDuration("SWEEP_CATCHUP_BASE_SECONDS", time.Second, 60*time.Second)
	catchUpSpacing      = EnvPositiveDuration("SWEEP_CATCHUP_SPACING_SECONDS", time.Second, 90*time.Second)
	catchUpMaxDelay     = EnvPositiveDuration("SWEEP_CATCHUP_MAX_SECONDS", time.Second, 30*time.Minute)
	catchUpSlotsClaimed atomic.Int64
)

// nextCatchUpDelay hands out the next stagger slot. Process-wide and monotonic,
// so the order is the order loops start — deterministic enough to test, and it
// needs no coordination between loops.
func nextCatchUpDelay() time.Duration {
	slot := catchUpSlotsClaimed.Add(1) - 1
	delay := catchUpBaseDelay + time.Duration(slot)*catchUpSpacing
	if delay > catchUpMaxDelay {
		delay = catchUpMaxDelay
	}
	return delay
}

// LoopConfig describes a background loop's schedule.
//
// It replaced a positional `runImmediately bool`. The old parameter's
// false branch meant "wait one full interval, measured from process start", which
// on a continuously-deployed platform meant "never run at all" for any interval
// longer than the deploy cadence — seven production sweeps ran exactly one cycle
// in the life of production because of it.
//
// The replacement is not merely a renamed field: a full process-anchored interval
// is no longer expressible at any call site. Whatever a caller sets, the first
// cycle happens at boot, after a bounded StartDelay, or at the time persisted
// state says it is due.
type LoopConfig struct {
	// Name identifies the loop in logs and in background_service_runs. It is the
	// primary key of the persisted state, so renaming a loop resets its history.
	Name string

	// Interval is the steady-state period between cycles. Must be positive.
	Interval time.Duration

	// StopCh is optional (nil-safe): the explicit shutdown signal that services
	// pair with ctx cancellation.
	StopCh <-chan struct{}

	// RunAtBoot runs a cycle on the boot path itself, before any waiting. Use it
	// for loops that are cheap, local, or whose output an operator expects to see
	// right after a deploy. Leaving it false is SAFE — it costs at most StartDelay
	// (or the persisted due time), never a full interval.
	RunAtBoot bool

	// StartDelay bounds the first cycle when RunAtBoot is false and there is no
	// persisted state. Zero means defaultFirstCycleDelay. Always capped at
	// Interval, so a 30s loop is never pushed out to 15m.
	StartDelay time.Duration

	// Store makes the schedule durable. Leave nil to get the process-wide default
	// store for loops at or above runStatePersistenceThreshold, and no persistence
	// below it. Set it explicitly only in tests, or to opt a short loop in.
	Store RunStore

	// RunLease is how long this loop's claim stays valid before another instance
	// may treat a crashed run as abandoned. Zero means defaultRunLease. Raise it
	// for a loop whose cycle can legitimately run longer than an hour.
	RunLease time.Duration
}

// RunScheduledLoop runs `work` on a schedule, returning when `ctx` is canceled or
// `cfg.StopCh` is closed.
//
// Two layers of `recover()` are intentional:
//
//  1. The outer recover (top of the function) catches a panic in the scheduling
//     machinery itself. Without it, that panic would bubble out into the
//     supervising goroutine and crash the process.
//  2. The inner per-cycle recover (inside runWork) lets a single bad cycle fail
//     without taking down the loop. The next cycle still fires, and the panic is
//     recorded in the run trace.
//
// Both layers log via `slog.Default()` with field keys matching the project's
// slog convention (`service`, `panic`, `stack`).
//
// The loop is driven by a timer rather than a time.Ticker on purpose: every delay
// is recomputed from the outcome of the previous cycle (ran / refused / due at),
// so no wait is ever anchored to process start.
func RunScheduledLoop(ctx context.Context, cfg LoopConfig, work func(context.Context)) {
	defer func() {
		if r := recover(); r != nil {
			stack := debug.Stack()
			slog.Default().Error("background service panic — service stopping",
				"service", cfg.Name,
				"panic", r,
				"stack", string(stack),
			)
			invokePanicHandler(cfg.Name, r, stack)
		}
	}()

	runner := newLoopRunner(cfg, work)

	// The boot cycle is forced past the due check: a caller asking for RunAtBoot
	// wants output after a deploy, which is the behaviour ten healthy loops rely
	// on today. The claim's LEASE check still applies, so a boot cycle cannot
	// collide with a run already in flight on a draining instance.
	var next time.Duration
	if cfg.RunAtBoot {
		ran := runner.runCycle(ctx, true)
		next = runner.delayAfter(ctx, ran)
	} else {
		next = runner.firstCycleDelay(ctx)
	}

	for {
		if !sleepOrStop(ctx, cfg.StopCh, next) {
			return
		}
		ran := runner.runCycle(ctx, false)
		next = runner.delayAfter(ctx, ran)
	}
}

// sleepOrStop waits `d`, returning false if the loop should exit instead. A timer
// rather than time.Sleep so a deploy landing inside a long wait shuts the process
// down immediately.
func sleepOrStop(ctx context.Context, stopCh <-chan struct{}, d time.Duration) bool {
	if d < 0 {
		d = 0
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-stopCh:
		return false
	case <-timer.C:
		return true
	}
}

// loopRunner holds the normalised config for one running loop.
type loopRunner struct {
	name       string
	interval   time.Duration
	startDelay time.Duration
	lease      time.Duration
	store      RunStore
	work       func(context.Context)
}

// newLoopRunner normalises the config once, so every decision downstream can
// assume sane values instead of re-checking them.
func newLoopRunner(cfg LoopConfig, work func(context.Context)) *loopRunner {
	interval := cfg.Interval
	if interval <= 0 {
		// The old code let time.NewTicker panic here (caught by the outer
		// recover, killing the loop). Degrading to hourly and saying so keeps a
		// misconfigured interval from silently stopping a service.
		slog.Default().Error("background service configured with non-positive interval — defaulting to 1h",
			"service", cfg.Name, "interval", cfg.Interval)
		interval = time.Hour
	}

	startDelay := cfg.StartDelay
	if startDelay <= 0 {
		startDelay = defaultFirstCycleDelay
	}
	if startDelay > interval {
		startDelay = interval
	}

	lease := cfg.RunLease
	if lease <= 0 {
		lease = defaultRunLease
	}

	store := cfg.Store
	if typedNilStore(store) {
		store = nil
		if interval >= runStatePersistenceThreshold {
			if def := DefaultRunStore(); !typedNilStore(def) {
				store = def
			}
		}
	}

	return &loopRunner{
		name:       cfg.Name,
		interval:   interval,
		startDelay: startDelay,
		lease:      lease,
		store:      store,
		work:       work,
	}
}

// firstCycleDelay decides when the first cycle happens for a loop that does not
// run at boot. This is the heart of the design: with persisted state the answer
// comes from when the loop last COMPLETED, so a process that has just restarted
// picks up where its predecessor left off instead of restarting the clock.
func (r *loopRunner) firstCycleDelay(ctx context.Context) time.Duration {
	if r.store == nil {
		return r.startDelay
	}
	due, err := r.store.DueIn(ctx, r.name, r.interval)
	if err != nil {
		// Fail open toward running: losing the trace is better than losing the
		// sweep. The bounded start delay is still immune to deploy cadence.
		logStoreError(r.name, "due-check", err)
		return r.startDelay
	}
	if due <= 0 {
		delay := nextCatchUpDelay()
		slog.Default().Info("background service overdue at boot — catching up",
			"service", r.name,
			"interval", r.interval,
			"catch_up_in", delay,
		)
		return delay
	}
	slog.Default().Info("background service scheduled from persisted state",
		"service", r.name,
		"interval", r.interval,
		"next_cycle_in", due,
	)
	return due
}

// delayAfter computes the wait before the next cycle.
//
// The refused branch matters: a claim can be refused because a crashed process
// left a live claim behind, and waiting a full interval on refusal would let one
// crash cost a daily sweep a whole day. Re-asking the store for the real due time
// keeps a refusal cheap without turning the loop into a poller.
func (r *loopRunner) delayAfter(ctx context.Context, ran bool) time.Duration {
	if ran || r.store == nil {
		return r.interval
	}
	due, err := r.store.DueIn(ctx, r.name, r.interval)
	if err != nil {
		logStoreError(r.name, "due-check", err)
		return claimRetryDelay
	}
	if due < claimRetryDelay {
		return claimRetryDelay
	}
	return due
}

// runCycle runs one cycle, reporting whether it actually ran. `force` skips the
// due check (boot cycles) but never the concurrency check.
func (r *loopRunner) runCycle(ctx context.Context, force bool) bool {
	if r.store == nil {
		r.runWork(ctx)
		return true
	}

	token, claimed, err := r.store.Claim(ctx, r.name, r.interval, r.lease, force)
	if err != nil {
		// Fail open. If the database is unreachable the cycle itself will fail
		// fast anyway — every one of these loops is database-backed — so running
		// unclaimed cannot turn into a runaway against a third-party API.
		logStoreError(r.name, "claim", err)
		r.runWork(ctx)
		return true
	}
	if !claimed {
		slog.Default().Info("background service cycle skipped — not due, or already running elsewhere",
			"service", r.name,
		)
		return false
	}

	rec := &cycleRecorder{}
	start := time.Now()
	panicked := r.runWork(withCycleRecorder(ctx, rec))
	rows, cycleErr := rec.snapshot()

	// Detached context: a shutdown that cancels ctx mid-cycle must still be able
	// to close the claim out, or every deploy would leave a 'running' row behind
	// and the next boot would wait out the lease before retrying.
	completeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := r.store.Complete(completeCtx, r.name, token, CycleOutcome{
		Rows:     rows,
		Err:      cycleErr,
		Panicked: panicked,
		Duration: time.Since(start),
		// A cycle that returned because the process is shutting down did not
		// finish its work; recording it as complete would hand the next boot a
		// schedule that says "done for another interval".
		Interrupted: ctx.Err() != nil && !panicked,
	}); err != nil {
		logStoreError(r.name, "complete", err)
	}
	return true
}

// runWork isolates the per-cycle recover so a panic in one cycle doesn't stop the
// loop, and reports whether it panicked so the run trace can say so.
func (r *loopRunner) runWork(ctx context.Context) (panicked bool) {
	defer func() {
		if rec := recover(); rec != nil {
			panicked = true
			stack := debug.Stack()
			slog.Default().Error("background service tick panic — continuing",
				"service", r.name,
				"panic", rec,
				"stack", string(stack),
			)
			invokePanicHandler(r.name, rec, stack)
		}
	}()
	r.work(ctx)
	return false
}
