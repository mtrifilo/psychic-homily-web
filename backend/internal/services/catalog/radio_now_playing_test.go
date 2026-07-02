package catalog

// Tests for the station now-playing service (PSY-1022): live/fallback
// routing, show + artist matching (incl. the PSY-1073 ambiguity rule), and
// the per-station TTL cache (no per-request provider fan-out).

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// UNIT TESTS (No Database Required)
// =============================================================================

func TestRadioService_NilDB_NowPlaying(t *testing.T) {
	svc := &RadioService{db: nil}
	_, err := svc.GetStationNowPlaying(1)
	require.Error(t, err)
	assert.Equal(t, "database not initialized", err.Error())
}

func TestLiveChannelForStation(t *testing.T) {
	tests := []struct {
		source      string
		slug        string
		wantChannel string
		wantOK      bool
	}{
		// KEXP is single-stream: any station slug routes.
		{catalogm.PlaylistSourceKEXP, "kexp", "", true},
		{catalogm.PlaylistSourceKEXP, "kexp-second", "", true},
		// NTS routes only mapped slugs (channel 1 = our nts-radio station).
		{catalogm.PlaylistSourceNTS, "nts-radio", "1", true},
		{catalogm.PlaylistSourceNTS, "nts-2", "", false},
		// WFMU: the four seeded streams; unmapped slugs fall back.
		{catalogm.PlaylistSourceWFMU, "wfmu", wfmuLiveChannelMain, true},
		{catalogm.PlaylistSourceWFMU, "wfmu-drummer", wfmuLiveChannelDrummer, true},
		{catalogm.PlaylistSourceWFMU, "wfmu-rocknsoulradio", wfmuLiveChannelRockSoul, true},
		{catalogm.PlaylistSourceWFMU, "wfmu-sheena", wfmuLiveChannelSheena, true},
		{catalogm.PlaylistSourceWFMU, "wfmu-new-channel", "", false},
		// Manual / unknown sources never route live.
		{catalogm.PlaylistSourceManual, "anything", "", false},
		{"bogus", "anything", "", false},
	}
	for _, tt := range tests {
		channel, ok := liveChannelForStation(tt.source, tt.slug)
		assert.Equal(t, tt.wantOK, ok, "%s/%s ok", tt.source, tt.slug)
		assert.Equal(t, tt.wantChannel, channel, "%s/%s channel", tt.source, tt.slug)
	}
}

func TestNowPlayingCache_TTLExpiry(t *testing.T) {
	now := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	cache := newNowPlayingCache(90 * time.Second)
	cache.now = func() time.Time { return now }

	fetches := 0
	fetch := func() (*contracts.RadioNowPlayingResponse, error) {
		fetches++
		return &contracts.RadioNowPlayingResponse{Source: contracts.NowPlayingSourceLive}, nil
	}

	// First call fetches; second within the TTL is served from cache.
	_, err := cache.getOrFetch(1, fetch)
	require.NoError(t, err)
	_, err = cache.getOrFetch(1, fetch)
	require.NoError(t, err)
	assert.Equal(t, 1, fetches, "second request within TTL must not re-fetch")

	// A different station has its own entry.
	_, err = cache.getOrFetch(2, fetch)
	require.NoError(t, err)
	assert.Equal(t, 2, fetches)

	// Past the TTL the entry refreshes.
	now = now.Add(91 * time.Second)
	_, err = cache.getOrFetch(1, fetch)
	require.NoError(t, err)
	assert.Equal(t, 3, fetches, "request past TTL must re-fetch")
}

func TestNowPlayingCache_ErrorsNotCached(t *testing.T) {
	cache := newNowPlayingCache(90 * time.Second)
	fetches := 0
	fetch := func() (*contracts.RadioNowPlayingResponse, error) {
		fetches++
		if fetches == 1 {
			return nil, errors.New("db down")
		}
		return &contracts.RadioNowPlayingResponse{}, nil
	}

	_, err := cache.getOrFetch(1, fetch)
	require.Error(t, err)
	resp, err := cache.getOrFetch(1, fetch)
	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, 2, fetches, "error must not be cached; next request retries")
}

