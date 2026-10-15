package db

import (
	"context"
	"testing"

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
	defer writer.Close()
	_, err = writer.ExecContext(ctx, `BEGIN`)
	require.NoError(t, err)
	_, err = writer.ExecContext(ctx, `INSERT INTO invalid_index_probe VALUES (1)`)
	require.NoError(t, err)

	builder, err := sqlDB.Conn(ctx)
	require.NoError(t, err)
	defer builder.Close()
	_, err = builder.ExecContext(ctx, `SET lock_timeout = '200ms'`)
	require.NoError(t, err)
	_, err = builder.ExecContext(ctx,
		`CREATE INDEX CONCURRENTLY `+probeIndexName+` ON invalid_index_probe (value)`)
	require.Error(t, err, "the build must time out waiting for the open writer")
	require.Contains(t, err.Error(), "lock timeout")

	_, err = writer.ExecContext(ctx, `ROLLBACK`)
	require.NoError(t, err)

	var valid bool
	require.NoError(t, sqlDB.QueryRowContext(ctx,
		`SELECT x.indisvalid FROM pg_index x JOIN pg_class i ON i.oid = x.indexrelid
		  WHERE i.relname = $1`, probeIndexName).Scan(&valid))
	require.False(t, valid, "the failed build must leave the index INVALID")
}

func TestInvalidIndex_premise(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	leaveInvalidIndex(t, td.DB)

	// The schema assertion reads columns only, so it passes over the INVALID index.
	require.NoError(t, AssertRequiredSchema(td.DB))
}
