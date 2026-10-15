package catalog

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/testutil"
)

// RadioPlayExclusionsIntegrationSuite runs the seeded radio_play_exclusions
// rows and the SQL predicate against a migrated Postgres. Fixture strings are
// raw values as the station providers log them.
type RadioPlayExclusionsIntegrationSuite struct {
	suite.Suite
	testDB        *testutil.TestDatabase
	db            *gorm.DB
	seededMaxID   uint
	seededCount   int64
	episodeBySlug map[string]uint
	stationBySlug map[string]uint
}

func TestRadioPlayExclusionsIntegrationSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	suite.Run(t, new(RadioPlayExclusionsIntegrationSuite))
}

var radioExclusionTestCols = RadioPlayColumns{
	StationID:  "rsh.station_id",
	ArtistName: "rp.artist_name",
	TrackTitle: "rp.track_title",
	LabelName:  "rp.label_name",
	AlbumTitle: "rp.album_title",
}

const radioExclusionTestFrom = `
FROM radio_plays rp
JOIN radio_episodes re ON re.id = rp.episode_id
JOIN radio_shows rsh ON rsh.id = re.show_id`

func (s *RadioPlayExclusionsIntegrationSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
	s.Require().NoError(s.db.Model(&catalogm.RadioPlayExclusion{}).Count(&s.seededCount).Error)
	s.Require().NoError(s.db.Raw("SELECT coalesce(max(id), 0) FROM radio_play_exclusions").Scan(&s.seededMaxID).Error)
}

func (s *RadioPlayExclusionsIntegrationSuite) TearDownSuite() {
	s.testDB.Cleanup()
}

func (s *RadioPlayExclusionsIntegrationSuite) SetupTest() {
	s.cleanup()
	s.episodeBySlug = map[string]uint{}
	s.stationBySlug = map[string]uint{}
	// t-wfmu, t-rocknsoul and t-sheena share one network, like WFMU's flagship
	// and its stream-only channels; t-kexp and t-nts stand alone.
	network := &catalogm.RadioNetwork{Slug: "t-wfmu-net", Name: "t-wfmu-net"}
	s.Require().NoError(s.db.Create(network).Error)
	for _, slug := range []string{"t-kexp", "t-wfmu", "t-rocknsoul", "t-sheena", "t-nts"} {
		station := &catalogm.RadioStation{Name: slug, Slug: slug, BroadcastType: catalogm.BroadcastTypeInternet}
		if slug == "t-wfmu" || slug == "t-rocknsoul" || slug == "t-sheena" {
			station.NetworkID = &network.ID
		}
		s.Require().NoError(s.db.Create(station).Error)
		show := &catalogm.RadioShow{StationID: station.ID, Name: slug + " show", Slug: slug + "-show"}
		s.Require().NoError(s.db.Create(show).Error)
		episode := &catalogm.RadioEpisode{ShowID: show.ID, AirDate: "2026-07-01"}
		s.Require().NoError(s.db.Create(episode).Error)
		s.stationBySlug[slug] = station.ID
		s.episodeBySlug[slug] = episode.ID
	}
}

func (s *RadioPlayExclusionsIntegrationSuite) TearDownTest() {
	s.cleanup()
}

// cleanup removes fixture rows and any exclusion a test added, keeping the
// migration-seeded exclusions.
func (s *RadioPlayExclusionsIntegrationSuite) cleanup() {
	s.Require().NoError(s.db.Exec("DELETE FROM radio_play_exclusions WHERE id > ?", s.seededMaxID).Error)
	s.Require().NoError(s.db.Exec("DELETE FROM radio_plays").Error)
	s.Require().NoError(s.db.Exec("DELETE FROM radio_episodes").Error)
	s.Require().NoError(s.db.Exec("DELETE FROM radio_shows").Error)
	s.Require().NoError(s.db.Exec("DELETE FROM radio_stations WHERE slug LIKE 't-%'").Error)
	s.Require().NoError(s.db.Exec("DELETE FROM radio_networks WHERE slug LIKE 't-%'").Error)
}

// addPlays inserts n copies of a play on the station; title and label are nil
// when empty.
func (s *RadioPlayExclusionsIntegrationSuite) addPlays(station, artist, title, label string, n int) {
	plays := make([]catalogm.RadioPlay, n)
	for i := range plays {
		plays[i] = catalogm.RadioPlay{
			EpisodeID:  s.episodeBySlug[station],
			Position:   i,
			ArtistName: artist,
			// ProviderPlayID keeps repeated identical plays distinct under the
			// (episode_id, dedup_key) unique index.
			ProviderPlayID: stringPtr(fmt.Sprintf("%s|%s|%s|%d", station, artist, title, i)),
		}
		if title != "" {
			plays[i].TrackTitle = stringPtr(title)
		}
		if label != "" {
			plays[i].LabelName = stringPtr(label)
		}
	}
	s.Require().NoError(s.db.CreateInBatches(plays, 200).Error)
}

