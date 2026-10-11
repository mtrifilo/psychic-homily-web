package catalog

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// RadioTracksIntegrationSuite runs the track key functions, the rebuild and the
// listing against a migrated Postgres. Fixture strings are raw values as the
// station providers log them.
type RadioTracksIntegrationSuite struct {
	suite.Suite
	testDB    *testutil.TestDatabase
	db        *gorm.DB
	stations  map[string]uint
	episodes  map[string]uint
	playCount int
}

func TestRadioTracksIntegrationSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	suite.Run(t, new(RadioTracksIntegrationSuite))
}

func (s *RadioTracksIntegrationSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
}

func (s *RadioTracksIntegrationSuite) TearDownSuite() {
	s.testDB.Cleanup()
}

func (s *RadioTracksIntegrationSuite) SetupTest() {
	s.cleanup()
	s.stations = map[string]uint{}
	s.episodes = map[string]uint{}
	// t-wfmu and t-rocknsoul share a network, like WFMU's flagship and a
	// stream-only channel; t-kexp and t-nts stand alone.
	network := &catalogm.RadioNetwork{Slug: "t-wfmu-net", Name: "t-wfmu-net"}
	s.Require().NoError(s.db.Create(network).Error)
	for _, slug := range []string{"t-kexp", "t-wfmu", "t-rocknsoul", "t-nts"} {
		station := &catalogm.RadioStation{Name: slug, Slug: slug, BroadcastType: catalogm.BroadcastTypeInternet}
		if slug == "t-wfmu" || slug == "t-rocknsoul" {
			station.NetworkID = &network.ID
		}
		s.Require().NoError(s.db.Create(station).Error)
		s.stations[slug] = station.ID
	}
}

func (s *RadioTracksIntegrationSuite) TearDownTest() {
	s.cleanup()
}

func (s *RadioTracksIntegrationSuite) cleanup() {
	for _, stmt := range []string{
		"DELETE FROM radio_track_keys",
		"DELETE FROM radio_tracks",
		"DELETE FROM radio_plays",
		"DELETE FROM radio_episodes",
		"DELETE FROM radio_shows",
		"DELETE FROM radio_stations WHERE slug LIKE 't-%'",
		"DELETE FROM radio_networks WHERE slug LIKE 't-%'",
	} {
		s.Require().NoError(s.db.Exec(stmt).Error)
	}
}

// episode returns the episode of show "<station>-<show>" airing on date,
// creating the show and episode on first use.
func (s *RadioTracksIntegrationSuite) episode(station, show, date string) uint {
	key := station + "|" + show + "|" + date
	if id, ok := s.episodes[key]; ok {
		return id
	}
	slug := station + "-" + show
	var sh catalogm.RadioShow
	s.Require().NoError(s.db.Where("slug = ?", slug).Limit(1).Find(&sh).Error)
	if sh.ID == 0 {
		sh = catalogm.RadioShow{StationID: s.stations[station], Name: slug, Slug: slug}
		s.Require().NoError(s.db.Create(&sh).Error)
	}
	ep := &catalogm.RadioEpisode{ShowID: sh.ID, AirDate: date}
	s.Require().NoError(s.db.Create(ep).Error)
	s.episodes[key] = ep.ID
	return ep.ID
}

type radioTrackFixturePlay struct {
	station, show, date string
	artist              string
	title, album, label *string
	mb                  *string
}

func (s *RadioTracksIntegrationSuite) addPlay(p radioTrackFixturePlay, n int) {
	if p.show == "" {
		p.show = "main"
	}
	if p.date == "" {
		p.date = "2026-07-01"
	}
	ep := s.episode(p.station, p.show, p.date)
	for range n {
		s.playCount++
		play := catalogm.RadioPlay{
			EpisodeID:              ep,
			Position:               s.playCount,
			ArtistName:             p.artist,
			TrackTitle:             p.title,
			AlbumTitle:             p.album,
			LabelName:              p.label,
			MusicBrainzRecordingID: p.mb,
			// Keeps repeated identical plays distinct under the
			// (episode_id, dedup_key) unique index.
			ProviderPlayID: stringPtr(fmt.Sprintf("t-%d", s.playCount)),
		}
		s.Require().NoError(s.db.Create(&play).Error)
	}
}

