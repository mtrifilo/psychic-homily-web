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

	"psychic-homily-backend/internal/utils"
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

// MBAreaRelations is the response from the MusicBrainz area lookup endpoint with
// `inc=area-rels`. Only the relations are decoded — enough to walk a City up to
// its parent Subdivision (US state), the hierarchy the artist search omits.
type MBAreaRelations struct {
	Relations []MBAreaRelation `json:"relations"`
}

// MBAreaRelation is one relationship on an area. MusicBrainz models a city's
// containing state as a `part of` relation whose linked Area is the parent
// Subdivision; the Direction label varies by how the edit was entered, so a
// caller identifies the parent by Area.Type ("Subdivision"), never Direction.
type MBAreaRelation struct {
	Type      string  `json:"type"`
	Direction string  `json:"direction"`
	Area      *MBArea `json:"area"`
}

// MBURLRelation is a single URL relationship on a MusicBrainz artist. The
// caller anchors on the parsed host of URL.Resource (NOT the Type string),
// because Spotify links arrive under several type labels ("free streaming",
// "streaming") and Bandcamp under "bandcamp" — host-anchoring is the robust,
// label-independent identity check.
type MBURLRelation struct {
	Type string `json:"type"`
	// Ended marks a relationship MusicBrainz considers no longer current (dead
	// link, delisted release). Enrichment writers skip ended relations.
	Ended bool `json:"ended"`
	URL   struct {
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

// IsValidMBID reports whether s is a canonical 36-char MusicBrainz UUID — the
// trust-boundary check before an MB-supplied id is stored. Delegates to the single
// shared implementation in utils (PSY-1281 consolidated the regex there so the
// pipeline and catalog copies can't drift); kept as a pipeline-level name because
// MB-result validation reads naturally at this layer.
func IsValidMBID(s string) bool {
	return utils.IsValidMBID(s)
}

// MusicBrainzClient provides rate-limited access to the MusicBrainz API.
type MusicBrainzClient struct {
	client    *http.Client
	baseURL   string // defaults to mbBaseURL; overridden in tests to point at httptest
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
		baseURL:   mbBaseURL,
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
	searchURL := fmt.Sprintf("%s/artist/?query=artist:%s&fmt=json&limit=5", c.baseURL, encodedName)

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
	searchURL := fmt.Sprintf("%s/artist/?query=artist:%s&fmt=json&limit=%d", c.baseURL, encodedName, mbCandidateLimit)

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

	lookupURL := fmt.Sprintf("%s/artist/%s?inc=url-rels&fmt=json", c.baseURL, url.PathEscape(mbid))

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

// MBArtistArtistRelations is the response from the MusicBrainz artist lookup
// with `inc=artist-rels`. Only the relations array is decoded (PSY-1382).
type MBArtistArtistRelations struct {
	Relations []MBArtistRelation `json:"relations"`
}

// MBRelatedArtist is the nested peer artist on an artist-rels row.
type MBRelatedArtist struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// MBArtistRelation is one artist↔artist relationship. MusicBrainz repeats the
// same pair once per instrument/attribute on "member of band", and may keep
// both ended + current rows for "is person" — callers must dedupe.
type MBArtistRelation struct {
	Type       string           `json:"type"`
	TypeID     string           `json:"type-id"`
	Direction  string           `json:"direction"`
	Ended      bool             `json:"ended"`
	Attributes []string         `json:"attributes"`
	Artist     *MBRelatedArtist `json:"artist"`
}

// LookupArtistArtistRelations fetches an artist's artist-artist relationships
// from MusicBrainz (`inc=artist-rels`). Used by PSY-1382 to backfill member_of
// / side_project edges. mbid is path-escaped; shares this client's ~1 req/s throttle.
func (c *MusicBrainzClient) LookupArtistArtistRelations(ctx context.Context, mbid string) ([]MBArtistRelation, error) {
	if err := c.throttle(ctx); err != nil {
		return nil, err
	}

	lookupURL := fmt.Sprintf("%s/artist/%s?inc=artist-rels&fmt=json", c.baseURL, url.PathEscape(mbid))

	body, err := c.doRequestCtx(ctx, lookupURL)
	if err != nil {
		return nil, fmt.Errorf("musicbrainz artist-rels lookup failed: %w", err)
	}

	var result MBArtistArtistRelations
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse musicbrainz artist-rels response: %w", err)
	}

	return result.Relations, nil
}

// LookupAreaRelations fetches an area's relationships (inc=area-rels) so a caller
// can walk from a City to its parent Subdivision — the US state the artist search
// response leaves out when it tags a band by city alone (PSY-1255). Shares this
// client's process-wide ~1 req/s throttle; areaID is path-escaped so a malformed
// value cannot alter the request target.
func (c *MusicBrainzClient) LookupAreaRelations(ctx context.Context, areaID string) ([]MBAreaRelation, error) {
	if err := c.throttle(ctx); err != nil {
		return nil, err
	}

	lookupURL := fmt.Sprintf("%s/area/%s?inc=area-rels&fmt=json", c.baseURL, url.PathEscape(areaID))

	body, err := c.doRequestCtx(ctx, lookupURL)
	if err != nil {
		return nil, fmt.Errorf("musicbrainz area-rels lookup failed: %w", err)
	}

	var result MBAreaRelations
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse musicbrainz area-rels response: %w", err)
	}

	return result.Relations, nil
}

// mbReleaseGroupSearchLimit is the default number of release-group candidates a
// cover-art search fetches. The caller applies a strict artist+title gate, so a
// modest list improves recall of the correct match without inflating cost.
const mbReleaseGroupSearchLimit = 10

// MBReleaseGroupSearchResponse is the response from the release-group search endpoint.
type MBReleaseGroupSearchResponse struct {
	ReleaseGroups []MBReleaseGroupResult `json:"release-groups"`
}

// MBReleaseGroupResult is a release-group (the album abstraction spanning all of
// an album's editions) from a MusicBrainz search. ID is the MBID the Cover Art
// Archive is keyed on (coverartarchive.org/release-group/{id}).
type MBReleaseGroupResult struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	PrimaryType string `json:"primary-type"` // "Album" | "EP" | "Single" | "Other" | "" (populated by the browse endpoint; the search path may leave it blank)
	// SecondaryTypes flags a special album that ALSO carries a primary type: a Live /
	// Compilation / Soundtrack / Remix / DJ-mix release-group is primary-type "Album"
	// (or "EP") PLUS the secondary type. The discography importer (PSY-1282) keeps only
	// the studio core, so it skips any release-group with a non-empty secondary list —
	// the primary-type filter alone is NOT sufficient to exclude them.
	SecondaryTypes   []string         `json:"secondary-types"`
	FirstReleaseDate string           `json:"first-release-date"` // "YYYY" | "YYYY-MM" | "YYYY-MM-DD" | ""
	ArtistCredit     []MBArtistCredit `json:"artist-credit"`
}

