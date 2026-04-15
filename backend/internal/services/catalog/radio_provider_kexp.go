package catalog

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	kexpBaseURL        = "https://api.kexp.org"
	kexpUserAgent      = "PsychicHomily/1.0 (radio-playlist-indexer)"
	kexpDefaultTimeout = 30 * time.Second
	kexpRateLimit      = 1 * time.Second
)

// KEXPProvider implements RadioPlaylistProvider for KEXP's v2 REST API.
type KEXPProvider struct {
	httpClient  *http.Client
	baseURL     string
	rateLimiter *time.Ticker
}

// NewKEXPProvider creates a new KEXP provider with rate limiting.
func NewKEXPProvider() *KEXPProvider {
	return &KEXPProvider{
		httpClient: &http.Client{
			Timeout: kexpDefaultTimeout,
		},
		baseURL:     kexpBaseURL,
		rateLimiter: time.NewTicker(kexpRateLimit),
	}
}

// NewKEXPProviderWithClient creates a KEXP provider with a custom HTTP client and base URL.
// Exported for testing with httptest servers.
func NewKEXPProviderWithClient(client *http.Client, baseURL string) *KEXPProvider {
	return &KEXPProvider{
		httpClient:  client,
		baseURL:     baseURL,
		rateLimiter: time.NewTicker(1 * time.Millisecond), // fast for tests
	}
}

// Close stops the rate limiter ticker. Should be called when the provider is no longer needed.
func (p *KEXPProvider) Close() {
	if p.rateLimiter != nil {
		p.rateLimiter.Stop()
	}
}

// DiscoverShows returns all KEXP programs (shows).
func (p *KEXPProvider) DiscoverShows() ([]RadioShowImport, error) {
	var allShows []RadioShowImport

	// Fetch all hosts for name mapping
	hosts, err := p.fetchAllHosts()
	if err != nil {
		// Non-fatal: host names will be nil
		hosts = make(map[int]string)
	}

	url := fmt.Sprintf("%s/v2/programs/?limit=100", p.baseURL)
	for url != "" {
		<-p.rateLimiter.C

		resp, err := p.doGet(url)
		if err != nil {
			return nil, fmt.Errorf("fetching programs: %w", err)
		}

		var page kexpProgramsResponse
		if err := json.Unmarshal(resp, &page); err != nil {
			return nil, fmt.Errorf("parsing programs response: %w", err)
		}

		for _, prog := range page.Results {
			show := RadioShowImport{
				ExternalID: strconv.Itoa(prog.ID),
				Name:       prog.Name,
			}
			if prog.Description != "" {
				desc := prog.Description
				show.Description = &desc
			}
			if prog.ImageURI != "" {
				img := prog.ImageURI
				show.ImageURL = &img
			}
			// Map host IDs to names
			if len(prog.HostIDs) > 0 {
				var hostNames []string
				for _, hid := range prog.HostIDs {
					if name, ok := hosts[hid]; ok {
						hostNames = append(hostNames, name)
					}
				}
				if len(hostNames) > 0 {
					joined := strings.Join(hostNames, ", ")
					show.HostName = &joined
				}
			}

			// Build archive URL from program name.
			// KEXP website uses /shows/{Name-With-Hyphens}/ (case-sensitive).
			archiveURL := fmt.Sprintf("https://www.kexp.org/shows/%s/",
				strings.ReplaceAll(prog.Name, " ", "-"))
			show.ArchiveURL = &archiveURL

			allShows = append(allShows, show)
		}

		url = page.Next
	}

	return allShows, nil
}

// FetchNewEpisodes returns KEXP "shows" (broadcasts) for a program within [since, until].
// A zero until means no upper bound.
func (p *KEXPProvider) FetchNewEpisodes(showExternalID string, since time.Time, until time.Time) ([]RadioEpisodeImport, error) {
	var allEpisodes []RadioEpisodeImport

	sinceStr := since.UTC().Format(time.RFC3339)
	url := fmt.Sprintf("%s/v2/shows/?program_id=%s&start_time_after=%s&limit=100&ordering=start_time",
		p.baseURL, showExternalID, sinceStr)

	if !until.IsZero() {
		untilStr := until.UTC().Format(time.RFC3339)
		url += "&start_time_before=" + untilStr
	}

	for url != "" {
		<-p.rateLimiter.C

		resp, err := p.doGet(url)
		if err != nil {
			return nil, fmt.Errorf("fetching episodes: %w", err)
		}

		var page kexpShowsResponse
		if err := json.Unmarshal(resp, &page); err != nil {
			return nil, fmt.Errorf("parsing shows response: %w", err)
		}

		for _, show := range page.Results {
			ep := parseKEXPEpisode(show, showExternalID)
			allEpisodes = append(allEpisodes, ep)
		}

		url = page.Next
	}

	return allEpisodes, nil
}

// kexpPlaylistWindowFallback is added to a broadcast's start_time when the
// show detail response does not include an end_time. Programs are typically
// 1-4 hours long, so 5 hours gives a safety buffer without encroaching far
// into the next broadcast's playlist.
const kexpPlaylistWindowFallback = 5 * time.Hour

