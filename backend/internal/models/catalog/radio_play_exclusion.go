package catalog

import "time"

// Match kinds for radio_play_exclusions.match_kind: the radio_plays field a
// pattern is tested against. Enforced by radio_play_exclusions_match_kind_check.
const (
	RadioPlayExclusionMatchArtistName = "artist_name"
	RadioPlayExclusionMatchTrackTitle = "track_title"
	RadioPlayExclusionMatchLabelName  = "label_name"
)

// Pattern types for radio_play_exclusions.pattern_type. Exact patterns are
// stored in radio_normalize_name form and compared with equality; regex
// patterns are matched against the normalized value. Enforced by
// radio_play_exclusions_pattern_type_check.
const (
	RadioPlayExclusionPatternExact = "exact"
	RadioPlayExclusionPatternRegex = "regex"
)

// Categories for radio_play_exclusions.category. Station elements and talk
// exclude the whole play; a placeholder disqualifies only the matched field.
// Enforced by radio_play_exclusions_category_check.
const (
	RadioPlayExclusionCategoryStationElement = "station_element"
	RadioPlayExclusionCategoryTalk           = "talk"
	RadioPlayExclusionCategoryPlaceholder    = "placeholder"
)

// RadioPlayExclusion is one row of radio_play_exclusions: a pattern that marks
// radio plays (or one field of them) as noise for the enrichment rankings.
// StationID nil applies the row on every station.
type RadioPlayExclusion struct {
	ID          uint      `gorm:"primaryKey"`
	MatchKind   string    `gorm:"column:match_kind;not null"`
	PatternType string    `gorm:"column:pattern_type;not null"`
	Pattern     string    `gorm:"column:pattern;not null"`
	Category    string    `gorm:"column:category;not null"`
	StationID   *uint     `gorm:"column:station_id"`
	Reason      string    `gorm:"column:reason;not null"`
	CreatedAt   time.Time `gorm:"not null"`
}

// TableName specifies the table name for RadioPlayExclusion.
func (RadioPlayExclusion) TableName() string {
	return "radio_play_exclusions"
}
