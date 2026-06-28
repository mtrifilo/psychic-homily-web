package catalog

import (
	"fmt"
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// PSY-1272 per-show fetch recovery, end-to-end against the same testcontainers Postgres
// as RadioSyncSuite. These prove the per-SHOW last_playlist_fetch_at watermark drives
// `since` and advances independently per show, so a single persistently-failing show
// (e.g. a renamed/removed external_id) recovers its own gap without the rest of the
// station stalling — the gap PSY-1241's total-station gate deliberately left open.

// seedActiveShow creates an active show with an explicit per-show fetch watermark. A nil
// wm leaves last_playlist_fetch_at NULL (cold-start → the floor, like a never-fetched show).
func (s *RadioSyncSuite) seedActiveShow(stationID uint, ext string, wm *time.Time) catalogm.RadioShow {
	show := catalogm.RadioShow{
		StationID:           stationID,
		Name:                ext,
		Slug:                ext,
		ExternalID:          &ext,
		IsActive:            true,
		LastPlaylistFetchAt: wm,
	}
	s.Require().NoError(s.db.Create(&show).Error)
	return show
}

// showFetchAt reads a show's per-show watermark (the per-show analog of lastFetchAt).
func (s *RadioSyncSuite) showFetchAt(showID uint) *time.Time {
	var show catalogm.RadioShow
	s.Require().NoError(s.db.First(&show, showID).Error)
	return show.LastPlaylistFetchAt
}

// Two shows on one station with different watermarks must be fetched with DIFFERENT `since`
// bounds (PSY-1272): a recently-fetched show floors to the floor, while a show held stale
// past the floor widens back to its own gap. Before PSY-1272 both shared one station-level
// `since`, so the broken show's pre-floor gap was unrecoverable.
func (s *RadioSyncSuite) TestFetch_PerShow_SinceDivergesByShow() {
	st := s.seedStation(catalogm.PlaylistSourceNTS)
	recent := time.Now().Add(-time.Hour)
	stale := time.Now().Add(-60 * 24 * time.Hour)
	s.seedActiveShow(st.ID, "fresh-show", &recent)
	s.seedActiveShow(st.ID, "stale-show", &stale)

	// FetchNewEpisodes loops a station's shows sequentially, so no concurrent map writes.
	sinceByShow := map[string]time.Time{}
	s.svc.playlistProviderFactory = func(string) (RadioPlaylistProvider, error) {
		return &mockPlaylistProvider{
			fetchNewEpisodesFn: func(ext string, since, _ time.Time) ([]RadioEpisodeImport, error) {
				sinceByShow[ext] = since
				return nil, nil
			},
		}, nil
	}
	defer func() { s.svc.playlistProviderFactory = nil }()

	_, err := s.svc.FetchNewEpisodes(st.ID)
	s.Require().NoError(err)

	floor := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -fetchLookbackFloorDays)
	s.Require().Contains(sinceByShow, "fresh-show")
	s.Require().Contains(sinceByShow, "stale-show")
	s.True(sinceByShow["fresh-show"].Equal(floor),
		"a recently-fetched show floors to the floor; got %v want %v", sinceByShow["fresh-show"], floor)
	s.True(sinceByShow["stale-show"].Before(floor),
		"a show held stale past the floor widens past it to its own gap; got %v floor %v", sinceByShow["stale-show"], floor)
	s.True(sinceByShow["stale-show"].Before(time.Now().Add(-50*24*time.Hour)),
		"the stale show's `since` must reach back to its ~60d gap, not stop at the floor; got %v", sinceByShow["stale-show"])
}

