package catalog

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	ntsBaseURL        = "https://www.nts.live/api"
	ntsUserAgent      = "PsychicHomily/1.0 (radio-playlist-indexer)"
	ntsDefaultTimeout = 30 * time.Second
	ntsRateLimit      = 1 * time.Second
	ntsPageLimit      = 100
)

// NTSProvider implements RadioPlaylistProvider for NTS Radio's v2 REST API.
type NTSProvider struct {
	httpClient  *http.Client
	baseURL     string
	rateLimiter *time.Ticker
}

// NewNTSProvider creates a new NTS provider with rate limiting.
func NewNTSProvider() *NTSProvider {
	return &NTSProvider{
		httpClient: &http.Client{
			Timeout: ntsDefaultTimeout,
		},
		baseURL:     ntsBaseURL,
		rateLimiter: time.NewTicker(ntsRateLimit),
	}
}

// NewNTSProviderWithClient creates an NTS provider with a custom HTTP client and base URL.
// Exported for testing with httptest servers.
func NewNTSProviderWithClient(client *http.Client, baseURL string) *NTSProvider {
	return &NTSProvider{
		httpClient:  client,
		baseURL:     baseURL,
		rateLimiter: time.NewTicker(1 * time.Millisecond), // fast for tests
	}
}

// Close stops the rate limiter ticker. Should be called when the provider is no longer needed.
func (p *NTSProvider) Close() {
	if p.rateLimiter != nil {
		p.rateLimiter.Stop()
	}
}

// DiscoverShows returns all NTS shows/programs via the v2 API.
func (p *NTSProvider) DiscoverShows() ([]RadioShowImport, error) {
	var allShows []RadioShowImport

	offset := 0
	for {
		<-p.rateLimiter.C

		url := fmt.Sprintf("%s/v2/shows?offset=%d&limit=%d", p.baseURL, offset, ntsPageLimit)
		resp, err := p.doGet(url)
		if err != nil {
			return nil, fmt.Errorf("fetching shows: %w", err)
		}

		var page ntsShowsResponse
		if err := json.Unmarshal(resp, &page); err != nil {
			return nil, fmt.Errorf("parsing shows response: %w", err)
		}

		for _, ntsShow := range page.Results {
			show := parseNTSShow(ntsShow)
			allShows = append(allShows, show)
		}

		// Check if we have more pages
		if len(page.Results) < ntsPageLimit {
			break
		}
		offset += ntsPageLimit
	}

	return allShows, nil
}

// FetchNewEpisodes returns episodes for an NTS show since the given time.
func (p *NTSProvider) FetchNewEpisodes(showExternalID string, since time.Time) ([]RadioEpisodeImport, error) {
	var allEpisodes []RadioEpisodeImport

	offset := 0
	for {
		<-p.rateLimiter.C

		url := fmt.Sprintf("%s/v2/shows/%s/episodes?offset=%d&limit=%d",
			p.baseURL, showExternalID, offset, ntsPageLimit)
		resp, err := p.doGet(url)
		if err != nil {
			return nil, fmt.Errorf("fetching episodes for %s: %w", showExternalID, err)
		}

		var page ntsEpisodesResponse
		if err := json.Unmarshal(resp, &page); err != nil {
			return nil, fmt.Errorf("parsing episodes response: %w", err)
		}

		reachedOldEpisodes := false
		for _, ntsEp := range page.Results {
			ep := parseNTSEpisode(ntsEp, showExternalID)

			// Filter by since date
			if ntsEp.Broadcast != "" {
				broadcastTime, err := time.Parse("2006-01-02T15:04:05Z", ntsEp.Broadcast)
				if err != nil {
					// Try date-only format
					broadcastTime, err = time.Parse("2006-01-02", ntsEp.Broadcast)
				}
				if err == nil && broadcastTime.Before(since) {
					reachedOldEpisodes = true
					break
				}
			}

			allEpisodes = append(allEpisodes, ep)
		}

		if reachedOldEpisodes || len(page.Results) < ntsPageLimit {
			break
		}
		offset += ntsPageLimit
	}

	return allEpisodes, nil
}

// FetchPlaylist returns the track plays for a specific NTS episode.
// The episodeExternalID should be in the format "show-alias/episode-alias".
func (p *NTSProvider) FetchPlaylist(episodeExternalID string) ([]RadioPlayImport, error) {
	// episodeExternalID is "show-alias/episode-alias"
	parts := strings.SplitN(episodeExternalID, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid episode external ID format (expected show-alias/episode-alias): %s", episodeExternalID)
	}
	showAlias := parts[0]
	episodeAlias := parts[1]

	<-p.rateLimiter.C

	url := fmt.Sprintf("%s/v2/shows/%s/episodes/%s", p.baseURL, showAlias, episodeAlias)
	resp, err := p.doGet(url)
	if err != nil {
		return nil, fmt.Errorf("fetching episode detail: %w", err)
	}

	var detail ntsEpisodeDetail
	if err := json.Unmarshal(resp, &detail); err != nil {
		return nil, fmt.Errorf("parsing episode detail: %w", err)
	}

	// Many NTS episodes have no tracklist (DJ mixes, ambient sets) — return empty slice
	if len(detail.Tracklist) == 0 {
		return []RadioPlayImport{}, nil
	}

	var plays []RadioPlayImport
	for i, track := range detail.Tracklist {
		if track.Artist == "" {
			continue
		}

		play := RadioPlayImport{
			Position:   i,
			ArtistName: track.Artist,
		}

		if track.Title != "" {
			title := track.Title
			play.TrackTitle = &title
		}
		if track.Album != "" {
			album := track.Album
			play.AlbumTitle = &album
		}

		plays = append(plays, play)
	}

	// Re-number positions sequentially after any skipped entries
	for i := range plays {
		plays[i].Position = i
	}

	return plays, nil
}

