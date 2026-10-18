package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/testutil"
)

// connectReturnSlack is how far past its bound a refused connect may return:
// the bound ends the wait, and closing the pool and the dial follow it.
const connectReturnSlack = 3 * time.Second

// keepGlobalDB restores DB when the test ends, since Connect sets it.
func keepGlobalDB(t *testing.T) {
	t.Helper()
	saved := DB
	t.Cleanup(func() { DB = saved })
}

func connectConfig(t *testing.T, dsn string) *config.Config {
	t.Helper()
	// An empty PGCONNECT_TIMEOUT is unset to pgx, so the DSN alone decides.
	t.Setenv("PGCONNECT_TIMEOUT", "")
	keepGlobalDB(t)
	DB = nil
	return &config.Config{Database: config.DatabaseConfig{URL: dsn, MaxOpenConns: 2, MaxIdleConns: 1}}
}

// connectReturn runs connect and returns how long it took and its error,
// failing the test if it has not returned by limit.
func connectReturn(t *testing.T, limit time.Duration, connect func() error) (time.Duration, error) {
	t.Helper()
	start := time.Now()
	done := make(chan error, 1)
	go func() { done <- connect() }()
	select {
	case err := <-done:
		return time.Since(start), err
	case <-time.After(limit):
		t.Fatalf("connect still waiting after %s against a server that never answers", limit)
		return 0, nil
	}
}

// A server that accepts TCP and never answers is the case boot must refuse
// rather than wait on: without a bound, Connect waits until the operating
// system gives up on the socket.
func TestConnectRefusesUnansweringDatabaseWithinBound(t *testing.T) {
	cfg := connectConfig(t, testutil.UnansweringDSN(t))

	took, err := connectReturn(t, connectTimeout+connectReturnSlack, func() error { return Connect(cfg) })

	require.Error(t, err)
	require.ErrorContains(t, err, "database did not answer within 10s")
	require.GreaterOrEqual(t, took, connectTimeout-time.Second,
		"a refusal well before the bound is some other failure than the bound")
	require.Nil(t, DB, "a refused connect must not leave a handle behind")
}

// The ping bound holds on its own when the DSN turns the dial bound off
// (connect_timeout=0 is libpq's "wait indefinitely").
func TestConnectWithinPingBoundCoversAnUnboundedDial(t *testing.T) {
	cfg := connectConfig(t, testutil.UnansweringDSN(t)+" connect_timeout=0")

	_, err := connectReturn(t, time.Second+connectReturnSlack, func() error { return connectWithin(cfg, time.Second) })

	require.ErrorContains(t, err, "database did not answer within 1s")
}

// An explicit connect_timeout on the DSN is kept and reaches pgx: a shorter one
// ends the wait before the ping bound does, and the refusal names it.
func TestConnectWithinNamesTheShorterDialBound(t *testing.T) {
	cfg := connectConfig(t, testutil.UnansweringDSN(t)+" connect_timeout=1")

	_, err := connectReturn(t, time.Second+connectReturnSlack, func() error { return connectWithin(cfg, 30*time.Second) })

	require.ErrorContains(t, err, "database did not answer within 1s")
}

func TestWithDefaultConnectTimeout(t *testing.T) {
	cases := []struct {
		name string
		dsn  string
		want string
	}{
		{"keyword/value without one", "host=db port=5432 dbname=app", "host=db port=5432 dbname=app connect_timeout=10"},
		{"keyword/value with one", "host=db connect_timeout=3 dbname=app", "host=db connect_timeout=3 dbname=app"},
		{"keyword/value with spaced one", "host=db connect_timeout = 3", "host=db connect_timeout = 3"},
		{"keyword/value with a lookalike key", "host=db xconnect_timeout=3", "host=db xconnect_timeout=3 connect_timeout=10"},
		{"url without a query", "postgres://u:p@db:5432/app", "postgres://u:p@db:5432/app?connect_timeout=10"},
		{"url with a query", "postgres://u:p@db:5432/app?sslmode=disable", "postgres://u:p@db:5432/app?sslmode=disable&connect_timeout=10"},
		{"url with an empty query", "postgres://u:p@db:5432/app?", "postgres://u:p@db:5432/app?connect_timeout=10"},
		{"url with a trailing ampersand", "postgres://u:p@db/app?sslmode=disable&", "postgres://u:p@db/app?sslmode=disable&connect_timeout=10"},
		{"url with one", "postgres://u:p@db/app?connect_timeout=3&sslmode=disable", "postgres://u:p@db/app?connect_timeout=3&sslmode=disable"},
		{"postgresql scheme", "postgresql://u:p@db/app", "postgresql://u:p@db/app?connect_timeout=10"},
		{"url with a fragment", "postgres://u:p@db/app?sslmode=disable#x", "postgres://u:p@db/app?sslmode=disable&connect_timeout=10#x"},
		{"multi-host url", "postgres://u:p@a:5432,b:5432/app", "postgres://u:p@a:5432,b:5432/app?connect_timeout=10"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("PGCONNECT_TIMEOUT", "")
			got := withDefaultConnectTimeout(tc.dsn, connectTimeout)
			require.Equal(t, tc.want, got)

			// The rewritten DSN is one pgx parses, with the bound it says.
			parsed, err := pgconn.ParseConfig(got)
			require.NoError(t, err)
			require.Positive(t, parsed.ConnectTimeout)
		})
	}

	t.Run("PGCONNECT_TIMEOUT is kept", func(t *testing.T) {
		t.Setenv("PGCONNECT_TIMEOUT", "4")
		dsn := "postgres://u:p@db/app"
		require.Equal(t, dsn, withDefaultConnectTimeout(dsn, connectTimeout))
		require.Equal(t, 4*time.Second, effectiveConnectTimeout(dsn))
	})
}

// A healthy server connects as before, through the URL form the deploy uses,
// and the connect bound reaches the pool.
func TestConnectHealthyDatabase(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	ctx := context.Background()
	host, err := td.Container.Host(ctx)
	require.NoError(t, err)
	port, err := td.Container.MappedPort(ctx, "5432")
	require.NoError(t, err)

	dsn := fmt.Sprintf("postgres://test_user:test_password@%s:%s/test_db?sslmode=disable", host, port.Port())
	cfg := connectConfig(t, dsn)

	require.NoError(t, Connect(cfg))
	require.NotNil(t, DB)
	sqlDB, err := DB.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, sqlDB.Ping())
	require.Equal(t, 2, sqlDB.Stats().MaxOpenConnections)

	var one int
	require.NoError(t, DB.Raw("SELECT 1").Scan(&one).Error)
	require.Equal(t, 1, one)
	require.Equal(t, connectTimeout, effectiveConnectTimeout(withDefaultConnectTimeout(dsn, connectTimeout)))
}
