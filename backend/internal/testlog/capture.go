// Package testlog captures what a unit under test writes to the standard
// logger or to slog, so a test can assert on log CONTENT. Capture takes a
// *testing.T, so non-test code cannot call it without importing testing.
package testlog

import (
	"bytes"
	"log"
	"log/slog"
	"sync"
	"testing"
)

// captureInFlight serializes captures. The loggers Capture redirects are
// process-global, so two overlapping captures would split one test's output
// across both buffers; TryLock turns that into a loud panic instead of a green
// test that asserted against a stream it does not own.
var captureInFlight sync.Mutex

// syncBuf is a buffer safe for a logging goroutine to write to while the
// capturing goroutine reads. The standard logger's own mutex serializes writers
// against SetOutput but does not cover a reader reaching into the buffer.
type syncBuf struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

// Capture redirects BOTH the standard logger and the default slog logger for
// the duration of fn and returns everything either one emitted.
//
// Covering both matters: a caller that reads only log.Printf output returns ""
// the day its subject moves to slog, and a test asserting a secret is ABSENT
// from "" passes vacuously. Redirecting both means the assertion keeps its
// meaning across that port. Assert on a positive marker as well, so an empty
// capture fails instead of passing silently.
//
// The redirects are undone BEFORE the buffer is read, so a goroutine still
// logging after fn returns cannot truncate or race the read. A caller already
// inside a Capture, or running one under t.Parallel() alongside another, panics
// rather than asserting against a stream another test owns.
func Capture(t *testing.T, fn func()) string {
	t.Helper()

	if !captureInFlight.TryLock() {
		panic("testlog.Capture is already in flight: it owns the process-global loggers, so it cannot nest or run under t.Parallel()")
	}
	defer captureInFlight.Unlock()

	var buf syncBuf

	prevSlog := slog.Default()
	prevWriter := log.Writer()
	prevFlags := log.Flags()
	restore := func() {
		// slog.SetDefault re-points the standard logger at the slog handler, so
		// restore it first and let the explicit log.SetOutput below win.
		slog.SetDefault(prevSlog)
		log.SetOutput(prevWriter)
		log.SetFlags(prevFlags)
	}
	defer restore()

	// Same ordering as restore: slog first, then the standard logger, so direct
	// log.Printf calls land in the buffer as raw text rather than slog records.
	// LevelDebug so a debug-level record is not dropped before it is asserted on.
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	log.SetOutput(&buf)
	log.SetFlags(0)

	fn()

	// Read only once the loggers point back at their original destinations.
	restore()
	return buf.String()
}
