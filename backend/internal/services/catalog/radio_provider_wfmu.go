package catalog

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

const (
	wfmuBaseURL        = "https://wfmu.org"
	wfmuUserAgent      = "PsychicHomily/1.0 (radio-playlist-indexer)"
	wfmuDefaultTimeout = 30 * time.Second
	wfmuRateLimit      = 1 * time.Second
)

// WFMUProvider implements RadioPlaylistProvider for WFMU's HTML playlists.
type WFMUProvider struct {
	httpClient  *http.Client
	baseURL     string
	rateLimiter *time.Ticker
}

// NewWFMUProvider creates a new WFMU provider with rate limiting.
func NewWFMUProvider() *WFMUProvider {
	return &WFMUProvider{
		httpClient: &http.Client{
			Timeout: wfmuDefaultTimeout,
		},
		baseURL:     wfmuBaseURL,
		rateLimiter: time.NewTicker(wfmuRateLimit),
	}
}

// NewWFMUProviderWithClient creates a WFMU provider with a custom HTTP client and base URL.
// Exported for testing with httptest servers.
func NewWFMUProviderWithClient(client *http.Client, baseURL string) *WFMUProvider {
	return &WFMUProvider{
		httpClient:  client,
		baseURL:     baseURL,
		rateLimiter: time.NewTicker(1 * time.Millisecond), // fast for tests
	}
}

// Close stops the rate limiter ticker. Should be called when the provider is no longer needed.
func (p *WFMUProvider) Close() {
	if p.rateLimiter != nil {
		p.rateLimiter.Stop()
	}
}

// DiscoverShows returns all WFMU programs by parsing the DJ index page.
//
// NOTE (PSY-1073): the DJ index is one flat list spanning the 91.1 broadcast
// AND the three stream-only channels (Give the Drummer, Rock'n'Soul,
// Sheena's Jungle Room). Importing this unfiltered for every WFMU-family
// station duplicated the full 574-show catalog under each channel. Import
// flows must use DiscoverShowsForStation (the stationScopedShowDiscoverer
// path in radio_import.go) so each station only receives its own shows.
func (p *WFMUProvider) DiscoverShows() ([]RadioShowImport, error) {
	<-p.rateLimiter.C

	body, err := p.doGet(fmt.Sprintf("%s/playlists/", p.baseURL))
	if err != nil {
		return nil, fmt.Errorf("fetching DJ index: %w", err)
	}

	shows, err := parseWFMUDJIndex(body)
	if err != nil {
		return nil, fmt.Errorf("parsing DJ index: %w", err)
	}

	return shows, nil
}

// =============================================================================
// Station-scoped discovery (PSY-1073)
// =============================================================================

// wfmuStationChannels maps our seeded radio_stations.slug values (migration
// 20260502023012) to WFMU channel keys. A WFMU-family station whose slug is
// not in this map cannot be scope-discovered — DiscoverShowsForStation returns
// an error rather than silently importing the whole catalog again.
var wfmuStationChannels = map[string]string{
	"wfmu":                wfmuLiveChannelMain,
	"wfmu-drummer":        wfmuLiveChannelDrummer,
	"wfmu-rocknsoulradio": wfmuLiveChannelRockSoul,
	"wfmu-sheena":         wfmuLiveChannelSheena,
}

// wfmuChannelSchedulePaths are the wfmu.org pages that enumerate which
// programs air on each stream. /table is the 91.1 weekly schedule; the three
// channel landing pages each carry the channel's program roster (anchors with
// relative /playlists/{CODE} hrefs — description cross-references use
// absolute URLs and are excluded by extractShowCode's anchored regex).
// Verified live 2026-06-11: rosters are mutually disjoint and all roster
// codes exist in the DJ index.
var wfmuChannelSchedulePaths = map[string]string{
	wfmuLiveChannelMain:     "/table",
	wfmuLiveChannelDrummer:  "/drummer",
	wfmuLiveChannelRockSoul: "/rocknsoulradio",
	wfmuLiveChannelSheena:   "/sheena",
}

// wfmuChannelArtifactShows pins the channel-stream-as-show rows to their
// channels. These DJ-index entries are named after the stream itself and
// their "episodes" are the channel's whole-stream playlists aired between
// live-DJ slots. They are not all present on their channel's roster page
// (only RQ is, as of 2026-06-11), so without this override they would
// default to the flagship — which is exactly the "Rock'n'Soul Radio is the
// most-active show on every station" artifact PSY-1073 fixes. Their episode
// and play data stays intact, scoped to the channel station only.
var wfmuChannelArtifactShows = map[string]string{
	"GW": wfmuLiveChannelDrummer,  // "Give The Drummer Radio"
	"RQ": wfmuLiveChannelRockSoul, // "Rock'n'Soul Radio"
	"JZ": wfmuLiveChannelSheena,   // "Sheena's Jungle Room Stream"
}

// DiscoverShowsForStation returns only the WFMU programs that air on the
// given station's stream (PSY-1073). stationSlug must be one of the seeded
// WFMU-family slugs in wfmuStationChannels.
//
// Ownership rule (channels keep only shows provably theirs; everything
// ambiguous or unknown defaults to the flagship):
//  1. Channel-stream artifact codes (wfmuChannelArtifactShows) → their channel.
//  2. Codes on the 91.1 /table schedule → flagship, even when a channel also
//     rebroadcasts them (e.g. five 91.1 shows rerun on Rock'n'Soul).
//  3. Codes on exactly one channel roster page → that channel.
//  4. Everything else (defunct shows, codes on no roster) → flagship.
//
// NOTE (PSY-1349): rule 4's flagship default governs which station's discover run
// ROUTES a code's episodes — it no longer chooses a row's HOME. If a map-absent code
// already has a row under a sibling family station, upsertRadioShow's family
// stickiness reuses that row (and cmd/dedup-radio-shows keeps a unique existing
// substream home) instead of minting/moving to a flagship twin.
func (p *WFMUProvider) DiscoverShowsForStation(stationSlug string) ([]RadioShowImport, error) {
	channel, ok := wfmuStationChannels[stationSlug]
	if !ok {
		return nil, fmt.Errorf("unknown WFMU station slug %q: add it to wfmuStationChannels before discovery", stationSlug)
	}

	allShows, err := p.DiscoverShows()
	if err != nil {
		return nil, err
	}

	channelByCode, err := p.fetchShowChannels()
	if err != nil {
		return nil, err
	}

	var scoped []RadioShowImport
	for _, show := range allShows {
		owner, found := channelByCode[show.ExternalID]
		if !found {
			owner = wfmuLiveChannelMain // unknown/defunct → flagship
		}
		if owner == channel {
			scoped = append(scoped, show)
		}
	}
	return scoped, nil
}

