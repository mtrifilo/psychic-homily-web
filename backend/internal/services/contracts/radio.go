package contracts

import (
	"encoding/json"
	"time"
)

// ──────────────────────────────────────────────
// Radio Station types
// ──────────────────────────────────────────────

// CreateRadioStationRequest represents the data needed to create a new radio station
type CreateRadioStationRequest struct {
	Name             string           `json:"name" validate:"required"`
	Slug             string           `json:"slug"`
	Description      *string          `json:"description"`
	City             *string          `json:"city"`
	State            *string          `json:"state"`
	Country          *string          `json:"country"`
	Timezone         *string          `json:"timezone"`
	StreamURL        *string          `json:"stream_url"`
	StreamURLs       *json.RawMessage `json:"stream_urls,omitempty"`
	Website          *string          `json:"website"`
	DonationURL      *string          `json:"donation_url"`
	DonationEmbedURL *string          `json:"donation_embed_url"`
	LogoURL          *string          `json:"logo_url"`
	Social           *json.RawMessage `json:"social,omitempty"`
	BroadcastType    string           `json:"broadcast_type" validate:"required"`
	FrequencyMHz     *float64         `json:"frequency_mhz"`
	PlaylistSource   *string          `json:"playlist_source"`
	PlaylistConfig   *json.RawMessage `json:"playlist_config,omitempty"`
}

// UpdateRadioStationRequest represents the data that can be updated on a radio station
type UpdateRadioStationRequest struct {
	Name             *string          `json:"name"`
	Description      *string          `json:"description"`
	City             *string          `json:"city"`
	State            *string          `json:"state"`
	Country          *string          `json:"country"`
	Timezone         *string          `json:"timezone"`
	StreamURL        *string          `json:"stream_url"`
	StreamURLs       *json.RawMessage `json:"stream_urls,omitempty"`
	Website          *string          `json:"website"`
	DonationURL      *string          `json:"donation_url"`
	DonationEmbedURL *string          `json:"donation_embed_url"`
	LogoURL          *string          `json:"logo_url"`
	Social           *json.RawMessage `json:"social,omitempty"`
	BroadcastType    *string          `json:"broadcast_type"`
	FrequencyMHz     *float64         `json:"frequency_mhz"`
	PlaylistSource   *string          `json:"playlist_source"`
	PlaylistConfig   *json.RawMessage `json:"playlist_config,omitempty"`
	IsActive         *bool            `json:"is_active"`
}

// RadioStationDetailResponse represents the full radio station data returned to clients
type RadioStationDetailResponse struct {
	ID                  uint             `json:"id"`
	Name                string           `json:"name"`
	Slug                string           `json:"slug"`
	Description         *string          `json:"description"`
	City                *string          `json:"city"`
	State               *string          `json:"state"`
	Country             *string          `json:"country"`
	Timezone            *string          `json:"timezone"`
	StreamURL           *string          `json:"stream_url"`
	StreamURLs          *json.RawMessage `json:"stream_urls"`
	Website             *string          `json:"website"`
	DonationURL         *string          `json:"donation_url"`
	DonationEmbedURL    *string          `json:"donation_embed_url"`
	LogoURL             *string          `json:"logo_url"`
	Social              *json.RawMessage `json:"social"`
	BroadcastType       string           `json:"broadcast_type"`
	FrequencyMHz        *float64         `json:"frequency_mhz"`
	PlaylistSource      *string          `json:"playlist_source"`
	PlaylistConfig      *json.RawMessage `json:"playlist_config"`
	LastPlaylistFetchAt *time.Time       `json:"last_playlist_fetch_at"`
	IsActive            bool             `json:"is_active"`
	ShowCount           int              `json:"show_count"`
	CreatedAt           time.Time        `json:"created_at"`
	UpdatedAt           time.Time        `json:"updated_at"`
}

