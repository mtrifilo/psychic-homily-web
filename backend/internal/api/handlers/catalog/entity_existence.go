package catalog

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
)

type entityExistenceService interface {
	Exists(entityType, idOrSlug string) (bool, error)
	CurrentSlugForFormerSlug(entityType, idOrSlug string) (string, error)
	ServedSceneSlug(slug string) (string, bool, error)
	RadioStationExists(stationIDOrSlug string) (bool, error)
	RadioShowExists(stationIDOrSlug, showIDOrSlug string) (bool, error)
	RadioEpisodeExists(stationIDOrSlug, showIDOrSlug, airDate string) (bool, error)
	CollectionExists(collectionIDOrSlug string) (bool, error)
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

// EntityExistsResponse is the generic probe's whole answer: the status, and
// for a scene or a former slug the slug the page is served at.
type EntityExistsResponse struct {
	CanonicalSlug string `header:"Canonical-Slug" doc:"Scenes: the slug the scene's page is served at, which a member city or a case variant of the requested slug resolves to; absent for a scene whose canonical slug cannot be named. Shows, venues and artists: present only when the requested slug is a former slug, naming the entity's current slug; absent when the requested slug or id is live. Absent for every other entity type."`
}

// FamilyExistsResponse is a family probe's whole answer: the status alone.
type FamilyExistsResponse struct{}

func (h *EntityExistenceHandler) EntityExistsHandler(ctx context.Context, req *EntityExistsRequest) (*EntityExistsResponse, error) {
	if req.EntityType == "scenes" {
		canonical, exists, err := h.entityExistenceService.ServedSceneSlug(req.EntityID)
		if err := existenceError(exists, err); err != nil {
			return nil, err
		}
		return &EntityExistsResponse{CanonicalSlug: canonical}, nil
	}
	exists, err := h.entityExistenceService.Exists(req.EntityType, req.EntityID)
	if err != nil {
		return nil, existenceError(false, err)
	}
	if exists {
		return &EntityExistsResponse{}, nil
	}
	// Only a miss reads the former slugs, so a live slug costs the one lookup
	// it always has.
	current, err := h.entityExistenceService.CurrentSlugForFormerSlug(req.EntityType, req.EntityID)
	if err := existenceError(current != "", err); err != nil {
		return nil, err
	}
	return &EntityExistsResponse{CanonicalSlug: current}, nil
}

// existenceError turns a probe's verdict into the probe's error: none when the
// entity exists (an empty 200), the API's 404 when it does not, and a 500 when
// the probe could not decide.
func existenceError(exists bool, err error) error {
	if err != nil {
		return huma.Error500InternalServerError("Failed to check entity existence", err)
	}
	if !exists {
		return huma.Error404NotFound("Entity not found")
	}
	return nil
}

// familyExistenceAnswer is a family probe's answer for a verdict.
func familyExistenceAnswer(exists bool, err error) (*FamilyExistsResponse, error) {
	if err := existenceError(exists, err); err != nil {
		return nil, err
	}
	return &FamilyExistsResponse{}, nil
}

// RadioStationExistsRequest names a radio station.
type RadioStationExistsRequest struct {
	Slug string `path:"slug" doc:"Radio station slug or numeric ID" example:"kexp"`
}

// RadioStationExistsHandler handles HEAD /radio-stations/{slug}/exists.
func (h *EntityExistenceHandler) RadioStationExistsHandler(ctx context.Context, req *RadioStationExistsRequest) (*FamilyExistsResponse, error) {
	return familyExistenceAnswer(h.entityExistenceService.RadioStationExists(req.Slug))
}

// RadioShowExistsRequest names a radio show under the station that carries it.
type RadioShowExistsRequest struct {
	Slug     string `path:"slug" doc:"Radio station slug or numeric ID" example:"kexp"`
	ShowSlug string `path:"show_slug" doc:"Radio show slug or numeric ID" example:"the-morning-show"`
}

// RadioShowExistsHandler handles HEAD /radio-stations/{slug}/radio-shows/{show_slug}/exists.
func (h *EntityExistenceHandler) RadioShowExistsHandler(ctx context.Context, req *RadioShowExistsRequest) (*FamilyExistsResponse, error) {
	return familyExistenceAnswer(h.entityExistenceService.RadioShowExists(req.Slug, req.ShowSlug))
}

// RadioEpisodeExistsRequest names one day's episode of a radio show under the
// station that carries it.
type RadioEpisodeExistsRequest struct {
	Slug     string `path:"slug" doc:"Radio station slug or numeric ID" example:"kexp"`
	ShowSlug string `path:"show_slug" doc:"Radio show slug or numeric ID" example:"the-morning-show"`
	Date     string `path:"date" doc:"Air date (YYYY-MM-DD)" example:"2025-01-15"`
}

// RadioEpisodeExistsHandler handles
// HEAD /radio-stations/{slug}/radio-shows/{show_slug}/episodes/{date}/exists.
func (h *EntityExistenceHandler) RadioEpisodeExistsHandler(ctx context.Context, req *RadioEpisodeExistsRequest) (*FamilyExistsResponse, error) {
	return familyExistenceAnswer(h.entityExistenceService.RadioEpisodeExists(req.Slug, req.ShowSlug, req.Date))
}

// CollectionExistsRequest names a collection.
type CollectionExistsRequest struct {
	Slug string `path:"slug" doc:"Collection slug or numeric ID" example:"my-favorite-artists"`
}

// CollectionExistsHandler handles HEAD /collections/{slug}/exists, with the
// answer CollectionExists gives.
func (h *EntityExistenceHandler) CollectionExistsHandler(ctx context.Context, req *CollectionExistsRequest) (*FamilyExistsResponse, error) {
	return familyExistenceAnswer(h.entityExistenceService.CollectionExists(req.Slug))
}
