package catalog

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/contracts"
)

// GraphOverviewHandler serves the nightly precomputed "Map of the Scene".
type GraphOverviewHandler struct {
	graphOverviewService contracts.GraphOverviewServiceInterface
}

// NewGraphOverviewHandler creates a new graph overview handler.
func NewGraphOverviewHandler(svc contracts.GraphOverviewServiceInterface) *GraphOverviewHandler {
	return &GraphOverviewHandler{graphOverviewService: svc}
}

// graphOverviewCacheControl is the client-side caching policy for the map.
//
// The resource changes exactly ONCE A NIGHT, so an hour of client freshness
// costs at most an hour of staleness on a surface whose content is a day old by
// design, and it is what keeps a few hundred KB off the wire for every repeat
// visit. stale-while-revalidate lets a returning visitor render yesterday's map
// instantly while the new one loads in the background — the alternative on this
// surface is a blank zero-state, which is the thing the map exists to replace.
//
// A revalidation that does reach the server is answered by the ETag below, so
// the full payload only crosses the wire when the map actually changed.
const graphOverviewCacheControl = "public, max-age=3600, stale-while-revalidate=86400"

// GetGraphOverviewRequest is the Huma request for GET /graph/overview.
type GetGraphOverviewRequest struct {
	IfNoneMatch string `header:"If-None-Match" doc:"ETag from a previous response; a match is answered with 304"`
}

// GetGraphOverviewResponse is the Huma response for GET /graph/overview.
//
// Body is a POINTER so a 304 can carry no body at all. Huma skips serialization
// for 304 and 204 (transformAndWrite), which is what makes the revalidation
// path cost a header exchange instead of the payload.
type GetGraphOverviewResponse struct {
	Status       int
	ETag         string `header:"ETag"`
	CacheControl string `header:"Cache-Control"`
	Body         *contracts.GraphOverview
}

// GetGraphOverviewHandler handles GET /graph/overview.
func (h *GraphOverviewHandler) GetGraphOverviewHandler(ctx context.Context, req *GetGraphOverviewRequest) (*GetGraphOverviewResponse, error) {
	overview, etag, err := h.graphOverviewService.GetGraphOverview()
	if err != nil {
		logger.FromContext(ctx).Error("graph_overview_read_failed", "error", err.Error())
		return nil, huma.Error500InternalServerError("Failed to load the graph overview")
	}
	if overview == nil {
		// No snapshot has been built yet. 503 rather than an empty map: an empty
		// map is indistinguishable from "the scene is empty", and a client would
		// cache that answer.
		return nil, huma.Error503ServiceUnavailable("The graph overview has not been built yet")
	}

	resp := &GetGraphOverviewResponse{
		Status:       http.StatusOK,
		ETag:         etag,
		CacheControl: graphOverviewCacheControl,
	}
	if shared.IfNoneMatchCovers(req.IfNoneMatch, etag) {
		resp.Status = http.StatusNotModified
		return resp, nil
	}
	resp.Body = overview
	return resp, nil
}

// graphStartingPointsCacheControl is the caching policy for the suggestion list.
//
// The RANKING behind it changes once a night, like the map. The identity of
// each entry is resolved live, so an hour of client freshness is also the
// longest a renamed artist can keep being suggested — acceptable for a sentence
// whose click resolves the artist by id either way, and worth the round trips
// it saves on the surface that renders on every phone-width visit to /graph.
const graphStartingPointsCacheControl = "public, max-age=3600, stale-while-revalidate=86400"

// GetGraphStartingPointsRequest is the Huma request for
// GET /graph/starting-points. It takes nothing: the list is identical for every
// visitor, which is what lets it be cached at every layer.
type GetGraphStartingPointsRequest struct{}

// GetGraphStartingPointsResponse is the Huma response for
// GET /graph/starting-points.
type GetGraphStartingPointsResponse struct {
	CacheControl string `header:"Cache-Control"`
	Body         contracts.GraphStartingPointsResponse
}

// GetGraphStartingPointsHandler handles GET /graph/starting-points — the
// connectivity-ranked artists the /graph fallback hero suggests (PSY-1749).
//
// 200 WITH AN EMPTY LIST, never a 503. See
// contracts.GraphStartingPointsResponse: the client's answer to "nothing to
// suggest" is a random catalog artist, and that path is reached by an empty
// list, not by an error.
func (h *GraphOverviewHandler) GetGraphStartingPointsHandler(ctx context.Context, _ *GetGraphStartingPointsRequest) (*GetGraphStartingPointsResponse, error) {
	artists, err := h.graphOverviewService.GetGraphStartingPoints()
	if err != nil {
		logger.FromContext(ctx).Error("graph_starting_points_read_failed", "error", err.Error())
		return nil, huma.Error500InternalServerError("Failed to load graph starting points")
	}
	if artists == nil {
		artists = []contracts.GraphStartingPoint{}
	}

	return &GetGraphStartingPointsResponse{
		CacheControl: graphStartingPointsCacheControl,
		Body:         contracts.GraphStartingPointsResponse{Artists: artists},
	}, nil
}
