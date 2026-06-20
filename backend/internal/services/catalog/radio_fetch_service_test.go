package catalog

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"psychic-homily-backend/internal/services/contracts"
)

// testLogger returns a minimal slog.Logger suitable for testing.
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// TestRadioFetchService_StartStop verifies the background service starts and stops cleanly.
func TestRadioFetchService_StartStop(t *testing.T) {
	// Create a fetch service with a nil radio service — it will fail on actual operations
	// but Start/Stop should work cleanly.
	svc := &RadioFetchService{
		radioService:     &RadioService{db: nil},
		fetchInterval:    1 * time.Hour,
		affinityInterval: 24 * time.Hour,
		rematchInterval:  168 * time.Hour,
		stopCh:           make(chan struct{}),
		logger:           testLogger(),
	}

	ctx, cancel := context.WithCancel(context.Background())
	svc.Start(ctx)

	// Let it run briefly
	time.Sleep(50 * time.Millisecond)

	// Stop should complete without hanging
	cancel()
	done := make(chan struct{})
	go func() {
		svc.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Success — stopped cleanly
	case <-time.After(5 * time.Second):
		t.Fatal("Stop() timed out")
	}
}

// TestRadioFetchService_RunFetchCycleNoStations verifies fetch cycle handles no active stations.
func TestRadioFetchService_RunFetchCycleNoStations(t *testing.T) {
	// RadioService with nil DB will return error from GetActiveStationsWithPlaylistSource
	svc := &RadioFetchService{
		radioService:     &RadioService{db: nil},
		fetchInterval:    1 * time.Hour,
		affinityInterval: 24 * time.Hour,
		rematchInterval:  168 * time.Hour,
		stopCh:           make(chan struct{}),
		logger:           testLogger(),
	}

	// Should not panic
	svc.RunFetchCycleNow()
}

// TestRadioFetchService_RunAffinityCycleNilDB verifies affinity cycle handles nil DB gracefully.
func TestRadioFetchService_RunAffinityCycleNilDB(t *testing.T) {
	svc := &RadioFetchService{
		radioService:     &RadioService{db: nil},
		fetchInterval:    1 * time.Hour,
		affinityInterval: 24 * time.Hour,
		rematchInterval:  168 * time.Hour,
		stopCh:           make(chan struct{}),
		logger:           testLogger(),
	}

	// Should not panic — just log an error
	svc.RunAffinityCycleNow()
}

// TestRadioFetchService_RunReMatchCycleNilDB verifies re-match cycle handles nil DB gracefully.
func TestRadioFetchService_RunReMatchCycleNilDB(t *testing.T) {
	svc := &RadioFetchService{
		radioService:     &RadioService{db: nil},
		fetchInterval:    1 * time.Hour,
		affinityInterval: 24 * time.Hour,
		rematchInterval:  168 * time.Hour,
		discoverInterval: 24 * time.Hour,
		stopCh:           make(chan struct{}),
		logger:           testLogger(),
	}

	// Should not panic — just log an error
	svc.RunReMatchCycleNow()
}

// TestRadioFetchService_RunDiscoverCycleNilDB verifies discover cycle (PSY-671)
// handles nil DB gracefully — GetActiveStationsWithPlaylistSource errors out
// and the cycle logs the failure without panicking.
func TestRadioFetchService_RunDiscoverCycleNilDB(t *testing.T) {
	svc := &RadioFetchService{
		radioService:     &RadioService{db: nil},
		fetchInterval:    1 * time.Hour,
		affinityInterval: 24 * time.Hour,
		rematchInterval:  168 * time.Hour,
		discoverInterval: 24 * time.Hour,
		stopCh:           make(chan struct{}),
		logger:           testLogger(),
	}

	svc.RunDiscoverCycleNow()
}

// TestRadioFetchService_DiscoverInterval_EnvOverride verifies PSY-671's
// RADIO_DISCOVER_INTERVAL_HOURS env var is honored. Defaults to 24h.
func TestRadioFetchService_DiscoverInterval_EnvOverride(t *testing.T) {
	t.Setenv("RADIO_DISCOVER_INTERVAL_HOURS", "48")
	svc := NewRadioFetchService(&RadioService{db: nil}, nil)
	if svc.discoverInterval != 48*time.Hour {
		t.Fatalf("expected 48h discover interval, got %v", svc.discoverInterval)
	}
}

