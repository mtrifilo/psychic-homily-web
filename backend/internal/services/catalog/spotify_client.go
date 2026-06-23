package catalog

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Spotify Web API client for the image-enrichment backfill (PSY-1185).
//
// This is the TRANSPORT layer only: it speaks the client-credentials (app-only)
// auth flow, fetches/caches an app access token, and exposes the album search +
// artist lookup the enricher needs. It knows Spotify's wire format and nothing
// about our image-matching policy — the strict-match gate lives in the enricher
// (spotify_image_enricher.go), so the matching rules can be unit-tested without
// HTTP and the transport can be reused for any other Spotify need later.
//
// We store only references to Spotify-hosted images (URL + linkback), never the
// bytes — see the PSY-1175 architecture decision doc (D1/D3).

const (
	spotifyAPIBaseURL      = "https://api.spotify.com/v1"
	spotifyAccountsBaseURL = "https://accounts.spotify.com"
	spotifyUserAgent       = "PsychicHomily/1.0 (image-enrichment)"
	spotifyDefaultTimeout  = 20 * time.Second
	// spotifyRateLimit paces requests well under Spotify's rolling-window limit.
	// The backfill is not latency-sensitive, so a conservative cadence avoids 429s.
	spotifyRateLimit = 150 * time.Millisecond
	// spotifyTokenSafetyMargin is shaved off the token's reported lifetime so we
	// refresh slightly early and never send an about-to-expire token.
	spotifyTokenSafetyMargin = 60 * time.Second
	// spotify429MaxWait caps how long we honor a Retry-After before giving up, so
	// a pathological/hostile value cannot stall the backfill indefinitely.
	spotify429MaxWait = 30 * time.Second
	// spotifyDefaultSearchLimit is how many album candidates we ask for; the
	// enricher's strict gate filters them down (we never store the top hit blindly).
	spotifyDefaultSearchLimit = 10
	// spotifyErrorBodyLimit caps the response body retained in an error message.
	spotifyErrorBodyLimit = 512
)

// SpotifyClient is a minimal Spotify Web API client using the client-credentials
// flow (no user auth). Safe for sequential use by the backfill; the token cache
// is mutex-guarded so it is also safe under concurrent callers.
type SpotifyClient struct {
	httpClient   *http.Client
	apiBaseURL   string
	accountsURL  string
	rateLimiter  *time.Ticker
	clientID     string
	clientSecret string

	mu          sync.Mutex
	accessToken string
	tokenExpiry time.Time
}

// NewSpotifyClient builds a production Spotify client pointed at the real API.
func NewSpotifyClient(clientID, clientSecret string) *SpotifyClient {
	return &SpotifyClient{
		httpClient:   &http.Client{Timeout: spotifyDefaultTimeout},
		apiBaseURL:   spotifyAPIBaseURL,
		accountsURL:  spotifyAccountsBaseURL,
		rateLimiter:  time.NewTicker(spotifyRateLimit),
		clientID:     clientID,
		clientSecret: clientSecret,
	}
}

// NewSpotifyClientWithConfig points the client at custom base URLs (httptest
// servers) with a fast rate limiter. Exported for tests.
func NewSpotifyClientWithConfig(httpClient *http.Client, apiBaseURL, accountsURL, clientID, clientSecret string) *SpotifyClient {
	return &SpotifyClient{
		httpClient:   httpClient,
		apiBaseURL:   apiBaseURL,
		accountsURL:  accountsURL,
		rateLimiter:  time.NewTicker(1 * time.Millisecond), // fast for tests
		clientID:     clientID,
		clientSecret: clientSecret,
	}
}

// Close stops the rate limiter ticker. Call when the client is no longer needed.
func (c *SpotifyClient) Close() {
	if c.rateLimiter != nil {
		c.rateLimiter.Stop()
	}
}

// =============================================================================
// Public API
// =============================================================================

// SpotifyImage is one rendition of a Spotify image (album cover / artist photo).
type SpotifyImage struct {
	URL    string `json:"url"`
	Height int    `json:"height"`
	Width  int    `json:"width"`
}

