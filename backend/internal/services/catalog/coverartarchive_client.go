package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Cover Art Archive client for the image-enrichment backfill (PSY-1216).
//
// The Cover Art Archive (CAA) is a MusicBrainz project that serves cover images
// keyed by MusicBrainz release / release-group MBID. This is the TRANSPORT layer
// only: given a release-group MBID (resolved by a MusicBrainz search upstream), it
// reports whether the Archive has a front cover and returns a STABLE reference to
// it. It knows nothing about our match policy — the strict gate lives in the
// enricher (cover_art_enricher.go).
//
// We store only a REFERENCE to the CAA-hosted image, never the bytes — the
// PSY-1175 architecture decision doc (D1/D3). CAA art carries no reuse license, so
// it must never be persisted as bytes (architecture "Compliance posture").

const (
	caaBaseURL = "https://coverartarchive.org"
	// mbWebBaseURL is the human-facing MusicBrainz site (NOT the /ws/2 API). It is
	// the attribution linkback for a CAA cover: the Archive has no human page of
	// its own (the API endpoint returns JSON), and the release-group page on
	// MusicBrainz is where the art is curated.
	mbWebBaseURL = "https://musicbrainz.org"
	caaUserAgent = "PsychicHomily/1.0 (image-enrichment; https://psychichomily.com)"
	caaTimeout   = 20 * time.Second
	// caaRateLimit spaces CAA requests. CAA is CDN-served and explicitly does not
	// require rate limiting, but a small spacing keeps us a polite client.
	caaRateLimit = 100 * time.Millisecond
)

// CoverArtArchiveClient fetches release-group cover references from the Cover Art
// Archive. Safe for sequential use by the backfill.
type CoverArtArchiveClient struct {
	httpClient  *http.Client
	baseURL     string
	mbWebURL    string
	rateLimiter *time.Ticker
}

// NewCoverArtArchiveClient builds a production CAA client pointed at the real
// Archive + the real MusicBrainz site.
func NewCoverArtArchiveClient() *CoverArtArchiveClient {
	return &CoverArtArchiveClient{
		httpClient:  &http.Client{Timeout: caaTimeout},
		baseURL:     caaBaseURL,
		mbWebURL:    mbWebBaseURL,
		rateLimiter: time.NewTicker(caaRateLimit),
	}
}

// NewCoverArtArchiveClientWithConfig points the client at custom base URLs
// (httptest servers) with a fast rate limiter. Exported for tests.
func NewCoverArtArchiveClientWithConfig(httpClient *http.Client, baseURL, mbWebURL string) *CoverArtArchiveClient {
	return &CoverArtArchiveClient{
		httpClient:  httpClient,
		baseURL:     baseURL,
		mbWebURL:    mbWebURL,
		rateLimiter: time.NewTicker(1 * time.Millisecond), // fast for tests
	}
}

// Close stops the rate limiter ticker. Call when the client is no longer needed.
func (c *CoverArtArchiveClient) Close() {
	if c.rateLimiter != nil {
		c.rateLimiter.Stop()
	}
}

// CoverArtResult is a resolved cover reference: the stable image URL plus the
// human linkback used for attribution.
type CoverArtResult struct {
	ImageURL  string // stable coverartarchive.org/.../front reference (never bytes)
	SourceURL string // MusicBrainz release-group page (attribution linkback)
}

// caaImage is one image in a CAA manifest; only the front flag + presence matter.
type caaImage struct {
	Front bool   `json:"front"`
	Image string `json:"image"`
}

type caaManifest struct {
	Images []caaImage `json:"images"`
}

// FrontCover returns the front-cover reference for a release-group MBID, or
// (nil, nil) when the Archive has no front cover for it — a 404 (no art at all),
// or a manifest with no image flagged as the front. Both are normal "no cover"
// outcomes, not errors, so the enricher simply moves on to the next provider.
//
// The returned ImageURL is the Archive's STABLE redirect endpoint
// (coverartarchive.org/release-group/{mbid}/front) rather than the by-image-id
// direct URL, so the stored reference stays valid as the release-group's art is
// updated. The browser follows the redirect in <img src> (CSP already allows
// img-src https:; PSY-1175 D2).
func (c *CoverArtArchiveClient) FrontCover(ctx context.Context, releaseGroupMBID string) (*CoverArtResult, error) {
	id := strings.TrimSpace(releaseGroupMBID)
	if id == "" {
		return nil, fmt.Errorf("empty release-group mbid")
	}

	<-c.rateLimiter.C

	reqURL := fmt.Sprintf("%s/release-group/%s", c.baseURL, url.PathEscape(id))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating CAA request: %w", err)
	}
	req.Header.Set("User-Agent", caaUserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing CAA request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // deferred Close; nothing actionable on failure

	switch resp.StatusCode {
	case http.StatusOK:
		// fall through to parse
	case http.StatusNotFound:
		return nil, nil // no cover art for this release-group — a normal outcome
	default:
		return nil, fmt.Errorf("CAA returned status %d for release-group %s", resp.StatusCode, id)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading CAA response: %w", err)
	}

	var manifest caaManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return nil, fmt.Errorf("parsing CAA response: %w", err)
	}

	// Only store a cover when an image is explicitly flagged as the front — the
	// .../front endpoint 404s otherwise, so storing it would yield a broken <img>.
	hasFront := false
	for _, img := range manifest.Images {
		if img.Front && strings.TrimSpace(img.Image) != "" {
			hasFront = true
			break
		}
	}
	if !hasFront {
		return nil, nil
	}

	return &CoverArtResult{
		ImageURL:  fmt.Sprintf("%s/release-group/%s/front", c.baseURL, url.PathEscape(id)),
		SourceURL: fmt.Sprintf("%s/release-group/%s", c.mbWebURL, url.PathEscape(id)),
	}, nil
}
