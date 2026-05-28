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
		radioService:        &RadioService{db: nil},
		fetchInterval:       1 * time.Hour,
		affinityInterval:    24 * time.Hour,
		rematchInterval:     168 * time.Hour,
		stopCh:              make(chan struct{}),
		logger:              testLogger(),
		consecutiveFailures: make(map[uint]int),
		transientFailures:   make(map[uint]int),
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

// TestRadioFetchService_CircuitBreaker verifies that stations are skipped after
// reaching the circuit breaker threshold.
func TestRadioFetchService_CircuitBreaker(t *testing.T) {
	svc := &RadioFetchService{
		radioService:        &RadioService{db: nil},
		fetchInterval:       1 * time.Hour,
		affinityInterval:    24 * time.Hour,
		rematchInterval:     168 * time.Hour,
		stopCh:              make(chan struct{}),
		logger:              testLogger(),
		consecutiveFailures: make(map[uint]int),
		transientFailures:   make(map[uint]int),
	}

	// Set failures below threshold
	svc.SetConsecutiveFailures(1, radioCircuitBreakerThreshold-1)
	if svc.GetConsecutiveFailures(1) != radioCircuitBreakerThreshold-1 {
		t.Fatalf("expected %d failures, got %d", radioCircuitBreakerThreshold-1, svc.GetConsecutiveFailures(1))
	}

	// Set failures at threshold
	svc.SetConsecutiveFailures(2, radioCircuitBreakerThreshold)
	if svc.GetConsecutiveFailures(2) != radioCircuitBreakerThreshold {
		t.Fatalf("expected %d failures, got %d", radioCircuitBreakerThreshold, svc.GetConsecutiveFailures(2))
	}

	// Station 3 has no failures — should return 0
	if svc.GetConsecutiveFailures(3) != 0 {
		t.Fatalf("expected 0 failures for unknown station, got %d", svc.GetConsecutiveFailures(3))
	}
}

// TestRadioFetchService_RunFetchCycleNoStations verifies fetch cycle handles no active stations.
func TestRadioFetchService_RunFetchCycleNoStations(t *testing.T) {
	// RadioService with nil DB will return error from GetActiveStationsWithPlaylistSource
	svc := &RadioFetchService{
		radioService:        &RadioService{db: nil},
		fetchInterval:       1 * time.Hour,
		affinityInterval:    24 * time.Hour,
		rematchInterval:     168 * time.Hour,
		stopCh:              make(chan struct{}),
		logger:              testLogger(),
		consecutiveFailures: make(map[uint]int),
		transientFailures:   make(map[uint]int),
	}

	// Should not panic
	svc.RunFetchCycleNow()
}

// TestRadioFetchService_RunAffinityCycleNilDB verifies affinity cycle handles nil DB gracefully.
func TestRadioFetchService_RunAffinityCycleNilDB(t *testing.T) {
	svc := &RadioFetchService{
		radioService:        &RadioService{db: nil},
		fetchInterval:       1 * time.Hour,
		affinityInterval:    24 * time.Hour,
		rematchInterval:     168 * time.Hour,
		stopCh:              make(chan struct{}),
		logger:              testLogger(),
		consecutiveFailures: make(map[uint]int),
		transientFailures:   make(map[uint]int),
	}

	// Should not panic — just log an error
	svc.RunAffinityCycleNow()
}

// TestRadioFetchService_RunReMatchCycleNilDB verifies re-match cycle handles nil DB gracefully.
func TestRadioFetchService_RunReMatchCycleNilDB(t *testing.T) {
	svc := &RadioFetchService{
		radioService:        &RadioService{db: nil},
		fetchInterval:       1 * time.Hour,
		affinityInterval:    24 * time.Hour,
		rematchInterval:     168 * time.Hour,
		discoverInterval:    24 * time.Hour,
		stopCh:              make(chan struct{}),
		logger:              testLogger(),
		consecutiveFailures: make(map[uint]int),
		transientFailures:   make(map[uint]int),
	}

	// Should not panic — just log an error
	svc.RunReMatchCycleNow()
}

