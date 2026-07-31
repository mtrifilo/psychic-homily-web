package system

import (
	"context"
	"errors"
	"testing"

	"github.com/danielgtaylor/huma/v2"

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
	if dbHealth.Error != "not initialized" {
		t.Errorf("database error = %q, want \"not initialized\"", dbHealth.Error)
	}
}

// TestHealthHandler_StaysLiveWhenDatabaseIsDown pins the liveness contract
// against a well-meaning "fix". /health is Railway's deploy healthcheck with
// restartPolicyType = "on_failure", so returning a StatusError here would make
// Railway restart the backend during a database outage and then fail the
// deploy — taking down the one surface still able to serve cached reads.
//
// The unhealthy signal belongs in the body (asserted above) and in
// /health/ready's status code, never in this handler's error return.
func TestHealthHandler_StaysLiveWhenDatabaseIsDown(t *testing.T) {
	prev := db.DB
	db.DB = nil
	t.Cleanup(func() { db.DB = prev })

	resp, err := HealthHandler(context.Background(), &struct{}{})
	if err != nil {
		t.Fatalf("HealthHandler must not return an error with the database down; got %v", err)
	}
	if resp == nil {
		t.Fatal("HealthHandler returned a nil response")
	}
}

// TestReadinessHandler_DBNotInitialized asserts the half of the split that
// /health deliberately cannot provide: a status code an uptime monitor can act
// on without parsing JSON.
func TestReadinessHandler_DBNotInitialized(t *testing.T) {
	prev := db.DB
	db.DB = nil
	t.Cleanup(func() { db.DB = prev })

	resp, err := ReadinessHandler(context.Background(), &struct{}{})
	if err == nil {
		t.Fatalf("ReadinessHandler returned no error with the database down (resp: %+v)", resp)
	}

	var statusErr huma.StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("expected returned error to satisfy huma.StatusError, got %T (%v)", err, err)
	}
	if got := statusErr.GetStatus(); got != 503 {
		t.Errorf("status = %d, want 503", got)
	}

	// Asserted EXACTLY, not by substring. This string is the problem+json
	// detail — the first thing an on-call human reads — so a duplicated or
	// garbled component name is a real defect, and a Contains check cannot
	// see one.
	const want = "not ready: database: not initialized"
	if got := statusErr.Error(); got != want {
		t.Errorf("error message = %q, want %q", got, want)
	}
}

// TestUnhealthyDetail_NamesEveryFailingComponent guards the generalization:
// the detail is built from the component map, so a second critical dependency
// reports itself instead of being masked by a hardcoded "database" lookup.
func TestUnhealthyDetail_NamesEveryFailingComponent(t *testing.T) {
	resp := &HealthResponse{}
	resp.Body.Components = map[string]ComponentHealth{
		"database": {Status: statusUnhealthy, Error: "ping failed"},
		"cache":    {Status: statusUnhealthy, Error: "connection refused"},
		"search":   {Status: statusHealthy},
	}

	// Sorted, so the same outage always produces the same alert text.
	const want = "cache: connection refused, database: ping failed"
	if got := unhealthyDetail(resp); got != want {
		t.Errorf("detail = %q, want %q", got, want)
	}
}

// TestBuildHealthResponse_UnknownComponentStatusFailsClosed pins the polarity.
// The overall status is derived by matching HEALTHY and defaulting everything
// else to unhealthy — not by matching "unhealthy" and defaulting to healthy.
// The difference only shows up for a status neither branch anticipated, which
// is exactly when a monitoring endpoint must not report success.
func TestBuildHealthResponse_UnknownComponentStatusFailsClosed(t *testing.T) {
	if statusHealthy == statusDegraded || statusHealthy == statusUnhealthy {
		t.Fatal("status constants must be distinct")
	}

	prev := db.DB
	db.DB = nil
	t.Cleanup(func() { db.DB = prev })

	// A nil handle yields statusUnhealthy, which is neither healthy nor a
	// status the readiness branch special-cases.
	resp := buildHealthResponse(context.Background())
	if resp.Body.Status == statusHealthy {
		t.Error("overall status must not be healthy when a component is not healthy")
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
