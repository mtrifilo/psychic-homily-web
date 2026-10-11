package catalog

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// The radio_play_exclusions table and this file govern the enrichment
// rankings. Display aggregations (charts, top artists, now playing) filter the
// WFMU "Music behind DJ:" prefix separately with pseudoArtistExclusionSQL.

// RadioRankingDimension names what an enrichment ranking counts plays toward.
// It decides which fields form the key: a play with a blank key field never
// counts toward that dimension, and a placeholder exclusion disqualifies a play
// only for dimensions whose key includes the matched field.
type RadioRankingDimension string

const (
	// RadioRankingArtist keys on the artist name.
	RadioRankingArtist RadioRankingDimension = "artist"
	// RadioRankingTrack keys on the artist name and the track title.
	RadioRankingTrack RadioRankingDimension = "track"
	// RadioRankingLabel keys on the label name.
	RadioRankingLabel RadioRankingDimension = "label"
)

// keyMatchKinds returns the radio_play_exclusions match kinds that form the
// dimension's key. Panics on an unknown dimension: dimensions are compile-time
// constants, so an unknown one is a programming error.
func (d RadioRankingDimension) keyMatchKinds() []string {
	switch d {
	case RadioRankingArtist:
		return []string{catalogm.RadioPlayExclusionMatchArtistName}
	case RadioRankingTrack:
		return []string{catalogm.RadioPlayExclusionMatchArtistName, catalogm.RadioPlayExclusionMatchTrackTitle}
	case RadioRankingLabel:
		return []string{catalogm.RadioPlayExclusionMatchLabelName}
	default:
		panic(fmt.Sprintf("catalog: unknown radio ranking dimension %q", string(d)))
	}
}

// RadioNormalizedPlay is one play as the exclusion predicate sees it. Each
// field is what RadioNormalizedFieldSQL returns for the raw column: nil when
// the raw value is NULL or blank, otherwise the radio_normalize_name output,
// which is "" for a name whose characters are all non-ASCII. Taking the
// SQL-normalized value (not a Go re-normalization) is what keeps
// RadioPlayExclusions.Excludes and RadioPlayExcludedSQL in agreement, since
// the Go and SQL normalizers differ on non-ASCII edges.
type RadioNormalizedPlay struct {
	StationID  uint
	ArtistName *string
	TrackTitle *string
	LabelName  *string
}

func (p RadioNormalizedPlay) field(matchKind string) *string {
	switch matchKind {
	case catalogm.RadioPlayExclusionMatchArtistName:
		return p.ArtistName
	case catalogm.RadioPlayExclusionMatchTrackTitle:
		return p.TrackTitle
	case catalogm.RadioPlayExclusionMatchLabelName:
		return p.LabelName
	default:
		return nil
	}
}

// RadioExclusionRegexMaxLen is the longest regex pattern, in characters, that
// radio_play_exclusions accepts.
const RadioExclusionRegexMaxLen = 200

// radioExclusionRegexRefused matches regex syntax that radio_play_exclusions
// refuses, mirroring radio_play_exclusions_pattern_form_check: an escape of
// anything but a regex metacharacter or /, a POSIX bracket construct ([: [.
// [=), a (? group, a leading *, a brace, or an ASCII capital. What remains is
// read identically by Postgres ARE and Go RE2, and every such pattern RE2 can
// compile.
var radioExclusionRegexRefused = regexp.MustCompile(`\\[^\]\[.()*+?|^$\\{}/-]|\[[:.=]|\(\?|^\*|\{|[A-Z]`)

// radioExclusionRegexError reports why radio_play_exclusions would refuse a
// regex pattern, or nil when it accepts it.
func radioExclusionRegexError(pattern string) error {
	if utf8.RuneCountInString(pattern) > RadioExclusionRegexMaxLen {
		return fmt.Errorf("regex longer than %d characters", RadioExclusionRegexMaxLen)
	}
	if radioExclusionRegexRefused.MatchString(pattern) {
		return fmt.Errorf("regex uses syntax outside the Postgres/RE2 portable subset")
	}
	if _, err := regexp.Compile(pattern); err != nil {
		return err
	}
	return nil
}

type radioPlayExclusionRule struct {
	matchKind string
	category  string
	stationID *uint
	exact     string
	re        *regexp.Regexp
}

func (r radioPlayExclusionRule) matches(value *string) bool {
	if value == nil || *value == "" {
		return false
	}
	if r.re != nil {
		return r.re.MatchString(*value)
	}
	return *value == r.exact
}

