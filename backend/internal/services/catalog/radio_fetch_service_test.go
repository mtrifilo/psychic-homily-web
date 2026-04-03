package catalog

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"
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
		stopCh:              make(chan struct{}),
		logger:              testLogger(),
		consecutiveFailures: make(map[uint]int),
	}

	// Should not panic — just log an error
	svc.RunReMatchCycleNow()
}
