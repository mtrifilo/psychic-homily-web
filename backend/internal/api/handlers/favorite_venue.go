package handlers

import (
	"context"
	"fmt"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services"
)

// FavoriteVenueHandler handles favorite venue HTTP requests
type FavoriteVenueHandler struct {
	favoriteVenueService services.FavoriteVenueServiceInterface
}

// NewFavoriteVenueHandler creates a new favorite venue handler
func NewFavoriteVenueHandler(favoriteVenueService services.FavoriteVenueServiceInterface) *FavoriteVenueHandler {
	return &FavoriteVenueHandler{
		favoriteVenueService: favoriteVenueService,
	}
}

// FavoriteVenueRequest represents the HTTP request for favoriting a venue
type FavoriteVenueRequest struct {
	VenueID string `path:"venue_id" validate:"required" doc:"Venue ID"`
}

// FavoriteVenueResponse represents the HTTP response for favoriting a venue
type FavoriteVenueResponse struct {
	Body struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
}

// UnfavoriteVenueRequest represents the HTTP request for unfavoriting a venue
type UnfavoriteVenueRequest struct {
	VenueID string `path:"venue_id" validate:"required" doc:"Venue ID"`
}

// UnfavoriteVenueResponse represents the HTTP response for unfavoriting a venue
type UnfavoriteVenueResponse struct {
	Body struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
}

// GetFavoriteVenuesRequest represents the HTTP request for listing favorite venues
type GetFavoriteVenuesRequest struct {
	Limit  int `query:"limit" default:"50" doc:"Number of venues per page"`
	Offset int `query:"offset" default:"0" doc:"Offset for pagination"`
}

// GetFavoriteVenuesResponse represents the HTTP response for listing favorite venues
type GetFavoriteVenuesResponse struct {
	Body struct {
		Venues []*services.FavoriteVenueResponse `json:"venues"`
		Total  int64                             `json:"total"`
		Limit  int                               `json:"limit"`
		Offset int                               `json:"offset"`
	}
}

// CheckFavoritedRequest represents the HTTP request for checking if a venue is favorited
type CheckFavoritedRequest struct {
	VenueID string `path:"venue_id" validate:"required" doc:"Venue ID"`
}

// CheckFavoritedResponse represents the HTTP response for checking if a venue is favorited
type CheckFavoritedResponse struct {
	Body struct {
		IsFavorited bool `json:"is_favorited"`
	}
}

// GetFavoriteVenueShowsRequest represents the HTTP request for getting shows from favorite venues
type GetFavoriteVenueShowsRequest struct {
	Timezone string `query:"timezone" default:"America/Phoenix" doc:"Timezone for date filtering"`
	Limit    int    `query:"limit" default:"50" doc:"Number of shows per page"`
	Offset   int    `query:"offset" default:"0" doc:"Offset for pagination"`
}

// GetFavoriteVenueShowsResponse represents the HTTP response for getting shows from favorite venues
type GetFavoriteVenueShowsResponse struct {
	Body struct {
		Shows    []*services.FavoriteVenueShowResponse `json:"shows"`
		Total    int64                                 `json:"total"`
		Limit    int                                   `json:"limit"`
		Offset   int                                   `json:"offset"`
		Timezone string                                `json:"timezone"`
	}
}

