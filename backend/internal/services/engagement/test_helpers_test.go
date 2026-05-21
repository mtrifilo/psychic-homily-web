package engagement

import "gorm.io/gorm"

// stringPtr returns a pointer to a string. Test helper.
func stringPtr(s string) *string { return &s }

// concurrentTestPoolSize caps open connections for the concurrent-toggle
// suites. The cap queues goroutines at the pool (the realistic production
// shape) rather than opening a connection per goroutine and exhausting the
// container's max_connections (53300) — which would mask the unique-violation
// race the tests exist to catch. Sized well under Postgres' default
// max_connections while leaving enough room for INSERTs to interleave.
const concurrentTestPoolSize = 25

// boundTestPool caps the connection pool on a test DB. Used by suites whose
// concurrent tests fire many simultaneous calls; see concurrentTestPoolSize.
func boundTestPool(db *gorm.DB) {
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(concurrentTestPoolSize)
	}
}
