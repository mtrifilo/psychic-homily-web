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
// A SEPARATE route rather than more fields on GET /scenes/{slug}, matching the
// gaps and collections rails: this is a five-table join over the scene's entire
// show history serving one secondary row, a backlog figure that is fine minutes
// stale next to counts that are not, and a failure here costs one chip row
// rather than the whole scene page.
//
// An empty list is a normal answer, not an error: most scenes have no crew tag
// yet, and the row hides itself rather than rendering an empty shelf.
//
// A parseable place that has not cleared the scene venue threshold 404s, which
// follows /gaps, /collections, /shows and /graph rather than the permissive
// /new-artists rail on the same page.
//
// No HEAD sibling, unlike the week/day families: this is a data sub-resource
// the frontend fetches directly, never a reader-facing path that
// frontend/proxy.ts existence-checks before the page streams.
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
