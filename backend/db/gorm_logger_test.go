package db

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"psychic-homily-backend/internal/testutil"
)

func traceStatement(l logger.Interface, err error) {
	l.Trace(context.Background(), time.Now(), func() (string, int64) { return "SELECT traced_statement", 0 }, err)
}

func TestGormLoggerDropsOnlyCanceledStatements(t *testing.T) {
	for _, debug := range []bool{false, true} {
		t.Run(fmt.Sprintf("debug=%t", debug), func(t *testing.T) {
			var out bytes.Buffer
			l := newGormLogger(&out, debug)

			traceStatement(l, context.Canceled)
			traceStatement(l, fmt.Errorf("query: %w", context.Canceled))
			require.Empty(t, out.String(), "a canceled statement is the caller giving up, not a database error")

			traceStatement(l, errors.New("relation does not exist"))
			require.Contains(t, out.String(), "relation does not exist")
			require.Contains(t, out.String(), "gorm_logger_test.go:",
				"the line names the statement's caller, not the logger")

			out.Reset()
			traceStatement(l, context.DeadlineExceeded)
			require.Contains(t, out.String(), context.DeadlineExceeded.Error(),
				"a deadline cannot tell shutdown from a slow query, so it stays visible")

			out.Reset()
			traceStatement(l, gorm.ErrRecordNotFound)
			if debug {
				require.NotContains(t, out.String(), gorm.ErrRecordNotFound.Error(),
					"debug traces the statement but not as an error")
			} else {
				require.Empty(t, out.String())
			}
		})
	}
}

// LogMode returns a new logger, and gorm.Session and debug mode both go
// through it, so the filter has to carry over.
func TestGormLoggerFilterSurvivesLogMode(t *testing.T) {
	var out bytes.Buffer
	l := newGormLogger(&out, false).LogMode(logger.Info)

	traceStatement(l, context.Canceled)
	require.Empty(t, out.String())

	traceStatement(l, nil)
	require.Contains(t, out.String(), "SELECT traced_statement")
}

// The filter keys on the error the driver actually returns for a statement
// canceled mid-flight, so this cancels one on a real server.
func TestGormLoggerDropsAStatementCanceledMidFlight(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	var out bytes.Buffer
	session := td.DB.Session(&gorm.Session{Logger: newGormLogger(&out, false)})

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(200*time.Millisecond, cancel)
	start := time.Now()
	err := session.WithContext(ctx).Exec("SELECT pg_sleep(30)").Error
	require.ErrorIs(t, err, context.Canceled)
	require.Less(t, time.Since(start), 10*time.Second, "the statement was canceled, not completed")
	require.Empty(t, out.String())

	err = session.Exec("SELECT * FROM table_that_does_not_exist").Error
	require.Error(t, err)
	require.Contains(t, out.String(), "table_that_does_not_exist")
	require.Contains(t, out.String(), "gorm_logger_test.go:")
}