// FetchShowOwnership returns external show code → owning station slug for
// every code visible on WFMU's schedule pages, plus the channel-stream
// artifact codes. Used by cmd/dedup-radio-shows to compute the canonical owner
// for existing duplicated rows. Codes ABSENT from the map are deliberately
// treated differently by the two consumers (PSY-1349): discovery routes them to
// the flagship (rule 4 above), but the dedup keeps a unique existing substream
// home rather than forcing the flagship default — absence from volatile roster
// pages is weak evidence, an established row is strong.
func (p *WFMUProvider) FetchShowOwnership() (map[string]string, error) {
	channelByCode, err := p.fetchShowChannels()
	if err != nil {
		return nil, err
	}

	slugByChannel := make(map[string]string, len(wfmuStationChannels))
	for slug, ch := range wfmuStationChannels {
		slugByChannel[ch] = slug
	}

	ownership := make(map[string]string, len(channelByCode))
	for code, ch := range channelByCode {
		ownership[code] = slugByChannel[ch]
	}
	return ownership, nil
}

// fetchShowChannels fetches the four schedule pages and resolves each show
// code to its owning channel per the DiscoverShowsForStation rule set.
func (p *WFMUProvider) fetchShowChannels() (map[string]string, error) {
	codesByChannel := make(map[string]map[string]bool, len(wfmuChannelSchedulePaths))
	// Deterministic fetch order (map iteration order is random).
	for _, channel := range []string{wfmuLiveChannelMain, wfmuLiveChannelDrummer, wfmuLiveChannelRockSoul, wfmuLiveChannelSheena} {
		path := wfmuChannelSchedulePaths[channel]
		<-p.rateLimiter.C
		body, err := p.doGet(p.baseURL + path)
		if err != nil {
			return nil, fmt.Errorf("fetching %s schedule page %s: %w", channel, path, err)
		}
		codes, err := parseWFMUScheduleCodes(body)
		if err != nil {
			return nil, fmt.Errorf("parsing %s schedule page %s: %w", channel, path, err)
		}
		if len(codes) == 0 {
			// An empty roster means the page layout changed (or an error page
			// slipped through with HTTP 200). Treating it as "this channel
			// owns nothing" would silently dump the channel's shows on the
			// flagship — fail loudly instead.
			return nil, fmt.Errorf("no show codes found on %s schedule page %s: page layout may have changed", channel, path)
		}
		codesByChannel[channel] = codes
	}

	mainCodes := codesByChannel[wfmuLiveChannelMain]
	channelByCode := make(map[string]string)
	for code := range mainCodes {
		channelByCode[code] = wfmuLiveChannelMain
	}
	for _, channel := range []string{wfmuLiveChannelDrummer, wfmuLiveChannelRockSoul, wfmuLiveChannelSheena} {
		for code := range codesByChannel[channel] {
			if mainCodes[code] {
				continue // 91.1 broadcast wins over a channel rebroadcast
			}
			if existing, dup := channelByCode[code]; dup && existing != channel {
				// On two channel rosters at once — ambiguous, default flagship.
				channelByCode[code] = wfmuLiveChannelMain
				continue
			}
			channelByCode[code] = channel
		}
	}
	for code, channel := range wfmuChannelArtifactShows {
		channelByCode[code] = channel
	}
	return channelByCode, nil
}

// parseWFMUScheduleCodes extracts the set of show codes linked from a WFMU
// schedule page (/table or a channel landing page). Only anchors whose href
// is exactly /playlists/{CODE} count — extractShowCode's anchored regex
// excludes episode links, the index link, and absolute-URL cross-references
// inside program descriptions.
func parseWFMUScheduleCodes(body []byte) (map[string]bool, error) {
	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("parsing HTML: %w", err)
	}

	codes := make(map[string]bool)
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			if code := extractShowCode(getAttr(n, "href")); code != "" {
				codes[code] = true
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return codes, nil
}

// FetchNewEpisodes returns episodes for a WFMU show within [since, until].
// A zero until is treated as "today" (WFMU-local) — see wfmuTodayCap; future
// rows are never returned.
//
// Always scrapes the show archive page (/playlists/{CODE}), which lists every
// episode ever aired for the show — up to ~26 years of history. An earlier
// implementation used WFMU's RSS feed (/playlistfeed/{CODE}.xml) for recent
// windows, but the feed filters out fill-in / guest-DJ episodes that the
// archive page includes. The archive page is ~10x the bytes of the RSS feed
// but only one HTTP request per show per cycle, well within the per-instance
// rate budget.
func (p *WFMUProvider) FetchNewEpisodes(showExternalID string, since time.Time, until time.Time) ([]RadioEpisodeImport, error) {
	// Don't ingest next-day-or-later rows: cap `until` at today (WFMU-local). This
	// cap applies to ALL WFMU fetch paths — including a manual backfill that passes
	// an explicit future `until`, which is clamped to today (a future `until` would
	// only ever pull placeholders). Real backfills are unaffected: today's episodes
	// are kept by the inclusive date boundary (see wfmuTodayCap), and a past
	// `until` is older than the cap so it still wins. NOTE: the cap is a
	// provider-level filter on air_date; the create-on-first and manual-backfill
	// callers ALSO re-filter the returned rows through episodeInWindow against
	// their ORIGINAL (uncapped) `until`, which is always >= the cap, so it never
	// re-excludes a kept row nor admits a future one. (PSY-1240.)
	todayCap := wfmuTodayCap(time.Now())
	if until.IsZero() || todayCap.Before(until) {
		until = todayCap
	}
	return p.fetchEpisodesFromArchivePage(showExternalID, since, until)
}

// EpisodeListingIsExhaustive marks WFMU's fetch as a complete listing
// (ExhaustiveEpisodeLister, PSY-1286): fetchEpisodesFromArchivePage scrapes the
// show's full archive page — the page WFMU removes a playlist from when a DJ
// (or WFMU) deletes it — so absence from the result within the fetch window is
// an authoritative upstream-retraction signal.
func (p *WFMUProvider) EpisodeListingIsExhaustive() bool { return true }

