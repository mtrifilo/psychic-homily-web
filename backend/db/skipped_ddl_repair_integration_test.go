package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"psychic-homily-backend/internal/testutil"
)

// skippedDDLRepairName is the repair migration's name without its version, so
// the lookup holds when the version is re-stamped.
const skippedDDLRepairName = "repair_skipped_notification_filter_source_and_collection_fork"

func readSkippedDDLRepair(t *testing.T, direction string) string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join("migrations", "*_"+skippedDDLRepairName+"."+direction+".sql"))
	require.NoError(t, err)
	require.Len(t, matches, 1, "exactly one %s migration named %s", direction, skippedDDLRepairName)
	body, err := os.ReadFile(matches[0])
	require.NoError(t, err)
	return string(body)
}

// repairedTablesCatalog is every column, constraint and index of the two
// tables the repair migration restores, rendered so two databases with the
// same definitions render equal. Column position is left out: a column that
// is dropped and added again moves to the end of its table.
func repairedTablesCatalog(t *testing.T, sqlDB *sql.DB) []string {
	t.Helper()
	rows, err := sqlDB.Query(`
		SELECT 'column ' || table_name || '.' || column_name || ' ' || data_type
		       || coalesce('(' || character_maximum_length || ')', '')
		       || ' nullable=' || is_nullable
		       || ' default=' || coalesce(column_default, 'none')
		  FROM information_schema.columns
		 WHERE table_schema = current_schema()
		   AND table_name IN ('notification_filters', 'collections')
		UNION ALL
		SELECT 'constraint ' || conrelid::regclass || '.' || conname || ' ' || pg_get_constraintdef(oid)
		  FROM pg_constraint
		 WHERE conrelid IN ('notification_filters'::regclass, 'collections'::regclass)
		UNION ALL
		SELECT 'index ' || indexdef
		  FROM pg_indexes
		 WHERE schemaname = current_schema()
		   AND tablename IN ('notification_filters', 'collections')
		ORDER BY 1`)
	require.NoError(t, err)
	defer func() { assert.NoError(t, rows.Close()) }()
	var catalog []string
	for rows.Next() {
		var line string
		require.NoError(t, rows.Scan(&line))
		catalog = append(catalog, line)
	}
	require.NoError(t, rows.Err())
	return catalog
}

// The restored definitions, as a database migrated from scratch has them.
var skippedDDLRepairDefinitions = []string{
	"column notification_filters.source character varying(16) nullable=NO default='user'::character varying",
	"constraint notification_filters.notification_filters_source_check " +
		"CHECK (((source)::text = ANY ((ARRAY['user'::character varying, 'managed'::character varying])::text[])))",
	"index CREATE INDEX idx_notification_filters_user_source ON public.notification_filters " +
		"USING btree (user_id, source) WHERE (is_active = true)",
	"column collections.forked_from_collection_id bigint nullable=YES default=none",
	"constraint collections.collections_forked_from_collection_id_fkey " +
		"FOREIGN KEY (forked_from_collection_id) REFERENCES collections(id) ON DELETE SET NULL",
	"index CREATE INDEX idx_collections_forked_from ON public.collections " +
		"USING btree (forked_from_collection_id) WHERE (forked_from_collection_id IS NOT NULL)",
}

// A database whose pointer is past the two source migrations but which lacks
// their DDL refuses boot naming both columns, and the repair migration
// restores them with the definitions a database migrated from scratch has.
// The setup's full migration run has already applied the repair over the
// existing columns, which is the no-op case.
func TestSkippedDDLRepairMigration_integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()
	sqlDB, err := td.DB.DB()
	require.NoError(t, err)

	fromScratch := repairedTablesCatalog(t, sqlDB)
	for _, definition := range skippedDDLRepairDefinitions {
		require.Contains(t, fromScratch, definition)
	}
	require.NoError(t, AssertRequiredSchema(td.DB))

	requireBothColumnsRefused := func() {
		t.Helper()
		err := AssertRequiredSchema(td.DB)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "notification_filters.source")
		assert.Contains(t, err.Error(), "collections.forked_from_collection_id")
		assert.Contains(t, err.Error(), "refusing to boot")
	}
	applyRepair := func() {
		t.Helper()
		_, err := sqlDB.Exec(readSkippedDDLRepair(t, "up"))
		require.NoError(t, err)
	}

	_, err = sqlDB.Exec(`ALTER TABLE notification_filters DROP COLUMN source`)
	require.NoError(t, err)
	_, err = sqlDB.Exec(`ALTER TABLE collections DROP COLUMN forked_from_collection_id`)
	require.NoError(t, err)
	requireBothColumnsRefused()

	applyRepair()
	assert.Equal(t, fromScratch, repairedTablesCatalog(t, sqlDB))
	require.NoError(t, AssertRequiredSchema(td.DB))

	t.Run("applying it again changes nothing", func(t *testing.T) {
		applyRepair()
		assert.Equal(t, fromScratch, repairedTablesCatalog(t, sqlDB))
	})

	t.Run("down removes what it restores and up restores it again", func(t *testing.T) {
		_, err := sqlDB.Exec(readSkippedDDLRepair(t, "down"))
		require.NoError(t, err)
		catalog := repairedTablesCatalog(t, sqlDB)
		for _, definition := range skippedDDLRepairDefinitions {
			require.NotContains(t, catalog, definition)
		}
		requireBothColumnsRefused()

		_, err = sqlDB.Exec(readSkippedDDLRepair(t, "down"))
		require.NoError(t, err, "down is a no-op where the objects are absent")

		applyRepair()
		assert.Equal(t, fromScratch, repairedTablesCatalog(t, sqlDB))
		require.NoError(t, AssertRequiredSchema(td.DB))
	})
}
