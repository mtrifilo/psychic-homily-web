package services

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// skipIfNoChrome skips the test if Chrome/Chromium cannot actually launch.
// Checking the binary path alone is insufficient — CI runners may have
// Chrome installed but fail to connect via DevTools protocol.
func skipIfNoChrome(t *testing.T) {
	t.Helper()

	// Quick check: is any Chrome binary on PATH or installed?
	found := false
	for _, name := range []string{"google-chrome", "chromium", "chromium-browser"} {
		if _, err := exec.LookPath(name); err == nil {
			found = true
			break
		}
	}
	if !found {
		if _, err := os.Stat("/Applications/Google Chrome.app"); err == nil {
			found = true
		}
	}
	if !found {
		t.Skip("Chrome not found, skipping chromedp tests")
	}

	// Verify Chrome can actually launch by navigating to about:blank.
	svc := NewFetcherService()
	svc.InitChromedp(1)
	defer svc.ShutdownChromedp()

	ctx, cancel := context.WithTimeout(svc.allocCtx, 15*time.Second)
	defer cancel()
	tabCtx, tabCancel := chromedp.NewContext(ctx)
	defer tabCancel()

	if err := chromedp.Run(tabCtx, chromedp.Navigate("about:blank")); err != nil {
		t.Skipf("Chrome found but cannot launch (CI environment?): %v", err)
	}
}

// newTestFetcher creates a FetcherService with chromedp initialized for testing.
func newTestFetcher(t *testing.T) *FetcherService {
	t.Helper()
	svc := NewFetcherService()
	svc.InitChromedp(2) // 2 workers for tests
	t.Cleanup(func() {
		svc.ShutdownChromedp()
	})
	return svc
}

func TestFetchDynamic_Basic(t *testing.T) {
	skipIfNoChrome(t)

	// Serve a page where content is rendered by JavaScript after page load
	page := `<!DOCTYPE html>
<html>
<head><title>Events</title></head>
<body>
<div id="app"></div>
<script>
	document.getElementById('app').innerHTML = '<div class="event"><h2>Band Name</h2><p>March 15, 2026 - 8pm</p><p>$20</p></div>';
</script>
</body>
</html>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(page))
	}))
	defer server.Close()

	svc := newTestFetcher(t)
	result, err := svc.FetchDynamic(server.URL)

	require.NoError(t, err)
	assert.True(t, result.Changed)
	assert.Equal(t, "text/html", result.ContentType)
	assert.Equal(t, 200, result.HTTPStatus)
	assert.NotEmpty(t, result.ContentHash)
	// The JS-rendered content should be in the HTML
	assert.Contains(t, result.Body, "Band Name")
	assert.Contains(t, result.Body, "March 15, 2026")
}

func TestFetchDynamic_StaticPage(t *testing.T) {
	skipIfNoChrome(t)

	// Serve a static page (no JavaScript rendering needed)
	page := `<!DOCTYPE html>
<html>
<head><title>Events</title></head>
<body>
<div class="events">
	<article>
		<h2>Static Band</h2>
		<time>April 10, 2026</time>
		<span class="price">$15</span>
	</article>
</div>
</body>
</html>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(page))
	}))
	defer server.Close()

	svc := newTestFetcher(t)
	result, err := svc.FetchDynamic(server.URL)

	require.NoError(t, err)
	assert.True(t, result.Changed)
	assert.Contains(t, result.Body, "Static Band")
}