// The headline PSY-1272 case: on a station where one show 404s every run but a sibling
// succeeds, the failing show HOLDS its own watermark stale (so it recovers its gap later)
// while the healthy sibling AND the station roll-up both advance — the station is not
// stalled by the one broken show, yet the broken show is still recoverable.
func (s *RadioSyncSuite) TestFetch_PerShow_FailingShowHoldsWhileSiblingAdvances() {
	today := time.Now().Format("2006-01-02")
	st := s.seedStation(catalogm.PlaylistSourceNTS)
	// 60 days is genuinely beyond the 45-day floor, so the held value is one that would
	// later drive a wider recovery `since` (proven in TestFetch_PerShow_SinceDivergesByShow);
	// a within-floor stale value would behave identically to a cold-start on recovery.
	stale := time.Now().Add(-60 * 24 * time.Hour)
	okShow := s.seedActiveShow(st.ID, "ok-show", &stale)
	badShow := s.seedActiveShow(st.ID, "bad-show", &stale)

	s.svc.playlistProviderFactory = func(string) (RadioPlaylistProvider, error) {
		return &mockPlaylistProvider{
			fetchNewEpisodesFn: func(ext string, _, _ time.Time) ([]RadioEpisodeImport, error) {
				if ext == "bad-show" {
					return nil, fmt.Errorf("404 not found: renamed external_id")
				}
				return []RadioEpisodeImport{{ExternalID: "ok-show/ep1", ShowExternalID: "ok-show", AirDate: today}}, nil
			},
			fetchPlaylistFn: func(string) ([]RadioPlayImport, error) {
				return []RadioPlayImport{{Position: 1, ArtistName: "Artist"}}, nil
			},
		}, nil
	}
	defer func() { s.svc.playlistProviderFactory = nil }()

	_, err := s.svc.FetchNewEpisodes(st.ID)
	s.Require().NoError(err) // per-show fetch errors are recorded, not surfaced as a hard error

	hourAgo := time.Now().Add(-time.Hour)

	okWM := s.showFetchAt(okShow.ID)
	s.Require().NotNil(okWM)
	s.True(okWM.After(hourAgo), "the healthy show advances its own watermark to ~now; got %v", okWM)

	badWM := s.showFetchAt(badShow.ID)
	s.Require().NotNil(badWM)
	s.True(badWM.Before(hourAgo), "the failing show holds its own watermark stale (~60d); got %v", badWM)

	stationWM := s.lastFetchAt(st.ID)
	s.Require().NotNil(stationWM)
	s.True(stationWM.After(hourAgo),
		"the station roll-up advances when ≥1 show succeeds (the PSY-1272 gap: not stalled by one broken show); got %v", stationWM)
}

// The migration backfill (PSY-1272): existing shows are seeded from their station's
// watermark so the first post-deploy fetch doesn't re-scan the whole window, and a station
// whose watermark is held stale at deploy keeps its catch-up. A NULL station watermark
// leaves the show NULL (cold-start). This exercises the same correlated UPDATE the
// migration runs (db/migrations/20260628050544_add_radio_show_last_playlist_fetch.up.sql) —
// the migration itself runs against empty tables, so its data effect is covered here.
func (s *RadioSyncSuite) TestMigration_BackfillSeedsShowFromStation() {
	// Station with a watermark + two shows that have none yet.
	withWM := s.seedStation(catalogm.PlaylistSourceNTS)
	wm := time.Now().Add(-10 * 24 * time.Hour).UTC().Truncate(time.Second)
	s.setLastFetch(withWM.ID, wm)
	showA := s.seedActiveShow(withWM.ID, "seeded-a", nil)
	showB := s.seedActiveShow(withWM.ID, "seeded-b", nil)

	// Station with NO watermark + one show: must stay NULL (cold-start).
	noWM := s.seedStation(catalogm.PlaylistSourceKEXP)
	showC := s.seedActiveShow(noWM.ID, "coldstart-c", nil)

	// The migration's backfill statement, verbatim.
	s.Require().NoError(s.db.Exec(`
		UPDATE radio_shows rs
		   SET last_playlist_fetch_at = st.last_playlist_fetch_at
		  FROM radio_stations st
		 WHERE rs.station_id = st.id
		   AND st.last_playlist_fetch_at IS NOT NULL`).Error)

	for _, id := range []uint{showA.ID, showB.ID} {
		got := s.showFetchAt(id)
		s.Require().NotNil(got, "a show under a watermarked station must be seeded")
		s.WithinDuration(wm, got.UTC(), time.Second,
			"the show watermark must be seeded from its station; got %v want %v", got, wm)
	}
	s.Nil(s.showFetchAt(showC.ID), "a show under a NULL-watermark station must stay NULL (cold-start)")
}