func TestNowPlayingCache_FailedFetchDoesNotGrowMap(t *testing.T) {
	// Numeric station IDs reach the cache unvalidated, so probing
	// nonexistent IDs must not accumulate entries (memory DoS).
	cache := newNowPlayingCache(90 * time.Second)
	failing := func() (*contracts.RadioNowPlayingResponse, error) {
		return nil, errors.New("station not found")
	}
	for key := uint(1); key <= 100; key++ {
		_, err := cache.getOrFetch(key, failing)
		require.Error(t, err)
	}
	cache.mu.Lock()
	size := len(cache.entries)
	cache.mu.Unlock()
	assert.Equal(t, 0, size, "failed fetches must not leave entries behind")

	// A filled entry survives a later failed refresh (stale beats empty
	// for the map-size rule; the error itself still propagates).
	now := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	cache.now = func() time.Time { return now }
	_, err := cache.getOrFetch(1, func() (*contracts.RadioNowPlayingResponse, error) {
		return &contracts.RadioNowPlayingResponse{}, nil
	})
	require.NoError(t, err)
	now = now.Add(91 * time.Second)
	_, err = cache.getOrFetch(1, failing)
	require.Error(t, err)
	cache.mu.Lock()
	size = len(cache.entries)
	cache.mu.Unlock()
	assert.Equal(t, 1, size, "a previously-filled entry is retained")
}

func TestRecentArtistsFromPlayRows(t *testing.T) {
	row := func(name string, pos int) nowPlayingPlayRow {
		return nowPlayingPlayRow{RadioPlay: catalogm.RadioPlay{ArtistName: name, Position: pos}}
	}
	rows := []nowPlayingPlayRow{
		row("A", 1), row("B", 2), row("C", 3), row("B", 4),
		row("D", 5), row("E", 6), row("F", 7),
	}

	// Archive shape: skip the last row (the "current" track), dedup by name,
	// most-recent first, cap at 4.
	got := recentArtistsFromPlayRows(rows, true, "")
	names := make([]string, len(got))
	for i, p := range got {
		names[i] = p.ArtistName
	}
	assert.Equal(t, []string{"E", "D", "B", "C"}, names)

	// Live shape: nothing skipped positionally, but the live current artist
	// seeds the dedup set.
	got = recentArtistsFromPlayRows(rows, false, "F")
	names = names[:0]
	for _, p := range got {
		names = append(names, p.ArtistName)
	}
	assert.Equal(t, []string{"E", "D", "B", "C"}, names)

	// Empty input stays an empty (non-nil) slice.
	assert.Empty(t, recentArtistsFromPlayRows(nil, true, ""))
}

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

// fakeLiveProvider counts calls so cache tests can assert no per-request
// provider fan-out.
type fakeLiveProvider struct {
	live    *RadioLiveNowPlaying
	err     error
	calls   int
	channel string // last requested channel
}

func (f *fakeLiveProvider) FetchLiveNowPlaying(channel string) (*RadioLiveNowPlaying, error) {
	f.calls++
	f.channel = channel
	return f.live, f.err
}

type RadioNowPlayingIntegrationTestSuite struct {
	suite.Suite
	testDB       *testutil.TestDatabase
	db           *gorm.DB
	radioService *RadioService
}

func (suite *RadioNowPlayingIntegrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB
}

func (suite *RadioNowPlayingIntegrationTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

// SetupTest wipes data AND rebuilds the service so every test gets a cold
// now-playing cache.
func (suite *RadioNowPlayingIntegrationTestSuite) SetupTest() {
	suite.cleanupTables()
	suite.radioService = &RadioService{db: suite.db}
}

func (suite *RadioNowPlayingIntegrationTestSuite) TearDownTest() {
	suite.cleanupTables()
}

func (suite *RadioNowPlayingIntegrationTestSuite) cleanupTables() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM radio_plays")
	_, _ = sqlDB.Exec("DELETE FROM radio_episodes")
	_, _ = sqlDB.Exec("DELETE FROM radio_shows")
	_, _ = sqlDB.Exec("DELETE FROM radio_stations")
	_, _ = sqlDB.Exec("DELETE FROM radio_networks")
	_, _ = sqlDB.Exec("DELETE FROM artists")
}

func TestRadioNowPlayingIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(RadioNowPlayingIntegrationTestSuite))
}

// --- helpers ---------------------------------------------------------------

func (suite *RadioNowPlayingIntegrationTestSuite) createStation(name, slug, playlistSource string) *contracts.RadioStationDetailResponse {
	req := &contracts.CreateRadioStationRequest{
		Name:          name,
		Slug:          slug,
		BroadcastType: catalogm.BroadcastTypeBoth,
	}
	if playlistSource != "" {
		req.PlaylistSource = &playlistSource
	}
	resp, err := suite.radioService.CreateStation(req)
	suite.Require().NoError(err)
	return resp
}

func (suite *RadioNowPlayingIntegrationTestSuite) createShow(stationID uint, name, slug string, externalID *string, hostName *string) *catalogm.RadioShow {
	show := &catalogm.RadioShow{
		StationID:  stationID,
		Name:       name,
		Slug:       slug,
		ExternalID: externalID,
		HostName:   hostName,
		IsActive:   true,
	}
	suite.Require().NoError(suite.db.Create(show).Error)
	return show
}

func (suite *RadioNowPlayingIntegrationTestSuite) createEpisode(showID uint, airDate string) *catalogm.RadioEpisode {
	// Stamp a past air window so the episode passes the PSY-1285 air-window gate that
	// latestEpisodeForShow now shares with the feed. Every now-playing archive fixture
	// here is a past-aired episode, so a window a few days back is correct and
	// clock-independent (air_date still drives latest-selection ordering).
	// Truncated to Postgres timestamptz precision (µs): PSY-1306 compares these
	// fixture values with time.Equal after a DB round-trip, and Linux clocks carry
	// nanoseconds that Postgres drops (macOS clocks are µs, hiding this locally).
	now := time.Now().UTC().Truncate(time.Microsecond)
	starts := now.Add(-72 * time.Hour)
	ends := now.Add(-71 * time.Hour)
	ep := &catalogm.RadioEpisode{ShowID: showID, AirDate: airDate, StartsAt: &starts, EndsAt: &ends}
	suite.Require().NoError(suite.db.Create(ep).Error)
	return ep
}

func (suite *RadioNowPlayingIntegrationTestSuite) createPlay(episodeID uint, position int, artistName, trackTitle string, artistID *uint) {
	title := trackTitle
	play := &catalogm.RadioPlay{
		EpisodeID:  episodeID,
		Position:   position,
		ArtistName: artistName,
		TrackTitle: &title,
		ArtistID:   artistID,
	}
	suite.Require().NoError(suite.db.Create(play).Error)
}

func (suite *RadioNowPlayingIntegrationTestSuite) createArtist(name, slug string) *catalogm.Artist {
	artist := &catalogm.Artist{Name: name, Slug: &slug}
	suite.Require().NoError(suite.db.Create(artist).Error)
	return artist
}

func (suite *RadioNowPlayingIntegrationTestSuite) injectLiveProvider(fake *fakeLiveProvider) {
	suite.radioService.liveProviderFactory = func(string) (RadioLiveProvider, func(), bool) {
		return fake, func() {}, true
	}
}

// --- archive fallback --------------------------------------------------------

