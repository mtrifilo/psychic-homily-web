package catalog

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/services/contracts"
)

// ============================================================================
// Get Scene Collections (PSY-1847)
// ============================================================================

// GetSceneCollectionsRequest is the request for the scene's collections rail.
type GetSceneCollectionsRequest struct {
	Slug string `path:"slug" doc:"Scene slug (e.g. phoenix-az)" example:"phoenix-az"`
	// The rail shows 3-5 cards, so 5 is the default and 20 is a generous
	// ceiling for any other caller. Bounds are declared here (huma-native
	// tags) rather than in the service: the service's own clamp is a backstop
	// for direct callers, not a competing policy.
	Limit int `query:"limit" required:"false" minimum:"1" maximum:"20" default:"5" doc:"Max collections to return" example:"5"`
}

// GetSceneCollectionsResponse is the response for the scene's collections rail.
type GetSceneCollectionsResponse struct {
	Body struct {
		Collections []contracts.SceneCollectionSummary `json:"collections" doc:"Public collections relevant to this scene, most scene-local members first"`
	}
}

// GetSceneCollectionsHandler handles GET /scenes/{slug}/collections — the public
// collections whose members are substantially based in this scene.
//
// An empty list is a normal answer, not an error: most scenes have no
// qualifying collection yet, and the rail hides itself rather than rendering an
// empty shelf. A slug that is not a scene 404s, matching the sibling rails.
func (h *SceneHandler) GetSceneCollectionsHandler(ctx context.Context, req *GetSceneCollectionsRequest) (*GetSceneCollectionsResponse, error) {
	city, state, err := h.sceneService.ParseSceneSlug(req.Slug)
	if err != nil {
		return nil, huma.Error404NotFound("Scene not found")
	}

	collections, err := h.sceneService.GetSceneCollections(city, state, req.Limit)
	if err != nil {
		if mapped := shared.MapSceneError(err); mapped != nil {
			return nil, mapped
		}
		return nil, huma.Error500InternalServerError("Failed to get scene collections", err)
	}
	if collections == nil {
		collections = []contracts.SceneCollectionSummary{}
	}

	resp := &GetSceneCollectionsResponse{}
	resp.Body.Collections = collections
	return resp, nil
}