// addAlbumPlay inserts one play carrying a title, an album and a label on the
// station.
func (s *RadioPlayExclusionsIntegrationSuite) addAlbumPlay(station, artist, title, album, label string) {
	s.Require().NoError(s.db.Create(&catalogm.RadioPlay{
		EpisodeID:      s.episodeBySlug[station],
		ArtistName:     artist,
		TrackTitle:     stringPtr(title),
		AlbumTitle:     stringPtr(album),
		LabelName:      stringPtr(label),
		ProviderPlayID: stringPtr(fmt.Sprintf("%s|%s|%s|%s|%s", station, artist, title, album, label)),
	}).Error)
}

type radioPlayVerdict struct {
	PlayID     uint
	StationID  uint
	ArtistRaw  string
	ArtistNorm *string
	TitleNorm  *string
	LabelNorm  *string
	AlbumNorm  *string
	ExArtist   bool
	ExTrack    bool
	ExLabel    bool
	ExAlbum    bool
}

// verdicts evaluates RadioPlayExcludedSQL for every fixture play and every
// dimension, together with the SQL-normalized fields the Go predicate takes.
func (s *RadioPlayExclusionsIntegrationSuite) verdicts() []radioPlayVerdict {
	q := fmt.Sprintf(`SELECT rp.id AS play_id, rsh.station_id AS station_id, rp.artist_name AS artist_raw,
       %s AS artist_norm, %s AS title_norm, %s AS label_norm, %s AS album_norm,
       %s AS ex_artist, %s AS ex_track, %s AS ex_label, %s AS ex_album %s ORDER BY rp.id`,
		RadioNormalizedFieldSQL("rp.artist_name"),
		RadioNormalizedFieldSQL("rp.track_title"),
		RadioNormalizedFieldSQL("rp.label_name"),
		RadioNormalizedFieldSQL("rp.album_title"),
		RadioPlayExcludedSQL(RadioRankingArtist, radioExclusionTestCols),
		RadioPlayExcludedSQL(RadioRankingTrack, radioExclusionTestCols),
		RadioPlayExcludedSQL(RadioRankingLabel, radioExclusionTestCols),
		RadioPlayExcludedSQL(RadioRankingAlbum, radioExclusionTestCols),
		radioExclusionTestFrom)
	var out []radioPlayVerdict
	s.Require().NoError(s.db.Raw(q).Scan(&out).Error)
	return out
}

func (s *RadioPlayExclusionsIntegrationSuite) verdictFor(artistRaw string) radioPlayVerdict {
	for _, v := range s.verdicts() {
		if v.ArtistRaw == artistRaw {
			return v
		}
	}
	s.FailNow("no fixture play for artist " + artistRaw)
	return radioPlayVerdict{}
}

// assertGoAgreesWithSQL checks RadioPlayExclusions.Excludes against the SQL
// predicate for every fixture play and dimension.
func (s *RadioPlayExclusionsIntegrationSuite) assertGoAgreesWithSQL() {
	excl, err := LoadRadioPlayExclusions(s.db)
	s.Require().NoError(err)
	for _, v := range s.verdicts() {
		play := RadioNormalizedPlay{StationID: v.StationID, ArtistName: v.ArtistNorm, TrackTitle: v.TitleNorm, LabelName: v.LabelNorm, AlbumTitle: v.AlbumNorm}
		s.Equal(v.ExArtist, excl.Excludes(RadioRankingArtist, play), "artist dimension, play %q", v.ArtistRaw)
		s.Equal(v.ExTrack, excl.Excludes(RadioRankingTrack, play), "track dimension, play %q", v.ArtistRaw)
		s.Equal(v.ExLabel, excl.Excludes(RadioRankingLabel, play), "label dimension, play %q", v.ArtistRaw)
		s.Equal(v.ExAlbum, excl.Excludes(RadioRankingAlbum, play), "album dimension, play %q", v.ArtistRaw)
	}
}

func (s *RadioPlayExclusionsIntegrationSuite) TestSeededRowsLoadInGo() {
	s.Greater(s.seededCount, int64(0))
	var rows []catalogm.RadioPlayExclusion
	s.Require().NoError(s.db.Find(&rows).Error)
	for _, r := range rows {
		s.NotEmpty(r.Reason, "row %d has no reason", r.ID)
		s.Nil(r.StationID, "seeded row %d is global", r.ID)
	}
	_, err := NewRadioPlayExclusions(rows)
	s.NoError(err, "every seeded regex must be inside the portable subset")
}