func (suite *RadioNowPlayingIntegrationTestSuite) TestArchiveFallback_FullPayload() {
	station := suite.createStation("Manual FM", "manual-fm", catalogm.PlaylistSourceManual)
	artist := suite.createArtist("Matched Artist", "matched-artist")

	// Relative dates: latestEpisodeForShow is now aired-only-bounded (PSY-1205),
	// so fixed past dates would couple this to the wall clock; keep them aired.
	now := time.Now().UTC()
	latestAired := now.AddDate(0, 0, -2).Format("2006-01-02")

	// quiet-show has 1 episode; active-show has 2 → the heuristic picks it.
	quiet := suite.createShow(station.ID, "Quiet Show", "quiet-show", nil, nil)
	suite.createEpisode(quiet.ID, now.AddDate(0, 0, -12).Format("2006-01-02"))
	host := "DJ Host"
	active := suite.createShow(station.ID, "Active Show", "active-show", nil, &host)
	suite.createEpisode(active.ID, now.AddDate(0, 0, -9).Format("2006-01-02"))
	latest := suite.createEpisode(active.ID, latestAired)

	suite.createPlay(latest.ID, 1, "Opener", "First Song", nil)
	suite.createPlay(latest.ID, 2, "Matched Artist", "Middle Song", &artist.ID)
	suite.createPlay(latest.ID, 3, "Closer", "Latest Song", nil)

	resp, err := suite.radioService.GetStationNowPlaying(station.ID)
	suite.Require().NoError(err)

	suite.Equal(contracts.NowPlayingSourceLatestArchive, resp.Source)
	suite.False(resp.OnAir)
	suite.Require().NotNil(resp.Show)
	suite.Equal("Active Show", resp.Show.Name)
	suite.Equal("active-show", resp.Show.Slug)
	suite.Require().NotNil(resp.Show.HostName)
	suite.Equal("DJ Host", *resp.Show.HostName)
	suite.Require().NotNil(resp.ShowName)
	suite.Equal("Active Show", *resp.ShowName)
	suite.Require().NotNil(resp.EpisodeAirDate)
	suite.Equal(latestAired, *resp.EpisodeAirDate)
	// PSY-1306: the fallback episode's frozen window rides along so the ON AIR
	// box can render the "Latest playlist" date viewer-local.
	suite.Require().NotNil(resp.EpisodeStartsAt)
	suite.True(resp.EpisodeStartsAt.Equal(*latest.StartsAt))
	suite.Require().NotNil(resp.EpisodeEndsAt)
	suite.True(resp.EpisodeEndsAt.Equal(*latest.EndsAt))

	// Current = the latest logged play (highest position).
	suite.Require().NotNil(resp.CurrentTrack)
	suite.Equal("Closer", resp.CurrentTrack.ArtistName)
	suite.Require().NotNil(resp.CurrentTrack.TrackTitle)
	suite.Equal("Latest Song", *resp.CurrentTrack.TrackTitle)

	// Recents exclude the current play, most recent first, with the stored
	// match's slug joined in.
	suite.Require().Len(resp.RecentArtists, 2)
	suite.Equal("Matched Artist", resp.RecentArtists[0].ArtistName)
	suite.Require().NotNil(resp.RecentArtists[0].ArtistSlug)
	suite.Equal("matched-artist", *resp.RecentArtists[0].ArtistSlug)
	suite.Equal("Opener", resp.RecentArtists[1].ArtistName)
	suite.Nil(resp.RecentArtists[1].ArtistID)
}

