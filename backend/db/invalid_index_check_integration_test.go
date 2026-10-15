package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/testutil"
)

const probeIndexName = "invalid_index_probe_value_idx"

// leaveInvalidIndex fails a CREATE INDEX CONCURRENTLY on its lock_timeout the
// way a migration does: another session holds an open write on the table, so
// the build commits its catalog entry, then times out waiting for that writer
// and leaves the index behind with indisvalid = false.
func leaveInvalidIndex(t *testing.T, gormDB *gorm.DB) {
	t.Helper()
	ctx := context.Background()
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)

	_, err = sqlDB.ExecContext(ctx, `CREATE TABLE invalid_index_probe (value integer)`)
	require.NoError(t, err)

	writer, err := sqlDB.Conn(ctx)
	require.NoError(t, err)
	defer func() { assert.NoError(t, writer.Close()) }()
	_, err = writer.ExecContext(ctx, `BEGIN`)
	require.NoError(t, err)
	_, err = writer.ExecContext(ctx, `INSERT INTO invalid_index_probe VALUES (1)`)
	require.NoError(t, err)

	builder, err := sqlDB.Conn(ctx)
	require.NoError(t, err)
	defer func() { assert.NoError(t, builder.Close()) }()
	_, err = builder.ExecContext(ctx, `SET lock_timeout = '200ms'`)
	require.NoError(t, err)
	_, err = builder.ExecContext(ctx,
		`CREATE INDEX CONCURRENTLY `+probeIndexName+
			` ON invalid_index_probe (value) WHERE value > 0 AND value < 1000000`)
	require.Error(t, err, "the build must time out waiting for the open writer")
	require.Contains(t, err.Error(), "lock timeout")
	_, err = builder.ExecContext(ctx, `RESET lock_timeout`)
	require.NoError(t, err)

	_, err = writer.ExecContext(ctx, `ROLLBACK`)
	require.NoError(t, err)

	var valid bool
	require.NoError(t, sqlDB.QueryRowContext(ctx,
		`SELECT x.indisvalid FROM pg_index x JOIN pg_class i ON i.oid = x.indexrelid
		  WHERE i.relname = $1`, probeIndexName).Scan(&valid))
	require.False(t, valid, "the failed build must leave the index INVALID")
}

// A failed concurrent build leaves an INVALID index that the schema assertion
// passes; FlagInvalidIndexes reports it by name and boot continues.
func TestFlagInvalidIndexes_failedConcurrentBuild(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	leaveInvalidIndex(t, td.DB)

	// The schema assertion reads columns only, so it passes over the INVALID index.
	require.NoError(t, AssertRequiredSchema(td.DB))

	var fullDefinition string
	require.NoError(t, td.DB.Raw(
		`SELECT pg_get_indexdef(?::regclass)`, probeIndexName).Scan(&fullDefinition).Error)
	require.Greater(t, len(fullDefinition), invalidIndexDefinitionLimit,
		"the probe's definition must be long enough to show the cut")

	logs, events := runFlagInvalidIndexes(t, td.DB)

	require.Len(t, logs, 1)
	assert.Equal(t, "ERROR", logs[0]["level"])
	assert.Equal(t, "public", logs[0]["schema"])
	assert.Equal(t, "invalid_index_probe", logs[0]["table"])
	assert.Equal(t, probeIndexName, logs[0]["index"])
	assert.Equal(t, fullDefinition[:invalidIndexDefinitionLimit], logs[0]["definition"])

	require.Len(t, events, 1)
	assert.Equal(t, invalidIndexSentrySource, events[0].Tags["source"])
	require.Len(t, events[0].Exception, 1)
	assert.Equal(t,
		"1 INVALID index(es): public."+probeIndexName+" on invalid_index_probe",
		events[0].Exception[0].Value)
}

// A freshly migrated database has no INVALID index, so it logs and captures
// nothing.
func TestFlagInvalidIndexes_cleanDatabase(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	indexes, err := readInvalidIndexes(td.DB)
	require.NoError(t, err)
	assert.Empty(t, indexes)

	logs, events := runFlagInvalidIndexes(t, td.DB)
	assert.Empty(t, logs)
	assert.Empty(t, events)
}
