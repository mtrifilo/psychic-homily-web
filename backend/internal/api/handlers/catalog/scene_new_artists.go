package catalog

import (
	"context"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/services/contracts"
)

// ============================================================================
// Get Scene New Artists (PSY-1781)
// ============================================================================

// GetSceneNewArtistsRequest represents the request for a scene's new bands.
type GetSceneNewArtistsRequest struct {
	Slug  string `path:"slug" doc:"Scene slug (e.g. phoenix-az)" example:"phoenix-az"`
	Days  int    `query:"days" default:"30" minimum:"1" maximum:"365" doc:"Window in days — bands first listed inside [now-days, now]"`
	Limit int    `query:"limit" default:"10" minimum:"1" maximum:"50" doc:"Maximum number of bands to return, most recently listed first"`
}

// GetSceneNewArtistsResponse represents the response for a scene's new bands.
type GetSceneNewArtistsResponse struct {
	Body struct {
		// Artists is always non-nil, so a scene with no new bands marshals as
		// `[]` rather than `null` — an empty module is a state of a scene that
		// exists, not a missing scene.
		Artists []contracts.SceneNewArtistRow `json:"artists" doc:"Bands based in the scene first listed inside the window, most recently listed first"`
		// Total is the UNCAPPED count in the window. Clients render "+N more"
		// from Total - len(Artists), the same affordance the weekly digest uses,
		// so the cap never silently hides bands.
		Total int `json:"total" doc:"Total bands first listed in the window, before the limit is applied"`
	}
}

// GetSceneNewArtistsHandler handles GET /scenes/{slug}/new-artists — the scene
// page's named new-bands module, which replaced the Scene Pulse tile.
//
// "New" means FIRST LISTED: the band's catalog row was created inside the
// window. That is the weekly digest's definition, deliberately chosen over the
// pulse's first-approved-show-in-30-days so the rendered date ("first listed
// Aug 10") states the same fact the window selected on.
//
// An unknown scene slug is a 404. A KNOWN scene with no new bands is a 200 with
// an empty list: the module hides itself client-side, and a 404 here would take
// the whole scene page down with it.
func (h *SceneHandler) GetSceneNewArtistsHandler(ctx context.Context, req *GetSceneNewArtistsRequest) (*GetSceneNewArtistsResponse, error) {
	city, state, err := h.sceneService.ParseSceneSlug(req.Slug)
	if err != nil {
		return nil, huma.Error404NotFound("Scene not found")
	}

	days := req.Days
	if days == 0 {
		days = 30
	}
	limit := req.Limit
	if limit == 0 {
		limit = 10
	}

	now := time.Now().UTC()
	artists, total, err := h.sceneService.GetSceneNewArtists(city, state, now.AddDate(0, 0, -days), now, limit)
	if err != nil {
		if mapped := shared.MapSceneError(err); mapped != nil {
			return nil, mapped
		}
		return nil, huma.Error500InternalServerError("Failed to get scene new artists", err)
	}
	if artists == nil {
		artists = []contracts.SceneNewArtistRow{}
	}

	resp := &GetSceneNewArtistsResponse{}
	resp.Body.Artists = artists
	resp.Body.Total = total
	return resp, nil
}
