package community

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/admin"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/contracts"
)

// ContributeHandler handles public contribution opportunity endpoints.
type ContributeHandler struct {
	dataQualityService contracts.DataQualityServiceInterface
}

// NewContributeHandler creates a new contribute handler.
func NewContributeHandler(
	dataQualityService contracts.DataQualityServiceInterface,
) *ContributeHandler {
	return &ContributeHandler{
		dataQualityService: dataQualityService,
	}
}

// --- GetOpportunities ---

// GetOpportunitiesRequest is the Huma request for GET /contribute/opportunities
type GetOpportunitiesRequest struct{}

// GetOpportunitiesResponse is the Huma response for GET /contribute/opportunities
type GetOpportunitiesResponse struct {
	Body struct {
		Categories []admin.DataQualityCategoryResponse `json:"categories"`
		TotalItems int                                 `json:"total_items"`
	}
}

// GetOpportunitiesHandler handles GET /contribute/opportunities
func (h *ContributeHandler) GetOpportunitiesHandler(ctx context.Context, _ *GetOpportunitiesRequest) (*GetOpportunitiesResponse, error) {
	summary, err := h.dataQualityService.GetSummary()
	if err != nil {
		logger.FromContext(ctx).Error("contribute_opportunities_failed",
			"error", err.Error(),
		)
		return nil, huma.Error500InternalServerError("Failed to get contribution opportunities")
	}

	resp := &GetOpportunitiesResponse{}
	resp.Body.TotalItems = summary.TotalItems
	categories := make([]admin.DataQualityCategoryResponse, 0, len(summary.Categories))
	for _, c := range summary.Categories {
		categories = append(categories, admin.DataQualityCategoryResponse{
			Key:         c.Key,
			Label:       c.Label,
			EntityType:  c.EntityType,
			Count:       c.Count,
			Description: c.Description,
		})
	}
	resp.Body.Categories = categories
	return resp, nil
}

// --- GetOpportunityCategory ---

// GetOpportunityCategoryRequest is the Huma request for GET /contribute/opportunities/{category}
type GetOpportunityCategoryRequest struct {
	Category string `path:"category" doc:"Contribution opportunity category key"`
	Limit    int    `query:"limit" required:"false" doc:"Max results (default 20, max 200)"`
	Offset   int    `query:"offset" required:"false" doc:"Offset for pagination"`
}

// GetOpportunityCategoryResponse is the Huma response for GET /contribute/opportunities/{category}
type GetOpportunityCategoryResponse struct {
	Body struct {
		Items []admin.DataQualityItemResponse `json:"items"`
		Total int64                           `json:"total"`
	}
}

// GetOpportunityCategoryHandler handles GET /contribute/opportunities/{category}
func (h *ContributeHandler) GetOpportunityCategoryHandler(ctx context.Context, req *GetOpportunityCategoryRequest) (*GetOpportunityCategoryResponse, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}

	items, total, err := h.dataQualityService.GetCategoryItems(req.Category, limit, req.Offset)
	if err != nil {
		// Check if it's an unknown category error
		if err.Error() == "unknown category: "+req.Category {
			return nil, huma.Error400BadRequest("Unknown contribution category: " + req.Category)
		}
		logger.FromContext(ctx).Error("contribute_category_failed",
			"category", req.Category,
			"error", err.Error(),
		)
		return nil, huma.Error500InternalServerError("Failed to get contribution items")
	}

	resp := &GetOpportunityCategoryResponse{}
	resp.Body.Total = total
	respItems := make([]admin.DataQualityItemResponse, 0, len(items))
	for _, item := range items {
		respItems = append(respItems, admin.DataQualityItemResponse{
			EntityType: item.EntityType,
			EntityID:   item.EntityID,
			Name:       item.Name,
			Slug:       item.Slug,
			Reason:     item.Reason,
			ShowCount:  item.ShowCount,
		})
	}
	resp.Body.Items = respItems
	return resp, nil
}