// FavoriteVenueHandler handles POST /favorite-venues/{venue_id}
func (h *FavoriteVenueHandler) FavoriteVenueHandler(ctx context.Context, req *FavoriteVenueRequest) (*FavoriteVenueResponse, error) {
	requestID := logger.GetRequestID(ctx)

	// Get authenticated user
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	// Parse venue ID
	venueID, err := strconv.ParseUint(req.VenueID, 10, 32)
	if err != nil {
		logger.FromContext(ctx).Warn("favorite_venue_invalid_id",
			"venue_id_str", req.VenueID,
			"request_id", requestID,
		)
		return nil, huma.Error400BadRequest("Invalid venue ID")
	}

	logger.FromContext(ctx).Debug("favorite_venue_attempt",
		"user_id", user.ID,
		"venue_id", venueID,
	)

	// Favorite the venue
	err = h.favoriteVenueService.FavoriteVenue(user.ID, uint(venueID))
	if err != nil {
		logger.FromContext(ctx).Error("favorite_venue_failed",
			"user_id", user.ID,
			"venue_id", venueID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error422UnprocessableEntity(
			fmt.Sprintf("Failed to favorite venue (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Info("favorite_venue_success",
		"user_id", user.ID,
		"venue_id", venueID,
		"request_id", requestID,
	)

	return &FavoriteVenueResponse{
		Body: struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
		}{
			Success: true,
			Message: "Venue favorited successfully",
		},
	}, nil
}

// UnfavoriteVenueHandler handles DELETE /favorite-venues/{venue_id}
func (h *FavoriteVenueHandler) UnfavoriteVenueHandler(ctx context.Context, req *UnfavoriteVenueRequest) (*UnfavoriteVenueResponse, error) {
	requestID := logger.GetRequestID(ctx)

	// Get authenticated user
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	// Parse venue ID
	venueID, err := strconv.ParseUint(req.VenueID, 10, 32)
	if err != nil {
		logger.FromContext(ctx).Warn("unfavorite_venue_invalid_id",
			"venue_id_str", req.VenueID,
			"request_id", requestID,
		)
		return nil, huma.Error400BadRequest("Invalid venue ID")
	}

	logger.FromContext(ctx).Debug("unfavorite_venue_attempt",
		"user_id", user.ID,
		"venue_id", venueID,
	)

	// Unfavorite the venue
	err = h.favoriteVenueService.UnfavoriteVenue(user.ID, uint(venueID))
	if err != nil {
		logger.FromContext(ctx).Error("unfavorite_venue_failed",
			"user_id", user.ID,
			"venue_id", venueID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error422UnprocessableEntity(
			fmt.Sprintf("Failed to unfavorite venue (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Info("unfavorite_venue_success",
		"user_id", user.ID,
		"venue_id", venueID,
		"request_id", requestID,
	)

	return &UnfavoriteVenueResponse{
		Body: struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
		}{
			Success: true,
			Message: "Venue unfavorited successfully",
		},
	}, nil
}

// GetFavoriteVenuesHandler handles GET /favorite-venues
func (h *FavoriteVenueHandler) GetFavoriteVenuesHandler(ctx context.Context, req *GetFavoriteVenuesRequest) (*GetFavoriteVenuesResponse, error) {
	requestID := logger.GetRequestID(ctx)

	// Get authenticated user
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	// Validate pagination
	limit := req.Limit
	if limit < 1 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	logger.FromContext(ctx).Debug("get_favorite_venues_attempt",
		"user_id", user.ID,
		"limit", limit,
		"offset", offset,
	)

	// Get favorite venues
	venues, total, err := h.favoriteVenueService.GetUserFavoriteVenues(user.ID, limit, offset)
	if err != nil {
		logger.FromContext(ctx).Error("get_favorite_venues_failed",
			"user_id", user.ID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to get favorite venues (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Debug("get_favorite_venues_success",
		"user_id", user.ID,
		"count", len(venues),
		"total", total,
	)

	return &GetFavoriteVenuesResponse{
		Body: struct {
			Venues []*services.FavoriteVenueResponse `json:"venues"`
			Total  int64                             `json:"total"`
			Limit  int                               `json:"limit"`
			Offset int                               `json:"offset"`
		}{
			Venues: venues,
			Total:  total,
			Limit:  limit,
			Offset: offset,
		},
	}, nil
}

// CheckFavoritedHandler handles GET /favorite-venues/{venue_id}/check
func (h *FavoriteVenueHandler) CheckFavoritedHandler(ctx context.Context, req *CheckFavoritedRequest) (*CheckFavoritedResponse, error) {
	requestID := logger.GetRequestID(ctx)

	// Get authenticated user
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	// Parse venue ID
	venueID, err := strconv.ParseUint(req.VenueID, 10, 32)
	if err != nil {
		logger.FromContext(ctx).Warn("check_favorited_invalid_id",
			"venue_id_str", req.VenueID,
			"request_id", requestID,
		)
		return nil, huma.Error400BadRequest("Invalid venue ID")
	}

	// Check if favorited
	isFavorited, err := h.favoriteVenueService.IsVenueFavorited(user.ID, uint(venueID))
	if err != nil {
		logger.FromContext(ctx).Error("check_favorited_failed",
			"user_id", user.ID,
			"venue_id", venueID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to check if venue is favorited (request_id: %s)", requestID),
		)
	}

	return &CheckFavoritedResponse{
		Body: struct {
			IsFavorited bool `json:"is_favorited"`
		}{
			IsFavorited: isFavorited,
		},
	}, nil
}

// GetFavoriteVenueShowsHandler handles GET /favorite-venues/shows
func (h *FavoriteVenueHandler) GetFavoriteVenueShowsHandler(ctx context.Context, req *GetFavoriteVenueShowsRequest) (*GetFavoriteVenueShowsResponse, error) {
	requestID := logger.GetRequestID(ctx)

	// Get authenticated user
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	// Validate pagination
	limit := req.Limit
	if limit < 1 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	timezone := req.Timezone
	if timezone == "" {
		timezone = "America/Phoenix"
	}

	logger.FromContext(ctx).Debug("get_favorite_venue_shows_attempt",
		"user_id", user.ID,
		"timezone", timezone,
		"limit", limit,
		"offset", offset,
	)

	// Get upcoming shows from favorite venues
	shows, total, err := h.favoriteVenueService.GetUpcomingShowsFromFavorites(user.ID, timezone, limit, offset)
	if err != nil {
		logger.FromContext(ctx).Error("get_favorite_venue_shows_failed",
			"user_id", user.ID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to get shows from favorite venues (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Debug("get_favorite_venue_shows_success",
		"user_id", user.ID,
		"count", len(shows),
		"total", total,
	)

	return &GetFavoriteVenueShowsResponse{
		Body: struct {
			Shows    []*services.FavoriteVenueShowResponse `json:"shows"`
			Total    int64                                 `json:"total"`
			Limit    int                                   `json:"limit"`
			Offset   int                                   `json:"offset"`
			Timezone string                                `json:"timezone"`
		}{
			Shows:    shows,
			Total:    total,
			Limit:    limit,
			Offset:   offset,
			Timezone: timezone,
		},
	}, nil
}