// SpotifyExternalURLs carries the public web links Spotify attaches to objects;
// the `spotify` URL is the deep linkback we persist for attribution.
type SpotifyExternalURLs struct {
	Spotify string `json:"spotify"`
}

// SpotifyAlbumArtistRef is the trimmed artist reference on an album object.
// ID is Spotify's stable artist id — the enricher anchors on it (not the name)
// when the catalog artist has a curated Spotify link, so two distinct artists
// who share a name can't be confused.
type SpotifyAlbumArtistRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// SpotifyAlbum is the subset of a Spotify album object the enricher needs.
type SpotifyAlbum struct {
	ID           string                  `json:"id"`
	Name         string                  `json:"name"`
	Artists      []SpotifyAlbumArtistRef `json:"artists"`
	ReleaseDate  string                  `json:"release_date"` // "YYYY" | "YYYY-MM" | "YYYY-MM-DD"
	Images       []SpotifyImage          `json:"images"`
	ExternalURLs SpotifyExternalURLs     `json:"external_urls"`
}

// SpotifyArtist is the subset of a Spotify artist object the enricher needs.
type SpotifyArtist struct {
	ID           string              `json:"id"`
	Name         string              `json:"name"`
	Images       []SpotifyImage      `json:"images"`
	ExternalURLs SpotifyExternalURLs `json:"external_urls"`
}

type spotifySearchResponse struct {
	Albums struct {
		Items []SpotifyAlbum `json:"items"`
	} `json:"albums"`
}

// SearchAlbums searches Spotify for albums matching the artist + album title,
// using field filters (album:/artist:) for precision. Returns up to `limit`
// candidates in Spotify's relevance order; the enricher applies the strict-match
// policy. An empty artist or title returns no results without an API call.
func (c *SpotifyClient) SearchAlbums(artist, title string, limit int) ([]SpotifyAlbum, error) {
	if strings.TrimSpace(artist) == "" || strings.TrimSpace(title) == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 50 {
		limit = spotifyDefaultSearchLimit
	}

	params := url.Values{}
	params.Set("q", fmt.Sprintf(`album:"%s" artist:"%s"`, sanitizeQueryValue(title), sanitizeQueryValue(artist)))
	params.Set("type", "album")
	params.Set("limit", strconv.Itoa(limit))
	// market scopes results to US availability — tightens matches for a
	// US-centric catalog (a deliberate choice, not boilerplate).
	params.Set("market", "US")

	body, err := c.apiGet("/search?" + params.Encode())
	if err != nil {
		return nil, err
	}

	var sr spotifySearchResponse
	if err := json.Unmarshal(body, &sr); err != nil {
		return nil, fmt.Errorf("parsing search response: %w", err)
	}
	return sr.Albums.Items, nil
}

// GetArtist fetches a Spotify artist by its id (the segment in an
// open.spotify.com/artist/<id> link). Used for the high-confidence artist-photo
// path: the link is operator-curated, so the id is an exact match by construction.
func (c *SpotifyClient) GetArtist(spotifyID string) (*SpotifyArtist, error) {
	id := strings.TrimSpace(spotifyID)
	if id == "" {
		return nil, fmt.Errorf("empty spotify artist id")
	}

	body, err := c.apiGet("/artists/" + url.PathEscape(id))
	if err != nil {
		return nil, err
	}

	var a SpotifyArtist
	if err := json.Unmarshal(body, &a); err != nil {
		return nil, fmt.Errorf("parsing artist response: %w", err)
	}
	return &a, nil
}

// =============================================================================
// Internal helpers
// =============================================================================

type spotifyTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// ensureToken returns a cached app access token, fetching a fresh one via the
// client-credentials flow when the cache is empty or expired.
func (c *SpotifyClient) ensureToken() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.accessToken != "" && time.Now().Before(c.tokenExpiry) {
		return c.accessToken, nil
	}

	form := url.Values{}
	form.Set("grant_type", "client_credentials")

	req, err := http.NewRequest(http.MethodPost, c.accountsURL+"/api/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("creating token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", spotifyUserAgent)
	req.SetBasicAuth(c.clientID, c.clientSecret)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("requesting token: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // deferred Close; nothing actionable on failure

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading token response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("spotify token endpoint returned status %d: %s", resp.StatusCode, spotifyTruncateBody(string(body)))
	}

	var tok spotifyTokenResponse
	if err := json.Unmarshal(body, &tok); err != nil {
		return "", fmt.Errorf("parsing token response: %w", err)
	}
	if tok.AccessToken == "" {
		return "", fmt.Errorf("spotify token response had empty access_token")
	}

	c.accessToken = tok.AccessToken
	ttl := time.Duration(tok.ExpiresIn) * time.Second
	if ttl > spotifyTokenSafetyMargin {
		ttl -= spotifyTokenSafetyMargin
	}
	c.tokenExpiry = time.Now().Add(ttl)
	return c.accessToken, nil
}

