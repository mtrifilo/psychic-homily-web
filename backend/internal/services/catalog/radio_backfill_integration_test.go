package catalog

import (
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// PSY-1154 post-air backfill: these tests run against the same testcontainers
// Postgres as RadioSyncSuite (methods on the suite type can span files), so the
// CHECK constraints, the new playlist_fetch_attempts column, and the GORM join in
// ListBackfillCandidates are exercised for real.

func (s *RadioSyncSuite) seedBackfillStation() catalogm.RadioStation {
	src := catalogm.PlaylistSourceKEXP
	st := catalogm.RadioStation{
		Name:           "Backfill Station",
		Slug:           "backfill-station",
		BroadcastType:  catalogm.BroadcastTypeInternet,
		PlaylistSource: &src,
	}
	s.Require().NoError(s.db.Create(&st).Error)
	return st
}

func (s *RadioSyncSuite) seedShowFor(stationID uint, name, slug, ext string) catalogm.RadioShow {
	show := catalogm.RadioShow{StationID: stationID, Name: name, Slug: slug, ExternalID: &ext}
	s.Require().NoError(s.db.Create(&show).Error)
	return show
}

func (s *RadioSyncSuite) seedEpisodeFor(showID uint, ext, airDate, state string, attempts int, starts, ends *time.Time, now time.Time) catalogm.RadioEpisode {
	ep := catalogm.RadioEpisode{
		ShowID:                showID,
		ExternalID:            &ext,
		AirDate:               airDate,
		PlaylistState:         state,
		PlaylistFetchAttempts: attempts,
		StartsAt:              starts,
		EndsAt:                ends,
		Status:                catalogm.ComputeEpisodeStatus(starts, ends, state, now),
	}
	s.Require().NoError(s.db.Create(&ep).Error)
	return ep
}

func (s *RadioSyncSuite) reloadEpisode(id uint) catalogm.RadioEpisode {
	var ep catalogm.RadioEpisode
	s.Require().NoError(s.db.First(&ep, id).Error)
	return ep
}

// recentAiredBackfillFixture picks the air date for the end-to-end heal test: the
// ET calendar day before etNow, and the schedule weekday that matches it.
//
// RunBackfillCycleNow reads the wall clock — it passes time.Now() into
// ListBackfillCandidates — so the air date must satisfy both bounds that query
// applies against a `now` the test cannot control:
//
//   - the 7-day backfill lookback (air_date >= now-7d), and
//   - the give-up deadline (PSY-1562): a windowless episode past
//     air_date + RadioAirDateZoneSlack + RadioPlaylistGiveUpAfter is terminal and is
//     never re-listed, so the heal never fires and nothing is fetched.
//
// The earlier fixture picked the most recent SUNDAY, which honoured the lookback
// alone and is up to 6 days old. It passed on the day PSY-1558 landed and began
// failing the following Wednesday, four days a week. TestRecentAiredBackfillFixture
// holds this one to the give-up deadline explicitly so a future tightening fails
// there, deterministically, instead of here on a calendar.
//
// Yesterday rather than today because yesterday is the newest date that has
// unconditionally AIRED: the F4 shape's slot is 03:00–06:00 ET, which today has not
// reached when the suite runs before 6am ET.
func recentAiredBackfillFixture(etNow time.Time) (airDate string, episodeNow time.Time, slotDayOfWeek int) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		panic(err)
	}
	etNow = etNow.In(ny)

	d := etNow.AddDate(0, 0, -1)
	day := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, ny)

	// Noon ET — well after the 3–6am slot used by the F4 schedule shape.
	episodeNow = time.Date(day.Year(), day.Month(), day.Day(), 12, 0, 0, 0, ny)
	return day.Format("2006-01-02"), episodeNow, int(day.Weekday())
}