// The most-played non-music keys of the stage classification, as logged. Each
// carries a title and a real label, so every dimension is excluded by the rule
// itself rather than by a blank key field.
func (s *RadioPlayExclusionsIntegrationSuite) TestTopStationElementsAndTalkAreExcluded() {
	noise := []struct{ station, artist, title string }{
		{"t-rocknsoul", "Music behind DJ: Mr. Weather", "Current Weather"},
		{"t-wfmu", "Music behind DJ:", ""},
		{"t-wfmu", "Music behind DJ: BACK ANNOUNCE", ""},
		{"t-sheena", "Your DJ Speaks", ""},
		{"t-rocknsoul", "Music behind DJ: The Vampires' Sound Incorporation", "There Is No Satisfaction"},
		{"t-rocknsoul", "Music behind DJ: Dorothy Ashby", "Soul Vibrations"},
		{"t-sheena", "Where The Action Is! with Dr. Jeff", "Your DJ speaks"},
		{"t-wfmu", "Music behind DJ: Intro! Yeah! Party!!", ""},
		{"t-wfmu", "The Pidge Speaks", "Good Morning Glisteners!"},
		{"t-sheena", "Bob Barth", "WE ARE ON A BREAK"},
		{"t-rocknsoul", "-mic break-", ""},
		{"t-rocknsoul", "TV Clip", ""},
		{"t-wfmu", "Clip", "CBS-Cuba releases more than 2,000 prisoners"},
		{"t-wfmu", "Wake Weather", ""},
		{"t-rocknsoul", "DJ Terre T speaks", ""},
		{"t-wfmu", "Music behind DJ: Music behind DJ: Harry Stoneham", "Do You Know the Way to San Jose?"},
	}
	for _, n := range noise {
		title := n.title
		if title == "" {
			title = "Segment"
		}
		s.addPlays(n.station, n.artist, title, "Cadet", 1)
	}
	// Control: the same title and label under a real artist count everywhere.
	s.addPlays("t-kexp", "Dorothy Ashby", "Segment", "Cadet", 1)
	for _, v := range s.verdicts() {
		s.Require().NotNil(v.LabelNorm)
		s.Require().NotNil(v.TitleNorm)
		if v.ArtistRaw == "Dorothy Ashby" {
			s.False(v.ExArtist || v.ExTrack || v.ExLabel, "control play must count toward every ranking")
			continue
		}
		s.True(v.ExArtist && v.ExTrack && v.ExLabel, "noise play %q must be excluded from every ranking", v.ArtistRaw)
	}
	s.assertGoAgreesWithSQL()
}

func (s *RadioPlayExclusionsIntegrationSuite) TestKnownMultiStationSongsAreKept() {
	s.addPlays("t-kexp", "Man/Woman/Chainsaw", "Nosedive", "Partisan Records", 3)
	s.addPlays("t-wfmu", "Man/Woman/Chainsaw", "Nosedive", "Partisan Records", 2)
	s.addPlays("t-sheena", "Man/Woman/Chainsaw", "Nosedive", "Partisan Records", 1)
	// The plain twin of a "Music behind DJ:" bed still counts.
	s.addPlays("t-kexp", "Dorothy Ashby", "Soul Vibrations", "Cadet", 1)
	s.addPlays("t-nts", "Dorothy Ashby", "Soul Vibrations", "Cadet", 1)
	// Real artists that only look like talk markers.
	s.addPlays("t-kexp", "Widowspeak", "If You Change", "Captured Tracks", 1)
	s.addPlays("t-kexp", "Trees Speak", "Ghost We Know", "Soul Jazz", 1)
	s.addPlays("t-kexp", "Unknown Mortal Orchestra", "Multi-Love", "Jagjaguwar", 1)
	for _, v := range s.verdicts() {
		s.False(v.ExArtist || v.ExTrack || v.ExLabel, "music play %q must not be excluded", v.ArtistRaw)
	}
	s.assertGoAgreesWithSQL()
}

func (s *RadioPlayExclusionsIntegrationSuite) TestPlaceholderLabelExcludesOnlyTheLabel() {
	s.addPlays("t-kexp", "Lael Neale", "Star", "[no label]", 1)
	s.addPlays("t-wfmu", "Hand Habits", "Wildfire", "self-released [Bandcamp]", 1)
	s.addPlays("t-sheena", "Blondshell", "Joiner", "Self Released - Blondshell", 1)
	s.addPlays("t-wfmu", "Palm Ghosts", "Lullaby", "Not On Label (Palm Ghosts Self-released)", 1)
	s.addPlays("t-kexp", "Some Band", "Some Song", "Bandcamp", 1)
	for _, v := range s.verdicts() {
		s.True(v.ExLabel, "placeholder label of %q must be excluded from the label ranking", v.ArtistRaw)
		s.False(v.ExArtist, "%q must still count toward the artist ranking", v.ArtistRaw)
		s.False(v.ExTrack, "%q must still count toward the track ranking", v.ArtistRaw)
	}
	s.assertGoAgreesWithSQL()
}

func (s *RadioPlayExclusionsIntegrationSuite) TestRealLabelsAreKept() {
	for _, label := range []string{"Sub Pop Records", "Columbia", "KEXP", "WFMU", "Self Help Records", "No Labels Collective"} {
		s.addPlays("t-kexp", "Artist for "+label, "Song", label, 1)
	}
	for _, v := range s.verdicts() {
		s.False(v.ExLabel, "label of %q is a real label string", v.ArtistRaw)
	}
	s.assertGoAgreesWithSQL()
}

func (s *RadioPlayExclusionsIntegrationSuite) TestPlaceholderArtistKeepsLabel() {
	s.addPlays("t-nts", "Unknown Artist", "Unknown Track", "Sub Pop", 1)
	v := s.verdictFor("Unknown Artist")
	s.True(v.ExArtist)
	s.True(v.ExTrack)
	s.False(v.ExLabel, "a placeholder artist does not disqualify a real label")
	s.assertGoAgreesWithSQL()
}

