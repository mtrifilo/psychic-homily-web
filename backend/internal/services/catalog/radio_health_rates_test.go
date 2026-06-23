package catalog

import (
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// PSY-1201: on-write computation of the radio_station_health rates
// (recent_success_rate, play_match_rate, zero_play_episode_rate) over the trailing
// window. Runs against the same testcontainers Postgres as RadioSyncSuite.

func (s *RadioSyncSuite) seedSyncRunWithPlays(stationID uint, status string, startedAt time.Time, imported, matched int) {
	run := catalogm.RadioSyncRun{
		StationID:     stationID,
		RunType:       catalogm.RadioSyncRunTypeFetch,
		Trigger:       catalogm.RadioSyncRunTriggerScheduled,
		Status:        status,
		StartedAt:     startedAt,
		PlaysImported: imported,
		PlaysMatched:  matched,
	}
	if status != catalogm.RadioSyncRunStatusRunning {
		fin := startedAt.Add(time.Minute)
		run.FinishedAt = &fin
	}
	s.Require().NoError(s.db.Create(&run).Error)
}

func (s *RadioSyncSuite) seedEpisodeWithPlayCount(showID uint, ext, airDate string, playCount int) {
	ep := catalogm.RadioEpisode{ShowID: showID, ExternalID: &ext, AirDate: airDate, PlayCount: playCount}
	s.Require().NoError(s.db.Create(&ep).Error)
}

// computeStationRates derives all three rates over the trailing window, counts only
// in-window non-running/non-cancelled runs for the success denominator, sums plays for
// the match rate, and zero-play episodes for the episode rate.
func (s *RadioSyncSuite) TestComputeStationRates_Computed() {
	st := s.seedStation(catalogm.PlaylistSourceKEXP)
	now := time.Now()
	inWindow := now.Add(-2 * 24 * time.Hour)
	outWindow := now.Add(-35 * 24 * time.Hour)

	// Runs in window: 3 success + 1 failed → success_rate = 3/4 = 0.75.
	// Plays: 100/75 + 100/75 = matched 150 / imported 200 → play_match_rate = 0.75.
	s.seedSyncRunWithPlays(st.ID, catalogm.RadioSyncRunStatusSuccess, inWindow, 100, 75)
	s.seedSyncRunWithPlays(st.ID, catalogm.RadioSyncRunStatusSuccess, inWindow, 100, 75)
	s.seedSyncRunWithPlays(st.ID, catalogm.RadioSyncRunStatusSuccess, inWindow, 0, 0)
	s.seedSyncRunWithPlays(st.ID, catalogm.RadioSyncRunStatusFailed, inWindow, 0, 0)
	// A cancelled run must NOT count toward the success denominator.
	s.seedSyncRunWithPlays(st.ID, catalogm.RadioSyncRunStatusCancelled, inWindow, 0, 0)
	// Out-of-window success must NOT count.
	s.seedSyncRunWithPlays(st.ID, catalogm.RadioSyncRunStatusSuccess, outWindow, 500, 500)

	// Episodes: 4 in window, 1 with zero plays → zero_play_episode_rate = 1/4 = 0.25.
	// 1 zero-play episode out of window must NOT count.
	show := s.seedShowFor(st.ID, "Rates Show", "rates-show", "ext-rates")
	s.seedEpisodeWithPlayCount(show.ID, "e1", inWindow.Format("2006-01-02"), 5)
	s.seedEpisodeWithPlayCount(show.ID, "e2", inWindow.Format("2006-01-02"), 3)
	s.seedEpisodeWithPlayCount(show.ID, "e3", inWindow.Format("2006-01-02"), 8)
	s.seedEpisodeWithPlayCount(show.ID, "e4", inWindow.Format("2006-01-02"), 0)
	s.seedEpisodeWithPlayCount(show.ID, "e5", outWindow.Format("2006-01-02"), 0)

	success, playMatch, zeroPlay, ok := s.svc.computeStationRates(st.ID, now)
	s.Require().True(ok)
	s.Require().NotNil(success)
	s.InDelta(0.75, *success, 0.0001)
	s.Require().NotNil(playMatch)
	s.InDelta(0.75, *playMatch, 0.0001)
	s.Require().NotNil(zeroPlay)
	s.InDelta(0.25, *zeroPlay, 0.0001)
}

// No data in the window → each rate is nil (denominator zero), not 0.0.
func (s *RadioSyncSuite) TestComputeStationRates_NilWhenNoData() {
	st := s.seedStation(catalogm.PlaylistSourceKEXP)

	success, playMatch, zeroPlay, ok := s.svc.computeStationRates(st.ID, time.Now())
	s.Require().True(ok)
	s.Nil(success, "no terminal runs → nil success rate")
	s.Nil(playMatch, "no imported plays → nil play-match rate")
	s.Nil(zeroPlay, "no episodes → nil zero-play rate")
}

// play_match_rate is nil (not 0) when runs exist but imported zero plays (e.g. discover
// runs), so the card shows "—" rather than a misleading 0%.
func (s *RadioSyncSuite) TestComputeStationRates_PlayMatchNilWhenNoPlays() {
	st := s.seedStation(catalogm.PlaylistSourceKEXP)
	s.seedSyncRunWithPlays(st.ID, catalogm.RadioSyncRunStatusSuccess, time.Now().Add(-time.Hour), 0, 0)

	success, playMatch, _, ok := s.svc.computeStationRates(st.ID, time.Now())
	s.Require().True(ok)
	s.Require().NotNil(success)
	s.InDelta(1.0, *success, 0.0001, "1 success / 1 terminal")
	s.Nil(playMatch, "imported 0 plays → nil, not 0/0")
}

// The on-write path (updateStationHealth) persists the computed rates so the read
// endpoints surface them. Wires the seeded sync_runs/episodes through the real writer.
func (s *RadioSyncSuite) TestUpdateStationHealth_PersistsRates() {
	st := s.seedStation(catalogm.PlaylistSourceKEXP)
	now := time.Now()
	s.seedSyncRunWithPlays(st.ID, catalogm.RadioSyncRunStatusSuccess, now.Add(-time.Hour), 100, 90)
	show := s.seedShowFor(st.ID, "Rates Show", "rates-show", "ext-rates")
	s.seedEpisodeWithPlayCount(show.ID, "e1", now.Format("2006-01-02"), 4)
	s.seedEpisodeWithPlayCount(show.ID, "e2", now.Format("2006-01-02"), 0)

	s.svc.updateStationHealth(st.ID, catalogm.RadioSyncRunStatusSuccess, catalogm.RadioSyncRunTriggerScheduled, classifyError(nil))

	h := s.reloadStationHealth(st.ID)
	s.Require().NotNil(h.PlayMatchRate)
	s.InDelta(0.9, *h.PlayMatchRate, 0.0001)
	s.Require().NotNil(h.ZeroPlayEpisodeRate)
	s.InDelta(0.5, *h.ZeroPlayEpisodeRate, 0.0001)
	s.Require().NotNil(h.RecentSuccessRate)
	s.InDelta(1.0, *h.RecentSuccessRate, 0.0001)
}

func (s *RadioSyncSuite) reloadStationHealth(stationID uint) catalogm.RadioStationHealth {
	var h catalogm.RadioStationHealth
	s.Require().NoError(s.db.First(&h, "station_id = ?", stationID).Error)
	return h
}
