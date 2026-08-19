package catalog

import (
	"context"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/logger"
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
		// RepresentativeEmbed is chosen over the FULL roster (active-first), so the
		// preview's player is independent of the returned page (PSY-1294). null when
		// no band based in the metro has a Bandcamp embed.
		RepresentativeEmbed *contracts.SceneRepresentativeEmbed `json:"representative_embed" doc:"The single band whose Bandcamp embed represents the scene — computed over the full metro roster, so it's independent of the returned page. Populated on the first page only (offset 0); null on later pages, and null when no band based here has an embed."`
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
	resp.Body.RepresentativeEmbed = h.representativeEmbed(ctx, city, state, period, req.Offset, artists, total)

	return resp, nil
}

// representativeEmbed picks the scene's instant-payoff embed for the /atlas
// preview (PSY-1294): the top embed-having band in the active-first roster,
// chosen over the FULL roster so it can't silently fall below the fetched
// window. The roster is active-first ordered, so on the first page the top
// embed-having band is almost always already in `page` — derive it there for
// free (no second query). Only when the page holds none but the roster is larger
// do we scan the full roster for one (the coverage case). The player is
// secondary payoff: a lookup failure logs and yields no player rather than
// failing the roster response — which the full scene page also depends on
// (mirrors the non-fatal secondary lookups in artist_graph_card.go). Returned
// only for the first page (offset 0) — it's a scene-level field, not per-page.
//
// This runs for EVERY consumer of GET /scenes/{slug}/artists, including the full
// scene page (SceneDetail), which ignores the field — accepted because the
// common case is free (the embed comes from `page`) and the full-roster fallback
// fires only when the first page holds no embed but the roster is larger (rare,
// and scene-page loads are infrequent). Gate behind an opt-in param if that
// fallback ever shows up as measurable load.
func (h *SceneHandler) representativeEmbed(ctx context.Context, city, state string, activeWindowDays, offset int, page []*contracts.SceneArtistResponse, total int64) *contracts.SceneRepresentativeEmbed {
	if offset != 0 {
		return nil
	}
	for _, a := range page {
		if a.BandcampEmbedURL != nil && *a.BandcampEmbedURL != "" {
			return &contracts.SceneRepresentativeEmbed{
				EmbedURL:   *a.BandcampEmbedURL,
				ArtistName: a.Name,
				ArtistSlug: a.Slug,
			}
		}
	}
	if int64(len(page)) >= total {
		// The page IS the whole roster — no embed-having band exists anywhere.
		return nil
	}
	embed, err := h.sceneService.GetRepresentativeEmbed(city, state, activeWindowDays)
	if err != nil {
		logger.FromContext(ctx).Warn("scene-artists: representative embed lookup failed",
			"city", city, "state", state, "error", err)
		return nil
	}
	return embed
}

// ============================================================================
// Get Scene Upcoming Shows (PSY-1309)
// ============================================================================

// GetSceneShowsRequest represents the request for a scene's next upcoming shows.
type GetSceneShowsRequest struct {
	Slug  string `path:"slug" doc:"Scene slug (e.g. phoenix-az)" example:"phoenix-az"`
	Days  int    `query:"days" default:"7" minimum:"1" maximum:"30" doc:"Window in days — shows with event_date inside [now, now+days)"`
	// The 20-row ceiling this replaces was sized for the Atlas preview's
	// three-row peek, and was raised to 200 for a four-week scene-page
	// calendar that NO LONGER EXISTS: the scene root now reads the day
	// endpoint for two nights, and the window pages compose week payloads
	// (PSY-1850). The only surviving caller is that same three-row preview.
	//
	// 200 is kept rather than walked back because this is a public ceiling on
	// a stable endpoint and lowering it is a breaking change for a caller we
	// cannot see; it stays calendar-scale and bounded, and the nightly page's
	// own cap is 100 for a SINGLE night. The default is unchanged.
	Limit int `query:"limit" default:"3" minimum:"1" maximum:"200" doc:"Maximum number of shows to return, soonest first"`
}

// GetSceneShowsResponse represents the response for a scene's upcoming shows.
type GetSceneShowsResponse struct {
	Body struct {
		Shows []contracts.SceneShowSummary `json:"shows" doc:"The scene's next approved shows in the window, soonest first — metro-scoped (member-city shows included)"`
	}
}