// RadioStationListResponse represents a radio station in list views
type RadioStationListResponse struct {
	ID            uint     `json:"id"`
	Name          string   `json:"name"`
	Slug          string   `json:"slug"`
	City          *string  `json:"city"`
	State         *string  `json:"state"`
	Country       *string  `json:"country"`
	BroadcastType string   `json:"broadcast_type"`
	FrequencyMHz  *float64 `json:"frequency_mhz"`
	LogoURL       *string  `json:"logo_url"`
	IsActive      bool     `json:"is_active"`
	ShowCount     int      `json:"show_count"`
}

// ──────────────────────────────────────────────
// Radio Show types
// ──────────────────────────────────────────────

// CreateRadioShowRequest represents the data needed to create a new radio show
type CreateRadioShowRequest struct {
	Name            string           `json:"name" validate:"required"`
	Slug            string           `json:"slug"`
	HostName        *string          `json:"host_name"`
	Description     *string          `json:"description"`
	ScheduleDisplay *string          `json:"schedule_display"`
	Schedule        *json.RawMessage `json:"schedule,omitempty"`
	GenreTags       *json.RawMessage `json:"genre_tags,omitempty"`
	ArchiveURL      *string          `json:"archive_url"`
	ImageURL        *string          `json:"image_url"`
	ExternalID      *string          `json:"external_id"`
}

// UpdateRadioShowRequest represents the data that can be updated on a radio show
type UpdateRadioShowRequest struct {
	Name            *string          `json:"name"`
	HostName        *string          `json:"host_name"`
	Description     *string          `json:"description"`
	ScheduleDisplay *string          `json:"schedule_display"`
	Schedule        *json.RawMessage `json:"schedule,omitempty"`
	GenreTags       *json.RawMessage `json:"genre_tags,omitempty"`
	ArchiveURL      *string          `json:"archive_url"`
	ImageURL        *string          `json:"image_url"`
	IsActive        *bool            `json:"is_active"`
}

// RadioShowDetailResponse represents the full radio show data returned to clients
type RadioShowDetailResponse struct {
	ID              uint             `json:"id"`
	StationID       uint             `json:"station_id"`
	StationName     string           `json:"station_name"`
	StationSlug     string           `json:"station_slug"`
	Name            string           `json:"name"`
	Slug            string           `json:"slug"`
	HostName        *string          `json:"host_name"`
	Description     *string          `json:"description"`
	ScheduleDisplay *string          `json:"schedule_display"`
	Schedule        *json.RawMessage `json:"schedule"`
	GenreTags       *json.RawMessage `json:"genre_tags"`
	ArchiveURL      *string          `json:"archive_url"`
	ImageURL        *string          `json:"image_url"`
	IsActive        bool             `json:"is_active"`
	EpisodeCount    int64            `json:"episode_count"`
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
}

// RadioShowListResponse represents a radio show in list views
type RadioShowListResponse struct {
	ID              uint             `json:"id"`
	StationID       uint             `json:"station_id"`
	StationName     string           `json:"station_name"`
	Name            string           `json:"name"`
	Slug            string           `json:"slug"`
	HostName        *string          `json:"host_name"`
	GenreTags       *json.RawMessage `json:"genre_tags"`
	ImageURL        *string          `json:"image_url"`
	IsActive        bool             `json:"is_active"`
	EpisodeCount    int64            `json:"episode_count"`
}

// ──────────────────────────────────────────────
// Radio Episode types
// ──────────────────────────────────────────────

// RadioEpisodeResponse represents a radio episode in list views
type RadioEpisodeResponse struct {
	ID              uint      `json:"id"`
	ShowID          uint      `json:"show_id"`
	Title           *string   `json:"title"`
	AirDate         string    `json:"air_date"`
	AirTime         *string   `json:"air_time"`
	DurationMinutes *int      `json:"duration_minutes"`
	ArchiveURL      *string   `json:"archive_url"`
	PlayCount       int       `json:"play_count"`
	CreatedAt       time.Time `json:"created_at"`
}

