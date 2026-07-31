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
// against a well-meaning "fix". /health is the platform's DEPLOY healthcheck,
// so returning a StatusError here would fail every new deployment for as long
// as the database was down — you could not ship the fix for an outage during
// the outage.
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

// TestOverallStatus_FailsClosed pins the polarity against inputs the live
// dependency check cannot produce.
//
// This matters because the obvious alternative — match "unhealthy", default to
// healthy — is indistinguishable from the correct version for the two statuses
// we actually emit. It only diverges on an UNRECOGNISED status, so a test that
// feeds it recognised values passes under both polarities and guards nothing.
func TestOverallStatus_FailsClosed(t *testing.T) {
	cases := []struct {
		name       string
		components map[string]ComponentHealth
		want       string
	}{
		{"all healthy", map[string]ComponentHealth{"database": {Status: statusHealthy}}, statusHealthy},
		{"unhealthy", map[string]ComponentHealth{"database": {Status: statusUnhealthy}}, statusUnhealthy},
		// The cases that distinguish the two polarities:
		{"unrecognised status", map[string]ComponentHealth{"database": {Status: "degraded"}}, statusUnhealthy},
		{"empty status", map[string]ComponentHealth{"database": {Status: ""}}, statusUnhealthy},
		{"typo", map[string]ComponentHealth{"database": {Status: "helthy"}}, statusUnhealthy},
		{"one bad among many", map[string]ComponentHealth{
			"database": {Status: statusHealthy},
			"cache":    {Status: "who knows"},
		}, statusUnhealthy},
		// No components means nothing was checked. Reporting healthy there
		// would be asserting a fact never established.
		{"no components", map[string]ComponentHealth{}, statusUnhealthy},
		{"nil components", nil, statusUnhealthy},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := overallStatus(tc.components); got != tc.want {
				t.Errorf("overallStatus = %q, want %q", got, tc.want)
			}
		})
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
