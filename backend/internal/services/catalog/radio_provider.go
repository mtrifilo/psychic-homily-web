package catalog

import (
	"errors"
	"fmt"
	"net/http"
	"time"
)

// Sentinel errors for radio fetch classification (PSY-887). The fetch-service
// circuit breaker uses these via errors.Is / errors.As to distinguish transient
// errors (timeout, connection refused, 429) from permanent errors (5xx, parse
// failure, schema mismatch). Transient errors do NOT trip the breaker; they
// trigger a single in-cycle retry with brief backoff.
//
// Providers wrap their HTTP errors with RadioHTTPError so the classifier can
// inspect the status code without parsing error strings. Providers may also
// return ErrTransient / ErrPermanent directly when the cause is non-HTTP (e.g.
// a network timeout that doesn't make it through *url.Error).
var (
	// ErrTransient marks an error as recoverable on retry (timeout, conn refused, 429).
	// Joined via errors.Join so callers can detect it via errors.Is.
	ErrTransient = errors.New("transient radio provider error")

	// ErrPermanent marks an error as non-recoverable on retry (4xx other than 429,
	// 5xx, parse failure, schema mismatch). Joined via errors.Join so callers can
	// detect it via errors.Is.
	//
	// 5xx is classified as permanent (not transient) per the PSY-887 policy: a
	// sustained 5xx on a provider endpoint is a signal the breaker SHOULD catch —
	// either an outage worth alerting on or an API/schema drift that won't fix
	// itself on retry.
	ErrPermanent = errors.New("permanent radio provider error")
)

// RadioHTTPError wraps a non-2xx HTTP response from a radio provider so the
// fetch-service classifier can inspect the status code via errors.As. Providers
// SHOULD return this (via newRadioHTTPError) for any non-OK response — that
// lets the classifier route 429 to transient and 4xx/5xx to permanent without
// string-matching the error message.
type RadioHTTPError struct {
	Provider   string // "KEXP" / "WFMU" / "NTS"
	StatusCode int
	Body       string // truncated to radioHTTPErrorBodyLimit; included for debugging
}

// radioHTTPErrorBodyLimit caps the body slice retained on a RadioHTTPError so
// a provider that returns a multi-MB error page (e.g. an HTML 503 page from
// WFMU) doesn't pin that memory in every log line for the rest of the cycle.
// 512 bytes is enough to capture the "error reason" intro most providers ship.
const radioHTTPErrorBodyLimit = 512

func (e *RadioHTTPError) Error() string {
	return fmt.Sprintf("%s returned status %d: %s", e.Provider, e.StatusCode, e.Body)
}

// Unwrap returns ErrTransient for 429, ErrPermanent for any other non-2xx so
// errors.Is(err, ErrTransient) / errors.Is(err, ErrPermanent) just work.
func (e *RadioHTTPError) Unwrap() error {
	if e.StatusCode == http.StatusTooManyRequests {
		return ErrTransient
	}
	return ErrPermanent
}

// newRadioHTTPError builds a RadioHTTPError for use by provider doGet helpers.
// Body is truncated to radioHTTPErrorBodyLimit to keep error chains compact —
// providers SHOULD NOT pre-truncate; this helper does it centrally.
func newRadioHTTPError(provider string, statusCode int, body string) *RadioHTTPError {
	if len(body) > radioHTTPErrorBodyLimit {
		body = body[:radioHTTPErrorBodyLimit] + "...[truncated]"
	}
	return &RadioHTTPError{Provider: provider, StatusCode: statusCode, Body: body}
}

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
