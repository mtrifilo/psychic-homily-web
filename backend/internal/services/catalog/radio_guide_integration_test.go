package catalog

import (
	"encoding/json"
	"fmt"
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// PSY-1053 GetRadioGuide: ON NOW / UP NEXT computed from schedule JSONB in
// each station's timezone, at a pinned instant (the service takes `now`, so
// these tests are clock-independent). Runs on the RadioSyncSuite
// testcontainers Postgres.

// guideNow is Wed 2026-07-08 16:00 UTC == Wed 12:00 EDT — mid-slot for the
// fixtures below.
var guideNow = time.Date(2026, 7, 8, 16, 0, 0, 0, time.UTC)

func (s *RadioSyncSuite) seedGuideStation(slug string) catalogm.RadioStation {
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

func (s *RadioSyncSuite) seedGuideShow(stationID uint, code string, sched *catalogm.RadioSchedule) catalogm.RadioShow {
	show := catalogm.RadioShow{
		StationID:  stationID,
		Name:       "Show " + code,
		Slug:       fmt.Sprintf("show-%s-%d", code, stationID),
		ExternalID: &code,
		IsActive:   true,
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

func (s *RadioSyncSuite) nySched(slots ...catalogm.RadioScheduleSlot) *catalogm.RadioSchedule {
	return &catalogm.RadioSchedule{Timezone: "America/New_York", Slots: slots}
}

func (s *RadioSyncSuite) TestRadioGuide_OnNowAndUpNext() {
	st := s.seedGuideStation("test-guide-a")

	onNow := s.seedGuideShow(st.ID, "GA", s.nySched(
		catalogm.RadioScheduleSlot{DayOfWeek: 3, Start: "11:00", End: "14:00"}))
	upNext := s.seedGuideShow(st.ID, "GB", s.nySched(
		catalogm.RadioScheduleSlot{DayOfWeek: 3, Start: "14:00", End: "15:00"}))
	// Later today — beaten to UP NEXT by GB (one row per station).
	s.seedGuideShow(st.ID, "GC", s.nySched(
		catalogm.RadioScheduleSlot{DayOfWeek: 3, Start: "15:00", End: "16:00"}))
	// Thu 13:00 ET = 25h out — beyond the 24h UP NEXT horizon.
	s.seedGuideShow(st.ID, "GD", s.nySched(
		catalogm.RadioScheduleSlot{DayOfWeek: 4, Start: "13:00", End: "14:00"}))

	guide, err := s.svc.GetRadioGuide(guideNow)
	s.Require().NoError(err)

	s.Require().Len(guide.OnNow, 1)
	s.Equal(onNow.ID, guide.OnNow[0].Show.ID)
	s.Equal("test-guide-a", guide.OnNow[0].Station.Slug)
	s.Equal("America/New_York", guide.OnNow[0].StationTimezone)
	s.True(guide.OnNow[0].StartsAt.Equal(time.Date(2026, 7, 8, 15, 0, 0, 0, time.UTC)),
		"11:00 EDT == 15:00 UTC; got %v", guide.OnNow[0].StartsAt)
	s.True(guide.OnNow[0].EndsAt.Equal(time.Date(2026, 7, 8, 18, 0, 0, 0, time.UTC)))

	s.Require().Len(guide.UpNext, 1, "one UP NEXT row per station — the earliest")
	s.Equal(upNext.ID, guide.UpNext[0].Show.ID)
	s.True(guide.UpNext[0].StartsAt.Equal(time.Date(2026, 7, 8, 18, 0, 0, 0, time.UTC)))
}

// An overnight wrap (Tue 23:00–03:00 ET) is still ON NOW after station-local
// midnight — the yesterday-expansion the guide shares with WindowForDate's
// wrap convention.
func (s *RadioSyncSuite) TestRadioGuide_OvernightWrapOnNow() {
	st := s.seedGuideStation("test-guide-wrap")
	wrap := s.seedGuideShow(st.ID, "GW", s.nySched(
		catalogm.RadioScheduleSlot{DayOfWeek: 2, Start: "23:00", End: "03:00"}))

	// Wed 2026-07-08 04:00 UTC == Wed 00:00 EDT — inside Tuesday's wrap.
	now := time.Date(2026, 7, 8, 4, 0, 0, 0, time.UTC)
	guide, err := s.svc.GetRadioGuide(now)
	s.Require().NoError(err)

	s.Require().Len(guide.OnNow, 1)
	s.Equal(wrap.ID, guide.OnNow[0].Show.ID)
	s.True(guide.OnNow[0].EndsAt.Equal(time.Date(2026, 7, 8, 7, 0, 0, 0, time.UTC)),
		"wrap ends Wed 03:00 EDT == 07:00 UTC; got %v", guide.OnNow[0].EndsAt)
}

// Dormant shows and inactive stations contribute nothing; a station with no
// scheduled shows (KEXP/NTS today) is simply absent — the honesty contract.
func (s *RadioSyncSuite) TestRadioGuide_ExcludesDormantAndInactive() {
	live := s.seedGuideStation("test-guide-live")
	s.seedGuideShow(live.ID, "GL", s.nySched(
		catalogm.RadioScheduleSlot{DayOfWeek: 3, Start: "11:00", End: "14:00"}))

	ext := "GX"
	raw, mErr := json.Marshal(s.nySched(catalogm.RadioScheduleSlot{DayOfWeek: 3, Start: "11:00", End: "14:00"}))
	s.Require().NoError(mErr)
	rawMsg := json.RawMessage(raw)
	dormantShow := catalogm.RadioShow{
		StationID: live.ID, Name: "Dormant", Slug: "guide-dormant",
		ExternalID: &ext, IsActive: true, LifecycleState: catalogm.RadioLifecycleDormant,
		Schedule: &rawMsg,
	}
	s.Require().NoError(s.db.Create(&dormantShow).Error)

	off := s.seedGuideStation("test-guide-off")
	s.Require().NoError(s.db.Model(&catalogm.RadioStation{}).Where("id = ?", off.ID).
		Update("is_active", false).Error)
	s.seedGuideShow(off.ID, "GO", s.nySched(
		catalogm.RadioScheduleSlot{DayOfWeek: 3, Start: "11:00", End: "14:00"}))

	// Admin-deactivated (legacy is_active=false) but lifecycle-active — the
	// admin toggle must still remove it from the public guide.
	toggledOff := s.seedGuideShow(live.ID, "GT", s.nySched(
		catalogm.RadioScheduleSlot{DayOfWeek: 3, Start: "11:00", End: "14:00"}))
	s.Require().NoError(s.db.Model(&catalogm.RadioShow{}).Where("id = ?", toggledOff.ID).
		Update("is_active", false).Error)

	// A degenerate End==Start slot expands to 24h — past the 12h trust
	// ceiling the frontend renderers share — and must be dropped, not
	// claimed as a day-long ON NOW.
	s.seedGuideShow(live.ID, "GZ", s.nySched(
		catalogm.RadioScheduleSlot{DayOfWeek: 3, Start: "12:00", End: "12:00"}))

	guide, err := s.svc.GetRadioGuide(guideNow)
	s.Require().NoError(err)

	s.Require().Len(guide.OnNow, 1, "only the active show on the active station")
	s.Equal("test-guide-live", guide.OnNow[0].Station.Slug)
}
