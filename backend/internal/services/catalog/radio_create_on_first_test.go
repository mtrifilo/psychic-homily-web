package catalog

import (
	"context"
	"log/slog"
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// PSY-1153 create-on-first-episode + dormant reactivation. Runs against the same
// testcontainers Postgres as RadioSyncSuite (methods span files).

func (s *RadioSyncSuite) countShowsByExternalID(ext string) int64 {
	var n int64
	s.Require().NoError(s.db.Model(&catalogm.RadioShow{}).Where("external_id = ?", ext).Count(&n).Error)
	return n
}

// CreateShowIfHasEpisodes persists a row ONLY when the roster show has an episode in
// the window; an episode-less roster show stays invisible (no row).
func (s *RadioSyncSuite) TestCreateShowIfHasEpisodes_CreatesOnlyWithEpisodes() {
	now := time.Now()
	today := now.Format("2006-01-02")
	since, until := now.AddDate(0, 0, -90), now
	st := s.seedBackfillStation()

	s.svc.playlistProviderFactory = func(string) (RadioPlaylistProvider, error) {
		return &mockPlaylistProvider{
			fetchNewEpisodesFn: func(showExtID string, _, _ time.Time) ([]RadioEpisodeImport, error) {
				if showExtID == "has-eps" {
					return []RadioEpisodeImport{{ExternalID: "e1", ShowExternalID: "has-eps", AirDate: today}}, nil
				}
				return nil, nil // "no-eps" → empty roster show
			},
		}, nil
	}
	defer func() { s.svc.playlistProviderFactory = nil }()

	// Roster show WITH an episode in the window → row created, active.
	idA, createdA, err := s.svc.CreateShowIfHasEpisodes(st.ID,
		contracts.RadioRosterShow{ExternalID: "has-eps", Name: "Has Eps"}, since, until)
	s.Require().NoError(err)
	s.NotZero(idA)
	s.True(createdA)
	s.Equal(catalogm.RadioLifecycleActive, s.reloadShow(idA).LifecycleState)

	// Roster show with NO episode in the window → no row.
	idB, createdB, err := s.svc.CreateShowIfHasEpisodes(st.ID,
		contracts.RadioRosterShow{ExternalID: "no-eps", Name: "No Eps"}, since, until)
	s.Require().NoError(err)
	s.Zero(idB)
	s.False(createdB)
	s.Equal(int64(0), s.countShowsByExternalID("no-eps"), "episode-less roster show must not be persisted")
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

// End-to-end create-on-first: discovery persists nothing; the auto-backfill creates a
// row + imports episodes ONLY for the roster show that actually aired, leaving the
// episode-less roster show with no row.
func (s *RadioSyncSuite) TestAutoBackfill_CreateOnFirstEpisode() {
	now := time.Now()
	today := now.Format("2006-01-02")
	st := s.seedBackfillStation()

	s.svc.playlistProviderFactory = func(string) (RadioPlaylistProvider, error) {
		return &mockPlaylistProvider{
			discoverShowsFn: func() ([]RadioShowImport, error) {
				return []RadioShowImport{
					{ExternalID: "show-aired", Name: "Aired Show"},
					{ExternalID: "show-empty", Name: "Empty Show"},
				}, nil
			},
			fetchNewEpisodesFn: func(showExtID string, _, _ time.Time) ([]RadioEpisodeImport, error) {
				if showExtID == "show-aired" {
					return []RadioEpisodeImport{{ExternalID: "aired-ep1", ShowExternalID: "show-aired", AirDate: today}}, nil
				}
				return nil, nil
			},
			fetchPlaylistFn: func(string) ([]RadioPlayImport, error) {
				return []RadioPlayImport{{Position: 1, ArtistName: "A"}}, nil
			},
		}, nil
	}
	defer func() { s.svc.playlistProviderFactory = nil }()

	// Discover: no rows persisted, both shows returned as roster candidates.
	disc, err := s.svc.RunStationSync(context.Background(), st.ID, RunStationSyncOpts{
		Mode: catalogm.RadioSyncRunTypeDiscover, Trigger: catalogm.RadioSyncRunTriggerScheduled,
	})
	s.Require().NoError(err)
	s.Require().NotNil(disc.Discover)
	s.Len(disc.Discover.NewRosterShows, 2)
	s.Equal(int64(0), s.countShowsByExternalID("show-aired"))
	s.Equal(int64(0), s.countShowsByExternalID("show-empty"))

	// Auto-backfill: create-on-first-episode.
	fetchSvc := &RadioFetchService{
		radioService:     s.svc,
		stopCh:           make(chan struct{}),
		logger:           slog.Default(),
		autoBackfillDays: 90,
	}
	fetchSvc.wg.Add(1) // autoBackfillStation defers wg.Done()
	fetchSvc.autoBackfillStation(st.ID, st.Name, disc.Discover.NewRosterShows)

	// The aired show now has a row (active) with its episode imported.
	s.Equal(int64(1), s.countShowsByExternalID("show-aired"), "aired roster show is created on first episode")
	var aired catalogm.RadioShow
	s.Require().NoError(s.db.Where("external_id = ?", "show-aired").First(&aired).Error)
	s.Equal(catalogm.RadioLifecycleActive, aired.LifecycleState)
	var epCount int64
	s.Require().NoError(s.db.Model(&catalogm.RadioEpisode{}).Where("show_id = ?", aired.ID).Count(&epCount).Error)
	s.Positive(epCount, "the first episode is imported, not just the row created")

	// The episode-less roster show is never persisted.
	s.Equal(int64(0), s.countShowsByExternalID("show-empty"), "episode-less roster show stays invisible")
}
