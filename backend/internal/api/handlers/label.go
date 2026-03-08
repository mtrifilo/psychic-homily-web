package handlers

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/middleware"
	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services"
)

type LabelHandler struct {
	labelService    services.LabelServiceInterface
	auditLogService services.AuditLogServiceInterface
}

func NewLabelHandler(labelService services.LabelServiceInterface, auditLogService services.AuditLogServiceInterface) *LabelHandler {
	return &LabelHandler{
		labelService:    labelService,
		auditLogService: auditLogService,
	}
}

// ============================================================================
// List Labels
// ============================================================================

// ListLabelsRequest represents the request for listing labels
type ListLabelsRequest struct {
	Status string `query:"status" required:"false" doc:"Filter by status (active, inactive, defunct)" example:"active"`
	City   string `query:"city" required:"false" doc:"Filter by city" example:"Phoenix"`
	State  string `query:"state" required:"false" doc:"Filter by state" example:"AZ"`
}

// ListLabelsResponse represents the response for listing labels
type ListLabelsResponse struct {
	Body struct {
		Labels []*services.LabelListResponse `json:"labels" doc:"List of labels"`
		Count  int                           `json:"count" doc:"Number of labels"`
	}
}

// ListLabelsHandler handles GET /labels
func (h *LabelHandler) ListLabelsHandler(ctx context.Context, req *ListLabelsRequest) (*ListLabelsResponse, error) {
	filters := make(map[string]interface{})

	if req.Status != "" {
		filters["status"] = req.Status
	}
	if req.City != "" {
		filters["city"] = req.City
	}
	if req.State != "" {
		filters["state"] = req.State
	}

	labels, err := h.labelService.ListLabels(filters)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to fetch labels", err)
	}

	resp := &ListLabelsResponse{}
	resp.Body.Labels = labels
	resp.Body.Count = len(labels)

	return resp, nil
}

// ============================================================================
// Get Label
// ============================================================================

// GetLabelRequest represents the request for getting a single label
type GetLabelRequest struct {
	LabelID string `path:"label_id" doc:"Label ID or slug" example:"sub-pop"`
}

// GetLabelResponse represents the response for the get label endpoint
type GetLabelResponse struct {
	Body *services.LabelDetailResponse
}

// GetLabelHandler handles GET /labels/{label_id}
func (h *LabelHandler) GetLabelHandler(ctx context.Context, req *GetLabelRequest) (*GetLabelResponse, error) {
	var label *services.LabelDetailResponse
	var err error

	// Try to parse as numeric ID first
	if id, parseErr := strconv.ParseUint(req.LabelID, 10, 32); parseErr == nil {
		label, err = h.labelService.GetLabel(uint(id))
	} else {
		// Fall back to slug lookup
		label, err = h.labelService.GetLabelBySlug(req.LabelID)
	}

	if err != nil {
		var labelErr *apperrors.LabelError
		if errors.As(err, &labelErr) && labelErr.Code == apperrors.CodeLabelNotFound {
			return nil, huma.Error404NotFound("Label not found")
		}
		return nil, huma.Error500InternalServerError("Failed to fetch label", err)
	}

	return &GetLabelResponse{Body: label}, nil
}

// ============================================================================
// Create Label
// ============================================================================

