package middleware

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	autherrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
)

const recoverTestRequestID = "recover-test-request-id"

// serveRecoverPanic runs handler behind RecoverPanic on a request carrying a
// request ID and a context logger that writes JSON into the returned buffer.
// It returns the recorder and whatever RecoverPanic panicked with.
func serveRecoverPanic(t *testing.T, handler http.HandlerFunc) (*httptest.ResponseRecorder, *bytes.Buffer, any) {
	t.Helper()
	var logs bytes.Buffer
	log := logger.WithRequestID(slog.New(slog.NewJSONHandler(&logs, nil)), recoverTestRequestID)
	req := httptest.NewRequest(http.MethodGet, "/probe?token=query-secret", nil)
	req.Header.Set("Authorization", "Bearer header-secret")
	ctx := logger.NewContext(logger.SetRequestID(req.Context(), recoverTestRequestID), log)
	rr := httptest.NewRecorder()

	var repanicked any
	func() {
		defer func() { repanicked = recover() }()
		RecoverPanic(handler).ServeHTTP(rr, req.WithContext(ctx))
	}()
	return rr, &logs, repanicked
}

func panicLogLines(t *testing.T, logs *bytes.Buffer) []map[string]any {
	t.Helper()
	var lines []map[string]any
	for _, raw := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		if raw == "" {
			continue
		}
		var line map[string]any
		if err := json.Unmarshal([]byte(raw), &line); err != nil {
			t.Fatalf("log line %q is not JSON: %v", raw, err)
		}
		if line["msg"] == panicLogMsg {
			lines = append(lines, line)
		}
	}
	return lines
}

func TestRecoverPanic_AnswersEnvelopeWhenNothingWasWritten(t *testing.T) {
	rr, logs, repanicked := serveRecoverPanic(t, func(w http.ResponseWriter, _ *http.Request) {
		h := w.Header()
		h.Set("Access-Control-Allow-Origin", "https://app.example.com")
		h.Set("Content-Type", "text/calendar")
		h.Set("Content-Disposition", `attachment; filename="shows.ics"`)
		h.Set("Content-Length", "4096")
		h.Set("Content-Encoding", "gzip")
		h.Set("ETag", `"v1"`)
		h.Set("Last-Modified", "Mon, 05 Oct 2026 00:00:00 GMT")
		h.Set("Cache-Control", "public, max-age=300")
		panic(errors.New("boom: password=panic-secret"))
	})

	if repanicked != nil {
		t.Fatalf("RecoverPanic re-panicked with %v, want the panic answered", repanicked)
	}
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rr.Code)
	}
	var body JWTErrorResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %q is not the error envelope: %v", rr.Body.String(), err)
	}
	want := JWTErrorResponse{
		Success:   false,
		Message:   autherrors.ToExternalMessage(autherrors.CodeUnknown),
		ErrorCode: autherrors.CodeUnknown,
		RequestID: recoverTestRequestID,
	}
	if body != want {
		t.Errorf("body = %+v, want %+v", body, want)
	}

	h := rr.Header()
	if got := h.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
	if got := h.Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	for _, name := range []string{"Content-Disposition", "Content-Length", "Content-Encoding", "ETag", "Last-Modified"} {
		if got := h.Get(name); got != "" {
			t.Errorf("%s = %q survived onto the error response", name, got)
		}
	}
	if got := h.Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Errorf("Access-Control-Allow-Origin = %q, want it kept so a browser can read the error", got)
	}

	lines := panicLogLines(t, logs)
	if len(lines) != 1 {
		t.Fatalf("%d %s lines, want 1:\n%s", len(lines), panicLogMsg, logs)
	}
	line := lines[0]
	if line["level"] != "ERROR" || line["request_id"] != recoverTestRequestID ||
		line["status"] != float64(http.StatusInternalServerError) || line["response_started"] != false ||
		line["path"] != "/probe" {
		t.Errorf("panic line = %v", line)
	}
	if panicText, _ := line["panic"].(string); !strings.Contains(panicText, "boom") || strings.Contains(panicText, "panic-secret") {
		t.Errorf("panic value = %q, want the message with its secret scrubbed", panicText)
	}
	for _, secret := range []string{"header-secret", "query-secret"} {
		if strings.Contains(logs.String(), secret) {
			t.Errorf("the log carries %q from the request:\n%s", secret, logs)
		}
	}
}