// recentAiredBackfillFixture is a pure function of etNow, so the bound it has to
// respect is checked with a fake clock instead of on whichever day CI happens to run.
//
// The load-bearing case is "worst case within the ET day": a windowless episode's
// give-up deadline is measured from air_date parsed at UTC MIDNIGHT, and the gap
// between that and wall-clock now is widest at 23:59 ET — when UTC has already rolled
// into the next day. That is the moment that must still fit inside the deadline, and
// the moment the Sunday fixture blew past.
func TestRecentAiredBackfillFixture(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name        string
		etNow       time.Time
		wantAirDate string
	}{
		{"midnight ET picks yesterday", time.Date(2026, 7, 28, 0, 0, 0, 0, ny), "2026-07-27"},
		{"before the 6am slot boundary", time.Date(2026, 7, 28, 5, 59, 0, 0, ny), "2026-07-27"},
		{"noon ET picks yesterday", time.Date(2026, 7, 28, 12, 0, 0, 0, ny), "2026-07-27"},
		{"last minute of the ET day", time.Date(2026, 7, 28, 23, 59, 0, 0, ny), "2026-07-27"},
		{"across a month boundary", time.Date(2026, 8, 1, 12, 0, 0, 0, ny), "2026-07-31"},
		{"across a year boundary", time.Date(2027, 1, 1, 12, 0, 0, 0, ny), "2026-12-31"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			airDate, episodeNow, slotDayOfWeek := recentAiredBackfillFixture(tc.etNow)
			if airDate != tc.wantAirDate {
				t.Errorf("airDate = %s, want %s", airDate, tc.wantAirDate)
			}

			day, err := time.ParseInLocation("2006-01-02", airDate, ny)
			if err != nil {
				t.Fatalf("airDate %q does not parse: %v", airDate, err)
			}
			if slotDayOfWeek != int(day.Weekday()) {
				t.Errorf("slotDayOfWeek = %d, want %d (%s) — the schedule slot must match the air date's weekday",
					slotDayOfWeek, int(day.Weekday()), day.Weekday())
			}
			wantEpisodeNow := time.Date(day.Year(), day.Month(), day.Day(), 12, 0, 0, 0, ny)
			if !episodeNow.Equal(wantEpisodeNow) {
				t.Errorf("episodeNow = %s, want noon ET %s", episodeNow, wantEpisodeNow)
			}

			// Reproduce exactly what ListBackfillCandidates compares: parseImportDate
			// parses air_date with no zone, i.e. UTC midnight, against wall-clock now.
			airUTC, err := time.Parse("2006-01-02", airDate)
			if err != nil {
				t.Fatalf("airDate %q does not parse as UTC: %v", airDate, err)
			}
			age := tc.etNow.Sub(airUTC)
			deadline := catalogm.RadioAirDateZoneSlack + catalogm.RadioPlaylistGiveUpAfter
			if age > deadline {
				t.Errorf("air date %s is %s old at %s — past the windowless give-up deadline (%s), "+
					"so the give-up is terminal and the heal can never fire. The fixture can no "+
					"longer reach the deadline with a whole-day air_date; inject a clock into the "+
					"backfill sweep instead.",
					airDate, age, tc.etNow, deadline)
			}
			if age < 0 {
				t.Errorf("air date %s is in the future of etNow %s", airDate, tc.etNow)
			}
			// The lookback is the tighter of the two bounds now; assert it too so the
			// fixture stays honest whichever one moves.
			if age > 7*24*time.Hour {
				t.Errorf("air date %s is %s old at %s — outside the 7-day backfill lookback",
					airDate, age, tc.etNow)
			}
		})
	}
}

// recordPlaylistOutcome on an aired episode that returned plays settles it to
// complete + archived, refreshes play_count, stamps fetched_at, and leaves the
// attempt counter untouched.
func (s *RadioSyncSuite) TestRecordPlaylistOutcome_AiredWithPlays_Complete() {
	now := time.Now()
	start, end := now.Add(-3*time.Hour), now.Add(-1*time.Hour)
	st := s.seedBackfillStation()
	show := s.seedShowFor(st.ID, "Complete Show", "complete-show", "ext-complete")
	ep := s.seedEpisodeFor(show.ID, "ep-complete", now.Format("2006-01-02"),
		catalogm.RadioPlaylistStatePending, 0, &start, &end, now)

	s.Require().NoError(s.svc.recordPlaylistOutcome(&ep, 16, false, now))

	got := s.reloadEpisode(ep.ID)
	s.Equal(catalogm.RadioPlaylistStateComplete, got.PlaylistState)
	s.Equal(catalogm.RadioEpisodeStatusArchived, got.Status, "complete + aired → archived")
	s.Equal(16, got.PlayCount)
	s.Equal(0, got.PlaylistFetchAttempts, "a successful fetch never burns an attempt")
	s.Require().NotNil(got.PlaylistFetchedAt)
}

