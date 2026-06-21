package catalog

import (
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

	// Seed the last-before-cap attempt; one more failure → unavailable.
	ep2 := s.seedEpisodeFor(show.ID, "ep-exhaust", now.Format("2006-01-02"),
		catalogm.RadioPlaylistStatePending, catalogm.RadioBackfillMaxAttempts-1, &start, &end, now)
	s.Require().NoError(s.svc.recordPlaylistOutcome(&ep2, 0, true, now))
	got2 := s.reloadEpisode(ep2.ID)
	s.Equal(catalogm.RadioPlaylistStateUnavailable, got2.PlaylistState)
	s.Equal(catalogm.RadioBackfillMaxAttempts, got2.PlaylistFetchAttempts)
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

	candidates, err := s.svc.ListBackfillCandidates(7*24*time.Hour, catalogm.RadioBackfillMaxAttempts, now)
	s.Require().NoError(err)
	s.Require().Len(candidates, 1, "only showA has eligible aired-incomplete episodes")

	c := candidates[0]
	s.Equal(showA.ID, c.ShowID)
	s.Equal(st.ID, c.StationID)
	s.Equal(twoDaysAgo, c.Since.Format("2006-01-02"), "window starts at the earliest incomplete episode")
	s.Equal(today, c.Until.Format("2006-01-02"), "window ends at the latest incomplete episode")
}
