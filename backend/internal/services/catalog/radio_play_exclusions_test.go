package catalog

import (
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

var allRadioRankingDimensions = []RadioRankingDimension{RadioRankingArtist, RadioRankingTrack, RadioRankingLabel}

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
		"unknown match kind":   exclusionRow("album_title", "exact", "x", "talk"),
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
	bed := normPlay(1, "music behind dj: dorothy ashby", "soul vibrations", "cadet")
	plain := normPlay(1, "dorothy ashby", "soul vibrations", "cadet")
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
	// by a rule that matches the empty string.
	e := mustExclusions(t,
		exclusionRow("artist_name", "regex", `^$`, "talk"),
		exclusionRow("track_title", "regex", `^.*$`, "talk"),
	)
	empty := ""
	play := RadioNormalizedPlay{StationID: 1, ArtistName: &empty, TrackTitle: &empty, LabelName: &empty}
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
	cols := RadioPlayColumns{StationID: "rsh.station_id", ArtistName: "rp.artist_name", TrackTitle: "rp.track_title", LabelName: "rp.label_name"}
	assert.Panics(t, func() { RadioPlayExcludedSQL(RadioRankingDimension("release"), cols) })
}

func TestRadioPlayExcludedSQL_PanicsOnNonIdentifier(t *testing.T) {
	cols := RadioPlayColumns{StationID: "rsh.station_id", ArtistName: "rp.artist_name); DROP TABLE x; --", TrackTitle: "rp.track_title", LabelName: "rp.label_name"}
	assert.Panics(t, func() { RadioPlayExcludedSQL(RadioRankingArtist, cols) })
	assert.Panics(t, func() { RadioBroadcasterFloorSQL("rst.id") })
	assert.Panics(t, func() { RadioBroadcasterFloorSQL("1; DROP TABLE x") })
}

func TestRadioMeetsBroadcasterFloor(t *testing.T) {
	assert.False(t, RadioMeetsBroadcasterFloor(0))
	assert.False(t, RadioMeetsBroadcasterFloor(1))
	assert.True(t, RadioMeetsBroadcasterFloor(2))
	assert.True(t, RadioMeetsBroadcasterFloor(5))
}
