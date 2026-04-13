package catalog

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"
	"golang.org/x/net/html"
)

const (
	wfmuBaseURL        = "https://wfmu.org"
	wfmuUserAgent      = "PsychicHomily/1.0 (radio-playlist-indexer)"
	wfmuDefaultTimeout = 30 * time.Second
	wfmuRateLimit      = 1 * time.Second
	// wfmuRSSFallbackWindow is the age threshold beyond which FetchNewEpisodes
	// switches from the 10-item RSS feed to the full HTML archive page. RSS is
	// sufficient for ongoing incremental fetches; the archive page is required
	// for historical backfill.
	wfmuRSSFallbackWindow = 14 * 24 * time.Hour
)

// WFMUProvider implements RadioPlaylistProvider for WFMU's HTML playlists and RSS feeds.
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

// FetchNewEpisodes returns episodes for a WFMU show within [since, until].
// A zero until means no upper bound.
//
// WFMU's RSS feed (/playlistfeed/{CODE}.xml) returns only the most recent 10
// episodes, which works for incremental fetches but makes historical backfill
// impossible. When `since` falls within the RSS fallback window (~14 days ago),
// we use the fast RSS path. For older `since` values we scrape the full show
// archive page (/playlists/{CODE}), which lists every episode ever aired for
// the show — up to ~26 years of history.
func (p *WFMUProvider) FetchNewEpisodes(showExternalID string, since time.Time, until time.Time) ([]RadioEpisodeImport, error) {
	// Use RSS for recent windows: fast, sufficient, cheap.
	// A zero `since` means "all history" — always use the archive page.
	if !since.IsZero() && time.Since(since) < wfmuRSSFallbackWindow {
		return p.fetchEpisodesFromRSS(showExternalID, since, until)
	}
	return p.fetchEpisodesFromArchivePage(showExternalID, since, until)
}

// fetchEpisodesFromRSS fetches recent episodes from the WFMU playlist RSS feed
// (/playlistfeed/{CODE}.xml). The feed contains the 10 most recent episodes.
func (p *WFMUProvider) fetchEpisodesFromRSS(showExternalID string, since time.Time, until time.Time) ([]RadioEpisodeImport, error) {
	<-p.rateLimiter.C

	feedURL := fmt.Sprintf("%s/playlistfeed/%s.xml", p.baseURL, showExternalID)
	body, err := p.doGet(feedURL)
	if err != nil {
		return nil, fmt.Errorf("fetching RSS feed for %s: %w", showExternalID, err)
	}

	episodes, err := parseWFMURSSFeed(body, showExternalID, since, until)
	if err != nil {
		return nil, fmt.Errorf("parsing RSS feed for %s: %w", showExternalID, err)
	}

	return episodes, nil
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
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("WFMU returned status %d: %s", resp.StatusCode, string(body))
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

// collectText recursively collects all text content from a node.
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
// RSS Feed Parser — FetchNewEpisodes
// =============================================================================

// episodeIDRegex extracts the numeric show ID from WFMU playlist URLs.
// Matches: /playlists/shows/162230 or http://www.wfmu.org/playlists/shows/162230
var episodeIDRegex = regexp.MustCompile(`/playlists/shows/(\d+)`)

// parseWFMURSSFeed parses a WFMU playlist RSS feed and returns episodes within [since, until].
// A zero until means no upper bound.
func parseWFMURSSFeed(body []byte, showExternalID string, since time.Time, until time.Time) ([]RadioEpisodeImport, error) {
	fp := gofeed.NewParser()
	feed, err := fp.ParseString(string(body))
	if err != nil {
		return nil, fmt.Errorf("parsing RSS: %w", err)
	}

	var episodes []RadioEpisodeImport

	for _, item := range feed.Items {
		// Extract episode ID from the link URL
		epID := extractEpisodeID(item.Link)
		if epID == "" {
			// Try GUID as fallback
			epID = extractEpisodeID(item.GUID)
		}
		if epID == "" {
			continue
		}

		// Parse publish date
		var pubTime time.Time
		if item.PublishedParsed != nil {
			pubTime = *item.PublishedParsed
		} else if item.Published != "" {
			// Try manual parse if gofeed didn't handle it
			pubTime, _ = time.Parse(time.RFC1123Z, item.Published)
		}

		// Filter by since time
		if !pubTime.IsZero() && pubTime.Before(since) {
			continue
		}

		// Filter by until time
		if !pubTime.IsZero() && !until.IsZero() && pubTime.After(until) {
			continue
		}

		// Determine air date: prefer extracting from title (reflects local broadcast date),
		// fall back to pubDate in original timezone. WFMU RSS pubDates are in Eastern time
		// so a 10pm EDT show becomes the next day in UTC — we want the local broadcast date.
		airDate := ""
		if item.Title != "" {
			airDate = extractDateFromTitle(item.Title)
		}
		if airDate == "" && !pubTime.IsZero() {
			// Use the original timezone from the RSS feed if available
			if item.Published != "" {
				if parsed, err := time.Parse("Mon, 02 Jan 2006 15:04:05 -0700", item.Published); err == nil {
					airDate = parsed.Format("2006-01-02")
				}
			}
			if airDate == "" {
				airDate = pubTime.Format("2006-01-02")
			}
		}

		ep := RadioEpisodeImport{
			ExternalID:     epID,
			ShowExternalID: showExternalID,
			AirDate:        airDate,
		}

		if item.Title != "" {
			title := item.Title
			ep.Title = &title
		}

		if item.Link != "" {
			archive := item.Link
			ep.ArchiveURL = &archive
		}

		episodes = append(episodes, ep)
	}

	return episodes, nil
}

// titleDateRegex matches dates like "March 12, 2026" or "January 15, 2026" in titles.
var titleDateRegex = regexp.MustCompile(`(?i)(January|February|March|April|May|June|July|August|September|October|November|December)\s+(\d{1,2}),?\s+(\d{4})`)

// monthMap maps month names to month numbers.
var monthMap = map[string]time.Month{
	"january": time.January, "february": time.February, "march": time.March,
	"april": time.April, "may": time.May, "june": time.June,
	"july": time.July, "august": time.August, "september": time.September,
	"october": time.October, "november": time.November, "december": time.December,
}

// extractDateFromTitle parses a date from a WFMU RSS title like
// "WFMU Playlist: Show Name from March 12, 2026".
func extractDateFromTitle(title string) string {
	matches := titleDateRegex.FindStringSubmatch(title)
	if len(matches) < 4 {
		return ""
	}

	month, ok := monthMap[strings.ToLower(matches[1])]
	if !ok {
		return ""
	}
	day, err := strconv.Atoi(matches[2])
	if err != nil {
		return ""
	}
	year, err := strconv.Atoi(matches[3])
	if err != nil {
		return ""
	}

	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
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

	// Extract text from each cell
	cellTexts := make([]string, len(cells))
	for i, cell := range cells {
		cellTexts[i] = cleanCellText(collectText(cell))
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
