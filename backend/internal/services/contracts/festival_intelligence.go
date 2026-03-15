package contracts

// ──────────────────────────────────────────────
// Festival Intelligence types
// ──────────────────────────────────────────────

// FestivalSummary is a lightweight reference to a festival used in intelligence responses.
type FestivalSummary struct {
	ID          uint    `json:"id"`
	Name        string  `json:"name"`
	Slug        string  `json:"slug"`
	SeriesSlug  string  `json:"series_slug"`
	EditionYear int     `json:"edition_year"`
	City        *string `json:"city"`
	State       *string `json:"state"`
}

// ArtistSummary is a lightweight reference to an artist used in intelligence responses.
type ArtistSummary struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// SimilarFestival represents a festival with computed lineup overlap metrics.
type SimilarFestival struct {
	Festival          FestivalSummary `json:"festival"`
	SharedArtistCount int             `json:"shared_artist_count"`
	Jaccard           float64         `json:"jaccard"`
	WeightedScore     float64         `json:"weighted_score"`
	TopShared         []SharedArtist  `json:"top_shared"`
}

// SharedArtist represents an artist shared between two festivals with tier context.
type SharedArtist struct {
	ArtistID     uint   `json:"artist_id"`
	Name         string `json:"name"`
	Slug         string `json:"slug"`
	TierAtSource string `json:"tier_at_source"`
	TierAtTarget string `json:"tier_at_target"`
}

// FestivalOverlap is the detailed comparison between two specific festivals.
type FestivalOverlap struct {
	FestivalA     FestivalSummary `json:"festival_a"`
	FestivalB     FestivalSummary `json:"festival_b"`
	SharedArtists []SharedArtist  `json:"shared_artists"`
	Jaccard       float64         `json:"jaccard"`
	WeightedScore float64         `json:"weighted_score"`
	AOnlyCount    int             `json:"a_only_count"`
	BOnlyCount    int             `json:"b_only_count"`
}

// FestivalBreakouts contains breakout artists and milestone events for a festival.
type FestivalBreakouts struct {
	Breakouts  []ArtistBreakout  `json:"breakouts"`
	Milestones []ArtistMilestone `json:"milestones"`
}

// ArtistBreakout represents an artist whose billing tier improved across festivals.
type ArtistBreakout struct {
	Artist          ArtistSummary   `json:"artist"`
	CurrentTier     string          `json:"current_tier"`
	Trajectory      []TrajectoryEntry `json:"trajectory"`
	TierImprovement int             `json:"tier_improvement"`
	BreakoutScore   float64         `json:"breakout_score"`
}

// TrajectoryEntry is one stop in an artist's festival billing history.
type TrajectoryEntry struct {
	FestivalName string `json:"festival_name"`
	FestivalSlug string `json:"festival_slug"`
	Year         int    `json:"year"`
	Tier         string `json:"tier"`
}

// ArtistMilestone marks a notable moment in an artist's festival career.
type ArtistMilestone struct {
	Artist    ArtistSummary `json:"artist"`
	Milestone string        `json:"milestone"`
	Tier      string        `json:"tier"`
	Festival  string        `json:"festival"`
}

// ArtistTrajectory is the full festival billing history for a single artist.
type ArtistTrajectory struct {
	Artist           ArtistSummary   `json:"artist"`
	Appearances      []TrajectoryEntry `json:"appearances"`
	BestTier         string          `json:"best_tier"`
	TotalAppearances int             `json:"total_appearances"`
	BreakoutScore    float64         `json:"breakout_score"`
}

// SeriesComparison shows year-over-year analysis for a recurring festival series.
type SeriesComparison struct {
	SeriesSlug       string             `json:"series_slug"`
	Editions         []SeriesEdition    `json:"editions"`
	ReturningArtists []ReturningArtist  `json:"returning_artists"`
	Newcomers        []SeriesNewcomer   `json:"newcomers"`
	RetentionRate    float64            `json:"retention_rate"`
	LineupGrowth     float64            `json:"lineup_growth"`
}

// SeriesEdition describes one year/edition in a festival series.
type SeriesEdition struct {
	FestivalID  uint   `json:"festival_id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Year        int    `json:"year"`
	ArtistCount int    `json:"artist_count"`
}

// ReturningArtist is an artist who appears in multiple editions with tier changes.
type ReturningArtist struct {
	Artist ArtistSummary     `json:"artist"`
	Years  []int             `json:"years"`
	Tiers  map[string]string `json:"tiers"` // year (as string) -> tier
}

// SeriesNewcomer is an artist appearing for the first time in the latest compared edition.
type SeriesNewcomer struct {
	Artist ArtistSummary `json:"artist"`
	Tier   string        `json:"tier"`
}

// FestivalIntelligenceServiceInterface defines the contract for festival intelligence operations.
type FestivalIntelligenceServiceInterface interface {
	GetSimilarFestivals(festivalID uint, limit int) ([]SimilarFestival, error)
	GetFestivalOverlap(festivalAID, festivalBID uint) (*FestivalOverlap, error)
	GetFestivalBreakouts(festivalID uint) (*FestivalBreakouts, error)
	GetArtistFestivalTrajectory(artistID uint) (*ArtistTrajectory, error)
	GetSeriesComparison(seriesSlug string, years []int) (*SeriesComparison, error)
}
