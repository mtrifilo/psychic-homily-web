package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// errNTSNotFound is returned by doGet when the NTS API responds with 404.
// FetchPlaylist uses it to distinguish "no tracklist for this episode"
// (which is normal for DJ mixes) from actual HTTP failures.
var errNTSNotFound = errors.New("NTS API returned 404")

const (
	ntsBaseURL        = "https://www.nts.live/api"
	ntsUserAgent      = "PsychicHomily/1.0 (radio-playlist-indexer)"
	ntsDefaultTimeout = 30 * time.Second
	ntsRateLimit      = 1 * time.Second
	// ntsPageLimit is the max page size NTS will actually honor.
	// The /v2/shows/{alias}/episodes endpoint silently caps results at 12
	// regardless of the requested limit, so anything higher just wastes
	// API calls. The /v2/shows endpoint respects larger limits, but using
	// a single constant keeps the provider simple.
	ntsPageLimit = 12
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
			// NTS caps how deep offset pagination may go and returns 422
			// ("The requested offset is not allowed") past that cap instead of an
			// empty page. Once at least one page is in hand, treat that 422 as the
			// end of the list and return what we collected — failing the whole
			// discover on it discards every show already gathered.
			var httpErr *RadioHTTPError
			if offset > 0 && errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusUnprocessableEntity {
				break
			}
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

// FetchNewEpisodes returns episodes for an NTS show within [since, until].
// A zero until means no upper bound.
func (p *NTSProvider) FetchNewEpisodes(showExternalID string, since time.Time, until time.Time) ([]RadioEpisodeImport, error) {
	var allEpisodes []RadioEpisodeImport

	// datedEpisode pairs a parsed episode with the timestamp used to order and
	// window-filter it — the broadcast instant when present, else its
	// alias-recovered air date (see episodeFilterTime). (PSY-1241)
	type datedEpisode struct {
		ep RadioEpisodeImport
		at time.Time
	}

	offset := 0
	for {
		<-p.rateLimiter.C

		url := fmt.Sprintf("%s/v2/shows/%s/episodes?offset=%d&limit=%d",
			p.baseURL, showExternalID, offset, ntsPageLimit)
		resp, err := p.doGet(url)
		if err != nil {
			// Same NTS offset cap as DiscoverShows: a 422 past the first page
			// marks the end of available episodes, not a fetch failure.
			var httpErr *RadioHTTPError
			if offset > 0 && errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusUnprocessableEntity {
				break
			}
			return nil, fmt.Errorf("fetching episodes for %s: %w", showExternalID, err)
		}

		var page ntsEpisodesResponse
		if err := json.Unmarshal(resp, &page); err != nil {
			return nil, fmt.Errorf("parsing episodes response: %w", err)
		}

		// Sort this page newest-first before applying the early-exit. NTS pagination
		// is assumed to return episodes newest-first, but that ordering is the
		// provider's, unverified, and the wider fetch floor (PSY-1241) now routinely
		// keeps multiple in-window pages live for daily shows — so a page that is not
		// strictly descending would, with a naive break-on-first-old, drop an
		// in-window episode that happens to sit after an older one. Sorting makes the
		// per-page early-exit correct for any within-page order. (Cross-page ordering
		// is still assumed: a fully backdated later page is out of scope — see PSY-1241.)
		// An episode with no usable date at all (no broadcast AND no alias-recovered
		// air date) can't be ordered or window-filtered, so it is kept unconditionally;
		// one with only an alias date is still filtered by that date so a stale archive
		// is not surfaced into the recent feed.
		dated := make([]datedEpisode, 0, len(page.Results))
		for _, ntsEp := range page.Results {
			ep := parseNTSEpisode(ntsEp, showExternalID)
			if at, ok := episodeFilterTime(ep); ok {
				dated = append(dated, datedEpisode{ep: ep, at: at})
				continue
			}
			allEpisodes = append(allEpisodes, ep)
		}
		sort.Slice(dated, func(i, j int) bool { return dated[i].at.After(dated[j].at) })

		reachedOldEpisodes := false
		for _, d := range dated {
			// Filter by date range using the episode's broadcast/air-date timestamp.
			if d.at.Before(since) {
				reachedOldEpisodes = true
				break
			}
			// Skip episodes after the until bound
			if !until.IsZero() && d.at.After(until) {
				continue
			}
			allEpisodes = append(allEpisodes, d.ep)
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
//
// NTS serves tracklists from a separate sub-endpoint:
//
//	GET /v2/shows/{show-alias}/episodes/{ep-alias}/tracklist
//
// The episode detail endpoint does NOT include tracklist data. Many NTS
// episodes (DJ mixes, ambient sets) have no tracklist at all -- the
// tracklist endpoint may return 200 with an empty results array, or 404.
// Both cases are treated as "no tracks" and return an empty slice, not an
// error.
func (p *NTSProvider) FetchPlaylist(episodeExternalID string) ([]RadioPlayImport, error) {
	// episodeExternalID is "show-alias/episode-alias"
	parts := strings.SplitN(episodeExternalID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, fmt.Errorf("invalid episode external ID format (expected show-alias/episode-alias): %s", episodeExternalID)
	}
	showAlias := parts[0]
	episodeAlias := parts[1]

	<-p.rateLimiter.C

	url := fmt.Sprintf("%s/v2/shows/%s/episodes/%s/tracklist", p.baseURL, showAlias, episodeAlias)
	resp, err := p.doGet(url)
	if err != nil {
		// Episodes without tracklists may 404 -- treat as empty, not an error.
		if errors.Is(err, errNTSNotFound) {
			return []RadioPlayImport{}, nil
		}
		return nil, fmt.Errorf("fetching tracklist: %w", err)
	}

	var tracklist ntsTracklistResponse
	if err := json.Unmarshal(resp, &tracklist); err != nil {
		return nil, fmt.Errorf("parsing tracklist response: %w", err)
	}

	// Many NTS episodes have no tracklist (DJ mixes, ambient sets) — return empty slice
	if len(tracklist.Results) == 0 {
		return []RadioPlayImport{}, nil
	}

	var plays []RadioPlayImport
	for _, track := range tracklist.Results {
		if track.Artist == "" {
			continue
		}

		play := RadioPlayImport{
			ArtistName: track.Artist,
		}

		if track.Title != "" {
			title := track.Title
			play.TrackTitle = &title
		}

		plays = append(plays, play)
	}

	// Number positions sequentially (0-based) to match other providers.
	for i := range plays {
		plays[i].Position = i
	}

	return plays, nil
}

// FetchLiveNowPlaying returns the current broadcast on one NTS channel
// (PSY-1022). channel is the NTS channel_name ("1" or "2"), from the in-code
// station routing table. NTS's live endpoint is show-level only — no track
// data — so CurrentTrack/RecentTracks stay nil and the now-playing service
// fills recent artists from the archive fallback.
func (p *NTSProvider) FetchLiveNowPlaying(channel string) (*RadioLiveNowPlaying, error) {
	body, err := radioLiveGet(p.httpClient, p.baseURL+"/v2/live", ntsUserAgent, "NTS API")
	if err != nil {
		return nil, fmt.Errorf("fetching live broadcasts: %w", err)
	}

	var live ntsLiveResponse
	if err := json.Unmarshal(body, &live); err != nil {
		return nil, fmt.Errorf("parsing live response: %w", err)
	}

	for _, ch := range live.Results {
		if ch.ChannelName != channel {
			continue
		}
		if ch.Now == nil {
			return nil, nil // channel exists but reports nothing on air
		}
		// Prefer the embedded show details (proper-case name + show alias)
		// over the often ALL-CAPS broadcast_title.
		name := ch.Now.Embeds.Details.Name
		if name == "" {
			name = ch.Now.BroadcastTitle
		}
		if name == "" {
			return nil, nil
		}
		result := &RadioLiveNowPlaying{ShowName: name}
		if alias := ch.Now.Embeds.Details.ShowAlias; alias != "" {
			result.ShowExternalID = &alias
		}
		return result, nil
	}

	return nil, nil // requested channel not present in the live feed
}

// ntsAiringBroadcastSkewTolerance bounds how far the embedded episode's own
// broadcast INSTANT may sit from the live window's start before the airing is
// treated as a REPEAT and skipped. For a genuine first run the embedded
// episode IS this broadcast, so its `broadcast` matches start_timestamp (the
// observed rerun shape carries the ORIGINAL air instant instead — years off);
// 1h absorbs minor upstream clock/stamping skew while catching even a
// same-day rerun (original evening → overnight repeat is hours apart).
const ntsAiringBroadcastSkewTolerance = time.Hour

// ntsMaxAiringDuration caps the live-feed window accepted as an episode's
// frozen air window. end_timestamp is untrusted external input, and once
// frozen an absurd window would pin the episode "live" (and on the 10-min
// live-refresh ticker) until it passes — the same defensive-bound posture as
// the import path's future-air-date guard. NTS broadcasts run 1–4h; 12h is
// generous headroom.
const ntsMaxAiringDuration = 12 * time.Hour

// FetchCurrentAirings returns the channel's currently-airing NTS broadcast as
// a windowed episode import (PSY-1509), from the same /v2/live feed the live
// now-playing path reads. The feed's start_timestamp/end_timestamp are the
// airing's frozen window; the embedded episode details supply our composite
// external id (show-alias/episode-alias — the FetchPlaylist key).
//
// Skipped (nil, nil) rather than guessed:
//   - no embedded show_alias/episode_alias → no stable identity to ingest;
//   - a REPEAT (the embedded episode's own air date sits far from the live
//     window) → the airing is a rebroadcast of an ARCHIVE episode; creating a
//     now-windowed row under that episode's external id would rewrite the
//     archive episode's identity, and (show, air_date) would fabricate a new
//     airing that never happened. Repeats keep the pre-PSY-1509 behavior (no
//     deep-link date).
func (p *NTSProvider) FetchCurrentAirings(channel string) ([]RadioAiring, error) {
	body, err := radioLiveGet(p.httpClient, p.baseURL+"/v2/live", ntsUserAgent, "NTS API")
	if err != nil {
		return nil, fmt.Errorf("fetching live broadcasts: %w", err)
	}
	var live ntsLiveResponse
	if err := json.Unmarshal(body, &live); err != nil {
		return nil, fmt.Errorf("parsing live response: %w", err)
	}

	for _, ch := range live.Results {
		if ch.ChannelName != channel {
			continue
		}
		if ch.Now == nil {
			return nil, nil // channel exists but reports nothing on air
		}
		det := ch.Now.Embeds.Details
		if det.ShowAlias == "" || det.EpisodeAlias == "" {
			return nil, nil // no stable episode identity — never guess
		}
		start, ok := parseNTSBroadcast(ch.Now.StartTimestamp)
		if !ok {
			return nil, nil // an airing without a start instant can't be windowed
		}

		// Rerun guard, FAIL-CLOSED: only an airing verifiably identical to the
		// embedded episode's own broadcast is ingested. Anything else — a
		// repeat, or an episode we cannot date at all — is skipped: stamping a
		// live window under an archive episode's external id would rewrite
		// that episode's identity and fabricate an airing that never happened.
		//   - broadcast instant present → it must match the live start within
		//     ntsAiringBroadcastSkewTolerance (a first run IS this broadcast);
		//   - alias date only (day granularity) → it must be the live start's
		//     own local calendar day;
		//   - neither → unverifiable, skip.
		// Known residual: an alias-dated same-calendar-day repeat passes (both
		// airings share the date, so air_date at least stays right).
		if bAt, ok := parseNTSBroadcast(det.Broadcast); ok {
			if diff := start.Sub(bAt); diff > ntsAiringBroadcastSkewTolerance || diff < -ntsAiringBroadcastSkewTolerance {
				return nil, nil
			}
		} else if aliasDate := dateFromNTSAlias(det.EpisodeAlias); aliasDate != "" {
			if aliasDate != start.Format("2006-01-02") {
				return nil, nil
			}
		} else {
			return nil, nil
		}

		airTime := start.Format("15:04:05")
		ep := RadioEpisodeImport{
			ExternalID:     fmt.Sprintf("%s/%s", det.ShowAlias, det.EpisodeAlias),
			ShowExternalID: det.ShowAlias,
			AirDate:        start.Format("2006-01-02"),
			AirTime:        &airTime,
			StartsAt:       &start,
		}
		// Plausibility-cap the untrusted end bound (ntsMaxAiringDuration): an
		// absurd window is dropped, leaving the row honestly unbounded (never
		// falsely live) rather than frozen wrong forever.
		if end, ok := parseNTSBroadcast(ch.Now.EndTimestamp); ok && end.After(start) && end.Sub(start) <= ntsMaxAiringDuration {
			e := end
			ep.EndsAt = &e
			if dur := int(end.Sub(start).Minutes()); dur > 0 {
				ep.DurationMinutes = &dur
			}
		}
		if det.Name != "" {
			title := det.Name
			ep.Title = &title
		}
		if det.Mixcloud != "" {
			mixcloud := det.Mixcloud
			ep.ArchiveURL = &mixcloud
		}
		return []RadioAiring{{
			ShowExternalID: det.ShowAlias,
			ShowName:       det.Name,
			Episode:        ep,
		}}, nil
	}
	return nil, nil // requested channel not present in the live feed
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
	defer resp.Body.Close() //nolint:errcheck // deferred Close; nothing actionable on failure

	if resp.StatusCode == http.StatusNotFound {
		return nil, errNTSNotFound
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		// PSY-887: wrap with RadioHTTPError so the fetch-service circuit
		// breaker can classify (429 → transient, other non-OK → permanent)
		// via errors.As without parsing the error string.
		return nil, newRadioHTTPError("NTS API", resp.StatusCode, string(body))
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

	// NTS show artwork lives under media.picture_large (background_large as a
	// fallback). This endpoint exposes no host field, so HostName stays nil
	// (admins can fill it in via the edit drawer).
	if img := ntsShow.Media.PictureLarge; img != "" {
		show.ImageURL = &img
	} else if img := ntsShow.Media.BackgroundLarge; img != "" {
		show.ImageURL = &img
	}

	return show
}

// episodeFilterTime returns the timestamp used to window-filter and order an
// already-parsed episode: the precise broadcast instant (StartsAt) when present,
// else the day-granularity date recovered from the episode alias (AirDate). It
// derives entirely from the parsed `ep` so there is exactly one place that reads
// the raw NTS broadcast (parseNTSEpisode) — no second parse to keep in lockstep.
// The bool is false only when neither is available — a genuinely undateable
// episode, which the caller keeps unconditionally since no bound can be applied.
// Filtering on the alias date (rather than keeping every no-broadcast episode)
// stops a stale archive from surfacing as a recent episode. (PSY-1241)
func episodeFilterTime(ep RadioEpisodeImport) (time.Time, bool) {
	if ep.StartsAt != nil {
		return *ep.StartsAt, true
	}
	if ep.AirDate != "" {
		if t, err := time.Parse("2006-01-02", ep.AirDate); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// parseNTSEpisode converts an NTS episode into our episode import DTO.
func parseNTSEpisode(ntsEp ntsEpisode, showExternalID string) RadioEpisodeImport {
	// Build composite external ID: "show-alias/episode-alias"
	externalID := fmt.Sprintf("%s/%s", showExternalID, ntsEp.EpisodeAlias)

	ep := RadioEpisodeImport{
		ExternalID:     externalID,
		ShowExternalID: showExternalID,
	}

	// Parse the broadcast timestamp for both air date and air time, and preserve
	// the instant as the frozen window start (PSY-1152). NTS supplies no end /
	// duration here, so EndsAt stays nil — its episodes are Mixcloud archives, so
	// the windowless-end case settles to 'aired' (never falsely 'live').
	if t, ok := parseNTSBroadcast(ntsEp.Broadcast); ok {
		ep.AirDate = t.Format("2006-01-02")
		airTime := t.Format("15:04:05")
		ep.AirTime = &airTime
		start := t
		ep.StartsAt = &start
	}

	// Fallback for the rare episode with no `broadcast`: recover the date from
	// the episode alias (e.g. "anu-11th-july-2017"). No air time is available.
	if ep.AirDate == "" {
		ep.AirDate = dateFromNTSAlias(ntsEp.EpisodeAlias)
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

	return ep
}

// parseNTSBroadcast parses an NTS `broadcast` timestamp. NTS returns RFC3339
// with a numeric offset (e.g. "2021-11-04T12:00:00+00:00"); a few records may
// be date-only. Returns the parsed time and true on success. The literal-`Z`
// layout the provider used before never matched the offset form, so every
// episode's air date was silently dropped.
func parseNTSBroadcast(broadcast string) (time.Time, bool) {
	if broadcast == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339, broadcast); err == nil {
		return t, true
	}
	if t, err := time.Parse("2006-01-02", broadcast); err == nil {
		return t, true
	}
	return time.Time{}, false
}

// ntsAliasDateRegex captures a trailing "{day}{ordinal}-{month}-{year}" date in
// an NTS episode alias slug, e.g. "anu-11th-july-2017" -> 11, july, 2017.
var ntsAliasDateRegex = regexp.MustCompile(`(\d{1,2})(?:st|nd|rd|th)-([a-z]+)-(\d{4})$`)

// dateFromNTSAlias derives a YYYY-MM-DD air date from an episode alias slug like
// "anu-11th-july-2017". Returns "" when the alias has no trailing date or the
// date is invalid. Used as a fallback when the NTS API omits `broadcast`.
func dateFromNTSAlias(alias string) string {
	m := ntsAliasDateRegex.FindStringSubmatch(strings.ToLower(alias))
	if m == nil {
		return ""
	}
	day, _ := strconv.Atoi(m[1])
	month, ok := monthMap[m[2]]
	if !ok {
		return ""
	}
	year, _ := strconv.Atoi(m[3])
	t := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	// time.Date normalizes overflow (Feb 31 -> Mar 3), so a round-trip mismatch
	// means the alias held an impossible date — reject it.
	if t.Day() != day || t.Month() != month || t.Year() != year {
		return ""
	}
	return t.Format("2006-01-02")
}

// =============================================================================
// NTS API response types (not exported -- internal to provider)
// =============================================================================

type ntsShowsResponse struct {
	Results []ntsShow `json:"results"`
}

type ntsShow struct {
	Name        string   `json:"name"`
	Alias       string   `json:"show_alias"`
	Description string   `json:"description"`
	Media       ntsMedia `json:"media"`
}

// ntsMedia holds the image variants NTS attaches to a show/episode.
type ntsMedia struct {
	PictureLarge    string `json:"picture_large"`
	BackgroundLarge string `json:"background_large"`
}

type ntsEpisodesResponse struct {
	Results []ntsEpisode `json:"results"`
}

type ntsEpisode struct {
	Name         string `json:"name"`
	EpisodeAlias string `json:"episode_alias"`
	Broadcast    string `json:"broadcast"`
	Mixcloud     string `json:"mixcloud"`
}

// ntsLiveResponse matches GET /v2/live: one entry per channel, each with the
// current (`now`) broadcast and embedded episode details. Only the fields the
// live now-playing fetch and the airing-feed ingestion (PSY-1509) need are
// mapped. NOTE the embedded details are the EPISODE object (episode_alias,
// broadcast = the episode's ORIGINAL air instant, mixcloud) — for a repeat,
// `broadcast` predates start_timestamp by weeks/years, which is exactly what
// the rerun guard in FetchCurrentAirings keys on.
type ntsLiveResponse struct {
	Results []struct {
		ChannelName string `json:"channel_name"`
		Now         *struct {
			BroadcastTitle string `json:"broadcast_title"`
			StartTimestamp string `json:"start_timestamp"`
			EndTimestamp   string `json:"end_timestamp"`
			Embeds         struct {
				Details struct {
					Name         string `json:"name"`
					ShowAlias    string `json:"show_alias"`
					EpisodeAlias string `json:"episode_alias"`
					Broadcast    string `json:"broadcast"`
					Mixcloud     string `json:"mixcloud"`
				} `json:"details"`
			} `json:"embeds"`
		} `json:"now"`
	} `json:"results"`
}

// ntsTracklistResponse matches the JSON returned by
// GET /v2/shows/{alias}/episodes/{ep_alias}/tracklist. NTS wraps the track
// list in a "results" array with a metadata/resultset envelope.
type ntsTracklistResponse struct {
	Results []ntsTrackEntry `json:"results"`
}

// ntsTrackEntry represents a single track in an NTS episode tracklist.
// Only artist and title are actually used -- offset/duration are seconds
// within the episode (not wall-clock air times) so we don't populate
// AirTimestamp from them. Album, label, and release year are not
// available from the NTS tracklist endpoint.
type ntsTrackEntry struct {
	Artist string `json:"artist"`
	Title  string `json:"title"`
}
