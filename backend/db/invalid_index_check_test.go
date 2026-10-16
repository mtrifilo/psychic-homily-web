package db

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/testutil"
)

// runFlagInvalidIndexes runs FlagInvalidIndexes against gormDB and returns the
// log records it wrote and the Sentry events it captured. Returning at all is
// what shows boot continues.
func runFlagInvalidIndexes(t *testing.T, gormDB *gorm.DB) ([]map[string]any, []*sentry.Event) {
	t.Helper()
	return captureFlagInvalidIndexes(t, func(logger *slog.Logger, hub *sentry.Hub) {
		FlagInvalidIndexes(gormDB, logger, hub)
	})
}

// captureFlagInvalidIndexes runs flag with a JSON logger and a Sentry hub that
// sends nothing, and returns the log records and events it produced.
func captureFlagInvalidIndexes(t *testing.T, flag func(*slog.Logger, *sentry.Hub)) ([]map[string]any, []*sentry.Event) {
	t.Helper()

	var events []*sentry.Event
	client, err := sentry.NewClient(sentry.ClientOptions{
		// No DSN: BeforeSend still runs, and nothing leaves the process.
		BeforeSend: func(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
			events = append(events, event)
			return nil
		},
	})
	require.NoError(t, err)
	hub := sentry.NewHub(client, sentry.NewScope())

	var buf bytes.Buffer
	flag(slog.New(slog.NewJSONHandler(&buf, nil)), hub)

	var logs []map[string]any
	for dec := json.NewDecoder(&buf); dec.More(); {
		var record map[string]any
		require.NoError(t, dec.Decode(&record))
		logs = append(logs, record)
	}
	return logs, events
}

// A failed read is reported as a read error, not as a database with no INVALID
// index, and boot continues.
func TestFlagInvalidIndexes_readError(t *testing.T) {
	t.Parallel()

	logs, events := runFlagInvalidIndexes(t, testutil.UnavailableDB(t))

	require.Len(t, logs, 1)
	assert.Equal(t, "ERROR", logs[0]["level"])
	assert.Equal(t, invalidIndexReadErrorMessage, logs[0]["msg"])
	assert.NotEmpty(t, logs[0]["error"])

	require.Len(t, events, 1)
	assert.Equal(t, invalidIndexSentrySource, events[0].Tags["source"])
	// The wrapped read error is its own entry; the outermost error is last.
	exceptions := events[0].Exception
	require.NotEmpty(t, exceptions)
	assert.Contains(t, exceptions[len(exceptions)-1].Value, invalidIndexReadErrorMessage+": ")
}

// A read that does not answer within the bound is reported as a read error
// naming the bound, and boot continues.
func TestFlagInvalidIndexes_unansweringDatabase(t *testing.T) {
	t.Parallel()

	gormDB := unansweringDB(t)
	started := time.Now()
	logs, events := captureFlagInvalidIndexes(t, func(logger *slog.Logger, hub *sentry.Hub) {
		flagInvalidIndexesWithin(gormDB, logger, hub, 200*time.Millisecond)
	})
	assert.Less(t, time.Since(started), 5*time.Second)

	require.Len(t, logs, 1)
	assert.Equal(t, "ERROR", logs[0]["level"])
	assert.Equal(t, invalidIndexReadErrorMessage, logs[0]["msg"])
	assert.Contains(t, logs[0]["error"], "no answer within 200ms: ")

	require.Len(t, events, 1)
	assert.Equal(t, invalidIndexSentrySource, events[0].Tags["source"])
	exceptions := events[0].Exception
	require.NotEmpty(t, exceptions)
	assert.Contains(t, exceptions[len(exceptions)-1].Value, invalidIndexReadErrorMessage+": no answer within 200ms: ")
}
