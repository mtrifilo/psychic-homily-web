package catalog

import (
	"fmt"
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// PSY-1274: per-show sustained-outage escalation. The counter half lives in the
// fetch loop (bump on a provider fetch error, reset on success); the alerting half
// is the janitor's EscalateShowFetchFailureStreaks. Suite: RadioSyncSuite
// (radio_sync_integration_test.go, Postgres testcontainer).

// streakOf reads a show's consecutive-fetch-failure counter.
func (s *RadioSyncSuite) streakOf(showID uint) int {
	var show catalogm.RadioShow
	s.Require().NoError(s.db.First(&show, showID).Error)
	return show.ConsecutiveFetchFailures
}

// A show whose provider fetch errors accumulates a streak across runs; its healthy
// sibling stays at zero. A later successful fetch — even one returning ZERO episodes
// (the cadence-independence property: an infrequent show between airings) — resets
// the streak.
func (s *RadioSyncSuite) TestFetch_FailureStreak_BumpsAndResets() {
	st := s.seedStation(catalogm.PlaylistSourceNTS)
	broken := s.seedActiveShow(st.ID, "streak-broken", nil)
	healthy := s.seedActiveShow(st.ID, "streak-healthy", nil)

	brokenErrs := true
	s.svc.playlistProviderFactory = func(string) (RadioPlaylistProvider, error) {
		return &mockPlaylistProvider{
			fetchNewEpisodesFn: func(ext string, _, _ time.Time) ([]RadioEpisodeImport, error) {
				if ext == "streak-broken" && brokenErrs {
					return nil, fmt.Errorf("404 external id gone")
				}
				return nil, nil // success, zero episodes
			},
		}, nil
	}
	defer func() { s.svc.playlistProviderFactory = nil }()

	for i := 1; i <= 2; i++ {
		_, err := s.svc.FetchNewEpisodes(st.ID)
		s.Require().NoError(err)
		s.Equal(i, s.streakOf(broken.ID), "failing show's streak after run %d", i)
		s.Equal(0, s.streakOf(healthy.ID), "healthy sibling must stay at zero")
	}

	brokenErrs = false
	_, err := s.svc.FetchNewEpisodes(st.ID)
	s.Require().NoError(err)
	s.Equal(0, s.streakOf(broken.ID), "a successful zero-episode fetch must reset the streak")
}

// A MANUAL fetch run never bumps the streak — the counter means "consecutive
// SCHEDULED cycles failed", so an admin re-running a flapping station three times in
// ten minutes must not fabricate an ~18h sustained outage. A manual SUCCESS still
// resets (a verification run after fixing the external_id clears the condition).
func (s *RadioSyncSuite) TestFetch_FailureStreak_ManualRunsDoNotBump() {
	st := s.seedStation(catalogm.PlaylistSourceNTS)
	broken := s.seedActiveShow(st.ID, "manual-broken", nil)

	errs := true
	s.svc.playlistProviderFactory = func(string) (RadioPlaylistProvider, error) {
		return &mockPlaylistProvider{
			fetchNewEpisodesFn: func(string, time.Time, time.Time) ([]RadioEpisodeImport, error) {
				if errs {
					return nil, fmt.Errorf("404 external id gone")
				}
				return nil, nil
			},
		}, nil
	}
	defer func() { s.svc.playlistProviderFactory = nil }()

	for range 3 {
		_, err := s.svc.fetchNewEpisodes(st.ID, catalogm.RadioSyncRunTriggerManual)
		s.Require().NoError(err)
	}
	s.Equal(0, s.streakOf(broken.ID), "manual-run failures must not inflate the streak")

	s.setShowStreak(broken.ID, 2)
	errs = false
	_, err := s.svc.fetchNewEpisodes(st.ID, catalogm.RadioSyncRunTriggerManual)
	s.Require().NoError(err)
	s.Equal(0, s.streakOf(broken.ID), "a successful manual fetch must still reset the streak")
}

// setShowStreak stamps a show's counter directly (the janitor half is tested
// independently of the fetch loop that maintains it).
func (s *RadioSyncSuite) setShowStreak(showID uint, n int) {
	s.Require().NoError(s.db.Model(&catalogm.RadioShow{}).Where("id = ?", showID).
		UpdateColumn("consecutive_fetch_failures", n).Error)
}

// EscalateShowFetchFailureStreaks escalates ONLY an at-threshold show on a healthy,
// automated station: below-threshold, station-in-outage (covered by the PSY-1269
// station escalation), manual-source, never-fetched-station, and inactive-show
// candidates are all left alone.
func (s *RadioSyncSuite) TestEscalateShowFetchFailureStreaks() {
	now := time.Now()
	threshold := radioShowFetchFailureEscalationThreshold
	stationOutage := 18 * time.Hour

	healthy := s.seedStation(catalogm.PlaylistSourceNTS)
	s.setLastFetch(healthy.ID, now.Add(-1*time.Hour))
	escalated := s.seedActiveShow(healthy.ID, "esc-at-threshold", nil)
	s.setShowStreak(escalated.ID, threshold)
	below := s.seedActiveShow(healthy.ID, "esc-below", nil)
	s.setShowStreak(below.ID, threshold-1)

	inactiveShow := s.seedActiveShow(healthy.ID, "esc-inactive-show", nil)
	s.setShowStreak(inactiveShow.ID, threshold)
	s.Require().NoError(s.db.Model(&catalogm.RadioShow{}).Where("id = ?", inactiveShow.ID).
		Update("is_active", false).Error)

	noExt := s.seedActiveShow(healthy.ID, "esc-no-ext", nil)
	s.setShowStreak(noExt.ID, threshold)
	s.Require().NoError(s.db.Model(&catalogm.RadioShow{}).Where("id = ?", noExt.ID).
		Update("external_id", "").Error)

	// Retired = the documented remediation for a permanently-gone external_id
	// (PSY-1152); it must quiesce the alert even though the legacy is_active polling
	// gate may still be true and the streak keeps climbing.
	retired := s.seedActiveShow(healthy.ID, "esc-retired", nil)
	s.setShowStreak(retired.ID, threshold)
	s.Require().NoError(s.db.Model(&catalogm.RadioShow{}).Where("id = ?", retired.ID).
		Update("lifecycle_state", catalogm.RadioLifecycleRetired).Error)

	// Station itself in sustained outage → its shows are the station escalation's
	// problem, not per-show noise.
	outage := s.seedStation(catalogm.PlaylistSourceKEXP)
	s.setLastFetch(outage.ID, now.Add(-24*time.Hour))
	onOutage := s.seedActiveShow(outage.ID, "esc-on-outage-station", nil)
	s.setShowStreak(onOutage.ID, threshold)

	manual := s.seedStation(catalogm.PlaylistSourceManual)
	s.setLastFetch(manual.ID, now.Add(-1*time.Hour))
	onManual := s.seedActiveShow(manual.ID, "esc-on-manual", nil)
	s.setShowStreak(onManual.ID, threshold)

	// Never-fetched station (NULL watermark) → excluded, same carve-out as PSY-1269.
	neverFetched := s.seedStation(catalogm.PlaylistSourceWFMU)
	onNever := s.seedActiveShow(neverFetched.ID, "esc-on-never-fetched", nil)
	s.setShowStreak(onNever.ID, threshold)

	type escalation struct {
		showID   uint
		category string
	}
	var got []escalation
	s.svc.onShowPermanentFailure = func(_ error, showID uint, category string) {
		got = append(got, escalation{showID, category})
	}
	defer func() { s.svc.onShowPermanentFailure = nil }()

	count, err := s.svc.EscalateShowFetchFailureStreaks(threshold, stationOutage, now)
	s.Require().NoError(err)
	s.Equal(1, count, "only the at-threshold show on the healthy automated station escalates")
	s.Require().Len(got, 1)
	s.Equal(escalated.ID, got[0].showID)
	s.Equal(radioShowFetchOutageCategory, got[0].category)
}