// recordPlaylistOutcome on a live episode that returned plays settles it to
// partial (the playlist is still growing) without burning an attempt.
func (s *RadioSyncSuite) TestRecordPlaylistOutcome_LiveWithPlays_Partial() {
	now := time.Now()
	start, end := now.Add(-30*time.Minute), now.Add(30*time.Minute)
	st := s.seedBackfillStation()
	show := s.seedShowFor(st.ID, "Live Show", "live-show", "ext-live")
	ep := s.seedEpisodeFor(show.ID, "ep-live", now.Format("2006-01-02"),
		catalogm.RadioPlaylistStatePending, 0, &start, &end, now)

	s.Require().NoError(s.svc.recordPlaylistOutcome(&ep, 4, false, now))

	got := s.reloadEpisode(ep.ID)
	s.Equal(catalogm.RadioPlaylistStatePartial, got.PlaylistState)
	s.Equal(catalogm.RadioEpisodeStatusLive, got.Status)
	s.Equal(0, got.PlaylistFetchAttempts)
}

// A failed post-air attempt increments the counter and stamps fetched_at but does
// NOT touch play_count; reaching the cap flips the episode to unavailable.
func (s *RadioSyncSuite) TestRecordPlaylistOutcome_AiredEmpty_IncrementsThenUnavailable() {
	now := time.Now()
	start, end := now.Add(-3*time.Hour), now.Add(-1*time.Hour)
	st := s.seedBackfillStation()
	show := s.seedShowFor(st.ID, "Empty Show", "empty-show", "ext-empty")

	// First failed attempt from a fresh episode → pending, attempts=1, no play_count.
	ep := s.seedEpisodeFor(show.ID, "ep-empty", now.Format("2006-01-02"),
		catalogm.RadioPlaylistStatePending, 0, &start, &end, now)
	s.Require().NoError(s.svc.recordPlaylistOutcome(&ep, 0, true, now))
	got := s.reloadEpisode(ep.ID)
	s.Equal(catalogm.RadioPlaylistStatePending, got.PlaylistState)
	s.Equal(1, got.PlaylistFetchAttempts)
	s.Equal(0, got.PlayCount)
	s.Require().NotNil(got.PlaylistFetchedAt)

	s.Require().NotNil(got.PlaylistBackfillAttemptedAt,
		"a post-air attempt must stamp the cooldown memo so the next sweep skips it (PSY-1562)")

	// Seed the last-before-ceiling attempt; one more failure → unavailable.
	ep2 := s.seedEpisodeFor(show.ID, "ep-exhaust", now.Format("2006-01-02"),
		catalogm.RadioPlaylistStatePending, catalogm.RadioBackfillMaxAttempts-1, &start, &end, now)
	s.Require().NoError(s.svc.recordPlaylistOutcome(&ep2, 0, true, now))
	got2 := s.reloadEpisode(ep2.ID)
	s.Equal(catalogm.RadioPlaylistStateUnavailable, got2.PlaylistState)
	s.Equal(catalogm.RadioBackfillMaxAttempts, got2.PlaylistFetchAttempts)
}

