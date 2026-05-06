// Package shared provides cross-cutting helpers for background services
// (panic-safe ticker loops, etc.). Per-service business logic stays in
// the per-domain service packages — this package is intentionally tiny.
package shared

import (
	"context"
	"log/slog"
	"runtime/debug"
	"time"
)

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
			slog.Default().Error("background service panic — service stopping",
				"service", name,
				"panic", r,
				"stack", string(debug.Stack()),
			)
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
			slog.Default().Error("background service tick panic — continuing",
				"service", name,
				"panic", r,
				"stack", string(debug.Stack()),
			)
		}
	}()
	work(ctx)
}