// FetchPlaylist returns track plays for a KEXP "show" (episode).
//
// KEXP's /v2/plays endpoint does NOT support a show_id filter -- passing one is
// silently ignored and every request returns the global plays list. Instead we
// filter by broadcast time window using airdate_after/airdate_before:
//
//  1. GET /v2/shows/{id}/ to resolve the broadcast's start_time and end_time.
//  2. Use [start_time, end_time] as the bounds. If end_time is missing, fall
//     back to start_time + kexpPlaylistWindowFallback.
//  3. GET /v2/plays/?airdate_after=...&airdate_before=...&play_type=trackplay
//     and paginate via the `next` cursor.
//
// If the broadcast is not found (404) we return an empty playlist rather than
// an error so callers can continue processing other episodes.
func (p *KEXPProvider) FetchPlaylist(episodeExternalID string) ([]RadioPlayImport, error) {
	// Step 1: fetch the broadcast to get its time window.
	showDetailURL := fmt.Sprintf("%s/v2/shows/%s/", p.baseURL, episodeExternalID)

	<-p.rateLimiter.C
	resp, err := p.doGet(showDetailURL)
	if err != nil {
		// KEXP returned a non-200 (including 404 for missing broadcasts) --
		// treat as "no plays" so the import pipeline can continue.
		if strings.Contains(err.Error(), "status 404") {
			return nil, nil
		}
		return nil, fmt.Errorf("fetching show detail: %w", err)
	}

	var show kexpShow
	if err := json.Unmarshal(resp, &show); err != nil {
		return nil, fmt.Errorf("parsing show detail response: %w", err)
	}

	if show.StartTime == "" {
		return nil, nil
	}

	startTime, err := time.Parse(time.RFC3339, show.StartTime)
	if err != nil {
		return nil, fmt.Errorf("parsing show start_time %q: %w", show.StartTime, err)
	}

	// Use end_time from the broadcast when available for a precise window;
	// fall back to the fixed fallback duration when the API omits it.
	var endTime time.Time
	if show.EndTime != "" {
		parsed, err := time.Parse(time.RFC3339, show.EndTime)
		if err == nil && parsed.After(startTime) {
			endTime = parsed
		}
	}
	if endTime.IsZero() {
		endTime = startTime.Add(kexpPlaylistWindowFallback)
	}

	// Step 2: fetch plays filtered by the broadcast's time window.
	var allPlays []RadioPlayImport
	position := 0

	url := fmt.Sprintf("%s/v2/plays/?airdate_after=%s&airdate_before=%s&play_type=trackplay&limit=100&ordering=airdate",
		p.baseURL,
		startTime.UTC().Format(time.RFC3339),
		endTime.UTC().Format(time.RFC3339),
	)

	for url != "" {
		<-p.rateLimiter.C

		resp, err := p.doGet(url)
		if err != nil {
			return nil, fmt.Errorf("fetching plays: %w", err)
		}

		var page kexpPlaysResponse
		if err := json.Unmarshal(resp, &page); err != nil {
			return nil, fmt.Errorf("parsing plays response: %w", err)
		}

		for _, kPlay := range page.Results {
			// Defensive: filter again even though the API was asked to return
			// only trackplays, in case future API changes relax that filter.
			if kPlay.PlayType != "trackplay" {
				continue
			}

			play := parseKEXPPlay(kPlay, position)
			allPlays = append(allPlays, play)
			position++
		}

		url = page.Next
	}

	return allPlays, nil
}

// =============================================================================
// Internal helpers
// =============================================================================

// doGet performs an HTTP GET with the KEXP user agent and returns the response body.
func (p *KEXPProvider) doGet(url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", kexpUserAgent)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("KEXP API returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	return body, nil
}

// fetchAllHosts fetches all KEXP hosts and returns a map of ID → display name.
func (p *KEXPProvider) fetchAllHosts() (map[int]string, error) {
	hosts := make(map[int]string)
	url := fmt.Sprintf("%s/v2/hosts/?limit=100", p.baseURL)

	for url != "" {
		<-p.rateLimiter.C

		resp, err := p.doGet(url)
		if err != nil {
			return hosts, fmt.Errorf("fetching hosts: %w", err)
		}

		var page kexpHostsResponse
		if err := json.Unmarshal(resp, &page); err != nil {
			return hosts, fmt.Errorf("parsing hosts response: %w", err)
		}

		for _, h := range page.Results {
			hosts[h.ID] = h.Name
		}

		url = page.Next
	}

	return hosts, nil
}