func (s *RadioTracksIntegrationSuite) rebuild() *RadioTracksRebuildReport {
	report, err := RebuildRadioTracks(context.Background(), s.db)
	s.Require().NoError(err)
	s.Require().False(report.Skipped)
	return report
}

func (s *RadioTracksIntegrationSuite) track(artistKey, titleKey string) catalogm.RadioTrack {
	var t catalogm.RadioTrack
	s.Require().NoError(s.db.Where("artist_key = ? AND title_key = ?", artistKey, titleKey).First(&t).Error,
		"no track for (%q, %q)", artistKey, titleKey)
	return t
}

func (s *RadioTracksIntegrationSuite) trackIDForKey(artistKey, titleKey string) uint {
	var k catalogm.RadioTrackKey
	s.Require().NoError(s.db.Where("artist_key = ? AND title_key = ?", artistKey, titleKey).First(&k).Error,
		"no key row for (%q, %q)", artistKey, titleKey)
	return k.TrackID
}

func (s *RadioTracksIntegrationSuite) count(table string) int64 {
	var n int64
	s.Require().NoError(s.db.Table(table).Count(&n).Error)
	return n
}

// TestTitleKeyFixtures pins radio_track_title_key and radio_track_artist_key
// on the stripping cases the key was chosen on, including the known
// false positives and the non-Latin case the SQL normalizer empties.
func (s *RadioTracksIntegrationSuite) TestTitleKeyFixtures() {
	titleCases := []struct{ raw, want string }{
		{"Here Comes My Baby (Mono)", "here comes my baby"},
		{"Whip It - Live", "whip it"},
		{"Whip It - Live (2004)", "whip it"},
		{"B.O.B. (Bombs Over Baghdad)", "b.o.b"},
		{"This Must Be the Place (Naive Melody)", "this must be the place"},
		{"Never Alone feat. The Interrupters", "never alone"},
		{"Never Alone ft. The Interrupters", "never alone"},
		{"Never Alone Featuring The Interrupters", "never alone"},
		{"Never Alone feat The Interrupters", "never alone"},
		{"Song (feat. Someone) [Remastered 2011]", "song"},
		{"Song {Demo}", "song"},
		{"Song - Radio Edit", "song"},
		{"Song – Extended Version", "song"},
		{"Song — Live at KEXP", "song"},
		{"Song - Stereo Mix", "song"},
		// Stripping empties these, so the unstripped normalized title is kept.
		{"(Intro)", "intro"},
		{"[Untitled]", "untitled"},
		// Not stripped: no keyword in the dash tail, or no dash before it.
		{"Song - 2004 Remaster", "song - 2004 remaster"},
		{"Live Forever", "live forever"},
		{"Liveforever - Liveness", "liveforever - liveness"},
		// Known over-strips of the chosen regexes, pinned so a change is seen.
		{"A Feat of Strength", "a"},
		{"Hung Up On My Baby - Part 1 (Live) Extended", "hung up on my baby"},
		// Normalization after stripping: accents, case, edge punctuation,
		// interior whitespace.
		{"Café   Lights!", "cafe lights"},
		{"  Spaced Out  (Live)  ", "spaced out"},
		// The SQL normalizer trims non-ASCII edges, so a fully non-Latin title
		// keys to empty and its play is unkeyed.
		{"사랑", ""},
		{"Любовь", ""},
		{"", ""},
		{"   ", ""},
	}
	for _, c := range titleCases {
		var got string
		s.Require().NoError(s.db.Raw("SELECT radio_track_title_key(?)", c.raw).Scan(&got).Error)
		s.Equal(c.want, got, "radio_track_title_key(%q)", c.raw)
	}

	var null *string
	s.Require().NoError(s.db.Raw("SELECT radio_track_title_key(NULL)").Scan(&null).Error)
	s.Nil(null, "a NULL title has no key")

	artistCases := []struct{ raw, want string }{
		{"Café Tacvba", "cafe tacvba"},
		{"DEVO", "devo"},
		{"ADULT.", "adult"},
		{"Dead Pioneers feat. The Interrupters", "dead pioneers feat. the interrupters"},
		{"방탄소년단", ""},
	}
	for _, c := range artistCases {
		var got string
		s.Require().NoError(s.db.Raw("SELECT radio_track_artist_key(?)", c.raw).Scan(&got).Error)
		s.Equal(c.want, got, "radio_track_artist_key(%q)", c.raw)
	}
}

