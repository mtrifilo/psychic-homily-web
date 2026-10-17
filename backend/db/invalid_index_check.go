package db

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
	"gorm.io/gorm"
)

// invalidIndexDefinitionLimit is how many characters of an INVALID index's
// definition the ERROR line carries.
const invalidIndexDefinitionLimit = 120

// invalidIndexSentrySource is the value of the "source" tag on the Sentry
// event CheckInvalidIndexes captures, for both an INVALID index and a failed
// read.
const invalidIndexSentrySource = "schema.invalid_index"

// invalidIndexReadErrorMessage is the ERROR line and Sentry message for a
// failed read, which is never reported as a clean result.
const invalidIndexReadErrorMessage = "could not read pg_index; INVALID indexes were not checked"

// invalidIndex is an index in the connection's current schema whose
// pg_index.indisvalid is false. The planner never uses such an index.
type invalidIndex struct {
	SchemaName    string
	TableName     string
	IndexName     string
	QualifiedName string // schema.index, each part quoted where Postgres needs it
	Definition    string // the first invalidIndexDefinitionLimit characters
}

// readInvalidIndexes returns every index in the current schema that is not
// valid, ordered by table and index name. An index whose concurrent build is
// still running is not valid either, so it is returned too.
func readInvalidIndexes(gormDB *gorm.DB) ([]invalidIndex, error) {
	var indexes []invalidIndex
	err := gormDB.Raw(
		`SELECT n.nspname AS schema_name,
		        t.relname AS table_name,
		        i.relname AS index_name,
		        quote_ident(n.nspname) || '.' || quote_ident(i.relname) AS qualified_name,
		        left(pg_get_indexdef(x.indexrelid), ?) AS definition
		   FROM pg_index x
		   JOIN pg_class i ON i.oid = x.indexrelid
		   JOIN pg_class t ON t.oid = x.indrelid
		   JOIN pg_namespace n ON n.oid = i.relnamespace
		  WHERE NOT x.indisvalid AND n.nspname = current_schema()
		  ORDER BY t.relname, i.relname`,
		invalidIndexDefinitionLimit,
	).Scan(&indexes).Error
	return indexes, err
}

// invalidIndexesError is the refusal CheckInvalidIndexes returns. Its message
// names every INVALID index and the commands that recover each one.
type invalidIndexesError struct {
	indexes []invalidIndex // never empty
}

// describeInvalidIndexes names each index as "<QualifiedName> on <table>",
// comma-separated: the form both the refusal and the Sentry event use.
func describeInvalidIndexes(indexes []invalidIndex) string {
	names := make([]string, 0, len(indexes))
	for _, idx := range indexes {
		names = append(names, idx.QualifiedName+" on "+idx.TableName)
	}
	return strings.Join(names, ", ")
}

func (e *invalidIndexesError) Error() string {
	drops := make([]string, 0, len(e.indexes))
	for _, idx := range e.indexes {
		drops = append(drops, "DROP INDEX CONCURRENTLY IF EXISTS "+idx.QualifiedName+";")
	}
	return fmt.Sprintf(
		"%d INVALID index(es), which queries do not use: %s. Recover each with %s "+
			"then rerun its CREATE INDEX CONCURRENTLY from its migration "+
			"(IF NOT EXISTS keeps an INVALID index, so the drop comes first). "+
			"An index whose concurrent build is still running is INVALID until the build "+
			"finishes, so each of these is possibly a build in progress: if one is, let it "+
			"finish and restart instead of dropping it. Refusing to boot",
		len(e.indexes), describeInvalidIndexes(e.indexes), strings.Join(drops, " "))
}

// CheckInvalidIndexes returns an error naming every INVALID index in the
// current schema, after logging one ERROR line per index and capturing one
// Sentry event naming them all; the caller refuses boot on that error. With no
// INVALID index it returns nil and logs and captures nothing. A failed read,
// including one that does not answer within bootSchemaReadTimeout, is logged
// and captured as a read error and returns nil: an unreadable catalog is not
// an INVALID index, so it does not refuse boot.
func CheckInvalidIndexes(gormDB *gorm.DB, logger *slog.Logger, hub *sentry.Hub) error {
	return checkInvalidIndexesWithin(gormDB, logger, hub, bootSchemaReadTimeout)
}

func checkInvalidIndexesWithin(gormDB *gorm.DB, logger *slog.Logger, hub *sentry.Hub, bound time.Duration) error {
	indexes, err := readWithin(bound, func(ctx context.Context) ([]invalidIndex, error) {
		return readInvalidIndexes(gormDB.WithContext(ctx))
	})
	if err != nil {
		logger.Error(invalidIndexReadErrorMessage, "error", err)
		captureInvalidIndexEvent(hub, fmt.Errorf("%s: %w", invalidIndexReadErrorMessage, err))
		return nil
	}
	if len(indexes) == 0 {
		return nil
	}

	for _, idx := range indexes {
		logger.Error("INVALID index: queries do not use it; drop it and rerun its build",
			"schema", idx.SchemaName,
			"table", idx.TableName,
			"index", idx.IndexName,
			"definition", idx.Definition,
		)
	}
	captureInvalidIndexEvent(hub, fmt.Errorf("%d INVALID index(es): %s", len(indexes), describeInvalidIndexes(indexes)))
	return &invalidIndexesError{indexes: indexes}
}

func captureInvalidIndexEvent(hub *sentry.Hub, err error) {
	hub.WithScope(func(scope *sentry.Scope) {
		scope.SetTag("source", invalidIndexSentrySource)
		hub.CaptureException(err)
	})
}