// RadioPlayExclusions is the compiled form of radio_play_exclusions. It is
// immutable after construction and safe for concurrent use.
type RadioPlayExclusions struct {
	rules []radioPlayExclusionRule
}

// NewRadioPlayExclusions compiles exclusion rows. It returns an error for a row
// whose match kind, pattern type or category is unknown, or whose regex the
// table's pattern-form constraint would refuse, so a ranking never runs with a
// rule the SQL predicate would read differently.
func NewRadioPlayExclusions(rows []catalogm.RadioPlayExclusion) (*RadioPlayExclusions, error) {
	rules := make([]radioPlayExclusionRule, 0, len(rows))
	for _, row := range rows {
		desc := fmt.Sprintf("radio_play_exclusions id=%d (%s %s %q)", row.ID, row.MatchKind, row.PatternType, row.Pattern)
		switch row.MatchKind {
		case catalogm.RadioPlayExclusionMatchArtistName,
			catalogm.RadioPlayExclusionMatchTrackTitle,
			catalogm.RadioPlayExclusionMatchLabelName:
		default:
			return nil, fmt.Errorf("%s: unknown match kind", desc)
		}
		switch row.Category {
		case catalogm.RadioPlayExclusionCategoryStationElement,
			catalogm.RadioPlayExclusionCategoryTalk,
			catalogm.RadioPlayExclusionCategoryPlaceholder:
		default:
			return nil, fmt.Errorf("%s: unknown category %q", desc, row.Category)
		}
		if row.Pattern == "" {
			return nil, fmt.Errorf("%s: empty pattern", desc)
		}
		rule := radioPlayExclusionRule{
			matchKind: row.MatchKind,
			category:  row.Category,
			stationID: row.StationID,
		}
		switch row.PatternType {
		case catalogm.RadioPlayExclusionPatternExact:
			rule.exact = row.Pattern
		case catalogm.RadioPlayExclusionPatternRegex:
			if err := radioExclusionRegexError(row.Pattern); err != nil {
				return nil, fmt.Errorf("%s: %w", desc, err)
			}
			rule.re = regexp.MustCompile(row.Pattern)
		default:
			return nil, fmt.Errorf("%s: unknown pattern type", desc)
		}
		rules = append(rules, rule)
	}
	return &RadioPlayExclusions{rules: rules}, nil
}

// LoadRadioPlayExclusions reads and compiles every radio_play_exclusions row.
func LoadRadioPlayExclusions(db *gorm.DB) (*RadioPlayExclusions, error) {
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	var rows []catalogm.RadioPlayExclusion
	if err := db.Order("id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("loading radio play exclusions: %w", err)
	}
	return NewRadioPlayExclusions(rows)
}

// Excludes reports whether the play must not count toward a ranking of the
// given dimension. A play is excluded when a field in the dimension's key is
// blank (nil), or when a rule scoped to its station (or global) matches a
// non-empty field and the rule is a station element or talk (any field), or a
// placeholder on a field in the dimension's key. It returns the same answer as
// RadioPlayExcludedSQL over the same rows.
func (e *RadioPlayExclusions) Excludes(dim RadioRankingDimension, play RadioNormalizedPlay) bool {
	keyKinds := dim.keyMatchKinds()
	for _, kind := range keyKinds {
		if play.field(kind) == nil {
			return true
		}
	}
	for _, rule := range e.rules {
		if rule.stationID != nil && *rule.stationID != play.StationID {
			continue
		}
		if rule.category == catalogm.RadioPlayExclusionCategoryPlaceholder && !slices.Contains(keyKinds, rule.matchKind) {
			continue
		}
		if rule.matches(play.field(rule.matchKind)) {
			return true
		}
	}
	return false
}

// RadioPlayColumns names the SQL column references a query exposes for one
// radio play: the raw radio_plays fields and the station the play aired on.
// Each must be a plain or table-qualified identifier (rp.artist_name).
type RadioPlayColumns struct {
	StationID  string
	ArtistName string
	TrackTitle string
	LabelName  string
}

