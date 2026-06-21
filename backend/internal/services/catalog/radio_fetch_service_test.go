package catalog

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"sync"
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

// PSY-1154: RADIO_BACKFILL_INTERVAL_HOURS / RADIO_BACKFILL_LOOKBACK_DAYS env vars.
func TestRadioFetchService_BackfillInterval_Default(t *testing.T) {
	t.Setenv("RADIO_BACKFILL_INTERVAL_HOURS", "")
	svc := NewRadioFetchService(&RadioService{db: nil}, nil)
	if svc.backfillInterval != DefaultBackfillInterval {
		t.Fatalf("expected default %v, got %v", DefaultBackfillInterval, svc.backfillInterval)
	}
}

func TestRadioFetchService_BackfillInterval_EnvOverride(t *testing.T) {
	t.Setenv("RADIO_BACKFILL_INTERVAL_HOURS", "3")
	svc := NewRadioFetchService(&RadioService{db: nil}, nil)
	if svc.backfillInterval != 3*time.Hour {
		t.Fatalf("expected 3h backfill interval, got %v", svc.backfillInterval)
	}
}

func TestRadioFetchService_BackfillLookbackDays_Default(t *testing.T) {
	t.Setenv("RADIO_BACKFILL_LOOKBACK_DAYS", "")
	svc := NewRadioFetchService(&RadioService{db: nil}, nil)
	if svc.backfillLookbackDays != DefaultBackfillLookbackDays {
		t.Fatalf("expected default %d, got %d", DefaultBackfillLookbackDays, svc.backfillLookbackDays)
	}
}

func TestRadioFetchService_BackfillLookbackDays_EnvOverride(t *testing.T) {
	t.Setenv("RADIO_BACKFILL_LOOKBACK_DAYS", "14")
	svc := NewRadioFetchService(&RadioService{db: nil}, nil)
	if svc.backfillLookbackDays != 14 {
		t.Fatalf("expected 14, got %d", svc.backfillLookbackDays)
	}
}

func TestRadioFetchService_BackfillLookbackDays_DisableViaZero(t *testing.T) {
	t.Setenv("RADIO_BACKFILL_LOOKBACK_DAYS", "0")
	svc := NewRadioFetchService(&RadioService{db: nil}, nil)
	if svc.backfillLookbackDays != 0 {
		t.Fatalf("expected 0 (disabled), got %d", svc.backfillLookbackDays)
	}
}

func TestRadioFetchService_BackfillLookbackDays_NegativeFallsBackToDefault(t *testing.T) {
	t.Setenv("RADIO_BACKFILL_LOOKBACK_DAYS", "-3")
	svc := NewRadioFetchService(&RadioService{db: nil}, nil)
	if svc.backfillLookbackDays != DefaultBackfillLookbackDays {
		t.Fatalf("negative value should fall back to default %d, got %d", DefaultBackfillLookbackDays, svc.backfillLookbackDays)
	}
}

// PSY-1155: janitor env vars.
func TestRadioFetchService_Janitor_Defaults(t *testing.T) {
	t.Setenv("RADIO_JANITOR_INTERVAL_HOURS", "")
	t.Setenv("RADIO_JANITOR_DORMANT_DAYS", "")
	t.Setenv("RADIO_JANITOR_BACKFILL_LOOKBACK_DAYS", "")
	svc := NewRadioFetchService(&RadioService{db: nil}, nil)
	if !svc.janitorEnabled {
		t.Fatal("janitor should be enabled by default")
	}
	if svc.janitorInterval != DefaultJanitorInterval {
		t.Fatalf("expected default interval %v, got %v", DefaultJanitorInterval, svc.janitorInterval)
	}
	if svc.janitorDormantDays != DefaultJanitorDormantDays {
		t.Fatalf("expected default dormant %d, got %d", DefaultJanitorDormantDays, svc.janitorDormantDays)
	}
	if svc.janitorBackfillLookbackDays != DefaultJanitorBackfillLookbackDays {
		t.Fatalf("expected default lookback %d, got %d", DefaultJanitorBackfillLookbackDays, svc.janitorBackfillLookbackDays)
	}
}