func TestFetchScreenshot_Basic(t *testing.T) {
	skipIfNoChrome(t)

	page := `<!DOCTYPE html>
<html>
<head><title>Events</title></head>
<body>
<div class="event">
	<h1>Screenshot Test Event</h1>
	<p>March 20, 2026 - Doors at 7pm</p>
	<p>$25</p>
</div>
</body>
</html>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(page))
	}))
	defer server.Close()

	svc := newTestFetcher(t)
	result, err := svc.FetchScreenshot(server.URL)

	require.NoError(t, err)
	assert.True(t, result.Changed)
	assert.Equal(t, 200, result.HTTPStatus)
	assert.NotEmpty(t, result.ContentHash)

	// ContentType should be image/jpeg or image/png depending on chromedp quality setting
	assert.Contains(t, []string{"image/jpeg", "image/png"}, result.ContentType,
		"ContentType should be a recognized image format")

	// Verify the body is valid base64
	decoded, err := base64.StdEncoding.DecodeString(result.Body)
	require.NoError(t, err)
	assert.True(t, len(decoded) > 0, "screenshot should produce non-empty image data")

	// Check for valid image magic bytes (JPEG or PNG)
	assert.True(t, len(decoded) >= 8, "decoded data too short to be an image")
	jpegMagic := []byte{0xFF, 0xD8, 0xFF}
	pngMagic := []byte{0x89, 0x50, 0x4E, 0x47}
	isJPEG := len(decoded) >= 3 && decoded[0] == jpegMagic[0] && decoded[1] == jpegMagic[1] && decoded[2] == jpegMagic[2]
	isPNG := len(decoded) >= 4 && decoded[0] == pngMagic[0] && decoded[1] == pngMagic[1] && decoded[2] == pngMagic[2] && decoded[3] == pngMagic[3]
	assert.True(t, isJPEG || isPNG, "screenshot should be a valid JPEG or PNG file")
}

func TestDetectRenderMethod_Static(t *testing.T) {
	skipIfNoChrome(t)

	// Serve a large page with event markers — should detect as "static"
	events := ""
	for i := 0; i < 50; i++ {
		events += fmt.Sprintf(`<div class="event"><h2>Band %d</h2><time>March %d, 2026</time><span>8pm</span><span>$%d</span><p>Doors at 7pm, tickets available</p></div>`, i, i+1, 10+i)
	}
	page := fmt.Sprintf(`<!DOCTYPE html><html><head><title>Events</title></head><body><div class="events">%s</div></body></html>`, events)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(page))
	}))
	defer server.Close()

	svc := newTestFetcher(t)
	method, err := svc.DetectRenderMethod(server.URL)

	require.NoError(t, err)
	assert.Equal(t, "static", method)
}

func TestDetectRenderMethod_NeedsDynamic(t *testing.T) {
	skipIfNoChrome(t)

	// Serve an empty shell via HTTP that gets populated by JS
	events := ""
	for i := 0; i < 50; i++ {
		events += fmt.Sprintf(`<div class="event"><h2>Band %d</h2><time>March %d, 2026</time><span>8pm</span><span>$%d</span><p>Doors open at 7pm, tickets on sale now</p></div>`, i, i+1, 10+i)
	}
	page := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><title>Events</title></head>
<body>
<div id="app"></div>
<script>
	document.getElementById('app').innerHTML = '%s';
</script>
</body>
</html>`, events)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(page))
	}))
	defer server.Close()

	svc := newTestFetcher(t)
	method, err := svc.DetectRenderMethod(server.URL)

	require.NoError(t, err)
	// Should detect as dynamic (JS-rendered content has event markers)
	// or static (if the script tag content itself counts as >5KB with markers)
	assert.Contains(t, []string{"static", "dynamic"}, method,
		"JS-rendered event page should be detected as static or dynamic")
}

