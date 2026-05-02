package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/notification"
)

// NotificationFilterHandler handles notification filter HTTP requests.
type NotificationFilterHandler struct {
	filterService contracts.NotificationFilterServiceInterface
	jwtSecret     string
}

// NewNotificationFilterHandler creates a new notification filter handler.
func NewNotificationFilterHandler(
	filterService contracts.NotificationFilterServiceInterface,
	jwtSecret string,
) *NotificationFilterHandler {
	return &NotificationFilterHandler{
		filterService: filterService,
		jwtSecret:     jwtSecret,
	}
}

// ──────────────────────────────────────────────
// Request / Response types
// ──────────────────────────────────────────────

// ListFiltersRequest is the request for GET /me/notification-filters
type ListFiltersRequest struct{}

// ListFiltersResponse is the response for GET /me/notification-filters
type ListFiltersResponse struct {
	Body struct {
		Filters []contracts.NotificationFilterResponse `json:"filters"`
	}
}

// CreateFilterRequest is the request for POST /me/notification-filters
type CreateFilterRequest struct {
	Body struct {
		Name          string           `json:"name" doc:"Filter name" minLength:"1" maxLength:"128"`
		ArtistIDs     []int64          `json:"artist_ids,omitempty" required:"false" doc:"Artist IDs to match"`
		VenueIDs      []int64          `json:"venue_ids,omitempty" required:"false" doc:"Venue IDs to match"`
		LabelIDs      []int64          `json:"label_ids,omitempty" required:"false" doc:"Label IDs to match"`
		TagIDs        []int64          `json:"tag_ids,omitempty" required:"false" doc:"Tag IDs to match"`
		ExcludeTagIDs []int64          `json:"exclude_tag_ids,omitempty" required:"false" doc:"Tag IDs to exclude"`
		Cities        *json.RawMessage `json:"cities,omitempty" required:"false" doc:"Cities to match [{city, state}]"`
		PriceMaxCents *int             `json:"price_max_cents,omitempty" required:"false" doc:"Max price in cents (0 = free only)"`
		NotifyEmail   *bool            `json:"notify_email,omitempty" required:"false" doc:"Send email notifications"`
		NotifyInApp   *bool            `json:"notify_in_app,omitempty" required:"false" doc:"Send in-app notifications"`
	}
}

// CreateFilterResponse is the response for POST /me/notification-filters
type CreateFilterResponse struct {
	Body contracts.NotificationFilterResponse
}

// UpdateFilterRequest is the request for PATCH /me/notification-filters/{id}
type UpdateFilterRequest struct {
	ID string `path:"id" doc:"Filter ID"`
	Body struct {
		Name          *string          `json:"name,omitempty" required:"false" doc:"Filter name"`
		IsActive      *bool            `json:"is_active,omitempty" required:"false" doc:"Enable/disable filter"`
		ArtistIDs     *[]int64         `json:"artist_ids,omitempty" required:"false" doc:"Artist IDs to match"`
		VenueIDs      *[]int64         `json:"venue_ids,omitempty" required:"false" doc:"Venue IDs to match"`
		LabelIDs      *[]int64         `json:"label_ids,omitempty" required:"false" doc:"Label IDs to match"`
		TagIDs        *[]int64         `json:"tag_ids,omitempty" required:"false" doc:"Tag IDs to match"`
		ExcludeTagIDs *[]int64         `json:"exclude_tag_ids,omitempty" required:"false" doc:"Tag IDs to exclude"`
		Cities        *json.RawMessage `json:"cities,omitempty" required:"false" doc:"Cities to match [{city, state}]"`
		PriceMaxCents *int             `json:"price_max_cents,omitempty" required:"false" doc:"Max price in cents"`
		NotifyEmail   *bool            `json:"notify_email,omitempty" required:"false" doc:"Send email notifications"`
		NotifyInApp   *bool            `json:"notify_in_app,omitempty" required:"false" doc:"Send in-app notifications"`
	}
}

