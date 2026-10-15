package catalog

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// =============================================================================
// UNIT TESTS (No Database Required)
// =============================================================================

func exclusionRow(kind, patternType, pattern, category string) catalogm.RadioPlayExclusion {
	return catalogm.RadioPlayExclusion{
		MatchKind:   kind,
		PatternType: patternType,
		Pattern:     pattern,
		Category:    category,
		Reason:      "test",
	}
}

func mustExclusions(t *testing.T, rows ...catalogm.RadioPlayExclusion) *RadioPlayExclusions {
	t.Helper()
	e, err := NewRadioPlayExclusions(rows)
	require.NoError(t, err)
	return e
}

// normPlay builds a RadioNormalizedPlay from already-normalized values; an
// empty argument stands for a NULL or blank raw value.
func normPlay(station uint, artist, title, label string) RadioNormalizedPlay {
	opt := func(v string) *string {
		if v == "" {
			return nil
		}
		return &v
	}
	return RadioNormalizedPlay{StationID: station, ArtistName: opt(artist), TrackTitle: opt(title), LabelName: opt(label)}
}

// withAlbum returns the play with its normalized album title set; an empty
// argument stands for a NULL or blank raw value.
func withAlbum(p RadioNormalizedPlay, album string) RadioNormalizedPlay {
	if album == "" {
		p.AlbumTitle = nil
	} else {
		p.AlbumTitle = &album
	}
	return p
}

var allRadioRankingDimensions = []RadioRankingDimension{RadioRankingArtist, RadioRankingTrack, RadioRankingLabel, RadioRankingAlbum}

// radioExclusionRegexCorpus is checked against both the Go loader (here) and
// the table's CHECK constraint (integration suite); the two must agree on
// every entry.
var radioExclusionRegexCorpus = []struct {
	pattern string
	accept  bool
}{
	{`^music behind dj(:|$)`, true},
	{`^.+ speaks$`, true},
	{`^self[- ]*releas`, true},
	{`^not on (a )?label`, true},
	{`^a\.b\(c\)\[d\]\*\+\?\|\^\$\\\/\-$`, true},
	{`^dj[^a-z]+$`, true},
	{`(unclosed`, false},
	{`\bdj\b`, false},
	{`^\d+$`, false},
	{`\mdj`, false},
	{`(a)\1`, false},
	{`\€uro`, false},
	{`vampires\’ sound`, false},
	{`^[[:alpha:]]+$`, false},
	{`^[ [:alpha:]]+$`, false},
	{`^[[=a=]]x`, false},
	{`^[[.-.]]$`, false},
	{`^a[.]b`, false},
	{`(?i)dj`, false},
	{`dj(?=x)`, false},
	{`***=n/a`, false},
	{`*dj`, false},
	{`(a{100}){100}`, false},
	{`a{2}`, false},
	{`^Music behind DJ`, false},
	{`^$`, false},
	{`|`, false},
	{`^your dj|`, false},
	{`x*`, false},
	{`(a|)`, false},
	{`^`, false},
	{strings.Repeat("a", RadioExclusionRegexMaxLen), true},
	{strings.Repeat("a", RadioExclusionRegexMaxLen+1), false},
}

func TestRadioExclusionRegexCorpus_GoLoader(t *testing.T) {
	for _, c := range radioExclusionRegexCorpus {
		_, err := NewRadioPlayExclusions([]catalogm.RadioPlayExclusion{exclusionRow("artist_name", "regex", c.pattern, "talk")})
		if c.accept {
			assert.NoError(t, err, c.pattern)
		} else {
			assert.Error(t, err, c.pattern)
		}
	}
}

func TestNewRadioPlayExclusions_RejectsInvalidRows(t *testing.T) {
	cases := map[string]catalogm.RadioPlayExclusion{
		"unknown match kind":   exclusionRow("show_name", "exact", "x", "talk"),
		"unknown pattern type": exclusionRow("artist_name", "glob", "x*", "talk"),
		"unknown category":     exclusionRow("artist_name", "exact", "x", "spam"),
		"empty pattern":        exclusionRow("artist_name", "exact", "", "talk"),
	}
	for name, row := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := NewRadioPlayExclusions([]catalogm.RadioPlayExclusion{row})
			assert.Error(t, err)
		})
	}
}

func TestRadioPlayExclusions_TalkExcludesWholePlay(t *testing.T) {
	e := mustExclusions(t, exclusionRow("artist_name", "regex", `^music behind dj(:|$)`, "talk"))
	bed := withAlbum(normPlay(1, "music behind dj: dorothy ashby", "soul vibrations", "cadet"), "afro-harping")
	plain := withAlbum(normPlay(1, "dorothy ashby", "soul vibrations", "cadet"), "afro-harping")
	for _, dim := range allRadioRankingDimensions {
		assert.True(t, e.Excludes(dim, bed), dim)
		assert.False(t, e.Excludes(dim, plain), dim)
	}
}

