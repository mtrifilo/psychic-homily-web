package catalog

import (
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
	for _, slug := range []string{"t-kexp", "t-wfmu", "t-rocknsoul", "t-sheena", "t-nts"} {
		station := &catalogm.RadioStation{Name: slug, Slug: slug, BroadcastType: catalogm.BroadcastTypeInternet}
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
}

// addPlays inserts n copies of a play on the station; title and label are nil
// when empty.
func (s *RadioPlayExclusionsIntegrationSuite) addPlays(station, artist, title, label string, n int) {
	for i := 0; i < n; i++ {
		play := &catalogm.RadioPlay{
			EpisodeID:  s.episodeBySlug[station],
			Position:   i,
			ArtistName: artist,
		}
		if title != "" {
			play.TrackTitle = strPtr(title)
		}
		if label != "" {
			play.LabelName = strPtr(label)
		}
		// ProviderPlayID keeps repeated identical plays distinct under the
		// (episode_id, dedup_key) unique index.
		play.ProviderPlayID = strPtr(fmt.Sprintf("%s|%s|%s|%d", station, artist, title, i))
		s.Require().NoError(s.db.Create(play).Error)
	}
}

type radioPlayVerdict struct {
	PlayID     uint
	StationID  uint
	ArtistRaw  string
	ArtistNorm string
	TitleNorm  string
	LabelNorm  string
	ExArtist   bool
	ExTrack    bool
	ExLabel    bool
}

// verdicts evaluates RadioPlayExcludedSQL for every fixture play and every
// dimension, together with the SQL-normalized fields the Go predicate takes.
func (s *RadioPlayExclusionsIntegrationSuite) verdicts() []radioPlayVerdict {
	q := fmt.Sprintf(`SELECT rp.id AS play_id, rsh.station_id AS station_id, rp.artist_name AS artist_raw,
       coalesce(radio_normalize_name(rp.artist_name), '') AS artist_norm,
       coalesce(radio_normalize_name(rp.track_title), '') AS title_norm,
       coalesce(radio_normalize_name(rp.label_name), '') AS label_norm,
       %s AS ex_artist, %s AS ex_track, %s AS ex_label %s ORDER BY rp.id`,
		RadioPlayExcludedSQL(RadioRankingArtist, radioExclusionTestCols),
		RadioPlayExcludedSQL(RadioRankingTrack, radioExclusionTestCols),
		RadioPlayExcludedSQL(RadioRankingLabel, radioExclusionTestCols),
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
		play := RadioNormalizedPlay{StationID: v.StationID, ArtistName: v.ArtistNorm, TrackTitle: v.TitleNorm, LabelName: v.LabelNorm}
		s.Equal(v.ExArtist, excl.Excludes(RadioRankingArtist, play), "artist dimension, play %q", v.ArtistRaw)
		s.Equal(v.ExTrack, excl.Excludes(RadioRankingTrack, play), "track dimension, play %q", v.ArtistRaw)
		s.Equal(v.ExLabel, excl.Excludes(RadioRankingLabel, play), "label dimension, play %q", v.ArtistRaw)
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

// The most-played non-music keys of the stage classification, as logged.
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
		s.addPlays(n.station, n.artist, n.title, "", 1)
	}
	for _, v := range s.verdicts() {
		s.True(v.ExArtist && v.ExTrack && v.ExLabel, "noise play %q must be excluded from every ranking", v.ArtistRaw)
	}
	s.assertGoAgreesWithSQL()
}

func (s *RadioPlayExclusionsIntegrationSuite) TestKnownMultiStationSongsAreKept() {
	s.addPlays("t-kexp", "Man/Woman/Chainsaw", "Nosedive", "Partisan Records", 3)
	s.addPlays("t-wfmu", "Man/Woman/Chainsaw", "Nosedive", "", 2)
	s.addPlays("t-sheena", "Man/Woman/Chainsaw", "Nosedive", "", 1)
	// The plain twin of a "Music behind DJ:" bed still counts.
	s.addPlays("t-kexp", "Dorothy Ashby", "Soul Vibrations", "Cadet", 1)
	s.addPlays("t-nts", "Dorothy Ashby", "Soul Vibrations", "", 1)
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
	// An empty normalized value must never match, even for a rule written to
	// match the empty string.
	s.Require().NoError(s.db.Create(&catalogm.RadioPlayExclusion{
		MatchKind: "artist_name", PatternType: "regex", Pattern: "^$", Category: "talk", Reason: "test",
	}).Error)
	s.addPlays("t-kexp", "검정치마", "everything", "", 1)
	s.addPlays("t-nts", "ドレスコーズ", "ビューティフル", "日本コロムビア", 1)
	for _, v := range s.verdicts() {
		s.Equal("", v.ArtistNorm, "fixture premise: %q normalizes to empty in SQL", v.ArtistRaw)
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
	s.addPlays("t-sheena", "Bob Barth", "Announcements", "", 1)
	s.addPlays("t-kexp", "The Announcers", "Announcements", "", 1)
	s.True(s.verdictFor("Bob Barth").ExTrack)
	s.False(s.verdictFor("The Announcers").ExTrack)
	s.assertGoAgreesWithSQL()
}

type radioTrackFloorRow struct {
	Artist   string
	Title    string
	Plays    int
	Stations int
}

// qualifyingTracks groups non-excluded plays by normalized artist and title
// and keeps the keys that pass the single-station rule.
func (s *RadioPlayExclusionsIntegrationSuite) qualifyingTracks() map[string]radioTrackFloorRow {
	q := fmt.Sprintf(`SELECT radio_normalize_name(rp.artist_name) AS artist, radio_normalize_name(rp.track_title) AS title,
       count(*) AS plays, count(DISTINCT rsh.station_id) AS stations
%s
WHERE NOT (%s)
GROUP BY 1, 2
HAVING %s`, radioExclusionTestFrom, RadioPlayExcludedSQL(RadioRankingTrack, radioExclusionTestCols), RadioStationFloorSQL("rsh.station_id"))
	var rows []radioTrackFloorRow
	s.Require().NoError(s.db.Raw(q).Scan(&rows).Error)
	out := map[string]radioTrackFloorRow{}
	for _, r := range rows {
		s.True(RadioMeetsStationFloor(r.Stations), "SQL and Go floor agree for %s", r.Artist)
		out[r.Artist+" / "+r.Title] = r
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

func (s *RadioPlayExclusionsIntegrationSuite) TestSingleStationRuleCountsOnlyNonExcludedPlays() {
	// The sheena plays are excluded by a station-scoped rule, so the key's
	// non-excluded plays span one station and it does not qualify. The
	// prefixed bed is a separate, excluded key and adds no station to it.
	sheena := s.stationBySlug["t-sheena"]
	s.Require().NoError(s.db.Create(&catalogm.RadioPlayExclusion{
		MatchKind: "artist_name", PatternType: "exact", Pattern: "dorothy ashby", Category: "talk",
		StationID: &sheena, Reason: "test",
	}).Error)
	s.addPlays("t-kexp", "Dorothy Ashby", "Soul Vibrations", "", 3)
	s.addPlays("t-sheena", "Dorothy Ashby", "Soul Vibrations", "", 3)
	s.addPlays("t-rocknsoul", "Music behind DJ: Dorothy Ashby", "Soul Vibrations", "", 50)

	got := s.qualifyingTracks()
	s.NotContains(got, "dorothy ashby / soul vibrations")
	s.NotContains(got, "music behind dj: dorothy ashby / soul vibrations")
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
		"unknown kind":                {MatchKind: "album_title", PatternType: "exact", Pattern: "x", Category: "talk", Reason: "r"},
		"unknown category":            {MatchKind: "artist_name", PatternType: "exact", Pattern: "x", Category: "spam", Reason: "r"},
		"blank reason":                {MatchKind: "artist_name", PatternType: "exact", Pattern: "x", Category: "talk", Reason: "  "},
		"duplicate seeded rule":       {MatchKind: "artist_name", PatternType: "exact", Pattern: "unknown artist", Category: "placeholder", Reason: "dup"},
	}
	for name, row := range bad {
		row := row
		err := s.db.Create(&row).Error
		s.Error(err, name)
	}
	// An exact pattern that is not a valid regex is fine: it never reaches ~.
	ok := catalogm.RadioPlayExclusion{MatchKind: "label_name", PatternType: "exact", Pattern: "no label (boot", Category: "placeholder", Reason: "r"}
	s.NoError(s.db.Create(&ok).Error)
}