// AC1 end-to-end: a show whose external_id is renamed/removed 404s for weeks (its watermark
// holds stale), then an admin corrects the external_id and the very next incremental run must
// re-scan back to the held gap and IMPORT the pre-floor episodes — no manual backfill. This
// proves the full recovery LOOP through importEpisode, not just that `since` widens.
func (s *RadioSyncSuite) TestFetch_PerShow_RecoversGapAfterCorrection() {
	st := s.seedStation(catalogm.PlaylistSourceNTS)
	// Broken long enough that its gap predates the 45-day floor.
	stale := time.Now().Add(-70 * 24 * time.Hour)
	show := s.seedActiveShow(st.ID, "recover-show-broken", &stale)

	// An episode that aired during the broken window: older than the floor, newer than the
	// held watermark — recoverable only because the held watermark widens `since` past the floor.
	gapAirDate := time.Now().AddDate(0, 0, -55).Format("2006-01-02")
	var recoverSince time.Time
	s.svc.playlistProviderFactory = func(string) (RadioPlaylistProvider, error) {
		return &mockPlaylistProvider{
			fetchNewEpisodesFn: func(ext string, since, _ time.Time) ([]RadioEpisodeImport, error) {
				if ext == "recover-show-broken" {
					return nil, fmt.Errorf("404 not found: renamed external_id")
				}
				recoverSince = since
				return []RadioEpisodeImport{{ExternalID: "recover/gap-ep", ShowExternalID: ext, AirDate: gapAirDate}}, nil
			},
			fetchPlaylistFn: func(string) ([]RadioPlayImport, error) {
				return []RadioPlayImport{{Position: 1, ArtistName: "Artist"}}, nil
			},
		}, nil
	}
	defer func() { s.svc.playlistProviderFactory = nil }()

	// Run 1: still broken → watermark held, nothing imported.
	_, err := s.svc.FetchNewEpisodes(st.ID)
	s.Require().NoError(err)
	heldWM := s.showFetchAt(show.ID)
	s.Require().NotNil(heldWM)
	s.True(heldWM.Before(time.Now().Add(-time.Hour)), "while broken the show holds its watermark stale; got %v", heldWM)

	// Admin corrects the external_id (the show ROW persists; only the id field changes).
	s.Require().NoError(s.db.Model(&catalogm.RadioShow{}).Where("id = ?", show.ID).
		UpdateColumn("external_id", "recover-show").Error)

	// Run 2: recovery.
	_, err = s.svc.FetchNewEpisodes(st.ID)
	s.Require().NoError(err)

	floor := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -fetchLookbackFloorDays)
	s.True(recoverSince.Before(floor),
		"recovery `since` must widen past the floor to the held gap; got %v floor %v", recoverSince, floor)

	var imported int64
	s.Require().NoError(s.db.Model(&catalogm.RadioEpisode{}).
		Where("show_id = ? AND external_id = ?", show.ID, "recover/gap-ep").Count(&imported).Error)
	s.Equal(int64(1), imported, "the pre-floor gap episode must be imported via the incremental path on recovery")

	recoveredWM := s.showFetchAt(show.ID)
	s.Require().NotNil(recoveredWM)
	s.True(recoveredWM.After(time.Now().Add(-time.Hour)),
		"after a successful recovery fetch the show advances its own watermark; got %v", recoveredWM)
}