// MBArtistCredit is one credited artist on a release-group. Name is the credited
// name as it appears on the release (may be an alias / "feat." form); Artist.Name
// is the artist's canonical MusicBrainz name. The cover-art matcher checks both.
type MBArtistCredit struct {
	Name   string `json:"name"`
	Artist struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"artist"`
}

// SearchReleaseGroups searches MusicBrainz for release-groups matching an artist +
// release title, returning up to `limit` raw candidates with NO score/name filter
// — the caller applies its own strict gate (mirroring the SearchArtistCandidates
// contract, PSY-1191). The release-group MBID it returns is what the Cover Art
// Archive is keyed on. It shares this client's process-wide ~1 req/s throttle
// (PSY-1208) so cover-art enrichment and discovery never collectively exceed
// MusicBrainz's per-IP limit.
//
// artist and title are embedded as quoted Lucene phrases with any interior double
// quotes stripped, so a value can't break out of the field query. An empty artist
// or title returns no results without an API call.
func (c *MusicBrainzClient) SearchReleaseGroups(ctx context.Context, artist, title string, limit int) ([]MBReleaseGroupResult, error) {
	if strings.TrimSpace(artist) == "" || strings.TrimSpace(title) == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 100 {
		limit = mbReleaseGroupSearchLimit
	}

	if err := c.throttle(ctx); err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`artist:"%s" AND releasegroup:"%s"`, mbStripQuotes(artist), mbStripQuotes(title))
	searchURL := fmt.Sprintf("%s/release-group/?query=%s&fmt=json&limit=%d", c.baseURL, url.QueryEscape(query), limit)

	body, err := c.doRequestCtx(ctx, searchURL)
	if err != nil {
		return nil, fmt.Errorf("musicbrainz release-group search failed: %w", err)
	}

	var result MBReleaseGroupSearchResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse musicbrainz release-group response: %w", err)
	}
	return result.ReleaseGroups, nil
}

// mbStripQuotes removes interior double quotes from a Lucene phrase value so a
// title/artist containing a quote can't break out of the quoted field query.
func mbStripQuotes(s string) string {
	return strings.ReplaceAll(s, `"`, " ")
}

