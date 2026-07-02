package catalog

import (
	"context"
	"errors"
	"fmt"
	"io"
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
	// A zero until means no upper bound. Provider-specific exception: the WFMU
	// provider additionally caps `until` at today (WFMU-local) so it never returns
	// future-dated rows — WFMU pre-publishes upcoming-broadcast pages that would
	// otherwise import as 0-track placeholders (PSY-1240). KEXP/NTS do not cap.
	FetchNewEpisodes(showExternalID string, since time.Time, until time.Time) ([]RadioEpisodeImport, error)

	// FetchPlaylist returns the track plays for a specific episode.
	FetchPlaylist(episodeExternalID string) ([]RadioPlayImport, error)
}

// ExhaustiveEpisodeLister is an optional provider capability: implementing it
// asserts that FetchNewEpisodes returns EVERY episode the provider currently
// publishes for the show within [since, until] — not a paginated/filtered
// subset. Under that guarantee, a stored episode inside the window that is
// absent from the fetch result has been retracted upstream, which authorizes
// the retraction reconcile (PSY-1286) to delete its placeholder row. Providers
// that page or filter their listings (KEXP, NTS) must NOT implement this —
// absence from a partial listing means nothing.
//
// WFMU qualifies because its fetch scrapes the show's full archive page, the
// same source that stops listing a playlist the moment WFMU deletes it.
type ExhaustiveEpisodeLister interface {
	// EpisodeListingIsExhaustive reports whether the provider's fetch result
	// is a complete listing of the window (see interface doc).
	EpisodeListingIsExhaustive() bool
}

const (
	// radioLiveFetchTimeout bounds each live now-playing HTTP call. Live
	// fetches sit (behind the TTL cache) on a page-view path, so they get a
	// tight per-request budget instead of the 30s import-pipeline timeout.
	radioLiveFetchTimeout = 3 * time.Second

	// radioLiveBodyLimit caps a live-response body read. The largest real
	// payload (NTS /v2/live) is ~25KB; 2MB leaves generous headroom while
	// preventing a misbehaving provider from pinning unbounded memory.
	radioLiveBodyLimit = 2 << 20
)

// radioLiveGet performs a time-boxed, size-capped GET for live now-playing
// fetches. The URL must be built from in-code constants (never DB or user
// input — SSRF guard); callers pass their provider's httpClient so tests can
// point at httptest servers via the *WithClient constructors.
func radioLiveGet(client *http.Client, url, userAgent, providerName string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), radioLiveFetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // deferred Close; nothing actionable on failure

	body, err := io.ReadAll(io.LimitReader(resp.Body, radioLiveBodyLimit))
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, newRadioHTTPError(providerName, resp.StatusCode, string(body))
	}
	return body, nil
}

// RadioLiveProvider is the optional live now-playing extension of a radio
// provider (PSY-1022). Providers with a queryable live source (KEXP plays
// API, NTS live API, WFMU current-live-shows aggregator) implement it; the
// now-playing service type-asserts from RadioPlaylistProvider and falls back
// to the latest-archive payload when the assertion fails.
//
// channel selects the stream for multi-stream providers (NTS "1"/"2"; WFMU
// stream keys — see wfmuLiveChannel* constants). Single-stream providers
// (KEXP) ignore it. Channel values come from the in-code station routing
// table (liveChannelForStation), never from user input.
type RadioLiveProvider interface {
	// FetchLiveNowPlaying returns the channel's current broadcast, or
	// (nil, nil) when the provider answered but reports no active live
	// broadcast for the channel. Implementations must time-box their HTTP
	// calls (seconds, not the 30s import timeout) — this sits on a page-view
	// path, behind a TTL cache.
	FetchLiveNowPlaying(channel string) (*RadioLiveNowPlaying, error)
}

// RadioLiveNowPlaying is what a live adapter reports for one channel.
type RadioLiveNowPlaying struct {
	// ShowName is the provider-reported name of the show on air (required).
	ShowName string
	// ShowExternalID, when the live source carries it, matches
	// radio_shows.external_id (KEXP program id, WFMU program code, NTS show
	// alias) — a stronger match key than the name.
	ShowExternalID *string
	// HostName is the provider-reported host, when the source separates it
	// from the show name.
	HostName *string
	// CurrentTrack is the track on air right now; nil when the source is
	// show-level only (NTS) or the station is between tracks (KEXP airbreak).
	CurrentTrack *RadioPlayImport
	// RecentTracks are the tracks played just before CurrentTrack, most
	// recent first, when the live source carries a play history (KEXP).
	RecentTracks []RadioPlayImport
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
	// StartsAt/EndsAt are the episode's real air window as instants (PSY-1152),
	// FROZEN at ingest from whatever the provider supplies — KEXP/NTS carry
	// RFC3339 timestamps; WFMU has none until the schedule lands (PSY-1159). The
	// episode-lifecycle status (live/aired/...) is computed from this window,
	// never re-derived from a show's current schedule (which churns seasonally).
	StartsAt *time.Time `json:"starts_at,omitempty"`
	EndsAt   *time.Time `json:"ends_at,omitempty"`
}

// RadioPlayImport is the intermediate DTO for importing a track play from a provider.
type RadioPlayImport struct {
	Position int `json:"position"`
	// ProviderPlayID is a stable, provider-supplied play identifier (KEXP sets it
	// from the play `id`). When non-nil it becomes the row's generated dedup_key,
	// making re-imports idempotent even if positions shift across fetches. NTS and
	// WFMU have no such id and leave it nil, so dedup_key falls back to the
	// content hash over (position, artist_name, track_title). It must be either
	// nil or a non-empty string — an empty value would make dedup_key COALESCE to
	// '' and collide every such play in the episode (sanitizePlay enforces this).
	//
	// A future non-KEXP provider must normalize its id at its own boundary: it
	// shares the dedup_key namespace with the 32-char md5 content hash (no
	// separator) and the column is VARCHAR(255), so an id that is 32-hex-shaped or
	// longer than 255 chars must be namespaced/length-checked before reaching here.
	ProviderPlayID         *string    `json:"provider_play_id,omitempty"`
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