// RadioEpisodeDetailResponse represents the full radio episode data
type RadioEpisodeDetailResponse struct {
	ID              uint             `json:"id"`
	ShowID          uint             `json:"show_id"`
	ShowName        string           `json:"show_name"`
	ShowSlug        string           `json:"show_slug"`
	StationName     string           `json:"station_name"`
	StationSlug     string           `json:"station_slug"`
	Title           *string          `json:"title"`
	AirDate         string           `json:"air_date"`
	AirTime         *string          `json:"air_time"`
	DurationMinutes *int             `json:"duration_minutes"`
	Description     *string          `json:"description"`
	ArchiveURL      *string          `json:"archive_url"`
	MixcloudURL     *string          `json:"mixcloud_url"`
	GenreTags       *json.RawMessage `json:"genre_tags"`
	MoodTags        *json.RawMessage `json:"mood_tags"`
	PlayCount       int              `json:"play_count"`
	Plays           []RadioPlayResponse `json:"plays"`
	CreatedAt       time.Time        `json:"created_at"`
}

// ──────────────────────────────────────────────
// Radio Play types
// ──────────────────────────────────────────────

// RadioPlayResponse represents a single track played in a radio episode
type RadioPlayResponse struct {
	ID                     uint       `json:"id"`
	EpisodeID              uint       `json:"episode_id"`
	Position               int        `json:"position"`
	ArtistName             string     `json:"artist_name"`
	TrackTitle             *string    `json:"track_title"`
	AlbumTitle             *string    `json:"album_title"`
	LabelName              *string    `json:"label_name"`
	ReleaseYear            *int       `json:"release_year"`
	IsNew                  bool       `json:"is_new"`
	RotationStatus         *string    `json:"rotation_status"`
	DJComment              *string    `json:"dj_comment"`
	IsLivePerformance      bool       `json:"is_live_performance"`
	IsRequest              bool       `json:"is_request"`
	ArtistID               *uint      `json:"artist_id"`
	ArtistSlug             *string    `json:"artist_slug"`
	ReleaseID              *uint      `json:"release_id"`
	ReleaseSlug            *string    `json:"release_slug"`
	LabelID                *uint      `json:"label_id"`
	LabelSlug              *string    `json:"label_slug"`
	MusicBrainzArtistID    *string    `json:"musicbrainz_artist_id"`
	MusicBrainzRecordingID *string    `json:"musicbrainz_recording_id"`
	MusicBrainzReleaseID   *string    `json:"musicbrainz_release_id"`
	AirTimestamp           *time.Time `json:"air_timestamp"`
}

// ──────────────────────────────────────────────
// Aggregation / stats types
// ──────────────────────────────────────────────

// RadioTopArtistResponse represents a top-played artist for a show
type RadioTopArtistResponse struct {
	ArtistName   string  `json:"artist_name"`
	ArtistID     *uint   `json:"artist_id"`
	ArtistSlug   *string `json:"artist_slug"`
	PlayCount    int     `json:"play_count"`
	EpisodeCount int     `json:"episode_count"`
}

// RadioTopLabelResponse represents a top-featured label for a show
type RadioTopLabelResponse struct {
	LabelName string  `json:"label_name"`
	LabelID   *uint   `json:"label_id"`
	LabelSlug *string `json:"label_slug"`
	PlayCount int     `json:"play_count"`
}

// RadioAsHeardOnResponse represents a station/show where an entity was played
type RadioAsHeardOnResponse struct {
	StationID   uint   `json:"station_id"`
	StationName string `json:"station_name"`
	StationSlug string `json:"station_slug"`
	ShowID      uint   `json:"show_id"`
	ShowName    string `json:"show_name"`
	ShowSlug    string `json:"show_slug"`
	PlayCount   int    `json:"play_count"`
	LastPlayed  string `json:"last_played"`
}

