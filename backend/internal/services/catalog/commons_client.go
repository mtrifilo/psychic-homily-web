package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Wikimedia Commons client for artist-photo enrichment (PSY-1232).
//
// Given a Commons filename (from a Wikidata P18 claim), this resolves a
// hotlinkable image URL + its attribution (license + author) via the imageinfo
// API. We store only a REFERENCE to the Commons-hosted image (PSY-1175 D1/D3),
// hotlinked with CC attribution; CC-BY / CC-BY-SA permit this with author +
// license + linkback. It is fail-closed on licensing: an image whose license is
// NOT clearly reusable (a fair-use / all-rights-reserved file) is dropped here, so
// only freely-licensed photos ever reach the enricher.

const (
	commonsBaseURL   = "https://commons.wikimedia.org"
	commonsImageHost = "upload.wikimedia.org"
	commonsUserAgent = "PsychicHomily/1.0 (artist-photo-enrichment; https://psychichomily.com)"
	commonsTimeout   = 20 * time.Second
	commonsRateLimit = 100 * time.Millisecond
	// commonsThumbWidth requests a display-sized thumbnail rather than hotlinking
	// the full-res original (which can be many MB).
	commonsThumbWidth = 600
)

// CommonsClient resolves a Commons filename to a hotlinkable image + attribution.
type CommonsClient struct {
	httpClient  *http.Client
	baseURL     string
	rateLimiter *time.Ticker
}

// NewCommonsClient builds a production client pointed at the real Commons API.
func NewCommonsClient() *CommonsClient {
	return &CommonsClient{
		httpClient:  &http.Client{Timeout: commonsTimeout},
		baseURL:     commonsBaseURL,
		rateLimiter: time.NewTicker(commonsRateLimit),
	}
}

// NewCommonsClientWithConfig points the client at a custom base URL (httptest)
// with a fast rate limiter. Exported for tests.
func NewCommonsClientWithConfig(httpClient *http.Client, baseURL string) *CommonsClient {
	return &CommonsClient{
		httpClient:  httpClient,
		baseURL:     baseURL,
		rateLimiter: time.NewTicker(1 * time.Millisecond),
	}
}

// Close stops the rate limiter ticker.
func (c *CommonsClient) Close() {
	if c.rateLimiter != nil {
		c.rateLimiter.Stop()
	}
}

// CommonsImage is a resolved, hotlinkable Commons image with its attribution.
type CommonsImage struct {
	ImageURL       string // https upload.wikimedia.org thumbnail (or original) URL
	DescriptionURL string // the Commons file page — the attribution linkback
	License        string // e.g. "CC BY-SA 4.0", "Public domain"
	Author         string // photographer credit, HTML stripped ("" when none)
}

type commonsResponse struct {
	Query struct {
		Pages map[string]struct {
			Imageinfo []struct {
				URL            string `json:"url"`
				DescriptionURL string `json:"descriptionurl"`
				ThumbURL       string `json:"thumburl"`
				Extmetadata    struct {
					LicenseShortName struct {
						Value string `json:"value"`
					} `json:"LicenseShortName"`
					Artist struct {
						Value string `json:"value"`
					} `json:"Artist"`
				} `json:"extmetadata"`
			} `json:"imageinfo"`
		} `json:"pages"`
	} `json:"query"`
}

// ImageInfo resolves a Commons filename to a hotlinkable image + attribution, or
// (nil, nil) when the file is missing OR carries a non-reusable license. filename
// is a bare Commons filename (no "File:" prefix). Only freely-licensed images are
// returned — a fair-use / all-rights-reserved file is dropped (fail-closed).
func (c *CommonsClient) ImageInfo(ctx context.Context, filename string) (*CommonsImage, error) {
	name := strings.TrimSpace(filename)
	if name == "" {
		return nil, fmt.Errorf("empty commons filename")
	}

	<-c.rateLimiter.C

	params := url.Values{}
	params.Set("action", "query")
	params.Set("titles", "File:"+name)
	params.Set("prop", "imageinfo")
	params.Set("iiprop", "url|extmetadata")
	params.Set("iiurlwidth", strconv.Itoa(commonsThumbWidth))
	params.Set("format", "json")

	reqURL := c.baseURL + "/w/api.php?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating commons request: %w", err)
	}
	req.Header.Set("User-Agent", commonsUserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing commons request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // deferred Close; nothing actionable on failure

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("commons returned status %d for %s", resp.StatusCode, name)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading commons response: %w", err)
	}

	var parsed commonsResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parsing commons response: %w", err)
	}

	for _, page := range parsed.Query.Pages {
		if len(page.Imageinfo) == 0 {
			continue // a "missing" page has no imageinfo
		}
		ii := page.Imageinfo[0]
		license := strings.TrimSpace(ii.Extmetadata.LicenseShortName.Value)
		if !isReusableLicense(license) {
			return nil, nil // not freely licensed — drop it
		}
		img := ii.ThumbURL
		if img == "" {
			img = ii.URL
		}
		if img == "" {
			return nil, nil
		}
		return &CommonsImage{
			ImageURL:       img,
			DescriptionURL: ii.DescriptionURL,
			License:        license,
			Author:         stripCommonsHTML(ii.Extmetadata.Artist.Value),
		}, nil
	}
	return nil, nil // no usable page
}

// isReusableLicense reports whether a Commons LicenseShortName denotes a license
// we may hotlink + display with attribution. Fail-closed: anything not clearly
// free (CC-BY/BY-SA any version, CC0, public domain) is rejected — a fair-use,
// all-rights-reserved, or unrecognized license never gets stored.
func isReusableLicense(lic string) bool {
	l := strings.ToLower(strings.TrimSpace(lic))
	switch {
	case l == "":
		return false
	// Reject NonCommercial / NoDerivatives variants BEFORE the broad "cc by" accept:
	// "CC BY-NC*" forbids our (commercial-tier) use, and "CC BY-ND*" forbids the
	// resized thumbnail this code stores (a derivative). Both start with "cc by".
	case strings.Contains(l, "-nc") || strings.Contains(l, "-nd") ||
		strings.Contains(l, "noncommercial") || strings.Contains(l, "noderiv"):
		return false
	case strings.HasPrefix(l, "cc by"): // CC BY and CC BY-SA, all versions
		return true
	case strings.HasPrefix(l, "cc0"):
		return true
	case strings.Contains(l, "public domain"):
		return true
	case l == "pdm" || strings.HasPrefix(l, "pd-"):
		return true
	default:
		return false
	}
}

var commonsHTMLTagRE = regexp.MustCompile(`<[^>]*>`)

// stripCommonsHTML reduces the HTML "Artist" field Commons returns (often an
// anchor or span) to a plain author name: unescape entities FIRST, then drop any
// (now-literal) tags, then collapse whitespace. Order matters — unescaping before
// stripping means an entity-encoded tag (e.g. "&lt;script&gt;") becomes a real tag
// and is removed, so the stored value can never contain markup (defense for any
// future non-React consumer of image_author).
func stripCommonsHTML(s string) string {
	s = html.UnescapeString(s)
	s = commonsHTMLTagRE.ReplaceAllString(s, "")
	return strings.TrimSpace(strings.Join(strings.Fields(s), " "))
}
