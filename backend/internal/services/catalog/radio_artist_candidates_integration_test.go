package catalog

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// RadioArtistCandidatesIntegrationSuite runs the artist candidate ranking
// against a migrated Postgres with the seeded radio_play_exclusions rows. The
// clock is fixed at radioCandidateTestNow, so the window starts on 2026-02-02.
type RadioArtistCandidatesIntegrationSuite struct {
	suite.Suite
	testDB    *testutil.TestDatabase
	db        *gorm.DB
	reader    *RadioCandidatesReader
	stations  map[string]uint
	episodes  map[string]uint
	playCount int
}

var radioCandidateTestNow = time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)

const (
	radioCandidateTestWindowStart = "2026-02-02"
	radioCandidateTestBeforeStart = "2026-02-01"
	radioCandidateTestMBA         = "11111111-1111-4111-8111-111111111111"
	radioCandidateTestMBB         = "bbbbbbbb-2222-4222-8222-222222222222"
	radioCandidateTestMBCatalog   = "33333333-3333-4333-8333-333333333333"
)

func TestRadioArtistCandidatesIntegrationSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	suite.Run(t, new(RadioArtistCandidatesIntegrationSuite))
}

func (s *RadioArtistCandidatesIntegrationSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
	s.reader = NewRadioCandidatesReader(s.db)
	s.reader.now = func() time.Time { return radioCandidateTestNow }
}

func (s *RadioArtistCandidatesIntegrationSuite) TearDownSuite() {
	s.testDB.Cleanup()
}

func (s *RadioArtistCandidatesIntegrationSuite) SetupTest() {
	s.cleanup()
	s.stations = map[string]uint{}
	s.episodes = map[string]uint{}
	// t-wfmu and t-rocknsoul share a network; t-kexp stands alone.
	network := &catalogm.RadioNetwork{Slug: "t-wfmu-net", Name: "t-wfmu-net"}
	s.Require().NoError(s.db.Create(network).Error)
	for _, slug := range []string{"t-kexp", "t-wfmu", "t-rocknsoul"} {
		station := &catalogm.RadioStation{Name: slug + " name", Slug: slug, BroadcastType: catalogm.BroadcastTypeInternet}
		if slug != "t-kexp" {
			station.NetworkID = &network.ID
		}
		s.Require().NoError(s.db.Create(station).Error)
		s.stations[slug] = station.ID
	}
}

func (s *RadioArtistCandidatesIntegrationSuite) TearDownTest() {
	s.cleanup()
}

func (s *RadioArtistCandidatesIntegrationSuite) cleanup() {
	for _, stmt := range []string{
		"DELETE FROM background_service_runs WHERE name = '" + RadioTracksRebuildLoopName + "'",
		"DELETE FROM radio_track_keys",
		"DELETE FROM radio_tracks",
		"DELETE FROM radio_plays",
		"DELETE FROM radio_episodes",
		"DELETE FROM radio_shows",
		"DELETE FROM radio_stations WHERE slug LIKE 't-%'",
		"DELETE FROM radio_networks WHERE slug LIKE 't-%'",
		"DELETE FROM artist_aliases WHERE alias LIKE 'T %'",
		"DELETE FROM artists WHERE name LIKE 'T %'",
	} {
		s.Require().NoError(s.db.Exec(stmt).Error)
	}
}

// candidatePlay describes n identical plays. Show defaults to "main" and date
// to a day inside the window.
type candidatePlay struct {
	station, show, date string
	artist              string
	title, album, label string
	mb                  string
	artistID            *uint
	matchState          string
}

