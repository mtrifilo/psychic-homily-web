package system

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"

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

// TestReadinessHandler_DBHealthy_Integration covers readiness' success branch:
// with a reachable database it returns no error (HTTP 200) and a body that
// agrees with /health's, since both are assembled by buildHealthResponse.
func TestReadinessHandler_DBHealthy_Integration(t *testing.T) {
	testDB := testutil.SetupTestPostgres(t)
	t.Cleanup(testDB.Cleanup)

	prev := db.DB
	db.DB = testDB.DB
	t.Cleanup(func() { db.DB = prev })

	resp, err := ReadinessHandler(context.Background(), &struct{}{})
	if err != nil {
		t.Fatalf("ReadinessHandler returned error with a healthy database: %v", err)
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
}

// TestReadinessHandler_PingFails_Integration covers the 503 path that actually
// occurs in production.
//
// The unit tests reach 503 through a nil db.DB handle, which the server can
// never be in while serving: cmd/server/main.go calls log.Fatalf if db.Connect
// fails, so a running process always has a handle. The realistic outage is a
// handle that EXISTS and whose connection fails — Postgres restarting, the
// volume full, the proxy refusing. That is this test, driven by closing the
// pool out from under the handler.
func TestReadinessHandler_PingFails_Integration(t *testing.T) {
	testDB := testutil.SetupTestPostgres(t)
	t.Cleanup(testDB.Cleanup)

	prev := db.DB
	db.DB = testDB.DB
	t.Cleanup(func() { db.DB = prev })

	sqlDB, err := testDB.DB.DB()
	if err != nil {
		t.Fatalf("could not reach the underlying sql.DB: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("could not close the pool: %v", err)
	}

	resp, err := ReadinessHandler(context.Background(), &struct{}{})
	if err == nil {
		t.Fatalf("ReadinessHandler returned no error with a closed pool (resp: %+v)", resp)
	}

	var statusErr huma.StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("expected huma.StatusError, got %T (%v)", err, err)
	}
	if got := statusErr.GetStatus(); got != 503 {
		t.Errorf("status = %d, want 503", got)
	}
	const want = "not ready: database: ping failed"
	if got := statusErr.Error(); got != want {
		t.Errorf("error message = %q, want %q", got, want)
	}

	// Liveness must survive the same failure — this is the pairing that the
	// whole split exists to provide, asserted against a real broken connection
	// rather than a nil handle.
	live, liveErr := HealthHandler(context.Background(), &struct{}{})
	if liveErr != nil {
		t.Fatalf("HealthHandler must not error with a broken database; got %v", liveErr)
	}
	if live.Body.Status != "unhealthy" {
		t.Errorf("liveness body status = %q, want \"unhealthy\"", live.Body.Status)
	}
}

// TestReadinessHandler_PingTimesOut_Integration pins the bounded probe and the
// literal that distinguishes a hung dependency from a refused one.
//
// A database that accepts the connection and never answers is the shape of the
// outage this endpoint was built for, and it is the branch most likely to be
// silently lost in a refactor — deleting the WithTimeout leaves every other
// test green.
func TestReadinessHandler_PingTimesOut_Integration(t *testing.T) {
	testDB := testutil.SetupTestPostgres(t)
	t.Cleanup(testDB.Cleanup)

	prev := db.DB
	db.DB = testDB.DB
	t.Cleanup(func() { db.DB = prev })

	prevTimeout := databaseProbeTimeout
	databaseProbeTimeout = 1 * time.Nanosecond
	t.Cleanup(func() { databaseProbeTimeout = prevTimeout })

	start := time.Now()
	resp, err := ReadinessHandler(context.Background(), &struct{}{})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("ReadinessHandler returned no error past the probe deadline (resp: %+v)", resp)
	}
	var statusErr huma.StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("expected huma.StatusError, got %T (%v)", err, err)
	}
	if got := statusErr.GetStatus(); got != 503 {
		t.Errorf("status = %d, want 503", got)
	}
	const want = "not ready: database: ping timed out"
	if got := statusErr.Error(); got != want {
		t.Errorf("error message = %q, want %q", got, want)
	}
	// The point of the bound is that it RETURNS rather than blocking. A
	// generous ceiling still fails if the deadline stops being honoured.
	if elapsed > 5*time.Second {
		t.Errorf("probe took %s; the deadline is not bounding the call", elapsed)
	}
}