func TestRadioFetchService_DiscoverInterval_Default(t *testing.T) {
	t.Setenv("RADIO_DISCOVER_INTERVAL_HOURS", "")
	svc := NewRadioFetchService(&RadioService{db: nil}, nil)
	if svc.discoverInterval != DefaultDiscoverInterval {
		t.Fatalf("expected default %v, got %v", DefaultDiscoverInterval, svc.discoverInterval)
	}
}

// PSY-672: RADIO_AUTO_BACKFILL_DAYS env var. Default 90, 0 disables.
func TestRadioFetchService_AutoBackfillDays_Default(t *testing.T) {
	t.Setenv("RADIO_AUTO_BACKFILL_DAYS", "")
	svc := NewRadioFetchService(&RadioService{db: nil}, nil)
	if svc.autoBackfillDays != DefaultAutoBackfillDays {
		t.Fatalf("expected default %d, got %d", DefaultAutoBackfillDays, svc.autoBackfillDays)
	}
}

func TestRadioFetchService_AutoBackfillDays_EnvOverride(t *testing.T) {
	t.Setenv("RADIO_AUTO_BACKFILL_DAYS", "30")
	svc := NewRadioFetchService(&RadioService{db: nil}, nil)
	if svc.autoBackfillDays != 30 {
		t.Fatalf("expected 30, got %d", svc.autoBackfillDays)
	}
}

func TestRadioFetchService_AutoBackfillDays_DisableViaZero(t *testing.T) {
	t.Setenv("RADIO_AUTO_BACKFILL_DAYS", "0")
	svc := NewRadioFetchService(&RadioService{db: nil}, nil)
	if svc.autoBackfillDays != 0 {
		t.Fatalf("expected 0 (disabled), got %d", svc.autoBackfillDays)
	}
}

func TestRadioFetchService_AutoBackfillDays_NegativeFallsBackToDefault(t *testing.T) {
	t.Setenv("RADIO_AUTO_BACKFILL_DAYS", "-5")
	svc := NewRadioFetchService(&RadioService{db: nil}, nil)
	if svc.autoBackfillDays != DefaultAutoBackfillDays {
		t.Fatalf("negative value should fall back to default %d, got %d", DefaultAutoBackfillDays, svc.autoBackfillDays)
	}
}

// =============================================================================
// PSY-887: error classification + transient retry tests
// =============================================================================

// newTestFetchService returns a minimal RadioFetchService suitable for testing
// the transient-retry helper (fetchStationWithRetry) in isolation. The circuit
// breaker now lives in radio_station_health (PSY-1140) and is exercised by the
// DB-backed RadioSyncSuite, not here.
func newTestFetchService() *RadioFetchService {
	return &RadioFetchService{
		radioService: &RadioService{db: nil},
		stopCh:       make(chan struct{}),
		logger:       testLogger(),
	}
}