// wfmuTodayCap is the upper bound for a WFMU fetch: the current day in WFMU's
// timezone, expressed as a UTC-midnight instant so it lines up with
// parseWFMUArchivePage's date-only air_date parse — an air_date == today is kept
// (the filter's bound is inclusive: airTime.After(until) is false at equality),
// tomorrow-on is dropped. (If that air_date parse ever moves off UTC-midnight,
// this convention must move with it — they are coupled by intent, not a shared
// helper.)
//
// WFMU pre-publishes playlist pages for UPCOMING broadcasts; without this cap
// they import as future-dated, 0-track placeholder episodes that pollute the
// catalog and trip the empty_unexpected sync anomaly (PSY-1240). Nothing is
// lost: the same page imports normally once the broadcast has aired (the
// air_date doesn't change), and the post-air backfill fills its playlist.
// PSY-1204/1205 also hide such placeholders from the feeds.
//
// LIMIT (day granularity): this drops NEXT-DAY-AND-LATER placeholders — the
// observed pollution (a weekly show's next-occurrence page, PSY-1230). A page
// pre-published for a broadcast airing LATER TODAY is dated today, so the cap
// keeps it and it still imports as a today-dated 0-track row — a smaller residual
// that self-resolves once the show airs (backfill fills it). WFMU archive dates
// carry no time, so day granularity is the best a date-only cap can do;
// eliminating same-day-ahead empties would need playlist-non-emptiness (or air
// window) gating — out of scope, tracked with the empty_unexpected verification.
//
// The cap is recomputed per call against `now`. A discover/backfill run that
// spans ET midnight can therefore cap its early shows to the previous day and
// its later shows to the new day — a today-boundary episode is then included
// inconsistently within that one run, but recovered on the next run (no
// permanent loss). The `since` lower bound (fetchSince) is UTC-day-based with a
// multi-week floor (fetchLookbackFloorDays), so its boundary fuzz is immaterial;
// this upper bound must be ET-precise (a UTC "today" would admit tomorrow-ET
// placeholders all evening).
//
// LoadLocation only fails when the binary has no IANA tz database; production
// (and the rest of this radio tz subsystem — WindowForDate, the schedule
// scraper) relies on the deploy's tzdata, so the UTC fallback below is defensive
// and unreached there. Tests embed time/tzdata to be host-independent.
func wfmuTodayCap(now time.Time) time.Time {
	loc, err := time.LoadLocation(wfmuScheduleTimezone)
	if err != nil {
		loc = time.UTC // America/New_York ships in the std tz db; defensive only
	}
	t := now.In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// fetchEpisodesFromArchivePage fetches all historical episodes by scraping the
// show archive page (/playlists/{CODE}). This is the backfill path — WFMU has
// no API, so the full archive is only available as HTML. The archive page
// typically lists hundreds to thousands of episodes on a single page (no
// pagination), so one fetch covers the entire history of the show.
//
// A 404 from WFMU (unknown show code) is converted to an empty slice rather
// than an error, consistent with "no episodes" semantics.
func (p *WFMUProvider) fetchEpisodesFromArchivePage(showExternalID string, since time.Time, until time.Time) ([]RadioEpisodeImport, error) {
	<-p.rateLimiter.C

	pageURL := fmt.Sprintf("%s/playlists/%s", p.baseURL, showExternalID)
	body, err := p.doGet(pageURL)
	if err != nil {
		// Unknown show codes return 404 — treat as "no episodes" rather than error.
		if strings.Contains(err.Error(), "status 404") {
			return nil, nil
		}
		return nil, fmt.Errorf("fetching archive page for %s: %w", showExternalID, err)
	}

	episodes, err := parseWFMUArchivePage(body, showExternalID, since, until)
	if err != nil {
		return nil, fmt.Errorf("parsing archive page for %s: %w", showExternalID, err)
	}

	return episodes, nil
}

// FetchPlaylist returns the track plays for a specific WFMU episode by parsing the playlist page HTML.
func (p *WFMUProvider) FetchPlaylist(episodeExternalID string) ([]RadioPlayImport, error) {
	<-p.rateLimiter.C

	pageURL := fmt.Sprintf("%s/playlists/shows/%s", p.baseURL, episodeExternalID)
	body, err := p.doGet(pageURL)
	if err != nil {
		return nil, fmt.Errorf("fetching playlist page %s: %w", episodeExternalID, err)
	}

	plays, err := parseWFMUPlaylistPage(body)
	if err != nil {
		return nil, fmt.Errorf("parsing playlist page %s: %w", episodeExternalID, err)
	}

	return plays, nil
}

// =============================================================================
// Internal HTTP helper
// =============================================================================

// doGet performs an HTTP GET with the WFMU user agent and returns the response body.
func (p *WFMUProvider) doGet(url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", wfmuUserAgent)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // deferred Close; nothing actionable on failure

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		// PSY-887: wrap with RadioHTTPError so the fetch-service circuit
		// breaker can classify (429 → transient, other non-OK → permanent)
		// via errors.As without parsing the error string. The Error() string
		// still contains "status N" so existing string-match callers
		// (radio_provider_wfmu.go's "status 404" no-episodes check) keep working.
		return nil, newRadioHTTPError("WFMU", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	return body, nil
}

// =============================================================================
// DJ Index Parser — DiscoverShows
// =============================================================================

// parseWFMUDJIndex parses the WFMU playlists index page to extract show information.
// The page is organized by day of week with show entries containing links like:
//
//	/playlists/{CODE}  — the show code
//	/playlistfeed/{CODE}.xml — RSS feed
//
// Show names and DJ names are extracted from bold text and "with DJ Name" patterns.
func parseWFMUDJIndex(body []byte) ([]RadioShowImport, error) {
	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("parsing HTML: %w", err)
	}

	var shows []RadioShowImport
	seen := make(map[string]bool) // deduplicate by show code

	// Walk the DOM looking for links matching /playlists/{CODE} pattern
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			href := getAttr(n, "href")
			if code := extractShowCode(href); code != "" && !seen[code] {
				// Found a show link — extract context from surrounding nodes
				show := extractShowFromContext(n, code)
				if show != nil {
					seen[code] = true
					shows = append(shows, *show)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	return shows, nil
}

// showCodeRegex matches /playlists/{CODE} but NOT /playlists/shows/ or /playlists/ alone.
var showCodeRegex = regexp.MustCompile(`^/playlists/([A-Za-z0-9_]+)$`)

// extractShowCode extracts the show code from a /playlists/{CODE} href.
// Returns empty string if the href doesn't match the expected pattern.
func extractShowCode(href string) string {
	matches := showCodeRegex.FindStringSubmatch(href)
	if len(matches) != 2 {
		return ""
	}
	code := matches[1]
	// Exclude non-show paths
	if code == "shows" || code == "search" || code == "index" {
		return ""
	}
	return code
}

// extractShowFromContext extracts show name and DJ name from the DOM context
// surrounding a /playlists/{CODE} link. The typical structure has the show name
// in a bold element and "with DJ Name" text nearby.
func extractShowFromContext(linkNode *html.Node, code string) *RadioShowImport {
	// Walk up to find the containing block (usually a paragraph or div)
	container := findAncestorBlock(linkNode, 5)
	if container == nil {
		// Fall back to parent
		container = linkNode.Parent
	}

	// Extract all text from the container
	fullText := collectText(container)
	if fullText == "" {
		return nil
	}

	// Extract show name — look for bold text in the container
	showName := extractBoldText(container)
	if showName == "" {
		// Fall back: use text before "with" or before the link text
		showName = extractShowNameFromText(fullText)
	}
	if showName == "" {
		return nil
	}

	// Clean up show name
	showName = strings.TrimSpace(showName)
	// Remove trailing "with" if accidentally captured
	showName = strings.TrimSuffix(showName, " with")
	showName = strings.TrimSpace(showName)

	if showName == "" {
		return nil
	}

	show := &RadioShowImport{
		ExternalID: code,
		Name:       showName,
	}

	// Extract DJ name from "with DJ Name" pattern
	djName := extractDJName(fullText, showName)
	if djName != "" {
		show.HostName = &djName
	}

	// Set archive URL
	archiveURL := fmt.Sprintf("https://wfmu.org/playlists/%s", code)
	show.ArchiveURL = &archiveURL

	return show
}

// findAncestorBlock walks up from a node to find the nearest block-level ancestor.
func findAncestorBlock(n *html.Node, maxLevels int) *html.Node {
	current := n.Parent
	for i := 0; i < maxLevels && current != nil; i++ {
		if current.Type == html.ElementNode {
			switch current.Data {
			case "p", "div", "li", "td", "dd", "blockquote":
				return current
			}
		}
		current = current.Parent
	}
	return nil
}

// collectText recursively collects ALL text content from a node, hidden or
// not. Prefer collectVisibleText for anything scraped from WFMU cells that a
// DJ/widget can decorate — hidden comment-widget markup corrupted 123k track
// titles through this function (PSY-1327). collectText remains for surfaces
// verified widget-free (index containers, archive rows, bold titles).
func collectText(n *html.Node) string {
	if n == nil {
		return ""
	}
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			sb.WriteString(node.Data)
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.TrimSpace(sb.String())
}

// extractBoldText finds the first <b> or <strong> element and returns its text.
func extractBoldText(n *html.Node) string {
	if n == nil {
		return ""
	}
	var result string
	var walk func(*html.Node) bool
	walk = func(node *html.Node) bool {
		if node.Type == html.ElementNode && (node.Data == "b" || node.Data == "strong") {
			result = collectText(node)
			return true
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			if walk(c) {
				return true
			}
		}
		return false
	}
	walk(n)
	return result
}

// djNameRegex extracts DJ name from "with DJ Name" text patterns.
var djNameRegex = regexp.MustCompile(`(?i)\bwith\s+(.+?)(?:\s*[-–—]\s*|\s*\[|\s*\(|$)`)

// extractDJName extracts the DJ name from text using "with DJ Name" pattern.
func extractDJName(text string, showName string) string {
	// Remove the show name from the text to find "with DJ Name"
	remaining := text
	if idx := strings.Index(text, showName); idx >= 0 {
		remaining = text[idx+len(showName):]
	}

	matches := djNameRegex.FindStringSubmatch(remaining)
	if len(matches) < 2 {
		return ""
	}

	djName := strings.TrimSpace(matches[1])
	// Clean up common suffixes
	djName = cleanDJName(djName)
	if djName == "" || len(djName) > 100 {
		return ""
	}
	return djName
}

// cleanDJName strips common trailing patterns from extracted DJ names.
func cleanDJName(name string) string {
	// Remove trailing text that's clearly not part of the name
	cutoffs := []string{
		"playlists and archives",
		"playlists",
		"archives",
		"RSS feeds",
		"Hear a sample",
		"See its playlist",
	}
	lower := strings.ToLower(name)
	for _, cut := range cutoffs {
		if idx := strings.Index(lower, strings.ToLower(cut)); idx > 0 {
			name = strings.TrimSpace(name[:idx])
			lower = strings.ToLower(name)
		}
	}

	// Remove trailing punctuation (but keep periods — DJs may use initials like "Richard J.")
	name = strings.TrimRight(name, " -–—:,")
	return strings.TrimSpace(name)
}

// extractShowNameFromText attempts to extract a show name from raw text.
func extractShowNameFromText(text string) string {
	// Try to find text before "with" keyword
	lower := strings.ToLower(text)
	if idx := strings.Index(lower, " with "); idx > 0 {
		return strings.TrimSpace(text[:idx])
	}
	// Try to find text before " - " separator
	if idx := strings.Index(text, " - "); idx > 0 {
		return strings.TrimSpace(text[:idx])
	}
	return ""
}

// getAttr retrieves an attribute value from an HTML node.
func getAttr(n *html.Node, key string) string {
	for _, attr := range n.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

// =============================================================================
// Episode ID + Month Map (shared helpers)
// =============================================================================

// episodeIDRegex extracts the numeric show ID from WFMU playlist URLs.
// Matches: /playlists/shows/162230 or http://www.wfmu.org/playlists/shows/162230
var episodeIDRegex = regexp.MustCompile(`/playlists/shows/(\d+)`)

// monthMap maps month names to month numbers.
var monthMap = map[string]time.Month{
	"january": time.January, "february": time.February, "march": time.March,
	"april": time.April, "may": time.May, "june": time.June,
	"july": time.July, "august": time.August, "september": time.September,
	"october": time.October, "november": time.November, "december": time.December,
}

// extractEpisodeID pulls the numeric episode ID from a WFMU URL.
func extractEpisodeID(url string) string {
	matches := episodeIDRegex.FindStringSubmatch(url)
	if len(matches) < 2 {
		return ""
	}
	return matches[1]
}

// =============================================================================
// Archive Page Parser — FetchNewEpisodes (historical backfill)
// =============================================================================

// kdbEpisodeIDRegex extracts the numeric episode ID from a KenzoDB span id
// attribute like id="KDBepisode-78951". These spans wrap every episode row on
// a WFMU show archive page.
var kdbEpisodeIDRegex = regexp.MustCompile(`^KDBepisode-(\d+)$`)

// archiveDateRegex matches the "Month Day, Year:" date prefix that introduces
// each episode row in a WFMU show archive page (e.g. "May 8, 2018:").
var archiveDateRegex = regexp.MustCompile(`(?i)\b(January|February|March|April|May|June|July|August|September|October|November|December)\s+(\d{1,2}),\s+(\d{4})\b`)

// parseWFMUArchivePage parses a WFMU show archive page (/playlists/{CODE}) and
// returns every episode that has a playlist link, filtered to those with an
// AirDate within [since, until]. A zero `since` or `until` disables that bound.
//
// The archive page structure (as emitted by KenzoDB) is:
//
//	<div class="showlist">
//	  <ul>
//	    <li>
//	      <span class="KDBFavIcon KDBepisode" id="KDBepisode-{ID}">...</span>
//	      {Month} {Day}, {Year}:
//	      <b>{optional title}</b>
//	      <a href="/playlists/shows/{ID}">See the playlist</a>
//	      ...
//	    </li>
//	    ...
//	  </ul>
//	</div>
//
// There can be multiple <ul> blocks (grouped by year). Some <li>s are
// placeholders for fill-in episodes by guest hosts and have no playlist link —
// those are skipped. Pre-2009 episodes may use a legacy
// "Playlists/{ShowName}/xxx.html" link format; those are also skipped since
// their IDs are not addressable via the modern /playlists/shows/{ID} endpoint
// that FetchPlaylist expects.
func parseWFMUArchivePage(body []byte, showExternalID string, since time.Time, until time.Time) ([]RadioEpisodeImport, error) {
	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("parsing HTML: %w", err)
	}

	showlist := findArchiveShowlist(doc)
	if showlist == nil {
		// No showlist container means the page structure isn't recognizable
		// (e.g. an error page or a completely different template). Return
		// empty rather than error — matches "no episodes found" semantics.
		return nil, nil
	}

	// Collect every <li> inside the showlist div (they may be split across
	// multiple <ul>s, but all direct-or-nested <li> descendants are episode rows).
	var liNodes []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "li" {
			liNodes = append(liNodes, n)
			return // don't recurse into a <li>; episode rows don't nest
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(showlist)

	episodes := make([]RadioEpisodeImport, 0, len(liNodes))
	seen := make(map[string]bool, len(liNodes))

	for _, li := range liNodes {
		ep := parseArchiveEpisodeRow(li, showExternalID)
		if ep == nil {
			continue
		}
		if seen[ep.ExternalID] {
			continue
		}
		// Apply date range filters. Zero since/until disables that bound.
		if ep.AirDate != "" {
			if airTime, err := time.Parse("2006-01-02", ep.AirDate); err == nil {
				if !since.IsZero() && airTime.Before(since) {
					continue
				}
				if !until.IsZero() && airTime.After(until) {
					continue
				}
			}
		}
		seen[ep.ExternalID] = true
		episodes = append(episodes, *ep)
	}

	return episodes, nil
}

// findArchiveShowlist locates the <div class="showlist"> container that holds
// the episode list on a WFMU show archive page.
func findArchiveShowlist(n *html.Node) *html.Node {
	if n == nil {
		return nil
	}
	if n.Type == html.ElementNode && n.Data == "div" && hasClass(n, "showlist") {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findArchiveShowlist(c); found != nil {
			return found
		}
	}
	return nil
}

// parseArchiveEpisodeRow extracts a single episode from a <li> row on a WFMU
// show archive page. Returns nil if the row is a fill-in placeholder (no
// playlist link), uses a legacy URL format (pre-2009), or is otherwise
// unparseable.
func parseArchiveEpisodeRow(li *html.Node, showExternalID string) *RadioEpisodeImport {
	// Episode ID comes from the /playlists/shows/{ID} anchor — this is the
	// canonical link that FetchPlaylist can later fetch. Rows without such an
	// anchor (fill-ins, pre-2009 legacy HTML) are skipped.
	var playlistHref string
	var findPlaylistLink func(*html.Node)
	findPlaylistLink = func(n *html.Node) {
		if playlistHref != "" {
			return
		}
		if n.Type == html.ElementNode && n.Data == "a" {
			href := getAttr(n, "href")
			if strings.Contains(href, "/playlists/shows/") {
				playlistHref = href
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			findPlaylistLink(c)
		}
	}
	findPlaylistLink(li)

	episodeID := extractEpisodeID(playlistHref)
	if episodeID == "" {
		// Fall back to the KDBepisode-{ID} span, but only if we still have a
		// playlist link (a pure fill-in placeholder has neither).
		if playlistHref == "" {
			return nil
		}
		episodeID = extractKDBEpisodeID(li)
		if episodeID == "" {
			return nil
		}
	}

	// Extract the row text to find the date.
	rowText := collectText(li)
	airDate := extractArchiveDate(rowText)
	if airDate == "" {
		// Without a date we can't dedupe/sort reliably — skip.
		return nil
	}

	ep := &RadioEpisodeImport{
		ExternalID:     episodeID,
		ShowExternalID: showExternalID,
		AirDate:        airDate,
	}

	// Title: the <b> element following the date typically contains the
	// episode title (may be empty for routine weekly episodes).
	if title := extractArchiveTitle(li); title != "" {
		ep.Title = &title
	}

	// Build an absolute archive URL from the href. If the href is relative,
	// prefix with wfmuBaseURL so consumers have a canonical link.
	archiveURL := playlistHref
	if strings.HasPrefix(archiveURL, "/") {
		archiveURL = wfmuBaseURL + archiveURL
	}
	if archiveURL != "" {
		ep.ArchiveURL = &archiveURL
	}

	return ep
}

// extractKDBEpisodeID walks a <li> looking for a <span id="KDBepisode-{ID}">
// and returns the numeric ID. This is the KenzoDB-generated unique identifier
// attached to every episode row.
func extractKDBEpisodeID(n *html.Node) string {
	var result string
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if result != "" {
			return
		}
		if node.Type == html.ElementNode && node.Data == "span" {
			id := getAttr(node, "id")
			if matches := kdbEpisodeIDRegex.FindStringSubmatch(id); len(matches) == 2 {
				result = matches[1]
				return
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return result
}

// extractArchiveDate parses a "Month Day, Year" date from the text of an
// archive page episode row and returns it in YYYY-MM-DD format. Returns empty
// string if no date is found.
func extractArchiveDate(text string) string {
	matches := archiveDateRegex.FindStringSubmatch(text)
	if len(matches) < 4 {
		return ""
	}
	month, ok := monthMap[strings.ToLower(matches[1])]
	if !ok {
		return ""
	}
	day, err := strconv.Atoi(matches[2])
	if err != nil || day < 1 || day > 31 {
		return ""
	}
	year, err := strconv.Atoi(matches[3])
	if err != nil || year < 1900 || year > 2100 {
		return ""
	}
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
}

// extractArchiveTitle returns the text of the first <b> element inside a <li>
// that is not empty. Archive episode titles are typically wrapped in <b> tags
// (e.g. "Marathon Week 2 w/ co-host Fabio"). Returns empty string for rows
// with no bold title.
func extractArchiveTitle(li *html.Node) string {
	var result string
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if result != "" {
			return
		}
		if node.Type == html.ElementNode && (node.Data == "b" || node.Data == "strong") {
			text := strings.TrimSpace(collectText(node))
			if text != "" {
				result = text
				return
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(li)
	return result
}

// =============================================================================
// Playlist Page Parser — FetchPlaylist
// =============================================================================

// wfmuPlaylistRow represents a raw row extracted from a WFMU playlist table.
type wfmuPlaylistRow struct {
	Artist   string
	Track    string
	Album    string
	Label    string
	Year     string
	Format   string
	Comments string
	IsNew    bool
}

// parseWFMUPlaylistPage parses a WFMU playlist page HTML to extract track plays.
func parseWFMUPlaylistPage(body []byte) ([]RadioPlayImport, error) {
	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("parsing HTML: %w", err)
	}

	rows := extractPlaylistRows(doc)

	var plays []RadioPlayImport
	for i, row := range rows {
		if row.Artist == "" {
			continue
		}

		// Filter out pledge drive promos and station announcements
		if isPledgeOrPromo(row) {
			continue
		}

		play := RadioPlayImport{
			Position:   i,
			ArtistName: row.Artist,
			IsNew:      row.IsNew,
		}

		if row.Track != "" {
			track := row.Track
			play.TrackTitle = &track
		}
		if row.Album != "" {
			album := row.Album
			play.AlbumTitle = &album
		}
		if row.Label != "" {
			label := row.Label
			play.LabelName = &label
		}
		if row.Year != "" {
			if year := parseReleaseYear(row.Year); year > 0 {
				play.ReleaseYear = &year
			}
		}
		if row.Comments != "" {
			comment := row.Comments
			play.DJComment = &comment
		}

		plays = append(plays, play)
	}

	// Re-number positions sequentially after filtering
	for i := range plays {
		plays[i].Position = i
	}

	return plays, nil
}

// extractPlaylistRows walks the DOM to find playlist table rows and extract data.
// WFMU uses a <table> element (often with class "showlist") where each <tr> has
// cells for: Artist, Track, Album, Label, Year, Format, Comments, Images, New, Start Time.
func extractPlaylistRows(doc *html.Node) []wfmuPlaylistRow {
	// Find the playlist table
	table := findPlaylistTable(doc)
	if table == nil {
		return nil
	}

	// Find tbody or use table directly
	tbody := findElement(table, "tbody")
	if tbody == nil {
		tbody = table
	}

	var rows []wfmuPlaylistRow

	// Iterate over <tr> elements
	for tr := tbody.FirstChild; tr != nil; tr = tr.NextSibling {
		if tr.Type != html.ElementNode || tr.Data != "tr" {
			continue
		}

		// Skip header rows
		if hasClass(tr, "head") || hasChild(tr, "th") {
			continue
		}

		row := parsePlaylistTR(tr)
		if row != nil {
			rows = append(rows, *row)
		}
	}

	return rows
}

// findPlaylistTable finds the main playlist table in the DOM.
// WFMU uses class="showlist" or id containing "playlist".
func findPlaylistTable(n *html.Node) *html.Node {
	if n == nil {
		return nil
	}

	// Look for table elements
	if n.Type == html.ElementNode && n.Data == "table" {
		cls := getAttr(n, "class")
		id := getAttr(n, "id")
		if strings.Contains(cls, "showlist") ||
			strings.Contains(cls, "playlist") ||
			strings.Contains(id, "playlist") ||
			strings.Contains(id, "showlist") {
			return n
		}
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findPlaylistTable(c); found != nil {
			return found
		}
	}

	// If no specifically identified table found, look for any table with
	// multiple rows containing track-like data (artist/track patterns).
	return findLargestTable(n)
}

// findLargestTable finds the table with the most <tr> children as a fallback.
func findLargestTable(n *html.Node) *html.Node {
	var bestTable *html.Node
	bestCount := 0

	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "table" {
			// Skip comments table
			id := getAttr(node, "id")
			if id == "comments-table" {
				return
			}

			count := countTRChildren(node)
			if count > bestCount {
				bestCount = count
				bestTable = node
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)

	// Only return if we found a table with a reasonable number of rows
	if bestCount >= 3 {
		return bestTable
	}
	return nil
}

// countTRChildren counts the number of <tr> elements in a subtree.
func countTRChildren(n *html.Node) int {
	count := 0
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "tr" {
			count++
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return count
}

// parsePlaylistTR extracts track data from a single <tr> element.
// WFMU playlist columns (order may vary but typical):
// 0: Artist, 1: Track, 2: Album, 3: Label, 4: Year, 5: Format, 6: Comments, 7: Images, 8: New, 9: Start Time
func parsePlaylistTR(tr *html.Node) *wfmuPlaylistRow {
	cells := collectTDCells(tr)
	if len(cells) < 2 {
		return nil
	}

	row := &wfmuPlaylistRow{}

	// Extract each cell's VISIBLE text: the song cells embed WFMU's hidden
	// comment-widget markup (a "→" jump button + a `"Title" by "Artist"`
	// summary span) that a naive flatten writes into every track title
	// (PSY-1327).
	cellTexts := make([]string, len(cells))
	for i, cell := range cells {
		cellTexts[i] = cleanCellText(collectVisibleText(cell))
	}

	// Map cells to fields based on column count
	// WFMU typically has 10 columns: Artist, Track, Album, Label, Year, Format, Comments, Images, New/Special, Start Time
	switch {
	case len(cells) >= 10:
		row.Artist = cellTexts[0]
		row.Track = cellTexts[1]
		row.Album = cellTexts[2]
		row.Label = cellTexts[3]
		row.Year = cellTexts[4]
		row.Format = cellTexts[5]
		row.Comments = cellTexts[6]
		// cells[7] = Images (skip)
		row.IsNew = isNewFlagged(cells[8], cellTexts[8])
		// cells[9] = Start Time (skip)
	case len(cells) >= 7:
		row.Artist = cellTexts[0]
		row.Track = cellTexts[1]
		row.Album = cellTexts[2]
		row.Label = cellTexts[3]
		row.Year = cellTexts[4]
		row.Comments = cellTexts[6]
	case len(cells) >= 3:
		row.Artist = cellTexts[0]
		row.Track = cellTexts[1]
		if len(cells) > 2 {
			row.Album = cellTexts[2]
		}
	default:
		row.Artist = cellTexts[0]
		if len(cells) > 1 {
			row.Track = cellTexts[1]
		}
	}

	// Clean up fields
	row.Artist = strings.TrimSpace(row.Artist)
	row.Track = strings.TrimSpace(row.Track)
	row.Album = strings.TrimSpace(row.Album)
	row.Label = strings.TrimSpace(row.Label)
	row.Year = strings.TrimSpace(row.Year)
	row.Comments = strings.TrimSpace(row.Comments)

	if row.Artist == "" {
		return nil
	}

	return row
}

// collectTDCells returns all <td> children of a <tr>.
func collectTDCells(tr *html.Node) []*html.Node {
	var cells []*html.Node
	for c := tr.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == "td" {
			cells = append(cells, c)
		}
	}
	return cells
}

// cleanCellText cleans up text extracted from a table cell.
func cleanCellText(text string) string {
	// Remove favoriting star text
	text = strings.ReplaceAll(text, "Favoriting", "")
	// Normalize whitespace
	text = strings.Join(strings.Fields(text), " ")
	return strings.TrimSpace(text)
}

// isNewFlagged checks if a cell indicates a "new" release.
// WFMU may use text "New", a special CSS class, or an icon.
func isNewFlagged(cell *html.Node, text string) bool {
	cleanText := strings.ToLower(strings.TrimSpace(text))
	if cleanText == "new" || cleanText == "n" || cleanText == "yes" || cleanText == "*" {
		return true
	}

	// Check for class-based indicators
	if hasClass(cell, "new") || hasClass(cell, "is-new") {
		return true
	}

	// Check for image alt text containing "new"
	var hasNewImg bool
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "img" {
			alt := strings.ToLower(getAttr(n, "alt"))
			src := strings.ToLower(getAttr(n, "src"))
			if strings.Contains(alt, "new") || strings.Contains(src, "new") {
				hasNewImg = true
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(cell)

	return hasNewImg
}

// hasClass checks if an HTML node has a specific CSS class.
func hasClass(n *html.Node, cls string) bool {
	classes := getAttr(n, "class")
	for _, c := range strings.Fields(classes) {
		if c == cls {
			return true
		}
	}
	return false
}

// hasChild checks if a node has any child element of the given type.
func hasChild(n *html.Node, tag string) bool {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == tag {
			return true
		}
	}
	return false
}

// findElement finds the first descendant element with the given tag name.
func findElement(n *html.Node, tag string) *html.Node {
	if n == nil {
		return nil
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == tag {
			return c
		}
		if found := findElement(c, tag); found != nil {
			return found
		}
	}
	return nil
}

// =============================================================================
// Pledge/Promo filtering
// =============================================================================

// pledgePromoPatterns are patterns that indicate pledge drive promos or station announcements.
var pledgePromoPatterns = []string{
	"pledge",
	"fundraiser",
	"fund raiser",
	"marathon",
	"donate",
	"donation",
	"station id",
	"station identification",
	"promo",
	"psa",
	"public service",
	"underwriting",
	"sponsor",
}

// isPledgeOrPromo returns true if a playlist row appears to be a pledge drive promo
// or station announcement rather than a music track.
func isPledgeOrPromo(row wfmuPlaylistRow) bool {
	// Check artist and track fields for promo patterns
	checkFields := []string{
		strings.ToLower(row.Artist),
		strings.ToLower(row.Track),
		strings.ToLower(row.Comments),
	}

	for _, field := range checkFields {
		if field == "" {
			continue
		}
		for _, pattern := range pledgePromoPatterns {
			if strings.Contains(field, pattern) {
				return true
			}
		}
	}

	// Check for WFMU-specific promo indicators
	artistLower := strings.ToLower(row.Artist)
	if artistLower == "wfmu" || artistLower == "station id" || artistLower == "station break" {
		return true
	}

	return false
}

// =============================================================================
// Utility — also used by KEXP provider (parseReleaseYear is shared)
// =============================================================================

// parseWFMUReleaseYear extracts a 4-digit year from a string.
func parseWFMUReleaseYear(s string) int {
	s = strings.TrimSpace(s)
	if len(s) < 4 {
		return 0
	}
	// Try to extract first 4-digit number
	re := regexp.MustCompile(`\b(\d{4})\b`)
	match := re.FindStringSubmatch(s)
	if len(match) < 2 {
		return 0
	}
	year, err := strconv.Atoi(match[1])
	if err != nil || year < 1900 || year > 2100 {
		return 0
	}
	return year
}

// =============================================================================
// Live now-playing (PSY-1022)
// =============================================================================

// WFMU live channel keys (RadioLiveProvider channel argument). The station →
// channel routing table in radio_now_playing.go maps our station slugs onto
// these.
const (
	wfmuLiveChannelMain     = "wfmu"
	wfmuLiveChannelDrummer  = "drummer"
	wfmuLiveChannelRockSoul = "rocknsoul"
	wfmuLiveChannelSheena   = "sheena"
)

// wfmuLiveNowPlayingPath is the per-stream current-shows fragment WFMU's own
// homepage widget polls (/now-playing-widget.html fetches it every 3.5s and
// DOM-parses it — verified 2026-06-11, the PSY-1022 spike). There is no JSON
// source; this KenzoDB-generated HTML is the machine-readable one, with
// stable class hooks (.item-even/.item-odd, .streamtitle, .bigline,
// .smallline) that WFMU's own JS depends on. ch ids: 1=WFMU 91.1,
// 4=Give the Drummer Radio, 6=Rock'n'Soul Radio, 8=Sheena's Jungle Room.
const wfmuLiveNowPlayingPath = "/currentliveshows_aggregator.php?ch=1,4,6,8"

// FetchLiveNowPlaying returns the current broadcast on one WFMU stream
// (PSY-1022). channel is one of the wfmuLiveChannel* keys.
//
// On-air semantics: a stream counts as live only when its block carries a
// playlist link (= a live DJ is logging tracks). Without one the stream is
// looping unattended (automation), so we return nil and the caller serves the
// honest latest-archive fallback rather than claiming the stream is "ON AIR".
// This applies to the main 91.1 stream too (PSY-1239); it was previously exempted
// ("always on"), so unattended automation on the flagship surfaced as a wrong
// "ON AIR" show. This brings main in line with the side streams' existing rule.
//
// SCOPE / accepted limits (PSY-1239): this catches only LINK-LESS automation. A
// rebroadcast whose block re-serves the ORIGINAL show's /playlists/shows link
// still reads as live — and that is the more likely shape of the originally
// reported skew, so this change is a consistency fix, NOT a confirmed fix for
// that report; distinguishing a rebroadcast from a first airing is PSY-1240's
// scope. A genuinely-live show is also briefly off-air until its DJ logs the
// first track. The premise that the main block actually drops its link during
// automation is not yet verified against a captured off-air response — tracked
// in PSY-1253. All cases err toward not over-claiming live.
func (p *WFMUProvider) FetchLiveNowPlaying(channel string) (*RadioLiveNowPlaying, error) {
	body, err := radioLiveGet(p.httpClient, p.baseURL+wfmuLiveNowPlayingPath, wfmuUserAgent, "WFMU")
	if err != nil {
		return nil, fmt.Errorf("fetching current live shows: %w", err)
	}

	streams, err := parseWFMUCurrentLiveShows(body)
	if err != nil {
		return nil, fmt.Errorf("parsing current live shows: %w", err)
	}
	return streams[channel], nil // nil when the channel is absent or not live
}

// wfmuStreamChannelKey maps a .streamtitle text ("Give the Drummer Radio
// stream") to its wfmuLiveChannel* key, "" when unrecognized. Keyword
// matching mirrors WFMU's widget JS (Drummer/Soul/Sheena, else WFMU).
func wfmuStreamChannelKey(streamTitle string) string {
	switch {
	case strings.Contains(streamTitle, "Drummer"):
		return wfmuLiveChannelDrummer
	case strings.Contains(streamTitle, "Soul"):
		return wfmuLiveChannelRockSoul
	case strings.Contains(streamTitle, "Sheena"):
		return wfmuLiveChannelSheena
	case strings.Contains(streamTitle, "WFMU"):
		return wfmuLiveChannelMain
	default:
		return ""
	}
}

// wfmuBiglineTrackRegex parses a .bigline current-song text after whitespace
// collapse: `"TITLE" by ARTIST`, optionally prefixed with "Your DJ speaks
// over" while the DJ talks over the music. The title group is greedy so
// embedded quotes stay inside the title; entities are already decoded by the
// HTML parser.
var wfmuBiglineTrackRegex = regexp.MustCompile(`^(?:Your DJ speaks over\s+)?[“"](.*)[”"]\s+by\s+(.+)$`)

// wfmuKDBProgramIDRegex extracts the WFMU program code (our
// radio_shows.external_id for WFMU shows) from a KDBprogram-XX span id.
var wfmuKDBProgramIDRegex = regexp.MustCompile(`^KDBprogram-([A-Za-z0-9]+)$`)

// parseWFMUCurrentLiveShows parses the currentliveshows_aggregator fragment
// into per-channel live payloads. Any channel present but not live (no playlist
// link — the main 91.1 stream included, since PSY-1239) is omitted.
func parseWFMUCurrentLiveShows(body []byte) (map[string]*RadioLiveNowPlaying, error) {
	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}

	streams := make(map[string]*RadioLiveNowPlaying)
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "div" {
			class := getAttr(n, "class")
			if class == "item-even" || class == "item-odd" {
				if key, live := parseWFMULiveStreamBlock(n); key != "" && live != nil {
					streams[key] = live
				}
				return // stream blocks don't nest
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return streams, nil
}

// parseWFMULiveStreamBlock parses one .item-even/.item-odd stream block.
// Returns ("", nil) when the block is unrecognizable, (key, nil) when the
// stream is recognized but not live.
func parseWFMULiveStreamBlock(block *html.Node) (string, *RadioLiveNowPlaying) {
	var streamTitle, bigline, smallline, programCode string
	hasPlaylistLink := false

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch {
			case n.Data == "div" && getAttr(n, "class") == "streamtitle":
				// Only the leading text ("WFMU stream") — skip the nested
				// "(Schedule)" link by reading direct text children only.
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					if c.Type == html.TextNode {
						streamTitle += c.Data
					}
				}
			case n.Data == "div" && getAttr(n, "class") == "bigline":
				bigline = collapseWhitespace(collectVisibleText(n))
			case n.Data == "div" && getAttr(n, "class") == "smallline":
				smallline = collapseWhitespace(collectVisibleText(n))
				if span := findFirstChildSpan(n); span != nil {
					if m := wfmuKDBProgramIDRegex.FindStringSubmatch(getAttr(span, "id")); m != nil {
						programCode = m[1]
					}
				}
			case n.Data == "a" && episodeIDRegex.MatchString(getAttr(n, "href")):
				hasPlaylistLink = true
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(block)

	key := wfmuStreamChannelKey(collapseWhitespace(streamTitle))
	if key == "" {
		return "", nil
	}
	// Live-DJ rule (PSY-1239): a stream counts as live only with a playlist link
	// (a live DJ logging tracks) — applies to the main 91.1 stream too. See
	// FetchLiveNowPlaying for the full rationale and accepted limits.
	if !hasPlaylistLink {
		return key, nil
	}

	showName, hostName := parseWFMUSmallline(smallline)
	if showName == "" {
		return key, nil // can't even name the show — fall back to archive
	}

	live := &RadioLiveNowPlaying{ShowName: showName, HostName: hostName}
	if programCode != "" {
		code := programCode
		live.ShowExternalID = &code
	}
	if m := wfmuBiglineTrackRegex.FindStringSubmatch(bigline); m != nil && m[2] != "" {
		title := m[1]
		live.CurrentTrack = &RadioPlayImport{ArtistName: m[2], TrackTitle: &title}
	}
	return key, live
}

// parseWFMUSmallline splits "on Push Button Heaven with Jody Peyote" into
// show name + host. The last " with " is the separator so show names that
// themselves contain "with" survive; a smallline without one is all show.
func parseWFMUSmallline(s string) (showName string, hostName *string) {
	s = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "on "))
	if s == "" {
		return "", nil
	}
	if idx := strings.LastIndex(s, " with "); idx > 0 {
		host := strings.TrimSpace(s[idx+len(" with "):])
		show := strings.TrimSpace(s[:idx])
		if host != "" && show != "" {
			return show, &host
		}
	}
	return s, nil
}

// collectVisibleText collects text like collectText but skips KDBFavIcon
// spans and display:none DESCENDANTS. The fav-icon spans hold WFMU's
// favoriting/comment widgetry — not just the star <img>: the playlist song
// cells carry a comment-thread widget whose jump button glyph is "→" and
// whose hidden drop_*_summary_html span reads `"Title" by "Artist"`, which a
// naive text flatten concatenates into every track title (PSY-1327 — 123k
// stored plays read `X → "X" by "Artist"` before this). The display:none
// skip is the general rule the widget's pieces all satisfy, kept alongside
// the class skip so a widget moved outside the span stays excluded.
//
// The ROOT node's own visibility is deliberately ignored: WFMU hides whole
// per-show columns via td-level inline style (col_media, col_new_or_special
// in the fixture) and any real text a DJ enters there should still be
// captured — only hidden markup NESTED in a cell is widget chrome.
func collectVisibleText(n *html.Node) string {
	if n == nil {
		return ""
	}
	var sb strings.Builder
	var walk func(node *html.Node, isRoot bool)
	walk = func(node *html.Node, isRoot bool) {
		if !isRoot && node.Type == html.ElementNode {
			if node.Data == "span" && strings.Contains(getAttr(node, "class"), "KDBFavIcon") {
				return
			}
			if isStyleHidden(getAttr(node, "style")) {
				return
			}
		}
		if node.Type == html.TextNode {
			sb.WriteString(node.Data)
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c, false)
		}
	}
	walk(n, true)
	return strings.TrimSpace(sb.String())
}

// isStyleHidden reports whether an inline style hides the element —
// whitespace- and case-insensitive ("display: none", "DISPLAY:NONE",
// tab-separated declarations all count; CSS is case-insensitive here).
func isStyleHidden(style string) bool {
	return strings.Contains(strings.ToLower(strings.Join(strings.Fields(style), "")), "display:none")
}

// findFirstChildSpan returns the first <span> descendant of n, nil if none.
func findFirstChildSpan(n *html.Node) *html.Node {
	var found *html.Node
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if found != nil {
			return
		}
		if node.Type == html.ElementNode && node.Data == "span" {
			found = node
			return
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walk(c)
	}
	return found
}

// collapseWhitespace folds all whitespace runs (incl. newlines from HTML
// source formatting) into single spaces and trims.
func collapseWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