// MBBrowseReleaseGroupsResponse is the MusicBrainz browse response: the count drives
// pagination (browse caps 100 per page) and release-groups carries the page.
type MBBrowseReleaseGroupsResponse struct {
	Count         int                    `json:"release-group-count"`
	ReleaseGroups []MBReleaseGroupResult `json:"release-groups"`
}

// browseReleaseGroupPageSize is MusicBrainz's max browse page.
const browseReleaseGroupPageSize = 100

// browseReleaseGroupMaxPages caps one browse at 2,000 release-groups — far above any
// real artist's discography — so a bad/stale count field can't loop forever.
const browseReleaseGroupMaxPages = 20

// BrowseArtistReleaseGroups returns an artist's release-groups via the browse-by-MBID
// endpoint (/release-group?artist={mbid}), paginated to completion and filtered to the
// requested PRIMARY types (e.g. {"album":true,"ep":true}; nil/empty = all). It is the
// discography importer's source (PSY-1282): browse-by-MBID is identity-verified — the
// release-groups are THIS artist's, not a homonym's — so, unlike the title-search
// path, no name gate is needed.
//
// The primary-type filter is applied CLIENT-side (the response carries primary-type)
// rather than via the server `type` param, so the method neither depends on
// MusicBrainz's multi-value type-filter syntax nor loses the per-result type it needs
// for mapping. Shares the process-wide ~1 req/s throttle (PSY-1208). mbid is
// query-escaped so a malformed value can't alter the request target; an invalid one
// simply yields no results.
func (c *MusicBrainzClient) BrowseArtistReleaseGroups(ctx context.Context, mbid string, primaryTypes map[string]bool) ([]MBReleaseGroupResult, error) {
	if strings.TrimSpace(mbid) == "" {
		return nil, nil
	}

	var out []MBReleaseGroupResult
	offset := 0
	for page := 0; page < browseReleaseGroupMaxPages; page++ {
		if err := c.throttle(ctx); err != nil {
			return nil, err
		}
		browseURL := fmt.Sprintf("%s/release-group?artist=%s&fmt=json&limit=%d&offset=%d",
			c.baseURL, url.QueryEscape(mbid), browseReleaseGroupPageSize, offset)
		body, err := c.doRequestCtx(ctx, browseURL)
		if err != nil {
			return nil, fmt.Errorf("musicbrainz release-group browse failed: %w", err)
		}

		var resp MBBrowseReleaseGroupsResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("failed to parse musicbrainz browse response: %w", err)
		}

		for _, rg := range resp.ReleaseGroups {
			if len(primaryTypes) == 0 || primaryTypes[strings.ToLower(rg.PrimaryType)] {
				out = append(out, rg)
			}
		}

		offset += len(resp.ReleaseGroups)
		// Done when the whole set is walked, or a page came back empty (defensive: a
		// stale/missing count must not wedge the loop).
		if len(resp.ReleaseGroups) == 0 || offset >= resp.Count {
			break
		}
	}
	return out, nil
}

