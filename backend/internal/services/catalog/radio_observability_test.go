package catalog

import (
	"time"

	apperrors "psychic-homily-backend/internal/errors"
	catalogm "psychic-homily-backend/internal/models/catalog"
)

// PSY-1129/P5: admin observability read surfaces — sync-run feed + station health.
// Runs against the same testcontainers Postgres as RadioSyncSuite (methods span files).

// seedSyncRun inserts a radio_sync_runs row with an explicit started_at (for ordering)
// and the finished_at the lifecycle CHECK requires once the status is terminal.
func (s *RadioSyncSuite) seedSyncRun(stationID uint, status string, startedAt time.Time) catalogm.RadioSyncRun {
	run := catalogm.RadioSyncRun{
		StationID: stationID,
		RunType:   catalogm.RadioSyncRunTypeFetch,
		Trigger:   catalogm.RadioSyncRunTriggerScheduled,
		Status:    status,
		StartedAt: startedAt,
	}
	if status != catalogm.RadioSyncRunStatusRunning {
		fin := startedAt.Add(time.Minute)
		run.FinishedAt = &fin
	}
	s.Require().NoError(s.db.Create(&run).Error)
	return run
}

func (s *RadioSyncSuite) seedStationHealth(stationID uint, consecutiveFailures int, breaker string, successRate float64) {
	now := time.Now()
	rate := successRate
	playMatch := 0.5
	zeroPlay := 0.1
	tripped := now.Add(-time.Hour)
	h := catalogm.RadioStationHealth{
		StationID:           stationID,
		LastSuccessAt:       &now,
		LastRunAt:           &now,
		ConsecutiveFailures: consecutiveFailures,
		BreakerState:        breaker,
		BreakerTrippedAt:    &tripped,
		RecentSuccessRate:   &rate,
		PlayMatchRate:       &playMatch,
		ZeroPlayEpisodeRate: &zeroPlay,
	}
	s.Require().NoError(s.db.Create(&h).Error)
}

// Per-station feed: newest-first, paginated, with the matched-set total.
func (s *RadioSyncSuite) TestListSyncRuns_PerStationOrderingPaginationTotal() {
	st := s.seedStation(catalogm.PlaylistSourceKEXP)
	base := time.Now().Add(-time.Hour)
	oldest := s.seedSyncRun(st.ID, catalogm.RadioSyncRunStatusSuccess, base)
	mid := s.seedSyncRun(st.ID, catalogm.RadioSyncRunStatusSuccess, base.Add(10*time.Minute))
	newest := s.seedSyncRun(st.ID, catalogm.RadioSyncRunStatusSuccess, base.Add(20*time.Minute))

	stationID := st.ID
	page1, total, err := s.svc.ListSyncRuns(&stationID, "", "", 2, 0)
	s.Require().NoError(err)
	s.Equal(int64(3), total, "total counts all matched runs, not the page")
	s.Require().Len(page1, 2)
	s.Equal(newest.ID, page1[0].ID, "newest first")
	s.Equal(mid.ID, page1[1].ID)

	page2, _, err := s.svc.ListSyncRuns(&stationID, "", "", 2, 2)
	s.Require().NoError(err)
	s.Require().Len(page2, 1)
	s.Equal(oldest.ID, page2[0].ID)
}

// status filters to the exact run status.
func (s *RadioSyncSuite) TestListSyncRuns_StatusFilter() {
	st := s.seedStation(catalogm.PlaylistSourceKEXP)
	base := time.Now().Add(-time.Hour)
	s.seedSyncRun(st.ID, catalogm.RadioSyncRunStatusSuccess, base)
	failed := s.seedSyncRun(st.ID, catalogm.RadioSyncRunStatusFailed, base.Add(time.Minute))
	s.seedSyncRun(st.ID, catalogm.RadioSyncRunStatusPartial, base.Add(2*time.Minute))

	stationID := st.ID
	runs, total, err := s.svc.ListSyncRuns(&stationID, catalogm.RadioSyncRunStatusFailed, "", 50, 0)
	s.Require().NoError(err)
	s.Equal(int64(1), total)
	s.Require().Len(runs, 1)
	s.Equal(failed.ID, runs[0].ID)
	s.Equal(catalogm.RadioSyncRunStatusFailed, runs[0].Status)
}

