package system

import (
	"context"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/db"
)

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
	if resp.Body.Status == "unhealthy" {
		// Detail names the component, so an alert is actionable without a second
		// request. checkDatabaseHealth's Error strings are fixed literals — no
		// connection string or driver text reaches the response.
		return nil, huma.Error503ServiceUnavailable(
			"not ready: database " + resp.Body.Components["database"].Error,
		)
	}
	return resp, nil
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
	if dbHealth.Status == "unhealthy" {
		resp.Body.Status = "unhealthy"
	} else {
		resp.Body.Status = "healthy"
	}

	return resp
}

// checkDatabaseHealth verifies database connectivity
func checkDatabaseHealth(ctx context.Context) ComponentHealth {
	start := time.Now()

	// Get the underlying sql.DB from GORM
	gormDB := db.GetDB()
	if gormDB == nil {
		return ComponentHealth{
			Status:  "unhealthy",
			Latency: time.Since(start).String(),
			Error:   "database not initialized",
		}
	}

	sqlDB, err := gormDB.DB()
	if err != nil {
		return ComponentHealth{
			Status:  "unhealthy",
			Latency: time.Since(start).String(),
			Error:   "failed to get database connection",
		}
	}

	// Ping the database with context for timeout support
	if err := sqlDB.PingContext(ctx); err != nil {
		return ComponentHealth{
			Status:  "unhealthy",
			Latency: time.Since(start).String(),
			Error:   "database ping failed",
		}
	}

	return ComponentHealth{
		Status:  "healthy",
		Latency: time.Since(start).String(),
	}
}
