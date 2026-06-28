package catalog

import (
	"context"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/services/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// SceneHandler handles scene (city aggregation) endpoints.
type SceneHandler struct {
	sceneService contracts.SceneServiceInterface
}

// NewSceneHandler creates a new SceneHandler.
func NewSceneHandler(sceneService contracts.SceneServiceInterface) *SceneHandler {
	return &SceneHandler{
		sceneService: sceneService,
	}
}

// ============================================================================
// List Scenes
// ============================================================================

// ListScenesRequest represents the request for listing scenes.
type ListScenesRequest struct{}

// ListScenesResponse represents the response for listing scenes.
type ListScenesResponse struct {
	Body struct {
		Scenes []*contracts.SceneListResponse `json:"scenes" doc:"List of city scenes"`
		Count  int                            `json:"count" doc:"Number of scenes"`
	}
}

// ListScenesHandler handles GET /scenes — returns the metros (and non-US / no-CBSA
// fallback cities) that qualify as scenes, each displayed under its principal city.
func (h *SceneHandler) ListScenesHandler(ctx context.Context, req *ListScenesRequest) (*ListScenesResponse, error) {
	scenes, err := h.sceneService.ListScenes()
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to list scenes", err)
	}

	if scenes == nil {
		scenes = []*contracts.SceneListResponse{}
	}

	resp := &ListScenesResponse{}
	resp.Body.Scenes = scenes
	resp.Body.Count = len(scenes)

	return resp, nil
}

// ============================================================================
// Get Scene Detail
// ============================================================================

// GetSceneDetailRequest represents the request for getting scene detail.
type GetSceneDetailRequest struct {
	Slug string `path:"slug" doc:"Scene slug (e.g. phoenix-az)" example:"phoenix-az"`
}

// GetSceneDetailResponse represents the response for scene detail.
type GetSceneDetailResponse struct {
	Body *contracts.SceneDetailResponse
}

// GetSceneDetailHandler handles GET /scenes/{slug} — returns full computed scene detail.
func (h *SceneHandler) GetSceneDetailHandler(ctx context.Context, req *GetSceneDetailRequest) (*GetSceneDetailResponse, error) {
	city, state, err := h.sceneService.ParseSceneSlug(req.Slug)
	if err != nil {
		return nil, huma.Error404NotFound("Scene not found")
	}

	detail, err := h.sceneService.GetSceneDetail(city, state)
	if err != nil {
		if mapped := shared.MapSceneError(err); mapped != nil {
			return nil, mapped
		}
		return nil, huma.Error500InternalServerError("Failed to get scene detail", err)
	}

	return &GetSceneDetailResponse{Body: detail}, nil
}

// ============================================================================
// Get Scene Active Artists
// ============================================================================

// GetSceneActiveArtistsRequest represents the request for getting active artists in a scene.
type GetSceneActiveArtistsRequest struct {
	Slug   string `path:"slug" doc:"Scene slug (e.g. phoenix-az)" example:"phoenix-az"`
	Period int    `query:"period" default:"180" minimum:"7" maximum:"365" doc:"Active window in days — a roster band is flagged active when it has a show within this window or upcoming (default ~6 months)"`
	Limit  int    `query:"limit" default:"20" minimum:"1" maximum:"100" doc:"Maximum number of artists to return"`
	Offset int    `query:"offset" default:"0" minimum:"0" doc:"Offset for pagination"`
}

// GetSceneActiveArtistsResponse represents the response for a scene's roster.
type GetSceneActiveArtistsResponse struct {
	Body struct {
		Artists []*contracts.SceneArtistResponse `json:"artists" doc:"The scene's roster — bands based in the metro, active ones (is_active) first"`
		Total   int64                            `json:"total" doc:"Total roster size (all bands based in the metro), NOT just the active subset"`
	}
}