func (s *RadioTracksIntegrationSuite) TestRebuildAggregatesOneTrackAcrossSpellingsAndStations() {
	mb := stringPtr("aaaaaaaa-0000-0000-0000-000000000001")
	s.addPlay(radioTrackFixturePlay{station: "t-kexp", artist: "Devo", title: stringPtr("Whip It"), mb: mb,
		album: stringPtr("Freedom of Choice"), label: stringPtr("Warner")}, 2)
	s.addPlay(radioTrackFixturePlay{station: "t-wfmu", artist: "DEVO", title: stringPtr("Whip It - Live"),
		album: stringPtr("   "), date: "2026-08-15"}, 1)
	s.addPlay(radioTrackFixturePlay{station: "t-rocknsoul", artist: "Devo", title: stringPtr("Whip It (Mono)"),
		label: stringPtr("Warner"), date: "2026-06-02"}, 1)
	s.addPlay(radioTrackFixturePlay{station: "t-kexp", show: "late", artist: "Devo", title: stringPtr("Whip It"),
		mb: stringPtr("AAAAAAAA-0000-0000-0000-000000000001")}, 1)

	report := s.rebuild()
	s.Equal(int64(5), report.Plays)
	s.Equal(int64(0), report.UnkeyedPlays)
	s.Equal(int64(1), report.Tracks)
	s.Equal(int64(1), report.Keys)
	s.Equal(int64(1), report.Written)

	t := s.track("devo", "whip it")
	s.Equal("Devo", t.DisplayArtist, "most common raw artist spelling")
	s.Equal("Whip It", t.DisplayTitle, "most common raw title spelling")
	s.Equal(5, t.PlayCount)
	s.Equal(3, t.StationRowCount, "kexp, wfmu, rocknsoul")
	s.Equal(2, t.StationFamilyCount, "kexp and the wfmu network")
	s.Equal(4, t.ShowCount, "two kexp shows, one wfmu, one rocknsoul")
	s.Equal("2026-06-02", t.FirstAiredOn.Format("2006-01-02"))
	s.Equal("2026-08-15", t.LastAiredOn.Format("2006-01-02"))
	s.Require().NotNil(t.MBRecordingID, "tagged plays agree once lowercased")
	s.Equal(*mb, *t.MBRecordingID)
	s.False(t.MBRecordingIDConflict)
	s.Equal(2, t.AlbumPlayCount, "a whitespace-only album does not count")
	s.Equal(3, t.LabelPlayCount)
	s.False(t.Unkeyed)
	s.Equal(t.ID, s.trackIDForKey("devo", "whip it"))
}

