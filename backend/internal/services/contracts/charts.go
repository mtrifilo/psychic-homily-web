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
// so adding a value here means updating those tags, OrDefault below, AND the
// chartWindowStart switch in services/catalog/charts_service.go.
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

// MostAnticipatedMode discriminates the most-anticipated payload shape.
// Ranked is the engagement chart (every row cleared the save floor, counts
// included); soonest-upcoming is the fallback when too few shows qualify —
// date-ordered with counts OMITTED, so the frontend never renders the
// sparse-engagement numbers the floor exists to hide. (The fallback is only
// as full as the upcoming calendar: with zero upcoming shows it is empty.)
type MostAnticipatedMode string

const (
	MostAnticipatedModeRanked          MostAnticipatedMode = "ranked"
	MostAnticipatedModeSoonestUpcoming MostAnticipatedMode = "soonest_upcoming"
)

// MostAnticipatedShow is one row of the most-anticipated module. SaveCount
// is nil in soonest-upcoming fallback mode — omitted from the payload, never
// zero — so a sub-floor count can't leak into the UI. Rank follows the same
// rule: present (1-based, offset-stable) in ranked mode, nil in fallback
// (a date-ordered fallback list has no rank to claim).
type MostAnticipatedShow struct {
	ShowID      uint      `json:"show_id"`
	Title       string    `json:"title"`
	Slug        string    `json:"slug"`
	Date        time.Time `json:"date"`
	VenueName   string    `json:"venue_name"`
	VenueSlug   string    `json:"venue_slug"`
	City        string    `json:"city"`
	ArtistNames []string  `json:"artist_names"`
	SaveCount   *int      `json:"save_count,omitempty"`
	Rank        *int      `json:"rank,omitempty"`
}

// MostAnticipatedShows is the mode-discriminated most-anticipated payload.
// Total counts the FULL set the active mode displays — qualifying shows in
// ranked mode, all upcoming approved shows in fallback mode. Pagination
// applies to ranked mode only; fallback ignores offset (it is the module's
// floor, not a ranked list — a paginating client must key off mode, never
// off total alone). Pages are cached independently server-side, so mode and
// total are per-response facts: two pages of one window can come from
// snapshots up to a cache TTL apart, and a client must re-check mode on
// every page rather than assume cross-page consistency.
type MostAnticipatedShows struct {
	Mode  MostAnticipatedMode   `json:"mode"`
	Total int                   `json:"total"`
	Shows []MostAnticipatedShow `json:"shows"`
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
	Rank          int        `json:"rank"`
}

// BusiestVenue represents a venue ranked by shows hosted within a window.
type BusiestVenue struct {
	VenueID   uint   `json:"venue_id"`
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	City      string `json:"city"`
	State     string `json:"state"`
	ShowCount int    `json:"show_count"`
	Rank      int    `json:"rank"`
}

// OpenerToWatch represents an artist ranked by support slots played within a
// window, excluding artists who headlined at all in that window.
type OpenerToWatch struct {
	ArtistID         uint   `json:"artist_id"`
	Name             string `json:"name"`
	Slug             string `json:"slug"`
	City             string `json:"city"`
	State            string `json:"state"`
	SupportSlotCount int    `json:"support_slot_count"`
	Rank             int    `json:"rank"`
}

// NewRelease is one row of the windowed new-releases module: date-ordered,
// no engagement inputs. ReleaseDate is the world release date as a day-grain
// YYYY-MM-DD string — the same shape as every release contract, so a
// west-of-UTC client can't shift it a day by parsing a midnight timestamp —
// nil when unknown; AddedAt is when the release entered the graph. The
// ordering and window date is COALESCE(release_date, added_at-day): "new
// releases", not "new to the graph" — a backfilled release whose known world
// date is old orders by that old date and does not appear in the bounded
// windows (all_time has no lower bound, so it appears there, ordered by its
// old date). Only date-unknown releases surface by their graph-added day
// (ReleaseDate nil is the graph-new tell).
type NewRelease struct {
	ReleaseID   uint      `json:"release_id"`
	Title       string    `json:"title"`
	Slug        string    `json:"slug"`
	ReleaseType string    `json:"release_type"`
	ReleaseDate *string   `json:"release_date"`
	AddedAt     time.Time `json:"added_at"`
	ArtistNames []string  `json:"artist_names"`
	LabelNames  []string  `json:"label_names"`
	Rank        int       `json:"rank"`
}

// OnTheRadioArtist represents an artist ranked by resolved radio plays within
// a window. StationCount counts distinct broadcasters: stations grouped under
// a radio_network (e.g. WFMU's flagship + stream-only sub-channels) collapse
// to one, standalone stations count individually.
type OnTheRadioArtist struct {
	ArtistID     uint   `json:"artist_id"`
	Name         string `json:"name"`
	Slug         string `json:"slug"`
	City         string `json:"city"`
	State        string `json:"state"`
	PlayCount    int    `json:"play_count"`
	StationCount int    `json:"station_count"`
	IsNew        bool   `json:"is_new"`
	Rank         int    `json:"rank"`
}

