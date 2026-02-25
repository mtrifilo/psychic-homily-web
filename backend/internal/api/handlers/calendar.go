package handlers

import (
	"context"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services"
)

// CalendarHandler handles calendar feed and token management
type CalendarHandler struct {
	calendarService services.CalendarServiceInterface
	config          *config.Config
}

// NewCalendarHandler creates a new calendar handler
func NewCalendarHandler(calendarService services.CalendarServiceInterface, cfg *config.Config) *CalendarHandler {
	return &CalendarHandler{
		calendarService: calendarService,
		config:          cfg,
	}
}

// --- ICS Feed endpoint (Chi http.HandlerFunc, public, token-authenticated) ---

// GetCalendarFeedHandler serves the ICS calendar feed
func (h *CalendarHandler) GetCalendarFeedHandler(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if token == "" {
		http.Error(w, "missing token", http.StatusBadRequest)
		return
	}

	user, err := h.calendarService.ValidateCalendarToken(token)
	if err != nil {
		logger.FromContext(r.Context()).Warn("calendar_feed_invalid_token",
			"error", err.Error(),
		)
		http.Error(w, "invalid or expired token", http.StatusUnauthorized)
		return
	}

	frontendURL := h.config.Email.FrontendURL
	if frontendURL == "" {
		frontendURL = "http://localhost:3000"
	}

	icsData, err := h.calendarService.GenerateICSFeed(user.ID, frontendURL)
	if err != nil {
		logger.FromContext(r.Context()).Error("calendar_feed_generation_failed",
			"user_id", user.ID,
			"error", err.Error(),
		)
		http.Error(w, "failed to generate calendar feed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", "inline; filename=\"psychic-homily.ics\"")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.WriteHeader(http.StatusOK)
	w.Write(icsData)
}

// --- Token CRUD endpoints (Huma, protected) ---

// CreateCalendarTokenRequest is empty — user is derived from JWT context
type CreateCalendarTokenRequest struct{}

// CreateCalendarTokenResponse wraps the service response
type CreateCalendarTokenResponse struct {
	Body services.CalendarTokenCreateResponse
}

// CreateCalendarTokenHandler creates or regenerates a calendar token
func (h *CalendarHandler) CreateCalendarTokenHandler(ctx context.Context, req *CreateCalendarTokenRequest) (*CreateCalendarTokenResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	// Derive the API base URL from config
	// In production this is the backend's public URL (e.g. https://api.psychichomily.com)
	apiBaseURL := h.config.Email.FrontendURL
	if apiBaseURL == "" {
		apiBaseURL = "http://localhost:8080"
	}
	// The feed URL should use the API domain, not the frontend domain
	// Derive from the MusicDiscovery FrontendURL pattern, but use the API URL
	apiBaseURL = getAPIBaseURL(h.config)

	result, err := h.calendarService.CreateToken(user.ID, apiBaseURL)
	if err != nil {
		logger.FromContext(ctx).Error("create_calendar_token_failed",
			"user_id", user.ID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to create calendar token (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Info("create_calendar_token_success",
		"user_id", user.ID,
		"request_id", requestID,
	)

	return &CreateCalendarTokenResponse{Body: *result}, nil
}

// GetCalendarTokenStatusRequest is empty — user is derived from JWT context
type GetCalendarTokenStatusRequest struct{}

// GetCalendarTokenStatusResponse wraps the service response
type GetCalendarTokenStatusResponse struct {
	Body services.CalendarTokenStatusResponse
}

// GetCalendarTokenStatusHandler checks if a user has a calendar token
func (h *CalendarHandler) GetCalendarTokenStatusHandler(ctx context.Context, req *GetCalendarTokenStatusRequest) (*GetCalendarTokenStatusResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	status, err := h.calendarService.GetTokenStatus(user.ID)
	if err != nil {
		logger.FromContext(ctx).Error("get_calendar_token_status_failed",
			"user_id", user.ID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to get calendar token status (request_id: %s)", requestID),
		)
	}

	return &GetCalendarTokenStatusResponse{Body: *status}, nil
}

// DeleteCalendarTokenRequest is empty — user is derived from JWT context
type DeleteCalendarTokenRequest struct{}

// DeleteCalendarTokenResponse returns success status
type DeleteCalendarTokenResponse struct {
	Body struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
}

// DeleteCalendarTokenHandler removes a user's calendar token
func (h *CalendarHandler) DeleteCalendarTokenHandler(ctx context.Context, req *DeleteCalendarTokenRequest) (*DeleteCalendarTokenResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	err := h.calendarService.DeleteToken(user.ID)
	if err != nil {
		logger.FromContext(ctx).Error("delete_calendar_token_failed",
			"user_id", user.ID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error422UnprocessableEntity(
			fmt.Sprintf("Failed to delete calendar token (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Info("delete_calendar_token_success",
		"user_id", user.ID,
		"request_id", requestID,
	)

	return &DeleteCalendarTokenResponse{
		Body: struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
		}{
			Success: true,
			Message: "Calendar token deleted successfully",
		},
	}, nil
}

// getAPIBaseURL derives the API's public base URL from config
func getAPIBaseURL(cfg *config.Config) string {
	// In production, the API is at api.psychichomily.com
	// In development, it's localhost:8080
	frontendURL := cfg.Email.FrontendURL
	switch frontendURL {
	case "https://psychichomily.com":
		return "https://api.psychichomily.com"
	case "https://stage.psychichomily.com":
		return "https://api-stage.psychichomily.com"
	default:
		return "http://localhost:8080"
	}
}
