package handlers

import (
	"context"
	"fmt"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services"
)

// UserPreferencesHandler handles user preferences endpoints
type UserPreferencesHandler struct {
	userService services.UserServiceInterface
}

// NewUserPreferencesHandler creates a new user preferences handler
func NewUserPreferencesHandler(userService services.UserServiceInterface) *UserPreferencesHandler {
	return &UserPreferencesHandler{
		userService: userService,
	}
}

// SetFavoriteCitiesRequest represents the request to update favorite cities
type SetFavoriteCitiesRequest struct {
	Body struct {
		Cities []models.FavoriteCity `json:"cities" doc:"List of favorite cities (max 20)"`
	}
}

// SetFavoriteCitiesResponse represents the response after updating favorite cities
type SetFavoriteCitiesResponse struct {
	Body struct {
		Success bool                 `json:"success"`
		Message string               `json:"message"`
		Cities  []models.FavoriteCity `json:"cities"`
	}
}

// SetFavoriteCitiesHandler handles PUT /auth/preferences/favorite-cities
func (h *UserPreferencesHandler) SetFavoriteCitiesHandler(ctx context.Context, req *SetFavoriteCitiesRequest) (*SetFavoriteCitiesResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	cities := req.Body.Cities
	if cities == nil {
		cities = []models.FavoriteCity{}
	}

	if err := h.userService.SetFavoriteCities(user.ID, cities); err != nil {
		logger.FromContext(ctx).Error("set_favorite_cities_failed",
			"error", err.Error(),
			"user_id", user.ID,
			"request_id", requestID,
		)
		return nil, huma.Error422UnprocessableEntity(
			fmt.Sprintf("Failed to save favorite cities: %s", err.Error()),
		)
	}

	logger.FromContext(ctx).Info("set_favorite_cities_success",
		"user_id", user.ID,
		"count", len(cities),
	)

	return &SetFavoriteCitiesResponse{
		Body: struct {
			Success bool                 `json:"success"`
			Message string               `json:"message"`
			Cities  []models.FavoriteCity `json:"cities"`
		}{
			Success: true,
			Message: "Favorite cities updated",
			Cities:  cities,
		},
	}, nil
}
