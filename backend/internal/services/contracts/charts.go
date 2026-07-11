package contracts

import "time"

// ──────────────────────────────────────────────
// Charts types (top charts / trending content)
// ──────────────────────────────────────────────

// ChartWindow selects the rolling time window a chart is computed over.
// Windows are rolling (last 30/90 days), not calendar-aligned, so lists stay
// full on month/quarter boundaries.
//
// The string values are duplicated in `enum:"..."` struct tags on chart
// request structs (api/handlers/catalog/charts.go) — Go tags must be literals,
// so adding a value here means updating those tags too.
type ChartWindow string

const (
	ChartWindowMonth   ChartWindow = "month"
	ChartWindowQuarter ChartWindow = "quarter"
	ChartWindowAllTime ChartWindow = "all_time"
)

// OrDefault maps the empty/unknown window to the default (quarter). This is
// the single owner of the default — both the API layer and the services use
// it so an echoed `window` value always matches the data's actual window.
func (w ChartWindow) OrDefault() ChartWindow {
	switch w {
	case ChartWindowMonth, ChartWindowQuarter, ChartWindowAllTime:
		return w
	default:
		return ChartWindowQuarter
	}
}

// TrendingShow represents a show ranked by attendance (going + interested).
type TrendingShow struct {
	ShowID          uint      `json:"show_id"`
	Title           string    `json:"title"`
	Slug            string    `json:"slug"`
	Date            time.Time `json:"date"`
	VenueName       string    `json:"venue_name"`
	VenueSlug       string    `json:"venue_slug"`
	City            string    `json:"city"`
	ArtistNames     []string  `json:"artist_names"`
	GoingCount      int       `json:"going_count"`
	InterestedCount int       `json:"interested_count"`
	TotalAttendance int       `json:"total_attendance"`
}

// PopularArtist represents an artist ranked by followers and upcoming shows.
type PopularArtist struct {
	ArtistID          uint   `json:"artist_id"`
	Name              string `json:"name"`
	Slug              string `json:"slug"`
	ImageURL          string `json:"image_url"`
	FollowCount       int    `json:"follow_count"`
	UpcomingShowCount int    `json:"upcoming_show_count"`
	Score             int    `json:"score"`
}

// ActiveVenue represents a venue ranked by upcoming shows and followers.
type ActiveVenue struct {
	VenueID           uint   `json:"venue_id"`
	Name              string `json:"name"`
	Slug              string `json:"slug"`
	City              string `json:"city"`
	State             string `json:"state"`
	UpcomingShowCount int    `json:"upcoming_show_count"`
	FollowCount       int    `json:"follow_count"`
	Score             int    `json:"score"`
}

// HotRelease represents a release ranked by recent bookmarks.
type HotRelease struct {
	ReleaseID     uint       `json:"release_id"`
	Title         string     `json:"title"`
	Slug          string     `json:"slug"`
	ReleaseDate   *time.Time `json:"release_date"`
	ArtistNames   []string   `json:"artist_names"`
	BookmarkCount int        `json:"bookmark_count"`
}

// MostActiveArtist represents an artist ranked by shows played within a window.
type MostActiveArtist struct {
	ArtistID      uint       `json:"artist_id"`
	Name          string     `json:"name"`
	Slug          string     `json:"slug"`
	City          string     `json:"city"`
	State         string     `json:"state"`
	ShowCount     int        `json:"show_count"`
	HeadlinePct   int        `json:"headline_pct"`
	LastShowDate  *time.Time `json:"last_show_date"`
	LastShowSlug  string     `json:"last_show_slug"`
	LastShowVenue string     `json:"last_show_venue"`
}

// ChartsOverview contains condensed top-5 versions of all charts for dashboard use.
type ChartsOverview struct {
	TrendingShows  []TrendingShow  `json:"trending_shows"`
	PopularArtists []PopularArtist `json:"popular_artists"`
	ActiveVenues   []ActiveVenue   `json:"active_venues"`
	HotReleases    []HotRelease    `json:"hot_releases"`
}

// ──────────────────────────────────────────────
// Charts Service Interface
// ──────────────────────────────────────────────

// ChartsServiceInterface defines the contract for top charts / trending content.
type ChartsServiceInterface interface {
	GetTrendingShows(limit int) ([]TrendingShow, error)
	GetMostActiveArtists(window ChartWindow, limit int) ([]MostActiveArtist, error)
	GetPopularArtists(limit int) ([]PopularArtist, error)
	GetActiveVenues(limit int) ([]ActiveVenue, error)
	GetHotReleases(limit int) ([]HotRelease, error)
	GetChartsOverview() (*ChartsOverview, error)
}