// UpdateFilterResponse is the response for PATCH /me/notification-filters/{id}
type UpdateFilterResponse struct {
	Body contracts.NotificationFilterResponse
}

// DeleteFilterRequest is the request for DELETE /me/notification-filters/{id}
type DeleteFilterRequest struct {
	ID string `path:"id" doc:"Filter ID"`
}

// DeleteFilterResponse is the response for DELETE /me/notification-filters/{id}
type DeleteFilterResponse struct {
	Body struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
}

// QuickCreateFilterRequest is the request for POST /me/notification-filters/quick
type QuickCreateFilterRequest struct {
	Body struct {
		EntityType string `json:"entity_type" doc:"Entity type: artist, venue, label, or tag"`
		EntityID   int    `json:"entity_id" doc:"Entity ID"`
	}
}

// QuickCreateFilterResponse is the response for POST /me/notification-filters/quick
type QuickCreateFilterResponse struct {
	Body contracts.NotificationFilterResponse
}

// GetNotificationsRequest is the request for GET /me/notifications
type GetNotificationsRequest struct {
	Limit  int `query:"limit" default:"20" doc:"Number of notifications per page"`
	Offset int `query:"offset" default:"0" doc:"Offset for pagination"`
}

// GetNotificationsResponse is the response for GET /me/notifications
type GetNotificationsResponse struct {
	Body struct {
		Notifications []contracts.NotificationLogEntry `json:"notifications"`
		UnreadCount   int64                           `json:"unread_count"`
	}
}

// UnsubscribeFilterRequest is the request for POST /unsubscribe/filter/{id}
type UnsubscribeFilterRequest struct {
	ID  string `path:"id" doc:"Filter ID"`
	Body struct {
		Sig string `json:"sig" doc:"HMAC signature"`
	}
}

// UnsubscribeFilterResponse is the response for POST /unsubscribe/filter/{id}
type UnsubscribeFilterResponse struct {
	Body struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
}

// ──────────────────────────────────────────────
// Handlers
// ──────────────────────────────────────────────

// ListFiltersHandler handles GET /me/notification-filters
func (h *NotificationFilterHandler) ListFiltersHandler(ctx context.Context, _ *ListFiltersRequest) (*ListFiltersResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	filters, err := h.filterService.GetUserFilters(user.ID)
	if err != nil {
		requestID := logger.GetRequestID(ctx)
		logger.FromContext(ctx).Error("list_filters_failed",
			"user_id", user.ID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to list filters (request_id: %s)", requestID),
		)
	}

	resp := &ListFiltersResponse{}
	resp.Body.Filters = make([]contracts.NotificationFilterResponse, len(filters))
	for i, f := range filters {
		resp.Body.Filters[i] = filterToResponse(&f)
	}
	return resp, nil
}

