package testutil

import (
	"context"
	"time"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// CountQueries returns a session of db and a counter that every statement the
// session executes, successful or failed, increments. Paired with
// UnavailableDB it lets a test show that a path answers without querying.
func CountQueries(db *gorm.DB) (*gorm.DB, *int) {
	n := new(int)
	return db.Session(&gorm.Session{
		Logger: queryCounter{Interface: gormlogger.Default.LogMode(gormlogger.Silent), n: n},
	}), n
}

type queryCounter struct {
	gormlogger.Interface
	n *int
}

// LogMode keeps the counter on a session that changes the log level, as
// db.Debug() does.
func (q queryCounter) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	return queryCounter{Interface: q.Interface.LogMode(level), n: q.n}
}

func (q queryCounter) Trace(context.Context, time.Time, func() (string, int64), error) {
	*q.n++
}