// The per-show HOLD branch (episodesReturned>0 && episodesImported==0): a show fetches OK but
// every returned episode's ROW write fails, so its watermark must be held stale (not advanced).
// Forces the hard importEpisode error with an unparseable air_date that the date column rejects.
func (s *RadioSyncSuite) TestFetch_PerShow_AllEpisodeWritesFail_HoldsWatermark() {
	st := s.seedStation(catalogm.PlaylistSourceNTS)
	stale := time.Now().Add(-60 * 24 * time.Hour)
	show := s.seedActiveShow(st.ID, "writefail-show", &stale)

	s.svc.playlistProviderFactory = func(string) (RadioPlaylistProvider, error) {
		return &mockPlaylistProvider{
			fetchNewEpisodesFn: func(string, time.Time, time.Time) ([]RadioEpisodeImport, error) {
				// Unparseable air_date → importEpisode's INSERT into the date column fails, so
				// every returned episode hard-errors and none is imported (episodesReturned=1,
				// episodesImported=0 → the gate holds the watermark).
				return []RadioEpisodeImport{{ExternalID: "writefail/ep", AirDate: "not-a-date"}}, nil
			},
		}, nil
	}
	defer func() { s.svc.playlistProviderFactory = nil }()

	_, err := s.svc.FetchNewEpisodes(st.ID)
	s.Require().NoError(err) // per-episode import errors are recorded, not surfaced as a hard error

	// The bad-date episode must NOT have persisted (confirms the hard-error path was taken).
	var imported int64
	s.Require().NoError(s.db.Model(&catalogm.RadioEpisode{}).
		Where("show_id = ?", show.ID).Count(&imported).Error)
	s.Equal(int64(0), imported, "the bad-date episode must fail to persist")

	wm := s.showFetchAt(show.ID)
	s.Require().NotNil(wm)
	s.True(wm.Before(time.Now().Add(-time.Hour)),
		"a show whose every episode row-write fails must HOLD its watermark stale; got %v", wm)
}

// Contract lock (PSY-1272): advancing the operational watermark uses UpdateColumn, so it must
// NOT bump the show's content-modification updated_at (surfaced in the API). Guards against a
// future revert to GORM Update, which auto-touches updated_at on every 6h fetch cycle.
func (s *RadioSyncSuite) TestFetch_PerShow_AdvanceDoesNotBumpUpdatedAt() {
	today := time.Now().Format("2006-01-02")
	st := s.seedStation(catalogm.PlaylistSourceNTS)
	stale := time.Now().Add(-60 * 24 * time.Hour)
	show := s.seedActiveShow(st.ID, "noupdate-show", &stale)
	// Backdate updated_at so a spurious bump on the watermark advance would be detectable.
	oldUpdatedAt := time.Now().Add(-90 * 24 * time.Hour).UTC().Truncate(time.Second)
	s.Require().NoError(s.db.Model(&catalogm.RadioShow{}).Where("id = ?", show.ID).
		UpdateColumn("updated_at", oldUpdatedAt).Error)

	s.svc.playlistProviderFactory = func(string) (RadioPlaylistProvider, error) {
		return &mockPlaylistProvider{
			fetchNewEpisodesFn: func(string, time.Time, time.Time) ([]RadioEpisodeImport, error) {
				return []RadioEpisodeImport{{ExternalID: "noupdate/ep", AirDate: today}}, nil
			},
			fetchPlaylistFn: func(string) ([]RadioPlayImport, error) {
				return []RadioPlayImport{{Position: 1, ArtistName: "Artist"}}, nil
			},
		}, nil
	}
	defer func() { s.svc.playlistProviderFactory = nil }()

	_, err := s.svc.FetchNewEpisodes(st.ID)
	s.Require().NoError(err)

	var got catalogm.RadioShow
	s.Require().NoError(s.db.First(&got, show.ID).Error)
	s.Require().NotNil(got.LastPlaylistFetchAt)
	s.True(got.LastPlaylistFetchAt.After(time.Now().Add(-time.Hour)), "the watermark advances on a successful fetch")
	s.WithinDuration(oldUpdatedAt, got.UpdatedAt.UTC(), time.Second,
		"advancing the watermark must NOT bump updated_at; got %v want %v", got.UpdatedAt, oldUpdatedAt)
}
