package pipeline

import (
	"context"
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
	// mbMinScore is the minimum score the LEGACY SearchArtist path accepts.
	// IMPORTANT: the PSY-1191 discovery path (SearchArtistCandidates) is
	// deliberately SCORE-FREE — it applies an exact-name gate downstream instead,
	// because a score filter discards a correct match buried under a higher-scored
	// famous namesake. Do not extend mbMinScore to the candidate path.
	mbMinScore = 90
	// mbCandidateLimit caps the number of search candidates the discovery flow
	// fetches per artist. The exact-name gate (PSY-1191) keeps only candidates
	// whose name normalizes-equals the query, so a generous list improves recall
	// of the correct match buried under junk top-hits without inflating cost —
	// each kept candidate triggers exactly one rate-limited url-rels lookup.
	mbCandidateLimit = 15
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
	// BeginArea is the artist's origin/founding location (a City for bands).
	// MusicBrainz tags `area` as the broad area (often a Country) and
	// `begin-area` as the specific origin city — both are useful signals for
	// the region-confidence tier (PSY-1191).
	BeginArea *MBArea `json:"begin-area"`
}

// MBArea represents a geographic area from MusicBrainz. Type is one of
// "Country", "Subdivision" (US state), "City", etc.
type MBArea struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// MBArtistURLRelations is the response from the MusicBrainz artist lookup
// endpoint with `inc=url-rels`. Only the relations array is decoded.
type MBArtistURLRelations struct {
	Relations []MBURLRelation `json:"relations"`
}

// MBURLRelation is a single URL relationship on a MusicBrainz artist. The
// caller anchors on the parsed host of URL.Resource (NOT the Type string),
// because Spotify links arrive under several type labels ("free streaming",
// "streaming") and Bandcamp under "bandcamp" — host-anchoring is the robust,
// label-independent identity check.
type MBURLRelation struct {
	Type string `json:"type"`
	URL  struct {
		Resource string `json:"resource"`
	} `json:"url"`
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
	// Legacy path keeps its un-cancellable behavior via a background context.
	_ = c.throttle(context.Background())

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

// SearchArtistCandidates returns the raw MusicBrainz search results for a name,
// up to mbCandidateLimit, WITHOUT applying the score or name filters that
// SearchArtist uses. The discovery flow (PSY-1191) deliberately needs the full
// candidate list so it can apply its own exact-name gate — a top-match/score
// filter would discard the correct match when it is buried under a higher-scored
// famous namesake (e.g. the real "Dylan Day" under a junk top-hit). Identity is
// decided downstream by name normalization, never by MB score.
func (c *MusicBrainzClient) SearchArtistCandidates(ctx context.Context, name string) ([]MBArtistResult, error) {
	if err := c.throttle(ctx); err != nil {
		return nil, err
	}

	encodedName := url.QueryEscape(name)
	searchURL := fmt.Sprintf("%s/artist/?query=artist:%s&fmt=json&limit=%d", mbBaseURL, encodedName, mbCandidateLimit)

	body, err := c.doRequestCtx(ctx, searchURL)
	if err != nil {
		return nil, fmt.Errorf("musicbrainz search failed: %w", err)
	}

	var result MBArtistSearchResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse musicbrainz response: %w", err)
	}

	return result.Artists, nil
}

// LookupArtistURLRelations fetches an artist's URL relationships from
// MusicBrainz (`inc=url-rels`) and returns the relation list. The caller
// extracts platform links by host-anchoring on each relation's URL.Resource.
// mbid is a MusicBrainz UUID returned by a prior search; it is URL-path-escaped
// so a malformed value cannot alter the request target.
func (c *MusicBrainzClient) LookupArtistURLRelations(ctx context.Context, mbid string) ([]MBURLRelation, error) {
	if err := c.throttle(ctx); err != nil {
		return nil, err
	}

	lookupURL := fmt.Sprintf("%s/artist/%s?inc=url-rels&fmt=json", mbBaseURL, url.PathEscape(mbid))

	body, err := c.doRequestCtx(ctx, lookupURL)
	if err != nil {
		return nil, fmt.Errorf("musicbrainz url-rels lookup failed: %w", err)
	}

	var result MBArtistURLRelations
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse musicbrainz url-rels response: %w", err)
	}

	return result.Relations, nil
}

// throttle enforces the rate limit, interruptibly. It blocks until the next
// request slot is available OR ctx is cancelled (whichever comes first). The
// legacy SearchArtist path passes context.Background(), preserving its prior
// behavior; the PSY-1191 discovery path passes the request context so a
// disconnected admin stops the per-call rate-limit wait instead of holding the
// lock for the full interval.
func (c *MusicBrainzClient) throttle(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	elapsed := time.Since(c.lastReq)
	if wait := c.rateLimit - elapsed; wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	c.lastReq = time.Now()
	return nil
}

// doRequest performs an HTTP GET with proper headers (legacy, un-cancellable).
func (c *MusicBrainzClient) doRequest(url string) ([]byte, error) {
	return c.doRequestCtx(context.Background(), url)
}

// doRequestCtx performs a context-bound HTTP GET with proper headers.
func (c *MusicBrainzClient) doRequestCtx(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", mbUserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // deferred Close; nothing actionable on failure

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