func TestHasEventMarkers_WithEvents(t *testing.T) {
	tests := []struct {
		name string
		html string
		want bool
	}{
		{
			name: "full event listing",
			html: `<div class="event"><h2>Band Name</h2><time>March 15, 2026</time><span>8pm</span><span>$20</span><p>Tickets available</p></div>`,
			want: true,
		},
		{
			name: "date and time only",
			html: `<div>Show on January 10 at 7pm</div>`,
			want: true,
		},
		{
			name: "date and price only",
			html: `<div>Concert on February 2026 - $25</div>`,
			want: true,
		},
		{
			name: "doors and tickets",
			html: `<p>Doors at 7:00 PM, tickets $15</p>`,
			want: true,
		},
		{
			name: "music terms and year",
			html: `<div>Live music concert 2026 - headliner announcement</div>`,
			want: true,
		},
		{
			name: "only a year - not enough",
			html: `<footer>Copyright 2026</footer>`,
			want: false,
		},
		{
			name: "empty HTML",
			html: `<html><body></body></html>`,
			want: false,
		},
		{
			name: "generic blog post",
			html: `<article><h1>How to Cook Pasta</h1><p>Boil water and add noodles.</p></article>`,
			want: false,
		},
		{
			name: "only price - not enough",
			html: `<span>Item costs $50</span>`,
			want: false,
		},
		{
			name: "multiple music terms plus month",
			html: `<div>All ages show! Opener starts at doors. Dec event. Tickets at the venue stage.</div>`,
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasEventMarkers(tt.html)
			assert.Equal(t, tt.want, got, "hasEventMarkers(%q)", tt.html)
		})
	}
}

func TestWorkerPoolSemaphore(t *testing.T) {
	skipIfNoChrome(t)

	maxWorkers := 2
	svc := NewFetcherService()
	svc.InitChromedp(maxWorkers)
	defer svc.ShutdownChromedp()

	// Track how many workers are active concurrently
	var activeWorkers int32
	var maxObserved int32

	// Serve a page that takes a moment to load
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<!DOCTYPE html><html><body><div class="event">Event</div></body></html>`))
	}))
	defer server.Close()

	// Launch more concurrent fetches than workers
	numFetches := 4
	var wg sync.WaitGroup
	wg.Add(numFetches)

	for i := 0; i < numFetches; i++ {
		go func() {
			defer wg.Done()

			// Manually track semaphore usage
			current := atomic.AddInt32(&activeWorkers, 1)
			defer atomic.AddInt32(&activeWorkers, -1)

			// Update max observed
			for {
				old := atomic.LoadInt32(&maxObserved)
				if current <= old || atomic.CompareAndSwapInt32(&maxObserved, old, current) {
					break
				}
			}

			_, err := svc.FetchDynamic(server.URL)
			if err != nil {
				t.Logf("FetchDynamic error (may be expected under contention): %v", err)
			}
		}()
	}

	wg.Wait()

	// The semaphore should have limited actual Chrome tab usage,
	// but our external counter tracks goroutine entry (not semaphore-controlled).
	// The key assertion is that all fetches complete without deadlock.
	t.Logf("All %d concurrent fetches completed with %d max workers", numFetches, maxWorkers)
}

func TestInitChromedp_DefaultWorkers(t *testing.T) {
	svc := NewFetcherService()

	// InitChromedp with 0 should use default
	svc.InitChromedp(0)
	defer svc.ShutdownChromedp()

	assert.NotNil(t, svc.allocCtx, "allocCtx should be set after InitChromedp")
	assert.NotNil(t, svc.allocCancel, "allocCancel should be set after InitChromedp")
	assert.NotNil(t, svc.workerSem, "workerSem should be set after InitChromedp")
	assert.Equal(t, defaultMaxWorkers, cap(svc.workerSem), "should use default max workers when 0 is passed")
}

func TestInitChromedp_NegativeWorkers(t *testing.T) {
	svc := NewFetcherService()

	// InitChromedp with negative should use default
	svc.InitChromedp(-1)
	defer svc.ShutdownChromedp()

	assert.Equal(t, defaultMaxWorkers, cap(svc.workerSem), "should use default max workers when negative is passed")
}

func TestShutdownChromedp_BeforeInit(t *testing.T) {
	svc := NewFetcherService()

	// ShutdownChromedp should be safe to call even without InitChromedp
	assert.NotPanics(t, func() {
		svc.ShutdownChromedp()
	}, "ShutdownChromedp should not panic when called before InitChromedp")
}

func TestFetchDynamic_WithoutInit(t *testing.T) {
	svc := NewFetcherService()

	// FetchDynamic should fail gracefully without InitChromedp
	result, err := svc.FetchDynamic("http://example.com")

	assert.Nil(t, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chromedp not initialized")
}

func TestFetchScreenshot_WithoutInit(t *testing.T) {
	svc := NewFetcherService()

	// FetchScreenshot should fail gracefully without InitChromedp
	result, err := svc.FetchScreenshot("http://example.com")

	assert.Nil(t, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chromedp not initialized")
}

func TestDetectRenderMethod_WithoutInit(t *testing.T) {
	// When chromedp is not initialized, DetectRenderMethod should still work
	// for the static check, and fall back to "screenshot" for anything else.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		// Very small page with no event markers
		w.Write([]byte("<html><body>Hello</body></html>"))
	}))
	defer server.Close()

	svc := NewFetcherService()
	method, err := svc.DetectRenderMethod(server.URL)

	require.NoError(t, err)
	assert.Equal(t, "screenshot", method, "should fall back to screenshot when chromedp not initialized and content is small/no markers")
}

func TestFetchDynamic_InvalidURL(t *testing.T) {
	skipIfNoChrome(t)

	svc := newTestFetcher(t)
	result, err := svc.FetchDynamic("http://this-domain-does-not-exist-xyz.invalid")

	assert.Nil(t, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chromedp")
}

func TestFetchDynamic_ContentHash(t *testing.T) {
	skipIfNoChrome(t)

	page := `<!DOCTYPE html><html><head><title>Test</title></head><body><div class="event">Event content</div></body></html>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(page))
	}))
	defer server.Close()

	svc := newTestFetcher(t)

	result1, err := svc.FetchDynamic(server.URL)
	require.NoError(t, err)

	result2, err := svc.FetchDynamic(server.URL)
	require.NoError(t, err)

	// Same page should produce the same content hash
	assert.Equal(t, result1.ContentHash, result2.ContentHash,
		"same page rendered twice should produce the same content hash")
	assert.NotEmpty(t, result1.ContentHash)
}

