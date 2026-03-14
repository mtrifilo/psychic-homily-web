package pipeline

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Feed type constants.
const (
	FeedTypeICal = "ical"
	FeedTypeRSS  = "rss"
)

// Feed detection source constants.
const (
	FeedSourceLinkTag   = "link_tag"
	FeedSourceCommonURL = "common_url"
	FeedSourceAnchor    = "anchor"
)

// DetectedFeed represents a feed discovered during auto-detection.
type DetectedFeed struct {
	URL        string  `json:"url"`
	FeedType   string  `json:"feed_type"`
	Source     string  `json:"source"`
	Confidence float64 `json:"confidence"`
}

// FeedDetector auto-detects iCal and RSS feeds from venue calendar pages.
type FeedDetector struct {
	httpClient *http.Client
}

// NewFeedDetector creates a new FeedDetector.
func NewFeedDetector() *FeedDetector {
	return &FeedDetector{
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// NewFeedDetectorWithClient creates a FeedDetector with a custom HTTP client (for testing).
func NewFeedDetectorWithClient(client *http.Client) *FeedDetector {
	return &FeedDetector{httpClient: client}
}

// DetectFeeds scans HTML content for iCal and RSS feed links.
// Returns all detected feeds sorted by confidence (highest first).
func (d *FeedDetector) DetectFeeds(calendarURL string, htmlBody string) ([]DetectedFeed, error) {
	parsed, err := url.Parse(calendarURL)
	if err != nil {
		return nil, fmt.Errorf("invalid calendar URL: %w", err)
	}

	var feeds []DetectedFeed
	seen := make(map[string]bool)

	// 1. HTML <link> tags (highest confidence)
	for _, f := range d.detectFromLinkTags(htmlBody, parsed) {
		if !seen[f.URL] {
			feeds = append(feeds, f)
			seen[f.URL] = true
		}
	}

	// 2. HTML anchor tags
	for _, f := range d.detectFromAnchors(htmlBody, parsed) {
		if !seen[f.URL] {
			feeds = append(feeds, f)
			seen[f.URL] = true
		}
	}

	return feeds, nil
}

// ProbeURL checks a candidate URL for feed content via HEAD request.
func (d *FeedDetector) ProbeURL(candidateURL string) (*DetectedFeed, error) {
	req, err := http.NewRequest("HEAD", candidateURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "PsychicHomily/1.0 (feed-detector)")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}

	ct := strings.ToLower(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
	ct = strings.TrimSpace(ct)

	switch ct {
	case "text/calendar":
		return &DetectedFeed{URL: candidateURL, FeedType: FeedTypeICal, Source: FeedSourceCommonURL, Confidence: 0.9}, nil
	case "application/rss+xml", "application/atom+xml", "application/xml", "text/xml":
		return &DetectedFeed{URL: candidateURL, FeedType: FeedTypeRSS, Source: FeedSourceCommonURL, Confidence: 0.85}, nil
	}

	return nil, nil
}

// detectFromLinkTags scans <link rel="alternate"> tags in HTML.
func (d *FeedDetector) detectFromLinkTags(htmlBody string, baseURL *url.URL) []DetectedFeed {
	var feeds []DetectedFeed

	linkRe := regexp.MustCompile(`(?i)<link\s+[^>]*>`)
	for _, link := range linkRe.FindAllString(htmlBody, -1) {
		lo := strings.ToLower(link)
		if !strings.Contains(lo, `rel="alternate"`) && !strings.Contains(lo, `rel='alternate'`) {
			continue
		}

		href := extractAttr(link, "href")
		if href == "" {
			continue
		}

		feedURL := resolveURL(baseURL, href)
		if feedURL == "" {
			continue
		}

		linkType := strings.ToLower(extractAttr(link, "type"))
		switch linkType {
		case "text/calendar", "application/calendar+xml":
			feeds = append(feeds, DetectedFeed{URL: feedURL, FeedType: FeedTypeICal, Source: FeedSourceLinkTag, Confidence: 0.95})
		case "application/rss+xml", "application/atom+xml":
			feeds = append(feeds, DetectedFeed{URL: feedURL, FeedType: FeedTypeRSS, Source: FeedSourceLinkTag, Confidence: 0.95})
		}
	}

	return feeds
}

// detectFromAnchors scans <a> tags for feed-like links.
func (d *FeedDetector) detectFromAnchors(htmlBody string, baseURL *url.URL) []DetectedFeed {
	var feeds []DetectedFeed

	anchorRe := regexp.MustCompile(`(?i)<a\s+[^>]*href\s*=\s*["']([^"']+)["'][^>]*>(.*?)</a>`)
	for _, m := range anchorRe.FindAllStringSubmatch(htmlBody, -1) {
		if len(m) < 3 {
			continue
		}
		href, text := m[1], strings.ToLower(strings.TrimSpace(m[2]))
		feedURL := resolveURL(baseURL, href)
		if feedURL == "" {
			continue
		}

		lo := strings.ToLower(href)

		// File extension detection
		if strings.HasSuffix(lo, ".ics") {
			feeds = append(feeds, DetectedFeed{URL: feedURL, FeedType: FeedTypeICal, Source: FeedSourceAnchor, Confidence: 0.8})
			continue
		}
		if strings.HasSuffix(lo, ".rss") || strings.HasSuffix(lo, ".xml") {
			feeds = append(feeds, DetectedFeed{URL: feedURL, FeedType: FeedTypeRSS, Source: FeedSourceAnchor, Confidence: 0.7})
			continue
		}

		// webcal:// protocol
		if strings.HasPrefix(lo, "webcal://") {
			feeds = append(feeds, DetectedFeed{URL: feedURL, FeedType: FeedTypeICal, Source: FeedSourceAnchor, Confidence: 0.85})
			continue
		}

		// Keyword-based detection in link text
		icalKeywords := []string{"subscribe", "calendar feed", "ical", "icalendar", "add to calendar", "webcal"}
		rssKeywords := []string{"rss feed", "rss"}

		for _, kw := range icalKeywords {
			if strings.Contains(text, kw) {
				feeds = append(feeds, DetectedFeed{URL: feedURL, FeedType: FeedTypeICal, Source: FeedSourceAnchor, Confidence: 0.6})
				break
			}
		}
		for _, kw := range rssKeywords {
			if strings.Contains(text, kw) {
				feeds = append(feeds, DetectedFeed{URL: feedURL, FeedType: FeedTypeRSS, Source: FeedSourceAnchor, Confidence: 0.6})
				break
			}
		}
	}

	return feeds
}

// extractAttr extracts an attribute value from an HTML tag string.
func extractAttr(tag, attr string) string {
	for _, q := range []string{`"`, `'`} {
		re := regexp.MustCompile(fmt.Sprintf(`(?i)%s\s*=\s*%s([^%s]*)%s`, regexp.QuoteMeta(attr), q, q, q))
		if m := re.FindStringSubmatch(tag); len(m) > 1 {
			return m[1]
		}
	}
	return ""
}

// resolveURL resolves a potentially relative URL against a base URL.
func resolveURL(base *url.URL, href string) string {
	if strings.HasPrefix(href, "webcal://") {
		href = "https://" + strings.TrimPrefix(href, "webcal://")
	}
	parsed, err := url.Parse(href)
	if err != nil {
		return ""
	}
	return base.ResolveReference(parsed).String()
}
