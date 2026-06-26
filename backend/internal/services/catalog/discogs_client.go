package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Discogs database client for the image-enrichment backfill (PSY-1216).
//
// This is the TRANSPORT layer only: it speaks Discogs' token auth + required
// User-Agent and exposes a release search. It knows Discogs' wire format and
// nothing about our match policy — the strict gate lives in the enricher
// (cover_art_enricher.go).
//
// We store only a REFERENCE to the Discogs-hosted cover image, never the bytes
// (PSY-1175 D1/D3). Discogs requires the exact "Data provided by Discogs"
// attribution + a linkback to the release page; the enricher records
// source='discogs' + the release URL, and the frontend ImageAttribution component
// renders the required phrasing.

const (
	discogsBaseURL    = "https://api.discogs.com"
	discogsWebBaseURL = "https://www.discogs.com"
	// discogsUserAgent — Discogs rejects requests without a descriptive User-Agent.
	discogsUserAgent = "PsychicHomily/1.0 (image-enrichment; https://psychichomily.com)"
	discogsTimeout   = 20 * time.Second
	// discogsRateLimit paces requests under Discogs' authenticated limit of 60/min
	// (1/s). A touch over 1s keeps us safely under it.
	discogsRateLimit      = 1100 * time.Millisecond
	discogsDefaultPerPage = 10
	discogsErrorBodyLimit = 512
	// discogsImageHost is the host real cover images are served from. A release
	// with no image yields a static spacer on a different host; gating on this host
	// rejects those blank placeholders so we never store a spacer as a cover.
	discogsImageHost = "i.discogs.com"
)

// DiscogsClient is a minimal Discogs database client for cover-art search.
type DiscogsClient struct {
	httpClient  *http.Client
	baseURL     string
	webURL      string
	token       string
	rateLimiter *time.Ticker
}

// NewDiscogsClient builds a production Discogs client pointed at the real API.
// token is a Discogs personal-access / consumer token (required for image fields
// + the 60/min authenticated rate).
func NewDiscogsClient(token string) *DiscogsClient {
	return &DiscogsClient{
		httpClient:  &http.Client{Timeout: discogsTimeout},
		baseURL:     discogsBaseURL,
		webURL:      discogsWebBaseURL,
		token:       token,
		rateLimiter: time.NewTicker(discogsRateLimit),
	}
}

// NewDiscogsClientWithConfig points the client at custom base URLs (httptest
// servers) with a fast rate limiter. Exported for tests.
func NewDiscogsClientWithConfig(httpClient *http.Client, baseURL, webURL, token string) *DiscogsClient {
	return &DiscogsClient{
		httpClient:  httpClient,
		baseURL:     baseURL,
		webURL:      webURL,
		token:       token,
		rateLimiter: time.NewTicker(1 * time.Millisecond), // fast for tests
	}
}

// Close stops the rate limiter ticker. Call when the client is no longer needed.
func (c *DiscogsClient) Close() {
	if c.rateLimiter != nil {
		c.rateLimiter.Stop()
	}
}

// DiscogsRelease is the trimmed search result the enricher needs. Title is
// Discogs' combined "Artist - Title" form (search results do not split the two).
type DiscogsRelease struct {
	ID         int64
	Title      string // "Artist - Title"
	Year       int    // 0 when unknown / unparseable
	CoverImage string // https i.discogs.com cover URL
	SourceURL  string // https://www.discogs.com/release/{id}
}

type discogsSearchResponse struct {
	Results []discogsSearchResult `json:"results"`
}

type discogsSearchResult struct {
	ID         int64  `json:"id"`
	Title      string `json:"title"`
	Year       string `json:"year"`
	CoverImage string `json:"cover_image"`
	Type       string `json:"type"`
}

// SearchReleaseCovers searches Discogs for releases matching artist + title and
// returns candidates that carry a real cover image. The server-side artist /
// release_title filters narrow the search; the enricher applies the final strict
// gate + ambiguity skip. An empty artist or title returns no results without an
// API call. Candidates without an i.discogs.com cover image (e.g. a release with
// no art, served as a spacer) are dropped here so a blank placeholder can never
// reach the matcher.
func (c *DiscogsClient) SearchReleaseCovers(ctx context.Context, artist, title string, limit int) ([]DiscogsRelease, error) {
	if strings.TrimSpace(artist) == "" || strings.TrimSpace(title) == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 100 {
		limit = discogsDefaultPerPage
	}

	<-c.rateLimiter.C

	params := url.Values{}
	params.Set("type", "release")
	params.Set("artist", artist)
	params.Set("release_title", title)
	params.Set("per_page", strconv.Itoa(limit))

	reqURL := c.baseURL + "/database/search?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating discogs request: %w", err)
	}
	req.Header.Set("User-Agent", discogsUserAgent)
	req.Header.Set("Accept", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Discogs token="+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing discogs request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // deferred Close; nothing actionable on failure

	if resp.StatusCode == http.StatusTooManyRequests {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("discogs rate limited (HTTP 429)")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading discogs response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("discogs search returned status %d: %s", resp.StatusCode, discogsTruncateBody(string(body)))
	}

	var sr discogsSearchResponse
	if err := json.Unmarshal(body, &sr); err != nil {
		return nil, fmt.Errorf("parsing discogs response: %w", err)
	}

	out := make([]DiscogsRelease, 0, len(sr.Results))
	for _, r := range sr.Results {
		if r.Type != "" && r.Type != "release" {
			continue
		}
		if !isDiscogsCoverImage(r.CoverImage) {
			continue // no real cover (spacer / empty) — nothing to store
		}
		out = append(out, DiscogsRelease{
			ID:         r.ID,
			Title:      r.Title,
			Year:       parseDiscogsYear(r.Year),
			CoverImage: r.CoverImage,
			SourceURL:  fmt.Sprintf("%s/release/%d", c.webURL, r.ID),
		})
	}
	return out, nil
}

// isDiscogsCoverImage reports whether raw is a real Discogs cover image — an https
// URL on i.discogs.com. Releases without art return a spacer on a different host,
// which this rejects.
func isDiscogsCoverImage(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	return u.Scheme == "https" && strings.ToLower(u.Hostname()) == discogsImageHost
}

// parseDiscogsYear parses Discogs' string `year` field ("2003", "", "0") to an
// int, returning 0 for anything not a positive year.
func parseDiscogsYear(s string) int {
	y, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || y <= 0 {
		return 0
	}
	return y
}

// discogsTruncateBody caps an error-body slice so a multi-KB error page does not
// bloat the error chain / logs.
func discogsTruncateBody(s string) string {
	if len(s) > discogsErrorBodyLimit {
		return s[:discogsErrorBodyLimit] + "...[truncated]"
	}
	return s
}
