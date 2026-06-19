package catalog

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/testutil"
)

// RadioSyncSuite exercises RunStationSync's orchestration mechanics against a real
// Postgres (testcontainers) so the advisory lock, the lifecycle/breaker CHECK
// constraints, ON CONFLICT health upserts, and run/error recording are tested for
// real, not mocked. Provider HTTP is avoided: a kexp_api station with zero shows
// runs a clean fetch with no per-show provider calls; a manual-source station
// fails fast in getProvider, exercising the failure path without a network hit.
type RadioSyncSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
	svc    *RadioService
}

func TestRadioSyncSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration suite in -short")
	}
	suite.Run(t, new(RadioSyncSuite))
}

func (s *RadioSyncSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
	s.svc = &RadioService{db: s.db}
}

func (s *RadioSyncSuite) TearDownSuite() { s.testDB.Cleanup() }

func (s *RadioSyncSuite) TearDownTest() {
	sqlDB, err := s.db.DB()
	s.Require().NoError(err)
	for _, tbl := range []string{
		"radio_sync_run_errors", "radio_sync_runs", "radio_station_health",
		"radio_plays", "radio_episodes", "radio_shows", "radio_stations",
	} {
		_, _ = sqlDB.Exec("DELETE FROM " + tbl)
	}
}

func (s *RadioSyncSuite) seedStation(source string) catalogm.RadioStation {
	src := source
	st := catalogm.RadioStation{
		Name:           "Test " + source,
		Slug:           "test-radio-sync-" + source,
		BroadcastType:  catalogm.BroadcastTypeInternet,
		PlaylistSource: &src,
	}
	s.Require().NoError(s.db.Create(&st).Error)
	return st
}

func (s *RadioSyncSuite) runsForStation(stationID uint) []catalogm.RadioSyncRun {
	var runs []catalogm.RadioSyncRun
	s.Require().NoError(s.db.Where("station_id = ?", stationID).Order("id").Find(&runs).Error)
	return runs
}

// A clean fetch (kexp station, no shows) opens one success run with a consistent
// lifecycle (finished_at set) and seeds the health rollup.
func (s *RadioSyncSuite) TestFetchSuccess_WritesRunAndHealth() {
	st := s.seedStation(catalogm.PlaylistSourceKEXP)

	res, err := s.svc.RunStationSync(context.Background(), st.ID, RunStationSyncOpts{
		Mode: catalogm.RadioSyncRunTypeFetch, Trigger: catalogm.RadioSyncRunTriggerScheduled,
	})
	s.Require().NoError(err)
	s.Require().NotNil(res)
	s.NotZero(res.RunID)
	s.NotNil(res.Import)

	runs := s.runsForStation(st.ID)
	s.Require().Len(runs, 1)
	s.Equal(catalogm.RadioSyncRunStatusSuccess, runs[0].Status)
	s.Equal(catalogm.RadioSyncRunTypeFetch, runs[0].RunType)
	s.Equal(catalogm.RadioSyncRunTriggerScheduled, runs[0].Trigger)
	s.Require().NotNil(runs[0].FinishedAt, "terminal run must have finished_at (lifecycle CHECK)")
	s.False(runs[0].BreakerSkipped)

	var health catalogm.RadioStationHealth
	s.Require().NoError(s.db.First(&health, "station_id = ?", st.ID).Error)
	s.Require().NotNil(health.LastSuccessAt)
	s.Equal(0, health.ConsecutiveFailures)
}

// A failing mode (manual source → getProvider error) records a failed run, a
// categorized error row, surfaces the hard error, and bumps consecutive_failures.
func (s *RadioSyncSuite) TestFetchFailure_RecordsCategorizedError() {
	st := s.seedStation(catalogm.PlaylistSourceManual)

	res, err := s.svc.RunStationSync(context.Background(), st.ID, RunStationSyncOpts{
		Mode: catalogm.RadioSyncRunTypeFetch, Trigger: catalogm.RadioSyncRunTriggerScheduled,
	})
	s.Require().Error(err) // hard error surfaced to the caller
	s.Require().NotNil(res)
	s.NotZero(res.RunID)

	runs := s.runsForStation(st.ID)
	s.Require().Len(runs, 1)
	s.Equal(catalogm.RadioSyncRunStatusFailed, runs[0].Status)
	s.Require().NotNil(runs[0].FinishedAt)

	var errs []catalogm.RadioSyncRunError
	s.Require().NoError(s.db.Where("sync_run_id = ?", res.RunID).Find(&errs).Error)
	s.Require().Len(errs, 1)
	s.Equal(catalogm.RadioSyncRunErrorProviderUnreachable, errs[0].Category)

	var health catalogm.RadioStationHealth
	s.Require().NoError(s.db.First(&health, "station_id = ?", st.ID).Error)
	s.Nil(health.LastSuccessAt)
	s.Equal(1, health.ConsecutiveFailures)
}

