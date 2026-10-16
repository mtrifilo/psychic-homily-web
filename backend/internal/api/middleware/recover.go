package middleware

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"net/http"

	autherrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/observability"
)

// RecoverPanic turns a handler panic into a 500 carrying the auth error
// envelope with the request ID, and logs one ERROR line with the request ID and
// the scrubbed panic value. The request body and headers are never logged.
//
// Register it inside RequestIDMiddleware, whose ID it reads from the context,
// and outside the Sentry handler, which captures the panic and re-panics into
// this recover.
//
// Once the response has started (a final status, a body byte, a flush or a
// hijack went out), a 500 can no longer be sent. RecoverPanic then logs and
// re-panics with http.ErrAbortHandler, on which net/http closes the HTTP/1
// connection or resets the HTTP/2 stream without logging a stack, so the client
// sees a truncated response rather than a complete one. A handler's own
// http.ErrAbortHandler passes through unlogged.
func RecoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tracked := &startTrackingWriter{ResponseWriter: w}
		defer func() {
			recovered := recover()
			if recovered == nil {
				return
			}
			if isAbortHandler(recovered) {
				panic(http.ErrAbortHandler)
			}

			ctx := r.Context()
			attrs := []any{
				"method", r.Method,
				"path", r.URL.Path,
				"panic", observability.ScrubText(fmt.Sprint(recovered)),
				"response_started", tracked.started,
			}
			log := logger.FromContext(ctx)
			if tracked.started {
				log.Error(panicLogMsg, attrs...)
				panic(http.ErrAbortHandler)
			}
			log.Error(panicLogMsg, append(attrs, "status", http.StatusInternalServerError)...)

			// These headers describe the representation the handler meant to
			// send; left in place they would mislabel the error body or let a
			// cache keep it.
			header := w.Header()
			for _, name := range []string{"Content-Length", "Content-Encoding", "Content-Disposition", "ETag", "Last-Modified"} {
				header.Del(name)
			}
			header.Set("Cache-Control", "no-store")
			writeJWTError(ctx, w, logger.GetRequestID(ctx), autherrors.CodeUnknown,
				autherrors.ToExternalMessage(autherrors.CodeUnknown), http.StatusInternalServerError)
		}()
		next.ServeHTTP(tracked, r)
	})
}

// isAbortHandler reports whether a panic value is, or wraps,
// http.ErrAbortHandler. net/http recognizes only the bare sentinel, so
// RecoverPanic re-panics with that.
func isAbortHandler(recovered any) bool {
	err, ok := recovered.(error)
	return ok && errors.Is(err, http.ErrAbortHandler)
}

// panicLogMsg is the msg of the line RecoverPanic logs.
const panicLogMsg = "handler_panic"

// startTrackingWriter records whether any part of the response has gone out.
// It forwards Flush, Hijack and Push by assertion as well as through Unwrap,
// so the writers inside it keep those capabilities.
type startTrackingWriter struct {
	http.ResponseWriter
	started bool
}

// WriteHeader marks the response started for a final status. An informational
// 1xx other than 101 Switching Protocols leaves the final status unsent.
func (w *startTrackingWriter) WriteHeader(code int) {
	if code >= http.StatusOK || code == http.StatusSwitchingProtocols {
		w.started = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *startTrackingWriter) Write(p []byte) (int, error) {
	w.started = true
	return w.ResponseWriter.Write(p)
}

func (w *startTrackingWriter) Flush() {
	if http.NewResponseController(w.ResponseWriter).Flush() == nil {
		w.started = true
	}
}

func (w *startTrackingWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	conn, rw, err := http.NewResponseController(w.ResponseWriter).Hijack()
	if err == nil {
		w.started = true
	}
	return conn, rw, err
}

func (w *startTrackingWriter) Push(target string, opts *http.PushOptions) error {
	if p, ok := w.ResponseWriter.(http.Pusher); ok {
		return p.Push(target, opts)
	}
	return http.ErrNotSupported
}

func (w *startTrackingWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
