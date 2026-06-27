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

// Wikidata client for artist-photo enrichment (PSY-1232).
//
// Given a Wikidata entity id (a "Q…" id, reached from a MusicBrainz artist's
// Wikidata url-relation), this returns the entity's P18 "image" claim — a
// Wikimedia Commons filename. It uses the wbgetclaims API scoped to P18 so the
// response carries only that claim (no need to decode every property's value,
// which take heterogeneous shapes). The Commons client then resolves the filename
// to a hotlinkable URL + its license.

const (
	wikidataBaseURL   = "https://www.wikidata.org"
	wikidataUserAgent = "PsychicHomily/1.0 (artist-photo-enrichment; https://psychichomily.com)"
	wikidataTimeout   = 20 * time.Second
	// wikidataRateLimit spaces requests. Wikidata's action API has no hard
	// per-IP limit for light read use, but a small spacing keeps us polite.
	wikidataRateLimit = 100 * time.Millisecond
)

// WikidataClient resolves a Wikidata entity's P18 image claim.
type WikidataClient struct {
	httpClient  *http.Client
	baseURL     string
	rateLimiter *time.Ticker
}

// NewWikidataClient builds a production client pointed at the real Wikidata API.
func NewWikidataClient() *WikidataClient {
	return &WikidataClient{
		httpClient:  &http.Client{Timeout: wikidataTimeout},
		baseURL:     wikidataBaseURL,
		rateLimiter: time.NewTicker(wikidataRateLimit),
	}
}

// NewWikidataClientWithConfig points the client at a custom base URL (httptest)
// with a fast rate limiter. Exported for tests.
func NewWikidataClientWithConfig(httpClient *http.Client, baseURL string) *WikidataClient {
	return &WikidataClient{
		httpClient:  httpClient,
		baseURL:     baseURL,
		rateLimiter: time.NewTicker(1 * time.Millisecond),
	}
}

// Close stops the rate limiter ticker.
func (c *WikidataClient) Close() {
	if c.rateLimiter != nil {
		c.rateLimiter.Stop()
	}
}

// wikidataClaimsResponse is the wbgetclaims response, scoped to P18. P18's
// datavalue.value is a plain string (the Commons filename), so it decodes
// directly — unlike other property types whose value is an object.
type wikidataClaimsResponse struct {
	Claims map[string][]struct {
		Mainsnak struct {
			Datavalue struct {
				Value string `json:"value"`
			} `json:"datavalue"`
		} `json:"mainsnak"`
	} `json:"claims"`
}

// ImageFilename returns the Commons filename from a Wikidata entity's P18 image
// claim, or "" when the entity has no image. qid is a "Q…" id (path-escaped so a
// malformed value cannot alter the request target).
func (c *WikidataClient) ImageFilename(ctx context.Context, qid string) (string, error) {
	id := strings.TrimSpace(qid)
	if id == "" {
		return "", fmt.Errorf("empty wikidata qid")
	}

	<-c.rateLimiter.C

	params := url.Values{}
	params.Set("action", "wbgetclaims")
	params.Set("entity", id)
	params.Set("property", "P18")
	params.Set("format", "json")

	reqURL := c.baseURL + "/w/api.php?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return "", fmt.Errorf("creating wikidata request: %w", err)
	}
	req.Header.Set("User-Agent", wikidataUserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("executing wikidata request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // deferred Close; nothing actionable on failure

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("wikidata returned status %d for %s", resp.StatusCode, id)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading wikidata response: %w", err)
	}

	var parsed wikidataClaimsResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("parsing wikidata response: %w", err)
	}

	p18 := parsed.Claims["P18"]
	if len(p18) == 0 {
		return "", nil // no image claim — a normal outcome
	}
	return strings.TrimSpace(p18[0].Mainsnak.Datavalue.Value), nil
}
