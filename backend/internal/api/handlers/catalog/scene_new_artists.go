package catalog

import (
	"context"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/services/contracts"
)

// ============================================================================
// Get Scene Latest Artists (PSY-1781, redefined by PSY-1844)
// ============================================================================

// GetSceneNewArtistsRequest represents the request for a scene's latest bands.
//
// The `days` parameter PSY-1781 shipped is gone rather than deprecated. It
// selected a trailing window that no longer exists, so no value for it could be
// honoured; leaving it accepted-and-ignored would let a caller believe it had
// narrowed the list. Unknown query parameters are ignored (huma reads only
// declared names), so an old caller still passing `?days=30` gets the latest
// bands rather than an error.
//
// Limit carries NO `default:` tag on purpose. A tag would restate the number
// the service already owns as sceneLatestArtistsDefaultLimit, and struct tags
// cannot reference a constant, so the two could only be held together by a
// comment — an invariant nothing checks. huma skips absent parameters before
// parsing them (an empty value returns early, it is not parsed as an int), so
// an omitted limit arrives here as 0 and the service applies the one default
// that exists. The number is documented below rather than duplicated.
type GetSceneNewArtistsRequest struct {
	Slug  string `path:"slug" doc:"Scene slug (e.g. phoenix-az)" example:"phoenix-az"`
	Limit int    `query:"limit" minimum:"1" maximum:"50" doc:"Maximum number of bands to return, most recently listed first. Omit for the server default (5)."`
}

// GetSceneNewArtistsResponse represents the response for a scene's latest bands.
//
// There is no `total`. PSY-1781 published one so the client could render
// "+N more" and the digest's cap could never silently drop a band its cursor
// then advanced past. With the window gone there is no withheld set to count:
// what sits beyond the cap is simply the rest of the roster, which the scene
// page already lists in full with its own count in `stats.artist_count`.
// Publishing a second number for the same set is how two surfaces start
// contradicting each other.
type GetSceneNewArtistsResponse struct {
	Body struct {
		// Artists is always non-nil, so a scene with no bands based in it
		// marshals as `[]` rather than `null` — an empty module is a state of a
		// scene that exists, not a missing scene.
		Artists []contracts.SceneNewArtistRow `json:"artists" doc:"Bands based in the scene, most recently listed first"`
	}
}

// GetSceneNewArtistsHandler handles GET /scenes/{slug}/new-artists — the scene
// page's latest-additions module, which replaced the Scene Pulse tile.
//
// "Latest" means MOST RECENTLY LISTED: the roster ordered by the date each
// band's catalog row was created, newest first, with no window. PSY-1781 served
// a trailing 30-day window here (the weekly digest's definition); PSY-1844
// measured that against production, found it empty on 5 of 6 major scenes
// because rosters grow in seeding batches rather than continuously, and removed
// the window. The rendered date still states the fact the ordering selected on,
// which is why this is not the pulse's first-approved-show definition.
//
// A slug that does not PARSE is a 404. A parseable slug with no bands is a 200
// with an empty list: the module hides itself client-side, and a 404 here would
// take the whole scene page down with it.
//
// Note this is deliberately MORE permissive than GET /scenes/{slug}: like the
// roster query it wraps, it applies no venue-count gate, so a place that parses
// but has not yet cleared the scene threshold answers 200 with `[]` here while
// the detail route answers 404.
func (h *SceneHandler) GetSceneNewArtistsHandler(ctx context.Context, req *GetSceneNewArtistsRequest) (*GetSceneNewArtistsResponse, error) {
	city, state, err := h.sceneService.ParseSceneSlug(req.Slug)
	if err != nil {
		return nil, mapSceneSlugError(err)
	}

	artists, err := h.sceneService.GetSceneLatestArtists(city, state, time.Now().UTC(), req.Limit)
	if err != nil {
		if mapped := shared.MapSceneError(err); mapped != nil {
			return nil, mapped
		}
		return nil, huma.Error500InternalServerError("Failed to get scene latest artists", err)
	}
	if artists == nil {
		artists = []contracts.SceneNewArtistRow{}
	}

	resp := &GetSceneNewArtistsResponse{}
	resp.Body.Artists = artists
	return resp, nil
}