// ChartsSummary is the masthead proof-of-life stat strip: window-scoped
// counts of graph activity. ShowsAdded/NewArtists/NewReleases count entities
// ADDED to the graph in the window (created_at, the honest claim);
// RadioPlays counts plays on aired in-window episodes (pseudo-artist rows
// excluded, unmatched plays included — logging activity, not match rate);
// ActiveScenes counts distinct scenes (the shared scene-grouping identity)
// with at least one show played in the window.
type ChartsSummary struct {
	ShowsAdded   int `json:"shows_added"`
	NewArtists   int `json:"new_artists"`
	NewReleases  int `json:"new_releases"`
	RadioPlays   int `json:"radio_plays"`
	ActiveScenes int `json:"active_scenes"`
}

// FreshlyAddedItem is one row of the freshly-added footer ticker: the most
// recently added entities across types, newest first.
type FreshlyAddedItem struct {
	EntityType string    `json:"entity_type"` // artist | venue | release | station
	EntityID   uint      `json:"entity_id"`
	Name       string    `json:"name"`
	Slug       string    `json:"slug"`
	AddedAt    time.Time `json:"added_at"`
}

// PersonalTopVenue is the venue holding the most of a user's saved shows.
// Each saved show attributes to its primary venue (the repo's lowest-venue_id
// pick — see the pv lateral in show.go), so a multi-venue show counts once
// and the per-venue counts can never sum past the user's saved-show total.
type PersonalTopVenue struct {
	VenueID        uint   `json:"venue_id"`
	Name           string `json:"name"`
	Slug           string `json:"slug"`
	SavedShowCount int    `json:"saved_show_count"`
}

// PersonalChartsStats is the authed personal stats strip: all-time aggregates
// over the requesting user's own engagement rows. Zeros are a valid shape (a
// new user sees zeros and the frontend renders a "start marking shows" nudge);
// TopVenue is nil until the user has a saved show with a venue, and
// FirstActivityAt is nil until they have any bookmark row at all.
type PersonalChartsStats struct {
	SavedShows      int               `json:"saved_shows"`
	ArtistsFollowed int               `json:"artists_followed"`
	TopVenue        *PersonalTopVenue `json:"top_venue"`
	FirstActivityAt *time.Time        `json:"first_activity_at"`
}

// ChartsOverview contains condensed top-5 versions of the four original
// charts (trending shows, popular artists, active venues, hot releases).
// The windowed module charts (most-active-artists, busiest-venues,
// openers-to-watch, …) are separate endpoints and intentionally NOT included
// here — the overview payload gets reworked wholesale with the Broadsheet
// frontend.
type ChartsOverview struct {
	TrendingShows  []TrendingShow  `json:"trending_shows"`
	PopularArtists []PopularArtist `json:"popular_artists"`
	ActiveVenues   []ActiveVenue   `json:"active_venues"`
	HotReleases    []HotRelease    `json:"hot_releases"`
}

// ChartScene is one option in the charts scene switcher: a US Census CBSA
// metro with at least the coverage floor of in-window shows. Metro is the
// value the module endpoints accept as `scene`; City/State are the metro's
// principal-city display identity (the same identity the scenes directory
// shows). Fallback (city|state) scenes — non-US or no-CBSA — are not chart
// scopes and never appear here.
type ChartScene struct {
	Metro     string `json:"metro"`
	City      string `json:"city"`
	State     string `json:"state"`
	ShowCount int    `json:"show_count"`
}

// ──────────────────────────────────────────────
// Charts Service Interface
// ──────────────────────────────────────────────

// ChartsServiceInterface defines the contract for top charts / trending content.
// Param preconditions (services do not re-validate): limit >= 1 and, on the
// paginated module methods, offset >= 0. The legacy methods' limits are
// clamped Go-side (normalizeChartsLimit); the paginated module endpoints
// enforce theirs via huma default/minimum/maximum tags at the HTTP layer.
// `scene` is a CBSA metro code or "" for global — shape-validated at the HTTP
// layer (pattern tag); an unknown-but-well-formed scene yields empty results
// with a valid envelope, never an error.
type ChartsServiceInterface interface {
	GetTrendingShows(limit int) ([]TrendingShow, error)
	GetMostAnticipatedShows(scene string, limit, offset int) (*MostAnticipatedShows, error)
	GetMostActiveArtists(window ChartWindow, scene string, limit, offset int) ([]MostActiveArtist, int, error)
	GetBusiestVenues(window ChartWindow, scene string, limit, offset int) ([]BusiestVenue, int, error)
	GetOpenersToWatch(window ChartWindow, scene string, limit, offset int) ([]OpenerToWatch, int, error)
	GetOnTheRadioArtists(window ChartWindow, scene string, limit, offset int) ([]OnTheRadioArtist, int, error)
	GetNewReleases(window ChartWindow, scene string, limit, offset int) ([]NewRelease, int, error)
	GetChartsSummary(window ChartWindow, scene string) (*ChartsSummary, error)
	GetFreshlyAdded(scene string, limit int) ([]FreshlyAddedItem, error)
	GetChartScenes(window ChartWindow) ([]ChartScene, error)
	GetPersonalChartsStats(userID uint) (*PersonalChartsStats, error)
	GetPopularArtists(limit int) ([]PopularArtist, error)
	GetActiveVenues(limit int) ([]ActiveVenue, error)
	GetHotReleases(limit int) ([]HotRelease, error)
	GetChartsOverview() (*ChartsOverview, error)
}
