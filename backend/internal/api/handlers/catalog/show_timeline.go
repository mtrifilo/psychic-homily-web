package catalog

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/contracts"
)

// ============================================================================
// Show gig timeline
// ============================================================================

// GetShowTimelineRequest addresses a show by numeric id or slug.
//
// The parameter is named `show_id` for uniformity across the /shows sub-routes,
// which is what keeps TestShowSubRoutesShareOneParameterName meaningful.
type GetShowTimelineRequest struct {
	ShowID string `path:"show_id" doc:"Show ID or slug" example:"desert-doom-night"`
}

// GetShowTimelineResponse represents the show's gig timeline and bill recurrence.
type GetShowTimelineResponse struct {
	Body *contracts.ShowTimelineResponse
}

// GetShowTimelineHandler handles GET /shows/{show_id}/timeline — the headliner's
// adjacent dates and each billed act's recurrence in this show's place.
//
// Anonymous and public, so a show with nothing around it returns 200 with empty
// fields rather than an error: a page that exists must not be broken by a module
// that has nothing to say. Only an unknown show — or a non-approved one, which
// this surface must not distinguish from an unknown one — is a 404.
func (h *ShowHandler) GetShowTimelineHandler(ctx context.Context, req *GetShowTimelineRequest) (*GetShowTimelineResponse, error) {
	timeline, err := h.showService.GetShowTimeline(req.ShowID)
	if err != nil {
		if mapped := shared.MapShowError(err); mapped != nil {
			return nil, mapped
		}
		// Logged, not echoed. Huma serializes a wrapped error's text into the
		// response body, and this endpoint is anonymous and enumerable by show
		// id, so passing `err` would hand any caller raw Postgres error strings
		// during a migration or outage.
		logger.FromContext(ctx).Error("show_timeline_failed",
			"show_id_or_slug", req.ShowID,
			"error", err.Error(),
		)
		return nil, huma.Error500InternalServerError("Failed to get show timeline")
	}

	return &GetShowTimelineResponse{Body: timeline}, nil
}