// End-to-end PSY-1558 acceptance criterion: an episode found empty is not re-fetched on
// the next cycle. Two consecutive backfill cycles against a provider that returns no
// playlist must produce exactly ONE fetch — the second cycle is blocked by the cooldown
// memo the first one stamped. This is the loop that ran unchecked on production.
func (s *RadioSyncSuite) TestBackfillCycle_EmptyEpisodeIsNotRefetchedNextCycle() {
	now := time.Now()
	start, end := now.Add(-3*time.Hour), now.Add(-1*time.Hour)
	airDate := now.Format("2006-01-02")
	showExt, epExt := "ext-empty-loop", "ep-empty-loop"

	st := s.seedBackfillStation()
	show := s.seedShowFor(st.ID, "Empty Loop Show", "empty-loop-show", showExt)
	ep := s.seedEpisodeFor(show.ID, epExt, airDate, catalogm.RadioPlaylistStatePending, 0, &start, &end, now)

	var fetchPlaylistCalls int
	s.svc.playlistProviderFactory = func(string) (RadioPlaylistProvider, error) {
		return &mockPlaylistProvider{
			fetchNewEpisodesFn: func(string, time.Time, time.Time) ([]RadioEpisodeImport, error) {
				return []RadioEpisodeImport{{
					ExternalID: epExt, ShowExternalID: showExt, AirDate: airDate,
					StartsAt: &start, EndsAt: &end,
				}}, nil
			},
			// The upstream tracklist has not been published — the empty-forever case.
			fetchPlaylistFn: func(string) ([]RadioPlayImport, error) {
				fetchPlaylistCalls++
				return nil, nil
			},
		}, nil
	}
	defer func() { s.svc.playlistProviderFactory = nil }()

	fetchSvc := &RadioFetchService{
		radioService:         s.svc,
		stopCh:               make(chan struct{}),
		logger:               slog.Default(),
		backfillInterval:     time.Hour,
		backfillLookbackDays: 7,
	}

	fetchSvc.RunBackfillCycleNow()
	s.Require().Equal(1, fetchPlaylistCalls, "the first cycle must attempt the empty episode once")

	got := s.reloadEpisode(ep.ID)
	s.Equal(1, got.PlaylistFetchAttempts)
	s.Require().NotNil(got.PlaylistBackfillAttemptedAt, "the attempt must be memoized")

	fetchSvc.RunBackfillCycleNow()
	s.Equal(1, fetchPlaylistCalls,
		"the next cycle must NOT re-fetch an episode just found empty — PSY-1558's unmet acceptance criterion")

	got = s.reloadEpisode(ep.ID)
	s.Equal(1, got.PlaylistFetchAttempts, "a skipped cycle must not burn an attempt either")
}

// ListBackfillCandidates returns exactly the shows with aired, still-incomplete
// episodes within the lookback — grouped into one [min,max] air-date window each —
// and excludes complete, live, exhausted, and out-of-window episodes.
func (s *RadioSyncSuite) TestListBackfillCandidates_FiltersAndGroups() {
	now := time.Now()
	today := now.Format("2006-01-02")
	twoDaysAgo := now.AddDate(0, 0, -2).Format("2006-01-02")
	tenDaysAgo := now.AddDate(0, 0, -10).Format("2006-01-02")

	airedStart, airedEnd := now.Add(-3*time.Hour), now.Add(-1*time.Hour)
	oldStart, oldEnd := now.AddDate(0, 0, -2), now.AddDate(0, 0, -2).Add(2*time.Hour)
	wayOldStart, wayOldEnd := now.AddDate(0, 0, -10), now.AddDate(0, 0, -10).Add(2*time.Hour)
	liveStart, liveEnd := now.Add(-30*time.Minute), now.Add(30*time.Minute)

	st := s.seedBackfillStation()

	// showA: two aired incomplete episodes (today + 2d ago) → ONE candidate spanning both.
	showA := s.seedShowFor(st.ID, "Show A", "show-a", "ext-a")
	s.seedEpisodeFor(showA.ID, "a-today", today, catalogm.RadioPlaylistStatePending, 0, &airedStart, &airedEnd, now)
	s.seedEpisodeFor(showA.ID, "a-2d", twoDaysAgo, catalogm.RadioPlaylistStatePartial, 1, &oldStart, &oldEnd, now)

	// showB: aired but complete → excluded (SQL state filter).
	showB := s.seedShowFor(st.ID, "Show B", "show-b", "ext-b")
	s.seedEpisodeFor(showB.ID, "b-today", today, catalogm.RadioPlaylistStateComplete, 0, &airedStart, &airedEnd, now)

	// showC: incomplete but still live → excluded (Go aired predicate).
	showC := s.seedShowFor(st.ID, "Show C", "show-c", "ext-c")
	s.seedEpisodeFor(showC.ID, "c-live", today, catalogm.RadioPlaylistStatePending, 0, &liveStart, &liveEnd, now)

	// showD: aired incomplete but attempts at the cap → excluded (SQL attempts filter).
	showD := s.seedShowFor(st.ID, "Show D", "show-d", "ext-d")
	s.seedEpisodeFor(showD.ID, "d-exhausted", today, catalogm.RadioPlaylistStatePending,
		catalogm.RadioBackfillMaxAttempts, &airedStart, &airedEnd, now)

	// showE: aired incomplete but beyond the 7-day lookback → excluded (SQL air_date filter).
	showE := s.seedShowFor(st.ID, "Show E", "show-e", "ext-e")
	s.seedEpisodeFor(showE.ID, "e-old", tenDaysAgo, catalogm.RadioPlaylistStatePending, 0, &wayOldStart, &wayOldEnd, now)

	candidates, err := s.svc.ListBackfillCandidates(7*24*time.Hour, now)
	s.Require().NoError(err)
	s.Require().Len(candidates, 1, "only showA has eligible aired-incomplete episodes")

	c := candidates[0]
	s.Equal(showA.ID, c.ShowID)
	s.Equal(st.ID, c.StationID)
	s.Equal(twoDaysAgo, c.Since.Format("2006-01-02"), "window starts at the earliest incomplete episode")
	s.Equal(today, c.Until.Format("2006-01-02"), "window ends at the latest incomplete episode")
}