// TestClassifyError covers every routing rule in classifyError so a future
// refactor can't silently drop a transient case (PSY-887).
func TestClassifyError(t *testing.T) {
	// 429 wrapped in a RadioHTTPError, then double-wrapped in fmt.Errorf —
	// mimics how the error arrives at runFetchCycle through RadioService.
	rateLimited := &RadioHTTPError{Provider: "KEXP API", StatusCode: 429, Body: "rate limited"}
	wrappedRateLimited := fmt.Errorf("fetch episodes for show Foo: %w", rateLimited)

	// 503 wrapped likewise — the policy classifies 5xx as PERMANENT, not
	// transient, so the breaker can catch sustained provider outages.
	serverErr := &RadioHTTPError{Provider: "WFMU", StatusCode: 503, Body: "service unavailable"}
	wrappedServerErr := fmt.Errorf("discover shows: %w", serverErr)

	// 404 — permanent. NTS uses a separate sentinel for 404, but
	// RadioHTTPError-routed 404s should also be permanent.
	notFound := &RadioHTTPError{Provider: "KEXP API", StatusCode: 404, Body: ""}

	// Simulated dial-level error — *net.OpError covers connection refused etc.
	dialErr := &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}

	// Plain parse failure — always permanent.
	parseErr := fmt.Errorf("parsing shows response: %w", errors.New("invalid JSON"))

	cases := []struct {
		name string
		err  error
		want errorKind
	}{
		{"nil defaults to permanent (defensive)", nil, kindPermanent},
		{"context deadline exceeded → transient", context.DeadlineExceeded, kindTransient},
		{"wrapped context deadline → transient", fmt.Errorf("dial: %w", context.DeadlineExceeded), kindTransient},
		{"RadioHTTPError 429 → transient", rateLimited, kindTransient},
		{"wrapped 429 → transient", wrappedRateLimited, kindTransient},
		{"RadioHTTPError 503 → permanent (5xx is permanent per policy)", serverErr, kindPermanent},
		{"wrapped 503 → permanent", wrappedServerErr, kindPermanent},
		{"RadioHTTPError 404 → permanent", notFound, kindPermanent},
		{"net.OpError (dial refused) → transient", dialErr, kindTransient},
		{"wrapped dial refused → transient", fmt.Errorf("executing request: %w", dialErr), kindTransient},
		{"ErrTransient sentinel → transient", ErrTransient, kindTransient},
		{"wrapped ErrTransient → transient", fmt.Errorf("custom transient: %w", ErrTransient), kindTransient},
		{"ErrPermanent sentinel → permanent", ErrPermanent, kindPermanent},
		{"parse failure → permanent", parseErr, kindPermanent},
		{"plain string error → permanent (default)", errors.New("station not found"), kindPermanent},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyError(tc.err)
			if got != tc.want {
				t.Fatalf("classifyError(%v) = %v, want %v", tc.err, errorKindName(got), errorKindName(tc.want))
			}
		})
	}
}

// The persistent circuit-breaker state machine (transient-doesn't-trip,
// permanent-trips-at-threshold, half-open trial, per-station isolation,
// no-wedge-after-reset) moved from the in-memory map to radio_station_health in
// PSY-1140. Its transition logic is unit-tested purely in radio_breaker_test.go,
// and the end-to-end DB wiring (gate → run → health write-back, manual-probe
// policy) in radio_sync_integration_test.go's RadioSyncSuite.

// TestFetchStationWithRetry_TransientRecovers exercises the single-retry
// behavior: a transient error on the first attempt followed by success on
// the retry must surface as success (no error reported to the cycle counter).
func TestFetchStationWithRetry_TransientRecovers(t *testing.T) {
	svc := newTestFetchService()
	// Drop the retry backoff to a hair so the test stays fast — restored by
	// the test's lifetime alone (constant override isn't possible in Go, but
	// 500ms is acceptable for one test).
	var calls atomic.Int32
	call := func() (any, error) {
		calls.Add(1)
		if calls.Load() == 1 {
			// First attempt: transient timeout.
			return nil, context.DeadlineExceeded
		}
		// Retry: success.
		return &contracts.RadioImportResult{EpisodesImported: 1}, nil
	}

	start := time.Now()
	got, err := svc.fetchStationWithRetry(1, "TestStation", "fetch", call)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("expected nil error after retry recovery; got %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil result after retry recovery")
	}
	if calls.Load() != 2 {
		t.Fatalf("expected exactly 2 calls (initial + 1 retry); got %d", calls.Load())
	}
	if elapsed < radioTransientRetryBackoff {
		t.Fatalf("retry should sleep at least %v; elapsed=%v", radioTransientRetryBackoff, elapsed)
	}
}

