package handlers

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/services"
	"psychic-homily-backend/internal/services/catalog"
)

// SceneHandler handles scene (city aggregation) endpoints.
type SceneHandler struct {
	sceneService services.SceneServiceInterface
}

// NewSceneHandler creates a new SceneHandler.
func NewSceneHandler(sceneService services.SceneServiceInterface) *SceneHandler {
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
		Scenes []*services.SceneListResponse `json:"scenes" doc:"List of city scenes"`
		Count  int                           `json:"count" doc:"Number of scenes"`
	}
}

// ListScenesHandler handles GET /scenes — returns cities that qualify as scenes.
func (h *SceneHandler) ListScenesHandler(ctx context.Context, req *ListScenesRequest) (*ListScenesResponse, error) {
	scenes, err := h.sceneService.ListScenes()
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to list scenes", err)
	}

	if scenes == nil {
		scenes = []*services.SceneListResponse{}
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
	Body *services.SceneDetailResponse
}

// GetSceneDetailHandler handles GET /scenes/{slug} — returns full computed scene detail.
func (h *SceneHandler) GetSceneDetailHandler(ctx context.Context, req *GetSceneDetailRequest) (*GetSceneDetailResponse, error) {
	city, state, err := h.sceneService.ParseSceneSlug(req.Slug)
	if err != nil {
		return nil, huma.Error404NotFound("Scene not found")
	}

	detail, err := h.sceneService.GetSceneDetail(city, state)
	if err != nil {
		if isSceneNotFoundErr(err) {
			return nil, huma.Error404NotFound("Scene not found")
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
	Period int    `query:"period" default:"90" minimum:"7" maximum:"365" doc:"Period in days for activity window"`
	Limit  int    `query:"limit" default:"20" minimum:"1" maximum:"100" doc:"Maximum number of artists to return"`
	Offset int    `query:"offset" default:"0" minimum:"0" doc:"Offset for pagination"`
}

// GetSceneActiveArtistsResponse represents the response for active artists.
type GetSceneActiveArtistsResponse struct {
	Body struct {
		Artists []*services.SceneArtistResponse `json:"artists" doc:"List of active artists"`
		Total   int64                           `json:"total" doc:"Total number of active artists"`
	}
}

// GetSceneActiveArtistsHandler handles GET /scenes/{slug}/artists — returns artists ranked by show count.
func (h *SceneHandler) GetSceneActiveArtistsHandler(ctx context.Context, req *GetSceneActiveArtistsRequest) (*GetSceneActiveArtistsResponse, error) {
	city, state, err := h.sceneService.ParseSceneSlug(req.Slug)
	if err != nil {
		return nil, huma.Error404NotFound("Scene not found")
	}

	period := req.Period
	if period == 0 {
		period = 90
	}
	limit := req.Limit
	if limit == 0 {
		limit = 20
	}

	artists, total, err := h.sceneService.GetActiveArtists(city, state, period, limit, req.Offset)
	if err != nil {
		if isSceneNotFoundErr(err) {
			return nil, huma.Error404NotFound("Scene not found")
		}
		return nil, huma.Error500InternalServerError("Failed to get active artists", err)
	}

	if artists == nil {
		artists = []*services.SceneArtistResponse{}
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
	Body *services.SceneGenreResponse
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
		genres = []services.GenreCount{}
	}

	diversityIndex, err := h.sceneService.GetGenreDiversityIndex(city, state)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to get genre diversity index", err)
	}

	resp := &GetSceneGenresResponse{
		Body: &services.SceneGenreResponse{
			Genres:         genres,
			DiversityIndex: diversityIndex,
			DiversityLabel: catalog.DiversityLabel(diversityIndex),
		},
	}

	return resp, nil
}

// isSceneNotFoundErr checks if an error indicates a scene was not found.
func isSceneNotFoundErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return len(msg) >= 15 && msg[:15] == "scene not found"
}
