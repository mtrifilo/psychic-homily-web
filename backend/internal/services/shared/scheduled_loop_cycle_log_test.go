package shared

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newCycleLogCapture returns a JSON logger with AddSource on, as production
// logs, writing to the returned buffer. Time is dropped and the source line
// zeroed so two records written from different lines of one function compare
// byte for byte.
func newCycleLogCapture(level slog.Level) (*slog.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	handler := slog.NewJSONHandler(buf, &slog.HandlerOptions{
		Level:     level,
		AddSource: true,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			switch a.Key {
			case slog.TimeKey:
				return slog.Attr{}
			case slog.SourceKey:
				if src, ok := a.Value.Any().(*slog.Source); ok {
					return slog.Any(slog.SourceKey, &slog.Source{Function: src.Function, File: src.File})
				}
			}
			return a
		},
	})
	return slog.New(handler), buf
}

func TestLogCycleFailure_Level(t *testing.T) {
	cases := []struct {
		name  string
		err   error
		level string
	}{
		{"canceled context logs at Info", context.Canceled, "INFO"},
		{"wrapped canceled context logs at Info", fmt.Errorf("load venues: %w", context.Canceled), "INFO"},
		{"deadline exceeded logs at Error", context.DeadlineExceeded, "ERROR"},
		{"any other error logs at Error", errors.New("database unavailable"), "ERROR"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logger, buf := newCycleLogCapture(slog.LevelDebug)
			LogCycleFailure(logger, "sweep: cycle failed", tc.err)
			assert.Contains(t, buf.String(), `"level":"`+tc.level+`"`)
		})
	}
}

// The helper writes the same record the inline pattern it replaces wrote:
// level, message, attributes in order, and a source naming the calling
// function and file.
func TestLogCycleFailure_MatchesInlinePattern(t *testing.T) {
	for _, err := range []error{context.Canceled, errors.New("database unavailable")} {
		t.Run(err.Error(), func(t *testing.T) {
			logger, inline := newCycleLogCapture(slog.LevelDebug)
			logFn := logger.Error
			if errors.Is(err, context.Canceled) {
				logFn = logger.Info
			}
			logFn("sweep: cycle interrupted", "error", err, "scanned", 7, "set", 2)

			logger, helper := newCycleLogCapture(slog.LevelDebug)
			LogCycleFailure(logger, "sweep: cycle interrupted", err, "scanned", 7, "set", 2)

			require.NotEmpty(t, inline.String())
			assert.Equal(t, inline.String(), helper.String())
			assert.Contains(t, helper.String(), "TestLogCycleFailure_MatchesInlinePattern")
		})
	}
}

func TestLogCycleFailure_RespectsHandlerLevel(t *testing.T) {
	logger, buf := newCycleLogCapture(slog.LevelWarn)
	LogCycleFailure(logger, "sweep: cycle failed", context.Canceled)
	assert.Empty(t, buf.String(), "an Info record is dropped by a Warn handler")

	LogCycleFailure(logger, "sweep: cycle failed", errors.New("database unavailable"))
	assert.Contains(t, buf.String(), `"level":"ERROR"`)
}
