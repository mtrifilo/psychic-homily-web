package testutil

import "testing"

// The counter survives a session that changes the log level, as db.Debug()
// does, and counts statements that fail.
func TestCountQueries_CountsThroughLogModeChange(t *testing.T) {
	db, queries := CountQueries(UnavailableDB(t))

	var n int
	_ = db.Raw("SELECT 1").Scan(&n).Error
	_ = db.Debug().Raw("SELECT 1").Scan(&n).Error

	if *queries != 2 {
		t.Fatalf("counted %d queries, want 2", *queries)
	}
}
