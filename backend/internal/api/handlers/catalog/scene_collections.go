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
	// ceiling. These are huma-NATIVE tags and are therefore live, unlike the
	// go-playground `validate:"..."` tags this repo treats as dead. This is
	// the ONLY upper bound in the stack: the service clamps a non-positive
	// limit up to a default but deliberately declares no ceiling of its own,
	// so a change here is not silently contradicted a layer down.
	Limit int `query:"limit" minimum:"1" maximum:"20" default:"5" doc:"Max collections to return" example:"5"`
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
// empty shelf.
//
// A parseable place that has not cleared the scene venue threshold 404s, which
// follows /shows, /artists and /graph. It deliberately does NOT follow the
// adjacent /new-artists rail, which has no venue gate and answers 200 with []
// for such a place — so the two rails on this page really do disagree, and that
// is the gate's doing, not an oversight here.
func (h *SceneHandler) GetSceneCollectionsHandler(ctx context.Context, req *GetSceneCollectionsRequest) (*GetSceneCollectionsResponse, error) {
	city, state, err := h.sceneService.ParseSceneSlug(req.Slug)
	if err != nil {
		return nil, mapSceneSlugError(err)
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
