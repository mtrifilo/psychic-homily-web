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

func TestNewRadioPlayExclusions_RejectsInvalidRows(t *testing.T) {
	cases := map[string]catalogm.RadioPlayExclusion{
		"unknown match kind":   exclusionRow("album_title", "exact", "x", "talk"),
		"unknown pattern type": exclusionRow("artist_name", "glob", "x*", "talk"),
		"unknown category":     exclusionRow("artist_name", "exact", "x", "spam"),
		"empty pattern":        exclusionRow("artist_name", "exact", "", "talk"),
		"invalid RE2":          exclusionRow("artist_name", "regex", "(unclosed", "talk"),
		// Each of these compiles in at least one engine but is read
		// differently by the other, so the loader refuses it.
		"word boundary":       exclusionRow("artist_name", "regex", `\bdj\b`, "talk"),
		"digit class escape":  exclusionRow("artist_name", "regex", `^\d+$`, "talk"),
		"postgres word start": exclusionRow("artist_name", "regex", `\mdj`, "talk"),
		"back-reference":      exclusionRow("artist_name", "regex", `(a)\1`, "talk"),
		"posix class":         exclusionRow("artist_name", "regex", `^[[:alpha:]]+$`, "talk"),
		"inline flags":        exclusionRow("artist_name", "regex", `(?i)dj`, "talk"),
		"lookahead":           exclusionRow("artist_name", "regex", `dj(?=x)`, "talk"),
	}
	for name, row := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := NewRadioPlayExclusions([]catalogm.RadioPlayExclusion{row})
			assert.Error(t, err)
		})
	}
}

func TestNewRadioPlayExclusions_AcceptsPortableRegex(t *testing.T) {
	for _, pattern := range []string{`^music behind dj(:|$)`, `^.+ speaks$`, `^self[- ]*releas`, `^not on (a )?label`, `^a\.b\(c\)$`} {
		_, err := NewRadioPlayExclusions([]catalogm.RadioPlayExclusion{exclusionRow("artist_name", "regex", pattern, "talk")})
		assert.NoError(t, err, pattern)
	}
}

func TestRadioPlayExclusions_TalkExcludesWholePlay(t *testing.T) {
	e := mustExclusions(t, exclusionRow("artist_name", "regex", `^music behind dj(:|$)`, "talk"))
	play := RadioNormalizedPlay{StationID: 1, ArtistName: "music behind dj: dorothy ashby", TrackTitle: "soul vibrations", LabelName: "cadet"}
	for _, dim := range []RadioRankingDimension{RadioRankingArtist, RadioRankingTrack, RadioRankingLabel} {
		assert.True(t, e.Excludes(dim, play), dim)
	}

	plain := play
	plain.ArtistName = "dorothy ashby"
	for _, dim := range []RadioRankingDimension{RadioRankingArtist, RadioRankingTrack, RadioRankingLabel} {
		assert.False(t, e.Excludes(dim, plain), dim)
	}
}

func TestRadioPlayExclusions_PlaceholderOnlyDisqualifiesKeyField(t *testing.T) {
	e := mustExclusions(t,
		exclusionRow("label_name", "regex", `^self[- ]*releas`, "placeholder"),
		exclusionRow("artist_name", "exact", "unknown artist", "placeholder"),
		exclusionRow("track_title", "exact", "unknown track", "placeholder"),
	)

	selfReleased := RadioNormalizedPlay{StationID: 1, ArtistName: "lael neale", TrackTitle: "star", LabelName: "self-released"}
	assert.True(t, e.Excludes(RadioRankingLabel, selfReleased))
	assert.False(t, e.Excludes(RadioRankingArtist, selfReleased))
	assert.False(t, e.Excludes(RadioRankingTrack, selfReleased))

	unknownArtist := RadioNormalizedPlay{StationID: 1, ArtistName: "unknown artist", TrackTitle: "green island", LabelName: "sub pop"}
	assert.True(t, e.Excludes(RadioRankingArtist, unknownArtist))
	assert.True(t, e.Excludes(RadioRankingTrack, unknownArtist))
	assert.False(t, e.Excludes(RadioRankingLabel, unknownArtist))

	unknownTrack := RadioNormalizedPlay{StationID: 1, ArtistName: "rezzett", TrackTitle: "unknown track"}
	assert.False(t, e.Excludes(RadioRankingArtist, unknownTrack))
	assert.True(t, e.Excludes(RadioRankingTrack, unknownTrack))
	assert.False(t, e.Excludes(RadioRankingLabel, unknownTrack))
}

