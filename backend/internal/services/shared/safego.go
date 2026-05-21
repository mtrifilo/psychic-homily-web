package shared

import (
	"context"
	"log/slog"
	"runtime/debug"
)

// GoSafe runs `work` in a new goroutine guarded by a panic recover, then
// returns immediately. It is the canonical wrapper for fire-and-forget
// goroutines launched from request handlers and service methods (audit-log
// writes, Discord webhooks, notification fan-out, async DB touch-ups, etc.).
//
// Why this exists: Go treats a panic that escapes a goroutine as fatal — it
// crashes the entire process. An HTTP handler's Sentry middleware only wraps
// the request-serving goroutine, NOT the children it spawns with `go`. So a
// nil-pointer deref inside one fire-and-forget LogAction or sendWebhook would
// take down the whole server. GoSafe contains that blast radius to the one
// goroutine.
//
// The recover mirrors RunTickerLoop's guard and routes through the same
// process-wide PanicHandler (installed once in cmd/server/main.go after Sentry
// init), so a recovered panic is both logged via slog.Default() and escalated
// to Sentry tagged with `name`. When no handler is set (tests, CLIs), the
// slog.Error path still fires and the panic is otherwise swallowed.
//
// `ctx` is accepted so call sites can hand the goroutine a request- or
// service-scoped context (deadlines, request-id) and so future work that
// needs cancellation has a hook. GoSafe itself does not consume `ctx`; the
// goroutine outlives the request by design (that is the point of
// fire-and-forget), so `work` should capture only what it needs and must not
// assume `ctx` stays un-canceled. Pass context.Background() when there is no
// meaningful parent context.
//
// `name` labels the goroutine in logs and the Sentry `service` tag; use a
// short, stable identifier (e.g. "audit_log", "discord_webhook").
func GoSafe(ctx context.Context, name string, work func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				stack := debug.Stack()
				slog.Default().Error("fire-and-forget goroutine panic — recovered",
					"goroutine", name,
					"panic", r,
					"stack", string(stack),
				)
				invokePanicHandler(name, r, stack)
			}
		}()
		work()
	}()
}