func TestRadioPlayExclusions_PlaceholderOnlyDisqualifiesKeyField(t *testing.T) {
	e := mustExclusions(t,
		exclusionRow("label_name", "regex", `^self[- ]*releas`, "placeholder"),
		exclusionRow("artist_name", "exact", "unknown artist", "placeholder"),
		exclusionRow("track_title", "exact", "unknown track", "placeholder"),
	)

	selfReleased := normPlay(1, "lael neale", "star", "self-released")
	assert.True(t, e.Excludes(RadioRankingLabel, selfReleased))
	assert.False(t, e.Excludes(RadioRankingArtist, selfReleased))
	assert.False(t, e.Excludes(RadioRankingTrack, selfReleased))

	unknownArtist := normPlay(1, "unknown artist", "green island", "sub pop")
	assert.True(t, e.Excludes(RadioRankingArtist, unknownArtist))
	assert.True(t, e.Excludes(RadioRankingTrack, unknownArtist))
	assert.False(t, e.Excludes(RadioRankingLabel, unknownArtist))

	unknownTrack := normPlay(1, "rezzett", "unknown track", "trilogy tapes")
	assert.False(t, e.Excludes(RadioRankingArtist, unknownTrack))
	assert.True(t, e.Excludes(RadioRankingTrack, unknownTrack))
	assert.False(t, e.Excludes(RadioRankingLabel, unknownTrack))
}

func TestRadioPlayExclusions_BlankKeyFieldExcludesOnlyThatDimension(t *testing.T) {
	e := mustExclusions(t)
	noLabel := normPlay(1, "lael neale", "star", "")
	assert.True(t, e.Excludes(RadioRankingLabel, noLabel))
	assert.False(t, e.Excludes(RadioRankingArtist, noLabel))
	assert.False(t, e.Excludes(RadioRankingTrack, noLabel))

	noTitle := normPlay(1, "lael neale", "", "sub pop")
	assert.True(t, e.Excludes(RadioRankingTrack, noTitle))
	assert.False(t, e.Excludes(RadioRankingArtist, noTitle))
	assert.False(t, e.Excludes(RadioRankingLabel, noTitle))
}

func TestRadioPlayExclusions_EmptyNormalizedValueNeverMatches(t *testing.T) {
	// "" is a present raw value that radio_normalize_name trims to nothing (a
	// name of non-ASCII characters only). It is neither blank nor matched, even
	// by a rule that matches the empty string. The loader refuses such rules, so
	// they are built directly here.
	e := &RadioPlayExclusions{rules: []radioPlayExclusionRule{
		{matchKind: "artist_name", category: "talk", re: regexp.MustCompile(`^$`)},
		{matchKind: "track_title", category: "talk", re: regexp.MustCompile(`^.*$`)},
	}}
	empty := ""
	play := RadioNormalizedPlay{StationID: 1, ArtistName: &empty, TrackTitle: &empty, LabelName: &empty, AlbumTitle: &empty}
	for _, dim := range allRadioRankingDimensions {
		assert.False(t, e.Excludes(dim, play), dim)
	}
}

func TestRadioPlayExclusions_StationScope(t *testing.T) {
	sheena := uint(11)
	row := exclusionRow("track_title", "exact", "announcements", "talk")
	row.StationID = &sheena
	e := mustExclusions(t, row)

	assert.True(t, e.Excludes(RadioRankingTrack, normPlay(11, "bob barth", "announcements", "")))
	assert.False(t, e.Excludes(RadioRankingTrack, normPlay(7, "someone", "announcements", "")))
}

func TestRadioPlayExclusions_ExactIsWholeValue(t *testing.T) {
	e := mustExclusions(t, exclusionRow("artist_name", "exact", "unknown", "placeholder"))
	assert.True(t, e.Excludes(RadioRankingArtist, normPlay(1, "unknown", "", "")))
	assert.False(t, e.Excludes(RadioRankingArtist, normPlay(1, "unknown mortal orchestra", "", "")))
}

func TestRadioPlayExclusions_UnknownDimensionPanics(t *testing.T) {
	e := mustExclusions(t)
	assert.Panics(t, func() { e.Excludes(RadioRankingDimension("release"), RadioNormalizedPlay{}) })
	cols := RadioPlayColumns{StationID: "rsh.station_id", ArtistName: "rp.artist_name", TrackTitle: "rp.track_title", LabelName: "rp.label_name", AlbumTitle: "rp.album_title"}
	assert.Panics(t, func() { RadioPlayExcludedSQL(RadioRankingDimension("release"), cols) })
}

