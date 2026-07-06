package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// PSY-1333 integration: the slot-fetch work list (ShowsWithSlotBoundariesIn)
// and the single-show scoped fetch path (RunStationSync Mode=Fetch + ShowID).
// Suite: RadioSyncSuite (radio_sync_integration_test.go, Postgres testcontainer).

// setShowSchedule stamps a stored weekly schedule with one slot whose start/end
// bracket is expressed in UTC HH:MM for the given reference day.
func (s *RadioSyncSuite) setShowSchedule(showID uint, dayOfWeek int, start, end string) {
	sched := fmt.Sprintf(
		`{"timezone":"UTC","slots":[{"day_of_week":%d,"start":%q,"end":%q}]}`,
		dayOfWeek, start, end)
	s.Require().NoError(s.db.Model(&catalogm.RadioShow{}).Where("id = ?", showID).
		Update("schedule", json.RawMessage(sched)).Error)
}

func (s *RadioSyncSuite) TestShowsWithSlotBoundariesIn() {
	// A fixed reference instant keeps the weekday math deterministic:
	// 2026-07-03 is a Friday (weekday 5). Window = (09:55, 10:05] UTC.
	from := time.Date(2026, 7, 3, 9, 55, 0, 0, time.UTC)
	to := time.Date(2026, 7, 3, 10, 5, 0, 0, time.UTC)

	st := s.seedStation(catalogm.PlaylistSourceNTS)
	due := s.seedActiveShow(st.ID, "slot-due", nil)
	s.setShowSchedule(due.ID, 5, "10:00", "13:00") // start crosses in-window

	quiet := s.seedActiveShow(st.ID, "slot-quiet", nil)
	s.setShowSchedule(quiet.ID, 5, "14:00", "16:00") // no boundary in-window

	noExt := s.seedActiveShow(st.ID, "slot-no-ext", nil)
	s.setShowSchedule(noExt.ID, 5, "10:00", "13:00")
	s.Require().NoError(s.db.Model(&catalogm.RadioShow{}).Where("id = ?", noExt.ID).
		Update("external_id", "").Error)

	manual := s.seedStation(catalogm.PlaylistSourceManual)
	onManual := s.seedActiveShow(manual.ID, "slot-on-manual", nil)
	s.setShowSchedule(onManual.ID, 5, "10:00", "13:00")

	// One bad stored schedule must not starve the tick — it's skipped with a warn
	// while healthy siblings still fire.
	badSched := s.seedActiveShow(st.ID, "slot-bad-schedule", nil)
	s.Require().NoError(s.db.Model(&catalogm.RadioShow{}).Where("id = ?", badSched.ID).
		Update("schedule", json.RawMessage(`{"timezone":"Not/AZone","slots":[{"day_of_week":5,"start":"10:00","end":"13:00"}]}`)).Error)

	got, err := s.svc.ShowsWithSlotBoundariesIn(from, to)
	s.Require().NoError(err)
	s.Require().Len(got, 1, "only the automated station contributes")
	s.Equal([]uint{due.ID}, got[st.ID],
		"only the show with an in-window boundary and a fetchable identity is due")
}

// A Mode=Fetch run scoped by ShowID fetches EXACTLY that show, records the
// show on its run row, and never bumps the PSY-1274 failure streak — scoped
// attempts are extra tries, not scheduled cycles.
func (s *RadioSyncSuite) TestRunStationSync_FetchScopedToShow() {
	st := s.seedStation(catalogm.PlaylistSourceNTS)
	target := s.seedActiveShow(st.ID, "scoped-target", nil)
	s.seedActiveShow(st.ID, "scoped-sibling", nil)

	var fetchedExts []string
	targetErrs := true
	s.svc.playlistProviderFactory = func(string) (RadioPlaylistProvider, error) {
		return &mockPlaylistProvider{
			fetchNewEpisodesFn: func(ext string, _, _ time.Time) ([]RadioEpisodeImport, error) {
				fetchedExts = append(fetchedExts, ext)
				if targetErrs {
					return nil, fmt.Errorf("404 gone")
				}
				return nil, nil
			},
		}, nil
	}
	defer func() { s.svc.playlistProviderFactory = nil }()

	// Scoped run whose one show FAILS: streak must stay 0 (scoped ≠ cycle).
	res, err := s.svc.RunStationSync(context.Background(), st.ID, RunStationSyncOpts{
		Mode:    catalogm.RadioSyncRunTypeFetch,
		Trigger: catalogm.RadioSyncRunTriggerScheduled,
		ShowID:  &target.ID,
	})
	s.Require().NoError(err)
	s.Equal([]string{"scoped-target"}, fetchedExts, "only the scoped show is fetched")
	s.Equal(0, s.streakOf(target.ID), "a scoped failure must not bump the scheduled-cycle streak")

	var run catalogm.RadioSyncRun
	s.Require().NoError(s.db.First(&run, res.RunID).Error)
	s.Require().NotNil(run.ShowID)
	s.Equal(target.ID, *run.ShowID, "scoped run row carries the show id")

	// Scoped SUCCESS still resets a pre-existing streak.
	s.setShowStreak(target.ID, 2)
	targetErrs = false
	fetchedExts = nil
	_, err = s.svc.RunStationSync(context.Background(), st.ID, RunStationSyncOpts{
		Mode:    catalogm.RadioSyncRunTypeFetch,
		Trigger: catalogm.RadioSyncRunTriggerScheduled,
		ShowID:  &target.ID,
	})
	s.Require().NoError(err)
	s.Equal([]string{"scoped-target"}, fetchedExts)
	s.Equal(0, s.streakOf(target.ID), "a scoped success still clears the streak")
}