// RadioNewReleaseRadarEntry represents a new release discovered across radio stations
type RadioNewReleaseRadarEntry struct {
	ArtistName  string  `json:"artist_name"`
	ArtistID    *uint   `json:"artist_id"`
	ArtistSlug  *string `json:"artist_slug"`
	AlbumTitle  *string `json:"album_title"`
	LabelName   *string `json:"label_name"`
	ReleaseID   *uint   `json:"release_id"`
	ReleaseSlug *string `json:"release_slug"`
	LabelID     *uint   `json:"label_id"`
	LabelSlug   *string `json:"label_slug"`
	PlayCount   int     `json:"play_count"`
	StationCount int    `json:"station_count"`
}

// RadioStatsResponse represents overall radio stats
type RadioStatsResponse struct {
	TotalStations int   `json:"total_stations"`
	TotalShows    int   `json:"total_shows"`
	TotalEpisodes int   `json:"total_episodes"`
	TotalPlays    int64 `json:"total_plays"`
	MatchedPlays  int64 `json:"matched_plays"`
	UniqueArtists int   `json:"unique_artists"`
}

// ──────────────────────────────────────────────
// Import pipeline types
// ──────────────────────────────────────────────

// RadioImportResult summarizes the result of a station or incremental import operation.
type RadioImportResult struct {
	ShowsDiscovered  int      `json:"shows_discovered"`
	EpisodesImported int      `json:"episodes_imported"`
	PlaysImported    int      `json:"plays_imported"`
	PlaysMatched     int      `json:"plays_matched"`
	Errors           []string `json:"errors,omitempty"`
}

// EpisodeImportResult summarizes the result of importing a single episode's playlist.
type EpisodeImportResult struct {
	PlaysImported int `json:"plays_imported"`
	PlaysMatched  int `json:"plays_matched"`
}

// MatchResult summarizes the result of running the matching engine.
type MatchResult struct {
	Total     int `json:"total"`
	Matched   int `json:"matched"`
	Unmatched int `json:"unmatched"`
}

// ──────────────────────────────────────────────
// Unmatched play management types
// ──────────────────────────────────────────────

// UnmatchedPlayGroup represents a group of unmatched plays by artist name.
type UnmatchedPlayGroup struct {
	ArtistName       string            `json:"artist_name"`
	PlayCount        int               `json:"play_count"`
	StationNames     []string          `json:"station_names"`
	SuggestedMatches []SuggestedMatch  `json:"suggested_matches"`
}

// SuggestedMatch represents a suggested artist match for unmatched plays.
type SuggestedMatch struct {
	ArtistID   uint   `json:"artist_id"`
	ArtistName string `json:"artist_name"`
	ArtistSlug string `json:"artist_slug"`
}

// LinkPlayRequest represents a request to link a play to entities.
type LinkPlayRequest struct {
	ArtistID  *uint `json:"artist_id"`
	ReleaseID *uint `json:"release_id"`
	LabelID   *uint `json:"label_id"`
}

// BulkLinkRequest represents a request to bulk-link all plays by artist_name to an artist.
type BulkLinkRequest struct {
	ArtistName string `json:"artist_name"`
	ArtistID   uint   `json:"artist_id"`
}

// BulkLinkResult summarizes the result of a bulk link operation.
type BulkLinkResult struct {
	Updated int `json:"updated"`
}

// RadioFetchCycleResult summarizes the result of a radio fetch cycle.
type RadioFetchCycleResult struct {
	StationsProcessed int      `json:"stations_processed"`
	EpisodesImported  int      `json:"episodes_imported"`
	PlaysImported     int      `json:"plays_imported"`
	PlaysMatched      int      `json:"plays_matched"`
	Failures          int      `json:"failures"`
	Errors            []string `json:"errors,omitempty"`
}

// SyncAffinityResult summarizes the result of syncing radio affinity data
// to artist relationships.
type SyncAffinityResult struct {
	Created int `json:"created"`
	Updated int `json:"updated"`
	Deleted int `json:"deleted"`
}