// Per-station feed 404s on an unknown station; global feed is scoped correctly.
func (s *RadioSyncSuite) TestListSyncRuns_PerStationMissing404AndScoping() {
	missing := uint(999999)
	_, _, err := s.svc.ListSyncRuns(&missing, "", "", 50, 0)
	s.Require().Error(err)
	var radioErr *apperrors.RadioError
	s.Require().ErrorAs(err, &radioErr)
	s.Equal(apperrors.CodeRadioStationNotFound, radioErr.Code)

	// Two stations, one run each → per-station returns only its own; global sees both.
	a := s.seedStation(catalogm.PlaylistSourceKEXP)
	b := s.seedStation(catalogm.PlaylistSourceWFMU)
	now := time.Now().Add(-time.Hour)
	s.seedSyncRun(a.ID, catalogm.RadioSyncRunStatusSuccess, now)
	s.seedSyncRun(b.ID, catalogm.RadioSyncRunStatusSuccess, now.Add(time.Minute))

	aID := a.ID
	aRuns, aTotal, err := s.svc.ListSyncRuns(&aID, "", "", 50, 0)
	s.Require().NoError(err)
	s.Equal(int64(1), aTotal)
	s.Require().Len(aRuns, 1)
	s.Equal(a.ID, aRuns[0].StationID)

	global, gTotal, err := s.svc.ListSyncRuns(nil, "", "", 50, 0)
	s.Require().NoError(err)
	s.GreaterOrEqual(gTotal, int64(2))
	s.GreaterOrEqual(len(global), 2)
}

// GetStationHealth maps a present rollup row.
func (s *RadioSyncSuite) TestGetStationHealth_WithRow() {
	st := s.seedStation(catalogm.PlaylistSourceKEXP)
	s.seedStationHealth(st.ID, 2, catalogm.RadioBreakerStateOpen, 0.75)

	resp, err := s.svc.GetStationHealth(st.ID)
	s.Require().NoError(err)
	s.Equal(st.ID, resp.StationID)
	s.Equal(st.Name, resp.StationName)
	s.Equal(2, resp.ConsecutiveFailures)
	s.Equal(catalogm.RadioBreakerStateOpen, resp.BreakerState)
	s.Require().NotNil(resp.RecentSuccessRate)
	s.InDelta(0.75, *resp.RecentSuccessRate, 0.0001)
	// Assert the rest of the 1:1 mapping so a field-swap typo in stationHealthToResponse
	// is caught (e.g. play_match_rate ↔ zero_play_episode_rate).
	s.Require().NotNil(resp.PlayMatchRate)
	s.InDelta(0.5, *resp.PlayMatchRate, 0.0001)
	s.Require().NotNil(resp.ZeroPlayEpisodeRate)
	s.InDelta(0.1, *resp.ZeroPlayEpisodeRate, 0.0001)
	s.NotNil(resp.BreakerTrippedAt)
	s.NotNil(resp.LastSuccessAt)
	s.NotNil(resp.UpdatedAt)
}

// A station that has never run has no health row → zero-value ("never run") response.
func (s *RadioSyncSuite) TestGetStationHealth_NeverRunZeroValue() {
	st := s.seedStation(catalogm.PlaylistSourceKEXP)

	resp, err := s.svc.GetStationHealth(st.ID)
	s.Require().NoError(err)
	s.Equal(st.ID, resp.StationID)
	s.Equal(0, resp.ConsecutiveFailures)
	s.Equal(catalogm.RadioBreakerStateClosed, resp.BreakerState, "synthesized default")
	s.Nil(resp.RecentSuccessRate, "nil = never computed, not 0.0")
	s.Nil(resp.LastSuccessAt)
}

