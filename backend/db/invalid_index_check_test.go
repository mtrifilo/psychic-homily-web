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

// runCheckInvalidIndexes runs CheckInvalidIndexes against gormDB and returns
// the log records it wrote, the Sentry events it captured and its refusal.
func runCheckInvalidIndexes(t *testing.T, gormDB *gorm.DB) ([]map[string]any, []*sentry.Event, error) {
	t.Helper()
	return captureCheckInvalidIndexes(t, func(logger *slog.Logger, hub *sentry.Hub) error {
		return CheckInvalidIndexes(gormDB, logger, hub)
	})
}

// captureCheckInvalidIndexes runs check with a JSON logger and a Sentry hub
// that sends nothing, and returns the log records and events it produced and
// its error.
func captureCheckInvalidIndexes(t *testing.T, check func(*slog.Logger, *sentry.Hub) error) ([]map[string]any, []*sentry.Event, error) {
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
	checkErr := check(slog.New(slog.NewJSONHandler(&buf, nil)), hub)

	var logs []map[string]any
	for dec := json.NewDecoder(&buf); dec.More(); {
		var record map[string]any
		require.NoError(t, dec.Decode(&record))
		logs = append(logs, record)
	}
	return logs, events, checkErr
}

// A failed read is reported as a read error, not as a database with no INVALID
// index, and does not refuse boot.
func TestCheckInvalidIndexes_readError(t *testing.T) {
	t.Parallel()

	logs, events, err := runCheckInvalidIndexes(t, testutil.UnavailableDB(t))
	require.NoError(t, err, "an unreadable catalog must not refuse boot")

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
// naming the bound, and does not refuse boot.
func TestCheckInvalidIndexes_unansweringDatabase(t *testing.T) {
	t.Parallel()

	gormDB := testutil.UnansweringDB(t)
	started := time.Now()
	logs, events, err := captureCheckInvalidIndexes(t, func(logger *slog.Logger, hub *sentry.Hub) error {
		return checkInvalidIndexesWithin(gormDB, logger, hub, 200*time.Millisecond)
	})
	assert.Less(t, time.Since(started), 5*time.Second)
	require.NoError(t, err, "an unanswering catalog must not refuse boot")

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

// The refusal names every index with a drop command that uses its quoted
// qualified name, and says any of them is possibly a build in progress.
func TestInvalidIndexesError_namesEachIndexAndItsRecovery(t *testing.T) {
	t.Parallel()

	err := &invalidIndexesError{indexes: []invalidIndex{
		{SchemaName: "public", TableName: "shows", IndexName: "idx_a", QualifiedName: "public.idx_a"},
		{SchemaName: "public", TableName: "Venues", IndexName: "Idx_B", QualifiedName: `public."Idx_B"`},
	}}

	assert.Equal(t,
		`2 INVALID index(es), which queries do not use: public.idx_a on shows, public."Idx_B" on Venues. `+
			`Recover each with DROP INDEX CONCURRENTLY IF EXISTS public.idx_a; `+
			`DROP INDEX CONCURRENTLY IF EXISTS public."Idx_B"; `+
			`then rerun its CREATE INDEX CONCURRENTLY from its migration `+
			`(IF NOT EXISTS keeps an INVALID index, so the drop comes first). `+
			`An index whose concurrent build is still running is INVALID until the build finishes, `+
			`so each of these is possibly a build in progress: if one is, let it finish and restart `+
			`instead of dropping it. Refusing to boot`,
		err.Error())
}
