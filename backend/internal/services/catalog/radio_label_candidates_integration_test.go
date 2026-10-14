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

// RadioLabelCandidatesIntegrationSuite runs the label candidate ranking
// against a migrated Postgres with the seeded radio_play_exclusions rows. The
// clock is fixed at radioCandidateTestNow (2026-07-31), so the window starts on
// 2026-02-02, the recent velocity window on 2026-07-02 and the prior one on
// 2026-06-02.
type RadioLabelCandidatesIntegrationSuite struct {
	suite.Suite
	testDB    *testutil.TestDatabase
	db        *gorm.DB
	reader    *RadioCandidatesReader
	stations  map[string]uint
	episodes  map[string]uint
	playCount int
}

const (
	labelTestRecentStart = "2026-07-02"
	labelTestPriorEnd    = "2026-07-01"
	labelTestPriorStart  = "2026-06-02"
	labelTestBeforePrior = "2026-06-01"
	labelTestRecent      = "2026-07-20"
	labelTestPrior       = "2026-06-15"
	labelTestOlder       = "2026-04-01"
)

func TestRadioLabelCandidatesIntegrationSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	suite.Run(t, new(RadioLabelCandidatesIntegrationSuite))
}

func (s *RadioLabelCandidatesIntegrationSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
	s.reader = NewRadioCandidatesReader(s.db)
	s.reader.now = func() time.Time { return radioCandidateTestNow }
}

func (s *RadioLabelCandidatesIntegrationSuite) TearDownSuite() {
	s.testDB.Cleanup()
}