// parseKEXPEpisode converts a KEXP show (broadcast) into our episode import DTO.
func parseKEXPEpisode(show kexpShow, programExternalID string) RadioEpisodeImport {
	ep := RadioEpisodeImport{
		ExternalID:     strconv.Itoa(show.ID),
		ShowExternalID: programExternalID,
	}

	// Parse start_time to extract air_date and air_time
	if show.StartTime != "" {
		if t, err := time.Parse(time.RFC3339, show.StartTime); err == nil {
			airDate := t.Format("2006-01-02")
			airTime := t.Format("15:04:05")
			ep.AirDate = airDate
			ep.AirTime = &airTime

			// Calculate duration if end_time is available
			if show.EndTime != "" {
				if end, err := time.Parse(time.RFC3339, show.EndTime); err == nil {
					dur := int(end.Sub(t).Minutes())
					if dur > 0 {
						ep.DurationMinutes = &dur
					}
				}
			}
		}
	}

	if show.ProgramName != "" {
		name := show.ProgramName
		ep.Title = &name
	}

	if show.ArchiveURL != "" {
		archive := show.ArchiveURL
		ep.ArchiveURL = &archive
	}

	return ep
}

// parseKEXPPlay converts a KEXP play into our play import DTO.
func parseKEXPPlay(kPlay kexpPlay, position int) RadioPlayImport {
	play := RadioPlayImport{
		Position:          position,
		ArtistName:        kPlay.Artist,
		IsLivePerformance: kPlay.IsLive,
		IsRequest:         kPlay.IsRequest,
	}

	if kPlay.Song != "" {
		play.TrackTitle = &kPlay.Song
	}
	if kPlay.Album != "" {
		play.AlbumTitle = &kPlay.Album
	}
	if kPlay.Label != "" {
		play.LabelName = &kPlay.Label
	}
	if kPlay.ReleaseDate != "" {
		if year := parseReleaseYear(kPlay.ReleaseDate); year > 0 {
			play.ReleaseYear = &year
		}
	}
	if kPlay.RotationStatus != "" {
		play.RotationStatus = &kPlay.RotationStatus
	}
	if kPlay.Comment != "" {
		play.DJComment = &kPlay.Comment
	}
	if kPlay.IsNew {
		play.IsNew = true
	}

	// MusicBrainz IDs
	if kPlay.MusicBrainzArtistID != "" {
		play.MusicBrainzArtistID = &kPlay.MusicBrainzArtistID
	}
	if kPlay.MusicBrainzReleaseID != "" {
		play.MusicBrainzReleaseID = &kPlay.MusicBrainzReleaseID
	}
	if kPlay.MusicBrainzRecordingID != "" {
		play.MusicBrainzRecordingID = &kPlay.MusicBrainzRecordingID
	}

	// Parse air timestamp
	if kPlay.Airdate != "" {
		if t, err := time.Parse(time.RFC3339, kPlay.Airdate); err == nil {
			play.AirTimestamp = &t
		}
	}

	return play
}

// parseReleaseYear extracts a year from a date string.
// Handles: "2026", "2026-01-15", "2026-01-15T00:00:00Z", etc.
func parseReleaseYear(dateStr string) int {
	if len(dateStr) < 4 {
		return 0
	}
	year, err := strconv.Atoi(dateStr[:4])
	if err != nil {
		return 0
	}
	if year < 1900 || year > 2100 {
		return 0
	}
	return year
}

// =============================================================================
// KEXP API response types (not exported — internal to provider)
// =============================================================================

type kexpPaginatedResponse struct {
	Next    string `json:"next"`
	Count   int    `json:"count"`
}

type kexpProgramsResponse struct {
	kexpPaginatedResponse
	Results []kexpProgram `json:"results"`
}

type kexpProgram struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	ImageURI    string `json:"image_uri"`
	HostIDs     []int  `json:"host_ids"`
	IsActive    bool   `json:"is_active"`
}

type kexpHostsResponse struct {
	kexpPaginatedResponse
	Results []kexpHost `json:"results"`
}

type kexpHost struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type kexpShowsResponse struct {
	kexpPaginatedResponse
	Results []kexpShow `json:"results"`
}

type kexpShow struct {
	ID          int    `json:"id"`
	ProgramID   int    `json:"program_id"`
	ProgramName string `json:"program_name"`
	StartTime   string `json:"start_time"`
	EndTime     string `json:"end_time"`
	ArchiveURL  string `json:"archive_url"`
}

type kexpPlaysResponse struct {
	kexpPaginatedResponse
	Results []kexpPlay `json:"results"`
}

type kexpPlay struct {
	ID                     int    `json:"id"`
	PlayType               string `json:"play_type"`
	Airdate                string `json:"airdate"`
	Artist                 string `json:"artist"`
	Song                   string `json:"song"`
	Album                  string `json:"album"`
	Label                  string `json:"label_name"`
	ReleaseDate            string `json:"release_date"`
	RotationStatus         string `json:"rotation_status"`
	IsNew                  bool   `json:"is_new"`
	IsLive                 bool   `json:"is_live"`
	IsRequest              bool   `json:"is_request"`
	Comment                string `json:"comment"`
	MusicBrainzArtistID    string `json:"musicbrainz_artist_id"`
	MusicBrainzReleaseID   string `json:"musicbrainz_release_id"`
	MusicBrainzRecordingID string `json:"musicbrainz_recording_id"`
}
