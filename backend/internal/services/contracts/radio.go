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

// RadioNetworkInfo is the per-station view of its parent network, embedded
// in RadioStationDetailResponse and RadioStationListResponse. `is_flagship`
// is the bool on the *station* (radio_stations.is_flagship) — true means
// THIS station is the network's primary/default broadcast. Frontend uses
// it to render WFMU 91.1 as the default tab and the 3 stream-only siblings
// as secondary tabs.
type RadioNetworkInfo struct {
	Slug       string `json:"slug"`
	Name       string `json:"name"`
	IsFlagship bool   `json:"is_flagship"`
}

// RadioSiblingStationResponse is a sibling station within the same network,
// embedded in RadioStationDetailResponse.SiblingStations. Includes every
// station in the network OTHER than the one this response represents, so a
// tab bar can render the full set with the active tab highlighted.
type RadioSiblingStationResponse struct {
	ID            uint     `json:"id"`
	Slug          string   `json:"slug"`
	Name          string   `json:"name"`
	BroadcastType string   `json:"broadcast_type"`
	FrequencyMHz  *float64 `json:"frequency_mhz"`
	IsFlagship    bool     `json:"is_flagship"`
}

// RadioStationDetailResponse represents the full radio station data returned to clients
type RadioStationDetailResponse struct {
	ID                  uint                          `json:"id"`
	Name                string                        `json:"name"`
	Slug                string                        `json:"slug"`
	Description         *string                       `json:"description"`
	City                *string                       `json:"city"`
	State               *string                       `json:"state"`
	Country             *string                       `json:"country"`
	Timezone            *string                       `json:"timezone"`
	StreamURL           *string                       `json:"stream_url"`
	StreamURLs          *json.RawMessage              `json:"stream_urls"`
	Website             *string                       `json:"website"`
	DonationURL         *string                       `json:"donation_url"`
	DonationEmbedURL    *string                       `json:"donation_embed_url"`
	LogoURL             *string                       `json:"logo_url"`
	Social              *json.RawMessage              `json:"social"`
	BroadcastType       string                        `json:"broadcast_type"`
	FrequencyMHz        *float64                      `json:"frequency_mhz"`
	PlaylistSource      *string                       `json:"playlist_source"`
	PlaylistConfig      *json.RawMessage              `json:"playlist_config"`
	LastPlaylistFetchAt *time.Time                    `json:"last_playlist_fetch_at"`
	IsActive            bool                          `json:"is_active"`
	NetworkID           *uint                         `json:"network_id"`
	NetworkSlug         *string                       `json:"network_slug"`
	Network             *RadioNetworkInfo             `json:"network"`
	SiblingStations     []RadioSiblingStationResponse `json:"sibling_stations"`
	ShowCount           int                           `json:"show_count"`
	CreatedAt           time.Time                     `json:"created_at"`
	UpdatedAt           time.Time                     `json:"updated_at"`
}

// RadioStationListResponse represents a radio station in list views
type RadioStationListResponse struct {
	ID              uint                          `json:"id"`
	Name            string                        `json:"name"`
	Slug            string                        `json:"slug"`
	City            *string                       `json:"city"`
	State           *string                       `json:"state"`
	Country         *string                       `json:"country"`
	BroadcastType   string                        `json:"broadcast_type"`
	FrequencyMHz    *float64                      `json:"frequency_mhz"`
	LogoURL         *string                       `json:"logo_url"`
	IsActive        bool                          `json:"is_active"`
	NetworkID       *uint                         `json:"network_id"`
	NetworkSlug     *string                       `json:"network_slug"`
	Network         *RadioNetworkInfo             `json:"network"`
	SiblingStations []RadioSiblingStationResponse `json:"sibling_stations"`
	ShowCount       int                           `json:"show_count"`
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
	ID           uint             `json:"id"`
	StationID    uint             `json:"station_id"`
	StationName  string           `json:"station_name"`
	Name         string           `json:"name"`
	Slug         string           `json:"slug"`
	HostName     *string          `json:"host_name"`
	// ScheduleDisplay is the human-readable air slot ("Mon 9pm-12am"),
	// surfaced in list rows for the station-page shows directory (PSY-1050).
	ScheduleDisplay *string          `json:"schedule_display"`
	GenreTags       *json.RawMessage `json:"genre_tags"`
	ImageURL        *string          `json:"image_url"`
	IsActive        bool             `json:"is_active"`
	EpisodeCount    int64            `json:"episode_count"`
	// LatestAirDate is the air date (YYYY-MM-DD) of the show's most recent
	// episode, nil when the show has no episodes (PSY-1048).
	LatestAirDate *string `json:"latest_air_date"`
}

// ──────────────────────────────────────────────
// Radio Episode types
// ──────────────────────────────────────────────

