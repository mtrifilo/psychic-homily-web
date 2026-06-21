// Package sources exposes admin endpoints over the source-config registry
// (PSY-1164). The /ingest skill (the Catalog Refresh executor) uses these to
// read the stalest sources and stamp last_refreshed_at after a run.
package sources

import (
	"context"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/logger"
	adminm "psychic-homily-backend/internal/models/admin"
	"psychic-homily-backend/internal/services/sourceregistry"
)

// SourceHandler handles source-config registry admin endpoints.
type SourceHandler struct {
	svc *sourceregistry.SourceConfigService
}

// NewSourceHandler creates a new source registry handler.
func NewSourceHandler(svc *sourceregistry.SourceConfigService) *SourceHandler {
	return &SourceHandler{svc: svc}
}

// SourceConfigInfo is the API representation of a source-config row.
type SourceConfigInfo struct {
	ID                  uint       `json:"id"`
	EntityType          string     `json:"entity_type"`
	EntityID            uint       `json:"entity_id"`
	SourceURL           *string    `json:"source_url"`
	LastRefreshedAt     *time.Time `json:"last_refreshed_at"`
	LastContentHash     *string    `json:"last_content_hash"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

func toInfo(c *adminm.SourceConfig) SourceConfigInfo {
	return SourceConfigInfo{
		ID:                  c.ID,
		EntityType:          c.EntityType,
		EntityID:            c.EntityID,
		SourceURL:           c.SourceURL,
		LastRefreshedAt:     c.LastRefreshedAt,
		LastContentHash:     c.LastContentHash,
		ConsecutiveFailures: c.ConsecutiveFailures,
		CreatedAt:           c.CreatedAt,
		UpdatedAt:           c.UpdatedAt,
	}
}

// --- List stale ---

// ListStaleSourcesRequest is the Huma request for GET /admin/sources.
type ListStaleSourcesRequest struct {
	Limit       int `query:"limit" doc:"Max rows to return (0 = all)"`
	MaxFailures int `query:"max_failures" doc:"Exclude sources at or over this consecutive-failure count (0 = no filter)"`
}

// ListStaleSourcesResponse is the Huma response for GET /admin/sources.
type ListStaleSourcesResponse struct {
	Body struct {
		Sources []SourceConfigInfo `json:"sources"`
		Count   int                `json:"count"`
	}
}

// ListStaleHandler handles GET /admin/sources — stalest sources first.
func (h *SourceHandler) ListStaleHandler(ctx context.Context, req *ListStaleSourcesRequest) (*ListStaleSourcesResponse, error) {
	configs, err := h.svc.ListStale(req.Limit, req.MaxFailures)
	if err != nil {
		logger.FromContext(ctx).Error("source_list_stale_failed", "error", err.Error())
		return nil, huma.Error500InternalServerError("Failed to list stale sources")
	}

	resp := &ListStaleSourcesResponse{}
	resp.Body.Sources = make([]SourceConfigInfo, 0, len(configs))
	for i := range configs {
		resp.Body.Sources = append(resp.Body.Sources, toInfo(&configs[i]))
	}
	resp.Body.Count = len(configs)
	return resp, nil
}

// --- Register (upsert) ---

// RegisterSourceRequest is the Huma request for PUT /admin/sources.
type RegisterSourceRequest struct {
	Body struct {
		EntityType string  `json:"entity_type" enum:"venue,label" doc:"Entity kind the source belongs to"`
		EntityID   uint    `json:"entity_id" doc:"ID of the venue or label"`
		SourceURL  *string `json:"source_url,omitempty" required:"false" doc:"URL of the roster/calendar source"`
	}
}

// RegisterSourceResponse is the Huma response for PUT /admin/sources.
type RegisterSourceResponse struct {
	Body SourceConfigInfo
}

// RegisterHandler handles PUT /admin/sources — register or update a source.
func (h *SourceHandler) RegisterHandler(ctx context.Context, req *RegisterSourceRequest) (*RegisterSourceResponse, error) {
	out, err := h.svc.CreateOrUpdate(&adminm.SourceConfig{
		EntityType: req.Body.EntityType,
		EntityID:   req.Body.EntityID,
		SourceURL:  req.Body.SourceURL,
	})
	if err != nil {
		return nil, huma.Error422UnprocessableEntity(err.Error())
	}
	return &RegisterSourceResponse{Body: toInfo(out)}, nil
}

// --- Record refresh ---

// RefreshSourceRequest is the Huma request for POST /admin/sources/refresh.
type RefreshSourceRequest struct {
	Body struct {
		EntityType  string  `json:"entity_type" enum:"venue,label" doc:"Entity kind"`
		EntityID    uint    `json:"entity_id" doc:"ID of the venue or label"`
		ContentHash *string `json:"content_hash,omitempty" required:"false" doc:"Optional content hash for change detection"`
	}
}

// RefreshSourceResponse is the Huma response for POST /admin/sources/refresh.
type RefreshSourceResponse struct {
	Body SourceConfigInfo
}

// RefreshHandler handles POST /admin/sources/refresh — stamp a successful refresh.
func (h *SourceHandler) RefreshHandler(ctx context.Context, req *RefreshSourceRequest) (*RefreshSourceResponse, error) {
	out, err := h.mutate(ctx, req.Body.EntityType, req.Body.EntityID, func() error {
		return h.svc.RecordRefresh(req.Body.EntityType, req.Body.EntityID, req.Body.ContentHash)
	})
	if err != nil {
		return nil, err
	}
	return &RefreshSourceResponse{Body: toInfo(out)}, nil
}

// --- Record failure ---

// FailSourceRequest is the Huma request for POST /admin/sources/failure.
type FailSourceRequest struct {
	Body struct {
		EntityType string `json:"entity_type" enum:"venue,label" doc:"Entity kind"`
		EntityID   uint   `json:"entity_id" doc:"ID of the venue or label"`
	}
}

// FailSourceResponse is the Huma response for POST /admin/sources/failure.
type FailSourceResponse struct {
	Body SourceConfigInfo
}

// FailHandler handles POST /admin/sources/failure — record a failed refresh.
func (h *SourceHandler) FailHandler(ctx context.Context, req *FailSourceRequest) (*FailSourceResponse, error) {
	out, err := h.mutate(ctx, req.Body.EntityType, req.Body.EntityID, func() error {
		return h.svc.IncrementFailures(req.Body.EntityType, req.Body.EntityID)
	})
	if err != nil {
		return nil, err
	}
	return &FailSourceResponse{Body: toInfo(out)}, nil
}

// mutate guards a refresh/failure write: 404 if the entity has no registered
// source, runs the mutation, and returns the updated row.
func (h *SourceHandler) mutate(ctx context.Context, entityType string, entityID uint, fn func() error) (*adminm.SourceConfig, error) {
	existing, err := h.svc.GetByEntity(entityType, entityID)
	if err != nil {
		logger.FromContext(ctx).Error("source_lookup_failed", "entity_type", entityType, "entity_id", entityID, "error", err.Error())
		return nil, huma.Error500InternalServerError("Failed to load source config")
	}
	if existing == nil {
		return nil, huma.Error404NotFound("Source config not registered for this entity")
	}
	if err := fn(); err != nil {
		return nil, huma.Error422UnprocessableEntity(err.Error())
	}
	updated, err := h.svc.GetByEntity(entityType, entityID)
	if err != nil || updated == nil {
		return nil, huma.Error500InternalServerError("Failed to reload source config")
	}
	return updated, nil
}