func (s *RadioPlayExclusionsIntegrationSuite) TestNonLatinNamesAreNotNoise() {
	// radio_normalize_name trims ASCII-only edges, so these normalize to ''.
	// They are present values, not blank ones, and no rule matches them.
	s.addPlays("t-kexp", "검정치마", "everything", "Cheongchun Baekseo", 1)
	s.addPlays("t-nts", "ドレスコーズ", "ビューティフル", "日本コロムビア", 1)
	for _, v := range s.verdicts() {
		s.Require().NotNil(v.ArtistNorm)
		s.Equal("", *v.ArtistNorm, "fixture premise: %q normalizes to empty in SQL", v.ArtistRaw)
		s.False(v.ExArtist || v.ExTrack || v.ExLabel, "non-Latin play %q must not be excluded", v.ArtistRaw)
	}
	s.assertGoAgreesWithSQL()
}

func (s *RadioPlayExclusionsIntegrationSuite) TestStationScopedRule() {
	sheena := s.stationBySlug["t-sheena"]
	s.Require().NoError(s.db.Create(&catalogm.RadioPlayExclusion{
		MatchKind: "track_title", PatternType: "exact", Pattern: "announcements", Category: "talk",
		StationID: &sheena, Reason: "test",
	}).Error)
	s.addPlays("t-sheena", "Bob Barth", "Announcements", "Cadet", 1)
	s.addPlays("t-kexp", "The Announcers", "Announcements", "Cadet", 1)
	barth := s.verdictFor("Bob Barth")
	s.True(barth.ExArtist && barth.ExTrack && barth.ExLabel, "a scoped talk rule excludes the play from every ranking on its station")
	announcers := s.verdictFor("The Announcers")
	s.False(announcers.ExArtist || announcers.ExTrack || announcers.ExLabel, "the scoped rule does not apply on other stations")
	s.assertGoAgreesWithSQL()
}

type radioTrackFloorRow struct {
	Artist                  string
	Title                   string
	Plays                   int
	Stations                int
	Broadcasters            int
	Qualifies               bool
	QualifiesOnBroadcasters bool
}

// trackFloors groups non-excluded plays by normalized artist and title and
// reports, for every key, the single-station rule (station rows) and the
// stricter broadcaster floor. It checks that each SQL floor agrees with its Go
// form on every key, kept or not.
func (s *RadioPlayExclusionsIntegrationSuite) trackFloors() map[string]radioTrackFloorRow {
	q := fmt.Sprintf(`SELECT radio_normalize_name(rp.artist_name) AS artist, radio_normalize_name(rp.track_title) AS title,
       count(*) AS plays, count(DISTINCT rsh.station_id) AS stations, count(DISTINCT %s) AS broadcasters,
       %s AS qualifies, %s AS qualifies_on_broadcasters
%s
JOIN radio_stations rst ON rst.id = rsh.station_id
WHERE NOT (%s)
GROUP BY 1, 2`, RadioBroadcasterKeySQL("rst"), RadioStationFloorSQL("rsh.station_id"), RadioBroadcasterFloorSQL("rst"),
		radioExclusionTestFrom, RadioPlayExcludedSQL(RadioRankingTrack, radioExclusionTestCols))
	var rows []radioTrackFloorRow
	s.Require().NoError(s.db.Raw(q).Scan(&rows).Error)
	out := map[string]radioTrackFloorRow{}
	for _, r := range rows {
		s.Equal(r.Qualifies, RadioMeetsStationFloor(r.Stations), "station floor: SQL and Go agree for %s / %s", r.Artist, r.Title)
		s.Equal(r.QualifiesOnBroadcasters, RadioMeetsBroadcasterFloor(r.Broadcasters), "broadcaster floor: SQL and Go agree for %s / %s", r.Artist, r.Title)
		out[r.Artist+" / "+r.Title] = r
	}
	return out
}

// qualifyingTracks keeps the keys that pass the single-station rule.
func (s *RadioPlayExclusionsIntegrationSuite) qualifyingTracks() map[string]radioTrackFloorRow {
	out := map[string]radioTrackFloorRow{}
	for k, r := range s.trackFloors() {
		if r.Qualifies {
			out[k] = r
		}
	}
	return out
}

func (s *RadioPlayExclusionsIntegrationSuite) TestSingleStationRuleIgnoresVolume() {
	s.addPlays("t-rocknsoul", "Nirvana", "C Side of Ocho Rios", "", 150)
	s.addPlays("t-kexp", "Suki Waterhouse", "Back in Love", "", 1)
	s.addPlays("t-wfmu", "Suki Waterhouse", "Back in Love", "", 1)

	got := s.qualifyingTracks()
	s.NotContains(got, "nirvana / c side of ocho rios", "150 plays on one station never qualify")
	s.Contains(got, "suki waterhouse / back in love")
	s.Equal(2, got["suki waterhouse / back in love"].Stations)
}

