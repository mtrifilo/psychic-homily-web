package catalog

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// RadioRankingDimension names what an enrichment ranking counts plays toward.
// It decides which placeholder exclusions apply: a placeholder disqualifies a
// play only for dimensions whose key includes the matched field.
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

// RadioNormalizedPlay is one play as the exclusion predicate sees it: each
// field is the value radio_normalize_name returns for the raw column, with
// NULL as "". Taking the SQL-normalized value (not a Go re-normalization) is
// what keeps RadioPlayExclusions.Excludes and RadioPlayExcludedSQL in
// agreement, since the Go and SQL normalizers differ on non-ASCII edges.
type RadioNormalizedPlay struct {
	StationID  uint
	ArtistName string
	TrackTitle string
	LabelName  string
}

func (p RadioNormalizedPlay) field(matchKind string) string {
	switch matchKind {
	case catalogm.RadioPlayExclusionMatchArtistName:
		return p.ArtistName
	case catalogm.RadioPlayExclusionMatchTrackTitle:
		return p.TrackTitle
	case catalogm.RadioPlayExclusionMatchLabelName:
		return p.LabelName
	default:
		return ""
	}
}

// radioExclusionRegexNonPortable matches regex syntax that Postgres ARE and Go
// RE2 read differently or that only one of them accepts: an escape of a letter
// or digit, a POSIX class, or a (? group. It mirrors the
// radio_play_exclusions_pattern_form_check constraint.
var radioExclusionRegexNonPortable = regexp.MustCompile(`\\[\p{L}\p{N}]|\[\[:|\(\?`)

type radioPlayExclusionRule struct {
	matchKind string
	category  string
	stationID *uint
	exact     string
	re        *regexp.Regexp
}

func (r radioPlayExclusionRule) matches(value string) bool {
	if value == "" {
		return false
	}
	if r.re != nil {
		return r.re.MatchString(value)
	}
	return value == r.exact
}

// RadioPlayExclusions is the compiled form of radio_play_exclusions. It is
// immutable after construction and safe for concurrent use.
type RadioPlayExclusions struct {
	rules []radioPlayExclusionRule
}

// NewRadioPlayExclusions compiles exclusion rows. It returns an error for a row
// whose match kind, pattern type or category is unknown, or whose regex RE2
// cannot compile or that uses syntax outside the portable subset, so a ranking
// never runs with a rule the SQL predicate would read differently.
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
			if radioExclusionRegexNonPortable.MatchString(row.Pattern) {
				return nil, fmt.Errorf("%s: regex uses syntax outside the Postgres/RE2 portable subset", desc)
			}
			re, err := regexp.Compile(row.Pattern)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", desc, err)
			}
			rule.re = re
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
// given dimension. A play is excluded when a rule scoped to its station (or
// global) matches a non-empty field and the rule is a station element or talk
// (any field), or a placeholder on a field in the dimension's key. It returns
// the same answer as RadioPlayExcludedSQL over the same rows.
func (e *RadioPlayExclusions) Excludes(dim RadioRankingDimension, play RadioNormalizedPlay) bool {
	keyKinds := dim.keyMatchKinds()
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

var sqlColumnRef = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)?$`)

func mustSQLColumnRef(ref string) string {
	if !sqlColumnRef.MatchString(ref) {
		panic(fmt.Sprintf("catalog: %q is not a SQL column reference", ref))
	}
	return ref
}

// RadioPlayExcludedSQL returns a boolean SQL expression that is true when the
// play named by cols must not count toward a ranking of the given dimension.
// It is the SQL form of RadioPlayExclusions.Excludes over the live
// radio_play_exclusions rows; apply it to radio_plays rows before
// aggregating, as `WHERE NOT (<expr>)`. Each field is normalized once per play
// with radio_normalize_name. Panics on an unknown dimension or a column
// reference that is not an identifier, both programming errors.
func RadioPlayExcludedSQL(dim RadioRankingDimension, cols RadioPlayColumns) string {
	keyKinds := dim.keyMatchKinds()
	quoted := make([]string, len(keyKinds))
	for i, k := range keyKinds {
		quoted[i] = "'" + k + "'"
	}
	field := fmt.Sprintf(
		"CASE x.match_kind WHEN '%s' THEN n.artist_name WHEN '%s' THEN n.track_title WHEN '%s' THEN n.label_name END",
		catalogm.RadioPlayExclusionMatchArtistName,
		catalogm.RadioPlayExclusionMatchTrackTitle,
		catalogm.RadioPlayExclusionMatchLabelName,
	)
	// OFFSET 0 keeps the planner from inlining the normalization subquery, so
	// each field is normalized once per play rather than once per rule.
	return fmt.Sprintf(`EXISTS (
  SELECT 1
  FROM (SELECT radio_normalize_name(%s) AS artist_name,
               radio_normalize_name(%s) AS track_title,
               radio_normalize_name(%s) AS label_name
        OFFSET 0) n
  CROSS JOIN radio_play_exclusions x
  WHERE (x.station_id IS NULL OR x.station_id = %s)
    AND (x.category <> '%s' OR x.match_kind IN (%s))
    AND %s <> ''
    AND CASE x.pattern_type
          WHEN '%s' THEN %s = x.pattern
          ELSE %s ~ x.pattern
        END
)`,
		mustSQLColumnRef(cols.ArtistName),
		mustSQLColumnRef(cols.TrackTitle),
		mustSQLColumnRef(cols.LabelName),
		mustSQLColumnRef(cols.StationID),
		catalogm.RadioPlayExclusionCategoryPlaceholder,
		strings.Join(quoted, ", "),
		field,
		catalogm.RadioPlayExclusionPatternExact, field,
		field,
	)
}

// RadioMinDistinctStations is the single-station rule: a key (artist, track or
// label) whose non-excluded plays span fewer distinct radio_stations rows than
// this never qualifies for a ranking, whatever its play count.
const RadioMinDistinctStations = 2

// RadioMeetsStationFloor applies the single-station rule to an aggregated key,
// given the number of distinct radio_stations rows among its non-excluded
// plays.
func RadioMeetsStationFloor(distinctStations int) bool {
	return distinctStations >= RadioMinDistinctStations
}

// RadioStationFloorSQL returns the single-station rule as an aggregate boolean
// for a HAVING clause, counting distinct values of the station id column over
// the group's rows. The rows must already be filtered with
// RadioPlayExcludedSQL. Panics on a column reference that is not an
// identifier.
func RadioStationFloorSQL(stationIDColumn string) string {
	return fmt.Sprintf("count(DISTINCT %s) >= %d", mustSQLColumnRef(stationIDColumn), RadioMinDistinctStations)
}