// PSY-1285: mostActiveShow ranks by VISIBLE-aired episodes, so a show padded with
// not-yet-aired / 0-track placeholder rows does not out-rank a show with real archived
// content (which the old ungated COUNT would, yielding an empty now-playing payload).
func (suite *RadioNowPlayingIntegrationTestSuite) TestArchiveFallback_PicksShowWithVisibleContent() {
	station := suite.createStation("Pad FM", "pad-fm", catalogm.PlaylistSourceManual)
	now := time.Now().UTC()
	ptr := func(t time.Time) *time.Time { return &t }

	// Padded show: MORE rows, but all not-yet-aired (future-windowed) → 0 visible.
	// Distinct external_ids so they don't collide on the (show_id, air_date, external_id)
	// unique index.
	padded := suite.createShow(station.ID, "Padded Show", "padded-show", nil, nil)
	for i := 0; i < 3; i++ {
		ext := fmt.Sprintf("pad-%d", i)
		ep := &catalogm.RadioEpisode{
			ShowID:     padded.ID,
			AirDate:    now.Format("2006-01-02"),
			ExternalID: &ext,
			StartsAt:   ptr(now.Add(time.Duration(i+1) * time.Hour)),
			EndsAt:     ptr(now.Add(time.Duration(i+2) * time.Hour)),
		}
		suite.Require().NoError(suite.db.Create(ep).Error)
	}
	// Real show: FEWER rows but a genuinely aired episode (createEpisode stamps a past window).
	realShow := suite.createShow(station.ID, "Real Show", "real-show", nil, nil)
	ep := suite.createEpisode(realShow.ID, now.AddDate(0, 0, -1).Format("2006-01-02"))
	suite.createPlay(ep.ID, 1, "Real Artist", "Real Song", nil)

	resp, err := suite.radioService.GetStationNowPlaying(station.ID)
	suite.Require().NoError(err)
	suite.Equal(contracts.NowPlayingSourceLatestArchive, resp.Source)
	suite.Require().NotNil(resp.Show)
	suite.Equal("Real Show", resp.Show.Name, "the show with VISIBLE archived content is picked, not the row-padded one")
	suite.Require().NotNil(resp.CurrentTrack)
	suite.Equal("Real Artist", resp.CurrentTrack.ArtistName)
}

func (suite *RadioNowPlayingIntegrationTestSuite) TestArchiveFallback_EmptyStation() {
	station := suite.createStation("Empty FM", "empty-fm", "")

	resp, err := suite.radioService.GetStationNowPlaying(station.ID)
	suite.Require().NoError(err)

	suite.Equal(contracts.NowPlayingSourceLatestArchive, resp.Source)
	suite.False(resp.OnAir)
	suite.Nil(resp.Show)
	suite.Nil(resp.ShowName)
	suite.Nil(resp.CurrentTrack)
	suite.Empty(resp.RecentArtists)
}

func (suite *RadioNowPlayingIntegrationTestSuite) TestNowPlaying_StationNotFound() {
	_, err := suite.radioService.GetStationNowPlaying(99999)
	suite.Require().Error(err)
}

// --- live path ---------------------------------------------------------------

func (suite *RadioNowPlayingIntegrationTestSuite) TestLive_MatchedShowAndArtist() {
	station := suite.createStation("KEXP", "kexp", catalogm.PlaylistSourceKEXP)
	ext := "16"
	suite.createShow(station.ID, "The Morning Show", "the-morning-show", &ext, nil)
	artist := suite.createArtist("Diana Ross", "diana-ross")

	title := "I'm Coming Out"
	host := "John Richards"
	fake := &fakeLiveProvider{live: &RadioLiveNowPlaying{
		ShowName:       "The Morning Show",
		ShowExternalID: &ext,
		HostName:       &host,
		CurrentTrack:   &RadioPlayImport{ArtistName: "Diana Ross", TrackTitle: &title},
		RecentTracks: []RadioPlayImport{
			{ArtistName: "Chic"},
			{ArtistName: "Diana Ross"}, // current artist → deduped out
			{ArtistName: "Sister Sledge"},
		},
	}}
	suite.injectLiveProvider(fake)

	resp, err := suite.radioService.GetStationNowPlaying(station.ID)
	suite.Require().NoError(err)

	suite.Equal(contracts.NowPlayingSourceLive, resp.Source)
	suite.True(resp.OnAir)
	suite.Require().NotNil(resp.Show)
	suite.Equal("the-morning-show", resp.Show.Slug)
	suite.Require().NotNil(resp.ShowName)
	suite.Equal("The Morning Show", *resp.ShowName)
	suite.Require().NotNil(resp.HostName)
	suite.Equal("John Richards", *resp.HostName)
	suite.Nil(resp.EpisodeAirDate, "live payloads carry no archive air date")
	suite.Nil(resp.EpisodeStartsAt, "live payloads carry no archive window (PSY-1306)")
	suite.Nil(resp.EpisodeEndsAt)

	suite.Require().NotNil(resp.CurrentTrack)
	suite.Equal("Diana Ross", resp.CurrentTrack.ArtistName)
	suite.Require().NotNil(resp.CurrentTrack.ArtistID, "live track artist should match by exact name")
	suite.Equal(artist.ID, *resp.CurrentTrack.ArtistID)
	suite.Require().NotNil(resp.CurrentTrack.ArtistSlug)
	suite.Equal("diana-ross", *resp.CurrentTrack.ArtistSlug)

	suite.Require().Len(resp.RecentArtists, 2)
	suite.Equal("Chic", resp.RecentArtists[0].ArtistName)
	suite.Equal("Sister Sledge", resp.RecentArtists[1].ArtistName)
}

