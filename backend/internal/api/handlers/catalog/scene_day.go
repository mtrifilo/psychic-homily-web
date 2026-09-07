package catalog

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/services/contracts"
)

// ============================================================================
// Get Scene Day
// ============================================================================

// GetSceneDayRequest addresses a SPECIFIC calendar date — the stable permalink,
// and the canonical URL for both day routes.
//
// The current-night route needs its own request type (below) rather than
// reusing this one with an empty Date: huma treats every declared path param as
// required, so a shared struct makes /scenes/{slug}/day fail validation with 422
// before the handler ever runs. Making Date a pointer is not an option either —
// huma panics on pointer path params. Same split, same reason, as the two week
// request types above.
type GetSceneDayRequest struct {
	Slug string `path:"slug" doc:"Scene slug (e.g. phoenix-az)" example:"phoenix-az"`
	Date string `path:"date" doc:"ISO calendar date (e.g. 2026-07-31)" example:"2026-07-31"`
}

// GetSceneCurrentDayRequest addresses the scene's current NIGHT.
//
// Resolved server-side, in the scene's own timezone and against the scene's own
// 6am night boundary, so a reader in Berlin and a reader in Phoenix asking about
// Phoenix get the same night — including at 01:00, when "tonight" is still the
// previous calendar date.
type GetSceneCurrentDayRequest struct {
	Slug string `path:"slug" doc:"Scene slug (e.g. phoenix-az)" example:"phoenix-az"`
}

// GetSceneDayResponse represents the response for one of a scene's days.
type GetSceneDayResponse struct {
	Body *contracts.SceneDayResponse
}

// GetSceneDayHandler handles GET /scenes/{slug}/day/{date} — one calendar day of
// a scene's shows.
//
// A valid scene with no shows that day returns 200 with an empty day, not 404: a
// real city having a quiet Tuesday is a fact, and 404ing it would break an
// already-shared permalink retroactively. Only an unknown/below-threshold scene
// or an impossible date is a 404.
func (h *SceneHandler) GetSceneDayHandler(ctx context.Context, req *GetSceneDayRequest) (*GetSceneDayResponse, error) {
	return h.sceneDay(req.Slug, req.Date)
}

// GetSceneCurrentDayHandler handles GET /scenes/{slug}/day — the scene's current
// night, resolved in the scene's own timezone.
func (h *SceneHandler) GetSceneCurrentDayHandler(ctx context.Context, req *GetSceneCurrentDayRequest) (*GetSceneDayResponse, error) {
	return h.sceneDay(req.Slug, "")
}

// sceneDay is the shared body of both day routes. dateKey == "" means the
// scene's current night.
func (h *SceneHandler) sceneDay(slug, dateKey string) (*GetSceneDayResponse, error) {
	city, state, err := h.sceneService.ParseSceneSlug(slug)
	if err != nil {
		return nil, mapSceneSlugError(err)
	}

	day, err := h.sceneService.GetSceneDay(city, state, dateKey)
	if err != nil {
		if mapped := shared.MapSceneError(err); mapped != nil {
			return nil, mapped
		}
		return nil, huma.Error500InternalServerError("Failed to get scene day", err)
	}

	return &GetSceneDayResponse{Body: day}, nil
}
