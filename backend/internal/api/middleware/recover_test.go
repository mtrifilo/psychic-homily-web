package middleware

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sentryhttp "github.com/getsentry/sentry-go/http"

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
		h.Set("Cache-Control", "public, max-age=300")
		h.Set("Set-Cookie", "link_intent=; Max-Age=0")
		for _, name := range representationHeaders {
			h.Set(name, "handler value")
		}
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
	for _, name := range representationHeaders {
		if got := h.Get(name); got != "" {
			t.Errorf("%s = %q survived onto the error response", name, got)
		}
	}
	if got := h.Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Errorf("Access-Control-Allow-Origin = %q, want it kept so a browser can read the error", got)
	}
	if got := h.Get("Set-Cookie"); got == "" {
		t.Error("Set-Cookie was dropped, want the handler's cookie kept")
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

// The panic line carries no email address from the panic value and keeps the
// rest of its text.
func TestRecoverPanic_RedactsEmailsFromThePanicLine(t *testing.T) {
	_, logs, _ := serveRecoverPanic(t, func(http.ResponseWriter, *http.Request) {
		panic(fmt.Errorf("lookup failed for %s: %w", "jane.doe+tag@mail.example.com", errors.New("not found")))
	})
	lines := panicLogLines(t, logs)
	if len(lines) != 1 {
		t.Fatalf("%d %s lines, want 1:\n%s", len(lines), panicLogMsg, logs)
	}
	if got, _ := lines[0]["panic"].(string); got != "lookup failed for "+redactedEmail+": not found" {
		t.Errorf("panic value = %q, want the email replaced by %q", got, redactedEmail)
	}
}

func TestScrubPanicValue(t *testing.T) {
	cases := []struct {
		name, value, want string
	}{
		{"bare email", "no user jane@example.com", "no user " + redactedEmail},
		{"key and colon", "Email:jane@example.com", "Email:" + redactedEmail},
		{"struct", "{ID:1 Email:jane@example.com}", "{ID:1 Email:" + redactedEmail + "}"},
		{"quoted", `{"email":"jane@example.com"}`, `{"email":"` + redactedEmail + `"}`},
		{"angle brackets", "Jane <jane@example.com>", "Jane <" + redactedEmail + ">"},
		{"dotless domain", "root@localhost refused", redactedEmail + " refused"},
		{"unicode", "jöhn@exämple.de", redactedEmail},
		{"two emails", "a@b.io, c@d.io", redactedEmail + ", " + redactedEmail},
		{"key and equals", "Email=jane@example.com", redactedEmail},
		{"apostrophe and equals", "o'brien@example.com, jane=doe@example.com", redactedEmail + ", " + redactedEmail},
		{"percent-encoded", "GET /api/users/jane%40example.com failed", "GET /api/users/" + redactedEmail + " failed"},
		{"double percent-encoded", "GET /u/jane%2540example.com", "GET /u/" + redactedEmail},
		{"doubled at sign", "bad input jane@@example.com", "bad input " + redactedEmail},
		{"chained at signs", "a@jane@example.com", redactedEmail},
		{"no email", "runtime error: index out of range [3] with length 2", "runtime error: index out of range [3] with length 2"},
		{"lone at sign", "meet @ noon", "meet @ noon"},
		// The token rules still redact their shapes.
		{"secret key", "boom: password=panic-secret", "boom: password=[REDACTED]"},
		{"bearer", "Bearer abc.def", "Bearer [REDACTED]"},
		{"query", "GET https://h.example/p?token=abc failed", "GET https://h.example/p?[REDACTED] failed"},
		{"url userinfo", "dial postgres://admin:pw@db.internal/app", "dial postgres://[REDACTED]@db.internal/app"},
		{"email as a secret value", "token=jane@example.com", "token=[REDACTED]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := scrubPanicValue(c.value); got != c.want {
				t.Errorf("scrubPanicValue(%q) = %q, want %q", c.value, got, c.want)
			}
		})
	}

	// The email pass never weakens the secret rules: a secret holding an '@',
	// or one whose key follows an email with no delimiter between them, is
	// redacted.
	secrets := map[string]string{
		"password=P@ssw0rd#2024":                               "P@ssw0rd#2024",
		"token=abc@def/SECRETTAIL":                             "SECRETTAIL",
		"api_key=k3y@part;REST_OF_KEY":                         "REST_OF_KEY",
		"login failed: password=hunter2@x:moresecret":          "moresecret",
		"client_secret=s3c@r3t(tail)":                          "s3c",
		`auth_token=x@y\TAIL`:                                  "TAIL",
		"password: P@ss#tail":                                  "tail",
		`{"password":"P@ss word#tail"}`:                        "P@ss word",
		"Authorization: Basic us@er/xyz":                       "xyz",
		"bad form email=jane@example.com&password=hunter2":     "hunter2",
		"body: email=jane%40example.com&api_key=k3ysecret&x=1": "k3ysecret",
		"from jane@example.com&auth_token=tok123 done":         "tok123",
		"user=jane@example.com|token=abc123":                   "abc123",
		"jane@example.com.session=abc123":                      "abc123",
		"jane@Bearer abc123":                                   "abc123",
	}
	for value, secret := range secrets {
		if got := scrubPanicValue(value); strings.Contains(got, secret) {
			t.Errorf("scrubPanicValue(%q) = %q, carries the secret %q", value, got, secret)
		}
	}

	// An email straddling the length cap (observability's sentryValueLimit,
	// 2000 runes) is redacted whole before the cap applies, so no fragment of
	// it survives the cut.
	value := strings.Repeat("x", 1990) + " jane.doe@example.com"
	got := scrubPanicValue(value)
	if strings.Contains(got, "jane") || strings.Contains(got, "example") {
		t.Errorf("scrubPanicValue left part of the capped email: %q", got[len(got)-40:])
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

// hijackableWriter is a fixedWriter whose Hijack succeeds.
type hijackableWriter struct{ fixedWriter }

func (w *hijackableWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) { return nil, nil, nil }

func TestStartTrackingWriter(t *testing.T) {
	t.Run("a successful hijack starts the response", func(t *testing.T) {
		w := &startTrackingWriter{ResponseWriter: &hijackableWriter{fixedWriter{header: http.Header{}}}}
		if _, _, err := w.Hijack(); err != nil || !w.started {
			t.Errorf("Hijack err %v, started %t; want nil and started", err, w.started)
		}
	})
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
	t.Run("a copy into the writer starts the response", func(t *testing.T) {
		for name, inner := range map[string]http.ResponseWriter{
			"inner ReaderFrom":    httptest.NewRecorder(),
			"no inner ReaderFrom": &fixedWriter{header: http.Header{}},
		} {
			w := &startTrackingWriter{ResponseWriter: inner}
			n, err := io.Copy(w, io.LimitReader(strings.NewReader("body"), 4))
			if n != 4 || err != nil || !w.started {
				t.Errorf("%s: copied %d, err %v, started %t; want 4 bytes and started", name, n, err, w.started)
			}
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

// Behind RecoverPanic, the Sentry handler and the response compressor, the
// order production registers them in, a handler can still hijack an HTTP/1
// connection, and a panic after the hijack aborts instead of writing a 500.
func TestRecoverPanic_KeepsHijackThroughTheProductionChain(t *testing.T) {
	hijacked := make(chan error, 1)
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		conn, _, err := http.NewResponseController(w).Hijack()
		hijacked <- err
		if err != nil {
			return
		}
		_, _ = conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: close\r\n\r\nok"))
		_ = conn.Close()
		panic("after hijack")
	})
	chain := RecoverPanic(sentryhttp.New(sentryhttp.Options{Repanic: true}).Handle(ResponseCompression()(handler)))
	server := httptest.NewServer(chain)
	defer server.Close()

	resp, err := server.Client().Get(server.URL)
	if hijackErr := <-hijacked; hijackErr != nil {
		t.Fatalf("Hijack through the chain: %v", hijackErr)
	}
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || string(body) != "ok" {
		t.Errorf("status %d body %q, want the hijacked connection's own 200 ok", resp.StatusCode, body)
	}
}