func TestRadioPlayExcludedSQL_PanicsOnNonIdentifier(t *testing.T) {
	cols := RadioPlayColumns{StationID: "rsh.station_id", ArtistName: "rp.artist_name); DROP TABLE x; --", TrackTitle: "rp.track_title", LabelName: "rp.label_name", AlbumTitle: "rp.album_title"}
	assert.Panics(t, func() { RadioPlayExcludedSQL(RadioRankingArtist, cols) })
	cols.ArtistName = "rp.artist_name"
	assert.NotPanics(t, func() { RadioPlayExcludedSQL(RadioRankingArtist, cols) })
	cols.AlbumTitle = ""
	assert.Panics(t, func() { RadioPlayExcludedSQL(RadioRankingArtist, cols) }, "every dimension reads the album column")
	assert.Panics(t, func() { RadioBroadcasterFloorSQL("rst.id") })
	assert.Panics(t, func() { RadioBroadcasterKeySQL("rst; DROP TABLE x") })
	assert.Panics(t, func() { RadioBroadcasterFloorSQL("1; DROP TABLE x") })
	assert.Panics(t, func() { RadioStationFloorSQL("1; DROP TABLE x") })
}

func TestRadioMeetsStationFloor(t *testing.T) {
	assert.False(t, RadioMeetsStationFloor(0))
	assert.False(t, RadioMeetsStationFloor(1))
	assert.True(t, RadioMeetsStationFloor(2))
	assert.True(t, RadioMeetsStationFloor(5))
}

func TestRadioMeetsBroadcasterFloor(t *testing.T) {
	assert.False(t, RadioMeetsBroadcasterFloor(0))
	assert.False(t, RadioMeetsBroadcasterFloor(1))
	assert.True(t, RadioMeetsBroadcasterFloor(2))
	assert.True(t, RadioMeetsBroadcasterFloor(5))
}

func TestRadioPlayExclusions_PlaceholderAlbumDisqualifiesOnlyTheAlbum(t *testing.T) {
	e := mustExclusions(t, exclusionRow("album_title", "exact", "single", "placeholder"))

	single := withAlbum(normPlay(1, "lael neale", "star", "sub pop"), "single")
	assert.True(t, e.Excludes(RadioRankingAlbum, single))
	assert.False(t, e.Excludes(RadioRankingArtist, single))
	assert.False(t, e.Excludes(RadioRankingTrack, single))
	assert.False(t, e.Excludes(RadioRankingLabel, single))

	album := withAlbum(normPlay(1, "lael neale", "star", "sub pop"), "star dust")
	assert.False(t, e.Excludes(RadioRankingAlbum, album))
	assert.False(t, e.Excludes(RadioRankingAlbum, withAlbum(album, "single rider")), "an exact pattern matches the whole album only")
}

func TestRadioPlayExclusions_BlankAlbumExcludesOnlyAlbumEvidence(t *testing.T) {
	e := mustExclusions(t)
	noAlbum := normPlay(1, "lael neale", "star", "sub pop")
	assert.True(t, e.Excludes(RadioRankingAlbum, noAlbum))
	assert.False(t, e.Excludes(RadioRankingArtist, noAlbum))
	assert.False(t, e.Excludes(RadioRankingTrack, noAlbum))
	assert.False(t, e.Excludes(RadioRankingLabel, noAlbum))
}

// A rule tests only the field its match kind names: a title equal to an
// excluded album name, or an album equal to an excluded title, is not matched.
func TestRadioPlayExclusions_AlbumAndTitleRulesStayOnTheirField(t *testing.T) {
	e := mustExclusions(t,
		exclusionRow("album_title", "exact", "single", "placeholder"),
		exclusionRow("track_title", "exact", "unknown track", "placeholder"),
	)

	titledSingle := withAlbum(normPlay(1, "lael neale", "single", "sub pop"), "acquainted with night")
	assert.False(t, e.Excludes(RadioRankingTrack, titledSingle), "an album rule does not test the title")
	assert.False(t, e.Excludes(RadioRankingAlbum, titledSingle))

	albumUnknownTrack := withAlbum(normPlay(1, "rezzett", "rezzett", "trilogy tapes"), "unknown track")
	assert.False(t, e.Excludes(RadioRankingAlbum, albumUnknownTrack), "a title rule does not test the album")
	assert.False(t, e.Excludes(RadioRankingTrack, albumUnknownTrack))

	unknownTrack := withAlbum(normPlay(1, "rezzett", "unknown track", "trilogy tapes"), "rezzett")
	assert.True(t, e.Excludes(RadioRankingTrack, unknownTrack))
	assert.False(t, e.Excludes(RadioRankingAlbum, unknownTrack), "a placeholder title leaves the album counting")
}

func TestRadioPlayExclusions_TalkAlbumExcludesWholePlay(t *testing.T) {
	e := mustExclusions(t, exclusionRow("album_title", "regex", `^station ids? `, "station_element"))
	ident := withAlbum(normPlay(1, "kexp", "top of the hour", "kexp"), "station id 2026")
	for _, dim := range allRadioRankingDimensions {
		assert.True(t, e.Excludes(dim, ident), dim)
	}
}
