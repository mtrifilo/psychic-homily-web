package catalog

import (
	"context"
	"encoding/json"
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// PSY-1238 schedule→episode-window stamping, against the testcontainers Postgres
// (methods on RadioSyncSuite). WFMU episodes carry a date but no air time, so the
// frozen [starts_at, ends_at] is derived from the show's stored weekly schedule.
// These exercise the create path, the heal-on-relist path (the create-on-first
// "schedule lands later" flow), the resolver's provider-wins / no-schedule
// fallbacks, and the churn guard — for real.

// windowFixture builds an air_date a few days before now (recent → inside the
// scheduleDerivedWindowMaxAgeDays churn guard, and already fully aired), a
// schedule whose only slot matches that day's weekday (9pm–midnight, overnight
// wrap), and the expected [start, end] America/New_York instants. Everything is
// relative to now so the fixture never ages out of the recency guard.
func windowFixture(now time.Time) (airDate string, schedule *json.RawMessage, wantStart, wantEnd time.Time) {
	ny, _ := time.LoadLocation("America/New_York")
	day := now.UTC().AddDate(0, 0, -2) // 2 days ago: recent, and the slot has elapsed
	sched := catalogm.RadioSchedule{
		Timezone: "America/New_York",
		Slots:    []catalogm.RadioScheduleSlot{{DayOfWeek: int(day.Weekday()), Start: "21:00", End: "00:00"}},
	}
	raw, _ := json.Marshal(sched)
	msg := json.RawMessage(raw)
	next := day.AddDate(0, 0, 1)
	wantStart = time.Date(day.Year(), day.Month(), day.Day(), 21, 0, 0, 0, ny)
	wantEnd = time.Date(next.Year(), next.Month(), next.Day(), 0, 0, 0, 0, ny)
	return day.Format("2006-01-02"), &msg, wantStart, wantEnd
}

func (s *RadioSyncSuite) seedShowWithSchedule(stationID uint, name, slug, ext string, schedule *json.RawMessage) catalogm.RadioShow {
	show := catalogm.RadioShow{StationID: stationID, Name: name, Slug: slug, ExternalID: &ext, Schedule: schedule}
	s.Require().NoError(s.db.Create(&show).Error)
	return show
}

// A newly-created WFMU episode (no provider window) is stamped with the
// schedule-derived frozen window on the create path, end to end.
func (s *RadioSyncSuite) TestWindowStamp_CreateFromSchedule() {
	now := time.Now()
	airDate, schedule, wantStart, wantEnd := windowFixture(now)
	st := s.seedStation(catalogm.PlaylistSourceWFMU)
	showExt := "M1"
	show := s.seedShowWithSchedule(st.ID, "Mona", "mona", showExt, schedule)

	s.svc.playlistProviderFactory = func(string) (RadioPlaylistProvider, error) {
		return &mockPlaylistProvider{
			fetchNewEpisodesFn: func(string, time.Time, time.Time) ([]RadioEpisodeImport, error) {
				// WFMU-style: a date, NO provider StartsAt/EndsAt.
				return []RadioEpisodeImport{{ExternalID: "ep-fri", ShowExternalID: showExt, AirDate: airDate}}, nil
			},
			fetchPlaylistFn: func(string) ([]RadioPlayImport, error) { return nil, nil },
		}, nil
	}
	defer func() { s.svc.playlistProviderFactory = nil }()

	ws := now.AddDate(0, 0, -7)
	we := now.AddDate(0, 0, 1)
	_, err := s.svc.RunStationSync(context.Background(), st.ID, RunStationSyncOpts{
		Mode: catalogm.RadioSyncRunTypeBackfill, Trigger: catalogm.RadioSyncRunTriggerManual,
		ShowID: &show.ID, WindowStart: &ws, WindowEnd: &we,
	})
	s.Require().NoError(err)

	var ep catalogm.RadioEpisode
	s.Require().NoError(s.db.Where("show_id = ? AND external_id = ?", show.ID, "ep-fri").First(&ep).Error)
	s.Require().NotNil(ep.StartsAt, "WFMU episode should be windowed from the schedule")
	s.Require().NotNil(ep.EndsAt)
	s.True(ep.StartsAt.Equal(wantStart), "starts 9pm ET, got %v want %v", ep.StartsAt, wantStart)
	s.True(ep.EndsAt.Equal(wantEnd), "ends next-day midnight ET (overnight wrap), got %v want %v", ep.EndsAt, wantEnd)
}

// A pre-existing windowless WFMU episode (imported before its schedule was
// scraped — the PSY-1153 create-on-first ordering) self-heals its frozen window
// from the schedule when re-listed, and its recomputed status is consistent.
func (s *RadioSyncSuite) TestWindowStamp_HealsWindowlessOnRelist() {
	now := time.Now()
	airDate, schedule, wantStart, wantEnd := windowFixture(now)
	st := s.seedStation(catalogm.PlaylistSourceWFMU)
	showExt := "JR"
	show := s.seedShowWithSchedule(st.ID, "Jessica", "jessica", showExt, schedule)

	// A windowless, already-complete episode (StartsAt/EndsAt nil) — the shape of
	// a WFMU row imported before window stamping existed / before the schedule landed.
	existing := s.seedEpisodeFor(show.ID, "ep-old", airDate,
		catalogm.RadioPlaylistStateComplete, 0, nil, nil, now)
	s.Require().Nil(existing.StartsAt)

	_, err := s.svc.reimportExistingEpisode(&existing,
		RadioEpisodeImport{ExternalID: "ep-old", ShowExternalID: showExt, AirDate: airDate},
		&mockPlaylistProvider{}, now) // complete → ShouldBackfillPlaylist false → provider unused
	s.Require().NoError(err)

	got := s.reloadEpisode(existing.ID)
	s.Require().NotNil(got.StartsAt, "windowless episode should heal from the schedule")
	s.True(got.StartsAt.Equal(wantStart), "healed start, got %v want %v", got.StartsAt, wantStart)
	s.True(got.EndsAt.Equal(wantEnd))
	// Past window + complete playlist → archived (no complete-but-scheduled/live contradiction).
	s.Equal(catalogm.RadioEpisodeStatusArchived, got.Status, "complete + aired → archived")
}

// episodeAirWindow: schedule-derived for a recent WFMU episode, provider-window
// wins for KEXP/NTS, nil for a show with no schedule, and nil past the churn guard.
func (s *RadioSyncSuite) TestEpisodeAirWindow_Resolver() {
	now := time.Now()
	airDate, schedule, wantStart, _ := windowFixture(now)
	st := s.seedStation(catalogm.PlaylistSourceWFMU)
	scheduled := s.seedShowWithSchedule(st.ID, "Burn It Down", "burn-it-down", "NK", schedule)
	bare := s.seedShowFor(st.ID, "No Schedule", "no-schedule", "NS")

	s.Run("recent WFMU episode with schedule → derived window", func() {
		start, end := s.svc.episodeAirWindow(scheduled.ID, RadioEpisodeImport{AirDate: airDate}, now)
		s.Require().NotNil(start)
		s.Require().NotNil(end)
		s.True(start.Equal(wantStart))
	})

	s.Run("provider window wins (KEXP/NTS already set it)", func() {
		provStart := now.Add(-3 * time.Hour)
		provEnd := now.Add(-1 * time.Hour)
		start, end := s.svc.episodeAirWindow(scheduled.ID, RadioEpisodeImport{
			AirDate: airDate, StartsAt: &provStart, EndsAt: &provEnd,
		}, now)
		s.Require().NotNil(start)
		s.True(start.Equal(provStart), "provider StartsAt must be returned unchanged")
		s.True(end.Equal(provEnd))
	})

	s.Run("show without a schedule → nil window", func() {
		start, end := s.svc.episodeAirWindow(bare.ID, RadioEpisodeImport{AirDate: airDate}, now)
		s.Nil(start)
		s.Nil(end)
	})

	s.Run("air_date older than the churn guard → nil window (frozen-window safety)", func() {
		old := now.UTC().AddDate(0, 0, -(scheduleDerivedWindowMaxAgeDays + 5)).Format("2006-01-02")
		start, end := s.svc.episodeAirWindow(scheduled.ID, RadioEpisodeImport{AirDate: old}, now)
		s.Nil(start, "an old episode must not be re-windowed from the current (maybe-churned) schedule")
		s.Nil(end)
	})
}