func (s *RadioArtistCandidatesIntegrationSuite) add(p candidatePlay, n int) {
	if p.show == "" {
		p.show = "main"
	}
	if p.date == "" {
		p.date = "2026-07-01"
	}
	key := p.station + "|" + p.show + "|" + p.date
	ep, ok := s.episodes[key]
	if !ok {
		slug := p.station + "-" + p.show
		var sh catalogm.RadioShow
		s.Require().NoError(s.db.Where("slug = ?", slug).Limit(1).Find(&sh).Error)
		if sh.ID == 0 {
			sh = catalogm.RadioShow{StationID: s.stations[p.station], Name: slug, Slug: slug}
			s.Require().NoError(s.db.Create(&sh).Error)
		}
		episode := &catalogm.RadioEpisode{ShowID: sh.ID, AirDate: p.date}
		s.Require().NoError(s.db.Create(episode).Error)
		ep = episode.ID
		s.episodes[key] = ep
	}
	opt := func(v string) *string {
		if v == "" {
			return nil
		}
		return &v
	}
	state := p.matchState
	if state == "" {
		state = catalogm.RadioPlayMatchStateNoMatch
		if p.artistID != nil {
			state = catalogm.RadioPlayMatchStateMatched
		}
	}
	for range n {
		s.playCount++
		play := catalogm.RadioPlay{
			EpisodeID:           ep,
			Position:            s.playCount,
			ArtistName:          p.artist,
			TrackTitle:          opt(p.title),
			AlbumTitle:          opt(p.album),
			LabelName:           opt(p.label),
			MusicBrainzArtistID: opt(p.mb),
			ArtistID:            p.artistID,
			MatchState:          state,
			ProviderPlayID:      stringPtr(fmt.Sprintf("t-%d", s.playCount)),
		}
		s.Require().NoError(s.db.Create(&play).Error)
	}
}

// addAcross adds n plays on each of t-kexp and t-wfmu.
func (s *RadioArtistCandidatesIntegrationSuite) addAcross(p candidatePlay, n int) {
	for _, st := range []string{"t-kexp", "t-wfmu"} {
		p.station = st
		s.add(p, n)
	}
}

func (s *RadioArtistCandidatesIntegrationSuite) createArtist(name string, mb *string) uint {
	slug := fmt.Sprintf("t-artist-%d", time.Now().UnixNano())
	a := &catalogm.Artist{Name: name, Slug: &slug, MusicBrainzArtistID: mb}
	s.Require().NoError(s.db.Create(a).Error)
	return a.ID
}

func (s *RadioArtistCandidatesIntegrationSuite) list(lane contracts.RadioCandidateLaneFilter, sort contracts.RadioArtistCandidateSort, limit int) *contracts.RadioArtistCandidateListing {
	listing, err := s.reader.ListArtistCandidates(context.Background(), contracts.RadioArtistCandidatesQuery{Lane: lane, Sort: sort, Limit: limit})
	s.Require().NoError(err)
	return listing
}

func (s *RadioArtistCandidatesIntegrationSuite) all() map[string]contracts.RadioArtistCandidate {
	listing := s.list(contracts.RadioCandidateLaneFilterAll, contracts.RadioArtistCandidateSortScore, 500)
	byKey := map[string]contracts.RadioArtistCandidate{}
	for _, c := range listing.Candidates {
		byKey[c.ArtistKey] = c
	}
	return byKey
}

func (s *RadioArtistCandidatesIntegrationSuite) TestWindowAndFloorsAreReported() {
	listing := s.list(contracts.RadioCandidateLaneFilterAll, contracts.RadioArtistCandidateSortScore, 10)
	s.Equal("2026-07-31", listing.AsOf)
	s.Equal(radioCandidateTestWindowStart, listing.WindowStart)
	s.Equal(180, listing.WindowDays)
	s.Equal(2, listing.MinStationRows)
	s.Equal(3, listing.MinPlays)
	s.NotNil(listing.Candidates)
	s.Empty(listing.Candidates)
	s.Zero(listing.Total)
	s.Nil(listing.TracksRebuiltAt)
}

func (s *RadioArtistCandidatesIntegrationSuite) TestSingleStationArtistIsExcluded() {
	s.add(candidatePlay{station: "t-kexp", artist: "T Lonely"}, 10)
	s.add(candidatePlay{station: "t-kexp", artist: "T Pair"}, 2)
	s.add(candidatePlay{station: "t-wfmu", artist: "T Pair"}, 1)

	got := s.all()
	s.NotContains(got, "t lonely", "ten plays on one station row never qualify")
	s.Contains(got, "t pair")
	s.Equal(2, got["t pair"].StationRowCount)
	s.Equal(3, got["t pair"].PlayCount)
}

