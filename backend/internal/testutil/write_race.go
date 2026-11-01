package testutil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// These drive a race against a real database deterministically: a write that
// lands inside a service's window (WithWriteBeforeItsFirst), and a concurrent
// write that must wait on a row lock the service holds
// (StartOnAnotherConnection).

// Hook installs a callback on hooked. BeforeQuery, BeforeCreate and
// BeforeUpdate are the ones these races use.
type Hook func(hooked *gorm.DB, callback func(*gorm.DB)) error

// BeforeQuery fires before every SELECT the handle issues.
func BeforeQuery(hooked *gorm.DB, callback func(*gorm.DB)) error {
	return hooked.Callback().Query().Before("gorm:query").Register("testutil:before_query", callback)
}

// BeforeCreate fires before every INSERT the handle issues.
func BeforeCreate(hooked *gorm.DB, callback func(*gorm.DB)) error {
	return hooked.Callback().Create().Before("gorm:create").Register("testutil:before_create", callback)
}

// BeforeUpdate fires before every UPDATE the handle issues.
func BeforeUpdate(hooked *gorm.DB, callback func(*gorm.DB)) error {
	return hooked.Callback().Update().Before("gorm:update").Register("testutil:before_update", callback)
}

// WithWriteBeforeItsFirst returns a handle on db's connections that runs
// write once, before the first statement any of hooks fires on, and a flag
// reporting whether write ran. write receives the in-flight statement's
// handle. A service built on the returned handle therefore sees write land
// after any check its caller made and before the statement the hook names.
func WithWriteBeforeItsFirst(t testing.TB, db *gorm.DB, write func(inFlight *gorm.DB), hooks ...Hook) (*gorm.DB, *bool) {
	t.Helper()
	sqlDB, err := db.DB()
	require.NoError(t, err)
	hooked, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{TranslateError: true})
	require.NoError(t, err)

	ran := false
	callback := func(inFlight *gorm.DB) {
		if ran {
			return
		}
		ran = true
		write(inFlight)
	}
	for _, hook := range hooks {
		require.NoError(t, hook(hooked, callback))
	}
	return hooked, &ran
}

// StartOnAnotherConnection runs write in a goroutine and returns once write
// has either finished or some statement in db's database is waiting on a
// lock. The returned channel delivers write's result.
func StartOnAnotherConnection(t testing.TB, db *gorm.DB, write func() error) <-chan error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- write() }()

	require.Eventually(t, func() bool {
		if len(done) > 0 {
			return true
		}
		var waiting int64
		err := db.Raw(
			"SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'",
		).Scan(&waiting).Error
		return err == nil && waiting > 0
	}, 10*time.Second, 5*time.Millisecond, "the concurrent write neither finished nor waited on a lock")
	return done
}

// AwaitConcurrentWrite waits for a write StartOnAnotherConnection started and
// returns its result, failing the test when it does not finish.
func AwaitConcurrentWrite(t testing.TB, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(10 * time.Second):
		require.FailNow(t, "the concurrent write did not finish")
		return nil
	}
}