// Holding the per-station advisory lock on another connection makes RunStationSync
// a no-op that writes no row (single-runner).
func (s *RadioSyncSuite) TestLockContention_NoRow() {
	st := s.seedStation(catalogm.PlaylistSourceKEXP)

	sqlDB, err := s.db.DB()
	s.Require().NoError(err)
	conn, err := sqlDB.Conn(context.Background())
	s.Require().NoError(err)
	defer conn.Close()
	key := fnvHash(fmt.Sprintf("radio_sync:station:%d", st.ID))
	_, err = conn.ExecContext(context.Background(), "SELECT pg_advisory_lock($1)", key)
	s.Require().NoError(err)
	defer conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", key)

	res, err := s.svc.RunStationSync(context.Background(), st.ID, RunStationSyncOpts{
		Mode: catalogm.RadioSyncRunTypeFetch, Trigger: catalogm.RadioSyncRunTriggerScheduled,
	})
	s.Require().NoError(err)
	s.Require().NotNil(res)
	s.True(res.LockContended)
	s.Zero(res.RunID)
	s.Empty(s.runsForStation(st.ID), "contended run must leave no row")
}

// Scheduled honors an open breaker (skipped row); manual bypasses it (runs).
func (s *RadioSyncSuite) TestBreaker_ScheduledHonors_ManualBypasses() {
	st := s.seedStation(catalogm.PlaylistSourceKEXP)
	s.Require().NoError(s.db.Create(&catalogm.RadioStationHealth{
		StationID:    st.ID,
		BreakerState: catalogm.RadioBreakerStateOpen,
	}).Error)

	// Scheduled → skipped + breaker_skipped.
	res, err := s.svc.RunStationSync(context.Background(), st.ID, RunStationSyncOpts{
		Mode: catalogm.RadioSyncRunTypeFetch, Trigger: catalogm.RadioSyncRunTriggerScheduled,
	})
	s.Require().NoError(err)
	s.True(res.Skipped)

	// Manual → bypasses the breaker and runs.
	res2, err := s.svc.RunStationSync(context.Background(), st.ID, RunStationSyncOpts{
		Mode: catalogm.RadioSyncRunTypeFetch, Trigger: catalogm.RadioSyncRunTriggerManual,
	})
	s.Require().NoError(err)
	s.False(res2.Skipped)

	runs := s.runsForStation(st.ID)
	s.Require().Len(runs, 2)
	s.Equal(catalogm.RadioSyncRunStatusSkipped, runs[0].Status)
	s.True(runs[0].BreakerSkipped)
	s.Require().NotNil(runs[0].FinishedAt)
	s.Equal(catalogm.RadioSyncRunStatusSuccess, runs[1].Status)
	s.False(runs[1].BreakerSkipped)
}

// consecutive_failures increments on failure and resets on a later success
// (ON CONFLICT column-set upsert on station_id).
func (s *RadioSyncSuite) TestHealth_FailureThenSuccessResets() {
	st := s.seedStation(catalogm.PlaylistSourceManual) // fails in getProvider

	_, err := s.svc.RunStationSync(context.Background(), st.ID, RunStationSyncOpts{
		Mode: catalogm.RadioSyncRunTypeFetch, Trigger: catalogm.RadioSyncRunTriggerScheduled,
	})
	s.Require().Error(err)
	var h1 catalogm.RadioStationHealth
	s.Require().NoError(s.db.First(&h1, "station_id = ?", st.ID).Error)
	s.Equal(1, h1.ConsecutiveFailures)

	// Flip to a working source (kexp, no shows → clean success) and re-run.
	s.Require().NoError(s.db.Model(&catalogm.RadioStation{}).
		Where("id = ?", st.ID).
		Update("playlist_source", catalogm.PlaylistSourceKEXP).Error)

	_, err = s.svc.RunStationSync(context.Background(), st.ID, RunStationSyncOpts{
		Mode: catalogm.RadioSyncRunTypeFetch, Trigger: catalogm.RadioSyncRunTriggerScheduled,
	})
	s.Require().NoError(err)
	var h2 catalogm.RadioStationHealth
	s.Require().NoError(s.db.First(&h2, "station_id = ?", st.ID).Error)
	s.Equal(0, h2.ConsecutiveFailures, "success must reset the failure counter")
	s.Require().NotNil(h2.LastSuccessAt)
}

// A second sequential run (lock released between) succeeds — the lock is not
// leaked across runs.
func (s *RadioSyncSuite) TestSequentialRuns_LockReleasedBetween() {
	st := s.seedStation(catalogm.PlaylistSourceKEXP)
	for i := 0; i < 2; i++ {
		res, err := s.svc.RunStationSync(context.Background(), st.ID, RunStationSyncOpts{
			Mode: catalogm.RadioSyncRunTypeFetch, Trigger: catalogm.RadioSyncRunTriggerScheduled,
		})
		s.Require().NoError(err)
		s.False(res.LockContended, "run %d should acquire the lock", i)
	}
	s.Len(s.runsForStation(st.ID), 2)
}
