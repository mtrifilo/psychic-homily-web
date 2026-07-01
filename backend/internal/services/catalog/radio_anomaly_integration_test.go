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

// seedFetchRunsAt inserts terminal fetch runs with the given play counts + status, dated
// from base (one minute apart). started_at controls baseline ordering/recency.
func (s *RadioSyncSuite) seedFetchRunsAt(stationID uint, plays []int, status string, base time.Time) {
	for i, p := range plays {
		started := base.Add(time.Duration(i) * time.Minute)
		finished := started.Add(time.Second)
		run := catalogm.RadioSyncRun{
			StationID:     stationID,
			RunType:       catalogm.RadioSyncRunTypeFetch,
			Trigger:       catalogm.RadioSyncRunTriggerScheduled,
			Status:        status,
			PlaysImported: p,
			StartedAt:     started,
			FinishedAt:    &finished,
		}
		s.Require().NoError(s.db.Create(&run).Error)
	}
}

// seedFetchRuns establishes a station's "normal" volume: success runs dated inside the
// trailing-baseline window, before the run under test.
func (s *RadioSyncSuite) seedFetchRuns(stationID uint, plays []int) {
	s.seedFetchRunsAt(stationID, plays, catalogm.RadioSyncRunStatusSuccess, time.Now().Add(-6*time.Hour))
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
	now := time.Now()
	today := now.Format("2006-01-02")
	// An AIRED window (started 2h ago, ended 1h ago) so the first-import playlist fetch runs
	// — a windowless episode dated today is now 'scheduled' until its broadcast day ends
	// (PSY-1287), which would skip the fetch; this test is about anomaly detection, not
	// windowless timing, so give it a real aired window (realistic for KEXP).
	airedStart, airedEnd := now.Add(-2*time.Hour), now.Add(-1*time.Hour)
	st := s.seedStation(catalogm.PlaylistSourceKEXP)
	s.seedFetchRuns(st.ID, []int{48, 50, 52, 45, 51, 49}) // trailing mean ~49

	ext := "vol-show"
	show := catalogm.RadioShow{StationID: st.ID, Name: "Vol Show", Slug: "vol-show", ExternalID: &ext, IsActive: true}
	s.Require().NoError(s.db.Create(&show).Error)

	s.svc.playlistProviderFactory = func(string) (RadioPlaylistProvider, error) {
		return &mockPlaylistProvider{
			fetchNewEpisodesFn: func(string, time.Time, time.Time) ([]RadioEpisodeImport, error) {
				return []RadioEpisodeImport{{ExternalID: "vol-ep1", ShowExternalID: ext, AirDate: today, StartsAt: &airedStart, EndsAt: &airedEnd}}, nil
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

// A SUSTAINED outage keeps getting flagged — the baseline is success-only, so the partial
// rows a long outage produces do NOT poison it. Regression guard for the self-poisoning a
// success+partial baseline would cause: here 20 newer 0-play PARTIAL runs sit on top of 5
// known-good successes; with the (buggy) success+partial query the 20 zeros would fill the
// 20-sample window and drag the mean below MinMean → the fresh 0-play fetch would wrongly
// pass. Success-only keeps the ~50 baseline intact, so it must still flag.
func (s *RadioSyncSuite) TestFetch_VolumeAnomaly_SuccessOnlyBaselineNotSelfPoisoned() {
	st := s.seedStation(catalogm.PlaylistSourceKEXP)
	s.seedFetchRunsAt(st.ID, []int{50, 50, 50, 50, 50}, catalogm.RadioSyncRunStatusSuccess, time.Now().Add(-6*time.Hour))
	s.seedFetchRunsAt(st.ID, make([]int, 20), catalogm.RadioSyncRunStatusPartial, time.Now().Add(-2*time.Hour))

	res, err := s.svc.RunStationSync(context.Background(), st.ID, RunStationSyncOpts{
		Mode: catalogm.RadioSyncRunTypeFetch, Trigger: catalogm.RadioSyncRunTriggerScheduled,
	})
	s.Require().NoError(err)
	s.Equal(catalogm.RadioSyncRunStatusPartial, s.statusOf(res.RunID),
		"success-only baseline keeps flagging through a sustained outage (not self-poisoned)")
	s.Equal(int64(1), s.countEmptyUnexpected(res.RunID))
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

// seedShowWithStaleFetch creates an active show on the station and stamps BOTH the
// station roll-up AND the show's own watermark ~30 days in the past, so a test can observe
// whether a fetch run advances either. Stamping the SHOW too keeps the helper honest under
// PSY-1272: `since` is now computed per show, so a station-only stale stamp would silently
// cold-start the show. Delegates to seedActiveShow so all show seeding stays in one place.
func (s *RadioSyncSuite) seedShowWithStaleFetch(stationID uint, ext string) {
	stale := time.Now().Add(-30 * 24 * time.Hour)
	s.seedActiveShow(stationID, ext, &stale)
	s.Require().NoError(s.db.Model(&catalogm.RadioStation{}).Where("id = ?", stationID).
		Update("last_playlist_fetch_at", stale).Error)
}

func (s *RadioSyncSuite) lastFetchAt(stationID uint) *time.Time {
	var st catalogm.RadioStation
	s.Require().NoError(s.db.First(&st, stationID).Error)
	return st.LastPlaylistFetchAt
}

// PSY-1241: a fetch run where every show's provider fetch errors (a total-station
// outage) must NOT advance last_playlist_fetch_at — it stays stale so fetchSince's
// catch-up branch re-scans the true gap once the provider recovers.
func (s *RadioSyncSuite) TestFetch_TotalOutage_HoldsLastFetchTimestamp() {
	st := s.seedStation(catalogm.PlaylistSourceNTS)
	s.seedShowWithStaleFetch(st.ID, "outage-show")

	s.svc.playlistProviderFactory = func(string) (RadioPlaylistProvider, error) {
		return &mockPlaylistProvider{
			fetchNewEpisodesFn: func(string, time.Time, time.Time) ([]RadioEpisodeImport, error) {
				return nil, fmt.Errorf("provider unreachable")
			},
		}, nil
	}
	defer func() { s.svc.playlistProviderFactory = nil }()

	_, err := s.svc.FetchNewEpisodes(st.ID)
	s.Require().NoError(err) // per-show fetch errors are recorded, not surfaced as a hard error

	got := s.lastFetchAt(st.ID)
	s.Require().NotNil(got)
	s.True(got.Before(time.Now().Add(-time.Hour)),
		"a total provider outage must leave last_playlist_fetch_at stale (~30d old), got %v", got)
}

// PSY-1241 companion: a healthy fetch (provider returns an episode that imports)
// DOES advance last_playlist_fetch_at to ~now.
func (s *RadioSyncSuite) TestFetch_Success_AdvancesLastFetchTimestamp() {
	today := time.Now().Format("2006-01-02")
	st := s.seedStation(catalogm.PlaylistSourceNTS)
	s.seedShowWithStaleFetch(st.ID, "ok-show")

	s.svc.playlistProviderFactory = func(string) (RadioPlaylistProvider, error) {
		return &mockPlaylistProvider{
			fetchNewEpisodesFn: func(_ string, _, _ time.Time) ([]RadioEpisodeImport, error) {
				return []RadioEpisodeImport{{ExternalID: "ok-show/ep1", ShowExternalID: "ok-show", AirDate: today}}, nil
			},
			fetchPlaylistFn: func(string) ([]RadioPlayImport, error) {
				return []RadioPlayImport{{Position: 1, ArtistName: "Artist"}}, nil
			},
		}, nil
	}
	defer func() { s.svc.playlistProviderFactory = nil }()

	_, err := s.svc.FetchNewEpisodes(st.ID)
	s.Require().NoError(err)

	got := s.lastFetchAt(st.ID)
	s.Require().NotNil(got)
	s.True(got.After(time.Now().Add(-time.Hour)),
		"a healthy fetch must advance last_playlist_fetch_at to ~now, got %v", got)
}

// PSY-1241 AC2 (now per-show, PSY-1272): after a multi-week outage held a show's
// watermark stale beyond the 45-day floor, a recovery fetch must re-scan back to the
// TRUE gap (the stale watermark), not clamp forward to the floor — otherwise outage-era
// episodes older than the floor are skipped forever. This proves the widen-past-floor
// half of recovery end to end (the hold half is TestFetch_TotalOutage_HoldsLastFetchTimestamp)
// by capturing the `since` fetchSince hands the provider for that show. Since PSY-1272 the
// per-SHOW watermark — not the station roll-up — drives this `since`.
func (s *RadioSyncSuite) TestFetch_RecoversTrueGapBeyondFloor() {
	st := s.seedStation(catalogm.PlaylistSourceNTS)
	ext := "recover-show"
	show := catalogm.RadioShow{StationID: st.ID, Name: ext, Slug: ext, ExternalID: &ext, IsActive: true}
	s.Require().NoError(s.db.Create(&show).Error)

	// A ~60-day-stale watermark is the state a >45-day outage leaves behind (held by
	// shouldAdvanceLastFetch across the outage), older than the floor. Stamped on the
	// SHOW (PSY-1272) — that is what fetchSince reads to compute this show's `since`.
	stale := time.Now().Add(-60 * 24 * time.Hour)
	s.Require().NoError(s.db.Model(&catalogm.RadioShow{}).Where("id = ?", show.ID).
		Update("last_playlist_fetch_at", stale).Error)

	var capturedSince time.Time
	s.svc.playlistProviderFactory = func(string) (RadioPlaylistProvider, error) {
		return &mockPlaylistProvider{
			fetchNewEpisodesFn: func(_ string, since, _ time.Time) ([]RadioEpisodeImport, error) {
				capturedSince = since
				return nil, nil
			},
		}, nil
	}
	defer func() { s.svc.playlistProviderFactory = nil }()

	_, err := s.svc.FetchNewEpisodes(st.ID)
	s.Require().NoError(err)

	floor := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -fetchLookbackFloorDays)
	s.True(capturedSince.Before(floor),
		"recovery `since` must widen past the floor; got %v floor %v", capturedSince, floor)
	s.True(capturedSince.Before(time.Now().Add(-50*24*time.Hour)),
		"recovery `since` must reach back to the ~60d gap, not stop at the 45d floor; got %v", capturedSince)
}

func (s *RadioSyncSuite) setLastFetch(stationID uint, t time.Time) {
	s.Require().NoError(s.db.Model(&catalogm.RadioStation{}).Where("id = ?", stationID).
		Update("last_playlist_fetch_at", t).Error)
}

// PSY-1269: EscalateStaleFetchOutages escalates active, automated stations whose
// last_playlist_fetch_at has been stale beyond the threshold (a sustained total-fetch
// outage) — and ONLY those: a fresh station, a manual-source station, and a
// never-fetched (NULL-watermark) station are all left alone.
func (s *RadioSyncSuite) TestEscalateStaleFetchOutages() {
	now := time.Now()

	stale := s.seedStation(catalogm.PlaylistSourceNTS)
	s.setLastFetch(stale.ID, now.Add(-24*time.Hour))

	fresh := s.seedStation(catalogm.PlaylistSourceKEXP)
	s.setLastFetch(fresh.ID, now.Add(-1*time.Hour))

	manual := s.seedStation(catalogm.PlaylistSourceManual)
	s.setLastFetch(manual.ID, now.Add(-24*time.Hour)) // stale, but manual → no automated fetch

	_ = s.seedStation(catalogm.PlaylistSourceWFMU) // never fetched (NULL watermark) → excluded

	// Inactive (deactivated) station that is also stale → excluded by is_active=TRUE.
	// Created directly (not seedStation) so is_active can be forced false past the
	// gorm default:true (a struct zero-value would be omitted and default to true).
	inactiveSrc := catalogm.PlaylistSourceNTS
	inactive := catalogm.RadioStation{
		Name: "Inactive Stale", Slug: "test-inactive-stale",
		BroadcastType: catalogm.BroadcastTypeInternet, PlaylistSource: &inactiveSrc,
	}
	s.Require().NoError(s.db.Create(&inactive).Error)
	s.Require().NoError(s.db.Model(&catalogm.RadioStation{}).Where("id = ?", inactive.ID).
		Update("is_active", false).Error)
	s.setLastFetch(inactive.ID, now.Add(-24*time.Hour))

	type escalation struct {
		stationID uint
		category  string
	}
	var got []escalation
	s.svc.onPermanentFailure = func(_ error, stationID uint, category string) {
		got = append(got, escalation{stationID, category})
	}
	defer func() { s.svc.onPermanentFailure = nil }()

	count, err := s.svc.EscalateStaleFetchOutages(18*time.Hour, now)
	s.Require().NoError(err)
	s.Equal(1, count, "only the stale, active, automated station is escalated")
	s.Require().Len(got, 1)
	s.Equal(stale.ID, got[0].stationID)
	s.Equal(radioFetchOutageCategory, got[0].category)
}
