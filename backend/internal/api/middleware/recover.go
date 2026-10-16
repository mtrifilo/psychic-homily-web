package middleware

import (
	"bufio"
	"errors"
	"fmt"
	"io"
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
// http.ErrAbortHandler passes through with no log line (the Sentry handler,
// which sits inside, still captures it).
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

			// The error body inherits no representation metadata from the
			// response the handler meant to send, so nothing mislabels it or
			// lets a cache keep it. Every other header stays: CORS, security and
			// the request ID, and also any Set-Cookie the handler set, which
			// can be the clearing of a stale cookie.
			header := w.Header()
			for _, name := range representationHeaders {
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

// representationHeaders describe a response's content or its cache validity.
var representationHeaders = []string{
	"Content-Disposition", "Content-Encoding", "Content-Language", "Content-Length",
	"Content-Location", "Content-Range", "ETag", "Expires", "Last-Modified",
}

// panicLogMsg is the msg of the line RecoverPanic logs.
const panicLogMsg = "handler_panic"

// startTrackingWriter records whether any part of the response has gone out.
// It implements Flush, Hijack, Push and io.ReaderFrom as well as Unwrap, for
// the reason bodylessEncodingStripper gives: the writers inside it find those
// capabilities by direct type assertion. sentryhttp's HTTP/1 wrapper offers
// Hijack only when its inner writer is a Flusher, a Hijacker and an
// io.ReaderFrom together.
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

// ReadFrom marks the response started, as net/http's own ReadFrom writes the
// header before copying.
func (w *startTrackingWriter) ReadFrom(src io.Reader) (int64, error) {
	w.started = true
	if rf, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		return rf.ReadFrom(src)
	}
	// The anonymous struct hides this ReadFrom from io.Copy.
	return io.Copy(struct{ io.Writer }{w.ResponseWriter}, src)
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
