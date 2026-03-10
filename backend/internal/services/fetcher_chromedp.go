package services

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"regexp"
	"time"

	"github.com/chromedp/chromedp"
)

// Default chromedp configuration constants.
const (
	defaultMaxWorkers   = 3
	semaphoreAcquireTimeout = 30 * time.Second
	pageWaitTimeout     = 8 * time.Second
	screenshotQuality   = 90
)

// Common CSS selectors for event containers on venue calendar pages.
var eventSelectors = []string{
	".event",
	".events",
	".event-list",
	".event-card",
	"[class*='event']",
	".show-listing",
	".calendar-event",
	"article",
}

// InitChromedp initializes the chromedp allocator and worker pool semaphore.
// maxWorkers controls the number of concurrent Chrome tabs (each ~200MB RAM).
// This is intentionally not called in NewFetcherService — callers must opt in.
func (s *FetcherService) InitChromedp(maxWorkers int) {
	if maxWorkers <= 0 {
		maxWorkers = defaultMaxWorkers
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-extensions", true),
		chromedp.Flag("disable-background-networking", true),
		chromedp.Flag("disable-default-apps", true),
		chromedp.Flag("disable-sync", true),
		chromedp.Flag("disable-translate", true),
		chromedp.Flag("mute-audio", true),
	)

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	s.allocCtx = allocCtx
	s.allocCancel = allocCancel
	s.workerSem = make(chan struct{}, maxWorkers)

	log.Printf("chromedp initialized with %d max concurrent workers", maxWorkers)
}

// ShutdownChromedp gracefully shuts down the chromedp allocator.
// Safe to call even if InitChromedp was never called.
func (s *FetcherService) ShutdownChromedp() {
	if s.allocCancel != nil {
		s.allocCancel()
		log.Printf("chromedp allocator shut down")
	}
}

// acquireSemaphore blocks until a worker slot is available or the timeout expires.
func (s *FetcherService) acquireSemaphore() error {
	if s.workerSem == nil {
		return fmt.Errorf("chromedp not initialized: call InitChromedp first")
	}
	ctx, cancel := context.WithTimeout(context.Background(), semaphoreAcquireTimeout)
	defer cancel()

	select {
	case s.workerSem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("timed out waiting for chromedp worker slot after %s", semaphoreAcquireTimeout)
	}
}

// releaseSemaphore returns a worker slot to the pool.
func (s *FetcherService) releaseSemaphore() {
	if s.workerSem != nil {
		<-s.workerSem
	}
}

// FetchDynamic renders a URL with headless Chrome and returns the fully rendered DOM HTML.
// Used for Tier 2 (dynamic) venue calendar pages where content is JS-rendered.
// Never uses networkidle — waits for event container CSS selectors with an 8s timeout.
func (s *FetcherService) FetchDynamic(url string) (*FetchResult, error) {
	if err := s.acquireSemaphore(); err != nil {
		return nil, err
	}
	defer s.releaseSemaphore()

	// Create a new tab context from the shared allocator
	tabCtx, tabCancel := chromedp.NewContext(s.allocCtx)
	defer tabCancel()

	// Navigate and wait for content
	var html string
	actions := []chromedp.Action{
		chromedp.Navigate(url),
	}

	// Try to wait for an event container selector (best-effort, 8s timeout)
	waitCtx, waitCancel := context.WithTimeout(tabCtx, pageWaitTimeout)
	defer waitCancel()

	if err := chromedp.Run(tabCtx, actions...); err != nil {
		return nil, fmt.Errorf("chromedp navigate to %s: %w", url, err)
	}

	// Try each event selector — stop at first match
	selectorFound := false
	for _, sel := range eventSelectors {
		err := chromedp.Run(waitCtx, chromedp.WaitVisible(sel, chromedp.ByQuery))
		if err == nil {
			selectorFound = true
			break
		}
	}

	if !selectorFound {
		// No selector matched within timeout — page may still have content.
		// Wait a fixed duration as fallback to let any JS finish.
		fallbackCtx, fallbackCancel := context.WithTimeout(tabCtx, 3*time.Second)
		defer fallbackCancel()
		chromedp.Run(fallbackCtx, chromedp.Sleep(2*time.Second)) //nolint:errcheck
	}

	// Extract the full rendered DOM
	if err := chromedp.Run(tabCtx, chromedp.OuterHTML("html", &html, chromedp.ByQuery)); err != nil {
		return nil, fmt.Errorf("chromedp extract HTML from %s: %w", url, err)
	}

	hash := computeContentHash(html)

	return &FetchResult{
		Changed:     true,
		Body:        html,
		ContentHash: hash,
		HTTPStatus:  200,
		ContentType: "text/html",
	}, nil
}

