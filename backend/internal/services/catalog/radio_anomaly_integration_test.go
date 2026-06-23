package catalog

import (
	"context"
	"fmt"
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// PSY-1156 volume-anomaly guard, end-to-end against the same testcontainers Postgres as
// RadioSyncSuite. These cover the wiring (baseline query → status downgrade → error row);
// the rule's boundaries are unit-tested in TestVolumeAnomaly.

// seedFetchRuns inserts terminal (success) fetch runs with the given play counts, dated
// recently so they fall inside the trailing-baseline window. Used to establish a station's
// "normal" volume before the run under test.
func (s *RadioSyncSuite) seedFetchRuns(stationID uint, plays []int) {
	base := time.Now().Add(-6 * time.Hour)
	for i, p := range plays {
		started := base.Add(time.Duration(i) * time.Minute)
		finished := started.Add(time.Second)
		run := catalogm.RadioSyncRun{
			StationID:     stationID,
			RunType:       catalogm.RadioSyncRunTypeFetch,
			Trigger:       catalogm.RadioSyncRunTriggerScheduled,
			Status:        catalogm.RadioSyncRunStatusSuccess,
			PlaysImported: p,
			StartedAt:     started,
			FinishedAt:    &finished,
		}
		s.Require().NoError(s.db.Create(&run).Error)
	}
}

func (s *RadioSyncSuite) countEmptyUnexpected(runID uint) int64 {
	var n int64
	s.Require().NoError(s.db.Model(&catalogm.RadioSyncRunError{}).
		Where("sync_run_id = ? AND category = ?", runID, catalogm.RadioSyncRunErrorEmptyUnexpected).
		Count(&n).Error)
	return n
}

func (s *RadioSyncSuite) statusOf(runID uint) string {
	var run catalogm.RadioSyncRun
	s.Require().NoError(s.db.First(&run, runID).Error)
	return run.Status
}

// The headline case (PSY-1126): a station whose recent fetches imported ~50 plays gets a
// fetch that imports 0 — it is flagged partial + empty_unexpected, not silent success.
func (s *RadioSyncSuite) TestFetch_VolumeAnomaly_FlagsBelowBaseline() {
	st := s.seedStation(catalogm.PlaylistSourceKEXP)
	s.seedFetchRuns(st.ID, []int{48, 50, 52, 45, 51, 49}) // trailing mean ~49

	// kexp station with no shows → a clean fetch importing 0 plays.
	res, err := s.svc.RunStationSync(context.Background(), st.ID, RunStationSyncOpts{
		Mode: catalogm.RadioSyncRunTypeFetch, Trigger: catalogm.RadioSyncRunTriggerScheduled,
	})
	s.Require().NoError(err)
	s.Require().NotNil(res)
	s.Require().NotNil(res.Import)
	s.Equal(0, res.Import.PlaysImported, "no shows → 0 plays imported")

	s.Equal(catalogm.RadioSyncRunStatusPartial, s.statusOf(res.RunID), "a 0-vs-~50 run is downgraded to partial")
	s.Equal(int64(1), s.countEmptyUnexpected(res.RunID), "exactly one empty_unexpected error row")
}

// A normal-volume fetch (≈ the trailing baseline) is NOT flagged — no false positive on
// the typical case. Exercises the real play-import path via a mock provider.
func (s *RadioSyncSuite) TestFetch_VolumeAnomaly_NormalVolumeNotFlagged() {
	today := time.Now().Format("2006-01-02")
	st := s.seedStation(catalogm.PlaylistSourceKEXP)
	s.seedFetchRuns(st.ID, []int{48, 50, 52, 45, 51, 49}) // trailing mean ~49

	ext := "vol-show"
	show := catalogm.RadioShow{StationID: st.ID, Name: "Vol Show", Slug: "vol-show", ExternalID: &ext, IsActive: true}
	s.Require().NoError(s.db.Create(&show).Error)

	s.svc.playlistProviderFactory = func(string) (RadioPlaylistProvider, error) {
		return &mockPlaylistProvider{
			fetchNewEpisodesFn: func(string, time.Time, time.Time) ([]RadioEpisodeImport, error) {
				return []RadioEpisodeImport{{ExternalID: "vol-ep1", ShowExternalID: ext, AirDate: today}}, nil
			},
			fetchPlaylistFn: func(string) ([]RadioPlayImport, error) {
				plays := make([]RadioPlayImport, 0, 40)
				for i := 0; i < 40; i++ {
					plays = append(plays, RadioPlayImport{Position: i + 1, ArtistName: fmt.Sprintf("Artist %d", i)})
				}
				return plays, nil
			},
		}, nil
	}
	defer func() { s.svc.playlistProviderFactory = nil }()

	res, err := s.svc.RunStationSync(context.Background(), st.ID, RunStationSyncOpts{
		Mode: catalogm.RadioSyncRunTypeFetch, Trigger: catalogm.RadioSyncRunTriggerScheduled,
	})
	s.Require().NoError(err)
	s.Require().NotNil(res.Import)
	s.GreaterOrEqual(res.Import.PlaysImported, 15, "a normal fetch imports well above the 30%-of-~49 threshold")
	s.Equal(int64(0), s.countEmptyUnexpected(res.RunID), "a normal-volume run is not flagged")
}

// With too few prior runs there is no trustworthy baseline, so even a 0-play fetch is not
// flagged (the guard waits to accumulate history).
func (s *RadioSyncSuite) TestFetch_VolumeAnomaly_NoBaselineNotFlagged() {
	st := s.seedStation(catalogm.PlaylistSourceKEXP)
	s.seedFetchRuns(st.ID, []int{50, 50, 50}) // only 3 < volumeAnomalyMinRuns

	res, err := s.svc.RunStationSync(context.Background(), st.ID, RunStationSyncOpts{
		Mode: catalogm.RadioSyncRunTypeFetch, Trigger: catalogm.RadioSyncRunTriggerScheduled,
	})
	s.Require().NoError(err)
	s.Equal(catalogm.RadioSyncRunStatusSuccess, s.statusOf(res.RunID), "no baseline → not flagged")
	s.Equal(int64(0), s.countEmptyUnexpected(res.RunID))
}
