package shared

import (
	"bytes"
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
)

// failingWriter returns the configured error on every Write call. Used to
// exercise the failure branch where the helpers swallow the write error.
type failingWriter struct {
	err error
}

func (f *failingWriter) Write(_ []byte) (int, error) {
	return 0, f.err
}

func TestSafeWrite_WritesPayload(t *testing.T) {
	rr := httptest.NewRecorder()
	SafeWrite(context.Background(), rr, []byte("hello"))
	if got := rr.Body.String(); got != "hello" {
		t.Errorf("SafeWrite body = %q, want %q", got, "hello")
	}
}

func TestSafeWrite_SwallowsWriteError(t *testing.T) {
	// Helper must not panic or propagate when the underlying writer
	// errors. This is the entire reason the helper exists.
	fw := &failingWriter{err: errors.New("broken pipe")}
	SafeWrite(context.Background(), fw, []byte("payload"))
}

func TestSafeEncode_EncodesJSON(t *testing.T) {
	var buf bytes.Buffer
	SafeEncode(context.Background(), &buf, map[string]int{"a": 1})

	got := strings.TrimSpace(buf.String())
	want := `{"a":1}`
	if got != want {
		t.Errorf("SafeEncode body = %q, want %q", got, want)
	}
}

func TestSafeEncode_SwallowsEncodeError(t *testing.T) {
	// Underlying writer error surfaces through json.Encoder.Encode as
	// the same error; the helper must swallow it.
	fw := &failingWriter{err: errors.New("broken pipe")}
	SafeEncode(context.Background(), fw, map[string]int{"a": 1})
}

func TestSafeEncode_AppendsNewline(t *testing.T) {
	// json.NewEncoder.Encode terminates each value with a newline. We
	// preserve that behavior — some callers (notably one-click
	// unsubscribe responses) have always emitted the trailing newline
	// and changing it would shift the wire format.
	var buf bytes.Buffer
	SafeEncode(context.Background(), &buf, map[string]bool{"ok": true})
	if !strings.HasSuffix(buf.String(), "\n") {
		t.Errorf("SafeEncode output %q missing trailing newline", buf.String())
	}
}