func (s *RadioTracksIntegrationSuite) TestRebuildKeepsUnkeyedPlaysOnTheMarkerRow() {
	s.addPlay(radioTrackFixturePlay{station: "t-nts", artist: "방탄소년단", title: stringPtr("Dynamite"),
		mb: stringPtr("bbbbbbbb-0000-0000-0000-000000000001")}, 2)
	s.addPlay(radioTrackFixturePlay{station: "t-wfmu", artist: "Someone", title: nil}, 1)
	s.addPlay(radioTrackFixturePlay{station: "t-wfmu", artist: "Someone", title: stringPtr("  ")}, 1)
	s.addPlay(radioTrackFixturePlay{station: "t-kexp", artist: "Someone", title: stringPtr("사랑")}, 1)
	s.addPlay(radioTrackFixturePlay{station: "t-kexp", artist: "Someone", title: stringPtr("Real Song")}, 1)

	report := s.rebuild()
	s.Equal(int64(6), report.Plays)
	s.Equal(int64(5), report.UnkeyedPlays)
	s.Equal(int64(2), report.Tracks, "the marker row and one keyed track")

	marker := s.track("", "")
	s.True(marker.Unkeyed)
	s.Equal(5, marker.PlayCount)
	s.Equal(3, marker.StationRowCount)
	s.Equal("", marker.DisplayArtist)
	s.Equal("", marker.DisplayTitle)
	s.Nil(marker.MBRecordingID, "the marker row aggregates unrelated plays, so it carries no recording id")
	s.False(marker.MBRecordingIDConflict)
	s.Equal(marker.ID, s.trackIDForKey("", ""))

	s.Equal(1, s.track("someone", "real song").PlayCount)
}

func (s *RadioTracksIntegrationSuite) TestRecordingIDJoinsKeysTransitively() {
	m1 := stringPtr("cccccccc-0000-0000-0000-000000000001")
	m2 := stringPtr("cccccccc-0000-0000-0000-000000000002")
	// Two artist-credit spellings of one recording: two keys, one track.
	s.addPlay(radioTrackFixturePlay{station: "t-kexp", artist: "Dead Pioneers feat. The Interrupters", title: stringPtr("Never Alone"), mb: m1}, 4)
	s.addPlay(radioTrackFixturePlay{station: "t-kexp", artist: "Dead Pioneers ft. The Interrupters", title: stringPtr("Never Alone"), mb: m1}, 1)
	// An untagged play on another station under the second key joins with it.
	s.addPlay(radioTrackFixturePlay{station: "t-nts", artist: "Dead Pioneers ft. The Interrupters", title: stringPtr("Never Alone")}, 1)
	// A third key shares a second recording id with the second key only.
	s.addPlay(radioTrackFixturePlay{station: "t-kexp", artist: "Dead Pioneers ft. The Interrupters", title: stringPtr("Never Alone"), mb: m2}, 1)
	s.addPlay(radioTrackFixturePlay{station: "t-wfmu", artist: "Dead Pioneers & The Interrupters", title: stringPtr("Never Alone"), mb: m2}, 1)

	report := s.rebuild()
	s.Equal(2, report.JoinedKeys)
	s.Equal(int64(1), report.Tracks)
	s.Equal(int64(3), report.Keys)

	t := s.track("dead pioneers & the interrupters", "never alone")
	s.Equal(8, t.PlayCount)
	s.Equal(3, t.StationRowCount)
	s.Equal(3, t.StationFamilyCount)
	s.Equal("Dead Pioneers feat. The Interrupters", t.DisplayArtist)
	s.Nil(t.MBRecordingID, "the joined track carries two recording ids")
	s.True(t.MBRecordingIDConflict)
	for _, artist := range []string{"dead pioneers feat. the interrupters", "dead pioneers ft. the interrupters", "dead pioneers & the interrupters"} {
		s.Equal(t.ID, s.trackIDForKey(artist, "never alone"), "key %q maps to the joined track", artist)
	}

	listing, err := NewRadioTracksReader(s.db).ListTopRadioTracks(context.Background(), contracts.RadioTrackSortPlays, 10)
	s.Require().NoError(err)
	s.Require().Len(listing.Tracks, 1)
	s.Equal(3, listing.Tracks[0].KeyCount)
}