// browseReleasePageSize is MusicBrainz's max browse page for releases.
const browseReleasePageSize = 100

// browseReleaseMaxPages caps one release browse at 1,000 releases per
// release-group — a release-group rarely has more than a few dozen releases
// (pressings/regions), so this is a defensive bound, not a working limit.
const browseReleaseMaxPages = 10

// MBBrowseReleasesResponse is the paginated response from the MusicBrainz
// release browse endpoint (`/release?release-group={mbid}&inc=url-rels`).
type MBBrowseReleasesResponse struct {
	Count    int               `json:"release-count"`
	Releases []MBReleaseResult `json:"releases"`
}

// MBReleaseResult is one release in a browse-by-release-group result. Only the
// fields the link enrichment needs are decoded; Relations carries the url-rels.
type MBReleaseResult struct {
	ID        string          `json:"id"`
	Title     string          `json:"title"`
	Status    string          `json:"status"` // "Official", "Bootleg", …
	Date      string          `json:"date"`
	Relations []MBURLRelation `json:"relations"`
}

// BrowseReleaseURLRelations walks every release in a release-group (by RG-MBID)
// with url-rels included — one identity-verified call chain, no name search.
// MusicBrainz hangs streaming/purchase links on RELEASES, not release-groups
// (spiked 2026-07-01: RG-level url-rels were 0/50; release-level 35/50), so link
// enrichment must browse at this level. Rate-limited via the shared throttle;
// interruptible via ctx.
func (c *MusicBrainzClient) BrowseReleaseURLRelations(ctx context.Context, rgMBID string) ([]MBReleaseResult, error) {
	if strings.TrimSpace(rgMBID) == "" {
		return nil, nil
	}

	var out []MBReleaseResult
	offset := 0
	for page := 0; page < browseReleaseMaxPages; page++ {
		if err := c.throttle(ctx); err != nil {
			return nil, err
		}
		browseURL := fmt.Sprintf("%s/release?release-group=%s&inc=url-rels&fmt=json&limit=%d&offset=%d",
			c.baseURL, url.QueryEscape(rgMBID), browseReleasePageSize, offset)
		body, err := c.doRequestCtx(ctx, browseURL)
		if err != nil {
			return nil, fmt.Errorf("musicbrainz release browse failed: %w", err)
		}

		var resp MBBrowseReleasesResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("failed to parse musicbrainz release browse response: %w", err)
		}

		out = append(out, resp.Releases...)

		offset += len(resp.Releases)
		// Done when the whole set is walked, or a page came back empty (defensive:
		// a stale/missing count must not wedge the loop).
		if len(resp.Releases) == 0 || offset >= resp.Count {
			break
		}
	}
	return out, nil
}

// throttle enforces the rate limit, interruptibly. It blocks until the next
// request slot is available OR ctx is cancelled (whichever comes first). The
// legacy SearchArtist path passes context.Background(), preserving its prior
// behavior; the PSY-1191 discovery path passes the request context so a
// disconnected admin stops the per-call rate-limit wait instead of holding the
// lock for the full interval.
//
// CAVEAT (PSY-1208): the lock is held across the whole wait, so only the
// POST-acquisition wait is ctx-cancellable — acquiring c.mu is not. With one
// shared client across discovery + enrichment, a caller can block up to ~one
// rateLimit interval acquiring the lock behind an in-flight call on the shared
// client. That wait is BOUNDED (the HTTP round-trip runs outside the lock, and
// each throttle releases c.mu on return) — it is the intended cost of a true
// ~1 req/s process-wide limit, NOT a deadlock. Note the contending enrichment
// call (SearchArtist) holds the lock with an un-cancellable context.Background(),
// so even ctx cancellation on the waiting side can't shorten that ~1.1s hold.
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
