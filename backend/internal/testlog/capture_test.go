package testlog

import (
	"bytes"
	"log"
	"log/slog"
	"strings"
	"testing"
)

// TestCaptureCollectsBothLoggers pins that a capture sees output written
// through the standard logger and through slog's default.
func TestCaptureCollectsBothLoggers(t *testing.T) {
	output := Capture(t, func() {
		log.Printf("STDLOG-MARKER")
		slog.Info("SLOG-MARKER")
	})

	for _, want := range []string{"STDLOG-MARKER", "SLOG-MARKER"} {
		if !strings.Contains(output, want) {
			t.Errorf("expected %q in the capture, got:\n%s", want, output)
		}
	}
}

// TestCaptureRestoresGlobals pins the restore ordering. slog.SetDefault
// re-points the standard logger's writer, so restoring slog after the standard
// logger would strand it on the dead capture buffer: every later test's output
// would vanish while every test still passed. Both the default-handler case and
// the case where a real slog handler is already installed are covered, because
// only the latter exercises that re-pointing.
func TestCaptureRestoresGlobals(t *testing.T) {
	t.Run("default_handler", func(t *testing.T) {
		prevSlog := slog.Default()
		prevWriter := log.Writer()
		prevFlags := log.Flags()

		Capture(t, func() { log.Print("ignored") })

		assertGlobalsRestored(t, prevSlog, prevWriter, prevFlags)
	})

	t.Run("real_slog_handler_installed", func(t *testing.T) {
		var sink bytes.Buffer
		installed := slog.New(slog.NewTextHandler(&sink, nil))

		prevSlog := slog.Default()
		prevWriter := log.Writer()
		prevFlags := log.Flags()
		t.Cleanup(func() {
			slog.SetDefault(prevSlog)
			log.SetOutput(prevWriter)
			log.SetFlags(prevFlags)
		})

		slog.SetDefault(installed)
		outerWriter := log.Writer()
		outerFlags := log.Flags()

		Capture(t, func() { log.Print("ignored") })

		assertGlobalsRestored(t, installed, outerWriter, outerFlags)

		// The standard logger must still reach the installed handler's sink.
		sink.Reset()
		log.Print("AFTER-CAPTURE")
		if !strings.Contains(sink.String(), "AFTER-CAPTURE") {
			t.Errorf("the standard logger no longer reaches the installed slog handler; sink:\n%s", sink.String())
		}
	})
}

func assertGlobalsRestored(t *testing.T, wantSlog *slog.Logger, wantWriter any, wantFlags int) {
	t.Helper()

	if slog.Default() != wantSlog {
		t.Error("slog.Default() was not restored")
	}
	if log.Writer() != wantWriter {
		t.Error("log.Writer() was not restored")
	}
	if log.Flags() != wantFlags {
		t.Errorf("log.Flags() = %d, want %d", log.Flags(), wantFlags)
	}
}

// TestCaptureRestoresAfterPanic pins that a panic in fn still restores the
// globals and releases the lock, so one failing test cannot break every test
// that runs after it.
func TestCaptureRestoresAfterPanic(t *testing.T) {
	prevSlog := slog.Default()
	prevWriter := log.Writer()
	prevFlags := log.Flags()

	func() {
		defer func() {
			if recover() == nil {
				t.Error("expected the panic to propagate out of Capture")
			}
		}()
		Capture(t, func() { panic("boom") })
	}()

	assertGlobalsRestored(t, prevSlog, prevWriter, prevFlags)

	// The lock was released, so a later capture still works.
	if output := Capture(t, func() { log.Print("AFTER-PANIC") }); !strings.Contains(output, "AFTER-PANIC") {
		t.Errorf("Capture is unusable after a panic; got:\n%s", output)
	}
}

// TestCaptureRejectsNesting pins that overlapping use panics rather than
// silently splitting one subject's output across two buffers.
func TestCaptureRejectsNesting(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected a nested Capture to panic")
		}
		if msg, ok := r.(string); !ok || !strings.Contains(msg, "already in flight") {
			t.Errorf("expected an already-in-flight panic, got %v", r)
		}
	}()

	Capture(t, func() {
		Capture(t, func() {})
	})
}