// TestFetchStationWithRetry_NoRetryOnPermanent verifies that permanent errors
// surface immediately without burning the backoff. This is the "fail fast"
// half of the policy.
func TestFetchStationWithRetry_NoRetryOnPermanent(t *testing.T) {
	svc := newTestFetchService()

	var calls atomic.Int32
	permanent := &RadioHTTPError{Provider: "KEXP API", StatusCode: 500, Body: ""}
	call := func() (any, error) {
		calls.Add(1)
		return nil, permanent
	}

	start := time.Now()
	got, err := svc.fetchStationWithRetry(1, "TestStation", "fetch", call)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected permanent error to surface")
	}
	if got != nil {
		t.Fatalf("expected nil result on permanent error; got %v", got)
	}
	if calls.Load() != 1 {
		t.Fatalf("permanent errors must NOT retry; got %d calls", calls.Load())
	}
	// Permanent path should be well under the backoff window — confirms we
	// didn't accidentally sleep on the non-retry branch.
	if elapsed >= radioTransientRetryBackoff {
		t.Fatalf("permanent path should not sleep; elapsed=%v >= backoff %v", elapsed, radioTransientRetryBackoff)
	}
}

// TestFetchStationWithRetry_TransientPersists confirms that a station whose
// transient error doesn't clear on retry surfaces the retry error to the
// caller (which then bumps the transient counter — NOT the breaker counter).
func TestFetchStationWithRetry_TransientPersists(t *testing.T) {
	svc := newTestFetchService()

	var calls atomic.Int32
	rateLimited := &RadioHTTPError{Provider: "WFMU", StatusCode: 429, Body: ""}
	call := func() (any, error) {
		calls.Add(1)
		return nil, rateLimited
	}

	got, err := svc.fetchStationWithRetry(1, "TestStation", "fetch", call)
	if err == nil {
		t.Fatal("expected error to surface when retry also fails")
	}
	if got != nil {
		t.Fatalf("expected nil result on persisted error; got %v", got)
	}
	if calls.Load() != 2 {
		t.Fatalf("expected initial + 1 retry = 2 calls; got %d", calls.Load())
	}
	if classifyError(err) != kindTransient {
		t.Fatalf("persisted error must still classify as transient; got %v", errorKindName(classifyError(err)))
	}
}

// TestFetchStationWithRetry_ShutdownAbortsBackoff verifies the stopCh-aware
// backoff: a service shutdown mid-backoff returns the original error
// immediately rather than wasting the full retry-backoff window.
func TestFetchStationWithRetry_ShutdownAbortsBackoff(t *testing.T) {
	svc := newTestFetchService()
	// Close stopCh BEFORE the call so the select hits the shutdown branch
	// immediately on the first transient error.
	close(svc.stopCh)

	var calls atomic.Int32
	call := func() (any, error) {
		calls.Add(1)
		return nil, context.DeadlineExceeded
	}

	start := time.Now()
	_, err := svc.fetchStationWithRetry(1, "TestStation", "fetch", call)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected the transient error to surface when shutdown aborts retry")
	}
	if calls.Load() != 1 {
		t.Fatalf("shutdown must abort before retry; got %d calls", calls.Load())
	}
	// We took the stopCh branch, so elapsed should be far less than the
	// backoff window. Allow some slack for scheduling.
	if elapsed >= radioTransientRetryBackoff {
		t.Fatalf("shutdown should abort backoff; elapsed=%v >= backoff %v", elapsed, radioTransientRetryBackoff)
	}
}

// TestRadioHTTPError_UnwrapClassification documents the contract that the
// provider doGet helpers depend on: a RadioHTTPError's Unwrap returns
// ErrTransient for 429 and ErrPermanent for everything else. If a future
// refactor changes the Unwrap behavior, classifyError's branches break too.
func TestRadioHTTPError_UnwrapClassification(t *testing.T) {
	cases := []struct {
		status int
		want   error
	}{
		{429, ErrTransient},
		{500, ErrPermanent},
		{503, ErrPermanent},
		{404, ErrPermanent},
		{400, ErrPermanent},
		{418, ErrPermanent}, // teapot — should still route to permanent
	}

	for _, tc := range cases {
		err := &RadioHTTPError{Provider: "Test", StatusCode: tc.status}
		if !errors.Is(err, tc.want) {
			t.Fatalf("status %d: errors.Is(_, %v) = false; want true", tc.status, tc.want)
		}
	}
}
