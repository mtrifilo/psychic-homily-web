// Package shared provides cross-cutting helpers for background services
// (panic-safe ticker loops, etc.). Per-service business logic stays in
// the per-domain service packages — this package is intentionally tiny.
package shared

import (
	"context"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"
)

// PanicHandler is invoked when a panic is recovered inside RunTickerLoop.
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

// SetPanicHandler installs a process-wide handler for ticker-loop panics.
// Pass nil to clear (used by tests via t.Cleanup).
//
// Intended to be called once at startup from cmd/server/main.go after
// Sentry is initialised. Safe to call concurrently with RunTickerLoop.
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
			slog.Default().Error("ticker-loop panic handler itself panicked",
				"service", service,
				"panic", r,
			)
		}
	}()
	h(service, panicValue, stack)
}

// RunTickerLoop runs `work` on every tick of `interval`, returning when
// `ctx` is canceled or `stopCh` is closed.
//
// Two layers of `recover()` are intentional:
//
//  1. The outer recover (top of the function) catches a panic in the
//     ticker setup itself — `time.NewTicker(interval)` panics if
//     `interval <= 0`, for example. Without it, that panic would bubble
//     out into the supervising goroutine and crash the process.
//  2. The inner per-tick recover (inside `runOneCycle`) lets a single
//     bad tick fail without taking down the loop. The next tick still
//     fires.
//
// Both layers log via `slog.Default()` with field keys matching the
// project's slog convention (`service`, `panic`, `stack`).
//
// `runImmediately` is a convenience for services that want to fire
// `work` once at startup before entering the ticker loop. Most existing
// services do this so an admin exercising the service doesn't have to
// wait a full interval to see output. The startup cycle is wrapped in
// the same per-cycle recover, so a panic there also doesn't kill the
// loop.
//
// `stopCh` is optional (nil-safe). The existing services pair `ctx` with
// a `close(stopCh)`-based stop channel for explicit shutdown signals;
// the helper preserves that semantics.
func RunTickerLoop(
	ctx context.Context,
	name string,
	interval time.Duration,
	stopCh <-chan struct{},
	runImmediately bool,
	work func(context.Context),
) {
	defer func() {
		if r := recover(); r != nil {
			stack := debug.Stack()
			slog.Default().Error("background service panic — service stopping",
				"service", name,
				"panic", r,
				"stack", string(stack),
			)
			invokePanicHandler(name, r, stack)
		}
	}()

	if runImmediately {
		runOneCycle(ctx, name, work)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-stopCh:
			return
		case <-ticker.C:
			runOneCycle(ctx, name, work)
		}
	}
}

// runOneCycle isolates the per-tick recover so a panic in one tick
// doesn't stop the loop. Exposed only inside this package — callers
// drive cycles through RunTickerLoop.
func runOneCycle(ctx context.Context, name string, work func(context.Context)) {
	defer func() {
		if r := recover(); r != nil {
			stack := debug.Stack()
			slog.Default().Error("background service tick panic — continuing",
				"service", name,
				"panic", r,
				"stack", string(stack),
			)
			invokePanicHandler(name, r, stack)
		}
	}()
	work(ctx)
}
