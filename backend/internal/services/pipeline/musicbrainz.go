package pipeline

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	mbBaseURL   = "https://musicbrainz.org/ws/2"
	mbUserAgent = "PsychicHomily/1.0 (https://psychichomily.com)"
	// MusicBrainz rate limit: 1 request per second
	mbRateLimit = 1100 * time.Millisecond
	// Minimum score to accept a MusicBrainz match
	mbMinScore = 90
)

// MBArtistSearchResponse is the response from the MusicBrainz artist search endpoint.
type MBArtistSearchResponse struct {
	Artists []MBArtistResult `json:"artists"`
}

// MBArtistResult represents an artist from MusicBrainz search results.
type MBArtistResult struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	SortName       string  `json:"sort-name"`
	Score          int     `json:"score"`
	Disambiguation string  `json:"disambiguation"`
	Type           string  `json:"type"`
	Country        string  `json:"country"`
	Area           *MBArea `json:"area"`
}

// MBArea represents a geographic area from MusicBrainz.
type MBArea struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// MBLookupResult holds the result of a MusicBrainz artist lookup.
type MBLookupResult struct {
	MBID           string `json:"mbid"`
	Name           string `json:"name"`
	Score          int    `json:"score"`
	Disambiguation string `json:"disambiguation,omitempty"`
	Country        string `json:"country,omitempty"`
	Type           string `json:"type,omitempty"`
}

// MusicBrainzClient provides rate-limited access to the MusicBrainz API.
type MusicBrainzClient struct {
	client    *http.Client
	mu        sync.Mutex
	lastReq   time.Time
	rateLimit time.Duration
	minScore  int
}

// NewMusicBrainzClient creates a new rate-limited MusicBrainz API client.
func NewMusicBrainzClient() *MusicBrainzClient {
	return &MusicBrainzClient{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		rateLimit: mbRateLimit,
		minScore:  mbMinScore,
	}
}

// SearchArtist searches MusicBrainz for an artist by name.
// Returns the best match with score >= minScore, or nil if no match found.
func (c *MusicBrainzClient) SearchArtist(name string) (*MBLookupResult, error) {
	c.throttle()

	encodedName := url.QueryEscape(name)
	searchURL := fmt.Sprintf("%s/artist/?query=artist:%s&fmt=json&limit=5", mbBaseURL, encodedName)

	body, err := c.doRequest(searchURL)
	if err != nil {
		return nil, fmt.Errorf("musicbrainz search failed: %w", err)
	}

	var result MBArtistSearchResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse musicbrainz response: %w", err)
	}

	if len(result.Artists) == 0 {
		return nil, nil
	}

	// Find best match with score >= minScore and case-insensitive name match
	var bestMatch *MBArtistResult
	for i, a := range result.Artists {
		if a.Score < c.minScore {
			continue
		}
		if !strings.EqualFold(a.Name, name) {
			continue
		}
		if bestMatch == nil || a.Score > bestMatch.Score {
			bestMatch = &result.Artists[i]
		}
	}

	if bestMatch == nil {
		return nil, nil
	}

	return &MBLookupResult{
		MBID:           bestMatch.ID,
		Name:           bestMatch.Name,
		Score:          bestMatch.Score,
		Disambiguation: bestMatch.Disambiguation,
		Country:        bestMatch.Country,
		Type:           bestMatch.Type,
	}, nil
}

// throttle enforces the rate limit.
func (c *MusicBrainzClient) throttle() {
	c.mu.Lock()
	defer c.mu.Unlock()

	elapsed := time.Since(c.lastReq)
	if elapsed < c.rateLimit {
		time.Sleep(c.rateLimit - elapsed)
	}
	c.lastReq = time.Now()
}

// doRequest performs an HTTP GET with proper headers.
func (c *MusicBrainzClient) doRequest(url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", mbUserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable {
		return nil, fmt.Errorf("rate limited (HTTP %d)", resp.StatusCode)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return body, nil
}