func (suite *RadioNowPlayingIntegrationTestSuite) TestLive_AmbiguousShowNameYieldsNilShow() {
	// PSY-1073: WFMU's catalog duplication can produce same-name shows. Two
	// shows with the same name in the requested station → no link, raw name.
	station := suite.createStation("WFMU", "wfmu", catalogm.PlaylistSourceWFMU)
	suite.createShow(station.ID, "Duplicated Show", "duplicated-show", nil, nil)
	suite.createShow(station.ID, "Duplicated Show", "duplicated-show-2", nil, nil)

	fake := &fakeLiveProvider{live: &RadioLiveNowPlaying{ShowName: "Duplicated Show"}}
	suite.injectLiveProvider(fake)

	resp, err := suite.radioService.GetStationNowPlaying(station.ID)
	suite.Require().NoError(err)

	suite.Equal(contracts.NowPlayingSourceLive, resp.Source)
	suite.True(resp.OnAir)
	suite.Nil(resp.Show, "ambiguous name must not link")
	suite.Require().NotNil(resp.ShowName)
	suite.Equal("Duplicated Show", *resp.ShowName)
	suite.Equal(wfmuLiveChannelMain, fake.channel, "station slug routes to its own stream")
}

func (suite *RadioNowPlayingIntegrationTestSuite) TestLive_MissingShowNameMatchesNothing() {
	station := suite.createStation("KEXP", "kexp", catalogm.PlaylistSourceKEXP)
	fake := &fakeLiveProvider{live: &RadioLiveNowPlaying{ShowName: "Not In Our Catalog"}}
	suite.injectLiveProvider(fake)

	resp, err := suite.radioService.GetStationNowPlaying(station.ID)
	suite.Require().NoError(err)
	suite.Equal(contracts.NowPlayingSourceLive, resp.Source)
	suite.Nil(resp.Show)
	suite.Require().NotNil(resp.ShowName)
	suite.Equal("Not In Our Catalog", *resp.ShowName)
}

func (suite *RadioNowPlayingIntegrationTestSuite) TestLive_RecentArtistsBorrowedFromArchive() {
	// Show-level-only live source (NTS/WFMU shape): hops come from the
	// matched show's latest archived episode.
	station := suite.createStation("WFMU", "wfmu", catalogm.PlaylistSourceWFMU)
	ext := "P2"
	show := suite.createShow(station.ID, "Push Button Heaven", "push-button-heaven", &ext, nil)
	ep := suite.createEpisode(show.ID, "2026-06-04")
	suite.createPlay(ep.ID, 1, "Earlier Artist", "Song A", nil)
	suite.createPlay(ep.ID, 2, "Later Artist", "Song B", nil)

	title := "Circling the Church"
	fake := &fakeLiveProvider{live: &RadioLiveNowPlaying{
		ShowName:       "Push Button Heaven",
		ShowExternalID: &ext,
		CurrentTrack:   &RadioPlayImport{ArtistName: "david a jaycock", TrackTitle: &title},
	}}
	suite.injectLiveProvider(fake)

	resp, err := suite.radioService.GetStationNowPlaying(station.ID)
	suite.Require().NoError(err)

	suite.Equal(contracts.NowPlayingSourceLive, resp.Source)
	suite.Require().NotNil(resp.Show)
	suite.Equal("push-button-heaven", resp.Show.Slug)
	suite.Require().Len(resp.RecentArtists, 2)
	suite.Equal("Later Artist", resp.RecentArtists[0].ArtistName)
	suite.Equal("Earlier Artist", resp.RecentArtists[1].ArtistName)
}

