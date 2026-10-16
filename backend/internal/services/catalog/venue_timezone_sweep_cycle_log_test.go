package catalog

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"

	"psychic-homily-backend/internal/testutil"
)

const timezoneSnapshotRefreshFailed = "could not refresh the timezone name snapshot; the venue-local " +
	"zone guard is running against a possibly stale allowlist"

// A cycle that fails on a database error writes, from runCycle, the inline
// logger.Error record for the snapshot refresh and then the one for the sweep.
func TestVenueTimezoneSweep_FailedCycleLogsAtError(t *testing.T) {
	logger, got := testutil.NewLogCapture(slog.LevelDebug)
	sweep := NewVenueTimezoneSweep(testutil.UnavailableDB(t))
	sweep.logger = logger
	sweep.runCycle(context.Background())

	inline, want := testutil.NewLogCaptureAs(slog.LevelDebug, (*VenueTimezoneSweep).runCycle)
	closed := errors.New("sql: database is closed")
	inline.Error(timezoneSnapshotRefreshFailed, "error", closed)
	inline.Error("venue timezone sweep: cycle failed", "error", closed)
	assert.Equal(t, want.String(), got.String())
}

// A canceled cycle fails both the snapshot refresh and the sweep on the
// context's error, and both log at Info.
func (suite *VenueServiceIntegrationTestSuite) TestVenueTimezoneSweep_CanceledCycleLogsAtInfo() {
	logger, got := testutil.NewLogCapture(slog.LevelDebug)
	sweep := NewVenueTimezoneSweep(suite.db)
	sweep.logger = logger
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	sweep.runCycle(ctx)

	inline, want := testutil.NewLogCaptureAs(slog.LevelDebug, (*VenueTimezoneSweep).runCycle)
	inline.Info(timezoneSnapshotRefreshFailed, "error", context.Canceled)
	inline.Info("venue timezone sweep: cycle failed", "error", context.Canceled)
	suite.Equal(want.String(), got.String())
}
