package services

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// --- NewCleanupService ---

func TestNewCleanupService(t *testing.T) {
	// Pass nil — will call db.GetDB() which returns nil in test env
	svc := NewCleanupService(nil)
	assert.NotNil(t, svc)
	assert.Equal(t, DefaultCleanupInterval, svc.interval)
	assert.NotNil(t, svc.stopCh)
	assert.NotNil(t, svc.logger)
}

func TestNewCleanupService_EnvOverride(t *testing.T) {
	t.Setenv("CLEANUP_INTERVAL_HOURS", "12")
	svc := NewCleanupService(nil)
	assert.Equal(t, 12*time.Hour, svc.interval)
}

func TestNewCleanupService_InvalidEnvIgnored(t *testing.T) {
	t.Setenv("CLEANUP_INTERVAL_HOURS", "not-a-number")
	svc := NewCleanupService(nil)
	assert.Equal(t, DefaultCleanupInterval, svc.interval)
}

func TestNewCleanupService_ZeroEnvIgnored(t *testing.T) {
	t.Setenv("CLEANUP_INTERVAL_HOURS", "0")
	svc := NewCleanupService(nil)
	assert.Equal(t, DefaultCleanupInterval, svc.interval)
}

func TestNewCleanupService_NegativeEnvIgnored(t *testing.T) {
	t.Setenv("CLEANUP_INTERVAL_HOURS", "-5")
	svc := NewCleanupService(nil)
	assert.Equal(t, DefaultCleanupInterval, svc.interval)
}

// --- Start / Stop lifecycle ---

func TestCleanupService_StartStop(t *testing.T) {
	svc := NewCleanupService(nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start should not block or panic (cleanup cycle will log DB error and continue)
	svc.Start(ctx)

	// Give it a moment to run the initial cleanup cycle
	time.Sleep(50 * time.Millisecond)

	// Stop should return without hanging
	done := make(chan struct{})
	go func() {
		svc.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Success — stopped cleanly
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() did not return within timeout")
	}
}

func TestCleanupService_ContextCancellation(t *testing.T) {
	svc := NewCleanupService(nil)
	ctx, cancel := context.WithCancel(context.Background())

	svc.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	// Cancel the context — run loop should exit
	cancel()

	// wg.Wait should unblock
	done := make(chan struct{})
	go func() {
		svc.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("goroutine did not exit after context cancellation")
	}
}

// --- RunCleanupNow ---

func TestCleanupService_RunCleanupNow_NilDB(t *testing.T) {
	svc := NewCleanupService(nil)
	// Should not panic — the DB error gets logged and the cycle returns
	assert.NotPanics(t, func() {
		svc.RunCleanupNow()
	})
}
