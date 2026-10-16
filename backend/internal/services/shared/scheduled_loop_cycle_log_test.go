package shared

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/testlog"
)

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
			logger, buf := testlog.NewJSONCapture(slog.LevelDebug)
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
			logger, inline := testlog.NewJSONCapture(slog.LevelDebug)
			logFn := logger.Error
			if errors.Is(err, context.Canceled) {
				logFn = logger.Info
			}
			logFn("sweep: cycle interrupted", "error", err, "scanned", 7, "set", 2)

			logger, helper := testlog.NewJSONCapture(slog.LevelDebug)
			LogCycleFailure(logger, "sweep: cycle interrupted", err, "scanned", 7, "set", 2)

			require.NotEmpty(t, inline.String())
			assert.Equal(t, inline.String(), helper.String())
			assert.Contains(t, helper.String(), "TestLogCycleFailure_MatchesInlinePattern")
		})
	}
}

func TestLogCycleFailure_RespectsHandlerLevel(t *testing.T) {
	logger, buf := testlog.NewJSONCapture(slog.LevelWarn)
	LogCycleFailure(logger, "sweep: cycle failed", context.Canceled)
	assert.Empty(t, buf.String(), "an Info record is dropped by a Warn handler")

	LogCycleFailure(logger, "sweep: cycle failed", errors.New("database unavailable"))
	assert.Contains(t, buf.String(), `"level":"ERROR"`)
}

// A statement that a canceled context cuts short fails with an error wrapping
// context.Canceled, so a cycle that shutdown interrupts mid-query logs at Info,
// the level that agrees with the runner recording the cycle as interrupted.
func TestLogCycleFailure_QueryCanceledMidStatementLogsAtInfo(t *testing.T) {
	db, _ := setupRunStore(t)
	sleep := func(tx *gorm.DB) error { return tx.Exec("SELECT pg_sleep(1)").Error }
	cases := map[string]func(*gorm.DB) error{
		"statement":   sleep,
		"transaction": func(tx *gorm.DB) error { return tx.Transaction(sleep) },
	}
	for name, run := range cases {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			const cancelAfter = 50 * time.Millisecond
			timer := time.AfterFunc(cancelAfter, cancel)
			defer timer.Stop()

			start := time.Now()
			err := run(db.WithContext(ctx))
			require.Error(t, err)
			assert.GreaterOrEqual(t, time.Since(start), cancelAfter, "the statement was running when the context ended")
			assert.ErrorIs(t, err, context.Canceled)

			logger, buf := testlog.NewJSONCapture(slog.LevelDebug)
			LogCycleFailure(logger, "sweep: cycle failed", fmt.Errorf("load venues: %w", err))
			assert.Contains(t, buf.String(), `"level":"INFO"`)
		})
	}
}
