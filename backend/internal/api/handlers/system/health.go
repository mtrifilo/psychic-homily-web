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
//
// There is deliberately no "degraded": overallStatus derives the summary by
// matching healthy and treating everything else as unhealthy, so no code path
// could emit it. Publishing a status the API cannot produce would give
// consumers an alert rule that never fires. Add it together with the
// critical/non-critical classification that would make it reachable, not
// before.
const (
	statusHealthy   = "healthy"
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
//
// A var, not a const, solely so tests can shorten it — nothing in production
// reassigns it.
var databaseProbeTimeout = 2 * time.Second

// ComponentHealth represents the health status of a single component
type ComponentHealth struct {
	Status  string `json:"status" example:"healthy" doc:"Component health status: healthy, unhealthy"`
	Latency string `json:"latency,omitempty" example:"1.23ms" doc:"Response time for the health check"`
	Error   string `json:"error,omitempty" example:"ping failed" doc:"Error message if unhealthy"`
}

// HealthResponse represents the health check response
type HealthResponse struct {
	// A 200 carrying neither Cache-Control nor Expires is heuristically
	// cacheable (RFC 9111 §4.2.2), and the caching failure direction here is
	// always the dangerous one: a stale 200 masks an outage, while a 503 is not
	// heuristically cacheable and so cannot produce a false alarm. An outage
	// hidden behind a cached success is the exact shape of the incident these
	// endpoints exist to catch, so refuse the cache explicitly.
	CacheControl string `header:"Cache-Control"`

	Body struct {
		Status     string                     `json:"status" example:"healthy" doc:"Overall health status: healthy, unhealthy"`
		Components map[string]ComponentHealth `json:"components" doc:"Health status of individual components"`
		Timestamp  string                     `json:"timestamp" example:"2024-01-15T10:30:00Z" doc:"Time of health check"`
	}
}

// LIVENESS vs READINESS — the split is load-bearing, do not collapse it.
//
// /health is the platform's DEPLOY healthcheck (backend/railway.toml:
// healthcheckPath = "/health"). It answers "is this process alive?" and
// therefore ALWAYS returns 200 while the process is serving, even when it
// reports a dependency as unhealthy in the body.
//
// That looks like a bug and is not. Two verified facts set the shape:
//
//  1. The deploy healthcheck gates a NEW deployment going live — if it never
//     returns 200 the deployment is marked failed and the previous one keeps
//     serving. Railway does not poll it after a deployment is live, so it is
//     not a runtime monitor and nothing here restarts a running process.
//  2. This process already refuses to start without a database:
//     cmd/server/main.go log.Fatalf's when db.Connect fails, before the HTTP
//     server binds.
//
// Together those mean a dependency-aware /health would add no signal. During a
// database outage a new deployment never reaches the healthcheck at all — it
// exits at boot — so making /health 503 would only add a second way to express
// a failure the boot sequence already enforces, while coupling the deploy gate
// to a runtime condition it should not care about.
//
// The case that /health genuinely cannot serve is the one that matters here: a
// process that booted fine and whose database failed LATER. It is alive, it is
// correctly reporting 200 for liveness, and nothing about that answers whether
// it can still do its job.
//
// /health/ready answers that question and returns 503 when it cannot. It lives
// on its own path precisely so the monitorable signal is not the same signal a
// platform gates deploys on. That is the endpoint uptime monitoring and
// alerting should watch.
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
	if resp.Body.Status != statusHealthy {
		return nil, huma.Error503ServiceUnavailable("not ready: " + unhealthyDetail(resp))
	}
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
	resp := &HealthResponse{CacheControl: "no-store"}
	resp.Body.Components = make(map[string]ComponentHealth)
	resp.Body.Timestamp = time.Now().UTC().Format(time.RFC3339)

	// Check database health
	dbHealth := checkDatabaseHealth(ctx)
	resp.Body.Components["database"] = dbHealth

	resp.Body.Status = overallStatus(resp.Body.Components)

	return resp
}

// overallStatus summarises the component map.
//
// The polarity is the point: it matches HEALTHY and treats everything else as
// unhealthy, rather than matching "unhealthy" and defaulting to healthy. Those
// are equivalent for the statuses we emit today and differ for any status
// neither branch anticipated — a typo, a new component reporting its own
// vocabulary, a zero value from an uninitialised struct. A monitoring endpoint
// must fail CLOSED on a status it does not recognise; reporting "healthy"
// because a value was unfamiliar is how a monitor goes quiet during an outage.
//
// Split out from buildHealthResponse so this can be tested against inputs the
// live dependency check cannot produce.
func overallStatus(components map[string]ComponentHealth) string {
	// No components means nothing was actually checked. Reporting healthy would
	// assert a fact never established — the same fail-open shape, one level up.
	if len(components) == 0 {
		return statusUnhealthy
	}
	for _, c := range components {
		if c.Status != statusHealthy {
			return statusUnhealthy
		}
	}
	return statusHealthy
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
