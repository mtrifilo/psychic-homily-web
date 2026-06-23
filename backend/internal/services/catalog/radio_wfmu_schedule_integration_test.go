package catalog

import (
	"encoding/json"
	"fmt"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

func updateReq(schedule *json.RawMessage, locked *bool) *contracts.UpdateRadioShowRequest {
	return &contracts.UpdateRadioShowRequest{Schedule: schedule, ScheduleLocked: locked}
}

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

	matched, unmatched, cleared, err := s.svc.ApplyWFMUSchedule(entries)
	s.Require().NoError(err)
	s.Equal(1, matched, "only the flagship WA show matches")
	s.Equal(1, unmatched, "ZZ has no row, deferred")
	s.Equal(0, cleared, "2 entries is below the clear-on-absence floor, so nothing is cleared")

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

	_, _, _, err := s.svc.ApplyWFMUSchedule([]WFMUScheduleEntry{
		{Code: "WA", Name: "Wake", Slots: []catalogm.RadioScheduleSlot{{DayOfWeek: 1, Start: "06:00", End: "09:00"}}},
	})
	s.Require().NoError(err)

	// A later scrape (seasonal churn) replaces the slots wholesale.
	_, _, _, err = s.svc.ApplyWFMUSchedule([]WFMUScheduleEntry{
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
	_, _, _, err := s.svc.ApplyWFMUSchedule([]WFMUScheduleEntry{
		{Code: "WA", Name: "Wake", Slots: []catalogm.RadioScheduleSlot{{DayOfWeek: 1, Start: "06:00", End: "09:00"}}},
	})
	s.Require().Error(err)
}

// --- PSY-1186 hardening: provenance (schedule_locked) + clear-on-absence ---

const sampleScheduleJSON = `{"timezone":"America/New_York","slots":[{"day_of_week":1,"start":"06:00","end":"09:00"}]}`

// seedScheduledShow creates a flagship show that already has a schedule, optionally locked.
func (s *RadioSyncSuite) seedScheduledShow(stationID uint, name, slug, ext string, locked bool) catalogm.RadioShow {
	e := ext
	raw := json.RawMessage(sampleScheduleJSON)
	show := catalogm.RadioShow{
		StationID: stationID, Name: name, Slug: slug, ExternalID: &e,
		Schedule: &raw, ScheduleLocked: locked,
	}
	s.Require().NoError(s.db.Create(&show).Error)
	return show
}

func (s *RadioSyncSuite) scheduleLockedOf(showID uint) bool {
	var show catalogm.RadioShow
	s.Require().NoError(s.db.First(&show, showID).Error)
	return show.ScheduleLocked
}

// unrecognizedEntries builds n entries whose codes match NO seeded show (recognized == 0) —
// the markup-shift case: a parse that yields plenty of codes but none that map to a real row.
func unrecognizedEntries(n int) []WFMUScheduleEntry {
	out := make([]WFMUScheduleEntry, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, WFMUScheduleEntry{
			Code:  fmt.Sprintf("Z%03d", i),
			Name:  "Filler",
			Slots: []catalogm.RadioScheduleSlot{{DayOfWeek: 1, Start: "06:00", End: "09:00"}},
		})
	}
	return out
}

// A schedule_locked show is NOT overwritten by the scrape (admin-curated); an unlocked one is.
func (s *RadioSyncSuite) TestApplyWFMUSchedule_SkipsLockedShow() {
	s.wipeRadioTables()
	flagship := s.seedWFMUStation(wfmuFlagshipStationSlug)
	locked := s.seedScheduledShow(flagship.ID, "Locked Show", "locked-show", "LK", true)
	unlocked := s.seedScheduledShow(flagship.ID, "Unlocked Show", "unlocked-show", "UL", false)

	entries := []WFMUScheduleEntry{
		{Code: "LK", Name: "Locked Show", Slots: []catalogm.RadioScheduleSlot{{DayOfWeek: 3, Start: "10:00", End: "12:00"}}},
		{Code: "UL", Name: "Unlocked Show", Slots: []catalogm.RadioScheduleSlot{{DayOfWeek: 3, Start: "10:00", End: "12:00"}}},
	}
	matched, _, _, err := s.svc.ApplyWFMUSchedule(entries)
	s.Require().NoError(err)
	s.Equal(1, matched, "only the unlocked show is written")

	// Locked show keeps its original Monday 06:00-09:00 slot (untouched).
	lk := s.scheduleOf(locked.ID)
	s.Require().NotNil(lk)
	s.Equal(catalogm.RadioScheduleSlot{DayOfWeek: 1, Start: "06:00", End: "09:00"}, lk.Slots[0], "locked schedule is not clobbered")
	// Unlocked show is updated to the scraped Wednesday 10:00-12:00 slot.
	ul := s.scheduleOf(unlocked.ID)
	s.Require().NotNil(ul)
	s.Equal(catalogm.RadioScheduleSlot{DayOfWeek: 3, Start: "10:00", End: "12:00"}, ul.Slots[0], "unlocked schedule is overwritten")
}

// Clear-on-absence nulls an unlocked show's schedule when its code drops off the grid, but
// leaves locked shows alone — and only when enough shows were RECOGNIZED (matched a row) for
// the scrape to be trustworthy.
func (s *RadioSyncSuite) TestApplyWFMUSchedule_ClearsAbsentUnlockedOnly() {
	s.wipeRadioTables()
	flagship := s.seedWFMUStation(wfmuFlagshipStationSlug)

	// Seed ≥ floor shows that ARE in the scrape, so it clears the recognized-shows guard.
	var entries []WFMUScheduleEntry
	for i := 0; i < wfmuScheduleClearMinEntries; i++ {
		code := fmt.Sprintf("R%02d", i)
		s.seedShowWithExternalID(flagship.ID, "Recognized "+code, "rec-"+code, code)
		entries = append(entries, WFMUScheduleEntry{
			Code: code, Name: "Recognized",
			Slots: []catalogm.RadioScheduleSlot{{DayOfWeek: 1, Start: "06:00", End: "09:00"}},
		})
	}
	// Two shows that dropped off the grid (absent from entries).
	gone := s.seedScheduledShow(flagship.ID, "Gone Show", "gone-show", "GONE", false)        // unlocked → cleared
	keptLock := s.seedScheduledShow(flagship.ID, "Kept Locked", "kept-locked", "KEPT", true) // locked → kept

	matched, _, cleared, err := s.svc.ApplyWFMUSchedule(entries)
	s.Require().NoError(err)
	s.Equal(wfmuScheduleClearMinEntries, matched, "all recognized shows are written")
	s.Equal(1, cleared, "only the unlocked absent show is cleared")
	s.Nil(s.scheduleOf(gone.ID), "unlocked absent show's schedule is cleared")
	s.NotNil(s.scheduleOf(keptLock.ID), "locked absent show's schedule is preserved")
}

// A scrape that RECOGNIZES too few shows clears nothing — even with many parsed codes. Guards
// against a markup shift that yields ≥floor junk codes (matching no row) wiping the lineup via
// NOT IN <junk> (the floor is on recognized shows, not parsed-code count).
func (s *RadioSyncSuite) TestApplyWFMUSchedule_UnrecognizedScrapeClearsNothing() {
	s.wipeRadioTables()
	flagship := s.seedWFMUStation(wfmuFlagshipStationSlug)
	gone := s.seedScheduledShow(flagship.ID, "Gone Show", "gone-show", "GONE", false)

	// 25 codes (> the 20 floor) that match nothing → recognized == 0.
	_, _, cleared, err := s.svc.ApplyWFMUSchedule(unrecognizedEntries(25))
	s.Require().NoError(err)
	s.Equal(0, cleared, "junk codes that match no show do not authorize a clear")
	s.NotNil(s.scheduleOf(gone.ID), "schedule is preserved when too few shows are recognized")
}

// UpdateShow auto-locks a hand-edited schedule; an explicit schedule_locked=false unlocks it.
func (s *RadioSyncSuite) TestUpdateShow_ScheduleEditAutoLocks() {
	s.wipeRadioTables()
	flagship := s.seedWFMUStation(wfmuFlagshipStationSlug)
	show := s.seedShowWithExternalID(flagship.ID, "Editable", "editable", "ED")
	s.False(s.scheduleLockedOf(show.ID), "starts unlocked")

	raw := json.RawMessage(sampleScheduleJSON)
	_, err := s.svc.UpdateShow(show.ID, updateReq(&raw, nil))
	s.Require().NoError(err)
	s.True(s.scheduleLockedOf(show.ID), "editing the schedule auto-locks it")

	unlock := false
	_, err = s.svc.UpdateShow(show.ID, updateReq(nil, &unlock))
	s.Require().NoError(err)
	s.False(s.scheduleLockedOf(show.ID), "explicit schedule_locked=false resumes auto-scrape")
}