func (s *RadioTracksIntegrationSuite) TestRecordingIDNeverDividesAKey() {
	s.addPlay(radioTrackFixturePlay{station: "t-kexp", artist: "Cornershop", title: stringPtr("Brimful of Asha"),
		mb: stringPtr("dddddddd-0000-0000-0000-000000000001")}, 2)
	s.addPlay(radioTrackFixturePlay{station: "t-kexp", artist: "Cornershop", title: stringPtr("Brimful of Asha (Norman Cook Remix)"),
		mb: stringPtr("dddddddd-0000-0000-0000-000000000002")}, 1)
	s.addPlay(radioTrackFixturePlay{station: "t-wfmu", artist: "Cornershop", title: stringPtr("Brimful of Asha")}, 1)

	report := s.rebuild()
	s.Equal(0, report.JoinedKeys)
	s.Equal(int64(1), report.Tracks)

	t := s.track("cornershop", "brimful of asha")
	s.Equal(4, t.PlayCount)
	s.Nil(t.MBRecordingID)
	s.True(t.MBRecordingIDConflict)
}

func (s *RadioTracksIntegrationSuite) TestRebuildIsIdempotentAndKeepsTrackIDs() {
	s.addPlay(radioTrackFixturePlay{station: "t-kexp", artist: "Suki Waterhouse", title: stringPtr("Back in Love")}, 2)
	s.addPlay(radioTrackFixturePlay{station: "t-nts", artist: "Gelli Haha", title: stringPtr("Klouds")}, 1)
	s.rebuild()
	before := s.track("suki waterhouse", "back in love")

	again := s.rebuild()
	s.Equal(int64(0), again.Written, "a rebuild over unchanged plays writes no track row")
	s.Equal(int64(0), again.Deleted)
	after := s.track("suki waterhouse", "back in love")
	s.Equal(before.ID, after.ID)
	s.True(before.RebuiltAt.Equal(after.RebuiltAt), "an unchanged row keeps its rebuilt_at")

	s.addPlay(radioTrackFixturePlay{station: "t-wfmu", artist: "Suki Waterhouse", title: stringPtr("Back In Love (Edit)")}, 1)
	changed := s.rebuild()
	s.Equal(int64(1), changed.Written, "only the track the new play touched is rewritten")
	updated := s.track("suki waterhouse", "back in love")
	s.Equal(before.ID, updated.ID, "a track keeps its id while its key names a track")
	s.Equal(3, updated.PlayCount)
	s.Equal(2, updated.StationFamilyCount)
}

func (s *RadioTracksIntegrationSuite) TestRebuildDeletesTracksWhosePlaysAreGone() {
	s.addPlay(radioTrackFixturePlay{station: "t-kexp", artist: "Kept", title: stringPtr("Song")}, 1)
	s.addPlay(radioTrackFixturePlay{station: "t-kexp", artist: "Gone", title: stringPtr("Song")}, 1)
	s.rebuild()
	s.Equal(int64(2), s.count("radio_tracks"))

	s.Require().NoError(s.db.Exec("DELETE FROM radio_plays WHERE artist_name = 'Gone'").Error)
	report := s.rebuild()
	s.Equal(int64(1), report.Deleted)
	s.Equal(int64(1), s.count("radio_tracks"))
	s.Equal(int64(1), s.count("radio_track_keys"))
	s.Equal(1, s.track("kept", "song").PlayCount)
}

// A key joined into an existing track keeps that track's id even when the new
// key sorts first, and a key that stops sharing a recording id gets its own
// track back.
func (s *RadioTracksIntegrationSuite) TestJoinKeepsTheExistingTrackID() {
	mb := stringPtr("eeeeeeee-0000-0000-0000-000000000001")
	s.addPlay(radioTrackFixturePlay{station: "t-kexp", artist: "Steve Lacy, SZA", title: stringPtr("Is It Cool"), mb: mb}, 2)
	s.rebuild()
	existing := s.track("steve lacy, sza", "is it cool")

	s.addPlay(radioTrackFixturePlay{station: "t-nts", artist: "Steve Lacy & SZA", title: stringPtr("Is It Cool"), mb: mb}, 1)
	report := s.rebuild()
	s.Equal(1, report.JoinedKeys)
	s.Equal(int64(1), report.Tracks)
	joined := s.track("steve lacy, sza", "is it cool")
	s.Equal(existing.ID, joined.ID)
	s.Equal(3, joined.PlayCount)
	s.Equal(existing.ID, s.trackIDForKey("steve lacy & sza", "is it cool"))

	s.Require().NoError(s.db.Exec("UPDATE radio_plays SET musicbrainz_recording_id = NULL WHERE artist_name = 'Steve Lacy & SZA'").Error)
	s.rebuild()
	s.Equal(int64(2), s.count("radio_tracks"))
	s.Equal(existing.ID, s.track("steve lacy, sza", "is it cool").ID)
	s.NotEqual(existing.ID, s.trackIDForKey("steve lacy & sza", "is it cool"))
}