// Scoped fetch runs are invisible to the volume-anomaly guard: they are never
// themselves flagged, and their (single-show scale) play counts never enter the
// station-sweep baseline.
func (s *RadioSyncSuite) TestVolumeAnomaly_IgnoresShowScopedRuns() {
	st := s.seedStation(catalogm.PlaylistSourceKEXP)
	show := s.seedActiveShow(st.ID, "anomaly-scoped", nil)

	// Baseline: 5 healthy full-sweep successes at ~100 plays…
	for range 5 {
		s.Require().NoError(s.db.Create(&catalogm.RadioSyncRun{
			StationID: radioSyncStationID(st.ID), RunType: catalogm.RadioSyncRunTypeFetch,
			Trigger: catalogm.RadioSyncRunTriggerScheduled,
			Status:  catalogm.RadioSyncRunStatusSuccess,
			PlaysImported: 100,
			StartedAt:     time.Now().Add(-time.Hour), FinishedAt: ptrTime(time.Now().Add(-time.Hour)),
		}).Error)
	}
	// …plus a pile of scoped zero-play successes that would poison the mean if counted.
	for range 10 {
		s.Require().NoError(s.db.Create(&catalogm.RadioSyncRun{
			StationID: radioSyncStationID(st.ID), ShowID: &show.ID,
			RunType: catalogm.RadioSyncRunTypeFetch,
			Trigger: catalogm.RadioSyncRunTriggerScheduled,
			Status:  catalogm.RadioSyncRunStatusSuccess,
			PlaysImported: 0,
			StartedAt:     time.Now().Add(-30 * time.Minute), FinishedAt: ptrTime(time.Now().Add(-30 * time.Minute)),
		}).Error)
	}

	anomaly, _ := s.svc.detectVolumeAnomaly(st.ID, 0, 0)
	s.True(anomaly,
		"a zero-play sweep against the 100-play baseline must still flag — scoped runs must not dilute the mean")
}

// Scoped fetch runs are BREAKER-NEUTRAL: their failures never stack the station
// counter (a boundary-cluster tick with a station-level blip would otherwise trip
// the 5-failure breaker in minutes), their outcomes never reset genuine sweep
// accumulation, and an OPEN-past-cooldown breaker is never consumed as a scoped
// run's half-open trial — the scoped run is skipped and the breaker left open
// for the next sweep to probe.
func (s *RadioSyncSuite) TestScopedFetch_BreakerNeutral() {
	st := s.seedStation(catalogm.PlaylistSourceNTS)
	target := s.seedActiveShow(st.ID, "breaker-scoped", nil)

	s.svc.playlistProviderFactory = func(string) (RadioPlaylistProvider, error) {
		return &mockPlaylistProvider{
			fetchNewEpisodesFn: func(string, time.Time, time.Time) ([]RadioEpisodeImport, error) {
				return nil, fmt.Errorf("provider down")
			},
		}, nil
	}
	defer func() { s.svc.playlistProviderFactory = nil }()

	scopedOpts := RunStationSyncOpts{
		Mode:    catalogm.RadioSyncRunTypeFetch,
		Trigger: catalogm.RadioSyncRunTriggerScheduled,
		ShowID:  &target.ID,
	}

	// Failing scoped runs leave station health untouched (no row, closed/zero).
	for range 3 {
		_, err := s.svc.RunStationSync(context.Background(), st.ID, scopedOpts)
		s.Require().NoError(err)
	}
	snap := s.svc.readBreakerSnapshot(st.ID)
	s.Equal(catalogm.RadioBreakerStateClosed, snap.state, "scoped failures must not move the breaker")
	s.Equal(0, snap.failures, "scoped failures must not stack the counter")

	// Breaker OPEN past cooldown: a scoped run must be skipped, not promoted to
	// the half-open trial (and certainly not allowed to close it via `partial`).
	trippedAt := time.Now().Add(-24 * time.Hour)
	s.Require().NoError(s.db.Where("station_id = ?", st.ID).Delete(&catalogm.RadioStationHealth{}).Error)
	s.Require().NoError(s.db.Create(&catalogm.RadioStationHealth{
		StationID: st.ID, BreakerState: catalogm.RadioBreakerStateOpen,
		ConsecutiveFailures: 5, BreakerTrippedAt: &trippedAt,
	}).Error)

	res, err := s.svc.RunStationSync(context.Background(), st.ID, scopedOpts)
	s.Require().NoError(err)
	s.True(res.Skipped, "an open-past-cooldown breaker must skip a scoped run, not trial it")
	snap = s.svc.readBreakerSnapshot(st.ID)
	s.Equal(catalogm.RadioBreakerStateOpen, snap.state, "the breaker stays open for the next sweep to probe")
	s.Equal(5, snap.failures)
}