func (s *RadioPlayExclusionsIntegrationSuite) TestStationFloorCountsRowsBroadcasterFloorCollapsesNetworks() {
	// Three stations of one network: the single-station rule counts each row,
	// so the key qualifies; the stricter broadcaster floor counts one network
	// and rejects it until an independent broadcaster plays it.
	s.addPlays("t-rocknsoul", "The Turtles", "Buzzsaw", "", 20)
	s.addPlays("t-wfmu", "The Turtles", "Buzzsaw", "", 5)
	s.addPlays("t-sheena", "The Turtles", "Buzzsaw", "", 1)
	floors := s.trackFloors()["the turtles / buzzsaw"]
	s.Equal(3, floors.Stations)
	s.Equal(1, floors.Broadcasters)
	s.True(floors.Qualifies, "network siblings are separate stations under the single-station rule")
	s.False(floors.QualifiesOnBroadcasters, "network siblings are one broadcaster")

	s.addPlays("t-kexp", "The Turtles", "Buzzsaw", "", 1)
	floors = s.trackFloors()["the turtles / buzzsaw"]
	s.Equal(4, floors.Stations)
	s.Equal(2, floors.Broadcasters)
	s.True(floors.Qualifies)
	s.True(floors.QualifiesOnBroadcasters, "an independent broadcaster passes the stricter floor")
}

func (s *RadioPlayExclusionsIntegrationSuite) TestBlankKeyFieldsNeverCount() {
	s.addPlays("t-kexp", "Lael Neale", "Star", "", 1)
	s.addPlays("t-nts", "Hand Habits", "Wildfire", "   ", 1)
	s.addPlays("t-kexp", "Sub Pop Band", "   ", "Sub Pop", 1)

	for _, v := range s.verdicts() {
		switch v.ArtistRaw {
		case "Lael Neale", "Hand Habits":
			s.True(v.ExLabel, "%q has no label, so it never counts toward a label ranking", v.ArtistRaw)
			s.False(v.ExArtist || v.ExTrack, "%q still counts toward artist and track rankings", v.ArtistRaw)
		case "Sub Pop Band":
			s.True(v.ExTrack, "a blank title never counts toward a track ranking")
			s.False(v.ExArtist || v.ExLabel)
		}
	}

	// The NULL and blank labels span two broadcasters, yet form no label key.
	q := fmt.Sprintf(`SELECT count(*) %s WHERE NOT (%s)`,
		radioExclusionTestFrom, RadioPlayExcludedSQL(RadioRankingLabel, radioExclusionTestCols))
	var labelPlays int64
	s.Require().NoError(s.db.Raw(q).Scan(&labelPlays).Error)
	s.Equal(int64(1), labelPlays, "only the Sub Pop play counts toward a label ranking")
	s.assertGoAgreesWithSQL()
}

// The table's CHECK constraint and the Go loader accept exactly the same regex
// patterns.
func (s *RadioPlayExclusionsIntegrationSuite) TestRegexCorpusAgreesWithGoLoader() {
	for _, c := range radioExclusionRegexCorpus {
		row := catalogm.RadioPlayExclusion{MatchKind: "track_title", PatternType: "regex", Pattern: c.pattern, Category: "talk", Reason: "corpus"}
		err := s.db.Create(&row).Error
		if c.accept {
			s.NoError(err, "the constraint must accept %q", c.pattern)
		} else {
			s.Error(err, "the constraint must refuse %q", c.pattern)
		}
	}
	_, err := LoadRadioPlayExclusions(s.db)
	s.NoError(err, "every row the constraint accepted must load in Go")
}

func (s *RadioPlayExclusionsIntegrationSuite) TestManyRegexRowsStillMatch() {
	for i := 0; i < 40; i++ {
		s.Require().NoError(s.db.Create(&catalogm.RadioPlayExclusion{
			MatchKind: "artist_name", PatternType: "regex", Pattern: fmt.Sprintf("^zz%d segment$", i),
			Category: "talk", Reason: "test",
		}).Error)
	}
	s.addPlays("t-kexp", "ZZ7 Segment", "", "", 1)
	s.addPlays("t-kexp", "ZZ39 Segment", "", "", 1)
	s.addPlays("t-kexp", "ZZ7 Segments", "Song", "Label", 1)
	s.True(s.verdictFor("ZZ7 Segment").ExArtist)
	s.True(s.verdictFor("ZZ39 Segment").ExArtist)
	s.False(s.verdictFor("ZZ7 Segments").ExArtist)
	s.assertGoAgreesWithSQL()
}

func (s *RadioPlayExclusionsIntegrationSuite) TestSingleStationRuleCountsOnlyNonExcludedPlays() {
	// The sheena plays are excluded by a station-scoped rule, so they add no
	// station: the key spans t-kexp alone, then t-kexp and t-nts.
	sheena := s.stationBySlug["t-sheena"]
	s.Require().NoError(s.db.Create(&catalogm.RadioPlayExclusion{
		MatchKind: "artist_name", PatternType: "exact", Pattern: "dorothy ashby", Category: "talk",
		StationID: &sheena, Reason: "test",
	}).Error)
	s.addPlays("t-kexp", "Dorothy Ashby", "Soul Vibrations", "", 3)
	s.addPlays("t-sheena", "Dorothy Ashby", "Soul Vibrations", "", 3)
	s.NotContains(s.qualifyingTracks(), "dorothy ashby / soul vibrations")

	s.addPlays("t-nts", "Dorothy Ashby", "Soul Vibrations", "", 1)
	got := s.qualifyingTracks()
	s.Require().Contains(got, "dorothy ashby / soul vibrations")
	s.Equal(2, got["dorothy ashby / soul vibrations"].Stations, "the excluded sheena plays add no station")
	s.Equal(4, got["dorothy ashby / soul vibrations"].Plays)
}

