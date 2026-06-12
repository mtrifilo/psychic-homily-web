package catalog

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/services/contracts"
)

// ============================================================================
// Get Festival Graph (PSY-1080)
// ============================================================================

// GetFestivalGraphRequest represents the request for the festival-scoped
// co-bill graph. `Types` mirrors the scene graph's filter: a comma-separated
// list; empty means all allowed festival edge types. Huma forbids pointer
// query params, so the empty string is the explicit "no filter" sentinel.
type GetFestivalGraphRequest struct {
	FestivalID string `path:"festival_id" doc:"Festival ID or slug" example:"m3f-2026"`
	Types      string `query:"types" required:"false" doc:"Comma-separated relationship types (festival_cobill,shared_bills,shared_label,similar,radio_cooccurrence). Empty = all allowed types." example:"festival_cobill,shared_bills"`
}

// GetFestivalGraphResponse represents the response for the festival graph.
type GetFestivalGraphResponse struct {
	Body *contracts.FestivalGraphResponse
}

// GetFestivalGraphHandler handles GET /festivals/{festival_id}/graph — returns
// the co-bill subgraph of the festival's lineup with billing-tier clusters
// (PSY-1080). Shape mirrors GET /scenes/{slug}/graph so ForceGraphView
// consumes it unchanged.
func (h *FestivalHandler) GetFestivalGraphHandler(ctx context.Context, req *GetFestivalGraphRequest) (*GetFestivalGraphResponse, error) {
	festivalID, err := h.resolveFestivalID(req.FestivalID)
	if err != nil {
		return nil, err
	}

	graph, err := h.festivalService.GetFestivalGraph(festivalID, parseTypesQueryParam(req.Types))
	if err != nil {
		if mapped := shared.MapFestivalError(err); mapped != nil {
			return nil, mapped
		}
		return nil, huma.Error500InternalServerError("Failed to get festival graph", err)
	}

	return &GetFestivalGraphResponse{Body: graph}, nil
}