func (s *RadioArtistCandidatesIntegrationSuite) TestTwoStreamsOfOneNetworkCountAsTwoRows() {
	s.add(candidatePlay{station: "t-wfmu", artist: "T Network"}, 2)
	s.add(candidatePlay{station: "t-rocknsoul", artist: "T Network"}, 2)

	got := s.all()
	s.Require().Contains(got, "t network")
	s.Equal(2, got["t network"].StationRowCount)
	s.Equal(1, got["t network"].StationFamilyCount)
}

func (s *RadioArtistCandidatesIntegrationSuite) TestExcludedNamesAreExcluded() {
	s.addAcross(candidatePlay{artist: "Unknown Artist"}, 5)
	s.addAcross(candidatePlay{artist: "Various Artists"}, 5)
	s.addAcross(candidatePlay{artist: "Music behind DJ: T Bed"}, 5)
	s.addAcross(candidatePlay{artist: "Wake Weather"}, 5)
	// A talk marker in the title excludes the whole play, artist included.
	s.addAcross(candidatePlay{artist: "T Talker", title: "Your DJ speaks"}, 5)
	// Names that normalize to empty are not one artist.
	s.addAcross(candidatePlay{artist: "東京"}, 5)
	s.addAcross(candidatePlay{artist: "   "}, 5)

	s.Empty(s.all())
}

func (s *RadioArtistCandidatesIntegrationSuite) TestWindowEdge() {
	// Two plays inside plus one on the window's first day: three counted.
	s.addAcross(candidatePlay{artist: "T Edge In"}, 1)
	s.add(candidatePlay{station: "t-kexp", artist: "T Edge In", date: radioCandidateTestWindowStart}, 1)
	// Two plays inside plus one the day before: two counted.
	s.addAcross(candidatePlay{artist: "T Edge Out"}, 1)
	s.add(candidatePlay{station: "t-kexp", artist: "T Edge Out", date: radioCandidateTestBeforeStart}, 1)
	// The second station only before the window: one station row.
	s.add(candidatePlay{station: "t-kexp", artist: "T Old Station"}, 3)
	s.add(candidatePlay{station: "t-wfmu", artist: "T Old Station", date: "2025-12-01"}, 3)

	got := s.all()
	s.Require().Contains(got, "t edge in")
	s.Equal(3, got["t edge in"].PlayCount)
	s.Equal(radioCandidateTestWindowStart, got["t edge in"].FirstAiredOn)
	s.NotContains(got, "t edge out")
	s.NotContains(got, "t old station")
}

func (s *RadioArtistCandidatesIntegrationSuite) TestLanes() {
	s.addAcross(candidatePlay{artist: "T One Id", mb: radioCandidateTestMBA}, 2)
	s.add(candidatePlay{station: "t-kexp", artist: "T One Id"}, 1)
	// An upper-case id is the same id.
	s.add(candidatePlay{station: "t-kexp", artist: "T Two Ids", mb: radioCandidateTestMBA}, 2)
	s.add(candidatePlay{station: "t-wfmu", artist: "T Two Ids", mb: radioCandidateTestMBB}, 2)
	s.add(candidatePlay{station: "t-wfmu", artist: "T Two Ids", mb: "BBBBBBBB-2222-4222-8222-222222222222"}, 1)
	// A value not shaped like a UUID is absent.
	s.addAcross(candidatePlay{artist: "T Bad Id", mb: "not-an-id"}, 2)
	s.addAcross(candidatePlay{artist: "T No Id"}, 2)

	got := s.all()
	one := got["t one id"]
	s.Equal(contracts.RadioCandidateLaneAuto, one.Lane)
	s.Require().NotNil(one.MBArtistID)
	s.Equal(radioCandidateTestMBA, *one.MBArtistID)
	s.Equal([]string{radioCandidateTestMBA}, one.MBArtistIDs)
	s.False(one.MBArtistIDConflict)

	two := got["t two ids"]
	s.Equal(contracts.RadioCandidateLaneHuman, two.Lane)
	s.Nil(two.MBArtistID)
	s.True(two.MBArtistIDConflict)
	s.Equal([]string{radioCandidateTestMBA, radioCandidateTestMBB}, two.MBArtistIDs)

	for _, key := range []string{"t bad id", "t no id"} {
		s.Equal(contracts.RadioCandidateLaneHuman, got[key].Lane, key)
		s.Empty(got[key].MBArtistIDs, key)
		s.NotNil(got[key].MBArtistIDs, key)
	}

	auto := s.list(contracts.RadioCandidateLaneFilterAuto, contracts.RadioArtistCandidateSortScore, 50)
	s.Equal(1, auto.Total)
	s.Equal(1, auto.AutoTotal)
	s.Equal(3, auto.HumanTotal)
	s.Require().Len(auto.Candidates, 1)
	s.Equal("t one id", auto.Candidates[0].ArtistKey)

	human := s.list(contracts.RadioCandidateLaneFilterHuman, contracts.RadioArtistCandidateSortScore, 2)
	s.Equal(3, human.Total)
	s.Len(human.Candidates, 2, "limit caps the rows, not the total")
	for _, c := range human.Candidates {
		s.Equal(contracts.RadioCandidateLaneHuman, c.Lane)
	}
}

