package catalog

import (
	catalogm "psychic-homily-backend/internal/models/catalog"
)

// PSY-1159 ApplyWFMUSchedule, end-to-end against the same testcontainers Postgres as
// RadioSyncSuite. Covers the exact-by-code match, the validated JSONB write, the WFMU 91.1
// scoping (PSY-1127), unmatched deferral, and re-run overwrite. Parser correctness is in
// TestParseWFMUScheduleTable_*.
//
// The test DB migrations SEED the real WFMU family stations/shows (slugs wfmu,
// wfmu-drummer, …). TearDownTest wipes the radio tables after each test, but the FIRST
// suite test runs before any teardown — so each test here wipes first for a clean slate.

func (s *RadioSyncSuite) wipeRadioTables() {
	sqlDB, err := s.db.DB()
	s.Require().NoError(err)
	for _, tbl := range []string{
		"radio_sync_run_errors", "radio_sync_runs", "radio_station_health",
		"radio_plays", "radio_episodes", "radio_shows", "radio_stations",
	} {
		_, err := sqlDB.Exec("DELETE FROM " + tbl)
		s.Require().NoError(err)
	}
}

func (s *RadioSyncSuite) seedWFMUStation(slug string) catalogm.RadioStation {
	src := catalogm.PlaylistSourceWFMU
	st := catalogm.RadioStation{
		Name:           "WFMU " + slug,
		Slug:           slug,
		BroadcastType:  catalogm.BroadcastTypeInternet,
		PlaylistSource: &src,
	}
	s.Require().NoError(s.db.Create(&st).Error)
	return st
}

func (s *RadioSyncSuite) seedShowWithExternalID(stationID uint, name, slug, ext string) catalogm.RadioShow {
	e := ext
	show := catalogm.RadioShow{StationID: stationID, Name: name, Slug: slug, ExternalID: &e}
	s.Require().NoError(s.db.Create(&show).Error)
	return show
}

func (s *RadioSyncSuite) scheduleOf(showID uint) *catalogm.RadioSchedule {
	var show catalogm.RadioShow
	s.Require().NoError(s.db.First(&show, showID).Error)
	if show.Schedule == nil {
		return nil
	}
	sched, err := catalogm.ParseRadioSchedule(show.Schedule)
	s.Require().NoError(err)
	return sched
}

func (s *RadioSyncSuite) TestApplyWFMUSchedule_MatchesByCodeAndScopes() {
	s.wipeRadioTables()
	flagship := s.seedWFMUStation(wfmuFlagshipStationSlug)
	sub := s.seedWFMUStation("wfmu-drummer")

	wake := s.seedShowWithExternalID(flagship.ID, "Wake", "wfmu-wake", "WA")
	subWake := s.seedShowWithExternalID(sub.ID, "Wake (rebroadcast)", "drummer-wake", "WA") // same code, other station

	entries := []WFMUScheduleEntry{
		{Code: "WA", Name: "Wake", Slots: []catalogm.RadioScheduleSlot{{DayOfWeek: 1, Start: "06:00", End: "09:00"}}},
		{Code: "ZZ", Name: "Ghost Show", Slots: []catalogm.RadioScheduleSlot{{DayOfWeek: 2, Start: "09:00", End: "12:00"}}}, // no row
	}

	matched, unmatched, err := s.svc.ApplyWFMUSchedule(entries)
	s.Require().NoError(err)
	s.Equal(1, matched, "only the flagship WA show matches")
	s.Equal(1, unmatched, "ZZ has no row, deferred")

	// Flagship WA got the validated schedule with the IANA zone + correct slot.
	sched := s.scheduleOf(wake.ID)
	s.Require().NotNil(sched, "flagship WA schedule written")
	s.Equal("America/New_York", sched.Timezone)
	s.Require().Len(sched.Slots, 1)
	s.Equal(catalogm.RadioScheduleSlot{DayOfWeek: 1, Start: "06:00", End: "09:00"}, sched.Slots[0])

	// The same code under a sub-stream station is NOT touched (the table is 91.1 only).
	s.Nil(s.scheduleOf(subWake.ID), "sub-stream WA show is not in scope, stays unscheduled")
}

func (s *RadioSyncSuite) TestApplyWFMUSchedule_OverwritesOnRerun() {
	s.wipeRadioTables()
	flagship := s.seedWFMUStation(wfmuFlagshipStationSlug)
	wake := s.seedShowWithExternalID(flagship.ID, "Wake", "wfmu-wake", "WA")

	_, _, err := s.svc.ApplyWFMUSchedule([]WFMUScheduleEntry{
		{Code: "WA", Name: "Wake", Slots: []catalogm.RadioScheduleSlot{{DayOfWeek: 1, Start: "06:00", End: "09:00"}}},
	})
	s.Require().NoError(err)

	// A later scrape (seasonal churn) replaces the slots wholesale.
	_, _, err = s.svc.ApplyWFMUSchedule([]WFMUScheduleEntry{
		{Code: "WA", Name: "Wake", Slots: []catalogm.RadioScheduleSlot{{DayOfWeek: 2, Start: "10:00", End: "12:00"}}},
	})
	s.Require().NoError(err)

	sched := s.scheduleOf(wake.ID)
	s.Require().NotNil(sched)
	s.Require().Len(sched.Slots, 1)
	s.Equal(catalogm.RadioScheduleSlot{DayOfWeek: 2, Start: "10:00", End: "12:00"}, sched.Slots[0], "re-run overwrites, not appends")
}

func (s *RadioSyncSuite) TestApplyWFMUSchedule_NoFlagshipStation() {
	s.wipeRadioTables() // guarantees no flagship row exists
	_, _, err := s.svc.ApplyWFMUSchedule([]WFMUScheduleEntry{
		{Code: "WA", Name: "Wake", Slots: []catalogm.RadioScheduleSlot{{DayOfWeek: 1, Start: "06:00", End: "09:00"}}},
	})
	s.Require().Error(err)
}