// Exact patterns are stored in radio_normalize_name form; a normalizer change
// that moves any stored pattern off its output would silently disable it.
func (s *RadioPlayExclusionsIntegrationSuite) TestExactPatternsAreNormalizerOutput() {
	var drifted []string
	s.Require().NoError(s.db.Raw(`SELECT pattern FROM radio_play_exclusions
WHERE pattern_type = 'exact' AND pattern IS DISTINCT FROM radio_normalize_name(pattern)`).Scan(&drifted).Error)
	s.Empty(drifted)
}

// Station-scoped placeholder and regex rules follow the same dimension rules
// as global ones, and only on their station.
func (s *RadioPlayExclusionsIntegrationSuite) TestStationScopedPlaceholderAndRegexRules() {
	kexp := s.stationBySlug["t-kexp"]
	s.Require().NoError(s.db.Create(&catalogm.RadioPlayExclusion{
		MatchKind: "label_name", PatternType: "exact", Pattern: "house tapes", Category: "placeholder",
		StationID: &kexp, Reason: "test",
	}).Error)
	s.Require().NoError(s.db.Create(&catalogm.RadioPlayExclusion{
		MatchKind: "artist_name", PatternType: "regex", Pattern: "^kexp (live|session) ", Category: "talk",
		StationID: &kexp, Reason: "test",
	}).Error)
	s.addPlays("t-kexp", "Lael Neale", "Star", "House Tapes", 1)
	s.addPlays("t-nts", "Hand Habits", "Wildfire", "House Tapes", 1)
	s.addPlays("t-kexp", "KEXP Live Session Host", "Intro", "Sub Pop", 1)
	s.addPlays("t-nts", "KEXP Live Session Host", "Intro", "Sub Pop", 1)
	s.addPlays("t-kexp", "검정치마", "Everything", "Sub Pop", 1)

	for _, v := range s.verdicts() {
		onKEXP := v.StationID == kexp
		switch v.ArtistRaw {
		case "Lael Neale", "Hand Habits":
			s.Equal(onKEXP, v.ExLabel, "%q: the scoped placeholder label applies on its station only", v.ArtistRaw)
			s.False(v.ExArtist || v.ExTrack, "%q: a placeholder label leaves artist and track counting", v.ArtistRaw)
		case "KEXP Live Session Host":
			s.Equal(onKEXP, v.ExArtist && v.ExTrack && v.ExLabel, "scoped talk regex excludes the whole play on its station")
			if !onKEXP {
				s.False(v.ExArtist || v.ExTrack || v.ExLabel, "scoped talk regex does not apply on other stations")
			}
		case "검정치마":
			s.False(v.ExArtist || v.ExTrack || v.ExLabel, "an empty normalized value matches no scoped rule")
		}
	}
	s.assertGoAgreesWithSQL()
}

// Scoped rows are unique per station, and leave with their station.
func (s *RadioPlayExclusionsIntegrationSuite) TestScopedRowUniquenessAndCascade() {
	kexp := s.stationBySlug["t-kexp"]
	nts := s.stationBySlug["t-nts"]
	row := func(station uint) *catalogm.RadioPlayExclusion {
		return &catalogm.RadioPlayExclusion{
			MatchKind: "track_title", PatternType: "exact", Pattern: "announcements", Category: "talk",
			StationID: &station, Reason: "test",
		}
	}
	s.Require().NoError(s.db.Create(row(kexp)).Error)
	s.Error(s.db.Create(row(kexp)).Error, "the same scoped rule twice on one station is refused")
	s.Require().NoError(s.db.Create(row(nts)).Error, "the same rule on another station is a separate row")

	s.Require().NoError(s.db.Exec("DELETE FROM radio_plays").Error)
	s.Require().NoError(s.db.Exec("DELETE FROM radio_episodes").Error)
	s.Require().NoError(s.db.Exec("DELETE FROM radio_shows WHERE station_id = ?", kexp).Error)
	s.Require().NoError(s.db.Exec("DELETE FROM radio_stations WHERE id = ?", kexp).Error)
	var left int64
	s.Require().NoError(s.db.Model(&catalogm.RadioPlayExclusion{}).Where("station_id = ?", kexp).Count(&left).Error)
	s.Zero(left, "deleting a station deletes its scoped rules")
	var kept int64
	s.Require().NoError(s.db.Model(&catalogm.RadioPlayExclusion{}).Where("station_id = ?", nts).Count(&kept).Error)
	s.Equal(int64(1), kept)
}

