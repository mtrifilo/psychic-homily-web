package handlers

import (
	"context"
	"fmt"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services"
)

// AdminUserHandler handles admin user management
type AdminUserHandler struct {
	userService services.UserServiceInterface
}

// NewAdminUserHandler creates a new admin user handler
func NewAdminUserHandler(
	userService services.UserServiceInterface,
) *AdminUserHandler {
	return &AdminUserHandler{
		userService: userService,
	}
}

// GetAdminUsersRequest represents the HTTP request for listing users
type GetAdminUsersRequest struct {
	Limit  int    `query:"limit" default:"50" doc:"Number of users to return (max 100)"`
	Offset int    `query:"offset" default:"0" doc:"Offset for pagination"`
	Search string `query:"search" doc:"Search by email or username"`
}

// GetAdminUsersResponse represents the HTTP response for listing users
type GetAdminUsersResponse struct {
	Body struct {
		Users []*services.AdminUserResponse `json:"users"`
		Total int64                         `json:"total"`
	}
}

// GetAdminUsersHandler handles GET /admin/users
func (h *AdminUserHandler) GetAdminUsersHandler(ctx context.Context, req *GetAdminUsersRequest) (*GetAdminUsersResponse, error) {
	requestID := logger.GetRequestID(ctx)

	_, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	// Validate limit
	limit := req.Limit
	if limit < 1 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	logger.FromContext(ctx).Debug("admin_users_list_attempt",
		"limit", limit,
		"offset", offset,
		"search", req.Search,
	)

	// Build filters
	filters := services.AdminUserFilters{
		Search: req.Search,
	}

	// Get users
	users, total, err := h.userService.ListUsers(limit, offset, filters)
	if err != nil {
		logger.FromContext(ctx).Error("admin_users_list_failed",
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to get users (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Debug("admin_users_list_success",
		"count", len(users),
		"total", total,
	)

	return &GetAdminUsersResponse{
		Body: struct {
			Users []*services.AdminUserResponse `json:"users"`
			Total int64                         `json:"total"`
		}{
			Users: users,
			Total: total,
		},
	}, nil
}