// GetSceneShowsHandler handles GET /scenes/{slug}/shows — the Atlas preview
// panel's "Next 7 days" row. Metro-scoped (unlike the literal-city
// upcoming-shows endpoint), so a Tempe show counts toward the Phoenix scene.
func (h *SceneHandler) GetSceneShowsHandler(ctx context.Context, req *GetSceneShowsRequest) (*GetSceneShowsResponse, error) {
	city, state, err := h.sceneService.ParseSceneSlug(req.Slug)
	if err != nil {
		return nil, huma.Error404NotFound("Scene not found")
	}

	days := req.Days
	if days == 0 {
		days = 7
	}
	limit := req.Limit
	if limit == 0 {
		limit = 3
	}

	shows, err := h.sceneService.GetSceneUpcomingShows(city, state, days, limit)
	if err != nil {
		if mapped := shared.MapSceneError(err); mapped != nil {
			return nil, mapped
		}
		return nil, huma.Error500InternalServerError("Failed to get scene shows", err)
	}
	if shows == nil {
		shows = []contracts.SceneShowSummary{}
	}

	resp := &GetSceneShowsResponse{}
	resp.Body.Shows = shows
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
	// ClusterBy selects the cluster signal (PSY-1262): "venue" (default,
	// each artist's most-played in-scene venue) or "community" (the nightly
	// Leiden similarity partition, labeled "Around {artist}"). Huma-native
	// enum tag — go-playground validate tags are dead in this repo.
	ClusterBy string `query:"cluster_by" enum:"venue,community" default:"venue" doc:"Cluster signal: venue (default) or community (similarity partition)." example:"community"`
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

	graph, err := h.sceneService.GetSceneGraph(city, state, parseTypesQueryParam(req.Types), req.ClusterBy)
	if err != nil {
		if mapped := shared.MapSceneError(err); mapped != nil {
			return nil, mapped
		}
		return nil, huma.Error500InternalServerError("Failed to get scene graph", err)
	}

	return &GetSceneGraphResponse{Body: graph}, nil
}

// ============================================================================
// Get Scene Week
// ============================================================================

// GetSceneWeekRequest addresses a SPECIFIC ISO week — the stable permalink.
//
// The current-week route needs its own request type (below) rather than
// reusing this one with an empty Week: huma treats every declared path param
// as required, so a shared struct makes /scenes/{slug}/week fail validation
// with 422 before the handler ever runs. Making Week a pointer is not an
// option either — huma panics on pointer path params.
type GetSceneWeekRequest struct {
	Slug string `path:"slug" doc:"Scene slug (e.g. phoenix-az)" example:"phoenix-az"`
	Week string `path:"week" doc:"ISO-8601 week key (e.g. 2026-W31)" example:"2026-W31"`
}

// GetSceneCurrentWeekRequest addresses the scene's CURRENT week.
//
// "Current" is resolved server-side, in the scene's own timezone rather than
// the viewer's, so a reader in Berlin and a reader in Chicago both get the
// same Chicago week.
type GetSceneCurrentWeekRequest struct {
	Slug string `path:"slug" doc:"Scene slug (e.g. phoenix-az)" example:"phoenix-az"`
}

// GetSceneWeekResponse represents the response for a scene's week.
type GetSceneWeekResponse struct {
	Body *contracts.SceneWeekResponse
}

// GetSceneWeekHandler handles GET /scenes/{slug}/week and
// /scenes/{slug}/week/{week} — one ISO week of a scene's shows grouped by day.
//
// A valid scene with no shows that week returns 200 with an empty week, not
// 404: a real city having a quiet week is a fact, and 404ing it would make an
// already-shared permalink break retroactively. Only an unknown/below-threshold
// scene or a malformed week key is a 404.
func (h *SceneHandler) GetSceneWeekHandler(ctx context.Context, req *GetSceneWeekRequest) (*GetSceneWeekResponse, error) {
	return h.sceneWeek(req.Slug, req.Week)
}

// GetSceneCurrentWeekHandler handles GET /scenes/{slug}/week — the scene's
// current week, resolved in the scene's own timezone.
func (h *SceneHandler) GetSceneCurrentWeekHandler(ctx context.Context, req *GetSceneCurrentWeekRequest) (*GetSceneWeekResponse, error) {
	return h.sceneWeek(req.Slug, "")
}

// sceneWeek is the shared body of both week routes. weekKey == "" means the
// scene's current week.
func (h *SceneHandler) sceneWeek(slug, weekKey string) (*GetSceneWeekResponse, error) {
	city, state, err := h.sceneService.ParseSceneSlug(slug)
	if err != nil {
		return nil, huma.Error404NotFound("Scene not found")
	}

	week, err := h.sceneService.GetSceneWeek(city, state, weekKey)
	if err != nil {
		if mapped := shared.MapSceneError(err); mapped != nil {
			return nil, mapped
		}
		return nil, huma.Error500InternalServerError("Failed to get scene week", err)
	}

	return &GetSceneWeekResponse{Body: week}, nil
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
