package db

import (
	"context"
	"testing"
	"time"

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

	writer, err := sqlDB.BeginTx(ctx, nil)
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

	require.NoError(t, writer.Rollback())

	var valid bool
	require.NoError(t, sqlDB.QueryRowContext(ctx,
		`SELECT x.indisvalid FROM pg_index x JOIN pg_class i ON i.oid = x.indexrelid
		  WHERE i.relname = $1`, probeIndexName).Scan(&valid))
	require.False(t, valid, "the failed build must leave the index INVALID")
}

// A freshly migrated database has no INVALID index, so it logs and captures
// nothing and boot continues. Each subtest leaves the database with no INVALID
// index again when it passes.
func TestCheckInvalidIndexes(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	logs, events, err := runCheckInvalidIndexes(t, td.DB)
	require.NoError(t, err, "a freshly migrated database must not refuse boot")
	assert.Empty(t, logs, "a freshly migrated database must log nothing")
	assert.Empty(t, events, "a freshly migrated database must capture nothing")

	t.Run("a failed build refuses boot until it is dropped and rebuilt", func(t *testing.T) {
		checkFailedBuildRefusesBoot(t, td.DB)
	})
	t.Run("a running build refuses boot until it finishes", func(t *testing.T) {
		checkRunningBuildRefusesBoot(t, td.DB)
	})
}

// A failed concurrent build leaves an INVALID index that the schema assertion
// passes; CheckInvalidIndexes reports it by name and refuses boot with its
// recovery commands, and the recovery they name clears the refusal.
func checkFailedBuildRefusesBoot(t *testing.T, gormDB *gorm.DB) {
	leaveInvalidIndex(t, gormDB)

	// The schema assertion reads columns only, so it passes over the INVALID index.
	require.NoError(t, AssertRequiredSchema(gormDB))

	var fullDefinition string
	require.NoError(t, gormDB.Raw(
		`SELECT pg_get_indexdef(?::regclass)`, probeIndexName).Scan(&fullDefinition).Error)
	require.Greater(t, len(fullDefinition), invalidIndexDefinitionLimit,
		"the probe's definition must be long enough to show the cut")

	logs, events, err := runCheckInvalidIndexes(t, gormDB)

	var refusal *invalidIndexesError
	require.ErrorAs(t, err, &refusal)
	require.Len(t, refusal.indexes, 1)
	assert.Equal(t, "public."+probeIndexName, refusal.indexes[0].QualifiedName)
	assert.Contains(t, err.Error(), "1 INVALID index(es), which queries do not use: public."+probeIndexName+" on invalid_index_probe.")
	assert.Contains(t, err.Error(), "DROP INDEX CONCURRENTLY IF EXISTS public."+probeIndexName+";")
	assert.Contains(t, err.Error(), "Refusing to boot")

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

	// The recovery the refusal names: drop, then rerun the build without IF NOT EXISTS.
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	_, err = sqlDB.Exec(`DROP INDEX CONCURRENTLY IF EXISTS public.` + probeIndexName)
	require.NoError(t, err)
	_, err = sqlDB.Exec(`CREATE INDEX CONCURRENTLY ` + probeIndexName +
		` ON invalid_index_probe (value) WHERE value > 0 AND value < 1000000`)
	require.NoError(t, err)

	logs, events, err = runCheckInvalidIndexes(t, gormDB)
	require.NoError(t, err, "the recovered index must not refuse boot")
	assert.Empty(t, logs)
	assert.Empty(t, events)
}

// An index whose concurrent build is still running is INVALID until the build
// finishes, so a boot during the build is refused with that index named as
// possibly in progress; once the build finishes, boot continues.
func checkRunningBuildRefusesBoot(t *testing.T, gormDB *gorm.DB) {
	ctx := context.Background()
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)

	const indexName = "invalid_index_running_probe_idx"
	_, err = sqlDB.ExecContext(ctx, `CREATE TABLE invalid_index_running_probe (value integer)`)
	require.NoError(t, err)

	// An open write holds the build in its wait for writers, after the build
	// has committed the index's catalog entry.
	writer, err := sqlDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	writerOpen := true
	defer func() {
		if writerOpen {
			assert.NoError(t, writer.Rollback())
		}
	}()
	_, err = writer.ExecContext(ctx, `INSERT INTO invalid_index_running_probe VALUES (1)`)
	require.NoError(t, err)

	built := make(chan error, 1)
	go func() {
		_, err := sqlDB.ExecContext(ctx,
			`CREATE INDEX CONCURRENTLY `+indexName+` ON invalid_index_running_probe (value)`)
		built <- err
	}()
	require.Eventually(t, func() bool {
		var found int
		err := sqlDB.QueryRowContext(ctx,
			`SELECT count(*) FROM pg_class WHERE relname = $1 AND relkind = 'i'`, indexName).Scan(&found)
		return err == nil && found == 1
	}, 10*time.Second, 20*time.Millisecond, "the build must commit its catalog entry")

	logs, events, err := runCheckInvalidIndexes(t, gormDB)
	var refusal *invalidIndexesError
	require.ErrorAs(t, err, &refusal)
	require.Len(t, refusal.indexes, 1)
	assert.Equal(t, indexName, refusal.indexes[0].IndexName)
	assert.Contains(t, err.Error(), "possibly a build in progress: if one is, let it finish and restart instead of dropping it")
	assert.Len(t, logs, 1)
	assert.Len(t, events, 1)

	require.NoError(t, writer.Rollback())
	writerOpen = false
	select {
	case err := <-built:
		require.NoError(t, err, "the build must finish once the writer is gone")
	case <-time.After(30 * time.Second):
		t.Fatal("the concurrent build did not finish within 30s of the writer ending")
	}

	logs, events, err = runCheckInvalidIndexes(t, gormDB)
	require.NoError(t, err, "a finished build must not refuse boot")
	assert.Empty(t, logs)
	assert.Empty(t, events)
}