// RadioEpisodePreviewArtist is one artist in an episode row's short
// "played" preview — raw name plus knowledge-graph link when matched (PSY-1048).
type RadioEpisodePreviewArtist struct {
	ArtistName string  `json:"artist_name"`
	ArtistID   *uint   `json:"artist_id"`
	ArtistSlug *string `json:"artist_slug"`
}

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
	// ArtistPreview holds the first few distinct artists from the episode's
	// playlist, in play order (PSY-1048).
	ArtistPreview []RadioEpisodePreviewArtist `json:"artist_preview"`
}

// RadioStationEpisodeRow is an episode row in the station-scoped and
// dial-wide latest-playlists feeds: episode fields plus show and channel
// (station) attribution (PSY-1048).
type RadioStationEpisodeRow struct {
	ID            uint                        `json:"id"`
	Title         *string                     `json:"title"`
	AirDate       string                      `json:"air_date"`
	PlayCount     int                         `json:"play_count"`
	ArchiveURL    *string                     `json:"archive_url"`
	ShowID        uint                        `json:"show_id"`
	ShowName      string                      `json:"show_name"`
	ShowSlug      string                      `json:"show_slug"`
	StationID     uint                        `json:"station_id"`
	StationName   string                      `json:"station_name"`
	StationSlug   string                      `json:"station_slug"`
	ArtistPreview []RadioEpisodePreviewArtist `json:"artist_preview"`
}

