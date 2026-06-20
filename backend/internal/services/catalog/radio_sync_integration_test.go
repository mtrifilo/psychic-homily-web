package catalog

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

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
	s.Nil(res.Import, "a failed run carries no executor payload")
	s.Nil(res.Discover)

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
	defer func() { _ = conn.Close() }()
	key := fnvHash(fmt.Sprintf("radio_sync:station:%d", st.ID))
	_, err = conn.ExecContext(context.Background(), "SELECT pg_advisory_lock($1)", key)
	s.Require().NoError(err)
	defer func() { _, _ = conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", key) }()

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

// A backfill imports episodes/plays via a mock provider; re-running it over the
// same window must not duplicate radio_plays (idempotent re-import via the
// (episode_id, dedup_key) unique index + ON CONFLICT). Also asserts the backfill
// run persists run_type + the requested window.
func (s *RadioSyncSuite) TestBackfill_IdempotentReimport() {
	st := s.seedStation(catalogm.PlaylistSourceKEXP)
	showExt := "show-ext-1"
	show := catalogm.RadioShow{StationID: st.ID, Name: "Backfill Show", Slug: "backfill-show", ExternalID: &showExt}
	s.Require().NoError(s.db.Create(&show).Error)

	trackA, trackB := "Track A", "Track B"
	s.svc.playlistProviderFactory = func(string) (RadioPlaylistProvider, error) {
		return &mockPlaylistProvider{
			fetchNewEpisodesFn: func(string, time.Time, time.Time) ([]RadioEpisodeImport, error) {
				return []RadioEpisodeImport{{ExternalID: "ep-1", ShowExternalID: showExt, AirDate: "2026-06-15"}}, nil
			},
			fetchPlaylistFn: func(string) ([]RadioPlayImport, error) {
				return []RadioPlayImport{
					{Position: 1, ArtistName: "Artist A", TrackTitle: &trackA},
					{Position: 2, ArtistName: "Artist B", TrackTitle: &trackB},
				}, nil
			},
		}, nil
	}
	defer func() { s.svc.playlistProviderFactory = nil }()

	ws := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	we := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	opts := RunStationSyncOpts{
		Mode: catalogm.RadioSyncRunTypeBackfill, Trigger: catalogm.RadioSyncRunTriggerManual,
		ShowID: &show.ID, WindowStart: &ws, WindowEnd: &we,
	}

	res1, err := s.svc.RunStationSync(context.Background(), st.ID, opts)
	s.Require().NoError(err)
	s.Require().NotNil(res1.Import)
	s.Equal(2, res1.Import.PlaysImported)

	var run catalogm.RadioSyncRun
	s.Require().NoError(s.db.First(&run, res1.RunID).Error)
	s.Equal(catalogm.RadioSyncRunTypeBackfill, run.RunType)
	s.Require().NotNil(run.WindowStart, "backfill run must persist the requested window")
	s.Require().NotNil(run.WindowEnd)

	var afterFirst int64
	s.Require().NoError(s.db.Model(&catalogm.RadioPlay{}).Count(&afterFirst).Error)
	s.Equal(int64(2), afterFirst)

	// Re-run the same window: no duplicate plays.
	_, err = s.svc.RunStationSync(context.Background(), st.ID, opts)
	s.Require().NoError(err)
	var afterSecond int64
	s.Require().NoError(s.db.Model(&catalogm.RadioPlay{}).Count(&afterSecond).Error)
	s.Equal(int64(2), afterSecond, "re-import must not duplicate radio_plays")
}

// The scheduled-cycle skip guard: a lock-contended (no-op) run must NOT touch the
// persistent breaker counter (RunStationSync returns LockContended before opening a
// run or rolling up health). Regression guard for the lock-contention path in
// runFetchCycle now that the breaker lives in radio_station_health (PSY-1140).
func (s *RadioSyncSuite) TestFetchCycle_LockContended_DoesNotResetBreaker() {
	st := s.seedStation(catalogm.PlaylistSourceKEXP)
	// Pre-seed persistent health below the breaker threshold so the gate allows the
	// station (not blocked). A lock-contended no-op must leave this counter intact.
	s.Require().NoError(s.db.Create(&catalogm.RadioStationHealth{
		StationID:           st.ID,
		ConsecutiveFailures: radioCircuitBreakerThreshold - 2,
		BreakerState:        catalogm.RadioBreakerStateClosed,
	}).Error)

	fs := &RadioFetchService{
		radioService: s.svc,
		logger:       testLogger(),
		stopCh:       make(chan struct{}),
	}

	// Hold the station's advisory lock so the cycle's RunStationSync contends.
	sqlDB, err := s.db.DB()
	s.Require().NoError(err)
	conn, err := sqlDB.Conn(context.Background())
	s.Require().NoError(err)
	defer func() { _ = conn.Close() }()
	key := fnvHash(fmt.Sprintf("radio_sync:station:%d", st.ID))
	_, err = conn.ExecContext(context.Background(), "SELECT pg_advisory_lock($1)", key)
	s.Require().NoError(err)
	defer func() { _, _ = conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", key) }()

	fs.runFetchCycle()

	var health catalogm.RadioStationHealth
	s.Require().NoError(s.db.First(&health, "station_id = ?", st.ID).Error)
	s.Equal(radioCircuitBreakerThreshold-2, health.ConsecutiveFailures,
		"a lock-contended no-op run must not touch the persistent failure counter")
	s.Empty(s.runsForStation(st.ID), "contended run writes no row")
}

// Discover routes through the orchestrator: writes a discover-typed run + creates
// the discovered shows + rolls up health (the discover path mirrors fetch and was
// otherwise untested).
func (s *RadioSyncSuite) TestDiscover_WritesRunAndShows() {
	st := s.seedStation(catalogm.PlaylistSourceKEXP)
	s.svc.playlistProviderFactory = func(string) (RadioPlaylistProvider, error) {
		return &mockPlaylistProvider{
			discoverShowsFn: func() ([]RadioShowImport, error) {
				return []RadioShowImport{
					{ExternalID: "show-a", Name: "Show A"},
					{ExternalID: "show-b", Name: "Show B"},
				}, nil
			},
		}, nil
	}
	defer func() { s.svc.playlistProviderFactory = nil }()

	res, err := s.svc.RunStationSync(context.Background(), st.ID, RunStationSyncOpts{
		Mode: catalogm.RadioSyncRunTypeDiscover, Trigger: catalogm.RadioSyncRunTriggerScheduled,
	})
	s.Require().NoError(err)
	s.Require().NotNil(res.Discover)
	s.Equal(2, res.Discover.ShowsDiscovered)
	s.Equal(2, res.Discover.ShowsNew)

	runs := s.runsForStation(st.ID)
	s.Require().Len(runs, 1)
	s.Equal(catalogm.RadioSyncRunTypeDiscover, runs[0].RunType)
	s.Equal(catalogm.RadioSyncRunStatusSuccess, runs[0].Status)
	s.Require().NotNil(runs[0].FinishedAt)

	var showCount int64
	s.Require().NoError(s.db.Model(&catalogm.RadioShow{}).Where("station_id = ?", st.ID).Count(&showCount).Error)
	s.Equal(int64(2), showCount)
}

// Cycle-level Skipped guard: a breaker-open (Skipped) run writes a skipped row but
// must NOT reset the persistent failure counter (a 'skipped' status leaves the
// breaker untouched — only last_run_at moves). Sibling of the lock-contention case.
func (s *RadioSyncSuite) TestFetchCycle_BreakerOpen_DoesNotResetBreaker() {
	st := s.seedStation(catalogm.PlaylistSourceKEXP)
	// Open + a non-zero counter, no trip time → within-cooldown ⇒ blocked. The skip
	// must leave the counter at threshold (proves it does not reset on skip).
	s.Require().NoError(s.db.Create(&catalogm.RadioStationHealth{
		StationID:           st.ID,
		BreakerState:        catalogm.RadioBreakerStateOpen,
		ConsecutiveFailures: radioCircuitBreakerThreshold,
	}).Error)

	fs := &RadioFetchService{
		radioService: s.svc,
		logger:       testLogger(),
		stopCh:       make(chan struct{}),
	}

	fs.runFetchCycle()

	var health catalogm.RadioStationHealth
	s.Require().NoError(s.db.First(&health, "station_id = ?", st.ID).Error)
	s.Equal(radioCircuitBreakerThreshold, health.ConsecutiveFailures,
		"a breaker-skipped run must not reset the persistent failure counter")
	runs := s.runsForStation(st.ID)
	s.Require().Len(runs, 1, "a breaker skip writes a skipped row (unlike lock contention)")
	s.Equal(catalogm.RadioSyncRunStatusSkipped, runs[0].Status)
	s.True(runs[0].BreakerSkipped)
}

// A partial run (imported data + some per-episode error) RESETS consecutive_failures
// — it is not a breaker failure (matches the in-memory PSY-887 posture), so a
// chronically-noisy-but-healthy station never climbs the persistent counter.
func (s *RadioSyncSuite) TestPartial_ResetsConsecutiveFailures() {
	st := s.seedStation(catalogm.PlaylistSourceKEXP)
	showExt := "show-partial"
	show := catalogm.RadioShow{StationID: st.ID, Name: "Partial Show", Slug: "partial-show", ExternalID: &showExt}
	s.Require().NoError(s.db.Create(&show).Error)
	// Pre-seed a non-zero failure count to prove partial RESETS it.
	s.Require().NoError(s.db.Create(&catalogm.RadioStationHealth{
		StationID: st.ID, ConsecutiveFailures: 3, BreakerState: catalogm.RadioBreakerStateClosed,
	}).Error)

	s.svc.playlistProviderFactory = func(string) (RadioPlaylistProvider, error) {
		return &mockPlaylistProvider{
			fetchNewEpisodesFn: func(string, time.Time, time.Time) ([]RadioEpisodeImport, error) {
				return []RadioEpisodeImport{{ExternalID: "ep-p", ShowExternalID: showExt, AirDate: "2026-06-15"}}, nil
			},
			fetchPlaylistFn: func(string) ([]RadioPlayImport, error) {
				return nil, fmt.Errorf("provider 500 boom") // → episode fetch error → partial
			},
		}, nil
	}
	defer func() { s.svc.playlistProviderFactory = nil }()

	ws := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	we := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	res, err := s.svc.RunStationSync(context.Background(), st.ID, RunStationSyncOpts{
		Mode: catalogm.RadioSyncRunTypeBackfill, Trigger: catalogm.RadioSyncRunTriggerManual,
		ShowID: &show.ID, WindowStart: &ws, WindowEnd: &we,
	})
	s.Require().NoError(err)
	s.Equal(catalogm.RadioSyncRunStatusPartial, res.Status)

	var health catalogm.RadioStationHealth
	s.Require().NoError(s.db.First(&health, "station_id = ?", st.ID).Error)
	s.Equal(0, health.ConsecutiveFailures, "a partial run must reset the failure counter")
}

// A panic inside the executor must still terminate the run's trace (close it as
// failed with finished_at set) and re-propagate — guarding the panic-recovery
// defer + failRun so a panicked run never orphans a status=running row.
func (s *RadioSyncSuite) TestExecutorPanic_ClosesRunAsFailed() {
	st := s.seedStation(catalogm.PlaylistSourceKEXP)
	showExt := "show-panic"
	show := catalogm.RadioShow{StationID: st.ID, Name: "Panic Show", Slug: "panic-show", ExternalID: &showExt}
	s.Require().NoError(s.db.Create(&show).Error)
	s.svc.playlistProviderFactory = func(string) (RadioPlaylistProvider, error) {
		return &mockPlaylistProvider{
			fetchNewEpisodesFn: func(string, time.Time, time.Time) ([]RadioEpisodeImport, error) {
				panic("boom inside provider")
			},
		}, nil
	}
	defer func() { s.svc.playlistProviderFactory = nil }()

	s.Require().Panics(func() {
		_, _ = s.svc.RunStationSync(context.Background(), st.ID, RunStationSyncOpts{
			Mode: catalogm.RadioSyncRunTypeFetch, Trigger: catalogm.RadioSyncRunTriggerScheduled,
		})
	}, "an executor panic must re-propagate")

	runs := s.runsForStation(st.ID)
	s.Require().Len(runs, 1)
	s.Equal(catalogm.RadioSyncRunStatusFailed, runs[0].Status, "a panicked run must be closed as failed")
	s.Require().NotNil(runs[0].FinishedAt, "a panicked run must still set finished_at (lifecycle CHECK)")
}

// A cancel that lands mid-backfill must WIN over the run's own terminal close: the
// backfill's progressFn observes status='cancelled' and stops, and the close path's
// WHERE status='running' guard leaves the row 'cancelled' (not overwritten to
// success/partial) with health untouched. This is the regression guard for the
// cancellation design — the close/fail/progress UPDATEs all key on status='running'
// so exactly one of {cancel, close} wins. (PSY-1135, adversarial-review.)
func (s *RadioSyncSuite) TestBackfill_CancelMidRun_WinsOverClose() {
	st := s.seedStation(catalogm.PlaylistSourceKEXP)
	showExt := "cancel-show-ext"
	show := catalogm.RadioShow{StationID: st.ID, Name: "Cancel Show", Slug: "cancel-show", ExternalID: &showExt}
	s.Require().NoError(s.db.Create(&show).Error)

	var runID uint
	track := "Track A"
	s.svc.playlistProviderFactory = func(string) (RadioPlaylistProvider, error) {
		return &mockPlaylistProvider{
			fetchNewEpisodesFn: func(string, time.Time, time.Time) ([]RadioEpisodeImport, error) {
				return []RadioEpisodeImport{{ExternalID: "ep-1", ShowExternalID: showExt, AirDate: "2026-06-15"}}, nil
			},
			// FetchPlaylist runs inside importEpisode, BEFORE the post-episode
			// progressFn. Cancelling here means the very next progressFn check
			// observes status='cancelled' and stops the import — deterministically
			// racing the close path against an already-terminal row.
			fetchPlaylistFn: func(string) ([]RadioPlayImport, error) {
				s.Require().NotZero(runID, "OnRunOpened must set runID before the executor runs")
				s.Require().NoError(s.svc.CancelSyncRun(runID))
				return []RadioPlayImport{{Position: 1, ArtistName: "Artist A", TrackTitle: &track}}, nil
			},
		}, nil
	}
	defer func() { s.svc.playlistProviderFactory = nil }()

	ws := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	we := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	res, err := s.svc.RunStationSync(context.Background(), st.ID, RunStationSyncOpts{
		Mode: catalogm.RadioSyncRunTypeBackfill, Trigger: catalogm.RadioSyncRunTriggerManual,
		ShowID: &show.ID, WindowStart: &ws, WindowEnd: &we,
		OnRunOpened: func(id uint) { runID = id },
	})
	s.Require().NoError(err)
	s.Require().NotNil(res)
	s.Equal(catalogm.RadioSyncRunStatusCancelled, res.Status, "close path must not overwrite the cancel")

	var run catalogm.RadioSyncRun
	s.Require().NoError(s.db.First(&run, res.RunID).Error)
	s.Equal(catalogm.RadioSyncRunStatusCancelled, run.Status)
	s.Require().NotNil(run.FinishedAt)

	// The cancelled branch returns before updateStationHealth, so a cancelled run
	// is health-neutral: no success recorded, not counted as a failure.
	var health catalogm.RadioStationHealth
	if herr := s.db.First(&health, "station_id = ?", st.ID).Error; herr == nil {
		s.Nil(health.LastSuccessAt, "a cancelled run must not record a success")
		s.Equal(0, health.ConsecutiveFailures, "a cancelled run is not a failure")
	}
}

// runAutoBackfillShow cancels its in-flight run on service shutdown (s.stopCh) and
// joins the watcher (<-watcherExited) before returning, so no goroutine does DB
// work past Stop()'s WaitGroup barrier. A blocking provider holds the run open so
// the cancel is observed mid-run, deterministically (no sleeps). Replaces the
// coverage of the deleted waitForJobCompletion shutdown test. (PSY-1135.)
func (s *RadioSyncSuite) TestAutoBackfillShow_CancelsOnShutdown() {
	st := s.seedStation(catalogm.PlaylistSourceKEXP)
	showExt := "shutdown-show-ext"
	show := catalogm.RadioShow{StationID: st.ID, Name: "Shutdown Show", Slug: "shutdown-show", ExternalID: &showExt}
	s.Require().NoError(s.db.Create(&show).Error)

	fs := &RadioFetchService{
		radioService: s.svc,
		logger:       testLogger(),
		stopCh:       make(chan struct{}),
	}

	runStarted := make(chan struct{})
	proceed := make(chan struct{})
	var once sync.Once
	track := "T"
	s.svc.playlistProviderFactory = func(string) (RadioPlaylistProvider, error) {
		return &mockPlaylistProvider{
			fetchNewEpisodesFn: func(string, time.Time, time.Time) ([]RadioEpisodeImport, error) {
				return []RadioEpisodeImport{{ExternalID: "ep-1", ShowExternalID: showExt, AirDate: "2026-06-15"}}, nil
			},
			// Block the run open (mid-episode) until the test has closed stopCh and
			// confirmed the watcher cancelled the run.
			fetchPlaylistFn: func(string) ([]RadioPlayImport, error) {
				once.Do(func() { close(runStarted); <-proceed })
				return []RadioPlayImport{{Position: 1, ArtistName: "A", TrackTitle: &track}}, nil
			},
		}, nil
	}
	defer func() { s.svc.playlistProviderFactory = nil }()

	since := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	var res *RunStationSyncResult
	done := make(chan struct{})
	go func() {
		res = fs.runAutoBackfillShow(st.ID, show.ID, since, until)
		close(done)
	}()

	<-runStarted     // run row open + executing, blocked inside FetchPlaylist
	close(fs.stopCh) // watcher (holds runID via OnRunOpened) cancels the run
	s.Require().Eventually(func() bool {
		var run catalogm.RadioSyncRun
		if err := s.db.Where("station_id = ?", st.ID).Order("id DESC").First(&run).Error; err != nil {
			return false
		}
		return run.Status == catalogm.RadioSyncRunStatusCancelled
	}, 3*time.Second, 10*time.Millisecond, "watcher must cancel the in-flight run on stopCh")
	close(proceed) // let the episode finish; progressFn observes the cancel → stops

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		s.Require().FailNow("runAutoBackfillShow did not return after the shutdown cancel (watcher not joined?)")
	}
	s.Require().NotNil(res)
	s.Equal(catalogm.RadioSyncRunStatusCancelled, res.Status)
}

// ───────────────────── persistent breaker state machine (PSY-1140) ─────────────────────

// The breaker opens after radioCircuitBreakerThreshold consecutive PERMANENT
// failures (a manual-source station fails in getProvider, classified permanent),
// recording breaker_tripped_at. End-to-end through RunStationSync's health write-back.
func (s *RadioSyncSuite) TestBreaker_OpensAfterThresholdPermanentFailures() {
	st := s.seedStation(catalogm.PlaylistSourceManual) // getProvider error → permanent

	for i := 0; i < radioCircuitBreakerThreshold; i++ {
		_, _ = s.svc.RunStationSync(context.Background(), st.ID, RunStationSyncOpts{
			Mode: catalogm.RadioSyncRunTypeFetch, Trigger: catalogm.RadioSyncRunTriggerScheduled,
		})
	}

	var health catalogm.RadioStationHealth
	s.Require().NoError(s.db.First(&health, "station_id = ?", st.ID).Error)
	s.Equal(radioCircuitBreakerThreshold, health.ConsecutiveFailures)
	s.Equal(catalogm.RadioBreakerStateOpen, health.BreakerState, "breaker opens at the threshold")
	s.Require().NotNil(health.BreakerTrippedAt, "an opened breaker records when it tripped")
}

// The headline AC: a tripped breaker survives a process restart because it is pure
// DB state now. A BRAND-NEW service instance (no in-memory carry-over — the old
// PSY-887 map would be empty and let the station run) must still skip the station.
func (s *RadioSyncSuite) TestBreaker_PersistsAcrossRestart() {
	st := s.seedStation(catalogm.PlaylistSourceKEXP)
	trippedRecently := time.Now().Add(-time.Minute) // well within the cooldown
	s.Require().NoError(s.db.Create(&catalogm.RadioStationHealth{
		StationID:           st.ID,
		BreakerState:        catalogm.RadioBreakerStateOpen,
		ConsecutiveFailures: radioCircuitBreakerThreshold,
		BreakerTrippedAt:    &trippedRecently,
	}).Error)

	fs := &RadioFetchService{radioService: s.svc, logger: testLogger(), stopCh: make(chan struct{})}
	fs.runFetchCycle()

	runs := s.runsForStation(st.ID)
	s.Require().Len(runs, 1)
	s.Equal(catalogm.RadioSyncRunStatusSkipped, runs[0].Status, "a tripped breaker must survive a restart")
	s.True(runs[0].BreakerSkipped)
}

// open → half_open → closed: past the cooldown, a scheduled run is allowed as a
// half-open trial; a successful trial closes the breaker and clears the trip time.
func (s *RadioSyncSuite) TestBreaker_HalfOpenTrial_SuccessCloses() {
	st := s.seedStation(catalogm.PlaylistSourceKEXP)
	trippedLongAgo := time.Now().Add(-radioBreakerCooldown - time.Minute)
	s.Require().NoError(s.db.Create(&catalogm.RadioStationHealth{
		StationID:           st.ID,
		BreakerState:        catalogm.RadioBreakerStateOpen,
		ConsecutiveFailures: radioCircuitBreakerThreshold,
		BreakerTrippedAt:    &trippedLongAgo,
	}).Error)

	res, err := s.svc.RunStationSync(context.Background(), st.ID, RunStationSyncOpts{
		Mode: catalogm.RadioSyncRunTypeFetch, Trigger: catalogm.RadioSyncRunTriggerScheduled,
	})
	s.Require().NoError(err)
	s.False(res.Skipped, "past cooldown the breaker allows a half-open trial")
	s.Equal(catalogm.RadioSyncRunStatusSuccess, res.Status)

	var health catalogm.RadioStationHealth
	s.Require().NoError(s.db.First(&health, "station_id = ?", st.ID).Error)
	s.Equal(catalogm.RadioBreakerStateClosed, health.BreakerState, "a successful trial closes the breaker")
	s.Equal(0, health.ConsecutiveFailures)
	s.Nil(health.BreakerTrippedAt, "closing clears breaker_tripped_at")
}

// half_open → open: a failed half-open trial re-opens the breaker with a fresh
// cooldown (so the next trial waits another full cooldown).
func (s *RadioSyncSuite) TestBreaker_HalfOpenTrial_FailureReopens() {
	st := s.seedStation(catalogm.PlaylistSourceManual) // permanent fail in getProvider
	trippedLongAgo := time.Now().Add(-radioBreakerCooldown - time.Minute)
	s.Require().NoError(s.db.Create(&catalogm.RadioStationHealth{
		StationID:           st.ID,
		BreakerState:        catalogm.RadioBreakerStateOpen,
		ConsecutiveFailures: radioCircuitBreakerThreshold,
		BreakerTrippedAt:    &trippedLongAgo,
	}).Error)

	_, err := s.svc.RunStationSync(context.Background(), st.ID, RunStationSyncOpts{
		Mode: catalogm.RadioSyncRunTypeFetch, Trigger: catalogm.RadioSyncRunTriggerScheduled,
	})
	s.Require().Error(err) // the trial failed

	var health catalogm.RadioStationHealth
	s.Require().NoError(s.db.First(&health, "station_id = ?", st.ID).Error)
	s.Equal(catalogm.RadioBreakerStateOpen, health.BreakerState, "a failed trial re-opens the breaker")
	s.Equal(radioCircuitBreakerThreshold+1, health.ConsecutiveFailures)
	s.Require().NotNil(health.BreakerTrippedAt)
	s.True(health.BreakerTrippedAt.After(trippedLongAgo), "re-open refreshes breaker_tripped_at")
}

// Manual-probe policy (LOCKED): a manual run bypasses the gate AND a successful
// manual probe CLOSES the breaker (the asymmetric half-open-probe semantics).
func (s *RadioSyncSuite) TestBreaker_ManualProbe_SuccessCloses() {
	st := s.seedStation(catalogm.PlaylistSourceKEXP)
	trippedRecently := time.Now().Add(-time.Minute) // within cooldown — irrelevant, manual bypasses
	s.Require().NoError(s.db.Create(&catalogm.RadioStationHealth{
		StationID:           st.ID,
		BreakerState:        catalogm.RadioBreakerStateOpen,
		ConsecutiveFailures: radioCircuitBreakerThreshold,
		BreakerTrippedAt:    &trippedRecently,
	}).Error)

	res, err := s.svc.RunStationSync(context.Background(), st.ID, RunStationSyncOpts{
		Mode: catalogm.RadioSyncRunTypeFetch, Trigger: catalogm.RadioSyncRunTriggerManual,
	})
	s.Require().NoError(err)
	s.False(res.Skipped, "manual bypasses an open breaker (operator override)")

	var health catalogm.RadioStationHealth
	s.Require().NoError(s.db.First(&health, "station_id = ?", st.ID).Error)
	s.Equal(catalogm.RadioBreakerStateClosed, health.BreakerState, "a manual success closes the breaker")
	s.Equal(0, health.ConsecutiveFailures)
}

// Manual-probe policy (LOCKED): a manual FAILURE never increments the counter / trips
// the breaker — the operator chose to poke a known-bad station.
func (s *RadioSyncSuite) TestBreaker_ManualProbe_FailureDoesNotTrip() {
	st := s.seedStation(catalogm.PlaylistSourceManual) // permanent fail
	s.Require().NoError(s.db.Create(&catalogm.RadioStationHealth{
		StationID:           st.ID,
		BreakerState:        catalogm.RadioBreakerStateClosed,
		ConsecutiveFailures: radioCircuitBreakerThreshold - 1, // one short of tripping
	}).Error)

	_, err := s.svc.RunStationSync(context.Background(), st.ID, RunStationSyncOpts{
		Mode: catalogm.RadioSyncRunTypeFetch, Trigger: catalogm.RadioSyncRunTriggerManual,
	})
	s.Require().Error(err)

	var health catalogm.RadioStationHealth
	s.Require().NoError(s.db.First(&health, "station_id = ?", st.ID).Error)
	s.Equal(catalogm.RadioBreakerStateClosed, health.BreakerState, "a manual failure must not trip the breaker")
	s.Equal(radioCircuitBreakerThreshold-1, health.ConsecutiveFailures, "a manual failure must not increment the counter")
}

// markBreakerHalfOpen flips a tripped breaker to half_open AND refreshes
// breaker_tripped_at to the trial-start time (the timestamp refresh is what bounds a
// stranded trial — adversarial-review fix). Direct method test: the half_open state
// only exists in the DB mid-run, so the end-to-end trial tests can't observe it.
func (s *RadioSyncSuite) TestMarkBreakerHalfOpen_StampsTrialStart() {
	st := s.seedStation(catalogm.PlaylistSourceKEXP)
	trippedLongAgo := time.Now().Add(-radioBreakerCooldown - time.Hour)
	s.Require().NoError(s.db.Create(&catalogm.RadioStationHealth{
		StationID:           st.ID,
		BreakerState:        catalogm.RadioBreakerStateOpen,
		ConsecutiveFailures: radioCircuitBreakerThreshold,
		BreakerTrippedAt:    &trippedLongAgo,
	}).Error)

	s.svc.markBreakerHalfOpen(st.ID)

	var health catalogm.RadioStationHealth
	s.Require().NoError(s.db.First(&health, "station_id = ?", st.ID).Error)
	s.Equal(catalogm.RadioBreakerStateHalfOpen, health.BreakerState, "open → half_open at trial start")
	s.Require().NotNil(health.BreakerTrippedAt)
	s.True(health.BreakerTrippedAt.After(trippedLongAgo), "trial start refreshes breaker_tripped_at")
}

// A breaker STRANDED at half_open (a trial that was cancelled/panicked so
// updateStationHealth never resolved it) must NOT re-trial every cycle: with a
// recent breaker_tripped_at it is gate-blocked for a full cooldown, exactly like an
// open breaker. Regression guard for the cooldown-defeat the adversarial review found.
func (s *RadioSyncSuite) TestBreaker_StrandedHalfOpen_RespectsCooldown() {
	st := s.seedStation(catalogm.PlaylistSourceKEXP)
	trippedRecently := time.Now().Add(-time.Minute) // trial started a minute ago, never resolved
	s.Require().NoError(s.db.Create(&catalogm.RadioStationHealth{
		StationID:           st.ID,
		BreakerState:        catalogm.RadioBreakerStateHalfOpen,
		ConsecutiveFailures: radioCircuitBreakerThreshold,
		BreakerTrippedAt:    &trippedRecently,
	}).Error)

	res, err := s.svc.RunStationSync(context.Background(), st.ID, RunStationSyncOpts{
		Mode: catalogm.RadioSyncRunTypeFetch, Trigger: catalogm.RadioSyncRunTriggerScheduled,
	})
	s.Require().NoError(err)
	s.True(res.Skipped, "a stranded half_open within cooldown must be blocked, not re-trialed")

	runs := s.runsForStation(st.ID)
	s.Require().Len(runs, 1)
	s.Equal(catalogm.RadioSyncRunStatusSkipped, runs[0].Status)
	s.True(runs[0].BreakerSkipped)
}

// Per-station isolation: wedging one station's breaker (permanent failures to the
// threshold) must not touch another station's health row. Now a schema property
// (station_id PK) rather than an in-memory map; this guards a future query that
// drops the station_id predicate.
func (s *RadioSyncSuite) TestBreaker_PerStationIsolation() {
	wedged := s.seedStation(catalogm.PlaylistSourceManual) // permanent fail in getProvider
	healthy := s.seedStation(catalogm.PlaylistSourceKEXP)  // clean fetch (no shows)

	// Drive the wedged station to the breaker threshold.
	for i := 0; i < radioCircuitBreakerThreshold; i++ {
		_, _ = s.svc.RunStationSync(context.Background(), wedged.ID, RunStationSyncOpts{
			Mode: catalogm.RadioSyncRunTypeFetch, Trigger: catalogm.RadioSyncRunTriggerScheduled,
		})
	}
	// Run the healthy station once (success).
	_, err := s.svc.RunStationSync(context.Background(), healthy.ID, RunStationSyncOpts{
		Mode: catalogm.RadioSyncRunTypeFetch, Trigger: catalogm.RadioSyncRunTriggerScheduled,
	})
	s.Require().NoError(err)

	var wh catalogm.RadioStationHealth
	s.Require().NoError(s.db.First(&wh, "station_id = ?", wedged.ID).Error)
	s.Equal(catalogm.RadioBreakerStateOpen, wh.BreakerState, "wedged station opens")
	s.Equal(radioCircuitBreakerThreshold, wh.ConsecutiveFailures)

	var hh catalogm.RadioStationHealth
	s.Require().NoError(s.db.First(&hh, "station_id = ?", healthy.ID).Error)
	s.Equal(catalogm.RadioBreakerStateClosed, hh.BreakerState, "the other station is unaffected")
	s.Equal(0, hh.ConsecutiveFailures)
}

// ───────────────────── typed errors + Sentry escalation (PSY-1141) ─────────────────────

// A PERMANENT failure on a scheduled run escalates via the onPermanentFailure seam;
// the SAME permanent failure on a manual run does NOT (the operator already sees the
// result). End-to-end through RunStationSync's escalation hook.
func (s *RadioSyncSuite) TestEscalation_PermanentScheduledFires_ManualDoesNot() {
	st := s.seedStation(catalogm.PlaylistSourceManual) // getProvider error → permanent hard failure

	var categories []string
	s.svc.onPermanentFailure = func(_ error, _ uint, category string) {
		categories = append(categories, category)
	}
	defer func() { s.svc.onPermanentFailure = nil }()

	_, err := s.svc.RunStationSync(context.Background(), st.ID, RunStationSyncOpts{
		Mode: catalogm.RadioSyncRunTypeFetch, Trigger: catalogm.RadioSyncRunTriggerScheduled,
	})
	s.Require().Error(err)
	s.Require().Len(categories, 1, "a permanent scheduled failure must escalate")
	s.Equal(catalogm.RadioSyncRunErrorProviderUnreachable, categories[0])

	categories = nil
	_, err = s.svc.RunStationSync(context.Background(), st.ID, RunStationSyncOpts{
		Mode: catalogm.RadioSyncRunTypeFetch, Trigger: catalogm.RadioSyncRunTriggerManual,
	})
	s.Require().Error(err)
	s.Empty(categories, "a manual failure must NOT escalate")
}

// End-to-end: a truncated play records a radio_sync_run_errors row categorized as
// 'truncation' — the case the old string heuristic could never reach (it always
// bucketed drop-summaries as validation_drop). The headline PSY-1141 AC.
func (s *RadioSyncSuite) TestBackfill_TruncationRecordsTruncationCategory() {
	st := s.seedStation(catalogm.PlaylistSourceKEXP)
	showExt := "trunc-show"
	show := catalogm.RadioShow{StationID: st.ID, Name: "Trunc Show", Slug: "trunc-show", ExternalID: &showExt}
	s.Require().NoError(s.db.Create(&show).Error)

	overLength := strings.Repeat("z", 600) // > 500-rune column → salvaged via truncation
	s.svc.playlistProviderFactory = func(string) (RadioPlaylistProvider, error) {
		return &mockPlaylistProvider{
			fetchNewEpisodesFn: func(string, time.Time, time.Time) ([]RadioEpisodeImport, error) {
				return []RadioEpisodeImport{{ExternalID: "ep-1", ShowExternalID: showExt, AirDate: "2026-06-15"}}, nil
			},
			fetchPlaylistFn: func(string) ([]RadioPlayImport, error) {
				return []RadioPlayImport{{Position: 1, ArtistName: overLength}}, nil
			},
		}, nil
	}
	defer func() { s.svc.playlistProviderFactory = nil }()

	ws := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	we := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	res, err := s.svc.RunStationSync(context.Background(), st.ID, RunStationSyncOpts{
		Mode: catalogm.RadioSyncRunTypeBackfill, Trigger: catalogm.RadioSyncRunTriggerManual,
		ShowID: &show.ID, WindowStart: &ws, WindowEnd: &we,
	})
	s.Require().NoError(err)
	s.Equal(catalogm.RadioSyncRunStatusPartial, res.Status, "a truncation makes the run partial")

	var errs []catalogm.RadioSyncRunError
	s.Require().NoError(s.db.Where("sync_run_id = ?", res.RunID).Find(&errs).Error)
	s.Require().Len(errs, 1)
	s.Equal(catalogm.RadioSyncRunErrorTruncation, errs[0].Category,
		"PSY-1141: a truncation records as 'truncation', not validation_drop")
	s.Require().NotNil(errs[0].EpisodeRef)
	s.Equal("ep-1", *errs[0].EpisodeRef, "structured error carries the episode ref")
}
