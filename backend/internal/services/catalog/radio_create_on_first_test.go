package catalog

import (
	"context"
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// PSY-1153 create-on-first-episode + dormant reactivation. Runs against the same
// testcontainers Postgres as RadioSyncSuite (methods span files).

func (s *RadioSyncSuite) countShowsByExternalID(ext string) int64 {
	var n int64
	s.Require().NoError(s.db.Model(&catalogm.RadioShow{}).Where("external_id = ?", ext).Count(&n).Error)
	return n
}

// The discover run itself does create-on-first (under its lock/breaker): a roster show
// that aired in the window is created + its episode imported; an episode-less roster
// show gets no row. Because BOTH the scheduled cycle and the manual admin trigger flow
// through RunStationSync(discover), this one path covers both — manual discover now
// materializes aired shows too (PSY-1153 fix). No separate auto-backfill drain.
func (s *RadioSyncSuite) TestDiscover_CreateOnFirstEpisode() {
	now := time.Now()
	today := now.Format("2006-01-02")
	since, until := now.AddDate(0, 0, -90), now
	st := s.seedBackfillStation()

	s.svc.playlistProviderFactory = func(string) (RadioPlaylistProvider, error) {
		return &mockPlaylistProvider{
			discoverShowsFn: func() ([]RadioShowImport, error) {
				return []RadioShowImport{
					{ExternalID: "show-aired", Name: "Aired Show"},
					{ExternalID: "show-empty", Name: "Empty Show"},
				}, nil
			},
			fetchNewEpisodesFn: func(ext string, _, _ time.Time) ([]RadioEpisodeImport, error) {
				if ext == "show-aired" {
					return []RadioEpisodeImport{{ExternalID: "a-ep1", ShowExternalID: "show-aired", AirDate: today}}, nil
				}
				return nil, nil // show-empty has no episodes in the window
			},
			fetchPlaylistFn: func(string) ([]RadioPlayImport, error) {
				return []RadioPlayImport{{Position: 1, ArtistName: "A"}}, nil
			},
		}, nil
	}
	defer func() { s.svc.playlistProviderFactory = nil }()

	res, err := s.svc.RunStationSync(context.Background(), st.ID, RunStationSyncOpts{
		Mode: catalogm.RadioSyncRunTypeDiscover, Trigger: catalogm.RadioSyncRunTriggerScheduled,
		WindowStart: &since, WindowEnd: &until,
	})
	s.Require().NoError(err)
	s.Require().NotNil(res.Discover)
	s.Equal(2, res.Discover.ShowsNew, "both roster shows are new candidates")
	s.ElementsMatch([]string{"Aired Show"}, res.Discover.CreatedShowNames, "only the aired show is created")

	// Aired show: row created (active) WITH its episode imported (not an empty row).
	s.Equal(int64(1), s.countShowsByExternalID("show-aired"))
	var aired catalogm.RadioShow
	s.Require().NoError(s.db.Where("external_id = ?", "show-aired").First(&aired).Error)
	s.Equal(catalogm.RadioLifecycleActive, aired.LifecycleState)
	var epCount int64
	s.Require().NoError(s.db.Model(&catalogm.RadioEpisode{}).Where("show_id = ?", aired.ID).Count(&epCount).Error)
	s.Positive(epCount, "the first episode is imported with the row — no empty placeholder")

	// Episode-less roster show: never persisted (§9 dec 1).
	s.Equal(int64(0), s.countShowsByExternalID("show-empty"), "episode-less roster show stays invisible")
}

// An episode that aired OUTSIDE the create window does not create a row.
func (s *RadioSyncSuite) TestDiscover_CreateOnFirst_OutsideWindowNotCreated() {
	now := time.Now()
	old := now.AddDate(0, 0, -120).Format("2006-01-02") // older than the 90-day window
	since, until := now.AddDate(0, 0, -90), now
	st := s.seedBackfillStation()

	s.svc.playlistProviderFactory = func(string) (RadioPlaylistProvider, error) {
		return &mockPlaylistProvider{
			discoverShowsFn: func() ([]RadioShowImport, error) {
				return []RadioShowImport{{ExternalID: "show-stale", Name: "Stale Show"}}, nil
			},
			fetchNewEpisodesFn: func(string, time.Time, time.Time) ([]RadioEpisodeImport, error) {
				return []RadioEpisodeImport{{ExternalID: "stale-ep", ShowExternalID: "show-stale", AirDate: old}}, nil
			},
		}, nil
	}
	defer func() { s.svc.playlistProviderFactory = nil }()

	res, err := s.svc.RunStationSync(context.Background(), st.ID, RunStationSyncOpts{
		Mode: catalogm.RadioSyncRunTypeDiscover, Trigger: catalogm.RadioSyncRunTriggerScheduled,
		WindowStart: &since, WindowEnd: &until,
	})
	s.Require().NoError(err)
	s.Empty(res.Discover.CreatedShowNames)
	s.Equal(int64(0), s.countShowsByExternalID("show-stale"), "an only-stale-episode roster show is not created")
}

// reactivateShowIfDormant flips dormant→active, leaves active untouched, and never
// resurrects a (manual-only) retired show.
func (s *RadioSyncSuite) TestReactivateShowIfDormant() {
	now := time.Now()
	st := s.seedBackfillStation()
	dormant := s.seedShowWithState(st.ID, "Dormant", "react-dormant", "react-d", catalogm.RadioLifecycleDormant)
	active := s.seedShowWithState(st.ID, "Active", "react-active", "react-a", catalogm.RadioLifecycleActive)
	retired := s.seedShowWithState(st.ID, "Retired", "react-retired", "react-r", catalogm.RadioLifecycleRetired)

	s.svc.reactivateShowIfDormant(dormant.ID, now)
	s.svc.reactivateShowIfDormant(active.ID, now)
	s.svc.reactivateShowIfDormant(retired.ID, now)

	s.Equal(catalogm.RadioLifecycleActive, s.reloadShow(dormant.ID).LifecycleState, "dormant → active")
	s.Equal(catalogm.RadioLifecycleActive, s.reloadShow(active.ID).LifecycleState, "active unchanged")
	s.Equal(catalogm.RadioLifecycleRetired, s.reloadShow(retired.ID).LifecycleState, "retired never auto-reactivated")
}

// End-to-end: importing a new episode for an existing DORMANT show (a DJ returning
// from a leave of absence) reactivates it to active in real time.
func (s *RadioSyncSuite) TestImportEpisode_ReactivatesDormantShow() {
	now := time.Now()
	today := now.Format("2006-01-02")
	st := s.seedBackfillStation()
	show := s.seedShowWithState(st.ID, "Returning DJ", "returning-dj", "ret-dj", catalogm.RadioLifecycleDormant)

	s.svc.playlistProviderFactory = func(string) (RadioPlaylistProvider, error) {
		return &mockPlaylistProvider{
			fetchNewEpisodesFn: func(string, time.Time, time.Time) ([]RadioEpisodeImport, error) {
				return []RadioEpisodeImport{{ExternalID: "ret-ep1", ShowExternalID: "ret-dj", AirDate: today}}, nil
			},
			fetchPlaylistFn: func(string) ([]RadioPlayImport, error) {
				return []RadioPlayImport{{Position: 1, ArtistName: "Returner"}}, nil
			},
		}, nil
	}
	defer func() { s.svc.playlistProviderFactory = nil }()

	since, until := now.AddDate(0, 0, -7), now
	_, err := s.svc.RunStationSync(context.Background(), st.ID, RunStationSyncOpts{
		Mode: catalogm.RadioSyncRunTypeBackfill, Trigger: catalogm.RadioSyncRunTriggerAutoBackfill,
		ShowID: &show.ID, WindowStart: &since, WindowEnd: &until,
	})
	s.Require().NoError(err)

	s.Equal(catalogm.RadioLifecycleActive, s.reloadShow(show.ID).LifecycleState,
		"a new episode reactivates the dormant show")
}