// PSY-1287: a windowless aired episode that falsely gave up ('unavailable') is still
// discovered as a backfill candidate so a re-list can heal the window and fetch. Under
// PSY-1562 the 'unavailable' LABEL is not what excludes it — the give-up deadline is,
// and this episode is inside it.
func (s *RadioSyncSuite) TestListBackfillCandidates_IncludesStrandedWindowlessUnavailable() {
	now := time.Now()
	airDate := now.AddDate(0, 0, -1).Format("2006-01-02")

	st := s.seedBackfillStation()
	show := s.seedShowFor(st.ID, "Stranded F4", "stranded-f4", "ext-f4")
	s.seedEpisodeFor(show.ID, "ep-stranded", airDate, catalogm.RadioPlaylistStateUnavailable, 5, nil, nil, now)

	candidates, err := s.svc.ListBackfillCandidates(7*24*time.Hour, now)
	s.Require().NoError(err)
	s.Require().Len(candidates, 1)
	s.Equal(show.ID, candidates[0].ShowID)
}

// PSY-1558/PSY-1562: the same stranded windowless episode stops being a candidate once
// its give-up deadline passes — the production loop, where an episode whose playlist was
// never published upstream sat inside the 7-day lookback being re-selected forever.
func (s *RadioSyncSuite) TestListBackfillCandidates_ExcludesWindowlessPastGiveUpDeadline() {
	now := time.Now()
	staleAir := now.Add(-catalogm.RadioAirDateZoneSlack - catalogm.RadioPlaylistGiveUpAfter - 24*time.Hour)

	st := s.seedBackfillStation()
	show := s.seedShowFor(st.ID, "Stale Stranded", "stale-stranded", "ext-stale")
	s.seedEpisodeFor(show.ID, "ep-stale-stranded", staleAir.Format("2006-01-02"),
		catalogm.RadioPlaylistStateUnavailable, 5, nil, nil, now)

	candidates, err := s.svc.ListBackfillCandidates(7*24*time.Hour, now)
	s.Require().NoError(err)
	s.Empty(candidates, "a windowless give-up past its deadline is final")
}

// PSY-1562's core acceptance criterion at the query level: an episode whose post-air
// attempt just ran is NOT re-selected by the next sweep, and IS once the cooldown has
// elapsed. Before this, 23 stranded episodes were re-listed on every hourly sweep,
// producing 269 zero-yield backfill runs a day on production.
func (s *RadioSyncSuite) TestListBackfillCandidates_CooldownSkipsJustAttemptedEpisode() {
	now := time.Now()
	start, end := now.Add(-3*time.Hour), now.Add(-1*time.Hour)

	st := s.seedBackfillStation()
	show := s.seedShowFor(st.ID, "Cooldown Show", "cooldown-show", "ext-cooldown")
	ep := s.seedEpisodeFor(show.ID, "ep-cooldown", now.Format("2006-01-02"),
		catalogm.RadioPlaylistStatePending, 1, &start, &end, now)

	candidates, err := s.svc.ListBackfillCandidates(7*24*time.Hour, now)
	s.Require().NoError(err)
	s.Require().Len(candidates, 1, "with no attempt recorded the episode is eligible")

	// Record an attempt as of now — exactly what recordPlaylistOutcome writes.
	s.Require().NoError(s.db.Model(&ep).Update("playlist_backfill_attempted_at", now).Error)

	candidates, err = s.svc.ListBackfillCandidates(7*24*time.Hour, now.Add(time.Hour))
	s.Require().NoError(err)
	s.Empty(candidates, "an episode attempted an hour ago must not be re-selected by the next hourly sweep")

	candidates, err = s.svc.ListBackfillCandidates(7*24*time.Hour, now.Add(catalogm.RadioPlaylistRetryCooldown))
	s.Require().NoError(err)
	s.Require().Len(candidates, 1, "once the cooldown elapses the episode is eligible again")
}