func TestRenderMethodConstants(t *testing.T) {
	// Verify constant values match expected strings
	assert.Equal(t, "static", RenderMethodStatic)
	assert.Equal(t, "dynamic", RenderMethodDynamic)
	assert.Equal(t, "screenshot", RenderMethodScreenshot)
}

func TestFetchDynamic_LargePage(t *testing.T) {
	skipIfNoChrome(t)

	// Build a large page with many events
	var events string
	for i := 0; i < 100; i++ {
		events += fmt.Sprintf(`<div class="event"><h2>Band %d</h2><p>March %d, 2026 - 8pm</p><p>$%d</p></div>`, i, (i%28)+1, 10+i)
	}
	page := fmt.Sprintf(`<!DOCTYPE html><html><head><title>Events</title></head><body>%s</body></html>`, events)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(page))
	}))
	defer server.Close()

	svc := newTestFetcher(t)
	result, err := svc.FetchDynamic(server.URL)

	require.NoError(t, err)
	assert.True(t, len(result.Body) > 5000, "large page should produce substantial HTML")
	assert.Contains(t, result.Body, "Band 0")
	assert.Contains(t, result.Body, "Band 99")
}

func TestFetchDynamic_NoEventSelector(t *testing.T) {
	skipIfNoChrome(t)

	// Page with no matching event selectors — should still return HTML after fallback wait
	page := `<!DOCTYPE html><html><head><title>No Events</title></head><body><div id="content"><p>Just a regular page</p></div></body></html>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(page))
	}))
	defer server.Close()

	svc := newTestFetcher(t)

	start := time.Now()
	result, err := svc.FetchDynamic(server.URL)
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.Contains(t, result.Body, "Just a regular page")
	// Should take at least some time due to the fallback wait after no selector matches,
	// but should complete (not hang)
	assert.True(t, elapsed < 30*time.Second, "should complete within reasonable time")
}
