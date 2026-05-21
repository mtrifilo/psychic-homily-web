package system

import (
	"context"
	"testing"

	"psychic-homily-backend/db"
)

// TestHealthHandler_DBNotInitialized exercises the no-DB branch with no
// external dependencies: when the global GORM handle is nil, the database
// component is reported unhealthy and the overall status follows.
func TestHealthHandler_DBNotInitialized(t *testing.T) {
	// db.DB is a package global; save and restore it so this test never
	// leaks a nil handle into a later test that expects a real connection.
	prev := db.DB
	db.DB = nil
	t.Cleanup(func() { db.DB = prev })

	resp, err := HealthHandler(context.Background(), &struct{}{})
	if err != nil {
		t.Fatalf("HealthHandler returned error: %v", err)
	}
	if resp.Body.Status != "unhealthy" {
		t.Errorf("overall status = %q, want \"unhealthy\"", resp.Body.Status)
	}
	if resp.Body.Timestamp == "" {
		t.Error("expected non-empty timestamp")
	}
	dbHealth, ok := resp.Body.Components["database"]
	if !ok {
		t.Fatal("expected a \"database\" component in the response")
	}
	if dbHealth.Status != "unhealthy" {
		t.Errorf("database status = %q, want \"unhealthy\"", dbHealth.Status)
	}
	if dbHealth.Error != "database not initialized" {
		t.Errorf("database error = %q, want \"database not initialized\"", dbHealth.Error)
	}
}

// TestCheckDatabaseHealth_NotInitialized covers checkDatabaseHealth's nil
// branch directly (the same condition, asserted at the function boundary).
func TestCheckDatabaseHealth_NotInitialized(t *testing.T) {
	prev := db.DB
	db.DB = nil
	t.Cleanup(func() { db.DB = prev })

	got := checkDatabaseHealth(context.Background())
	if got.Status != "unhealthy" {
		t.Errorf("status = %q, want \"unhealthy\"", got.Status)
	}
	if got.Latency == "" {
		t.Error("expected non-empty latency even on the failure path")
	}
}