// FetchScreenshot renders a URL with headless Chrome and captures a full-page PNG screenshot.
// Used for Tier 3 (screenshot) venue calendar pages where DOM is obfuscated.
// Returns the screenshot as a base64-encoded string in FetchResult.Body.
func (s *FetcherService) FetchScreenshot(url string) (*FetchResult, error) {
	if err := s.acquireSemaphore(); err != nil {
		return nil, err
	}
	defer s.releaseSemaphore()

	// Create a new tab context from the shared allocator
	tabCtx, tabCancel := chromedp.NewContext(s.allocCtx)
	defer tabCancel()

	// Navigate to the URL
	if err := chromedp.Run(tabCtx, chromedp.Navigate(url)); err != nil {
		return nil, fmt.Errorf("chromedp navigate to %s: %w", url, err)
	}

	// Try to wait for an event container selector (best-effort, 8s timeout)
	waitCtx, waitCancel := context.WithTimeout(tabCtx, pageWaitTimeout)
	defer waitCancel()

	selectorFound := false
	for _, sel := range eventSelectors {
		err := chromedp.Run(waitCtx, chromedp.WaitVisible(sel, chromedp.ByQuery))
		if err == nil {
			selectorFound = true
			break
		}
	}

	if !selectorFound {
		fallbackCtx, fallbackCancel := context.WithTimeout(tabCtx, 3*time.Second)
		defer fallbackCancel()
		chromedp.Run(fallbackCtx, chromedp.Sleep(2*time.Second)) //nolint:errcheck
	}

	// Capture full-page screenshot.
	// chromedp.FullScreenshot with quality > 0 produces JPEG; quality 0 produces PNG.
	// We use JPEG (quality 90) for smaller file sizes — better for AI extraction API calls.
	var buf []byte
	if err := chromedp.Run(tabCtx, chromedp.FullScreenshot(&buf, screenshotQuality)); err != nil {
		return nil, fmt.Errorf("chromedp screenshot of %s: %w", url, err)
	}

	b64 := base64.StdEncoding.EncodeToString(buf)
	hash := computeContentHash(b64)

	// Detect format from magic bytes
	contentType := "image/jpeg"
	if len(buf) >= 8 && buf[0] == 0x89 && buf[1] == 0x50 && buf[2] == 0x4E && buf[3] == 0x47 {
		contentType = "image/png"
	}

	return &FetchResult{
		Changed:     true,
		Body:        b64,
		ContentHash: hash,
		HTTPStatus:  200,
		ContentType: contentType,
	}, nil
}

// DetectRenderMethod auto-detects the best rendering tier for a URL.
// Returns one of: "static", "dynamic", "screenshot".
//
// Decision logic:
//  1. Plain HTTP GET — if body >5KB and has event markers -> "static"
//  2. Otherwise try FetchDynamic — if rendered body >5KB and has event markers -> "dynamic"
//  3. Otherwise -> "screenshot"
func (s *FetcherService) DetectRenderMethod(url string) (string, error) {
	// Step 1: Try static fetch
	result, err := s.Fetch(url, "", "")
	if err == nil && result.Changed && len(result.Body) > 5000 && hasEventMarkers(result.Body) {
		return "static", nil
	}

	// Step 2: Try dynamic fetch (requires chromedp to be initialized)
	if s.allocCtx == nil {
		// chromedp not initialized — can't test dynamic rendering
		return "screenshot", nil
	}

	dynamicResult, err := s.FetchDynamic(url)
	if err == nil && len(dynamicResult.Body) > 5000 && hasEventMarkers(dynamicResult.Body) {
		return "dynamic", nil
	}

	// Step 3: Fall back to screenshot
	return "screenshot", nil
}

// Event marker patterns for detecting calendar/event content in HTML.
var (
	// Date patterns: "January", "February", ..., "2024", "2025", "2026", etc.
	monthPattern = regexp.MustCompile(`(?i)\b(january|february|march|april|may|june|july|august|september|october|november|december|jan|feb|mar|apr|jun|jul|aug|sep|oct|nov|dec)\b`)
	yearPattern  = regexp.MustCompile(`\b20[2-3]\d\b`)

	// Time patterns: "7pm", "8:00", "7:30 PM", "doors"
	timePattern = regexp.MustCompile(`(?i)\b\d{1,2}(:\d{2})?\s*(am|pm|AM|PM)\b`)
	doorsPattern = regexp.MustCompile(`(?i)\bdoors\b`)

	// Price patterns: "$10", "$25.00"
	pricePattern = regexp.MustCompile(`\$\d+`)

	// Music/event terms
	musicTermPattern = regexp.MustCompile(`(?i)\b(tickets?|lineup|opener|headliner|live music|concert|tour|sold out|venue|stage|set\s+times?|all\s+ages|21\+|18\+)\b`)
)

// hasEventMarkers checks whether HTML content contains patterns typical of
// venue calendar / event listing pages. It's intentionally generous — false
// positives are cheap (we just use a cheaper tier), false negatives waste
// money on unnecessary screenshots.
func hasEventMarkers(html string) bool {
	markers := 0

	if monthPattern.MatchString(html) {
		markers++
	}
	if yearPattern.MatchString(html) {
		markers++
	}
	if timePattern.MatchString(html) {
		markers++
	}
	if doorsPattern.MatchString(html) {
		markers++
	}
	if pricePattern.MatchString(html) {
		markers++
	}
	if musicTermPattern.MatchString(html) {
		markers++
	}

	// Require at least 2 different marker types to consider it an event page.
	// A single match (e.g., just a year) could be any page.
	return markers >= 2
}