func (s *RadioSyncSuite) TestGetStationHealth_MissingStation404() {
	_, err := s.svc.GetStationHealth(999999)
	s.Require().Error(err)
	var radioErr *apperrors.RadioError
	s.Require().ErrorAs(err, &radioErr)
	s.Equal(apperrors.CodeRadioStationNotFound, radioErr.Code)
}

// ListStationHealth returns a card per station, including stations that never ran.
func (s *RadioSyncSuite) TestListStationHealth_IncludesNeverRun() {
	withHealth := s.seedStation(catalogm.PlaylistSourceKEXP)
	neverRun := s.seedStation(catalogm.PlaylistSourceWFMU)
	s.seedStationHealth(withHealth.ID, 0, catalogm.RadioBreakerStateClosed, 0.9)

	all, err := s.svc.ListStationHealth()
	s.Require().NoError(err)

	byID := make(map[uint]bool)
	var healthy, never bool
	for _, c := range all {
		byID[c.StationID] = true
		if c.StationID == withHealth.ID {
			s.Require().NotNil(c.RecentSuccessRate)
			s.InDelta(0.9, *c.RecentSuccessRate, 0.0001)
			healthy = true
		}
		if c.StationID == neverRun.ID {
			s.Nil(c.RecentSuccessRate, "never-run station has nil rates")
			s.Equal(catalogm.RadioBreakerStateClosed, c.BreakerState)
			never = true
		}
	}
	s.True(healthy, "station with a health row appears")
	s.True(never, "station without a health row still appears (never-run card)")
}

// PSY-1343: the scope filter separates show-scoped slot-fetch rows (PSY-1333)
// from the station sweeps. 'sweep' hides scoped FETCH rows only — backfill rows
// carry a show_id too but stay visible (operator-initiated, rare); 'scoped'
// inverts; ""/'all'/unknown are the unfiltered feed.
func (s *RadioSyncSuite) TestListSyncRuns_ScopeFilter() {
	st := s.seedStation(catalogm.PlaylistSourceWFMU)
	show := s.seedActiveShow(st.ID, "scope-filter-show", nil)
	base := time.Now().Add(-time.Hour)

	sweep := s.seedSyncRun(st.ID, catalogm.RadioSyncRunStatusSuccess, base)

	scoped := s.seedSyncRun(st.ID, catalogm.RadioSyncRunStatusSuccess, base.Add(time.Minute))
	s.Require().NoError(s.db.Model(&catalogm.RadioSyncRun{}).Where("id = ?", scoped.ID).
		Update("show_id", show.ID).Error)

	backfill := s.seedSyncRun(st.ID, catalogm.RadioSyncRunStatusSuccess, base.Add(2*time.Minute))
	s.Require().NoError(s.db.Model(&catalogm.RadioSyncRun{}).Where("id = ?", backfill.ID).
		Updates(map[string]any{"run_type": catalogm.RadioSyncRunTypeBackfill, "show_id": show.ID}).Error)

	ids := func(scope string) []uint {
		runs, total, err := s.svc.ListSyncRuns(&st.ID, "", scope, 50, 0)
		s.Require().NoError(err)
		s.Equal(int64(len(runs)), total)
		out := make([]uint, 0, len(runs))
		for _, r := range runs {
			out = append(out, r.ID)
		}
		return out
	}

	s.ElementsMatch([]uint{sweep.ID, scoped.ID, backfill.ID}, ids(""), "empty scope = everything")
	s.ElementsMatch([]uint{sweep.ID, scoped.ID, backfill.ID}, ids(SyncRunScopeAll))
	s.ElementsMatch([]uint{sweep.ID, backfill.ID}, ids(SyncRunScopeSweep),
		"'sweep' hides scoped FETCH rows but keeps show-carrying backfills")
	s.ElementsMatch([]uint{scoped.ID}, ids(SyncRunScopeScoped))
}