// CreateLabelRequest represents the request for creating a label
type CreateLabelRequest struct {
	Body struct {
		Name        string  `json:"name" doc:"Label name" example:"Sub Pop"`
		City        *string `json:"city,omitempty" required:"false" doc:"City" example:"Seattle"`
		State       *string `json:"state,omitempty" required:"false" doc:"State" example:"WA"`
		Country     *string `json:"country,omitempty" required:"false" doc:"Country" example:"US"`
		FoundedYear *int    `json:"founded_year,omitempty" required:"false" doc:"Year founded" example:"1988"`
		Status      string  `json:"status,omitempty" required:"false" doc:"Status (active, inactive, defunct)" example:"active"`
		Description *string `json:"description,omitempty" required:"false" doc:"Description"`
		Instagram   *string `json:"instagram,omitempty" required:"false" doc:"Instagram handle"`
		Facebook    *string `json:"facebook,omitempty" required:"false" doc:"Facebook URL"`
		Twitter     *string `json:"twitter,omitempty" required:"false" doc:"Twitter handle"`
		YouTube     *string `json:"youtube,omitempty" required:"false" doc:"YouTube URL"`
		Spotify     *string `json:"spotify,omitempty" required:"false" doc:"Spotify URL"`
		SoundCloud  *string `json:"soundcloud,omitempty" required:"false" doc:"SoundCloud URL"`
		Bandcamp    *string `json:"bandcamp,omitempty" required:"false" doc:"Bandcamp URL"`
		Website     *string `json:"website,omitempty" required:"false" doc:"Website URL"`
	}
}

// CreateLabelResponse represents the response for creating a label
type CreateLabelResponse struct {
	Body *services.LabelDetailResponse
}

