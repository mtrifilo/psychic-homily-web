package db

import (
	"context"
	"fmt"
	"net"
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

// connectConfig returns a config for dsn and restores DB when the test ends,
// since Connect sets it.
func connectConfig(t *testing.T, dsn string) *config.Config {
	t.Helper()
	// An empty PGCONNECT_TIMEOUT is unset to pgx, so the DSN alone decides.
	t.Setenv("PGCONNECT_TIMEOUT", "")
	saved := DB
	t.Cleanup(func() { DB = saved })
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
// rather than wait on: nothing else in Connect ends that wait.
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

// An explicit connect_timeout on the DSN is kept and reaches pgx, in both DSN
// forms: a shorter one ends the wait before the ping bound does, and the
// refusal names it.
func TestConnectWithinNamesTheShorterDialBound(t *testing.T) {
	keyword := testutil.UnansweringDSN(t)
	parsed, err := pgconn.ParseConfig(keyword)
	require.NoError(t, err)
	urlForm := fmt.Sprintf("postgres://nobody@127.0.0.1:%d/nothing?sslmode=disable&connect_timeout=1", parsed.Port)

	for name, dsn := range map[string]string{"keyword/value": keyword + " connect_timeout=1", "url": urlForm} {
		t.Run(name, func(t *testing.T) {
			cfg := connectConfig(t, dsn)

			_, err := connectReturn(t, time.Second+connectReturnSlack, func() error { return connectWithin(cfg, 30*time.Second) })

			require.ErrorContains(t, err, "database did not answer within 1s per address (connect_timeout)")
		})
	}
}

func TestWithDefaultConnectTimeout(t *testing.T) {
	cases := []struct {
		name        string
		dsn         string
		want        string
		wantTimeout time.Duration
	}{
		{"keyword/value without one", "host=db port=5432 dbname=app", "connect_timeout=10 host=db port=5432 dbname=app", 10 * time.Second},
		{"keyword/value with one", "host=db connect_timeout=3 dbname=app", "connect_timeout=10 host=db connect_timeout=3 dbname=app", 3 * time.Second},
		{"keyword/value with spaced one", "host=db connect_timeout = 3", "connect_timeout=10 host=db connect_timeout = 3", 3 * time.Second},
		{"keyword/value ending in an empty value", "host=db dbname=", "connect_timeout=10 host=db dbname=", 10 * time.Second},
		{"keyword/value with the key inside a quoted value", "host=db password='a connect_timeout=1 b'", "connect_timeout=10 host=db password='a connect_timeout=1 b'", 10 * time.Second},
		{"url without a query", "postgres://u:p@db:5432/app", "postgres://u:p@db:5432/app?connect_timeout=10", 10 * time.Second},
		{"url with a query", "postgres://u:p@db:5432/app?sslmode=disable", "postgres://u:p@db:5432/app?sslmode=disable&connect_timeout=10", 10 * time.Second},
		{"url with an empty query", "postgres://u:p@db:5432/app?", "postgres://u:p@db:5432/app?connect_timeout=10", 10 * time.Second},
		{"url with a trailing ampersand", "postgres://u:p@db/app?sslmode=disable&", "postgres://u:p@db/app?sslmode=disable&connect_timeout=10", 10 * time.Second},
		{"url with one", "postgres://u:p@db/app?connect_timeout=3&sslmode=disable", "postgres://u:p@db/app?connect_timeout=3&sslmode=disable", 3 * time.Second},
		{"postgresql scheme", "postgresql://u:p@db/app", "postgresql://u:p@db/app?connect_timeout=10", 10 * time.Second},
		{"url with a fragment", "postgres://u:p@db/app?sslmode=disable#x", "postgres://u:p@db/app?sslmode=disable&connect_timeout=10#x", 10 * time.Second},
		{"multi-host url", "postgres://u:p@a:5432,b:5432/app", "postgres://u:p@a:5432,b:5432/app?connect_timeout=10", 10 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("PGCONNECT_TIMEOUT", "")
			got := withDefaultConnectTimeout(tc.dsn, connectTimeout)
			require.Equal(t, tc.want, got)
			require.Equal(t, tc.wantTimeout, effectiveConnectTimeout(got), "the bound pgx reads from the rewritten DSN")
		})
	}

	t.Run("the rewrite leaves every other setting as pgx read it", func(t *testing.T) {
		t.Setenv("PGCONNECT_TIMEOUT", "")
		for _, dsn := range []string{"host=db dbname=", "host=db user=app password=", "host=db password='a connect_timeout=1 b'"} {
			before, err := pgconn.ParseConfig(dsn)
			require.NoError(t, err)
			after, err := pgconn.ParseConfig(withDefaultConnectTimeout(dsn, connectTimeout))
			require.NoError(t, err)
			require.Equal(t, before.Database, after.Database, dsn)
			require.Equal(t, before.User, after.User, dsn)
			require.Equal(t, before.Password, after.Password, dsn)
		}
	})

	t.Run("a blank DSN is left for pgx to read as it would", func(t *testing.T) {
		t.Setenv("PGCONNECT_TIMEOUT", "")
		for _, dsn := range []string{"", "   "} {
			require.Equal(t, dsn, withDefaultConnectTimeout(dsn, connectTimeout))
		}
	})

	t.Run("a sub-second bound rounds up rather than to no bound", func(t *testing.T) {
		t.Setenv("PGCONNECT_TIMEOUT", "")
		require.Equal(t, "connect_timeout=1 host=db", withDefaultConnectTimeout("host=db", 500*time.Millisecond))
	})

	t.Run("PGCONNECT_TIMEOUT is kept", func(t *testing.T) {
		t.Setenv("PGCONNECT_TIMEOUT", "4")
		dsn := "postgres://u:p@db/app"
		require.Equal(t, dsn, withDefaultConnectTimeout(dsn, connectTimeout))
		require.Equal(t, 4*time.Second, effectiveConnectTimeout(dsn))
	})
}

// A database that refuses the connection fails fast with the driver's error,
// not the bound, and leaves no handle behind.
func TestConnectReportsARefusedConnectionWithoutTheBound(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	require.NoError(t, ln.Close())
	cfg := connectConfig(t, fmt.Sprintf("host=127.0.0.1 port=%d user=nobody dbname=nothing sslmode=disable", port))

	took, err := connectReturn(t, connectReturnSlack, func() error { return connectWithin(cfg, 30*time.Second) })

	require.ErrorContains(t, err, "failed to connect to database:")
	require.NotContains(t, err.Error(), "did not answer within")
	require.Less(t, took, connectReturnSlack)
	require.Nil(t, DB)
}

func TestDescribeBound(t *testing.T) {
	require.Equal(t, "none", describeBound(0))
	require.Equal(t, "10s", describeBound(connectTimeout))
}

// A healthy server connects through the URL form the deploy uses, with the
// bounded ping on and the default connect_timeout on the DSN.
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
}