// Station-scoped rules are probed through the station_id index, not by
// scanning the whole table for every play.
func (s *RadioPlayExclusionsIntegrationSuite) TestScopedRulesUseStationIndex() {
	sheena := s.stationBySlug["t-sheena"]
	s.Require().NoError(s.db.Create(&catalogm.RadioPlayExclusion{
		MatchKind: "track_title", PatternType: "exact", Pattern: "announcements", Category: "talk",
		StationID: &sheena, Reason: "test",
	}).Error)
	s.addPlays("t-kexp", "Some Artist", "Some Song", "Some Label", 200)
	// A tiny table makes a sequential scan look cheapest; the production
	// shape is many plays against a table of a few hundred rows.
	s.Require().NoError(s.db.Exec("ANALYZE radio_play_exclusions").Error)
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SET LOCAL enable_seqscan = off").Error; err != nil {
			return err
		}
		q := fmt.Sprintf(`EXPLAIN (FORMAT JSON) SELECT count(*) %s WHERE NOT (%s)`,
			radioExclusionTestFrom, RadioPlayExcludedSQL(RadioRankingTrack, radioExclusionTestCols))
		var planJSON string
		if err := tx.Raw(q).Row().Scan(&planJSON); err != nil {
			return err
		}
		s.Contains(planJSON, "idx_radio_play_exclusions_station", "the scoped probe can use the station index")
		return nil
	})
	s.Require().NoError(err)
}

// Global rules are read once per query, not once per play.
func (s *RadioPlayExclusionsIntegrationSuite) TestGlobalRulesAreReadOncePerQuery() {
	s.addPlays("t-kexp", "Some Artist", "Some Song", "Some Label", 200)
	s.addPlays("t-wfmu", "Music behind DJ: Some Artist", "Some Song", "Some Label", 50)
	for _, dim := range allRadioRankingDimensions {
		q := fmt.Sprintf(`EXPLAIN (ANALYZE, FORMAT JSON) SELECT count(*) %s WHERE NOT (%s)`,
			radioExclusionTestFrom, RadioPlayExcludedSQL(dim, radioExclusionTestCols))
		var planJSON string
		s.Require().NoError(s.db.Raw(q).Row().Scan(&planJSON))
		var plans []map[string]any
		s.Require().NoError(json.Unmarshal([]byte(planJSON), &plans))
		scans := 0
		var walk func(node map[string]any)
		walk = func(node map[string]any) {
			if node["Relation Name"] == "radio_play_exclusions" {
				scans++
				s.LessOrEqual(node["Actual Loops"].(float64), float64(1), "dimension %s: radio_play_exclusions scanned per play", dim)
			}
			if children, ok := node["Plans"].([]any); ok {
				for _, c := range children {
					walk(c.(map[string]any))
				}
			}
		}
		walk(plans[0]["Plan"].(map[string]any))
		s.Greater(scans, 0, "dimension %s: plan reads radio_play_exclusions", dim)
	}
}

func (s *RadioPlayExclusionsIntegrationSuite) TestConstraintsRejectInvalidRows() {
	bad := map[string]catalogm.RadioPlayExclusion{
		"exact not normalized":        {MatchKind: "artist_name", PatternType: "exact", Pattern: "Unknown Artist", Category: "placeholder", Reason: "r"},
		"exact with edge punctuation": {MatchKind: "label_name", PatternType: "exact", Pattern: "[no label]", Category: "placeholder", Reason: "r"},
		"invalid regex":               {MatchKind: "artist_name", PatternType: "regex", Pattern: "(unclosed", Category: "talk", Reason: "r"},
		"word boundary":               {MatchKind: "artist_name", PatternType: "regex", Pattern: `\bdj\b`, Category: "talk", Reason: "r"},
		"postgres word start":         {MatchKind: "artist_name", PatternType: "regex", Pattern: `\mdj`, Category: "talk", Reason: "r"},
		"posix class":                 {MatchKind: "artist_name", PatternType: "regex", Pattern: `^[[:alpha:]]+$`, Category: "talk", Reason: "r"},
		"inline flags":                {MatchKind: "artist_name", PatternType: "regex", Pattern: `(?i)dj`, Category: "talk", Reason: "r"},
		"unknown kind":                {MatchKind: "show_name", PatternType: "exact", Pattern: "x", Category: "talk", Reason: "r"},
		"unknown category":            {MatchKind: "artist_name", PatternType: "exact", Pattern: "x", Category: "spam", Reason: "r"},
		"blank reason":                {MatchKind: "artist_name", PatternType: "exact", Pattern: "x", Category: "talk", Reason: "  "},
		"duplicate seeded rule":       {MatchKind: "artist_name", PatternType: "exact", Pattern: "unknown artist", Category: "placeholder", Reason: "dup"},
	}
	for name, row := range bad {
		err := s.db.Create(&row).Error
		s.Error(err, name)
	}
	// An exact pattern that is not a valid regex is fine: it never reaches ~.
	ok := catalogm.RadioPlayExclusion{MatchKind: "label_name", PatternType: "exact", Pattern: "no label (boot", Category: "placeholder", Reason: "r"}
	s.NoError(s.db.Create(&ok).Error)
}

