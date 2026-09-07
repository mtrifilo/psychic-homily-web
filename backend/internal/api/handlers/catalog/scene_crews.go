package catalog

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/services/contracts"
)

// ============================================================================
// Get Scene Crews (PSY-1884)
// ============================================================================

// GetSceneCrewsRequest is the request for the scene's crews chip row.
type GetSceneCrewsRequest struct {
	Slug string `path:"slug" doc:"Scene slug (e.g. phoenix-az)" example:"phoenix-az"`
}

// GetSceneCrewsResponse is the response for the scene's crews chip row.
type GetSceneCrewsResponse struct {
	Body struct {
		Crews []contracts.SceneCrewSummary `json:"crews" doc:"Crew tags booking in this scene, most scene shows first"`
	}
}

// GetSceneCrewsHandler handles GET /scenes/{slug}/crews — the music bookers
// (promoters, DIY crews, named series) whose tag sits on shows at this scene's
// venues.
//
// A separate route off the blocking path, no HEAD sibling, and the same
// existence gate as its neighbours. GetSceneGapsHandler argues that route shape
// at length and the argument applies here unchanged.
//
// An empty list is a normal answer, not an error: most scenes carry no crew tag,
// and that is a fact about the scene rather than a fault in the request.
func (h *SceneHandler) GetSceneCrewsHandler(ctx context.Context, req *GetSceneCrewsRequest) (*GetSceneCrewsResponse, error) {
	city, state, err := h.sceneService.ParseSceneSlug(req.Slug)
	if err != nil {
		return nil, huma.Error404NotFound("Scene not found")
	}

	crews, err := h.sceneService.GetSceneCrews(city, state)
	if err != nil {
		if mapped := shared.MapSceneError(err); mapped != nil {
			return nil, mapped
		}
		return nil, huma.Error500InternalServerError("Failed to get scene crews", err)
	}
	if crews == nil {
		crews = []contracts.SceneCrewSummary{}
	}

	resp := &GetSceneCrewsResponse{}
	resp.Body.Crews = crews
	return resp, nil
}