func TestRecoverPanic_AbortsOnceTheResponseStarted(t *testing.T) {
	starts := map[string]func(http.ResponseWriter){
		"status": func(w http.ResponseWriter) { w.WriteHeader(http.StatusOK) },
		"body":   func(w http.ResponseWriter) { _, _ = w.Write([]byte("partial")) },
		"flush":  func(w http.ResponseWriter) { w.(http.Flusher).Flush() },
	}
	for name, start := range starts {
		t.Run(name, func(t *testing.T) {
			rr, logs, repanicked := serveRecoverPanic(t, func(w http.ResponseWriter, _ *http.Request) {
				start(w)
				panic("boom")
			})
			if err, _ := repanicked.(error); err != http.ErrAbortHandler { //nolint:errorlint // net/http compares the sentinel by identity
				t.Fatalf("re-panicked with %v, want the bare http.ErrAbortHandler", repanicked)
			}
			if rr.Code != http.StatusOK || strings.Contains(rr.Body.String(), autherrors.CodeUnknown) {
				t.Errorf("status %d body %q: the error envelope was written onto a started response", rr.Code, rr.Body.String())
			}
			lines := panicLogLines(t, logs)
			if len(lines) != 1 || lines[0]["response_started"] != true || lines[0]["status"] != nil {
				t.Errorf("panic lines = %v, want one with response_started true and no status", lines)
			}
		})
	}
}

// net/http aborts quietly only on the bare sentinel, so a wrapped one is
// re-panicked as the sentinel itself.
func TestRecoverPanic_PassesErrAbortHandlerThroughUnlogged(t *testing.T) {
	values := map[string]any{
		"bare":    http.ErrAbortHandler,
		"wrapped": fmt.Errorf("stream closed: %w", http.ErrAbortHandler),
	}
	for name, value := range values {
		t.Run(name, func(t *testing.T) {
			_, logs, repanicked := serveRecoverPanic(t, func(http.ResponseWriter, *http.Request) {
				panic(value)
			})
			if err, _ := repanicked.(error); err != http.ErrAbortHandler { //nolint:errorlint // net/http compares the sentinel by identity
				t.Fatalf("re-panicked with %v, want the bare http.ErrAbortHandler", repanicked)
			}
			if logs.Len() != 0 {
				t.Errorf("logged for http.ErrAbortHandler:\n%s", logs)
			}
		})
	}
}

// An informational status leaves the final status unsent, so a panic after one
// still answers 500. A real server is used because httptest.ResponseRecorder
// treats a 1xx as the final status.
func TestRecoverPanic_InformationalStatusIsNotAStart(t *testing.T) {
	server := httptest.NewServer(RecoverPanic(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusEarlyHints)
		panic("boom")
	})))
	defer server.Close()

	resp, err := server.Client().Get(server.URL)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusInternalServerError || !strings.Contains(string(body), autherrors.CodeUnknown) {
		t.Errorf("status %d body %q, want 500 with the error envelope", resp.StatusCode, body)
	}
}

// fixedWriter is a ResponseWriter with none of the optional interfaces.
type fixedWriter struct{ header http.Header }

func (w *fixedWriter) Header() http.Header         { return w.header }
func (w *fixedWriter) Write(p []byte) (int, error) { return len(p), nil }
func (w *fixedWriter) WriteHeader(int)             {}

func TestStartTrackingWriter(t *testing.T) {
	t.Run("switching protocols starts the response", func(t *testing.T) {
		w := &startTrackingWriter{ResponseWriter: &fixedWriter{header: http.Header{}}}
		w.WriteHeader(http.StatusSwitchingProtocols)
		if !w.started {
			t.Error("101 left the response unstarted")
		}
	})
	t.Run("an unsupported flush or hijack sends nothing", func(t *testing.T) {
		w := &startTrackingWriter{ResponseWriter: &fixedWriter{header: http.Header{}}}
		w.Flush()
		if _, _, err := w.Hijack(); !errors.Is(err, http.ErrNotSupported) {
			t.Errorf("Hijack error = %v, want http.ErrNotSupported", err)
		}
		if err := w.Push("/x", nil); !errors.Is(err, http.ErrNotSupported) {
			t.Errorf("Push error = %v, want http.ErrNotSupported", err)
		}
		if w.started {
			t.Error("an unsupported flush or hijack marked the response started")
		}
	})
	t.Run("a supported flush reaches the inner writer", func(t *testing.T) {
		rr := httptest.NewRecorder()
		w := &startTrackingWriter{ResponseWriter: rr}
		w.Flush()
		if !rr.Flushed || !w.started {
			t.Errorf("flushed %t started %t, want both", rr.Flushed, w.started)
		}
	})
}
