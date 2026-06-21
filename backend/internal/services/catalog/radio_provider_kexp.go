package catalog

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
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
	// kexpHostMapBroadcastSampleSize is the number of recent broadcasts the
	// provider walks to build the program_id → host_name map used by
	// DiscoverShows. KEXP's /v2/programs/ endpoint does NOT carry host info,
	// so we derive it from the most recent broadcasts (where each show has a
	// resolved `host_names` array). 1000 broadcasts spans ~166 days at 6
	// shows/day, which covers every actively-aired program with margin.
	kexpHostMapBroadcastSampleSize = 1000
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
//
// PSY-509: KEXP's /v2/programs/ endpoint does NOT include host info on the
// program object — there are no `host_ids` or `host_names` fields. To attach
// host_name to each program we walk a slice of the most recent /v2/shows/
// (broadcast-level) records and build a program_id → host_names map; each
// broadcast carries a resolved `host_names` array. Host-map fetch failures
// are non-fatal: programs are still returned, just without host_name.
func (p *KEXPProvider) DiscoverShows() ([]RadioShowImport, error) {
	var allShows []RadioShowImport

	// Build program_id → host_name map from recent broadcasts.
	// Errors are logged and treated as non-fatal so program discovery still
	// works even if the host-mapping lookup fails.
	hostMap, err := p.fetchProgramHostNames()
	if err != nil {
		slog.Warn("kexp: failed to build program→host map; host_name will be nil for discovered shows",
			"error", err,
		)
		hostMap = make(map[int]string)
	} else if len(hostMap) == 0 {
		slog.Warn("kexp: program→host map is empty after fetching broadcasts; host_name will be nil for discovered shows")
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
			// Attach host_name from broadcast-derived map. Programs that have
			// not aired in the broadcast sample window will have no entry —
			// that's fine, host_name stays nil and admins can fill it in.
			if name, ok := hostMap[prog.ID]; ok && name != "" {
				n := name
				show.HostName = &n
			}

			// PSY-405: intentionally leave ArchiveURL nil for discovered shows.
			// KEXP's per-show URL casing is not derivable from the API name
			// (e.g. "90.TEEN" → /shows/90.-teen/, "Astral Plane" → lowercased
			// /shows/astral-plane/). Any fabricated URL is wrong for roughly
			// 20 of 26 active programs. Admins set the canonical archive URL
			// via the edit drawer; the 6 seed-level shows already have one.

			allShows = append(allShows, show)
		}

		url = page.Next
	}

	return allShows, nil
}