// TestRadioFetchService_RunDiscoverCycleNilDB verifies discover cycle (PSY-671)
// handles nil DB gracefully — GetActiveStationsWithPlaylistSource errors out
// and the cycle logs the failure without panicking.
func TestRadioFetchService_RunDiscoverCycleNilDB(t *testing.T) {
	svc := &RadioFetchService{
		radioService:        &RadioService{db: nil},
		fetchInterval:       1 * time.Hour,
		affinityInterval:    24 * time.Hour,
		rematchInterval:     168 * time.Hour,
		discoverInterval:    24 * time.Hour,
		stopCh:              make(chan struct{}),
		logger:              testLogger(),
		consecutiveFailures: make(map[uint]int),
		transientFailures:   make(map[uint]int),
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

// PSY-672: waitForJobCompletion respects stopCh — a service shutdown mid-poll
// should return jobWaitShutdown within one poll interval so the drain goroutine
// can return cleanly without orphan ticks.
func TestRadioFetchService_WaitForJobCompletion_ShutdownAbort(t *testing.T) {
	svc := &RadioFetchService{
		radioService:        &RadioService{db: nil},
		stopCh:              make(chan struct{}),
		logger:              testLogger(),
		consecutiveFailures: make(map[uint]int),
		transientFailures:   make(map[uint]int),
	}

	// Close stopCh immediately; the wait should return jobWaitShutdown on the
	// first select iteration without ever hitting GetImportJob.
	close(svc.stopCh)
	done := make(chan struct{})
	var (
		job    *contracts.RadioImportJobResponse
		result jobWaitResult
	)
	go func() {
		job, result = svc.waitForJobCompletion(1)
		close(done)
	}()
	select {
	case <-done:
		if result != jobWaitShutdown {
			t.Fatalf("expected jobWaitShutdown on stopCh close, got %v", result)
		}
		if job != nil {
			t.Fatal("expected nil job on stopCh close")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("waitForJobCompletion did not honor stopCh within 2s")
	}
}

// =============================================================================
// PSY-887: error classification + transient retry tests
// =============================================================================

// newTestFetchService returns a minimal RadioFetchService suitable for testing
// the PSY-887 circuit-breaker classifier helpers in isolation. Maps are
// pre-allocated so the test can use the typed counter setters/getters without
// nil-map panics.
func newTestFetchService() *RadioFetchService {
	return &RadioFetchService{
		radioService:        &RadioService{db: nil},
		stopCh:              make(chan struct{}),
		logger:              testLogger(),
		consecutiveFailures: make(map[uint]int),
		transientFailures:   make(map[uint]int),
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

// TestCircuitBreaker_TransientDoesNotTrip exercises the core PSY-887 fix:
// even 10x the threshold worth of transient errors must NOT trip the breaker.
// AC item (a). Before PSY-887, this loop would have wedged the station after
// 5 timeouts; now consecutiveFailures stays at 0 and the station remains
// eligible for the next attempt.
func TestCircuitBreaker_TransientDoesNotTrip(t *testing.T) {
	svc := newTestFetchService()
	const stationID uint = 1

	rateLimited := &RadioHTTPError{Provider: "KEXP API", StatusCode: 429, Body: ""}

	// Record more transient failures than the breaker threshold — should
	// only bump transientFailures, never consecutiveFailures.
	for i := 0; i < radioCircuitBreakerThreshold*2; i++ {
		kind := svc.recordStationFailure(stationID, rateLimited)
		if kind != kindTransient {
			t.Fatalf("iteration %d: expected kindTransient, got %v", i, errorKindName(kind))
		}
	}

	if got := svc.GetConsecutiveFailures(stationID); got != 0 {
		t.Fatalf("transient failures must NOT bump consecutiveFailures; got %d", got)
	}
	if got := svc.GetTransientFailures(stationID); got != radioCircuitBreakerThreshold*2 {
		t.Fatalf("expected %d transient failures, got %d", radioCircuitBreakerThreshold*2, got)
	}

	skip, _ := svc.stationBreakerSkip(stationID)
	if skip {
		t.Fatal("breaker tripped on transient-only failures — PSY-887 regression")
	}
}

// TestCircuitBreaker_PermanentTrips verifies the breaker still catches the
// failure modes it was designed for: 5xx responses, parse failures, 4xx
// (non-429) errors. AC item (b).
func TestCircuitBreaker_PermanentTrips(t *testing.T) {
	svc := newTestFetchService()
	const stationID uint = 1

	cases := []error{
		&RadioHTTPError{Provider: "WFMU", StatusCode: 503, Body: ""},
		fmt.Errorf("parsing response: %w", errors.New("EOF")),
		&RadioHTTPError{Provider: "NTS API", StatusCode: 400, Body: "bad request"},
		errors.New("schema mismatch on field foo"),
		&RadioHTTPError{Provider: "KEXP API", StatusCode: 500, Body: ""},
	}

	// Hit threshold exactly — breaker should trip on the 5th.
	for i, e := range cases {
		kind := svc.recordStationFailure(stationID, e)
		if kind != kindPermanent {
			t.Fatalf("case %d (%v): expected kindPermanent, got %v", i, e, errorKindName(kind))
		}
	}

	if got := svc.GetConsecutiveFailures(stationID); got != radioCircuitBreakerThreshold {
		t.Fatalf("expected breaker at threshold %d, got %d", radioCircuitBreakerThreshold, got)
	}

	skip, failures := svc.stationBreakerSkip(stationID)
	if !skip {
		t.Fatalf("breaker should trip at threshold (%d failures); skip=false", failures)
	}
}

// TestCircuitBreaker_NoWedgeAfterReset exercises AC item (c): the breaker must
// not wedge across cycle boundaries under the new policy. The pre-PSY-887 bug
// shape was: 5 transient failures → breaker trips → station can't succeed
// while skipped → wedged until next cycle. Under Option A, transient failures
// don't trip the breaker, and a successful fetch resets BOTH counters, so the
// station never gets stuck in a "skipped, can't recover" state.
func TestCircuitBreaker_NoWedgeAfterReset(t *testing.T) {
	svc := newTestFetchService()
	const stationID uint = 1

	// Simulate a stretch of transient blips (the original wedge trigger).
	timeout := context.DeadlineExceeded
	for i := 0; i < radioCircuitBreakerThreshold; i++ {
		svc.recordStationFailure(stationID, timeout)
	}

	// Breaker should still be open (no skip) because transient ≠ breaker bump.
	if skip, _ := svc.stationBreakerSkip(stationID); skip {
		t.Fatal("breaker should not trip on transient failures (no-wedge invariant)")
	}

	// One success: BOTH counters reset, station is fully clean going into the
	// next cycle.
	svc.recordStationSuccess(stationID)

	if got := svc.GetConsecutiveFailures(stationID); got != 0 {
		t.Fatalf("recordStationSuccess must reset consecutiveFailures; got %d", got)
	}
	if got := svc.GetTransientFailures(stationID); got != 0 {
		t.Fatalf("recordStationSuccess must reset transientFailures; got %d", got)
	}

	// Permanent failures from this point should accumulate from zero — i.e. the
	// success "closed the wound", a single new permanent error doesn't push us
	// to threshold because old transient blips don't haunt us.
	svc.recordStationFailure(stationID, errors.New("permanent error"))
	if got := svc.GetConsecutiveFailures(stationID); got != 1 {
		t.Fatalf("post-success permanent count should start fresh at 1; got %d", got)
	}
}

// TestCircuitBreaker_PerStationIsolation verifies AC item (d): one wedged
// station does not affect other stations. The failures map is keyed by
// station ID, so this should be naturally true — the test guards against a
// future refactor that accidentally globalizes the breaker.
func TestCircuitBreaker_PerStationIsolation(t *testing.T) {
	svc := newTestFetchService()

	// Wedge station 1 with permanent errors.
	for i := 0; i < radioCircuitBreakerThreshold; i++ {
		svc.recordStationFailure(1, errors.New("permanent"))
	}
	if skip, _ := svc.stationBreakerSkip(1); !skip {
		t.Fatal("station 1 should be wedged")
	}

	// Stations 2 and 3 should be untouched.
	if skip, failures := svc.stationBreakerSkip(2); skip {
		t.Fatalf("station 2 should not be wedged; failures=%d skip=%v", failures, skip)
	}
	if skip, failures := svc.stationBreakerSkip(3); skip {
		t.Fatalf("station 3 should not be wedged; failures=%d skip=%v", failures, skip)
	}

	// Wedge station 2 separately — confirms the counters are per-station.
	for i := 0; i < radioCircuitBreakerThreshold; i++ {
		svc.recordStationFailure(2, errors.New("permanent"))
	}

	// Station 1 should still be wedged at exactly threshold (not double-counted).
	if got := svc.GetConsecutiveFailures(1); got != radioCircuitBreakerThreshold {
		t.Fatalf("station 1 failures should be unchanged at %d; got %d", radioCircuitBreakerThreshold, got)
	}

	// Resetting station 1 should not affect station 2.
	svc.recordStationSuccess(1)
	if got := svc.GetConsecutiveFailures(1); got != 0 {
		t.Fatalf("station 1 should reset; got %d", got)
	}
	if got := svc.GetConsecutiveFailures(2); got != radioCircuitBreakerThreshold {
		t.Fatalf("station 2 should still be wedged; got %d", got)
	}
}

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