// CreateFilterHandler handles POST /me/notification-filters
func (h *NotificationFilterHandler) CreateFilterHandler(ctx context.Context, req *CreateFilterRequest) (*CreateFilterResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	notifyEmail := true
	if req.Body.NotifyEmail != nil {
		notifyEmail = *req.Body.NotifyEmail
	}
	notifyInApp := true
	if req.Body.NotifyInApp != nil {
		notifyInApp = *req.Body.NotifyInApp
	}

	input := contracts.CreateFilterInput{
		Name:          req.Body.Name,
		ArtistIDs:     req.Body.ArtistIDs,
		VenueIDs:      req.Body.VenueIDs,
		LabelIDs:      req.Body.LabelIDs,
		TagIDs:        req.Body.TagIDs,
		ExcludeTagIDs: req.Body.ExcludeTagIDs,
		PriceMaxCents: req.Body.PriceMaxCents,
		NotifyEmail:   notifyEmail,
		NotifyInApp:   notifyInApp,
	}

	if req.Body.Cities != nil {
		input.Cities = *req.Body.Cities
	}

	filter, err := h.filterService.CreateFilter(user.ID, input)
	if err != nil {
		requestID := logger.GetRequestID(ctx)
		logger.FromContext(ctx).Error("create_filter_failed",
			"user_id", user.ID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error422UnprocessableEntity(err.Error())
	}

	logger.FromContext(ctx).Info("create_filter_success",
		"user_id", user.ID,
		"filter_id", filter.ID,
		"filter_name", filter.Name,
	)

	return &CreateFilterResponse{
		Body: filterToResponse(filter),
	}, nil
}

// UpdateFilterHandler handles PATCH /me/notification-filters/{id}
func (h *NotificationFilterHandler) UpdateFilterHandler(ctx context.Context, req *UpdateFilterRequest) (*UpdateFilterResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	filterID, err := strconv.ParseUint(req.ID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid filter ID")
	}

	input := contracts.UpdateFilterInput{
		Name:          req.Body.Name,
		IsActive:      req.Body.IsActive,
		ArtistIDs:     req.Body.ArtistIDs,
		VenueIDs:      req.Body.VenueIDs,
		LabelIDs:      req.Body.LabelIDs,
		TagIDs:        req.Body.TagIDs,
		ExcludeTagIDs: req.Body.ExcludeTagIDs,
		Cities:        req.Body.Cities,
		PriceMaxCents: req.Body.PriceMaxCents,
		NotifyEmail:   req.Body.NotifyEmail,
		NotifyInApp:   req.Body.NotifyInApp,
	}

	filter, err := h.filterService.UpdateFilter(user.ID, uint(filterID), input)
	if err != nil {
		requestID := logger.GetRequestID(ctx)
		logger.FromContext(ctx).Error("update_filter_failed",
			"user_id", user.ID,
			"filter_id", filterID,
			"error", err.Error(),
			"request_id", requestID,
		)
		if err.Error() == "filter not found" {
			return nil, huma.Error404NotFound("Filter not found")
		}
		return nil, huma.Error422UnprocessableEntity(
			fmt.Sprintf("Failed to update filter (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Info("update_filter_success",
		"user_id", user.ID,
		"filter_id", filterID,
	)

	return &UpdateFilterResponse{
		Body: filterToResponse(filter),
	}, nil
}

// DeleteFilterHandler handles DELETE /me/notification-filters/{id}
func (h *NotificationFilterHandler) DeleteFilterHandler(ctx context.Context, req *DeleteFilterRequest) (*DeleteFilterResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	filterID, err := strconv.ParseUint(req.ID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid filter ID")
	}

	if err := h.filterService.DeleteFilter(user.ID, uint(filterID)); err != nil {
		requestID := logger.GetRequestID(ctx)
		logger.FromContext(ctx).Error("delete_filter_failed",
			"user_id", user.ID,
			"filter_id", filterID,
			"error", err.Error(),
			"request_id", requestID,
		)
		if err.Error() == "filter not found" {
			return nil, huma.Error404NotFound("Filter not found")
		}
		return nil, huma.Error422UnprocessableEntity(
			fmt.Sprintf("Failed to delete filter (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Info("delete_filter_success",
		"user_id", user.ID,
		"filter_id", filterID,
	)

	return &DeleteFilterResponse{
		Body: struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
		}{
			Success: true,
			Message: "Filter deleted",
		},
	}, nil
}

// QuickCreateFilterHandler handles POST /me/notification-filters/quick
func (h *NotificationFilterHandler) QuickCreateFilterHandler(ctx context.Context, req *QuickCreateFilterRequest) (*QuickCreateFilterResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	if req.Body.EntityID <= 0 {
		return nil, huma.Error400BadRequest("Invalid entity ID")
	}

	filter, err := h.filterService.QuickCreateFilter(user.ID, req.Body.EntityType, uint(req.Body.EntityID))
	if err != nil {
		requestID := logger.GetRequestID(ctx)
		logger.FromContext(ctx).Error("quick_create_filter_failed",
			"user_id", user.ID,
			"entity_type", req.Body.EntityType,
			"entity_id", req.Body.EntityID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error422UnprocessableEntity(err.Error())
	}

	logger.FromContext(ctx).Info("quick_create_filter_success",
		"user_id", user.ID,
		"filter_id", filter.ID,
		"entity_type", req.Body.EntityType,
		"entity_id", req.Body.EntityID,
	)

	return &QuickCreateFilterResponse{
		Body: filterToResponse(filter),
	}, nil
}

// GetNotificationsHandler handles GET /me/notifications
func (h *NotificationFilterHandler) GetNotificationsHandler(ctx context.Context, req *GetNotificationsRequest) (*GetNotificationsResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	// Clamp pagination
	limit := req.Limit
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	notifications, err := h.filterService.GetUserNotifications(user.ID, limit, offset)
	if err != nil {
		requestID := logger.GetRequestID(ctx)
		logger.FromContext(ctx).Error("get_notifications_failed",
			"user_id", user.ID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to get notifications (request_id: %s)", requestID),
		)
	}

	unreadCount, err := h.filterService.GetUnreadCount(user.ID)
	if err != nil {
		// Non-fatal
		logger.FromContext(ctx).Warn("get_unread_count_failed",
			"user_id", user.ID,
			"error", err.Error(),
		)
	}

	resp := &GetNotificationsResponse{}
	resp.Body.Notifications = notifications
	resp.Body.UnreadCount = unreadCount
	return resp, nil
}

// UnsubscribeFilterHandler handles POST /unsubscribe/filter/{id}
// Public endpoint, HMAC-signed (no auth required).
func (h *NotificationFilterHandler) UnsubscribeFilterHandler(ctx context.Context, req *UnsubscribeFilterRequest) (*UnsubscribeFilterResponse, error) {
	filterID, err := strconv.ParseUint(req.ID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid filter ID")
	}

	if !notification.VerifyFilterUnsubscribeSignature(uint(filterID), req.Body.Sig, h.jwtSecret) {
		return nil, huma.Error403Forbidden("Invalid unsubscribe link")
	}

	if err := h.filterService.PauseFilter(uint(filterID)); err != nil {
		logger.FromContext(ctx).Error("unsubscribe_filter_failed",
			"filter_id", filterID,
			"error", err.Error(),
		)
		return nil, huma.Error500InternalServerError("Failed to pause filter")
	}

	logger.FromContext(ctx).Info("unsubscribe_filter_success",
		"filter_id", filterID,
	)

	return &UnsubscribeFilterResponse{
		Body: struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
		}{
			Success: true,
			Message: "Filter paused",
		},
	}, nil
}

// ──────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────

// filterToResponse converts a model to API response.
func filterToResponse(f *models.NotificationFilter) contracts.NotificationFilterResponse {
	resp := contracts.NotificationFilterResponse{
		ID:            f.ID,
		Name:          f.Name,
		IsActive:      f.IsActive,
		ArtistIDs:     int64ArrayToSlice(f.ArtistIDs),
		VenueIDs:      int64ArrayToSlice(f.VenueIDs),
		LabelIDs:      int64ArrayToSlice(f.LabelIDs),
		TagIDs:        int64ArrayToSlice(f.TagIDs),
		ExcludeTagIDs: int64ArrayToSlice(f.ExcludeTagIDs),
		Cities:        f.Cities,
		PriceMaxCents: f.PriceMaxCents,
		NotifyEmail:   f.NotifyEmail,
		NotifyInApp:   f.NotifyInApp,
		NotifyPush:    f.NotifyPush,
		MatchCount:    f.MatchCount,
		LastMatchedAt: f.LastMatchedAt,
		CreatedAt:     f.CreatedAt,
		UpdatedAt:     f.UpdatedAt,
	}
	return resp
}

// int64ArrayToSlice converts pq.Int64Array to []int64, returning nil for empty/nil arrays.
func int64ArrayToSlice(arr []int64) []int64 {
	if len(arr) == 0 {
		return nil
	}
	return arr
}
