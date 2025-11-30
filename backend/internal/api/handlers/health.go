package handlers

import (
	"context"
)

// HealthResponse represents the health check response
type HealthResponse struct {
	Body struct {
		Status string `json:"status" example:"ok" doc:"Health status of the API"`
	}
}

// HealthHandler handles health check requests
func HealthHandler(ctx context.Context, input *struct{}) (*HealthResponse, error) {
	resp := &HealthResponse{}
	resp.Body.Status = "ok"
	return resp, nil
}