// apiGet performs an authenticated GET against the Spotify API, retrying once on
// a 429 after honoring the Retry-After header.
func (c *SpotifyClient) apiGet(path string) ([]byte, error) {
	return c.apiGetWithRetry(path, true)
}

func (c *SpotifyClient) apiGetWithRetry(path string, allowRetry bool) ([]byte, error) {
	token, err := c.ensureToken()
	if err != nil {
		return nil, err
	}

	<-c.rateLimiter.C

	req, err := http.NewRequest(http.MethodGet, c.apiBaseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", spotifyUserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}

	// 429: drain + close this body explicitly (we recurse before any defer would
	// fire), wait out Retry-After, and retry exactly once.
	if resp.StatusCode == http.StatusTooManyRequests && allowRetry {
		wait := parseSpotifyRetryAfter(resp.Header.Get("Retry-After"))
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		if wait > 0 {
			time.Sleep(wait)
		}
		return c.apiGetWithRetry(path, false)
	}

	defer resp.Body.Close() //nolint:errcheck // deferred Close; nothing actionable on failure

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("spotify API %s returned status %d: %s", path, resp.StatusCode, spotifyTruncateBody(string(body)))
	}
	return body, nil
}

// parseSpotifyRetryAfter reads a Retry-After header. Per RFC 9110 §10.2.3 it may
// be EITHER delta-seconds ("120") OR an HTTP-date ("Wed, 21 Oct 2025 07:28:00
// GMT"); we handle both forms. The result is clamped to spotify429MaxWait so a
// pathological value can't stall the backfill, and a non-empty header we cannot
// parse falls back to the rate-limit cadence WITH a debug log (so a future
// header-format change is observable rather than silently swallowed).
func parseSpotifyRetryAfter(h string) time.Duration {
	h = strings.TrimSpace(h)
	if h == "" {
		return spotifyRateLimit
	}
	if secs, err := strconv.Atoi(h); err == nil {
		return clampRetryWait(time.Duration(secs) * time.Second)
	}
	if t, err := http.ParseTime(h); err == nil {
		return clampRetryWait(time.Until(t))
	}
	slog.Debug("spotify: unparseable Retry-After header; using default backoff", "retry_after", h)
	return spotifyRateLimit
}

// clampRetryWait floors a backoff at 0 (past dates / negative deltas → no sleep)
// and caps it at spotify429MaxWait.
func clampRetryWait(d time.Duration) time.Duration {
	if d <= 0 {
		return 0
	}
	if d > spotify429MaxWait {
		return spotify429MaxWait
	}
	return d
}

// sanitizeQueryValue strips the double quotes we wrap field-filter values in, so
// a title/artist containing a quote cannot break out of the `album:"..."` filter.
func sanitizeQueryValue(s string) string {
	return strings.ReplaceAll(s, `"`, " ")
}

// spotifyTruncateBody caps an error-body slice so a multi-KB error page does not
// bloat the error chain / logs.
func spotifyTruncateBody(s string) string {
	if len(s) > spotifyErrorBodyLimit {
		return s[:spotifyErrorBodyLimit] + "...[truncated]"
	}
	return s
}

// bestImageURL returns the widest available image URL (Spotify orders images
// widest-first, but we select by max width defensively). Empty when none.
func bestImageURL(images []SpotifyImage) string {
	best := ""
	bestW := -1
	for _, img := range images {
		if img.URL == "" {
			continue
		}
		if img.Width > bestW {
			bestW = img.Width
			best = img.URL
		}
	}
	return best
}