func (s *RadioTracksIntegrationSuite) TestConcurrentRebuildIsSkipped() {
	s.addPlay(radioTrackFixturePlay{station: "t-kexp", artist: "Anyone", title: stringPtr("Song")}, 1)
	holder := s.db.Begin()
	s.Require().NoError(holder.Error)
	defer holder.Rollback()
	s.Require().NoError(holder.Exec("SELECT pg_advisory_xact_lock(?)", radioTracksRebuildLock).Error)

	report, err := RebuildRadioTracks(context.Background(), s.db)
	s.Require().NoError(err)
	s.True(report.Skipped)
	s.Equal(int64(0), s.count("radio_tracks"))
}

func (s *RadioTracksIntegrationSuite) TestListTopRadioTracks() {
	// a: 4 plays on one station. b: 3 plays on 3 rows, 2 families.
	// c: 2 plays on 2 families. The marker row has 5 plays and is never listed.
	s.addPlay(radioTrackFixturePlay{station: "t-wfmu", artist: "A", title: stringPtr("One")}, 4)
	s.addPlay(radioTrackFixturePlay{station: "t-wfmu", artist: "B", title: stringPtr("Two")}, 1)
	s.addPlay(radioTrackFixturePlay{station: "t-rocknsoul", artist: "B", title: stringPtr("Two")}, 1)
	s.addPlay(radioTrackFixturePlay{station: "t-kexp", artist: "B", title: stringPtr("Two")}, 1)
	s.addPlay(radioTrackFixturePlay{station: "t-kexp", artist: "C", title: stringPtr("Three")}, 1)
	s.addPlay(radioTrackFixturePlay{station: "t-nts", artist: "C", title: stringPtr("Three")}, 1)
	s.addPlay(radioTrackFixturePlay{station: "t-nts", artist: "Nobody", title: nil}, 5)
	s.rebuild()

	reader := NewRadioTracksReader(s.db)
	order := func(sort contracts.RadioTrackSort, limit int) []string {
		listing, err := reader.ListTopRadioTracks(context.Background(), sort, limit)
		s.Require().NoError(err)
		s.Equal(int64(3), listing.TotalTracks)
		s.Equal(int64(5), listing.UnkeyedPlays)
		var keys []string
		for _, t := range listing.Tracks {
			keys = append(keys, t.ArtistKey)
		}
		return keys
	}
	s.Equal([]string{"a", "b", "c"}, order(contracts.RadioTrackSortPlays, 10))
	s.Equal([]string{"b", "c", "a"}, order(contracts.RadioTrackSortStationRows, 10))
	s.Equal([]string{"b", "c", "a"}, order(contracts.RadioTrackSortStationFamilies, 10))
	s.Equal([]string{"a"}, order(contracts.RadioTrackSortPlays, 1))

	listing, err := reader.ListTopRadioTracks(context.Background(), contracts.RadioTrackSortPlays, 1)
	s.Require().NoError(err)
	top := listing.Tracks[0]
	s.Equal("A", top.DisplayArtist)
	s.Equal("One", top.DisplayTitle)
	s.Equal("2026-07-01", top.FirstAiredOn)
	s.Equal(1, top.KeyCount)

	_, err = reader.ListTopRadioTracks(context.Background(), contracts.RadioTrackSort("nope"), 10)
	s.Error(err)
}
