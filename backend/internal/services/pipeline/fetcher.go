package pipeline

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"time"

	"psychic-homily-backend/internal/services/contracts"
)

// FetcherService handles HTTP fetching with ETag/hash-based change detection.
// Optionally supports chromedp-based rendering for JS-heavy pages (call InitChromedp to enable).
type FetcherService struct {
	httpClient  *http.Client
	allocCtx    context.Context    // chromedp exec allocator context (nil until InitChromedp)
	allocCancel context.CancelFunc // cancels the allocator on shutdown
	workerSem   chan struct{}      // semaphore limiting concurrent Chrome tabs
}

// NewFetcherService creates a new FetcherService with a 30-second timeout
// and redirect capture.
func NewFetcherService() *FetcherService {
	return NewFetcherServiceWithTimeout(30 * time.Second)
}

// NewFetcherServiceWithTimeout creates a new FetcherService with a custom timeout.
// Exported for testing with short timeouts.
func NewFetcherServiceWithTimeout(timeout time.Duration) *FetcherService {
	client := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Don't follow redirects — we capture them manually
			return http.ErrUseLastResponse
		},
	}
	return &FetcherService{
		httpClient: client,
	}
}

// Fetch performs an HTTP GET to url with conditional request support.
// If lastETag is non-empty, an If-None-Match header is sent.
// If lastContentHash is non-empty, it is compared against the SHA256 of the
// response body to detect content changes even without ETag support.
func (s *FetcherService) Fetch(url string, lastETag string, lastContentHash string) (*contracts.FetchResult, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("User-Agent", "PsychicHomily/1.0 (venue-calendar-indexer)")

	if lastETag != "" {
		req.Header.Set("If-None-Match", lastETag)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", url, err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusMovedPermanently || resp.StatusCode == http.StatusPermanentRedirect:
		location := resp.Header.Get("Location")
		return &contracts.FetchResult{
			HTTPStatus:  resp.StatusCode,
			RedirectURL: location,
			Changed:     true,
		}, nil

	case resp.StatusCode == http.StatusNotModified:
		return &contracts.FetchResult{
			HTTPStatus: http.StatusNotModified,
			Changed:    false,
		}, nil

	case resp.StatusCode == http.StatusOK:
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("reading response body from %s: %w", url, err)
		}

		bodyStr := string(body)
		hash := computeContentHash(bodyStr)
		responseETag := resp.Header.Get("ETag")
		contentType := resp.Header.Get("Content-Type")

		changed := lastContentHash == "" || hash != lastContentHash

		result := &contracts.FetchResult{
			HTTPStatus:  http.StatusOK,
			Changed:     changed,
			ContentHash: hash,
			ETag:        responseETag,
			ContentType: contentType,
		}

		if changed {
			result.Body = bodyStr
		}

		return result, nil

	case resp.StatusCode == http.StatusForbidden:
		return nil, fmt.Errorf("HTTP 403 Forbidden fetching %s", url)

	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, fmt.Errorf("HTTP 429 Too Many Requests fetching %s", url)

	case resp.StatusCode >= 500:
		return nil, fmt.Errorf("HTTP %d server error fetching %s", resp.StatusCode, url)

	default:
		return nil, fmt.Errorf("unexpected HTTP %d fetching %s", resp.StatusCode, url)
	}
}

// computeContentHash returns the SHA256 hex digest of content.
func computeContentHash(content string) string {
	h := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", h)
}
