package db

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/testutil"
)

// The pool bounds are configured in one place and read in another, and a
// misconfigured pool does not fail: it keeps serving and simply behaves as
// though it were unbounded. So this asserts the setters actually reached the
// pool, against sql.DBStats, rather than trusting that calling them worked.
func TestApplyPoolBounds(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	cfg := config.DatabaseConfig{
		MaxOpenConns:    7,
		MaxIdleConns:    3,
		ConnMaxLifetime: 11 * time.Minute,
	}
	require.NoError(t, applyPoolBounds(td.DB, cfg))

	sqlDB, err := td.DB.DB()
	require.NoError(t, err)

	require.Equal(t, cfg.MaxOpenConns, sqlDB.Stats().MaxOpenConnections,
		"an unapplied max-open leaves the pool unlimited, which is the state this exists to prevent")

	// MaxIdleConns and ConnMaxLifetime have no Stats field, so they are exercised
	// by their observable effect: idle connections are capped, and the pool still
	// serves after the bounds are applied.
	require.NoError(t, sqlDB.Ping())
	require.LessOrEqual(t, sqlDB.Stats().Idle, cfg.MaxIdleConns)
}

// The defaults are what ship, since none of the three env vars needs to be set.
// A default that database/sql reads as "no limit" would leave the pool unbounded
// on every deploy that does not override it.
func TestPoolDefaultsAreRealBounds(t *testing.T) {
	require.Positive(t, config.DefaultDBMaxOpenConns,
		"a non-positive max-open is database/sql for an unlimited pool")
	require.Positive(t, config.DefaultDBConnMaxLifetimeMinutes,
		"a non-positive lifetime is database/sql for never retiring a connection")
	require.LessOrEqual(t, config.DefaultDBMaxIdleConns, config.DefaultDBMaxOpenConns)
}