// The seeded album_title rows match the album as logged and disqualify only
// the album: the play still counts toward the artist, track and label
// rankings. Real album titles that contain a seeded name are kept.
func (s *RadioPlayExclusionsIntegrationSuite) TestSeededAlbumPlaceholdersExcludeOnlyTheAlbum() {
	placeholders := map[string]string{
		"T Single":   "Single",
		"T ST":       "S/T",
		"T KEXP":     "Live on KEXP",
		"T WFMU On":  "Live On WFMU",
		"T WFMU At":  " Live at WFMU ",
		"T Single 2": "[Single]",
	}
	kept := map[string]string{
		"T Volume":   "Live at KEXP, Volume Three",
		"T Rider":    "Single Rider",
		"T Trans":    "Live on KEXP from Trans Musicales",
		"T Ordinary": "Blue",
	}
	for artist, album := range placeholders {
		s.addAlbumPlay("t-kexp", artist, "Song", album, "Sub Pop")
	}
	for artist, album := range kept {
		s.addAlbumPlay("t-wfmu", artist, "Song", album, "Sub Pop")
	}
	for _, v := range s.verdicts() {
		_, isPlaceholder := placeholders[v.ArtistRaw]
		s.Equal(isPlaceholder, v.ExAlbum, "album of %q", v.ArtistRaw)
		s.False(v.ExArtist || v.ExTrack || v.ExLabel, "%q still counts toward the artist, track and label rankings", v.ArtistRaw)
	}
	s.assertGoAgreesWithSQL()
}

// A rule tests only the field its match kind names.
func (s *RadioPlayExclusionsIntegrationSuite) TestAlbumAndTitleRulesStayOnTheirField() {
	s.addAlbumPlay("t-kexp", "T Title Single", "Single", "Sub Pop Singles Club", "Sub Pop")
	s.addAlbumPlay("t-kexp", "T Album Unknown Track", "Rezzett", "Unknown Track", "Sub Pop")
	s.addAlbumPlay("t-kexp", "T Title Unknown Track", "Unknown Track", "Rezzett", "Sub Pop")

	titleSingle := s.verdictFor("T Title Single")
	s.False(titleSingle.ExTrack, "the album rule 'single' does not test the title")
	s.False(titleSingle.ExAlbum)
	albumUnknown := s.verdictFor("T Album Unknown Track")
	s.False(albumUnknown.ExAlbum, "the title rule 'unknown track' does not test the album")
	s.False(albumUnknown.ExTrack)
	titleUnknown := s.verdictFor("T Title Unknown Track")
	s.True(titleUnknown.ExTrack)
	s.False(titleUnknown.ExAlbum, "a placeholder title leaves the album counting")
	s.assertGoAgreesWithSQL()
}

// Station-scoped album rows follow the same dimension rules, on their station
// only.
func (s *RadioPlayExclusionsIntegrationSuite) TestStationScopedAlbumRule() {
	kexp := s.stationBySlug["t-kexp"]
	s.Require().NoError(s.db.Create(&catalogm.RadioPlayExclusion{
		MatchKind: "album_title", PatternType: "exact", Pattern: "kexp sessions", Category: "placeholder",
		StationID: &kexp, Reason: "test",
	}).Error)
	s.addAlbumPlay("t-kexp", "T On KEXP", "Song", "KEXP Sessions", "Sub Pop")
	s.addAlbumPlay("t-nts", "T On NTS", "Song", "KEXP Sessions", "Sub Pop")

	onKEXP := s.verdictFor("T On KEXP")
	s.True(onKEXP.ExAlbum)
	s.False(onKEXP.ExArtist || onKEXP.ExTrack || onKEXP.ExLabel)
	onNTS := s.verdictFor("T On NTS")
	s.False(onNTS.ExAlbum || onNTS.ExArtist || onNTS.ExTrack || onNTS.ExLabel)
	s.assertGoAgreesWithSQL()
}

// A station element or talk row on a field outside a dimension's key excludes
// the whole play from that dimension too; the seed holds no such album or
// label row, so this covers the predicate once one is added.
func (s *RadioPlayExclusionsIntegrationSuite) TestTalkRowsOnAlbumAndLabelExcludeWholePlay() {
	s.addAlbumPlay("t-kexp", "T Ident Album", "Top of the Hour", "Station ID 2026", "Sub Pop")
	s.addAlbumPlay("t-kexp", "T Plain", "Top of the Hour", "Real Album", "Sub Pop")
	before := s.verdictFor("T Ident Album")
	s.False(before.ExArtist || before.ExTrack || before.ExLabel || before.ExAlbum)

	s.Require().NoError(s.db.Create(&catalogm.RadioPlayExclusion{
		MatchKind: "album_title", PatternType: "regex", Pattern: "^station ids? ", Category: "station_element", Reason: "test",
	}).Error)
	s.Require().NoError(s.db.Create(&catalogm.RadioPlayExclusion{
		MatchKind: "label_name", PatternType: "exact", Pattern: "kexp promo", Category: "talk", Reason: "test",
	}).Error)
	s.addAlbumPlay("t-nts", "T Promo Label", "Spot", "Real Album", "KEXP Promo")

	for _, artist := range []string{"T Ident Album", "T Promo Label"} {
		v := s.verdictFor(artist)
		s.True(v.ExArtist && v.ExTrack && v.ExLabel && v.ExAlbum, "%q is excluded from every dimension", artist)
	}
	plain := s.verdictFor("T Plain")
	s.False(plain.ExArtist || plain.ExTrack || plain.ExLabel || plain.ExAlbum)
	s.assertGoAgreesWithSQL()
}
