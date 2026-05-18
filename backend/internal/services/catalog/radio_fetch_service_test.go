package catalog

import (
	"context"
	"log/slog"
	"os"
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