// RadioEpisodeDetailResponse represents the full radio episode data
type RadioEpisodeDetailResponse struct {
	ID              uint                `json:"id"`
	ShowID          uint                `json:"show_id"`
	ShowName        string              `json:"show_name"`
	ShowSlug        string              `json:"show_slug"`
	StationName     string              `json:"station_name"`
	StationSlug     string              `json:"station_slug"`
	Title           *string             `json:"title"`
	AirDate         string              `json:"air_date"`
	AirTime         *string             `json:"air_time"`
	DurationMinutes *int                `json:"duration_minutes"`
	Description     *string             `json:"description"`
	ArchiveURL      *string             `json:"archive_url"`
	MixcloudURL     *string             `json:"mixcloud_url"`
	GenreTags       *json.RawMessage    `json:"genre_tags"`
	MoodTags        *json.RawMessage    `json:"mood_tags"`
	PlayCount       int                 `json:"play_count"`
	Plays           []RadioPlayResponse `json:"plays"`
	CreatedAt       time.Time           `json:"created_at"`
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
// Now-playing types (PSY-1022)
// ──────────────────────────────────────────────

// Now-playing source discriminators. "live" means the payload came from the
// station's provider live API (KEXP plays, NTS live, WFMU current-shows);
// "latest_archive" means the provider has no live source (or it failed) and
// the payload is the v1 heuristic — the most-active show's latest archived
// episode.
const (
	NowPlayingSourceLive          = "live"
	NowPlayingSourceLatestArchive = "latest_archive"
)

// RadioNowPlayingShowRef is the matched our-DB show behind a now-playing
// payload. Nil on the response when the live show name couldn't be matched
// to exactly one of the station's shows (PSY-1073: WFMU's catalog is
// duplicated across channel stations, so matching is scoped to the requested
// station and ambiguity yields nil rather than a wrong link).
type RadioNowPlayingShowRef struct {
	ID       uint    `json:"id"`
	Name     string  `json:"name"`
	Slug     string  `json:"slug"`
	HostName *string `json:"host_name"`
}

// RadioNowPlayingTrack is the current track of a now-playing payload. Field
// names mirror RadioPlayResponse (minus the persistence-only id/episode_id/
// position) so frontend track renderers work on both shapes.
type RadioNowPlayingTrack struct {
	ArtistName     string  `json:"artist_name"`
	TrackTitle     *string `json:"track_title"`
	AlbumTitle     *string `json:"album_title"`
	LabelName      *string `json:"label_name"`
	ReleaseYear    *int    `json:"release_year"`
	RotationStatus *string `json:"rotation_status"`
	DJComment      *string `json:"dj_comment"`
	ArtistID       *uint   `json:"artist_id"`
	ArtistSlug     *string `json:"artist_slug"`
	ReleaseID      *uint   `json:"release_id"`
	ReleaseSlug    *string `json:"release_slug"`
	LabelID        *uint   `json:"label_id"`
	LabelSlug      *string `json:"label_slug"`
}

// RadioNowPlayingResponse is the GET /radio-stations/{slug}/now-playing
// payload (PSY-1022).
//
// Invariant: Source == "live" implies OnAir == true — adapters that find no
// active broadcast yield the archive fallback instead of a half-live payload,
// so consumers can key the ON AIR treatment on either field consistently.
type RadioNowPlayingResponse struct {
	Source string `json:"source" enum:"live,latest_archive" doc:"Where this payload came from: the provider's live API, or the latest-archive fallback"`
	OnAir  bool   `json:"on_air" doc:"True only when a live source confirmed an active broadcast"`
	// Show is the matched our-DB show; nil when the live show name/external-id
	// couldn't be matched unambiguously. ShowName always carries the raw name
	// (live: as reported by the provider; archive: the DB show's name) so the
	// UI can render unmatched shows as plain text instead of a dead link.
	Show     *RadioNowPlayingShowRef `json:"show"`
	ShowName *string                 `json:"show_name"`
	// HostName is the live-reported host (e.g. WFMU's "... with Jody Peyote"),
	// set even when the show itself didn't match; nil for archive payloads
	// (use Show.HostName there).
	HostName     *string               `json:"host_name"`
	CurrentTrack *RadioNowPlayingTrack `json:"current_track"`
	// RecentArtists is up to 4 distinct previously-played artists (most recent
	// first), from the live source when it carries a play history (KEXP), else
	// from the fallback episode's playlist.
	RecentArtists []RadioEpisodePreviewArtist `json:"recent_artists"`
	// EpisodeAirDate (YYYY-MM-DD) is the air date of the archived episode the
	// payload was derived from; nil for live payloads.
	EpisodeAirDate *string `json:"episode_air_date"`
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
	ArtistName   string  `json:"artist_name"`
	ArtistID     *uint   `json:"artist_id"`
	ArtistSlug   *string `json:"artist_slug"`
	AlbumTitle   *string `json:"album_title"`
	LabelName    *string `json:"label_name"`
	ReleaseID    *uint   `json:"release_id"`
	ReleaseSlug  *string `json:"release_slug"`
	LabelID      *uint   `json:"label_id"`
	LabelSlug    *string `json:"label_slug"`
	PlayCount    int     `json:"play_count"`
	StationCount int     `json:"station_count"`
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

// RadioDiscoverResult summarizes the result of discovering shows for a station.
// ShowsDiscovered + ShowNames count every show the provider returned
// (idempotent upserts included). ShowsNew + NewShowNames count only the rows
// that didn't previously exist — callers use this delta to drive notifications
// on actually-new shows, not on every cycle. NewShowIDs is parallel to
// NewShowNames (same length, same order); the discover orchestrator uses the
// IDs to enqueue auto-backfill import jobs.
type RadioDiscoverResult struct {
	ShowsDiscovered int      `json:"shows_discovered"`
	ShowNames       []string `json:"show_names"`
	ShowsNew        int      `json:"shows_new"`
	NewShowNames    []string `json:"new_show_names"`
	NewShowIDs      []uint   `json:"new_show_ids"`
	Errors          []string `json:"errors,omitempty"`
}

// EpisodeImportResult summarizes the result of importing a single episode's playlist.
//
// DropSummary, when non-empty, is a single-line per-episode aggregate of plays
// that were dropped or truncated at the import boundary (PSY-885). Format:
// "dropped N plays: X over-length titles truncated, Y missing artist_name".
// The summary is also bubbled up to RadioImportResult.Errors by the batch
// orchestrators so partial-import outcomes are visible in admin job logs
// without ballooning the field with per-play entries.
type EpisodeImportResult struct {
	PlaysImported int    `json:"plays_imported"`
	PlaysMatched  int    `json:"plays_matched"`
	DropSummary   string `json:"drop_summary,omitempty"`
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
	ArtistName       string           `json:"artist_name"`
	PlayCount        int              `json:"play_count"`
	StationNames     []string         `json:"station_names"`
	SuggestedMatches []SuggestedMatch `json:"suggested_matches"`
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

// ──────────────────────────────────────────────
// Import job types
// ──────────────────────────────────────────────

// RadioImportJobResponse is the DTO for a radio import job.
type RadioImportJobResponse struct {
	ID                 uint       `json:"id"`
	ShowID             uint       `json:"show_id"`
	ShowName           string     `json:"show_name"`
	StationID          uint       `json:"station_id"`
	StationName        string     `json:"station_name"`
	Since              string     `json:"since"`
	Until              string     `json:"until"`
	Status             string     `json:"status"`
	EpisodesFound      int        `json:"episodes_found"`
	EpisodesImported   int        `json:"episodes_imported"`
	PlaysImported      int        `json:"plays_imported"`
	PlaysMatched       int        `json:"plays_matched"`
	CurrentEpisodeDate *string    `json:"current_episode_date,omitempty"`
	ErrorLog           *string    `json:"error_log,omitempty"`
	StartedAt          *time.Time `json:"started_at,omitempty"`
	CompletedAt        *time.Time `json:"completed_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// SyncAffinityResult summarizes the result of syncing radio affinity data
// to artist relationships.
type SyncAffinityResult struct {
	Created int `json:"created"`
	Updated int `json:"updated"`
	Deleted int `json:"deleted"`
	// Failed counts per-row writes that were logged and skipped; the sync
	// continues past individual failures rather than aborting the batch.
	Failed int `json:"failed"`
}

// ──────────────────────────────────────────────
// Radio Service Interface
// ──────────────────────────────────────────────

// RadioServiceInterface defines the contract for radio station, show, episode, and play operations.
type RadioServiceInterface interface {
	// Station CRUD
	CreateStation(req *CreateRadioStationRequest) (*RadioStationDetailResponse, error)
	GetStation(stationID uint) (*RadioStationDetailResponse, error)
	GetStationBySlug(slug string) (*RadioStationDetailResponse, error)
	ResolveStationIDBySlug(slug string) (uint, error)
	ListStations(filters map[string]interface{}) ([]*RadioStationListResponse, error)
	UpdateStation(stationID uint, req *UpdateRadioStationRequest) (*RadioStationDetailResponse, error)
	DeleteStation(stationID uint) error

	// Show CRUD
	CreateShow(stationID uint, req *CreateRadioShowRequest) (*RadioShowDetailResponse, error)
	GetShow(showID uint) (*RadioShowDetailResponse, error)
	GetShowBySlug(slug string) (*RadioShowDetailResponse, error)
	ListShows(stationID uint, sortBy string) ([]*RadioShowListResponse, error)
	UpdateShow(showID uint, req *UpdateRadioShowRequest) (*RadioShowDetailResponse, error)
	DeleteShow(showID uint) error

	// Episodes
	GetEpisodes(showID uint, limit, offset int) ([]*RadioEpisodeResponse, int64, error)
	GetEpisodeByShowAndDate(showID uint, airDate string) (*RadioEpisodeDetailResponse, error)
	GetEpisodeDetail(episodeID uint) (*RadioEpisodeDetailResponse, error)
	GetStationEpisodes(stationID uint, limit, offset int) ([]*RadioStationEpisodeRow, int64, error)
	GetRecentEpisodes(limit, offset int) ([]*RadioStationEpisodeRow, int64, error)

	// Now-playing (PSY-1022)
	GetStationNowPlaying(stationID uint) (*RadioNowPlayingResponse, error)

	// Aggregation queries
	GetTopArtistsForShow(showID uint, periodDays, limit int) ([]*RadioTopArtistResponse, error)
	GetTopLabelsForShow(showID uint, periodDays, limit int) ([]*RadioTopLabelResponse, error)
	GetTopArtistsForStation(stationID uint, periodDays, limit int) ([]*RadioTopArtistResponse, error)
	GetTopLabelsForStation(stationID uint, periodDays, limit int) ([]*RadioTopLabelResponse, error)
	GetAsHeardOnForArtist(artistID uint) ([]*RadioAsHeardOnResponse, error)
	GetAsHeardOnForRelease(releaseID uint) ([]*RadioAsHeardOnResponse, error)
	GetNewReleaseRadar(stationID uint, limit int) ([]*RadioNewReleaseRadarEntry, error)

	// Stats
	GetRadioStats() (*RadioStatsResponse, error)

	// Import pipeline
	ImportStation(stationID uint, backfillDays int) (*RadioImportResult, error)
	FetchNewEpisodes(stationID uint) (*RadioImportResult, error)
	ImportEpisodePlaylist(showID uint, episodeExternalID string) (*EpisodeImportResult, error)
	DiscoverStationShows(stationID uint) (*RadioDiscoverResult, error)
	ImportShowEpisodes(showID uint, since string, until string) (*RadioImportResult, error)

	// Matching
	MatchPlays(episodeID uint) (*MatchResult, error)

	// Unmatched play management
	GetUnmatchedPlays(stationID uint, limit, offset int) ([]*UnmatchedPlayGroup, int64, error)
	LinkPlay(playID uint, req *LinkPlayRequest) error
	BulkLinkPlays(req *BulkLinkRequest) (*BulkLinkResult, error)

	// Affinity
	ComputeAffinity() error
	SyncAffinityToRelationships() (*SyncAffinityResult, error)

	// Re-matching
	ReMatchUnmatched() (*MatchResult, error)

	// Import jobs
	CreateImportJob(showID uint, since, until string) (*RadioImportJobResponse, error)
	StartImportJob(jobID uint) error
	CancelImportJob(jobID uint) error
	GetImportJob(jobID uint) (*RadioImportJobResponse, error)
	ListImportJobs(showID uint) ([]*RadioImportJobResponse, error)
}