// FetchNewEpisodes returns KEXP "shows" (broadcasts) for a program within [since, until].
// A zero until means no upper bound.
//
// NOTE: The KEXP API's program_id query parameter is silently ignored — the
// endpoint returns ALL broadcasts regardless of which program was requested.
// We still pass it (in case KEXP fixes this) but filter client-side by
// comparing each broadcast's program_id to the requested showExternalID.
func (p *KEXPProvider) FetchNewEpisodes(showExternalID string, since time.Time, until time.Time) ([]RadioEpisodeImport, error) {
	var allEpisodes []RadioEpisodeImport

	programID, err := strconv.Atoi(showExternalID)
	if err != nil {
		return nil, fmt.Errorf("invalid KEXP program ID %q: %w", showExternalID, err)
	}

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
			// Client-side filter: KEXP API ignores program_id param.
			if show.ProgramID != programID {
				continue
			}
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
// PSY-1126: the /v2/plays endpoint's `next` cursor is computed against the
// FULL global plays archive (~millions of rows) and ignores the airdate
// window when deciding whether more pages exist. So `next` is non-empty even
// after the last in-window play -- and stays non-empty for tens of thousands
// of empty pages while offset walks the entire archive. Following it blindly
// (the original `for url != ""` loop) never terminates in practice: with the
// 1s rate limiter it would take ~10 hours / ~36k requests per episode, so the
// import is always interrupted before FetchPlaylist returns and the episode is
// persisted with ZERO plays. We therefore stop paginating as soon as a page
// yields no in-window results. Because results are ordered by airdate
// ascending and every KEXP broadcast fits comfortably under one 100-row page
// (~12-15 plays/hour), this captures the full playlist while bounding the
// request count. The airdate >= endTime guard below is a belt-and-suspenders
// stop for the rare window that legitimately exceeds one page.
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

		// PSY-1126: an EMPTY results page is the real end-of-playlist signal.
		// KEXP keeps returning `next` (offset-walking the whole archive) long
		// after the in-window plays are exhausted, so the `next` cursor is not
		// trustworthy as a loop condition -- but once the airdate window is
		// consumed KEXP returns pages with zero results. We key termination off
		// the raw result count (not the trackplay-matched count) so a page that
		// happens to contain only airbreaks does not prematurely stop a window
		// whose trackplays continue onto the next page.
		pastWindow := false

		for _, kPlay := range page.Results {
			// Results are ordered by airdate ascending. Once a play airs at or
			// after the window end, every later play is also out of window, so
			// we can stop. This bounds the rare window that spans >1 page and
			// guards against the airdate filter being ignored entirely.
			if !endTime.IsZero() && kPlay.Airdate != "" {
				if t, perr := time.Parse(time.RFC3339, kPlay.Airdate); perr == nil && !t.Before(endTime) {
					pastWindow = true
					break
				}
			}

			// Defensive: filter again even though the API was asked to return
			// only trackplays, in case future API changes relax that filter.
			if kPlay.PlayType != "trackplay" {
				continue
			}

			play := parseKEXPPlay(kPlay, position)
			allPlays = append(allPlays, play)
			position++
		}

		// Stop when this page returned no rows (window exhausted -> KEXP now
		// emits empty pages) or when we crossed the window end. Either signals
		// we've consumed the broadcast's full playlist; following `next`
		// further only walks empty pages.
		if len(page.Results) == 0 || pastWindow {
			break
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
	defer resp.Body.Close() //nolint:errcheck // deferred Close; nothing actionable on failure

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		// PSY-887: wrap with RadioHTTPError so the fetch-service circuit
		// breaker can classify (429 → transient, other non-OK → permanent)
		// via errors.As without parsing the error string.
		return nil, newRadioHTTPError("KEXP API", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	return body, nil
}

// fetchProgramHostNames builds a program_id → host_name map by walking the
// most recent broadcasts on /v2/shows/.
//
// PSY-509: KEXP's /v2/programs/ endpoint omits host info entirely (no
// host_ids, no host_names). The /v2/shows/ endpoint, however, includes a
// resolved `host_names` array on every broadcast. Sampling the most-recent
// broadcasts and keeping the first host_names seen per program gives us a
// best-effort mapping for every actively-aired program in O(1) batches of
// API calls.
//
// Returns a partial map plus an error if pagination fails; caller is
// expected to log and continue with the (possibly empty) partial map.
func (p *KEXPProvider) fetchProgramHostNames() (map[int]string, error) {
	hostMap := make(map[int]string)

	pageLimit := 100
	if kexpHostMapBroadcastSampleSize < pageLimit {
		pageLimit = kexpHostMapBroadcastSampleSize
	}

	url := fmt.Sprintf("%s/v2/shows/?ordering=-start_time&limit=%d", p.baseURL, pageLimit)
	pagesFetched := 0
	totalSeen := 0

	for url != "" && totalSeen < kexpHostMapBroadcastSampleSize {
		<-p.rateLimiter.C

		resp, err := p.doGet(url)
		if err != nil {
			slog.Warn("kexp: error fetching shows for host map",
				"error", err,
				"pages_fetched", pagesFetched,
				"programs_resolved", len(hostMap),
			)
			return hostMap, fmt.Errorf("fetching shows for host map: %w", err)
		}

		var page kexpShowListingsResponse
		if err := json.Unmarshal(resp, &page); err != nil {
			slog.Warn("kexp: error parsing shows response for host map",
				"error", err,
				"pages_fetched", pagesFetched,
				"programs_resolved", len(hostMap),
			)
			return hostMap, fmt.Errorf("parsing shows response for host map: %w", err)
		}

		pagesFetched++
		for _, sh := range page.Results {
			totalSeen++
			if sh.Program == 0 {
				continue
			}
			// Skip if we already have this program — keeps the most recent
			// host attribution because results are ordered by -start_time.
			if _, ok := hostMap[sh.Program]; ok {
				continue
			}
			if len(sh.HostNames) == 0 {
				continue
			}
			// Some KEXP broadcasts (overnight automation, "Guest DJ" filler)
			// have empty-string entries — filter those out.
			var clean []string
			for _, n := range sh.HostNames {
				if n != "" {
					clean = append(clean, n)
				}
			}
			if len(clean) == 0 {
				continue
			}
			hostMap[sh.Program] = strings.Join(clean, ", ")
		}

		url = page.Next
	}

	slog.Info("kexp: built program→host map from recent broadcasts",
		"pages_fetched", pagesFetched,
		"broadcasts_scanned", totalSeen,
		"programs_resolved", len(hostMap),
	)

	return hostMap, nil
}

// parseKEXPEpisode converts a KEXP show (broadcast) into our episode import DTO.
//
// PSY-813: The /v2/shows/ LIST endpoint (used by FetchNewEpisodes) does NOT
// include `end_time` or `archive_url` on its result objects — only the
// /v2/shows/{id}/ DETAIL endpoint (used by FetchPlaylist) carries them. As a
// result, episodes imported via FetchNewEpisodes ALWAYS land with
// DurationMinutes == nil and ArchiveURL == nil. The defensive `!= ""` guards
// below make that nil outcome graceful instead of a panic or garbage value.
// If/when callers need duration or archive URL populated, they must round-trip
// through the detail endpoint (currently only FetchPlaylist does so, and only
// to compute its playlist time window — it does not back-fill the episode
// record).
func parseKEXPEpisode(show kexpShow, programExternalID string) RadioEpisodeImport {
	ep := RadioEpisodeImport{
		ExternalID:     strconv.Itoa(show.ID),
		ShowExternalID: programExternalID,
	}

	// Parse start_time to extract air_date and air_time, and PRESERVE the full
	// RFC3339 instants as the episode's frozen air window (PSY-1152) — previously
	// these were decomposed into date/time/duration and the instants discarded.
	if show.StartTime != "" {
		if t, err := time.Parse(time.RFC3339, show.StartTime); err == nil {
			airDate := t.Format("2006-01-02")
			airTime := t.Format("15:04:05")
			ep.AirDate = airDate
			ep.AirTime = &airTime
			start := t
			ep.StartsAt = &start

			// Window end + duration, only for a forward window (end > start). A
			// backwards end < start (clock-skewed feed) would violate the DB
			// air-window CHECK (ends_at >= starts_at) and drop the whole episode;
			// the strict end.After(t) guard also skips a pointless zero-length
			// end == start window. Leave EndsAt nil instead (unbounded → never
			// falsely live), mirroring the dur > 0 guard.
			if show.EndTime != "" {
				if end, err := time.Parse(time.RFC3339, show.EndTime); err == nil && end.After(t) {
					ep.EndsAt = &end
					if dur := int(end.Sub(t).Minutes()); dur > 0 {
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

// kexpLivePlaysLimit is how many recent plays the live now-playing fetch
// pulls: 1 current + headroom for 4 distinct recent artists even with
// back-to-back same-artist spins and interleaved airbreaks.
const kexpLivePlaysLimit = 10

// FetchLiveNowPlaying returns KEXP's current broadcast (PSY-1022).
// KEXP is single-stream, so channel is ignored.
//
// Two time-boxed calls: /v2/shows/?limit=1 (current program + hosts — the
// play record carries only a broadcast-instance id, not the program) and
// /v2/plays/?limit=N (current track + recent plays). KEXP broadcasts 24/7,
// so a successful shows lookup means on-air.
func (p *KEXPProvider) FetchLiveNowPlaying(_ string) (*RadioLiveNowPlaying, error) {
	showBody, err := radioLiveGet(p.httpClient, fmt.Sprintf("%s/v2/shows/?limit=1", p.baseURL), kexpUserAgent, "KEXP API")
	if err != nil {
		return nil, fmt.Errorf("fetching current show: %w", err)
	}
	var shows kexpLiveShowsResponse
	if err := json.Unmarshal(showBody, &shows); err != nil {
		return nil, fmt.Errorf("parsing current show response: %w", err)
	}
	if len(shows.Results) == 0 || shows.Results[0].ProgramName == "" {
		return nil, nil // no current broadcast reported
	}
	current := shows.Results[0]

	live := &RadioLiveNowPlaying{ShowName: current.ProgramName}
	if current.Program > 0 {
		ext := strconv.Itoa(current.Program)
		live.ShowExternalID = &ext
	}
	if host := strings.Join(nonEmptyStrings(current.HostNames), ", "); host != "" {
		live.HostName = &host
	}

	playsBody, err := radioLiveGet(p.httpClient, fmt.Sprintf("%s/v2/plays/?limit=%d", p.baseURL, kexpLivePlaysLimit), kexpUserAgent, "KEXP API")
	if err != nil {
		return nil, fmt.Errorf("fetching current plays: %w", err)
	}
	var plays kexpPlaysResponse
	if err := json.Unmarshal(playsBody, &plays); err != nil {
		return nil, fmt.Errorf("parsing current plays response: %w", err)
	}

	for i, kPlay := range plays.Results {
		if kPlay.PlayType != "trackplay" || kPlay.Artist == "" {
			continue // airbreaks carry no track
		}
		play := parseKEXPPlay(kPlay, 0)
		if i == 0 {
			// The newest play is only "now playing" if it's the head of the
			// feed; a leading airbreak means we're between tracks.
			live.CurrentTrack = &play
		} else {
			live.RecentTracks = append(live.RecentTracks, play)
		}
	}

	return live, nil
}

// nonEmptyStrings filters empty entries (defensive against sparse host_names).
func nonEmptyStrings(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// parseKEXPPlay converts a KEXP play into our play import DTO.
func parseKEXPPlay(kPlay kexpPlay, position int) RadioPlayImport {
	play := RadioPlayImport{
		Position:          position,
		ArtistName:        kPlay.Artist,
		IsLivePerformance: kPlay.IsLive,
		IsRequest:         kPlay.IsRequest,
	}

	// KEXP plays carry a stable numeric id; stringify it (plain "12345", not
	// namespaced — dedup_key is scoped per episode_id) so re-imports dedup by id
	// rather than by content hash. Guard id <= 0 (missing/zero) → leave nil so the
	// content-hash fallback applies and we never write an empty dedup_key.
	if kPlay.ID > 0 {
		pid := strconv.Itoa(kPlay.ID)
		play.ProviderPlayID = &pid
	}

	if kPlay.Song != "" {
		play.TrackTitle = &kPlay.Song
	}
	if kPlay.Album != "" {
		play.AlbumTitle = &kPlay.Album
	}
	if len(kPlay.Labels) > 0 && kPlay.Labels[0] != "" {
		label := kPlay.Labels[0]
		play.LabelName = &label
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
	// MusicBrainz IDs — `artist_ids` is an array; take the first.
	if len(kPlay.ArtistIDs) > 0 && kPlay.ArtistIDs[0] != "" {
		mbArtist := kPlay.ArtistIDs[0]
		play.MusicBrainzArtistID = &mbArtist
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
	Next  string `json:"next"`
	Count int    `json:"count"`
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
	// PSY-509: KEXP's /v2/programs/ endpoint does NOT carry host info on
	// programs (no host_ids, no host_names). Host attribution is derived
	// from the broadcast-level /v2/shows/ endpoint via fetchProgramHostNames.
	IsActive bool `json:"is_active"`
}

// kexpShowListing models the broadcast-level /v2/shows/ response used by
// fetchProgramHostNames. Note this differs from kexpShow (used by
// FetchNewEpisodes/FetchPlaylist): the listing uses the API's actual field
// names (`program`, `host_names`) instead of the legacy `program_id` /
// id-based shape that the older code paths assume.
type kexpShowListingsResponse struct {
	kexpPaginatedResponse
	Results []kexpShowListing `json:"results"`
}

type kexpShowListing struct {
	ID        int      `json:"id"`
	Program   int      `json:"program"`
	HostNames []string `json:"host_names"`
}

type kexpShowsResponse struct {
	kexpPaginatedResponse
	Results []kexpShow `json:"results"`
}

// kexpLiveShowsResponse is the /v2/shows/?limit=1 shape the live now-playing
// fetch needs: the current broadcast's program reference, display name, and
// resolved host names (kexpShow drops host_names; kexpShowListing drops
// program_name — this carries both).
type kexpLiveShowsResponse struct {
	Results []struct {
		Program     int      `json:"program"`
		ProgramName string   `json:"program_name"`
		HostNames   []string `json:"host_names"`
	} `json:"results"`
}

type kexpShow struct {
	ID int `json:"id"`
	// The API names the program reference `program` (an int program ID), not
	// `program_id`. The wrong tag left ProgramID at 0, so FetchNewEpisodes'
	// client-side program filter dropped every broadcast.
	ProgramID   int    `json:"program"`
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
	ID             int      `json:"id"`
	PlayType       string   `json:"play_type"`
	Airdate        string   `json:"airdate"`
	Artist         string   `json:"artist"`
	Song           string   `json:"song"`
	Album          string   `json:"album"`
	Labels         []string `json:"labels"`
	ReleaseDate    string   `json:"release_date"`
	RotationStatus string   `json:"rotation_status"`
	IsLive         bool     `json:"is_live"`
	IsRequest      bool     `json:"is_request"`
	Comment        string   `json:"comment"`
	// KEXP exposes MusicBrainz artist IDs as an array (`artist_ids`); release
	// and recording are scalars (`release_id`, `recording_id`). There is no
	// `is_new` field on the play.
	ArtistIDs              []string `json:"artist_ids"`
	MusicBrainzReleaseID   string   `json:"release_id"`
	MusicBrainzRecordingID string   `json:"recording_id"`
}