func (s *RadioArtistCandidatesIntegrationSuite) TestCatalogArtistsAreNotCandidates() {
	linkedTo := s.createArtist("T Real Name", nil)
	// One linked play takes the whole key out.
	s.addAcross(candidatePlay{artist: "T Partly Linked"}, 2)
	s.add(candidatePlay{station: "t-kexp", artist: "T Partly Linked", artistID: &linkedTo}, 1)
	// Unlinked plays of names the catalog already answers to.
	s.createArtist("T Catalog Band", nil)
	s.addAcross(candidatePlay{artist: "t catalog band!"}, 2)
	aliased := s.createArtist("T Aliased Real", nil)
	s.Require().NoError(s.db.Create(&catalogm.ArtistAlias{ArtistID: aliased, Alias: "T Alias Band"}).Error)
	s.addAcross(candidatePlay{artist: "T Alias Band"}, 2)
	mb := radioCandidateTestMBCatalog
	s.createArtist("T Mb Real", &mb)
	s.addAcross(candidatePlay{artist: "T Mb Spelling", mb: radioCandidateTestMBCatalog}, 2)
	// Ambiguous plays are not counted: two plays remain.
	s.addAcross(candidatePlay{artist: "T Ambiguous"}, 1)
	s.add(candidatePlay{station: "t-kexp", artist: "T Ambiguous", matchState: catalogm.RadioPlayMatchStateAmbiguous}, 3)
	// The still-unmatched state counts like no_match.
	s.addAcross(candidatePlay{artist: "T Pending", matchState: catalogm.RadioPlayMatchStateUnmatched}, 2)

	got := s.all()
	for _, key := range []string{"t partly linked", "t catalog band", "t alias band", "t mb spelling", "t ambiguous"} {
		s.NotContains(got, key)
	}
	s.Contains(got, "t pending")
	s.Len(got, 1)
}

