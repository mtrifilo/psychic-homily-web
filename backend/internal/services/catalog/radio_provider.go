package catalog

import (
	"time"
)

// RadioPlaylistProvider is the interface all radio providers must implement.
// Each provider knows how to discover shows, fetch episodes, and fetch playlists
// from a specific radio station's API or data source.
type RadioPlaylistProvider interface {
	// DiscoverShows returns all available shows/programs from the station.
	DiscoverShows() ([]RadioShowImport, error)

	// FetchNewEpisodes returns episodes for a given show within [since, until].
	// A zero until means no upper bound.
	FetchNewEpisodes(showExternalID string, since time.Time, until time.Time) ([]RadioEpisodeImport, error)

	// FetchPlaylist returns the track plays for a specific episode.
	FetchPlaylist(episodeExternalID string) ([]RadioPlayImport, error)
}

// RadioShowImport is the intermediate DTO for importing a radio show from a provider.
type RadioShowImport struct {
	ExternalID  string  `json:"external_id"`
	Name        string  `json:"name"`
	HostName    *string `json:"host_name,omitempty"`
	Description *string `json:"description,omitempty"`
	ImageURL    *string `json:"image_url,omitempty"`
	ArchiveURL  *string `json:"archive_url,omitempty"`
}

// RadioEpisodeImport is the intermediate DTO for importing a radio episode from a provider.
type RadioEpisodeImport struct {
	ExternalID      string  `json:"external_id"`
	ShowExternalID  string  `json:"show_external_id"`
	Title           *string `json:"title,omitempty"`
	AirDate         string  `json:"air_date"` // YYYY-MM-DD
	AirTime         *string `json:"air_time,omitempty"`
	DurationMinutes *int    `json:"duration_minutes,omitempty"`
	ArchiveURL      *string `json:"archive_url,omitempty"`
}

// RadioPlayImport is the intermediate DTO for importing a track play from a provider.
type RadioPlayImport struct {
	Position               int        `json:"position"`
	ArtistName             string     `json:"artist_name"`
	TrackTitle             *string    `json:"track_title,omitempty"`
	AlbumTitle             *string    `json:"album_title,omitempty"`
	LabelName              *string    `json:"label_name,omitempty"`
	ReleaseYear            *int       `json:"release_year,omitempty"`
	IsNew                  bool       `json:"is_new"`
	RotationStatus         *string    `json:"rotation_status,omitempty"`
	DJComment              *string    `json:"dj_comment,omitempty"`
	IsLivePerformance      bool       `json:"is_live_performance"`
	IsRequest              bool       `json:"is_request"`
	MusicBrainzArtistID    *string    `json:"musicbrainz_artist_id,omitempty"`
	MusicBrainzRecordingID *string    `json:"musicbrainz_recording_id,omitempty"`
	MusicBrainzReleaseID   *string    `json:"musicbrainz_release_id,omitempty"`
	AirTimestamp           *time.Time `json:"air_timestamp,omitempty"`
}
