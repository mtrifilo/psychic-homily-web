package db

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/getsentry/sentry-go"
	"gorm.io/gorm"
)

// invalidIndexDefinitionLimit is how many characters of an INVALID index's
// definition the ERROR line carries.
const invalidIndexDefinitionLimit = 120

// invalidIndexSentrySource is the value of the "source" tag on the Sentry
// event FlagInvalidIndexes captures, for both an INVALID index and a failed
// read.
const invalidIndexSentrySource = "schema.invalid_index"

// invalidIndex is an index in the connection's current schema whose
// pg_index.indisvalid is false. The planner never uses such an index.
type invalidIndex struct {
	SchemaName string
	TableName  string
	IndexName  string
	Definition string // the first invalidIndexDefinitionLimit characters
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
		        left(pg_get_indexdef(x.indexrelid), ?) AS definition
		   FROM pg_index x
		   JOIN pg_class i ON i.oid = x.indexrelid
		   JOIN pg_class t ON t.oid = x.indrelid
		   JOIN pg_namespace n ON n.oid = i.relnamespace
		  WHERE NOT x.indisvalid AND n.nspname = current_schema()
		  ORDER BY t.relname, i.relname`,
		invalidIndexDefinitionLimit,
	).Scan(&indexes).Error
	if err != nil {
		return nil, err
	}
	return indexes, nil
}

// FlagInvalidIndexes logs one ERROR line per INVALID index in the current
// schema and captures one Sentry event naming them all; with none it logs and
// captures nothing. A failed read is logged and captured as a read error, so
// it is never mistaken for a clean result. It returns nothing: neither outcome
// refuses boot. Call after AssertRequiredSchema in cmd/server.
func FlagInvalidIndexes(gormDB *gorm.DB, logger *slog.Logger, hub *sentry.Hub) {
	indexes, err := readInvalidIndexes(gormDB)
	if err != nil {
		logger.Error("could not read pg_index; INVALID indexes were not checked", "error", err)
		captureInvalidIndexEvent(hub, fmt.Errorf("could not read pg_index; INVALID indexes were not checked: %w", err))
		return
	}
	if len(indexes) == 0 {
		return
	}

	names := make([]string, 0, len(indexes))
	for _, idx := range indexes {
		logger.Error("INVALID index: queries do not use it; drop it and rerun its build",
			"schema", idx.SchemaName,
			"table", idx.TableName,
			"index", idx.IndexName,
			"definition", idx.Definition,
		)
		names = append(names, fmt.Sprintf("%s.%s on %s", idx.SchemaName, idx.IndexName, idx.TableName))
	}
	captureInvalidIndexEvent(hub, fmt.Errorf("%d INVALID index(es): %s", len(indexes), strings.Join(names, ", ")))
}

func captureInvalidIndexEvent(hub *sentry.Hub, err error) {
	hub.WithScope(func(scope *sentry.Scope) {
		scope.SetTag("source", invalidIndexSentrySource)
		hub.CaptureException(err)
	})
}
