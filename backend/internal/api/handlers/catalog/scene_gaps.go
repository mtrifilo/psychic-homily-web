package catalog

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/services/contracts"
)

// ============================================================================
// Get Scene Gaps (PSY-1845)
// ============================================================================

// GetSceneGapsRequest represents the request for a scene's completeness gaps.
type GetSceneGapsRequest struct {
	Slug string `path:"slug" doc:"Scene slug (e.g. phoenix-az)" example:"phoenix-az"`
}

// GetSceneGapsResponse represents the response for a scene's completeness gaps.
type GetSceneGapsResponse struct {
	Body *contracts.SceneGapsResponse
}

// GetSceneGapsHandler handles GET /scenes/{slug}/gaps — the counts behind the
// scene page's one-line "help finish this scene" prompt.
//
// A SEPARATE route rather than two more fields on GET /scenes/{slug}, despite
// the payload cost being two integers. Bytes are not the argument — these three
// are:
//
//   - Query cost on the blocking path. The detail read already fans out to a
//     dozen aggregate queries before the scene page can render; these add two
//     more, one of them a four-table join over the scene's ENTIRE show history
//     (every join column is indexed, so it is not pathological — but it is
//     unbounded in the scene's age, unlike the windowed counts beside it) to
//     serve one line of secondary copy.
//   - Cacheability. A backlog figure is fine minutes stale; tonight's show
//     counts are not. Separate routes can hold separate freshness, which one
//     merged payload cannot.
//   - Failure isolation. A degraded gap line hides one sentence. The same
//     failure inside the detail read takes down the whole scene page.
//
// A parseable slug that is not a scene 404s, matching GET /scenes/{slug} — see
// GetSceneGaps for why this route is gated where /new-artists is not.
//
// No HEAD sibling, unlike the week/day families: this is a data sub-resource
// the frontend fetches directly, never a reader-facing path that
// frontend/proxy.ts existence-checks before the page streams.
func (h *SceneHandler) GetSceneGapsHandler(ctx context.Context, req *GetSceneGapsRequest) (*GetSceneGapsResponse, error) {
	city, state, err := h.sceneService.ParseSceneSlug(req.Slug)
	if err != nil {
		return nil, mapSceneSlugError(err)
	}

	gaps, err := h.sceneService.GetSceneGaps(city, state)
	if err != nil {
		if mapped := shared.MapSceneError(err); mapped != nil {
			return nil, mapped
		}
		return nil, huma.Error500InternalServerError("Failed to get scene gaps", err)
	}

	return &GetSceneGapsResponse{Body: gaps}, nil
}