// =============================================================================
// Internal helpers
// =============================================================================

// doGet performs an HTTP GET with the NTS user agent and returns the response body.
func (p *NTSProvider) doGet(url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", ntsUserAgent)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("NTS API returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	return body, nil
}

// parseNTSShow converts an NTS show into our show import DTO.
func parseNTSShow(ntsShow ntsShow) RadioShowImport {
	show := RadioShowImport{
		ExternalID: ntsShow.Alias,
		Name:       ntsShow.Name,
	}

	if ntsShow.Description != "" {
		desc := ntsShow.Description
		show.Description = &desc
	}

	// Build archive URL from alias
	archiveURL := fmt.Sprintf("https://www.nts.live/shows/%s", ntsShow.Alias)
	show.ArchiveURL = &archiveURL

	// Extract host name from the show's description_short field if available,
	// or from the hosts list
	if len(ntsShow.Hosts) > 0 {
		var hostNames []string
		for _, h := range ntsShow.Hosts {
			if h.Name != "" {
				hostNames = append(hostNames, h.Name)
			}
		}
		if len(hostNames) > 0 {
			joined := strings.Join(hostNames, ", ")
			show.HostName = &joined
		}
	}

	if ntsShow.ImageURL != "" {
		img := ntsShow.ImageURL
		show.ImageURL = &img
	}

	return show
}

// parseNTSEpisode converts an NTS episode into our episode import DTO.
func parseNTSEpisode(ntsEp ntsEpisode, showExternalID string) RadioEpisodeImport {
	// Build composite external ID: "show-alias/episode-alias"
	externalID := fmt.Sprintf("%s/%s", showExternalID, ntsEp.EpisodeAlias)

	ep := RadioEpisodeImport{
		ExternalID:     externalID,
		ShowExternalID: showExternalID,
	}

	// Parse broadcast date
	if ntsEp.Broadcast != "" {
		broadcastTime, err := time.Parse("2006-01-02T15:04:05Z", ntsEp.Broadcast)
		if err != nil {
			// Try date-only format
			broadcastTime, err = time.Parse("2006-01-02", ntsEp.Broadcast)
		}
		if err == nil {
			airDate := broadcastTime.Format("2006-01-02")
			airTime := broadcastTime.Format("15:04:05")
			ep.AirDate = airDate
			ep.AirTime = &airTime
		}
	}

	if ntsEp.Name != "" {
		name := ntsEp.Name
		ep.Title = &name
	}

	// Mixcloud URL is the archive for NTS episodes
	if ntsEp.Mixcloud != "" {
		mixcloud := ntsEp.Mixcloud
		ep.ArchiveURL = &mixcloud
	}

	// Duration if available
	if ntsEp.DurationMinutes > 0 {
		dur := ntsEp.DurationMinutes
		ep.DurationMinutes = &dur
	}

	return ep
}

// encodeTagsJSON converts a slice of strings to a JSON-encoded *json.RawMessage.
func encodeTagsJSON(tags []string) *json.RawMessage {
	if len(tags) == 0 {
		return nil
	}
	data, err := json.Marshal(tags)
	if err != nil {
		return nil
	}
	raw := json.RawMessage(data)
	return &raw
}

// =============================================================================
// NTS API response types (not exported -- internal to provider)
// =============================================================================

type ntsShowsResponse struct {
	Results []ntsShow `json:"results"`
}

type ntsShow struct {
	Name             string    `json:"name"`
	Alias            string    `json:"alias"`
	Description      string    `json:"description"`
	DescriptionShort string    `json:"description_short"`
	GenreTags        []string  `json:"genre_tags"`
	MoodTags         []string  `json:"mood_tags"`
	Location         string    `json:"location"`
	ImageURL         string    `json:"image_url"`
	Hosts            []ntsHost `json:"hosts"`
}

type ntsHost struct {
	Name string `json:"name"`
}

type ntsEpisodesResponse struct {
	Results []ntsEpisode `json:"results"`
}

type ntsEpisode struct {
	Name            string   `json:"name"`
	EpisodeAlias    string   `json:"episode_alias"`
	Broadcast       string   `json:"broadcast"`
	Mixcloud        string   `json:"mixcloud"`
	GenreTags       []string `json:"genre_tags"`
	MoodTags        []string `json:"mood_tags"`
	DurationMinutes int      `json:"duration"`
}

type ntsEpisodeDetail struct {
	Name            string          `json:"name"`
	EpisodeAlias    string          `json:"episode_alias"`
	Broadcast       string          `json:"broadcast"`
	Mixcloud        string          `json:"mixcloud"`
	GenreTags       []string        `json:"genre_tags"`
	MoodTags        []string        `json:"mood_tags"`
	DurationMinutes int             `json:"duration"`
	Tracklist       []ntsTrackEntry `json:"tracklist"`
}

type ntsTrackEntry struct {
	Artist string `json:"artist"`
	Title  string `json:"title"`
	Album  string `json:"album"`
}
