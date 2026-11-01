package main

import (
	"log/slog"
	"net/http"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"

	"psychic-homily-backend/internal/logger"
)

const (
	// accessLogMsg is the msg of the Info line written once a request's
	// handler has returned.
	accessLogMsg = "request"
	// accessLogStartMsg is the msg of the Debug line written before the
	// handler runs.
	accessLogStartMsg = "request_started"
)

// accessLog writes the access log of each request through the logger logFor
// returns, resolved per request. Before the handler runs it writes a Debug
// line with the method, path, Origin header and request id, so a request whose
// handler never returns can be found when the logger is at Debug; at Info,
// such a request writes no line. Once the handler returns it writes one Info
// line with the same fields plus status, duration_ms and bytes (the response
// body bytes the handler wrote, after any compression inside this middleware;
// net/http discards a HEAD response's body, so those bytes never reach the
// client).
//
// status is the status the handler wrote, 200 when it returned without writing
// one, as net/http then sends. A panic that passes through this middleware
// (RecoverPanic hands on http.ErrAbortHandler once the response has started)
// still gets its Info line, with aborted set and the status written before it
// (0 when none was).
//
// Register it outside RecoverPanic, so the 500 RecoverPanic writes for a panic
// goes through this writer and shows as status 500.
func accessLog(logFor func() *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			log := logFor()
			// Capacity for the completion fields, so appending them does not
			// reallocate.
			attrs := make([]any, 0, 16)
			attrs = append(attrs,
				"method", r.Method,
				"path", r.URL.Path,
				"origin", r.Header.Get("Origin"),
				"request_id", logger.GetRequestID(r.Context()),
			)
			log.Debug(accessLogStartMsg, attrs...)

			start := time.Now()
			ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
			returned := false
			defer func() {
				status := ww.Status()
				if status == 0 && returned {
					status = http.StatusOK
				}
				attrs = append(attrs,
					"status", status,
					"duration_ms", float64(time.Since(start).Microseconds())/1000,
					"bytes", ww.BytesWritten(),
				)
				if !returned {
					attrs = append(attrs, "aborted", true)
				}
				log.Info(accessLogMsg, attrs...)
			}()
			next.ServeHTTP(ww, r)
			returned = true
		})
	}
}
