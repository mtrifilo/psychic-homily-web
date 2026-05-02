package admin

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/contracts"
)

// DataQualityHandler handles data quality dashboard endpoints.
type DataQualityHandler struct {
	dataQualityService contracts.DataQualityServiceInterface
}

// NewDataQualityHandler creates a new data quality handler.
func NewDataQualityHandler(
	dataQualityService contracts.DataQualityServiceInterface,
) *DataQualityHandler {
	return &DataQualityHandler{
		dataQualityService: dataQualityService,
	}
}

// --- GetDataQualitySummary ---

// GetDataQualitySummaryRequest is the Huma request for GET /admin/data-quality
type GetDataQualitySummaryRequest struct{}

// GetDataQualitySummaryResponse is the Huma response for GET /admin/data-quality
type GetDataQualitySummaryResponse struct {
	Body struct {
		Categories []DataQualityCategoryResponse `json:"categories"`
		TotalItems int                           `json:"total_items"`
	}
}

// DataQualityCategoryResponse is the response format for a data quality category.
type DataQualityCategoryResponse struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	EntityType  string `json:"entity_type"`
	Count       int    `json:"count"`
	Description string `json:"description"`
}

// GetDataQualitySummaryHandler handles GET /admin/data-quality
func (h *DataQualityHandler) GetDataQualitySummaryHandler(ctx context.Context, _ *GetDataQualitySummaryRequest) (*GetDataQualitySummaryResponse, error) {
	_, err := shared.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	summary, err := h.dataQualityService.GetSummary()
	if err != nil {
		logger.FromContext(ctx).Error("data_quality_summary_failed",
			"error", err.Error(),
		)
		return nil, huma.Error500InternalServerError("Failed to get data quality summary")
	}

	resp := &GetDataQualitySummaryResponse{}
	resp.Body.TotalItems = summary.TotalItems
	categories := make([]DataQualityCategoryResponse, 0, len(summary.Categories))
	for _, c := range summary.Categories {
		categories = append(categories, DataQualityCategoryResponse{
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

// --- GetDataQualityCategory ---

// GetDataQualityCategoryRequest is the Huma request for GET /admin/data-quality/{category}
type GetDataQualityCategoryRequest struct {
	Category string `path:"category" doc:"Data quality category key"`
	Limit    int    `query:"limit" required:"false" doc:"Max results (default 50, max 200)"`
	Offset   int    `query:"offset" required:"false" doc:"Offset for pagination"`
}

// DataQualityItemResponse is the response format for a data quality item.
type DataQualityItemResponse struct {
	EntityType string `json:"entity_type"`
	EntityID   uint   `json:"entity_id"`
	Name       string `json:"name"`
	Slug       string `json:"slug"`
	Reason     string `json:"reason"`
	ShowCount  int    `json:"show_count"`
}

// GetDataQualityCategoryResponse is the Huma response for GET /admin/data-quality/{category}
type GetDataQualityCategoryResponse struct {
	Body struct {
		Items []DataQualityItemResponse `json:"items"`
		Total int64                     `json:"total"`
	}
}

// GetDataQualityCategoryHandler handles GET /admin/data-quality/{category}
func (h *DataQualityHandler) GetDataQualityCategoryHandler(ctx context.Context, req *GetDataQualityCategoryRequest) (*GetDataQualityCategoryResponse, error) {
	_, err := shared.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}

	items, total, err := h.dataQualityService.GetCategoryItems(req.Category, limit, req.Offset)
	if err != nil {
		// Check if it's an unknown category error
		if err.Error() == "unknown category: "+req.Category {
			return nil, huma.Error400BadRequest("Unknown data quality category: " + req.Category)
		}
		logger.FromContext(ctx).Error("data_quality_category_failed",
			"category", req.Category,
			"error", err.Error(),
		)
		return nil, huma.Error500InternalServerError("Failed to get data quality items")
	}

	resp := &GetDataQualityCategoryResponse{}
	resp.Body.Total = total
	respItems := make([]DataQualityItemResponse, 0, len(items))
	for _, item := range items {
		respItems = append(respItems, DataQualityItemResponse{
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
