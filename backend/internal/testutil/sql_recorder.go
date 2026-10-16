package testutil

import (
	"context"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm/logger"
)

// SQLRecorder is a GORM logger that records the text of every statement GORM
// executes, in order. Install it with db.Session(&gorm.Session{Logger: r}).
// Transaction BEGIN and COMMIT do not pass through GORM's logger.
type SQLRecorder struct {
	logger.Interface
	mu         sync.Mutex
	statements []string
}

// NewSQLRecorder returns a recorder that otherwise discards log output.
func NewSQLRecorder() *SQLRecorder { return &SQLRecorder{Interface: logger.Discard} }

// LogMode keeps the recorder installed whatever level GORM asks for.
func (r *SQLRecorder) LogMode(logger.LogLevel) logger.Interface { return r }

// Trace records one executed statement.
func (r *SQLRecorder) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.statements = append(r.statements, strings.TrimSpace(sql))
}

// Statements returns the recorded statements, oldest first.
func (r *SQLRecorder) Statements() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.statements...)
}
