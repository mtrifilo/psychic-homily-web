package handlers

import (
	"context"
	"time"

	"psychic-homily-backend/db"
)

// ComponentHealth represents the health status of a single component
type ComponentHealth struct {
	Status      string `json:"status" example:"healthy" doc:"Component health status: healthy, unhealthy"`
	Latency     string `json:"latency,omitempty" example:"1.23ms" doc:"Response time for the health check"`
	Error       string `json:"error,omitempty" example:"connection refused" doc:"Error message if unhealthy"`
}

// HealthResponse represents the health check response
type HealthResponse struct {
	Body struct {
		Status     string                     `json:"status" example:"healthy" doc:"Overall health status: healthy, degraded, unhealthy"`
		Components map[string]ComponentHealth `json:"components" doc:"Health status of individual components"`
		Timestamp  string                     `json:"timestamp" example:"2024-01-15T10:30:00Z" doc:"Time of health check"`
	}
}

// HealthHandler handles health check requests
// Returns detailed health information about the API and its dependencies
func HealthHandler(ctx context.Context, input *struct{}) (*HealthResponse, error) {
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

	return resp, nil
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
			Error:   "failed to get database connection: " + err.Error(),
		}
	}

	// Ping the database with context for timeout support
	if err := sqlDB.PingContext(ctx); err != nil {
		return ComponentHealth{
			Status:  "unhealthy",
			Latency: time.Since(start).String(),
			Error:   "database ping failed: " + err.Error(),
		}
	}

	return ComponentHealth{
		Status:  "healthy",
		Latency: time.Since(start).String(),
	}
}
