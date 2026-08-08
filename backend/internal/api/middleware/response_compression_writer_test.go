// This file is IN-package (unlike response_compression_test.go, which must be
// external to import the handlers it mounts) because it tests the unexported
// bodylessEncodingStripper directly. Going through the middleware would not do:
// chi's own compressor implements Flush and Hijack itself, so a handler always
// sees an http.Flusher regardless of whether OUR wrapper forwards anything — an
// end-to-end interface assertion would pass even with the forwarding deleted.
package middleware

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

// capableWriter is a ResponseWriter that implements every optional interface, so
// a failure to forward is observable rather than merely undetectable.
type capableWriter struct {
	*httptest.ResponseRecorder
	flushed  int
	hijacked int
	pushed   int
}

func (w *capableWriter) Flush() { w.flushed++ }

func (w *capableWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	w.hijacked++
	return nil, nil, errors.New("test hijack reached the underlying writer")
}

func (w *capableWriter) Push(string, *http.PushOptions) error {
	w.pushed++
	return nil
}

func newCapableWriter() *capableWriter {
	return &capableWriter{ResponseRecorder: httptest.NewRecorder()}
}

// The wrapper must be transparent to the optional interfaces. chi reaches for
// these by DIRECT TYPE ASSERTION rather than through http.ResponseController, so
// embedding the http.ResponseWriter interface alone silently strips them.
func TestBodylessEncodingStripperForwardsOptionalInterfaces(t *testing.T) {
	underlying := newCapableWriter()
	w := &bodylessEncodingStripper{ResponseWriter: underlying}

	flusher, ok := interface{}(w).(http.Flusher)
	if !ok {
		t.Fatal("wrapper is not an http.Flusher — streaming responses would stop flushing")
	}
	flusher.Flush()
	if underlying.flushed != 1 {
		t.Errorf("Flush reached the underlying writer %d times, want 1", underlying.flushed)
	}

	hijacker, ok := interface{}(w).(http.Hijacker)
	if !ok {
		t.Fatal("wrapper is not an http.Hijacker — websocket-style upgrades would break")
	}
	if _, _, err := hijacker.Hijack(); err == nil {
		t.Error("Hijack returned no error; expected the underlying writer's sentinel")
	}
	if underlying.hijacked != 1 {
		t.Errorf("Hijack reached the underlying writer %d times, want 1", underlying.hijacked)
	}

	pusher, ok := interface{}(w).(http.Pusher)
	if !ok {
		t.Fatal("wrapper is not an http.Pusher")
	}
	if err := pusher.Push("/x", nil); err != nil {
		t.Errorf("Push: %v", err)
	}
	if underlying.pushed != 1 {
		t.Errorf("Push reached the underlying writer %d times, want 1", underlying.pushed)
	}

	if unwrapped := w.Unwrap(); unwrapped != underlying {
		t.Error("Unwrap does not return the underlying writer, so http.ResponseController " +
			"cannot reach capabilities that are not forwarded explicitly")
	}
}

func TestBodylessEncodingStripperBodylessStatuses(t *testing.T) {
	for _, tc := range []struct {
		name          string
		status        int
		wantEncoding  string
		wantBodyBytes int
	}{
		{"304 strips the encoding and discards framing", http.StatusNotModified, "", 0},
		{"204 strips the encoding and discards framing", http.StatusNoContent, "", 0},
		{"200 keeps the encoding and the body", http.StatusOK, "gzip", 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			underlying := newCapableWriter()
			w := &bodylessEncodingStripper{ResponseWriter: underlying}

			// Stand in for what chi's compressor does before calling through.
			w.Header().Set("Content-Encoding", "gzip")
			w.WriteHeader(tc.status)
			// Stand in for the compressor's deferred Close flushing stream framing.
			if _, err := w.Write([]byte("body")); err != nil {
				t.Fatalf("Write: %v", err)
			}

			if got := underlying.Header().Get("Content-Encoding"); got != tc.wantEncoding {
				t.Errorf("Content-Encoding = %q, want %q", got, tc.wantEncoding)
			}
			if got := underlying.Body.Len(); got != tc.wantBodyBytes {
				t.Errorf("body = %d bytes, want %d", got, tc.wantBodyBytes)
			}
			if underlying.Code != tc.status {
				t.Errorf("status = %d, want %d", underlying.Code, tc.status)
			}
		})
	}
}

// Write must report the discarded bytes as accepted. The caller is the
// compressor's deferred Close; a short-write error there would surface as a
// confusing log line about a response that was actually correct.
func TestBodylessEncodingStripperReportsDiscardedWritesAsAccepted(t *testing.T) {
	w := &bodylessEncodingStripper{ResponseWriter: newCapableWriter()}
	w.WriteHeader(http.StatusNotModified)

	n, err := w.Write([]byte("gzip framing"))
	if err != nil {
		t.Errorf("Write returned %v, want nil", err)
	}
	if n != len("gzip framing") {
		t.Errorf("Write reported %d bytes, want %d", n, len("gzip framing"))
	}
}

// Only the FIRST status decides. chi's compressor deliberately propagates repeat
// WriteHeader calls, so without the guard a handler that committed a 200 body and
// then called WriteHeader(304) would start discarding bytes mid-response.
func TestBodylessEncodingStripperOnlyFirstStatusDecides(t *testing.T) {
	underlying := newCapableWriter()
	w := &bodylessEncodingStripper{ResponseWriter: underlying}

	w.Header().Set("Content-Encoding", "gzip")
	w.WriteHeader(http.StatusOK)
	w.WriteHeader(http.StatusNotModified)

	if _, err := w.Write([]byte("still the 200 body")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got := underlying.Body.String(); got != "still the 200 body" {
		t.Errorf("body = %q, want the committed 200 body — a later WriteHeader must not flip the "+
			"writer into discarding", got)
	}
	if got := underlying.Header().Get("Content-Encoding"); got != "gzip" {
		t.Errorf("Content-Encoding = %q, want gzip preserved for the committed 200", got)
	}
}
