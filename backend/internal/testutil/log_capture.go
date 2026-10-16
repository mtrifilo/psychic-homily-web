package testutil

import (
	"bytes"
	"log/slog"
	"reflect"
	"runtime"
)

// NewLogCapture returns a JSON logger with AddSource on, as production logs,
// writing to the returned buffer. Time is dropped and the source line zeroed,
// so two records written from different lines of one function compare byte for
// byte.
func NewLogCapture(level slog.Level) (*slog.Logger, *bytes.Buffer) {
	return newLogCapture(level, nil)
}

// NewLogCaptureAs is NewLogCapture with every record's source replaced by the
// function and file of fn, a function or method expression. A record a test
// writes inline through it compares byte for byte with one fn writes through
// NewLogCapture.
func NewLogCaptureAs(level slog.Level, fn any) (*slog.Logger, *bytes.Buffer) {
	f := runtime.FuncForPC(reflect.ValueOf(fn).Pointer())
	file, _ := f.FileLine(f.Entry())
	return newLogCapture(level, &slog.Source{Function: f.Name(), File: file})
}

func newLogCapture(level slog.Level, source *slog.Source) (*slog.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	handler := slog.NewJSONHandler(buf, &slog.HandlerOptions{
		Level:     level,
		AddSource: true,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			switch a.Key {
			case slog.TimeKey:
				return slog.Attr{}
			case slog.SourceKey:
				if source != nil {
					return slog.Any(slog.SourceKey, source)
				}
				if src, ok := a.Value.Any().(*slog.Source); ok {
					return slog.Any(slog.SourceKey, &slog.Source{Function: src.Function, File: src.File})
				}
			}
			return a
		},
	})
	return slog.New(handler), buf
}