func TestRadioFetchService_Janitor_EnvOverride(t *testing.T) {
	t.Setenv("RADIO_JANITOR_INTERVAL_HOURS", "12")
	t.Setenv("RADIO_JANITOR_DORMANT_DAYS", "45")
	t.Setenv("RADIO_JANITOR_BACKFILL_LOOKBACK_DAYS", "60")
	svc := NewRadioFetchService(&RadioService{db: nil}, nil)
	if !svc.janitorEnabled || svc.janitorInterval != 12*time.Hour {
		t.Fatalf("expected enabled + 12h, got enabled=%v interval=%v", svc.janitorEnabled, svc.janitorInterval)
	}
	if svc.janitorDormantDays != 45 || svc.janitorBackfillLookbackDays != 60 {
		t.Fatalf("expected dormant=45 lookback=60, got %d / %d", svc.janitorDormantDays, svc.janitorBackfillLookbackDays)
	}
}

func TestRadioFetchService_Janitor_DisableViaZero(t *testing.T) {
	t.Setenv("RADIO_JANITOR_INTERVAL_HOURS", "0")
	svc := NewRadioFetchService(&RadioService{db: nil}, nil)
	if svc.janitorEnabled {
		t.Fatal("RADIO_JANITOR_INTERVAL_HOURS=0 should disable the janitor")
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
// permanent-trips-at-threshold, half-open trial, no-wedge-after-reset) moved from
// the in-memory map to radio_station_health in PSY-1140. Its pure transition logic
// is table-tested in radio_breaker_test.go; the DB wiring (gate → run → health
// write-back, manual-probe policy, restart survival, and per-station isolation — now
// a station_id-PK schema property, tested via two distinct stations) is in
// radio_sync_integration_test.go's RadioSyncSuite.

// TestFetchStationWithRetry_TransientRecovers exercises the transient-retry
// recovery path: a transient error on the first attempt followed by success on
// the retry must surface as success (no error reported to the cycle counter).
func TestFetchStationWithRetry_TransientRecovers(t *testing.T) {
	svc := newTestFetchService()
	svc.retryBackoffFn = func(int) time.Duration { return 0 } // no real sleep
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

	got, err := svc.fetchStationWithRetry(1, "TestStation", "fetch", call)

	if err != nil {
		t.Fatalf("expected nil error after retry recovery; got %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil result after retry recovery")
	}
	if calls.Load() != 2 {
		t.Fatalf("expected exactly 2 calls (initial + 1 retry); got %d", calls.Load())
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
	if elapsed >= radioRetryBackoffBase {
		t.Fatalf("permanent path should not sleep; elapsed=%v >= base %v", elapsed, radioRetryBackoffBase)
	}
}

// TestFetchStationWithRetry_TransientPersists confirms that a station whose
// transient error doesn't clear on retry surfaces the retry error to the
// caller (which then bumps the transient counter — NOT the breaker counter).
func TestFetchStationWithRetry_TransientPersists(t *testing.T) {
	svc := newTestFetchService()
	svc.retryBackoffFn = func(int) time.Duration { return 0 } // no real sleep

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
	// Initial + (radioRetryMaxAttempts-1) retries; the per-client budget is inactive
	// at this low volume so all retries are attempted (PSY-1142).
	if calls.Load() != int32(radioRetryMaxAttempts) {
		t.Fatalf("expected %d total attempts; got %d", radioRetryMaxAttempts, calls.Load())
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
	// A long backoff so the closed stopCh deterministically wins the select before
	// time.After fires (with Full Jitter the real delay could be ~0 and race).
	svc.retryBackoffFn = func(int) time.Duration { return 30 * time.Second }
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
	// We took the stopCh branch, so elapsed should be far less than the 30s backoff.
	if elapsed >= radioRetryBackoffBase {
		t.Fatalf("shutdown should abort backoff; elapsed=%v", elapsed)
	}
}

// TestFullJitterBackoff asserts BOUNDS, not exact values (Full Jitter is random):
// the nth retry delay lies in [0, min(cap, base·2^n)), and a large exponent clamps
// to the cap without overflowing negative (PSY-1142).
func TestFullJitterBackoff(t *testing.T) {
	for exp := 0; exp < 4; exp++ {
		ceiling := radioRetryBackoffCap
		if scaled := radioRetryBackoffBase << exp; scaled < ceiling {
			ceiling = scaled
		}
		for i := 0; i < 300; i++ {
			d := fullJitterBackoff(exp)
			if d < 0 || d >= ceiling {
				t.Fatalf("exp=%d: delay %v out of [0,%v)", exp, d, ceiling)
			}
		}
	}
	for i := 0; i < 300; i++ {
		if d := fullJitterBackoff(40); d < 0 || d >= radioRetryBackoffCap {
			t.Fatalf("large exp must clamp to cap; got %v", d)
		}
	}
}

// TestRetryBudget covers the per-client tier (PSY-1142): inactive below minRequests,
// caps retries at ~ratio of requests, and recovers after the window expires. Uses an
// injected clock for determinism.
func TestRetryBudget(t *testing.T) {
	at := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)

	t.Run("inactive below minRequests", func(t *testing.T) {
		b := newRetryBudget()
		b.now = func() time.Time { return at }
		for i := 0; i < radioRetryBudgetMinReqs-1; i++ {
			b.noteRequest()
		}
		if !b.allowRetry() {
			t.Fatal("below minRequests the budget must allow retries (too little volume to storm)")
		}
	})

	t.Run("caps retries at the ratio, then recovers after the window", func(t *testing.T) {
		now := at
		b := newRetryBudget()
		b.now = func() time.Time { return now }
		const reqs = 20 // 10% → 2 retries permitted
		for i := 0; i < reqs; i++ {
			b.noteRequest()
		}
		allowed := 0
		for i := 0; i < 5; i++ {
			if b.allowRetry() {
				allowed++
			}
		}
		if allowed != 2 {
			t.Fatalf("expected 2 retries within the 10%% budget of %d requests; allowed=%d", reqs, allowed)
		}
		// Advance past the window → all events expire → budget recovers.
		now = now.Add(radioRetryBudgetWindow + time.Second)
		if !b.allowRetry() {
			t.Fatal("after the window expires the budget must recover")
		}
	})
}

// TestFetchStationWithRetry_BudgetSheds verifies the per-client budget actually
// stops a transient retry in fetchStationWithRetry when it is exhausted (PSY-1142).
func TestFetchStationWithRetry_BudgetSheds(t *testing.T) {
	svc := newTestFetchService()
	svc.retryBackoffFn = func(int) time.Duration { return 0 }

	at := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	b := svc.budget()
	b.now = func() time.Time { return at }
	// Pre-load a clearly-over-budget window: 100 requests + 20 retries (20% > 10%).
	for i := 0; i < 100; i++ {
		b.events = append(b.events, budgetEvent{at: at})
	}
	for i := 0; i < 20; i++ {
		b.events = append(b.events, budgetEvent{at: at, retry: true})
	}

	var calls atomic.Int32
	call := func() (any, error) {
		calls.Add(1)
		return nil, context.DeadlineExceeded // transient
	}
	_, err := svc.fetchStationWithRetry(1, "S", "fetch", call)
	if err == nil {
		t.Fatal("expected the transient error to surface")
	}
	if calls.Load() != 1 {
		t.Fatalf("a budget-shed transient must not retry; got %d calls", calls.Load())
	}
}

// TestFetchStationWithRetry_TurnsPermanentMidRetry covers the branch where a retry's
// error reclassifies from transient to permanent: the loop must stop and surface the
// permanent error, not keep retrying (PSY-1142).
func TestFetchStationWithRetry_TurnsPermanentMidRetry(t *testing.T) {
	svc := newTestFetchService()
	svc.retryBackoffFn = func(int) time.Duration { return 0 }

	var calls atomic.Int32
	call := func() (any, error) {
		if calls.Add(1) == 1 {
			return nil, context.DeadlineExceeded // transient → triggers a retry
		}
		return nil, &RadioHTTPError{Provider: "KEXP API", StatusCode: 500} // permanent → stop
	}

	_, err := svc.fetchStationWithRetry(1, "S", "fetch", call)
	if err == nil {
		t.Fatal("expected the permanent error to surface")
	}
	if classifyError(err) != kindPermanent {
		t.Fatalf("surfaced error must be permanent; got %v", errorKindName(classifyError(err)))
	}
	// Exactly 2 calls: the initial transient + the retry that turned permanent (no 3rd).
	if calls.Load() != 2 {
		t.Fatalf("must stop once the error turns permanent; got %d calls", calls.Load())
	}
}

// TestRetryBudget_Concurrent proves the shared budget is race-free under the
// fetch + discover goroutines' concurrent access (the gate is `go test -race`). PSY-1142.
func TestRetryBudget_Concurrent(t *testing.T) {
	b := newRetryBudget()
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				b.noteRequest()
				b.allowRetry()
			}
		}()
	}
	wg.Wait() // no assertion beyond "no race / no panic / no deadlock"
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