var (
	sqlIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	sqlColumnRef  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)?$`)
)

func mustSQLColumnRef(ref string) string {
	if !sqlColumnRef.MatchString(ref) {
		panic(fmt.Sprintf("catalog: %q is not a SQL column reference", ref))
	}
	return ref
}

// RadioNormalizedFieldSQL returns the SQL value RadioNormalizedPlay carries for
// a raw radio_plays text column: NULL when the raw value is NULL or has no
// non-space character, otherwise radio_normalize_name of it. Panics on a column
// reference that is not an identifier.
func RadioNormalizedFieldSQL(column string) string {
	c := mustSQLColumnRef(column)
	return fmt.Sprintf("CASE WHEN %s ~ '[^[:space:]]' THEN radio_normalize_name(%s) END", c, c)
}

// RadioPlayExcludedSQL returns a boolean SQL expression that is true when the
// play named by cols must not count toward a ranking of the given dimension.
// It is the SQL form of RadioPlayExclusions.Excludes over the live
// radio_play_exclusions rows; apply it to radio_plays rows before
// aggregating, as `WHERE NOT (<expr>)`. Panics on an unknown dimension or a
// column reference that is not an identifier, both programming errors.
func RadioPlayExcludedSQL(dim RadioRankingDimension, cols RadioPlayColumns) string {
	keyKinds := dim.keyMatchKinds()
	blankKey := make([]string, len(keyKinds))
	for i, kind := range keyKinds {
		blankKey[i] = "n." + kind + " IS NULL"
	}
	field := fmt.Sprintf(
		"CASE x.match_kind WHEN '%s' THEN n.artist_name WHEN '%s' THEN n.track_title WHEN '%s' THEN n.label_name END",
		catalogm.RadioPlayExclusionMatchArtistName,
		catalogm.RadioPlayExclusionMatchTrackTitle,
		catalogm.RadioPlayExclusionMatchLabelName,
	)
	// OFFSET 0 keeps the planner from inlining the normalization subquery, so
	// each field is normalized once per play rather than once per rule. Rules
	// are grouped so each group's regexes run as one alternation: Postgres
	// caches a few dozen compiled regexes per backend, and a pattern per row
	// would cycle that cache once the table holds more regex rows than it fits.
	// Each pattern is valid on its own, so wrapping it in a group and joining
	// with | matches exactly when one of the patterns matches.
	return fmt.Sprintf(`EXISTS (
  SELECT 1
  FROM (SELECT %s AS artist_name,
               %s AS track_title,
               %s AS label_name
        OFFSET 0) n
  WHERE %s
     OR EXISTS (
       SELECT 1
       FROM (SELECT match_kind, category, station_id, pattern_type,
                    '(' || string_agg(pattern, ')|(' ORDER BY pattern) || ')' AS regex,
                    array_agg(pattern) AS patterns
             FROM radio_play_exclusions
             GROUP BY match_kind, category, station_id, pattern_type) x
       WHERE (x.station_id IS NULL OR x.station_id = %s)
         AND (x.category <> '%s' OR x.match_kind IN (%s))
         AND %s <> ''
         AND CASE x.pattern_type
               WHEN '%s' THEN %s = ANY (x.patterns)
               ELSE %s ~ x.regex
             END
     )
)`,
		RadioNormalizedFieldSQL(cols.ArtistName),
		RadioNormalizedFieldSQL(cols.TrackTitle),
		RadioNormalizedFieldSQL(cols.LabelName),
		strings.Join(blankKey, " OR "),
		mustSQLColumnRef(cols.StationID),
		catalogm.RadioPlayExclusionCategoryPlaceholder,
		"'"+strings.Join(keyKinds, "', '")+"'",
		field,
		catalogm.RadioPlayExclusionPatternExact, field,
		field,
	)
}

// RadioMinDistinctBroadcasters is the single-station rule: a key (artist,
// track or label) whose non-excluded plays span fewer distinct broadcasters
// than this never qualifies for a ranking, whatever its play count. A
// broadcaster is a radio network, or a station that belongs to none
// (broadcasterKeySQL), so WFMU's flagship and its stream-only channels count
// once, as they do in the radio charts' station counts.
const RadioMinDistinctBroadcasters = 2

// RadioMeetsBroadcasterFloor applies the single-station rule to an aggregated
// key, given the number of distinct broadcasters among its non-excluded plays.
func RadioMeetsBroadcasterFloor(distinctBroadcasters int) bool {
	return distinctBroadcasters >= RadioMinDistinctBroadcasters
}

// RadioBroadcasterFloorSQL returns the single-station rule as an aggregate
// boolean for a HAVING clause. It counts distinct broadcasters over the
// group's rows through the radio_stations row aliased stationsAlias, so the
// query must join radio_stations under that alias, and its rows must already
// be filtered with RadioPlayExcludedSQL. Panics on an alias that is not an
// identifier.
func RadioBroadcasterFloorSQL(stationsAlias string) string {
	if !sqlIdentifier.MatchString(stationsAlias) {
		panic(fmt.Sprintf("catalog: %q is not a SQL identifier", stationsAlias))
	}
	return fmt.Sprintf("count(DISTINCT %s) >= %d", broadcasterKeySQL(stationsAlias), RadioMinDistinctBroadcasters)
}