// End-to-end PSY-1287 (F4 shape): windowless + unavailable after a false give-up,
// corrected early-morning schedule → backfill heals the window and imports the playlist.
//
// The slot weekday comes from the fixture rather than being pinned to Sunday: the heal
// matches a slot against the air date's own weekday (RadioSchedule window derivation),
// and the F4 shape lives in the pre-6am slot, not in the day it falls on.
func (s *RadioSyncSuite) TestBackfillCycle_HealsWindowlessUnavailableAfterScheduleFix() {
	ny, err := time.LoadLocation("America/New_York")
	s.Require().NoError(err)
	airDate, now, slotDayOfWeek := recentAiredBackfillFixture(time.Now().In(ny))
	showExt, epExt := "F4", "ep-f4-heal"

	schedRaw, _ := json.Marshal(catalogm.RadioSchedule{
		Timezone: "America/New_York",
		Slots:    []catalogm.RadioScheduleSlot{{DayOfWeek: slotDayOfWeek, Start: "03:00", End: "06:00"}},
	})
	schedule := json.RawMessage(schedRaw)

	st := s.seedStation(catalogm.PlaylistSourceWFMU)
	show := s.seedShowWithSchedule(st.ID, "Freeform Jazz Dance", "freeform-jazz-dance", showExt, &schedule)
	ep := s.seedEpisodeFor(show.ID, epExt, airDate, catalogm.RadioPlaylistStateUnavailable, 5, nil, nil, now)

	var fetchPlaylistCalls int
	track := "Groovy Track"
	s.svc.playlistProviderFactory = func(string) (RadioPlaylistProvider, error) {
		return &mockPlaylistProvider{
			fetchNewEpisodesFn: func(string, time.Time, time.Time) ([]RadioEpisodeImport, error) {
				return []RadioEpisodeImport{{
					ExternalID:     epExt,
					ShowExternalID: showExt,
					AirDate:        airDate,
				}}, nil
			},
			fetchPlaylistFn: func(string) ([]RadioPlayImport, error) {
				fetchPlaylistCalls++
				return []RadioPlayImport{{Position: 1, ArtistName: "Jazz Dancer", TrackTitle: &track}}, nil
			},
		}, nil
	}
	defer func() { s.svc.playlistProviderFactory = nil }()

	fetchSvc := &RadioFetchService{
		radioService:         s.svc,
		stopCh:               make(chan struct{}),
		logger:               slog.Default(),
		backfillInterval:     time.Hour,
		backfillLookbackDays: 7,
	}
	fetchSvc.RunBackfillCycleNow()

	s.Positive(fetchPlaylistCalls, "stranded windowless episode must be re-fetched after schedule heal")
	got := s.reloadEpisode(ep.ID)
	s.Require().NotNil(got.StartsAt, "window should be healed from the corrected schedule")
	s.Equal(catalogm.RadioPlaylistStateComplete, got.PlaylistState)
	s.Positive(got.PlayCount)
}

// A successful but EMPTY post-air re-fetch of an episode that already has plays must
// NOT zero its play_count (radio_plays is append-only; the rows still exist). It still
// burns an attempt and stays eligible. Regression guard for the play_count clobber.
func (s *RadioSyncSuite) TestRecordPlaylistOutcome_EmptyRefetch_PreservesPlayCount() {
	now := time.Now()
	start, end := now.Add(-3*time.Hour), now.Add(-1*time.Hour)
	st := s.seedBackfillStation()
	show := s.seedShowFor(st.ID, "Preserve Show", "preserve-show", "ext-preserve")
	ep := s.seedEpisodeFor(show.ID, "ep-preserve", now.Format("2006-01-02"),
		catalogm.RadioPlaylistStatePartial, 0, &start, &end, now)
	// Simulate a prior live snapshot of 5 plays already on the row.
	s.Require().NoError(s.db.Model(&ep).Update("play_count", 5).Error)
	ep.PlayCount = 5

	s.Require().NoError(s.svc.recordPlaylistOutcome(&ep, 0, false, now))

	got := s.reloadEpisode(ep.ID)
	s.Equal(5, got.PlayCount, "an empty re-fetch must not zero an episode that already has plays")
	// 'partial' rather than 'pending' (PSY-1562): an episode showing 5 tracks must not
	// be labelled as having no playlist. It stays backfill-eligible either way — the
	// aired branch of PlanPlaylistFetch accepts pending and partial alike.
	s.Equal(catalogm.RadioPlaylistStatePartial, got.PlaylistState, "an episode with plays stays 'partial', and stays eligible")
	s.Equal(1, got.PlaylistFetchAttempts, "empty post-air fetch burns one attempt")
}

