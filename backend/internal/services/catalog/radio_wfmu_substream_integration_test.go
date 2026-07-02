package catalog

import (
	"encoding/json"
	"fmt"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// PSY-1322 ApplyWFMUSubstreamSchedule: the partial-today merge rule, guarded
// clearing, lock skipping, and station scoping. Runs on the RadioSyncSuite
// testcontainers Postgres.

func (s *RadioSyncSuite) seedSubstreamStation(slug string) catalogm.RadioStation {
	src := catalogm.PlaylistSourceWFMU
	st := catalogm.RadioStation{
		Name:           "Test " + slug,
		Slug:           slug,
		BroadcastType:  catalogm.BroadcastTypeInternet,
		PlaylistSource: &src,
	}
	s.Require().NoError(s.db.Create(&st).Error)
	return st
}

func (s *RadioSyncSuite) seedSubstreamShow(stationID uint, code string, sched *catalogm.RadioSchedule, locked bool) catalogm.RadioShow {
	show := catalogm.RadioShow{
		StationID:      stationID,
		Name:           "Show " + code,
		Slug:           fmt.Sprintf("show-%s-%d", code, stationID),
		ExternalID:     &code,
		IsActive:       true,
		ScheduleLocked: locked,
	}
	if sched != nil {
		raw, err := json.Marshal(sched)
		s.Require().NoError(err)
		rawMsg := json.RawMessage(raw)
		show.Schedule = &rawMsg
	}
	s.Require().NoError(s.db.Create(&show).Error)
	return show
}

func (s *RadioSyncSuite) loadSchedule(showID uint) *catalogm.RadioSchedule {
	var show catalogm.RadioShow
	s.Require().NoError(s.db.First(&show, showID).Error)
	if show.Schedule == nil {
		return nil
	}
	sched, err := catalogm.ParseRadioSchedule(show.Schedule)
	s.Require().NoError(err)
	return sched
}

// fillerEntries seeds n matched (show + scraped entry) pairs so the
// recognized-shows floor is met without cluttering the case under test.
func (s *RadioSyncSuite) fillerEntries(stationID uint, n int, day int) []WFMUScheduleEntry {
	entries := make([]WFMUScheduleEntry, 0, n)
	for i := 0; i < n; i++ {
		code := fmt.Sprintf("F%02d", i)
		s.seedSubstreamShow(stationID, code, nil, false)
		entries = append(entries, WFMUScheduleEntry{Code: code, Name: "Filler " + code,
			Slots: []catalogm.RadioScheduleSlot{{DayOfWeek: day, Start: "06:00", End: "09:00"}}})
	}
	return entries
}

// The partial-today rule end-to-end: full-day slots come from the scrape, the
// scrape day's slots are preserved from the existing schedule, and a show
// whose only airing is the scrape day survives being absent from the scrape.
func (s *RadioSyncSuite) TestSubstreamApply_PartialTodayMerge() {
	st := s.seedSubstreamStation("test-sub-drummer")
	const today = 4 // the excluded weekday

	// Show A: existing schedule has a today slot + a stale Monday slot; the
	// scrape says its Monday slot moved to Tuesday. Expect: today preserved,
	// Monday gone, Tuesday in.
	aSched := &catalogm.RadioSchedule{Timezone: wfmuScheduleTimezone, Slots: []catalogm.RadioScheduleSlot{
		{DayOfWeek: today, Start: "12:00", End: "15:00"},
		{DayOfWeek: 1, Start: "09:00", End: "12:00"},
	}}
	a := s.seedSubstreamShow(st.ID, "AA", aSched, false)

	// Show B: airs ONLY today; absent from the scrape (already aired when the
	// rolling window was generated). Must survive untouched, not be cleared.
	bSched := &catalogm.RadioSchedule{Timezone: wfmuScheduleTimezone, Slots: []catalogm.RadioScheduleSlot{
		{DayOfWeek: today, Start: "20:00", End: "22:00"},
	}}
	b := s.seedSubstreamShow(st.ID, "BB", bSched, false)

	// Show C: dropped from the lineup entirely (only a full-day slot stored,
	// absent from the scrape). Must be cleared once the floor is met.
	cSched := &catalogm.RadioSchedule{Timezone: wfmuScheduleTimezone, Slots: []catalogm.RadioScheduleSlot{
		{DayOfWeek: 2, Start: "09:00", End: "12:00"},
	}}
	c := s.seedSubstreamShow(st.ID, "CC", cSched, false)

	entries := append(s.fillerEntries(st.ID, wfmuSubstreamClearMinEntries, 3),
		WFMUScheduleEntry{Code: "AA", Name: "Show AA", Slots: []catalogm.RadioScheduleSlot{
			{DayOfWeek: 2, Start: "09:00", End: "12:00"},
			// The scrape also carries a (partial) today slot — it must be
			// IGNORED in favor of the stored one.
			{DayOfWeek: today, Start: "13:00", End: "16:00"},
		}})

	matched, unmatched, cleared, err := s.svc.ApplyWFMUSubstreamSchedule("test-sub-drummer", entries, today)
	s.Require().NoError(err)
	s.Equal(0, unmatched)
	s.Equal(1, cleared, "only the fully-dropped show clears")
	s.GreaterOrEqual(matched, wfmuSubstreamClearMinEntries+1)

	aGot := s.loadSchedule(a.ID)
	s.Require().NotNil(aGot)
	s.ElementsMatch([]catalogm.RadioScheduleSlot{
		{DayOfWeek: today, Start: "12:00", End: "15:00"}, // preserved, NOT the scrape's 13:00
		{DayOfWeek: 2, Start: "09:00", End: "12:00"},     // scraped full day
	}, aGot.Slots)

	bGot := s.loadSchedule(b.ID)
	s.Require().NotNil(bGot, "a today-only show must survive the day it airs")
	s.Equal(bSched.Slots, bGot.Slots)

	s.Nil(s.loadSchedule(c.ID), "a show absent from every full day is cleared")
}

// Below the recognized floor nothing clears (suspect parse), but writes for
// recognized shows still land — same posture as the flagship PSY-1186 guard.
func (s *RadioSyncSuite) TestSubstreamApply_FloorDisablesClearsOnly() {
	st := s.seedSubstreamStation("test-sub-sheena")
	const today = 1

	dropped := s.seedSubstreamShow(st.ID, "DD", &catalogm.RadioSchedule{
		Timezone: wfmuScheduleTimezone,
		Slots:    []catalogm.RadioScheduleSlot{{DayOfWeek: 3, Start: "09:00", End: "12:00"}},
	}, false)
	updated := s.seedSubstreamShow(st.ID, "EE", nil, false)

	entries := []WFMUScheduleEntry{{Code: "EE", Name: "Show EE", Slots: []catalogm.RadioScheduleSlot{
		{DayOfWeek: 5, Start: "18:00", End: "20:00"},
	}}}

	matched, _, cleared, err := s.svc.ApplyWFMUSubstreamSchedule("test-sub-sheena", entries, today)
	s.Require().NoError(err)
	s.Equal(0, cleared, "one recognized show is far below the floor — no clears")
	s.Equal(1, matched)
	s.NotNil(s.loadSchedule(dropped.ID), "dropped show survives a suspect parse")
	s.NotNil(s.loadSchedule(updated.ID))
}

// schedule_locked shows are never written or cleared, and a code only touches
// the row on ITS station (PSY-1127 family scoping).
func (s *RadioSyncSuite) TestSubstreamApply_LockAndStationScoping() {
	sub := s.seedSubstreamStation("test-sub-rocknsoul")
	flagship := s.seedSubstreamStation("test-sub-other")
	const today = 0

	lockedSched := &catalogm.RadioSchedule{Timezone: wfmuScheduleTimezone, Slots: []catalogm.RadioScheduleSlot{
		{DayOfWeek: 6, Start: "10:00", End: "12:00"},
	}}
	locked := s.seedSubstreamShow(sub.ID, "LL", lockedSched, true)

	// Same code on another station — must not be touched by the sub apply.
	foreign := s.seedSubstreamShow(flagship.ID, "MM", nil, false)
	target := s.seedSubstreamShow(sub.ID, "MM", nil, false)

	entries := []WFMUScheduleEntry{
		{Code: "LL", Name: "Locked", Slots: []catalogm.RadioScheduleSlot{{DayOfWeek: 2, Start: "08:00", End: "10:00"}}},
		{Code: "MM", Name: "Shared Code", Slots: []catalogm.RadioScheduleSlot{{DayOfWeek: 3, Start: "08:00", End: "10:00"}}},
	}

	_, _, _, err := s.svc.ApplyWFMUSubstreamSchedule("test-sub-rocknsoul", entries, today)
	s.Require().NoError(err)

	lockedGot := s.loadSchedule(locked.ID)
	s.Require().NotNil(lockedGot)
	s.Equal(lockedSched.Slots, lockedGot.Slots, "locked schedule untouched")

	s.NotNil(s.loadSchedule(target.ID), "code writes to its own station's row")
	s.Nil(s.loadSchedule(foreign.ID), "sibling station's same-code row untouched")
}
