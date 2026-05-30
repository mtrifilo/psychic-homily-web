package catalog

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
)

type entityExistenceService interface {
	Exists(entityType, idOrSlug string) (bool, error)
}

type EntityExistenceHandler struct {
	entityExistenceService entityExistenceService
}

func NewEntityExistenceHandler(entityExistenceService entityExistenceService) *EntityExistenceHandler {
	return &EntityExistenceHandler{entityExistenceService: entityExistenceService}
}

type EntityExistsRequest struct {
	EntityType string `path:"entity_type" validate:"required" doc:"Entity prefix (shows, venues, artists, releases, labels, festivals, tags, scenes)"`
	EntityID   string `path:"entity_id" validate:"required" doc:"Entity ID or slug"`
}

type EntityExistsResponse struct{}

func (h *EntityExistenceHandler) EntityExistsHandler(ctx context.Context, req *EntityExistsRequest) (*EntityExistsResponse, error) {
	exists, err := h.entityExistenceService.Exists(req.EntityType, req.EntityID)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to check entity existence", err)
	}
	if !exists {
		return nil, huma.Error404NotFound("Entity not found")
	}
	return &EntityExistsResponse{}, nil
}