func (s *RadioLabelCandidatesIntegrationSuite) SetupTest() {
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

func (s *RadioLabelCandidatesIntegrationSuite) TearDownTest() {
	s.cleanup()
}

func (s *RadioLabelCandidatesIntegrationSuite) cleanup() {
	for _, stmt := range []string{
		"DELETE FROM radio_plays",
		"DELETE FROM radio_episodes",
		"DELETE FROM radio_shows",
		"DELETE FROM radio_stations WHERE slug LIKE 't-%'",
		"DELETE FROM radio_networks WHERE slug LIKE 't-%'",
		"DELETE FROM labels WHERE name LIKE 'T %'",
		"DELETE FROM artists WHERE name LIKE 'T %'",
	} {
		s.Require().NoError(s.db.Exec(stmt).Error)
	}
}

// labelPlay describes n identical plays. Show defaults to "main", date to a
// day in the recent velocity window and artist to "T Artist".
type labelPlay struct {
	station, show, date string
	label               string
	artist, album       string
	mbRelease           string
	artistID, labelID   *uint
	matchState          string
}

func (s *RadioLabelCandidatesIntegrationSuite) add(p labelPlay, n int) {
	if p.show == "" {
		p.show = "main"
	}
	if p.date == "" {
		p.date = labelTestRecent
	}
	if p.artist == "" {
		p.artist = "T Artist"
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
		if p.artistID != nil || p.labelID != nil {
			state = catalogm.RadioPlayMatchStateMatched
		}
	}
	for range n {
		s.playCount++
		play := catalogm.RadioPlay{
			EpisodeID:            ep,
			Position:             s.playCount,
			ArtistName:           p.artist,
			TrackTitle:           stringPtr(fmt.Sprintf("Song %d", s.playCount)),
			AlbumTitle:           opt(p.album),
			LabelName:            opt(p.label),
			MusicBrainzReleaseID: opt(p.mbRelease),
			ArtistID:             p.artistID,
			LabelID:              p.labelID,
			MatchState:           state,
			ProviderPlayID:       stringPtr(fmt.Sprintf("t-%d", s.playCount)),
		}
		s.Require().NoError(s.db.Create(&play).Error)
	}
}

// addAcross adds n plays on each of t-kexp and t-wfmu.
func (s *RadioLabelCandidatesIntegrationSuite) addAcross(p labelPlay, n int) {
	for _, st := range []string{"t-kexp", "t-wfmu"} {
		p.station = st
		s.add(p, n)
	}
}

func (s *RadioLabelCandidatesIntegrationSuite) createLabel(name string) uint {
	slug := fmt.Sprintf("t-label-%d", time.Now().UnixNano())
	l := &catalogm.Label{Name: name, Slug: &slug}
	s.Require().NoError(s.db.Create(l).Error)
	return l.ID
}

func (s *RadioLabelCandidatesIntegrationSuite) createArtist(name string) uint {
	slug := fmt.Sprintf("t-artist-%d", time.Now().UnixNano())
	a := &catalogm.Artist{Name: name, Slug: &slug}
	s.Require().NoError(s.db.Create(a).Error)
	return a.ID
}

func (s *RadioLabelCandidatesIntegrationSuite) list(lane contracts.RadioCandidateLaneFilter, sort contracts.RadioLabelCandidateSort, limit int) *contracts.RadioLabelCandidateListing {
	listing, err := s.reader.ListLabelCandidates(context.Background(), contracts.RadioLabelCandidatesQuery{Lane: lane, Sort: sort, Limit: limit})
	s.Require().NoError(err)
	return listing
}

func (s *RadioLabelCandidatesIntegrationSuite) all() map[string]contracts.RadioLabelCandidate {
	listing := s.list(contracts.RadioCandidateLaneFilterAll, contracts.RadioLabelCandidateSortScore, 500)
	byKey := map[string]contracts.RadioLabelCandidate{}
	for _, c := range listing.Candidates {
		byKey[c.LabelKey] = c
	}
	return byKey
}

func (s *RadioLabelCandidatesIntegrationSuite) TestWindowsAndFloorsAreReported() {
	listing := s.list(contracts.RadioCandidateLaneFilterAll, contracts.RadioLabelCandidateSortScore, 10)
	s.Equal("2026-07-31", listing.AsOf)
	s.Equal(radioCandidateTestWindowStart, listing.WindowStart)
	s.Equal(180, listing.WindowDays)
	s.Equal(labelTestRecentStart, listing.RecentStart)
	s.Equal(labelTestPriorStart, listing.PriorStart)
	s.Equal(30, listing.VelocityDays)
	s.Equal(2, listing.MinStationRows)
	s.Equal(1.5, listing.MinVelocity)
	s.Equal(3, listing.MinNewPlays)
	s.NotNil(listing.Candidates)
	s.Empty(listing.Candidates)
	s.Zero(listing.Total)
}

func (s *RadioLabelCandidatesIntegrationSuite) TestSingleStationLabelIsExcluded() {
	s.add(labelPlay{station: "t-kexp", label: "T House Label"}, 40)
	s.add(labelPlay{station: "t-kexp", label: "T Pair Label"}, 2)
	s.add(labelPlay{station: "t-wfmu", label: "T Pair Label"}, 1)
	// Two streams of one network are two station rows.
	s.add(labelPlay{station: "t-wfmu", label: "T Network Label"}, 2)
	s.add(labelPlay{station: "t-rocknsoul", label: "T Network Label"}, 2)

	got := s.all()
	s.NotContains(got, "t house label", "forty plays on one station row never qualify")
	s.Require().Contains(got, "t pair label")
	s.Equal(2, got["t pair label"].StationRowCount)
	s.Equal(3, got["t pair label"].PlayCount)
	s.Require().Contains(got, "t network label")
	s.Equal(2, got["t network label"].StationRowCount)
	s.Equal(1, got["t network label"].StationFamilyCount)
}

func (s *RadioLabelCandidatesIntegrationSuite) TestExcludedNamesAreExcluded() {
	for _, name := range []string{"Self-Released", "Self Released - T Band", "n/a", "Bandcamp", "[No Label]", "Not On Label"} {
		s.addAcross(labelPlay{label: name}, 3)
	}
	// A talk row excludes the whole play, label included.
	s.addAcross(labelPlay{label: "T Bed Label", artist: "Music behind DJ: T Bed"}, 3)
	// Blank labels and names that normalize to empty are not one label.
	s.addAcross(labelPlay{label: "   "}, 3)
	s.addAcross(labelPlay{}, 3)
	s.addAcross(labelPlay{label: "東京レコード"}, 2)

	listing := s.list(contracts.RadioCandidateLaneFilterAll, contracts.RadioLabelCandidateSortScore, 50)
	s.Empty(listing.Candidates)
	s.Equal(int64(4), listing.UnkeyablePlays, "the empty-key plays are counted when no key qualifies")

	// A placeholder artist excludes the play from the artist ranking only.
	s.addAcross(labelPlay{label: "T Compilation Label", artist: "Various Artists"}, 2)
	listing = s.list(contracts.RadioCandidateLaneFilterAll, contracts.RadioLabelCandidateSortScore, 50)
	s.Require().Len(listing.Candidates, 1)
	s.Equal("t compilation label", listing.Candidates[0].LabelKey)
	s.Equal(4, listing.Candidates[0].PlayCount)
	s.Empty(listing.Candidates[0].Artists, "a placeholder artist is not artist evidence")
	s.Zero(listing.Candidates[0].ArtistCount)
	s.Equal(int64(4), listing.UnkeyablePlays, "the empty-key plays are counted when a key qualifies")
}

func (s *RadioLabelCandidatesIntegrationSuite) TestFutureEpisodesAreNotCounted() {
	// One prior play per station and one recent play per station: flat.
	s.addAcross(labelPlay{label: "T Flat Label", date: labelTestPrior}, 1)
	s.addAcross(labelPlay{label: "T Flat Label"}, 1)
	// An episode dated after as_of carries plays before it airs.
	s.addAcross(labelPlay{label: "T Flat Label", date: "2026-08-07"}, 2)
	// A label seen only on future episodes has no counted plays.
	s.addAcross(labelPlay{label: "T Future Only", date: "2026-08-01"}, 3)

	// Plays on as_of itself count toward the window and the recent window.
	s.addAcross(labelPlay{label: "T Today Label", date: "2026-07-31"}, 2)

	got := s.all()
	s.NotContains(got, "t flat label", "future plays do not make a flat label rise")
	s.NotContains(got, "t future only")
	s.Require().Contains(got, "t today label")
	s.Equal(4, got["t today label"].PlayCount)
	s.Equal(4, got["t today label"].RecentPlayCount)
	s.Equal("2026-07-31", got["t today label"].LastAiredOn)
}

func (s *RadioLabelCandidatesIntegrationSuite) TestEvidenceCoversOnlyTheWindow() {
	s.addAcross(labelPlay{label: "T Windowed", artist: "T Inside", album: "Inside Album"}, 2)
	// Same label before the window and after as_of: never evidence.
	s.add(labelPlay{station: "t-rocknsoul", label: "T WINDOWED", artist: "T Before", album: "Before Album", date: radioCandidateTestBeforeStart}, 3)
	s.add(labelPlay{station: "t-rocknsoul", label: "T WINDOWED", artist: "T After", album: "After Album", date: "2026-08-02"}, 3)

	got := s.all()
	s.Require().Contains(got, "t windowed")
	c := got["t windowed"]
	s.Equal([]contracts.RadioCandidateName{{Name: "T Windowed", PlayCount: 4}}, c.LabelSpellings)
	s.Len(c.Stations, 2)
	s.Equal([]contracts.RadioLabelCandidateArtist{{Name: "T Inside", PlayCount: 4, ArtistIDs: []uint{}}}, c.Artists)
	s.Equal([]contracts.RadioCandidateName{{Name: "Inside Album", PlayCount: 4}}, c.ReleaseNames)
}

func (s *RadioLabelCandidatesIntegrationSuite) TestStationFloorCountsTheWholeWindow() {
	// Every velocity-window play is on t-kexp; the second station row aired
	// only before the prior window, still inside the 180-day window.
	s.add(labelPlay{station: "t-kexp", label: "T Older Second Row"}, 3)
	s.add(labelPlay{station: "t-wfmu", label: "T Older Second Row", date: labelTestOlder}, 1)

	got := s.all()
	s.Require().Contains(got, "t older second row")
	s.Equal(2, got["t older second row"].StationRowCount)
	s.Equal(3, got["t older second row"].RecentPlayCount)
	s.Equal(0, got["t older second row"].PriorPlayCount)
}

func (s *RadioLabelCandidatesIntegrationSuite) TestEvidenceListsAreCapped() {
	for i := range 7 {
		s.add(labelPlay{
			station:   []string{"t-kexp", "t-wfmu"}[i%2],
			label:     "T Big Label",
			artist:    fmt.Sprintf("T Capped Artist %d", i),
			album:     fmt.Sprintf("T Capped Album %d", i),
			mbRelease: fmt.Sprintf("%08x-1111-4111-8111-111111111111", i),
		}, 7-i)
	}

	got := s.all()
	s.Require().Contains(got, "t big label")
	c := got["t big label"]
	s.Len(c.Artists, 5)
	s.Equal(7, c.ArtistCount)
	s.Equal("T Capped Artist 0", c.Artists[0].Name)
	s.Len(c.ReleaseNames, 5)
	s.Equal(7, c.ReleaseNameCount)
	s.Len(c.MBReleaseIDs, 5)
	s.Equal(7, c.MBReleaseIDCount)
	s.Equal("00000000-1111-4111-8111-111111111111", c.MBReleaseIDs[0])
}

func (s *RadioLabelCandidatesIntegrationSuite) TestLinkedLabelsAreNotCandidates() {
	linked := s.createLabel("T Linked Elsewhere")
	// One linked play, before the window, takes every spelling of the key out.
	s.add(labelPlay{station: "t-kexp", label: "T Wás Linked!", date: "2025-06-01", labelID: &linked}, 1)
	s.addAcross(labelPlay{label: "t was linked."}, 3)
	// A linked play inside the window does the same.
	s.add(labelPlay{station: "t-kexp", label: "T Partly Linked", labelID: &linked}, 1)
	s.addAcross(labelPlay{label: "T Partly Linked"}, 3)
	s.addAcross(labelPlay{label: "T Unlinked"}, 3)

	got := s.all()
	s.NotContains(got, "t was linked")
	s.NotContains(got, "t partly linked")
	s.Contains(got, "t unlinked")
	s.Len(got, 1)
}

func (s *RadioLabelCandidatesIntegrationSuite) TestCatalogLabelNamesAreNotCandidates() {
	// Accent, case and edge punctuation fold on the catalog side too.
	s.createLabel("T Cátalog Label.")
	// Matched as the label matcher matches: radio_normalize_name equality.
	s.addAcross(labelPlay{label: "t catalog label!"}, 3)
	s.addAcross(labelPlay{label: "T CÁTALOG   LABEL"}, 3)
	// A longer name is a different key, so it stays a candidate.
	s.addAcross(labelPlay{label: "T Catalog Label Records"}, 3)

	got := s.all()
	s.NotContains(got, "t catalog label")
	s.Contains(got, "t catalog label records")
	s.Len(got, 1)
}

func (s *RadioLabelCandidatesIntegrationSuite) TestWindowEdge() {
	// The second station only on the window's first day: two rows.
	s.addAcross(labelPlay{label: "T Edge In"}, 2)
	s.add(labelPlay{station: "t-rocknsoul", label: "T Edge In", date: radioCandidateTestWindowStart}, 1)
	// The second station only on the day before: one row.
	s.add(labelPlay{station: "t-kexp", label: "T Edge Out"}, 4)
	s.add(labelPlay{station: "t-wfmu", label: "T Edge Out", date: radioCandidateTestBeforeStart}, 1)

	got := s.all()
	s.Require().Contains(got, "t edge in")
	s.Equal(5, got["t edge in"].PlayCount)
	s.Equal(3, got["t edge in"].StationRowCount)
	s.Equal(radioCandidateTestWindowStart, got["t edge in"].FirstAiredOn)
	s.NotContains(got, "t edge out")
}

func (s *RadioLabelCandidatesIntegrationSuite) TestVelocityWindowEdges() {
	// Recent window starts on 2026-07-02; the prior window is 2026-06-02 to
	// 2026-07-01. Each label has two station rows.
	twoRows := func(label, date string, kexp, wfmu int) {
		if kexp > 0 {
			s.add(labelPlay{station: "t-kexp", label: label, date: date}, kexp)
		}
		if wfmu > 0 {
			s.add(labelPlay{station: "t-wfmu", label: label, date: date}, wfmu)
		}
	}
	// 3 recent (on the recent window's first day) over 2 prior (on its last
	// day): exactly 1.5x.
	twoRows("T Exact Ratio", labelTestRecentStart, 2, 1)
	twoRows("T Exact Ratio", labelTestPriorEnd, 1, 1)
	// Moving one recent play to the day before makes it 2 over 3.
	twoRows("T Just Below", labelTestRecentStart, 1, 1)
	twoRows("T Just Below", labelTestPriorEnd, 2, 1)
	// The prior window's first day counts as prior: 3 over 2 is rising, while
	// the play the day before counts toward neither window.
	twoRows("T Prior Edge", labelTestRecent, 2, 1)
	twoRows("T Prior Edge", labelTestPriorStart, 1, 1)
	twoRows("T Prior Edge", labelTestBeforePrior, 3, 3)
	twoRows("T Prior Edge Plus", labelTestRecent, 2, 1)
	twoRows("T Prior Edge Plus", labelTestPriorStart, 2, 1)
	// No prior plays: three recent plays are rising, two are not.
	twoRows("T New Three", labelTestRecent, 2, 1)
	twoRows("T New Two", labelTestRecent, 1, 1)
	// Old plays only: neither window has any.
	twoRows("T Old Only", labelTestOlder, 5, 5)

	got := s.all()
	s.Require().Contains(got, "t exact ratio")
	exact := got["t exact ratio"]
	s.Equal(3, exact.RecentPlayCount)
	s.Equal(2, exact.PriorPlayCount)
	s.InDelta(1.5, exact.Velocity, 1e-9)
	s.InDelta(RadioLabelCandidateScore(2, 5, 3, 2), exact.Score, 1e-9)
	s.NotContains(got, "t just below")

	s.Require().Contains(got, "t prior edge")
	edge := got["t prior edge"]
	s.Equal(3, edge.RecentPlayCount)
	s.Equal(2, edge.PriorPlayCount)
	s.Equal(11, edge.PlayCount, "plays before the prior window still count toward the window total")
	s.NotContains(got, "t prior edge plus", "3 recent over 3 prior is flat")

	s.Require().Contains(got, "t new three")
	s.Equal(0, got["t new three"].PriorPlayCount)
	s.InDelta(3.0, got["t new three"].Velocity, 1e-9)
	s.NotContains(got, "t new two")
	s.NotContains(got, "t old only")
	s.Len(got, 3)
}

func (s *RadioLabelCandidatesIntegrationSuite) TestEveryCandidateIsInTheHumanLane() {
	s.addAcross(labelPlay{label: "T Lane One", mbRelease: radioCandidateTestMBA}, 2)
	s.addAcross(labelPlay{label: "T Lane Two"}, 2)
	s.addAcross(labelPlay{label: "T Lane Three"}, 2)

	all := s.list(contracts.RadioCandidateLaneFilterAll, contracts.RadioLabelCandidateSortScore, 50)
	s.Equal(3, all.Total)
	s.Equal(0, all.AutoTotal)
	s.Equal(3, all.HumanTotal)
	for _, c := range all.Candidates {
		s.Equal(contracts.RadioCandidateLaneHuman, c.Lane, c.LabelKey)
	}

	auto := s.list(contracts.RadioCandidateLaneFilterAuto, contracts.RadioLabelCandidateSortScore, 50)
	s.Zero(auto.Total)
	s.Empty(auto.Candidates)
	s.Equal(3, auto.HumanTotal)

	human := s.list(contracts.RadioCandidateLaneFilterHuman, contracts.RadioLabelCandidateSortScore, 2)
	s.Equal(3, human.Total)
	s.Len(human.Candidates, 2, "limit caps the rows, not the total")
}

func (s *RadioLabelCandidatesIntegrationSuite) TestEvidence() {
	known := s.createArtist("T Known Artist")
	add := func(p labelPlay, n int) { s.add(p, n) }
	add(labelPlay{station: "t-kexp", show: "a", label: "T Evidence Label", artist: "T Band One", album: "Album One", mbRelease: radioCandidateTestMBA}, 4)
	add(labelPlay{station: "t-kexp", show: "b", label: "T EVIDENCE LABEL ", artist: "t band one", album: "album one", mbRelease: "BBBBBBBB-2222-4222-8222-222222222222"}, 1)
	add(labelPlay{station: "t-wfmu", show: "a", label: "T Evidence Label", artist: "T Known Artist", album: "Album Two", artistID: &known, mbRelease: radioCandidateTestMBB}, 2)
	add(labelPlay{station: "t-wfmu", show: "a", label: "T Evidence Label", artist: "T Known Artist", album: "Album Two", mbRelease: "not-an-id"}, 1)
	add(labelPlay{station: "t-rocknsoul", show: "a", label: "T Evidence Label", artist: "T Band Three", date: "2026-03-01"}, 1)
	// Ambiguous artist matches still count for the label.
	add(labelPlay{station: "t-rocknsoul", show: "a", label: "T Evidence Label", artist: "T Band Four", album: "  ", matchState: catalogm.RadioPlayMatchStateAmbiguous}, 1)

	got := s.all()
	s.Require().Contains(got, "t evidence label")
	c := got["t evidence label"]
	s.Equal("T Evidence Label", c.DisplayName)
	s.Equal(2, c.LabelSpellingCount, "spellings are trimmed, case kept")
	s.Equal([]contracts.RadioCandidateName{{Name: "T Evidence Label", PlayCount: 9}, {Name: "T EVIDENCE LABEL", PlayCount: 1}}, c.LabelSpellings)
	s.Equal(contracts.RadioCandidateLaneHuman, c.Lane)
	s.Equal(10, c.PlayCount)
	s.Equal(9, c.RecentPlayCount)
	s.Equal(0, c.PriorPlayCount)
	s.Equal(3, c.StationRowCount)
	s.Equal(2, c.StationFamilyCount)
	s.Equal(4, c.ShowCount)
	s.Equal("2026-03-01", c.FirstAiredOn)
	s.Equal(labelTestRecent, c.LastAiredOn)
	s.InDelta(RadioLabelCandidateScore(3, 10, 9, 0), c.Score, 1e-9)
	s.False(c.NonLatinSpelling)

	s.Equal([]contracts.RadioCandidateStation{
		{StationID: s.stations["t-kexp"], Slug: "t-kexp", Name: "t-kexp name", PlayCount: 5},
		{StationID: s.stations["t-wfmu"], Slug: "t-wfmu", Name: "t-wfmu name", PlayCount: 3},
		{StationID: s.stations["t-rocknsoul"], Slug: "t-rocknsoul", Name: "t-rocknsoul name", PlayCount: 2},
	}, c.Stations)

	s.Equal(4, c.ArtistCount)
	s.Equal([]contracts.RadioLabelCandidateArtist{
		{Name: "T Band One", PlayCount: 5, ArtistIDs: []uint{}},
		{Name: "T Known Artist", PlayCount: 3, ArtistIDs: []uint{known}},
		{Name: "T Band Four", PlayCount: 1, ArtistIDs: []uint{}},
		{Name: "T Band Three", PlayCount: 1, ArtistIDs: []uint{}},
	}, c.Artists)

	s.Equal(2, c.ReleaseNameCount, "a blank album is not a release")
	s.Equal([]contracts.RadioCandidateName{{Name: "Album One", PlayCount: 5}, {Name: "Album Two", PlayCount: 3}}, c.ReleaseNames)

	s.Equal(2, c.MBReleaseIDCount, "ids compare lower-cased; a value not shaped like a UUID is absent")
	s.Equal([]string{radioCandidateTestMBA, radioCandidateTestMBB}, c.MBReleaseIDs)
}

func (s *RadioLabelCandidatesIntegrationSuite) TestNonLatinSpelling() {
	// Both key on the Latin part; the flag records that the key may be a
	// fragment of the name.
	s.add(labelPlay{station: "t-kexp", label: "T Faye Records (王菲)"}, 2)
	s.add(labelPlay{station: "t-wfmu", label: "T Faye Records"}, 2)

	got := s.all()
	s.Require().Contains(got, "t faye records")
	s.True(got["t faye records"].NonLatinSpelling)
	s.Equal(2, got["t faye records"].LabelSpellingCount)
}

func (s *RadioLabelCandidatesIntegrationSuite) TestSortsAndRecentPlayTieBreak() {
	// 50 plays, 40 recent over 10 prior: velocity 4, score 2 x ln 50 x 4 = 31.3.
	s.add(labelPlay{station: "t-kexp", label: "T Plays"}, 39)
	s.add(labelPlay{station: "t-wfmu", label: "T Plays"}, 1)
	s.add(labelPlay{station: "t-kexp", label: "T Plays", date: labelTestPrior}, 10)
	// 3 rows, 3 new plays: velocity 3, score 3 x ln 3 x 3 = 9.9.
	for _, st := range []string{"t-kexp", "t-wfmu", "t-rocknsoul"} {
		s.add(labelPlay{station: st, label: "T Stations"}, 1)
	}
	// 6 plays, 5 recent over 1 prior: velocity 5, score 2 x ln 6 x 5 = 17.9.
	s.add(labelPlay{station: "t-kexp", label: "T Velocity"}, 3)
	s.add(labelPlay{station: "t-wfmu", label: "T Velocity"}, 2)
	s.add(labelPlay{station: "t-kexp", label: "T Velocity", date: labelTestPrior}, 1)
	// Identical counts (velocity 4, score 11.1); T Recent aired later.
	s.addAcross(labelPlay{label: "T Earlier", date: "2026-07-03"}, 2)
	s.addAcross(labelPlay{label: "T Recent", date: "2026-07-25"}, 2)

	keys := func(sort contracts.RadioLabelCandidateSort) []string {
		return labelCandidateKeys(s.list(contracts.RadioCandidateLaneFilterAll, sort, 10).Candidates)
	}
	s.Equal([]string{"t plays", "t velocity", "t recent", "t earlier", "t stations"}, keys(contracts.RadioLabelCandidateSortScore))
	s.Equal([]string{"t velocity", "t plays", "t recent", "t earlier", "t stations"}, keys(contracts.RadioLabelCandidateSortVelocity))
	s.Equal("t plays", keys(contracts.RadioLabelCandidateSortPlays)[0])
	s.Equal("t stations", keys(contracts.RadioLabelCandidateSortStations)[0])
}
