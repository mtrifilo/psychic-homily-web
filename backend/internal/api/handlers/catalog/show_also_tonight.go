package catalog

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/services/contracts"
)

// ============================================================================
// Show "also tonight" rail
// ============================================================================

// GetShowAlsoTonightRequest addresses a show by numeric id or slug.
//
// The parameter MUST stay named `show_id`: every /shows/{show_id}/… route shares
// one chi position, and a single registration naming it differently silently
// renames the whole group (pinned by show_calendar_routing_test.go).
type GetShowAlsoTonightRequest struct {
	ShowID string `path:"show_id" doc:"Show ID or slug" example:"desert-doom-night"`
}

// GetShowAlsoTonightResponse represents the show's "also tonight" rail.
type GetShowAlsoTonightResponse struct {
	Body *contracts.ShowAlsoTonightResponse
}

// GetShowAlsoTonightHandler handles GET /shows/{show_id}/also-tonight — the other
// shows in this show's metro on this show's own venue-local date.
//
// It hangs off SceneHandler rather than ShowHandler because the answer is a
// scene question ("what is on in this metro tonight") that merely takes a show
// as its address. Routing it here keeps one definition of a metro's night.
//
// A show with no scene to look at returns 200 with an empty rail, so a show page
// is never broken by a rail that has nothing to say. Only an unknown show — or a
// non-approved one, which this anonymous surface must not distinguish from an
// unknown one — is a 404.
func (h *SceneHandler) GetShowAlsoTonightHandler(ctx context.Context, req *GetShowAlsoTonightRequest) (*GetShowAlsoTonightResponse, error) {
	rail, err := h.sceneService.GetShowAlsoTonight(req.ShowID)
	if err != nil {
		if mapped := shared.MapShowError(err); mapped != nil {
			return nil, mapped
		}
		return nil, huma.Error500InternalServerError("Failed to get also-tonight shows", err)
	}

	return &GetShowAlsoTonightResponse{Body: rail}, nil
}
