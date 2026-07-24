package catalog

// Airing-feed ingestion integration tests (PSY-1509), on the RadioSyncSuite
// Postgres. Providers are faked via playlistProviderFactory — no network.

import (
	"log/slog"
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// mockAiringProvider is a mockPlaylistProvider that also implements
// RadioAiringLister, mirroring the real KEXP/NTS providers' capability set.
type mockAiringProvider struct {
	mockPlaylistProvider
	airings      []RadioAiring
	airingsErr   error
	airingCalls  int
	lastChannel  string
	playlistFn   func(string) ([]RadioPlayImport, error) // optional; defaults to embedded mock
}

func (m *mockAiringProvider) FetchCurrentAirings(channel string) ([]RadioAiring, error) {
	m.airingCalls++
	m.lastChannel = channel
	return m.airings, m.airingsErr
}

func (m *mockAiringProvider) FetchPlaylist(episodeExternalID string) ([]RadioPlayImport, error) {
	if m.playlistFn != nil {
		return m.playlistFn(episodeExternalID)
	}
	return m.mockPlaylistProvider.FetchPlaylist(episodeExternalID)
}

// airingFor builds a windowed airing for a show external id.
func airingFor(showExt, epExt, showName string, starts, ends time.Time) RadioAiring {
	airTime := starts.Format("15:04:05")
	s, e := starts, ends
	return RadioAiring{
		ShowExternalID: showExt,
		ShowName:       showName,
		Episode: RadioEpisodeImport{
			ExternalID:     epExt,
			ShowExternalID: showExt,
			AirDate:        starts.Format("2006-01-02"),
			AirTime:        &airTime,
			StartsAt:       &s,
			EndsAt:         &e,
		},
	}
}

// TestIngestCurrentAirings_CreatesWindowedRowIdempotently: a matched airing
// creates the episode row with the feed's frozen window (status live, playlist
// pending); re-ingesting the same airing is a no-op — episode identity is
// (show, external_id).
func (s *RadioSyncSuite) TestIngestCurrentAirings_CreatesWindowedRowIdempotently() {
	now := time.Now()
	starts, ends := now.Add(-30*time.Minute), now.Add(90*time.Minute)

	st := s.seedStation(catalogm.PlaylistSourceKEXP)
	show := s.seedActiveShow(st.ID, "37", nil)

	mock := &mockAiringProvider{airings: []RadioAiring{airingFor("37", "67334", "Eastern Echoes", starts, ends)}}
	s.svc.playlistProviderFactory = func(string) (RadioPlaylistProvider, error) { return mock, nil }
	defer func() { s.svc.playlistProviderFactory = nil }()

	res := s.svc.IngestCurrentAirings(now)
	s.Equal(1, res.StationsPolled)
	s.Equal(1, res.RowsCreated)
	s.Equal(0, res.WindowsHealed)

	var ep catalogm.RadioEpisode
	s.Require().NoError(s.db.Where("show_id = ? AND external_id = ?", show.ID, "67334").First(&ep).Error)
	s.Require().NotNil(ep.StartsAt)
	s.True(ep.StartsAt.Equal(starts))
	s.Require().NotNil(ep.EndsAt)
	s.True(ep.EndsAt.Equal(ends))
	s.Equal(catalogm.RadioEpisodeStatusLive, ep.Status)
	s.Equal(catalogm.RadioPlaylistStatePending, ep.PlaylistState)

	// Second pass: idempotent — no new row, no heal (window already frozen).
	res = s.svc.IngestCurrentAirings(now)
	s.Equal(0, res.RowsCreated)
	s.Equal(0, res.WindowsHealed)
	var count int64
	s.Require().NoError(s.db.Model(&catalogm.RadioEpisode{}).Where("show_id = ?", show.ID).Count(&count).Error)
	s.EqualValues(1, count)
}

// TestIngestCurrentAirings_HealsEndlessRowAndReopensPlaylist: a sweep-created
// KEXP row (start_time from the listing, NO end — the listing publishes none)
// that was prematurely settled complete mid-air gets its end bound healed from
// the airing feed, flips live, and reopens to partial so the live refresh +
// final post-air fetch run at the right phase.
func (s *RadioSyncSuite) TestIngestCurrentAirings_HealsEndlessRowAndReopensPlaylist() {
	now := time.Now()
	starts, ends := now.Add(-30*time.Minute), now.Add(90*time.Minute)

	st := s.seedStation(catalogm.PlaylistSourceKEXP)
	show := s.seedActiveShow(st.ID, "37", nil)
	seeded := s.seedEpisodeFor(show.ID, "67334", starts.Format("2006-01-02"),
		catalogm.RadioPlaylistStateComplete, 0, &starts, nil, now)
	s.Require().NoError(s.db.Model(&catalogm.RadioEpisode{}).Where("id = ?", seeded.ID).
		Update("play_count", 7).Error)

	mock := &mockAiringProvider{airings: []RadioAiring{airingFor("37", "67334", "Eastern Echoes", starts, ends)}}
	s.svc.playlistProviderFactory = func(string) (RadioPlaylistProvider, error) { return mock, nil }
	defer func() { s.svc.playlistProviderFactory = nil }()

	res := s.svc.IngestCurrentAirings(now)
	s.Equal(0, res.RowsCreated)
	s.Equal(1, res.WindowsHealed)

	ep := s.reloadEpisode(seeded.ID)
	s.Require().NotNil(ep.StartsAt)
	s.True(ep.StartsAt.Equal(starts), "the frozen start is never rewritten")
	s.Require().NotNil(ep.EndsAt, "the missing end bound is healed from the airing feed")
	s.True(ep.EndsAt.Equal(ends))
	s.Equal(catalogm.RadioEpisodeStatusLive, ep.Status)
	s.Equal(catalogm.RadioPlaylistStatePartial, ep.PlaylistState,
		"a mid-air 'complete' verdict reopens so the live refresh can grow it")
}

// TestIngestCurrentAirings_UnmatchedAiringCreatesNothing: an airing matching no
// existing show creates neither an episode nor a show — airing feeds never mint
// radio_shows.
func (s *RadioSyncSuite) TestIngestCurrentAirings_UnmatchedAiringCreatesNothing() {
	now := time.Now()
	st := s.seedStation(catalogm.PlaylistSourceKEXP)
	s.seedActiveShow(st.ID, "37", nil)

	mock := &mockAiringProvider{airings: []RadioAiring{
		airingFor("999", "70001", "Unknown Special", now.Add(-10*time.Minute), now.Add(50*time.Minute)),
	}}
	s.svc.playlistProviderFactory = func(string) (RadioPlaylistProvider, error) { return mock, nil }
	defer func() { s.svc.playlistProviderFactory = nil }()

	res := s.svc.IngestCurrentAirings(now)
	s.Equal(1, res.StationsPolled)
	s.Equal(0, res.RowsCreated)

	var epCount, showCount int64
	s.Require().NoError(s.db.Model(&catalogm.RadioEpisode{}).Count(&epCount).Error)
	s.Require().NoError(s.db.Model(&catalogm.RadioShow{}).Where("station_id = ?", st.ID).Count(&showCount).Error)
	s.EqualValues(0, epCount)
	s.EqualValues(1, showCount, "airing ingestion must never create shows")
}

// TestIngestCurrentAirings_BlockedBreakerSkipsStation: an open, in-cooldown
// breaker keeps the airing poll off the station entirely.
func (s *RadioSyncSuite) TestIngestCurrentAirings_BlockedBreakerSkipsStation() {
	now := time.Now()
	st := s.seedStation(catalogm.PlaylistSourceKEXP)
	s.seedActiveShow(st.ID, "37", nil)
	tripped := now.Add(-time.Minute) // within the 30-min cooldown → gateBlocked
	s.Require().NoError(s.db.Create(&catalogm.RadioStationHealth{
		StationID:        st.ID,
		BreakerState:     catalogm.RadioBreakerStateOpen,
		BreakerTrippedAt: &tripped,
	}).Error)

	mock := &mockAiringProvider{airings: []RadioAiring{
		airingFor("37", "67334", "Eastern Echoes", now.Add(-10*time.Minute), now.Add(50*time.Minute)),
	}}
	s.svc.playlistProviderFactory = func(string) (RadioPlaylistProvider, error) { return mock, nil }
	defer func() { s.svc.playlistProviderFactory = nil }()

	res := s.svc.IngestCurrentAirings(now)
	s.Equal(0, res.StationsPolled)
	s.Equal(0, mock.airingCalls, "a blocked breaker must keep the airing poll off the provider")
}

// TestSlotFetchCycle_AiringIngestion_EndToEnd drives the real runSlotFetchCycle:
// a schedule-less KEXP-class show with NO episode rows gets its airing row
// created by the ingestion step AND scoped-fetched by the live-refresh work
// list in the SAME tick — the acceptance bound (row + growing playlist within
// one slot-fetch interval of broadcast start).
func (s *RadioSyncSuite) TestSlotFetchCycle_AiringIngestion_EndToEnd() {
	now := time.Now()
	starts, ends := now.Add(-30*time.Minute), now.Add(90*time.Minute)
	airDate := starts.Format("2006-01-02")
	showExt, epExt := "37", "67334"

	st := s.seedStation(catalogm.PlaylistSourceKEXP)
	show := s.seedActiveShow(st.ID, showExt, nil) // NO schedule, NO episodes

	track := "Live Track"
	var fetchPlaylistCalls int
	mock := &mockAiringProvider{
		airings: []RadioAiring{airingFor(showExt, epExt, "Eastern Echoes", starts, ends)},
		mockPlaylistProvider: mockPlaylistProvider{
			fetchNewEpisodesFn: func(string, time.Time, time.Time) ([]RadioEpisodeImport, error) {
				return []RadioEpisodeImport{{
					ExternalID: epExt, ShowExternalID: showExt, AirDate: airDate,
					StartsAt: &starts, EndsAt: &ends,
				}}, nil
			},
		},
		playlistFn: func(string) ([]RadioPlayImport, error) {
			fetchPlaylistCalls++
			return []RadioPlayImport{
				{Position: 1, ArtistName: "Live A", TrackTitle: &track},
				{Position: 2, ArtistName: "Live B", TrackTitle: &track},
			}, nil
		},
	}
	s.svc.playlistProviderFactory = func(string) (RadioPlaylistProvider, error) { return mock, nil }
	defer func() { s.svc.playlistProviderFactory = nil }()

	fetchSvc := &RadioFetchService{
		radioService:      s.svc,
		stopCh:            make(chan struct{}),
		logger:            slog.Default(),
		slotFetchInterval: 10 * time.Minute,
		lastSlotFetchAt:   time.Now(), // empty boundary window → only ingestion + live refresh drive
	}
	fetchSvc.runSlotFetchCycle()

	s.Positive(fetchPlaylistCalls, "the airing-created row must be scoped-fetched in the same tick")
	var ep catalogm.RadioEpisode
	s.Require().NoError(s.db.Where("show_id = ? AND external_id = ?", show.ID, epExt).First(&ep).Error)
	s.Require().NotNil(ep.StartsAt)
	s.Require().NotNil(ep.EndsAt)
	s.Equal(catalogm.RadioPlaylistStatePartial, ep.PlaylistState, "live playlist → partial after the tick")
	s.Equal(2, ep.PlayCount)
	s.Equal(0, ep.PlaylistFetchAttempts, "live refresh burns no post-air attempt")
}