// GetSceneActiveArtistsHandler handles GET /scenes/{slug}/artists — returns the scene's
// roster (bands based in the metro), active-first then by approved show count.
func (h *SceneHandler) GetSceneActiveArtistsHandler(ctx context.Context, req *GetSceneActiveArtistsRequest) (*GetSceneActiveArtistsResponse, error) {
	city, state, err := h.sceneService.ParseSceneSlug(req.Slug)
	if err != nil {
		return nil, huma.Error404NotFound("Scene not found")
	}

	period := req.Period
	if period == 0 {
		period = 180
	}
	limit := req.Limit
	if limit == 0 {
		limit = 20
	}

	artists, total, err := h.sceneService.GetActiveArtists(city, state, period, limit, req.Offset)
	if err != nil {
		if mapped := shared.MapSceneError(err); mapped != nil {
			return nil, mapped
		}
		return nil, huma.Error500InternalServerError("Failed to get active artists", err)
	}

	if artists == nil {
		artists = []*contracts.SceneArtistResponse{}
	}

	resp := &GetSceneActiveArtistsResponse{}
	resp.Body.Artists = artists
	resp.Body.Total = total

	return resp, nil
}

// ============================================================================
// Get Scene Genres
// ============================================================================

// GetSceneGenresRequest represents the request for getting scene genre distribution.
type GetSceneGenresRequest struct {
	Slug string `path:"slug" doc:"Scene slug (e.g. phoenix-az)" example:"phoenix-az"`
}

// GetSceneGenresResponse represents the response for scene genre distribution.
type GetSceneGenresResponse struct {
	Body *contracts.SceneGenreResponse
}

// GetSceneGenresHandler handles GET /scenes/{slug}/genres — returns genre distribution and diversity index.
func (h *SceneHandler) GetSceneGenresHandler(ctx context.Context, req *GetSceneGenresRequest) (*GetSceneGenresResponse, error) {
	city, state, err := h.sceneService.ParseSceneSlug(req.Slug)
	if err != nil {
		return nil, huma.Error404NotFound("Scene not found")
	}

	genres, err := h.sceneService.GetSceneGenreDistribution(city, state)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to get scene genre distribution", err)
	}
	if genres == nil {
		genres = []contracts.GenreCount{}
	}

	diversityIndex, err := h.sceneService.GetGenreDiversityIndex(city, state)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to get genre diversity index", err)
	}

	resp := &GetSceneGenresResponse{
		Body: &contracts.SceneGenreResponse{
			Genres:         genres,
			DiversityIndex: diversityIndex,
			DiversityLabel: catalog.DiversityLabel(diversityIndex),
		},
	}

	return resp, nil
}

// ============================================================================
// Get Scene Graph (PSY-367)
// ============================================================================

// GetSceneGraphRequest represents the request for the scene-scale graph.
// `Types` is a comma-separated list (e.g. "shared_bills,shared_label"); empty
// means all allowed scene edge types. Huma forbids pointer query params, so
// the empty string is the explicit "no filter" sentinel.
type GetSceneGraphRequest struct {
	Slug  string `path:"slug" doc:"Scene slug (e.g. phoenix-az)" example:"phoenix-az"`
	Types string `query:"types" doc:"Comma-separated relationship types (e.g. shared_bills,shared_label). Empty = all allowed types." example:"shared_bills,shared_label"`
}

// GetSceneGraphResponse represents the response for the scene-scale graph.
type GetSceneGraphResponse struct {
	Body *contracts.SceneGraphResponse
}

// GetSceneGraphHandler handles GET /scenes/{slug}/graph — returns the typed-edge
// scene-scale graph with computed venue-keyed clusters (per PSY-367 / spike PSY-368).
func (h *SceneHandler) GetSceneGraphHandler(ctx context.Context, req *GetSceneGraphRequest) (*GetSceneGraphResponse, error) {
	city, state, err := h.sceneService.ParseSceneSlug(req.Slug)
	if err != nil {
		return nil, huma.Error404NotFound("Scene not found")
	}

	graph, err := h.sceneService.GetSceneGraph(city, state, parseTypesQueryParam(req.Types))
	if err != nil {
		if mapped := shared.MapSceneError(err); mapped != nil {
			return nil, mapped
		}
		return nil, huma.Error500InternalServerError("Failed to get scene graph", err)
	}

	return &GetSceneGraphResponse{Body: graph}, nil
}

// parseTypesQueryParam splits the comma-separated `types` query param into a
// trimmed, non-empty slice. The service-side allowlist drops anything unknown.
func parseTypesQueryParam(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}
