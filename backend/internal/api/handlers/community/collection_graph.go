package community

import (
	"context"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/services/contracts"
)

// ============================================================================
// Get Collection Graph (PSY-366)
// ============================================================================

// GetCollectionGraphRequest is the request for GET /collections/{slug}/graph.
// `Types` is a comma-separated list (e.g. "shared_bills,similar"); empty means
// all allowed collection edge types. Huma forbids pointer query params, so the
// empty string is the explicit "no filter" sentinel.
type GetCollectionGraphRequest struct {
	Slug  string `path:"slug" doc:"Collection slug" example:"my-favorite-artists"`
	Types string `query:"types" doc:"Comma-separated relationship types (e.g. shared_bills,similar). Empty = all allowed types." example:"shared_bills,similar"`
}

// GetCollectionGraphResponse represents the response for the collection-scale graph.
type GetCollectionGraphResponse struct {
	Body *contracts.CollectionGraphResponse
}

// GetCollectionGraphHandler handles GET /collections/{slug}/graph — returns the
// typed-edge subgraph spanning the collection's artist items + their stored
// relationships. PSY-366. Anonymous viewers can read public collections;
// private collections require the viewer to be the creator.
func (h *CollectionHandler) GetCollectionGraphHandler(ctx context.Context, req *GetCollectionGraphRequest) (*GetCollectionGraphResponse, error) {
	var viewerID uint
	if user := middleware.GetUserFromContext(ctx); user != nil {
		viewerID = user.ID
	}

	graph, err := h.collectionService.GetCollectionGraph(req.Slug, viewerID, parseCollectionGraphTypesParam(req.Types))
	if err != nil {
		if mapped := shared.MapCollectionError(err); mapped != nil {
			return nil, mapped
		}
		return nil, huma.Error500InternalServerError("Failed to get collection graph", err)
	}

	return &GetCollectionGraphResponse{Body: graph}, nil
}

// parseCollectionGraphTypesParam splits the comma-separated `types` query param
// into a trimmed, non-empty slice. Service-side allowlist drops anything
// unknown. Mirrors parseTypesQueryParam in handlers/catalog/scene.go — kept
// local to this package to avoid an unrelated refactor that would move the
// helper to shared/.
func parseCollectionGraphTypesParam(raw string) []string {
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
