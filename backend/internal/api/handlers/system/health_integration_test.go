package system

import (
	"context"
	"testing"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/testutil"
)

// TestHealthHandler_DBHealthy_Integration covers the success branch of
// checkDatabaseHealth (and HealthHandler's healthy overall status) against a
// live Postgres test container. checkDatabaseHealth reads the db package
// global rather than an injected handle, so we point the global at the test
// connection for the duration of the test and restore it afterward.
func TestHealthHandler_DBHealthy_Integration(t *testing.T) {
	testDB := testutil.SetupTestPostgres(t)
	t.Cleanup(testDB.Cleanup)

	prev := db.DB
	db.DB = testDB.DB
	t.Cleanup(func() { db.DB = prev })

	resp, err := HealthHandler(context.Background(), &struct{}{})
	if err != nil {
		t.Fatalf("HealthHandler returned error: %v", err)
	}
	if resp.Body.Status != "healthy" {
		t.Errorf("overall status = %q, want \"healthy\"", resp.Body.Status)
	}
	dbHealth, ok := resp.Body.Components["database"]
	if !ok {
		t.Fatal("expected a \"database\" component in the response")
	}
	if dbHealth.Status != "healthy" {
		t.Errorf("database status = %q, want \"healthy\" (error: %s)", dbHealth.Status, dbHealth.Error)
	}
	if dbHealth.Latency == "" {
		t.Error("expected non-empty latency on the healthy path")
	}
}