func TestRadioPlayExclusions_EmptyFieldNeverMatches(t *testing.T) {
	// A regex that matches the empty string must still not exclude a play
	// whose normalized field is empty (blank raw value, or a non-Latin name
	// that radio_normalize_name trims to nothing).
	e := mustExclusions(t,
		exclusionRow("artist_name", "regex", `^$`, "talk"),
		exclusionRow("track_title", "regex", `^.*$`, "talk"),
	)
	play := RadioNormalizedPlay{StationID: 1}
	for _, dim := range []RadioRankingDimension{RadioRankingArtist, RadioRankingTrack, RadioRankingLabel} {
		assert.False(t, e.Excludes(dim, play), dim)
	}
}

func TestRadioPlayExclusions_StationScope(t *testing.T) {
	sheena := uint(11)
	row := exclusionRow("track_title", "exact", "announcements", "talk")
	row.StationID = &sheena
	e := mustExclusions(t, row)

	assert.True(t, e.Excludes(RadioRankingTrack, RadioNormalizedPlay{StationID: 11, ArtistName: "bob barth", TrackTitle: "announcements"}))
	assert.False(t, e.Excludes(RadioRankingTrack, RadioNormalizedPlay{StationID: 7, ArtistName: "someone", TrackTitle: "announcements"}))
}

func TestRadioPlayExclusions_ExactIsWholeValue(t *testing.T) {
	e := mustExclusions(t, exclusionRow("artist_name", "exact", "unknown", "placeholder"))
	assert.True(t, e.Excludes(RadioRankingArtist, RadioNormalizedPlay{ArtistName: "unknown"}))
	assert.False(t, e.Excludes(RadioRankingArtist, RadioNormalizedPlay{ArtistName: "unknown mortal orchestra"}))
}

func TestRadioPlayExclusions_UnknownDimensionPanics(t *testing.T) {
	e := mustExclusions(t)
	assert.Panics(t, func() { e.Excludes(RadioRankingDimension("release"), RadioNormalizedPlay{}) })
}

func TestRadioPlayExcludedSQL_Shape(t *testing.T) {
	cols := RadioPlayColumns{StationID: "rsh.station_id", ArtistName: "rp.artist_name", TrackTitle: "rp.track_title", LabelName: "rp.label_name"}

	label := RadioPlayExcludedSQL(RadioRankingLabel, cols)
	assert.True(t, strings.HasPrefix(label, "EXISTS ("))
	assert.Contains(t, label, "radio_normalize_name(rp.artist_name)")
	assert.Contains(t, label, "x.station_id = rsh.station_id")
	assert.Contains(t, label, "x.match_kind IN ('label_name')")

	track := RadioPlayExcludedSQL(RadioRankingTrack, cols)
	assert.Contains(t, track, "x.match_kind IN ('artist_name', 'track_title')")
}

func TestRadioPlayExcludedSQL_PanicsOnNonIdentifier(t *testing.T) {
	cols := RadioPlayColumns{StationID: "rsh.station_id", ArtistName: "rp.artist_name); DROP TABLE x; --", TrackTitle: "rp.track_title", LabelName: "rp.label_name"}
	assert.Panics(t, func() { RadioPlayExcludedSQL(RadioRankingArtist, cols) })
	assert.Panics(t, func() { RadioStationFloorSQL("1; DROP TABLE x") })
}

func TestRadioMeetsStationFloor(t *testing.T) {
	assert.False(t, RadioMeetsStationFloor(0))
	assert.False(t, RadioMeetsStationFloor(1))
	assert.True(t, RadioMeetsStationFloor(2))
	assert.True(t, RadioMeetsStationFloor(5))
	assert.Equal(t, "count(DISTINCT rsh.station_id) >= 2", RadioStationFloorSQL("rsh.station_id"))
}
