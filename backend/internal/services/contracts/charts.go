package contracts

import "time"

// ──────────────────────────────────────────────
// Charts types (top charts / trending content)
// ──────────────────────────────────────────────

// TrendingShow represents a show ranked by how many users have saved it.
type TrendingShow struct {
	ShowID      uint      `json:"show_id"`
	Title       string    `json:"title"`
	Slug        string    `json:"slug"`
	Date        time.Time `json:"date"`
	VenueName   string    `json:"venue_name"`
	VenueSlug   string    `json:"venue_slug"`
	City        string    `json:"city"`
	ArtistNames []string  `json:"artist_names"`
	SaveCount   int       `json:"save_count"`
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
	GetPopularArtists(limit int) ([]PopularArtist, error)
	GetActiveVenues(limit int) ([]ActiveVenue, error)
	GetHotReleases(limit int) ([]HotRelease, error)
	GetChartsOverview() (*ChartsOverview, error)
}