// --- degradation -------------------------------------------------------------

func (suite *RadioNowPlayingIntegrationTestSuite) TestLive_ProviderErrorDegradesToArchive() {
	station := suite.createStation("KEXP", "kexp", catalogm.PlaylistSourceKEXP)
	show := suite.createShow(station.ID, "Archive Show", "archive-show", nil, nil)
	ep := suite.createEpisode(show.ID, "2026-06-01")
	suite.createPlay(ep.ID, 1, "Archive Artist", "Archive Song", nil)

	fake := &fakeLiveProvider{err: fmt.Errorf("provider timeout")}
	suite.injectLiveProvider(fake)

	resp, err := suite.radioService.GetStationNowPlaying(station.ID)
	suite.Require().NoError(err, "provider failure must never error the endpoint")

	suite.Equal(contracts.NowPlayingSourceLatestArchive, resp.Source)
	suite.False(resp.OnAir)
	suite.Require().NotNil(resp.Show)
	suite.Equal("archive-show", resp.Show.Slug)
	suite.Require().NotNil(resp.CurrentTrack)
	suite.Equal("Archive Artist", resp.CurrentTrack.ArtistName)
	suite.Equal(1, fake.calls)
}

func (suite *RadioNowPlayingIntegrationTestSuite) TestLive_NothingOnAirDegradesToArchive() {
	station := suite.createStation("KEXP", "kexp", catalogm.PlaylistSourceKEXP)
	fake := &fakeLiveProvider{live: nil} // provider answered: nothing live
	suite.injectLiveProvider(fake)

	resp, err := suite.radioService.GetStationNowPlaying(station.ID)
	suite.Require().NoError(err)
	suite.Equal(contracts.NowPlayingSourceLatestArchive, resp.Source)
}

func (suite *RadioNowPlayingIntegrationTestSuite) TestLive_UnmappedWFMUSlugSkipsProvider() {
	station := suite.createStation("WFMU New Channel", "wfmu-new-channel", catalogm.PlaylistSourceWFMU)
	fake := &fakeLiveProvider{live: &RadioLiveNowPlaying{ShowName: "Should Not Appear"}}
	suite.injectLiveProvider(fake)

	resp, err := suite.radioService.GetStationNowPlaying(station.ID)
	suite.Require().NoError(err)
	suite.Equal(contracts.NowPlayingSourceLatestArchive, resp.Source)
	suite.Equal(0, fake.calls, "unmapped stream must not call the provider")
}

// --- caching -----------------------------------------------------------------

func (suite *RadioNowPlayingIntegrationTestSuite) TestCache_NoPerRequestFanOut() {
	station := suite.createStation("KEXP", "kexp", catalogm.PlaylistSourceKEXP)
	fake := &fakeLiveProvider{live: &RadioLiveNowPlaying{ShowName: "The Morning Show"}}
	suite.injectLiveProvider(fake)

	// Deterministic clock so the test controls expiry.
	now := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	cache := newNowPlayingCache(nowPlayingCacheTTL)
	cache.now = func() time.Time { return now }
	suite.radioService.npCache = cache

	for i := 0; i < 5; i++ {
		resp, err := suite.radioService.GetStationNowPlaying(station.ID)
		suite.Require().NoError(err)
		suite.Equal(contracts.NowPlayingSourceLive, resp.Source)
	}
	suite.Equal(1, fake.calls, "page views within the TTL must not fan out to the provider")

	// Past the TTL: exactly one more provider call.
	now = now.Add(nowPlayingCacheTTL + time.Second)
	_, err := suite.radioService.GetStationNowPlaying(station.ID)
	suite.Require().NoError(err)
	suite.Equal(2, fake.calls)
}
