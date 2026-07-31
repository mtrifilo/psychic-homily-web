package system

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/db"
)

// Component and overall health statuses. These are a wire contract — they
// appear in the OpenAPI doc tags below and in every consumer's alert rule — so
// they live here as constants rather than as literals repeated across handlers
// and tests.
const (
	statusHealthy   = "healthy"
	statusDegraded  = "degraded"
	statusUnhealthy = "unhealthy"
)

// databaseProbeTimeout bounds the dependency check so BOTH endpoints answer in
// bounded time under every failure mode.
//
// Without it the probe inherits only the request context, and the server sets no
// ReadTimeout/WriteTimeout, so a database that accepts the TCP connection but
// never answers — a blackholed route, a saturated pool, a storage stall — makes
// the handler block instead of respond. That breaks both halves of the split at
// once: liveness stops returning its guaranteed 200, and readiness never emits
// the 503 an alert rule is written against. A monitor sees a timeout, which is
// exactly the ambiguous signal this endpoint exists to replace.
//
// A hung dependency is not a hypothetical failure mode here: it is the observed
// shape of the outage this endpoint was built for, where TCP and TLS connected
// in ~60ms and no HTTP response ever came.
const databaseProbeTimeout = 2 * time.Second

// ComponentHealth represents the health status of a single component
type ComponentHealth struct {
	Status  string `json:"status" example:"healthy" doc:"Component health status: healthy, unhealthy"`
	Latency string `json:"latency,omitempty" example:"1.23ms" doc:"Response time for the health check"`
	Error   string `json:"error,omitempty" example:"connection refused" doc:"Error message if unhealthy"`
}

// HealthResponse represents the health check response
type HealthResponse struct {
	Body struct {
		Status     string                     `json:"status" example:"healthy" doc:"Overall health status: healthy, degraded, unhealthy"`
		Components map[string]ComponentHealth `json:"components" doc:"Health status of individual components"`
		Timestamp  string                     `json:"timestamp" example:"2024-01-15T10:30:00Z" doc:"Time of health check"`
	}
}

// LIVENESS vs READINESS — the split is load-bearing, do not collapse it.
//
// /health is Railway's deploy healthcheck (backend/railway.toml: healthcheckPath
// = "/health", restartPolicyType = "on_failure"). It answers "is this process
// alive?" and therefore ALWAYS returns 200 while the process is serving, even
// when it reports a dependency as unhealthy in the body.
//
// That looks like a bug and is not. Making /health return 503 when the database
// is unreachable would hand Railway a reason to restart the backend during a
// database outage, up to restartPolicyMaxRetries, and then mark the deploy
// failed — converting "API up, database down" into "nothing up at all". A
// prolonged database outage would take the API down with it and remove the one
// surface still able to serve cached reads.
//
// /health/ready answers the different question — "can this process actually do
// its job?" — and returns 503 when it cannot. That is the endpoint uptime
// monitoring and alerting should watch. Nothing restarts on its result.
//
// If you are here because a monitor reported 200 during an outage: point the
// monitor at /health/ready. Do not change /health.

// HealthHandler handles liveness checks. Always 200 while the process serves;
// the body carries per-component detail.
func HealthHandler(ctx context.Context, input *struct{}) (*HealthResponse, error) {
	return buildHealthResponse(ctx), nil
}

// ReadinessHandler handles readiness checks: 200 when every critical dependency
// is reachable, 503 when one is not.
//
// The status code is the point — it is what lets an uptime monitor detect a
// database outage without parsing JSON, and what /health deliberately cannot
// provide. The healthy body is identical to /health's so the two can be diffed.
func ReadinessHandler(ctx context.Context, input *struct{}) (*HealthResponse, error) {
	resp := buildHealthResponse(ctx)
	if resp.Body.Status == statusUnhealthy {
		return nil, huma.Error503ServiceUnavailable("not ready: " + unhealthyDetail(resp))
	}
	// statusDegraded stays READY on purpose: it means a NON-critical component is
	// down, and the process can still do its job. Taking readiness away for it
	// would page someone (and, wherever readiness gates traffic, shed it) over a
	// failure the service is designed to absorb. A component whose loss should
	// fail readiness is critical by definition and belongs in the statusUnhealthy
	// branch above.
	return resp, nil
}

// unhealthyDetail names every failing component and why, so the alert is
// actionable without a second request. Built from the component map rather than
// a hardcoded key, so adding a critical dependency cannot leave this reporting
// the one component that is still fine.
//
// Component Error strings are fixed literals (see checkDatabaseHealth) — no
// connection string, host, or driver text reaches the response.
func unhealthyDetail(resp *HealthResponse) string {
	failures := make([]string, 0, len(resp.Body.Components))
	for name, c := range resp.Body.Components {
		if c.Status == statusHealthy {
			continue
		}
		failures = append(failures, name+": "+c.Error)
	}
	// Map iteration order is random; sort so the same outage produces the same
	// alert text instead of a new one each probe.
	sort.Strings(failures)
	return strings.Join(failures, ", ")
}

// buildHealthResponse assembles the component report both endpoints share, so
// liveness and readiness can never disagree about what "healthy" means.
func buildHealthResponse(ctx context.Context) *HealthResponse {
	resp := &HealthResponse{}
	resp.Body.Components = make(map[string]ComponentHealth)
	resp.Body.Timestamp = time.Now().UTC().Format(time.RFC3339)

	// Check database health
	dbHealth := checkDatabaseHealth(ctx)
	resp.Body.Components["database"] = dbHealth

	// Determine overall status based on component health
	// - healthy: all components healthy
	// - degraded: some non-critical components unhealthy (none currently)
	// - unhealthy: critical components (database) unhealthy
	if dbHealth.Status == statusHealthy {
		resp.Body.Status = statusHealthy
	} else {
		resp.Body.Status = statusUnhealthy
	}

	return resp
}

// checkDatabaseHealth verifies database connectivity.
//
// Every Error string here is a fixed literal. Driver errors are deliberately
// NOT propagated: this response is public and unauthenticated, and a wrapped
// driver error leaks the host, port, and database name. The literals omit the
// component name because the caller supplies it (see unhealthyDetail).
func checkDatabaseHealth(ctx context.Context) ComponentHealth {
	start := time.Now()

	unhealthy := func(reason string) ComponentHealth {
		return ComponentHealth{
			Status:  statusUnhealthy,
			Latency: time.Since(start).String(),
			Error:   reason,
		}
	}

	// Get the underlying sql.DB from GORM
	gormDB := db.GetDB()
	if gormDB == nil {
		return unhealthy("not initialized")
	}

	sqlDB, err := gormDB.DB()
	if err != nil {
		return unhealthy("connection unavailable")
	}

	// Bound the ping so a hung database cannot hold the response open. See
	// databaseProbeTimeout — an unbounded probe is why a stalled dependency
	// produces a timeout instead of the status code alerting is written against.
	ctx, cancel := context.WithTimeout(ctx, databaseProbeTimeout)
	defer cancel()

	if err := sqlDB.PingContext(ctx); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			// Distinct from "ping failed": a refused connection is a database
			// that is down, a timeout is one that is reachable but not
			// answering. They point at different causes during an incident.
			return unhealthy("ping timed out")
		}
		return unhealthy("ping failed")
	}

	return ComponentHealth{
		Status:  statusHealthy,
		Latency: time.Since(start).String(),
	}
}