// play_count is monotonic: a non-empty but SHORTER re-fetch (10 → 3) must not shrink it
// below the rows already stored.
func (s *RadioSyncSuite) TestRecordPlaylistOutcome_ShortRefetch_DoesNotShrinkPlayCount() {
	now := time.Now()
	start, end := now.Add(-3*time.Hour), now.Add(-1*time.Hour)
	st := s.seedBackfillStation()
	show := s.seedShowFor(st.ID, "Monotonic Show", "monotonic-show", "ext-monotonic")
	ep := s.seedEpisodeFor(show.ID, "ep-monotonic", now.Format("2006-01-02"),
		catalogm.RadioPlaylistStatePartial, 0, &start, &end, now)
	s.Require().NoError(s.db.Model(&ep).Update("play_count", 10).Error)
	ep.PlayCount = 10

	s.Require().NoError(s.svc.recordPlaylistOutcome(&ep, 3, false, now))

	got := s.reloadEpisode(ep.ID)
	s.Equal(10, got.PlayCount, "play_count is monotonic; a shorter re-fetch must not shrink it")
	s.Equal(catalogm.RadioPlaylistStateComplete, got.PlaylistState, "aired + plays → complete")
}

// End-to-end: the backfill sweep (runBackfillCycle) finds an aired-incomplete episode,
// routes through RunStationSync(backfill) → importEpisode's existing-row re-fetch, and
// heals it to complete/archived. Asserts FetchPlaylist is ACTUALLY re-invoked — a guard
// against an inverted eligibility check that the isolated unit tests wouldn't catch.
func (s *RadioSyncSuite) TestBackfillCycle_HealsAiredIncompleteEpisode() {
	now := time.Now()
	start, end := now.Add(-3*time.Hour), now.Add(-1*time.Hour)
	airDate := now.Format("2006-01-02")
	showExt, epExt := "ext-heal", "ep-heal"

	st := s.seedBackfillStation()
	show := s.seedShowFor(st.ID, "Heal Show", "heal-show", showExt)
	ep := s.seedEpisodeFor(show.ID, epExt, airDate, catalogm.RadioPlaylistStatePending, 0, &start, &end, now)

	var fetchPlaylistCalls int
	track := "Heal Track"
	s.svc.playlistProviderFactory = func(string) (RadioPlaylistProvider, error) {
		return &mockPlaylistProvider{
			fetchNewEpisodesFn: func(string, time.Time, time.Time) ([]RadioEpisodeImport, error) {
				return []RadioEpisodeImport{{
					ExternalID:     epExt,
					ShowExternalID: showExt,
					AirDate:        airDate,
					StartsAt:       &start,
					EndsAt:         &end,
				}}, nil
			},
			fetchPlaylistFn: func(string) ([]RadioPlayImport, error) {
				fetchPlaylistCalls++
				return []RadioPlayImport{{Position: 1, ArtistName: "Healer", TrackTitle: &track}}, nil
			},
		}, nil
	}
	defer func() { s.svc.playlistProviderFactory = nil }()

	fetchSvc := &RadioFetchService{
		radioService:         s.svc,
		stopCh:               make(chan struct{}),
		logger:               slog.Default(),
		backfillInterval:     time.Hour,
		backfillLookbackDays: 7,
	}
	fetchSvc.RunBackfillCycleNow()

	s.Positive(fetchPlaylistCalls, "the sweep must actually re-fetch the incomplete aired episode's playlist")
	got := s.reloadEpisode(ep.ID)
	s.Equal(catalogm.RadioPlaylistStateComplete, got.PlaylistState)
	s.Equal(catalogm.RadioEpisodeStatusArchived, got.Status)
	s.Positive(got.PlayCount)
}