func (s *RadioArtistCandidatesIntegrationSuite) TestEvidence() {
	add := func(station, show, artist, title, album, label string, n int) {
		s.add(candidatePlay{station: station, show: show, artist: artist, title: title, album: album, label: label}, n)
	}
	add("t-kexp", "a", "T Evidence", "Song One", "Album One", "Label One", 4)
	add("t-kexp", "b", "T EVIDENCE", "Song One (Live)", "album one", "Self-Released", 1)
	add("t-wfmu", "a", "T Evidence", "Song Two", "Album Two", "Label Two", 2)
	add("t-rocknsoul", "a", "T Evidence", "Song Three", "Album Three", "Label Two", 1)
	add("t-rocknsoul", "a", "T Evidence", "Song Four", "Album Four", "", 1)
	add("t-rocknsoul", "a", "T Evidence", "Unknown", "Album Five", "", 1)
	add("t-rocknsoul", "a", "T Evidence", "", "Album Six", "", 1)
	report, err := RebuildRadioTracks(context.Background(), s.db)
	s.Require().NoError(err)
	s.Require().False(report.Skipped)

	got := s.all()
	s.Require().Contains(got, "t evidence")
	c := got["t evidence"]
	s.Equal("T Evidence", c.DisplayName)
	s.Equal(11, c.PlayCount, "placeholder and blank titles still count for the artist")
	s.Equal(3, c.StationRowCount)
	s.Equal(2, c.StationFamilyCount)
	s.Equal(4, c.ShowCount)
	s.InDelta(RadioArtistCandidateScore(3, 11, 4), c.Score, 1e-9)

	s.Equal([]contracts.RadioCandidateStation{
		{StationID: s.stations["t-kexp"], Slug: "t-kexp", Name: "t-kexp name", PlayCount: 5},
		{StationID: s.stations["t-rocknsoul"], Slug: "t-rocknsoul", Name: "t-rocknsoul name", PlayCount: 4},
		{StationID: s.stations["t-wfmu"], Slug: "t-wfmu", Name: "t-wfmu name", PlayCount: 2},
	}, c.Stations)

	// "Song One (Live)" shares Song One's track key; the placeholder title
	// "Unknown" and the blank title are not tracks.
	s.Require().Len(c.TopTracks, 3)
	s.Equal("Song One", c.TopTracks[0].Title)
	s.Equal(5, c.TopTracks[0].PlayCount)
	s.Equal("Song Two", c.TopTracks[1].Title)
	s.Equal(2, c.TopTracks[1].PlayCount)
	s.Equal(1, c.TopTracks[2].PlayCount)
	for _, tr := range c.TopTracks {
		s.NotZero(tr.TrackID)
		s.NotEqual("Unknown", tr.Title)
	}

	s.Equal(6, c.AlbumNameCount)
	s.Require().Len(c.AlbumNames, 5)
	s.Equal(contracts.RadioCandidateName{Name: "Album One", PlayCount: 5}, c.AlbumNames[0])
	s.Equal(contracts.RadioCandidateName{Name: "Album Two", PlayCount: 2}, c.AlbumNames[1])

	s.Equal(2, c.LabelNameCount, "the self-release placeholder is not a label")
	s.Equal([]contracts.RadioCandidateName{
		{Name: "Label One", PlayCount: 4},
		{Name: "Label Two", PlayCount: 3},
	}, c.LabelNames)

	listing := s.list(contracts.RadioCandidateLaneFilterAll, contracts.RadioArtistCandidateSortScore, 1)
	s.Nil(listing.TracksRebuiltAt, "the rebuild ran outside the loop, so the loop has no recorded success")
}

func (s *RadioArtistCandidatesIntegrationSuite) TestSortsAndRecentPlayTieBreak() {
	s.add(candidatePlay{station: "t-kexp", artist: "T Plays"}, 20)
	s.add(candidatePlay{station: "t-wfmu", artist: "T Plays"}, 1)
	for _, st := range []string{"t-kexp", "t-wfmu", "t-rocknsoul"} {
		s.add(candidatePlay{station: st, artist: "T Stations"}, 1)
	}
	// Identical counts; T Recent aired later.
	s.addAcross(candidatePlay{artist: "T Earlier", date: "2026-06-01"}, 2)
	s.addAcross(candidatePlay{artist: "T Recent", date: "2026-07-20"}, 2)

	keys := func(sort contracts.RadioArtistCandidateSort) []string {
		return candidateKeys(s.list(contracts.RadioCandidateLaneFilterAll, sort, 10).Candidates)
	}
	s.Equal("t plays", keys(contracts.RadioArtistCandidateSortPlays)[0])
	s.Equal("t stations", keys(contracts.RadioArtistCandidateSortStations)[0])
	byScore := keys(contracts.RadioArtistCandidateSortScore)
	s.Less(indexOf(byScore, "t recent"), indexOf(byScore, "t earlier"))
}

func indexOf(xs []string, x string) int {
	for i, v := range xs {
		if v == x {
			return i
		}
	}
	return -1
}