// CreateLabelHandler handles POST /labels
func (h *LabelHandler) CreateLabelHandler(ctx context.Context, req *CreateLabelRequest) (*CreateLabelResponse, error) {
	requestID := logger.GetRequestID(ctx)

	// Verify admin access
	user := middleware.GetUserFromContext(ctx)
	if user == nil || !user.IsAdmin {
		return nil, huma.Error403Forbidden("Admin access required")
	}

	if req.Body.Name == "" {
		return nil, huma.Error400BadRequest("Name is required")
	}

	serviceReq := &services.CreateLabelRequest{
		Name:        req.Body.Name,
		City:        req.Body.City,
		State:       req.Body.State,
		Country:     req.Body.Country,
		FoundedYear: req.Body.FoundedYear,
		Status:      req.Body.Status,
		Description: req.Body.Description,
		Instagram:   req.Body.Instagram,
		Facebook:    req.Body.Facebook,
		Twitter:     req.Body.Twitter,
		YouTube:     req.Body.YouTube,
		Spotify:     req.Body.Spotify,
		SoundCloud:  req.Body.SoundCloud,
		Bandcamp:    req.Body.Bandcamp,
		Website:     req.Body.Website,
	}

	label, err := h.labelService.CreateLabel(serviceReq)
	if err != nil {
		logger.FromContext(ctx).Error("create_label_failed",
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to create label (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		go func() {
			h.auditLogService.LogAction(user.ID, "create_label", "label", label.ID, nil)
		}()
	}

	logger.FromContext(ctx).Info("label_created",
		"label_id", label.ID,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	return &CreateLabelResponse{Body: label}, nil
}

// ============================================================================
// Update Label
// ============================================================================

// UpdateLabelRequest represents the request for updating a label
type UpdateLabelRequest struct {
	LabelID string `path:"label_id" doc:"Label ID or slug" example:"1"`
	Body    struct {
		Name        *string `json:"name,omitempty" required:"false" doc:"Label name"`
		City        *string `json:"city,omitempty" required:"false" doc:"City"`
		State       *string `json:"state,omitempty" required:"false" doc:"State"`
		Country     *string `json:"country,omitempty" required:"false" doc:"Country"`
		FoundedYear *int    `json:"founded_year,omitempty" required:"false" doc:"Year founded"`
		Status      *string `json:"status,omitempty" required:"false" doc:"Status (active, inactive, defunct)"`
		Description *string `json:"description,omitempty" required:"false" doc:"Description"`
		Instagram   *string `json:"instagram,omitempty" required:"false" doc:"Instagram handle"`
		Facebook    *string `json:"facebook,omitempty" required:"false" doc:"Facebook URL"`
		Twitter     *string `json:"twitter,omitempty" required:"false" doc:"Twitter handle"`
		YouTube     *string `json:"youtube,omitempty" required:"false" doc:"YouTube URL"`
		Spotify     *string `json:"spotify,omitempty" required:"false" doc:"Spotify URL"`
		SoundCloud  *string `json:"soundcloud,omitempty" required:"false" doc:"SoundCloud URL"`
		Bandcamp    *string `json:"bandcamp,omitempty" required:"false" doc:"Bandcamp URL"`
		Website     *string `json:"website,omitempty" required:"false" doc:"Website URL"`
	}
}

// UpdateLabelResponse represents the response for updating a label
type UpdateLabelResponse struct {
	Body *services.LabelDetailResponse
}

// UpdateLabelHandler handles PUT /labels/{label_id}
func (h *LabelHandler) UpdateLabelHandler(ctx context.Context, req *UpdateLabelRequest) (*UpdateLabelResponse, error) {
	requestID := logger.GetRequestID(ctx)

	// Verify admin access
	user := middleware.GetUserFromContext(ctx)
	if user == nil || !user.IsAdmin {
		return nil, huma.Error403Forbidden("Admin access required")
	}

	// Resolve label ID
	labelID, err := h.resolveLabelID(req.LabelID)
	if err != nil {
		return nil, err
	}

	serviceReq := &services.UpdateLabelRequest{
		Name:        req.Body.Name,
		City:        req.Body.City,
		State:       req.Body.State,
		Country:     req.Body.Country,
		FoundedYear: req.Body.FoundedYear,
		Status:      req.Body.Status,
		Description: req.Body.Description,
		Instagram:   req.Body.Instagram,
		Facebook:    req.Body.Facebook,
		Twitter:     req.Body.Twitter,
		YouTube:     req.Body.YouTube,
		Spotify:     req.Body.Spotify,
		SoundCloud:  req.Body.SoundCloud,
		Bandcamp:    req.Body.Bandcamp,
		Website:     req.Body.Website,
	}

	label, err := h.labelService.UpdateLabel(labelID, serviceReq)
	if err != nil {
		var labelErr *apperrors.LabelError
		if errors.As(err, &labelErr) && labelErr.Code == apperrors.CodeLabelNotFound {
			return nil, huma.Error404NotFound("Label not found")
		}
		logger.FromContext(ctx).Error("update_label_failed",
			"label_id", labelID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to update label (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		go func() {
			h.auditLogService.LogAction(user.ID, "edit_label", "label", labelID, nil)
		}()
	}

	logger.FromContext(ctx).Info("label_updated",
		"label_id", labelID,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	return &UpdateLabelResponse{Body: label}, nil
}

// ============================================================================
// Delete Label
// ============================================================================

// DeleteLabelRequest represents the request for deleting a label
type DeleteLabelRequest struct {
	LabelID string `path:"label_id" doc:"Label ID" example:"1"`
}

// DeleteLabelHandler handles DELETE /labels/{label_id}
func (h *LabelHandler) DeleteLabelHandler(ctx context.Context, req *DeleteLabelRequest) (*struct{}, error) {
	requestID := logger.GetRequestID(ctx)

	// Verify admin access
	user := middleware.GetUserFromContext(ctx)
	if user == nil || !user.IsAdmin {
		return nil, huma.Error403Forbidden("Admin access required")
	}

	// Resolve label ID
	labelID, err := h.resolveLabelID(req.LabelID)
	if err != nil {
		return nil, err
	}

	err = h.labelService.DeleteLabel(labelID)
	if err != nil {
		var labelErr *apperrors.LabelError
		if errors.As(err, &labelErr) && labelErr.Code == apperrors.CodeLabelNotFound {
			return nil, huma.Error404NotFound("Label not found")
		}
		logger.FromContext(ctx).Error("delete_label_failed",
			"label_id", labelID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to delete label (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		go func() {
			h.auditLogService.LogAction(user.ID, "delete_label", "label", labelID, nil)
		}()
	}

	logger.FromContext(ctx).Info("label_deleted",
		"label_id", labelID,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	return nil, nil
}

// ============================================================================
// Label Roster (Artists)
// ============================================================================

// GetLabelRosterRequest represents the request for getting a label's artists
type GetLabelRosterRequest struct {
	LabelID string `path:"label_id" doc:"Label ID or slug" example:"sub-pop"`
}

// GetLabelRosterResponse represents the response for the label roster endpoint
type GetLabelRosterResponse struct {
	Body struct {
		Artists []*services.LabelArtistResponse `json:"artists" doc:"List of artists"`
		Count   int                             `json:"count" doc:"Number of artists"`
	}
}

// GetLabelRosterHandler handles GET /labels/{label_id}/artists
func (h *LabelHandler) GetLabelRosterHandler(ctx context.Context, req *GetLabelRosterRequest) (*GetLabelRosterResponse, error) {
	// Resolve label ID
	labelID, err := h.resolveLabelID(req.LabelID)
	if err != nil {
		return nil, err
	}

	artists, err := h.labelService.GetLabelRoster(labelID)
	if err != nil {
		var labelErr *apperrors.LabelError
		if errors.As(err, &labelErr) && labelErr.Code == apperrors.CodeLabelNotFound {
			return nil, huma.Error404NotFound("Label not found")
		}
		return nil, huma.Error500InternalServerError("Failed to fetch label roster", err)
	}

	resp := &GetLabelRosterResponse{}
	resp.Body.Artists = artists
	resp.Body.Count = len(artists)

	return resp, nil
}

// ============================================================================
// Label Catalog (Releases)
// ============================================================================

// GetLabelCatalogRequest represents the request for getting a label's releases
type GetLabelCatalogRequest struct {
	LabelID string `path:"label_id" doc:"Label ID or slug" example:"sub-pop"`
}

// GetLabelCatalogResponse represents the response for the label catalog endpoint
type GetLabelCatalogResponse struct {
	Body struct {
		Releases []*services.LabelReleaseResponse `json:"releases" doc:"List of releases"`
		Count    int                               `json:"count" doc:"Number of releases"`
	}
}

// GetLabelCatalogHandler handles GET /labels/{label_id}/releases
func (h *LabelHandler) GetLabelCatalogHandler(ctx context.Context, req *GetLabelCatalogRequest) (*GetLabelCatalogResponse, error) {
	// Resolve label ID
	labelID, err := h.resolveLabelID(req.LabelID)
	if err != nil {
		return nil, err
	}

	releases, err := h.labelService.GetLabelCatalog(labelID)
	if err != nil {
		var labelErr *apperrors.LabelError
		if errors.As(err, &labelErr) && labelErr.Code == apperrors.CodeLabelNotFound {
			return nil, huma.Error404NotFound("Label not found")
		}
		return nil, huma.Error500InternalServerError("Failed to fetch label catalog", err)
	}

	resp := &GetLabelCatalogResponse{}
	resp.Body.Releases = releases
	resp.Body.Count = len(releases)

	return resp, nil
}

// ============================================================================
// Helpers
// ============================================================================

// resolveLabelID tries to parse the ID as a number first, then falls back to slug lookup
func (h *LabelHandler) resolveLabelID(idOrSlug string) (uint, error) {
	if id, parseErr := strconv.ParseUint(idOrSlug, 10, 32); parseErr == nil {
		return uint(id), nil
	}

	// Fall back to slug lookup
	label, err := h.labelService.GetLabelBySlug(idOrSlug)
	if err != nil {
		var labelErr *apperrors.LabelError
		if errors.As(err, &labelErr) && labelErr.Code == apperrors.CodeLabelNotFound {
			return 0, huma.Error404NotFound("Label not found")
		}
		return 0, huma.Error500InternalServerError("Failed to fetch label", err)
	}

	return label.ID, nil
}
