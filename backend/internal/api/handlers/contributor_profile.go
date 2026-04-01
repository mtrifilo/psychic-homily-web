package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/contracts"
)

// ContributorProfileHandler handles contributor profile HTTP requests.
type ContributorProfileHandler struct {
	profileService contracts.ContributorProfileServiceInterface
	userService    contracts.UserServiceInterface
}

// NewContributorProfileHandler creates a new contributor profile handler.
func NewContributorProfileHandler(
	profileService contracts.ContributorProfileServiceInterface,
	userService contracts.UserServiceInterface,
) *ContributorProfileHandler {
	return &ContributorProfileHandler{
		profileService: profileService,
		userService:    userService,
	}
}

// --- Request/Response Types ---

type GetPublicProfileRequest struct {
	Username string `path:"username" doc:"Username of the contributor"`
}

type GetPublicProfileResponse struct {
	Body *contracts.PublicProfileResponse
}

type GetContributionHistoryRequest struct {
	Username   string `path:"username" doc:"Username of the contributor"`
	Limit      int    `query:"limit" default:"20" doc:"Number of contributions per page (max 100)"`
	Offset     int    `query:"offset" default:"0" doc:"Offset for pagination"`
	EntityType string `query:"entity_type" default:"" doc:"Filter by entity type (show, venue, artist, release, label, festival)"`
}

type GetContributionHistoryResponse struct {
	Body struct {
		Contributions []*contracts.ContributionEntry `json:"contributions"`
		Total         int64                         `json:"total"`
		Limit         int                           `json:"limit"`
		Offset        int                           `json:"offset"`
	}
}

type GetOwnProfileResponse struct {
	Body *contracts.PublicProfileResponse
}

type GetOwnContributionsRequest struct {
	Limit      int    `query:"limit" default:"20" doc:"Number of contributions per page (max 100)"`
	Offset     int    `query:"offset" default:"0" doc:"Offset for pagination"`
	EntityType string `query:"entity_type" default:"" doc:"Filter by entity type"`
}

type GetOwnContributionsResponse struct {
	Body struct {
		Contributions []*contracts.ContributionEntry `json:"contributions"`
		Total         int64                         `json:"total"`
		Limit         int                           `json:"limit"`
		Offset        int                           `json:"offset"`
	}
}

type UpdateProfileVisibilityRequest struct {
	Body struct {
		Visibility string `json:"visibility" doc:"Profile visibility: public or private"`
	}
}

type UpdateProfileVisibilityResponse struct {
	Body struct {
		Success    bool   `json:"success"`
		Visibility string `json:"visibility"`
	}
}

type UpdatePrivacySettingsRequest struct {
	Body contracts.PrivacySettings
}

type UpdatePrivacySettingsResponse struct {
	Body struct {
		Success  bool                     `json:"success"`
		Settings contracts.PrivacySettings `json:"privacy_settings"`
	}
}

type GetUserSectionsRequest struct {
	Username string `path:"username" doc:"Username of the contributor"`
}

type GetUserSectionsResponse struct {
	Body struct {
		Sections []*contracts.ProfileSectionResponse `json:"sections"`
	}
}

type GetOwnSectionsResponse struct {
	Body struct {
		Sections []*contracts.ProfileSectionResponse `json:"sections"`
	}
}

type CreateSectionRequest struct {
	Body struct {
		Title    string `json:"title" doc:"Section title (1-255 chars)"`
		Content  string `json:"content" doc:"Section content (markdown, max 10000 chars)"`
		Position int    `json:"position" doc:"Position index (0-2)"`
	}
}

type CreateSectionResponse struct {
	Body *contracts.ProfileSectionResponse
}

type UpdateSectionRequest struct {
	SectionID string  `path:"section_id" doc:"Section ID to update"`
	Body      struct {
		Title     *string `json:"title,omitempty" required:"false" doc:"Section title"`
		Content   *string `json:"content,omitempty" required:"false" doc:"Section content (markdown)"`
		Position  *int    `json:"position,omitempty" required:"false" doc:"Position index"`
		IsVisible *bool   `json:"is_visible,omitempty" required:"false" doc:"Whether section is publicly visible"`
	}
}

type UpdateSectionResponse struct {
	Body *contracts.ProfileSectionResponse
}

type GetActivityHeatmapRequest struct {
	Username string `path:"username" doc:"Username of the contributor"`
}

type GetActivityHeatmapResponse struct {
	Body *contracts.ActivityHeatmapResponse
}

type DeleteSectionRequest struct {
	SectionID string `path:"section_id" doc:"Section ID to delete"`
}

type DeleteSectionResponse struct {
	Body struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
}

type GetPercentileRankingsRequest struct {
	Username string `path:"username" doc:"Username of the contributor"`
}

type GetPercentileRankingsResponse struct {
	Body *contracts.PercentileRankings
}

// --- Handlers ---

// GetPublicProfileHandler handles GET /users/{username}.
func (h *ContributorProfileHandler) GetPublicProfileHandler(ctx context.Context, req *GetPublicProfileRequest) (*GetPublicProfileResponse, error) {
	requestID := logger.GetRequestID(ctx)

	var viewerID *uint
	user := middleware.GetUserFromContext(ctx)
	if user != nil {
		viewerID = &user.ID
	}

	profile, err := h.profileService.GetPublicProfile(req.Username, viewerID)
	if err != nil {
		logger.FromContext(ctx).Error("get_public_profile_failed",
			"username", req.Username,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to get profile (request_id: %s)", requestID),
		)
	}

	if profile == nil {
		return nil, huma.Error404NotFound("User not found")
	}

	return &GetPublicProfileResponse{Body: profile}, nil
}

// GetContributionHistoryHandler handles GET /users/{username}/contributions.
func (h *ContributorProfileHandler) GetContributionHistoryHandler(ctx context.Context, req *GetContributionHistoryRequest) (*GetContributionHistoryResponse, error) {
	requestID := logger.GetRequestID(ctx)

	targetUser, err := h.userService.GetUserByUsername(req.Username)
	if err != nil {
		logger.FromContext(ctx).Error("get_contribution_history_user_lookup_failed",
			"username", req.Username,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to look up user (request_id: %s)", requestID),
		)
	}
	if targetUser == nil {
		return nil, huma.Error404NotFound("User not found")
	}

	viewer := middleware.GetUserFromContext(ctx)
	isOwner := viewer != nil && viewer.ID == targetUser.ID

	// Master privacy check
	if targetUser.ProfileVisibility == "private" && !isOwner {
		return nil, huma.Error404NotFound("User not found")
	}

	// Granular privacy check for contributions
	if !isOwner {
		privacy := contracts.DefaultPrivacySettings()
		if targetUser.PrivacySettings != nil {
			_ = json.Unmarshal(*targetUser.PrivacySettings, &privacy)
		}

		switch privacy.Contributions {
		case contracts.PrivacyHidden:
			return nil, huma.Error404NotFound("User not found")
		case contracts.PrivacyCountOnly:
			// Return total count but no items
			stats, err := h.profileService.GetContributionStats(targetUser.ID)
			if err != nil {
				logger.FromContext(ctx).Error("get_contribution_stats_failed",
					"user_id", targetUser.ID,
					"error", err.Error(),
					"request_id", requestID,
				)
				return nil, huma.Error500InternalServerError(
					fmt.Sprintf("Failed to get contributions (request_id: %s)", requestID),
				)
			}
			return &GetContributionHistoryResponse{
				Body: struct {
					Contributions []*contracts.ContributionEntry `json:"contributions"`
					Total         int64                         `json:"total"`
					Limit         int                           `json:"limit"`
					Offset        int                           `json:"offset"`
				}{
					Contributions: []*contracts.ContributionEntry{},
					Total:         stats.TotalContributions,
					Limit:         req.Limit,
					Offset:        req.Offset,
				},
			}, nil
		}
	}

	contributions, total, err := h.profileService.GetContributionHistory(targetUser.ID, req.Limit, req.Offset, req.EntityType)
	if err != nil {
		logger.FromContext(ctx).Error("get_contribution_history_failed",
			"user_id", targetUser.ID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to get contributions (request_id: %s)", requestID),
		)
	}

	return &GetContributionHistoryResponse{
		Body: struct {
			Contributions []*contracts.ContributionEntry `json:"contributions"`
			Total         int64                         `json:"total"`
			Limit         int                           `json:"limit"`
			Offset        int                           `json:"offset"`
		}{
			Contributions: contributions,
			Total:         total,
			Limit:         req.Limit,
			Offset:        req.Offset,
		},
	}, nil
}

// GetOwnProfileHandler handles GET /auth/profile/contributor.
func (h *ContributorProfileHandler) GetOwnProfileHandler(ctx context.Context, req *struct{}) (*GetOwnProfileResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	profile, err := h.profileService.GetOwnProfile(user.ID)
	if err != nil {
		logger.FromContext(ctx).Error("get_own_profile_failed",
			"user_id", user.ID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to get profile (request_id: %s)", requestID),
		)
	}

	if profile == nil {
		return nil, huma.Error404NotFound("Profile not found")
	}

	return &GetOwnProfileResponse{Body: profile}, nil
}

// GetOwnContributionsHandler handles GET /auth/profile/contributions.
func (h *ContributorProfileHandler) GetOwnContributionsHandler(ctx context.Context, req *GetOwnContributionsRequest) (*GetOwnContributionsResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	contributions, total, err := h.profileService.GetContributionHistory(user.ID, req.Limit, req.Offset, req.EntityType)
	if err != nil {
		logger.FromContext(ctx).Error("get_own_contributions_failed",
			"user_id", user.ID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to get contributions (request_id: %s)", requestID),
		)
	}

	return &GetOwnContributionsResponse{
		Body: struct {
			Contributions []*contracts.ContributionEntry `json:"contributions"`
			Total         int64                         `json:"total"`
			Limit         int                           `json:"limit"`
			Offset        int                           `json:"offset"`
		}{
			Contributions: contributions,
			Total:         total,
			Limit:         req.Limit,
			Offset:        req.Offset,
		},
	}, nil
}

// UpdateProfileVisibilityHandler handles PATCH /auth/profile/visibility.
func (h *ContributorProfileHandler) UpdateProfileVisibilityHandler(ctx context.Context, req *UpdateProfileVisibilityRequest) (*UpdateProfileVisibilityResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	visibility := req.Body.Visibility
	if visibility != "public" && visibility != "private" {
		return nil, huma.Error400BadRequest("Visibility must be 'public' or 'private'")
	}

	_, err := h.userService.UpdateUser(user.ID, map[string]any{
		"profile_visibility": visibility,
	})
	if err != nil {
		logger.FromContext(ctx).Error("update_profile_visibility_failed",
			"user_id", user.ID,
			"visibility", visibility,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to update visibility (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Info("profile_visibility_updated",
		"user_id", user.ID,
		"visibility", visibility,
	)

	return &UpdateProfileVisibilityResponse{
		Body: struct {
			Success    bool   `json:"success"`
			Visibility string `json:"visibility"`
		}{
			Success:    true,
			Visibility: visibility,
		},
	}, nil
}

// UpdatePrivacySettingsHandler handles PATCH /auth/profile/privacy.
func (h *ContributorProfileHandler) UpdatePrivacySettingsHandler(ctx context.Context, req *UpdatePrivacySettingsRequest) (*UpdatePrivacySettingsResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	settings, err := h.profileService.UpdatePrivacySettings(user.ID, req.Body)
	if err != nil {
		logger.FromContext(ctx).Error("update_privacy_settings_failed",
			"user_id", user.ID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error400BadRequest(err.Error())
	}

	logger.FromContext(ctx).Info("privacy_settings_updated", "user_id", user.ID)

	return &UpdatePrivacySettingsResponse{
		Body: struct {
			Success  bool                     `json:"success"`
			Settings contracts.PrivacySettings `json:"privacy_settings"`
		}{
			Success:  true,
			Settings: *settings,
		},
	}, nil
}

// GetUserSectionsHandler handles GET /users/{username}/sections.
func (h *ContributorProfileHandler) GetUserSectionsHandler(ctx context.Context, req *GetUserSectionsRequest) (*GetUserSectionsResponse, error) {
	requestID := logger.GetRequestID(ctx)

	targetUser, err := h.userService.GetUserByUsername(req.Username)
	if err != nil {
		logger.FromContext(ctx).Error("get_user_sections_lookup_failed",
			"username", req.Username,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to look up user (request_id: %s)", requestID),
		)
	}
	if targetUser == nil {
		return nil, huma.Error404NotFound("User not found")
	}

	// Master privacy check
	viewer := middleware.GetUserFromContext(ctx)
	isOwner := viewer != nil && viewer.ID == targetUser.ID

	if targetUser.ProfileVisibility == "private" && !isOwner {
		return nil, huma.Error404NotFound("User not found")
	}

	var sections []*contracts.ProfileSectionResponse
	if isOwner {
		sections, err = h.profileService.GetOwnSections(targetUser.ID)
	} else {
		// Check granular privacy
		privacy := contracts.DefaultPrivacySettings()
		if targetUser.PrivacySettings != nil {
			_ = json.Unmarshal(*targetUser.PrivacySettings, &privacy)
		}
		if privacy.ProfileSections == contracts.PrivacyHidden {
			sections = []*contracts.ProfileSectionResponse{}
		} else {
			sections, err = h.profileService.GetUserSections(targetUser.ID)
		}
	}
	if err != nil {
		logger.FromContext(ctx).Error("get_user_sections_failed",
			"user_id", targetUser.ID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to get sections (request_id: %s)", requestID),
		)
	}

	return &GetUserSectionsResponse{
		Body: struct {
			Sections []*contracts.ProfileSectionResponse `json:"sections"`
		}{
			Sections: sections,
		},
	}, nil
}

// GetOwnSectionsHandler handles GET /auth/profile/sections.
func (h *ContributorProfileHandler) GetOwnSectionsHandler(ctx context.Context, req *struct{}) (*GetOwnSectionsResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	sections, err := h.profileService.GetOwnSections(user.ID)
	if err != nil {
		logger.FromContext(ctx).Error("get_own_sections_failed",
			"user_id", user.ID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to get sections (request_id: %s)", requestID),
		)
	}

	return &GetOwnSectionsResponse{
		Body: struct {
			Sections []*contracts.ProfileSectionResponse `json:"sections"`
		}{
			Sections: sections,
		},
	}, nil
}

// CreateSectionHandler handles POST /auth/profile/sections.
func (h *ContributorProfileHandler) CreateSectionHandler(ctx context.Context, req *CreateSectionRequest) (*CreateSectionResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	section, err := h.profileService.CreateSection(user.ID, req.Body.Title, req.Body.Content, req.Body.Position)
	if err != nil {
		logger.FromContext(ctx).Error("create_section_failed",
			"user_id", user.ID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error400BadRequest(err.Error())
	}

	logger.FromContext(ctx).Info("profile_section_created",
		"user_id", user.ID,
		"section_id", section.ID,
	)

	return &CreateSectionResponse{Body: section}, nil
}

// UpdateSectionHandler handles PUT /auth/profile/sections/{section_id}.
func (h *ContributorProfileHandler) UpdateSectionHandler(ctx context.Context, req *UpdateSectionRequest) (*UpdateSectionResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	sectionID, err := strconv.ParseUint(req.SectionID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid section ID")
	}

	updates := map[string]interface{}{}
	if req.Body.Title != nil {
		updates["title"] = *req.Body.Title
	}
	if req.Body.Content != nil {
		updates["content"] = *req.Body.Content
	}
	if req.Body.Position != nil {
		updates["position"] = *req.Body.Position
	}
	if req.Body.IsVisible != nil {
		updates["is_visible"] = *req.Body.IsVisible
	}

	if len(updates) == 0 {
		return nil, huma.Error400BadRequest("No fields to update")
	}

	section, err := h.profileService.UpdateSection(user.ID, uint(sectionID), updates)
	if err != nil {
		logger.FromContext(ctx).Error("update_section_failed",
			"user_id", user.ID,
			"section_id", sectionID,
			"error", err.Error(),
			"request_id", requestID,
		)
		if err.Error() == "profile section not found" {
			return nil, huma.Error404NotFound("Profile section not found")
		}
		return nil, huma.Error400BadRequest(err.Error())
	}

	logger.FromContext(ctx).Info("profile_section_updated",
		"user_id", user.ID,
		"section_id", sectionID,
	)

	return &UpdateSectionResponse{Body: section}, nil
}

// GetActivityHeatmapHandler handles GET /users/{username}/activity-heatmap.
func (h *ContributorProfileHandler) GetActivityHeatmapHandler(ctx context.Context, req *GetActivityHeatmapRequest) (*GetActivityHeatmapResponse, error) {
	requestID := logger.GetRequestID(ctx)

	targetUser, err := h.userService.GetUserByUsername(req.Username)
	if err != nil {
		logger.FromContext(ctx).Error("get_activity_heatmap_user_lookup_failed",
			"username", req.Username,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to look up user (request_id: %s)", requestID),
		)
	}
	if targetUser == nil {
		return nil, huma.Error404NotFound("User not found")
	}

	viewer := middleware.GetUserFromContext(ctx)
	isOwner := viewer != nil && viewer.ID == targetUser.ID

	// Master privacy check
	if targetUser.ProfileVisibility == "private" && !isOwner {
		return nil, huma.Error404NotFound("User not found")
	}

	// Granular privacy check for contributions
	if !isOwner {
		privacy := contracts.DefaultPrivacySettings()
		if targetUser.PrivacySettings != nil {
			_ = json.Unmarshal(*targetUser.PrivacySettings, &privacy)
		}

		if privacy.Contributions == contracts.PrivacyHidden {
			return &GetActivityHeatmapResponse{
				Body: &contracts.ActivityHeatmapResponse{Days: []contracts.ActivityDay{}},
			}, nil
		}
	}

	heatmap, err := h.profileService.GetActivityHeatmap(targetUser.ID)
	if err != nil {
		logger.FromContext(ctx).Error("get_activity_heatmap_failed",
			"user_id", targetUser.ID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to get activity heatmap (request_id: %s)", requestID),
		)
	}

	return &GetActivityHeatmapResponse{Body: heatmap}, nil
}

// DeleteSectionHandler handles DELETE /auth/profile/sections/{section_id}.
func (h *ContributorProfileHandler) DeleteSectionHandler(ctx context.Context, req *DeleteSectionRequest) (*DeleteSectionResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	sectionID, err := strconv.ParseUint(req.SectionID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid section ID")
	}

	err = h.profileService.DeleteSection(user.ID, uint(sectionID))
	if err != nil {
		logger.FromContext(ctx).Error("delete_section_failed",
			"user_id", user.ID,
			"section_id", sectionID,
			"error", err.Error(),
			"request_id", requestID,
		)
		if err.Error() == "profile section not found" {
			return nil, huma.Error404NotFound("Profile section not found")
		}
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to delete section (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Info("profile_section_deleted",
		"user_id", user.ID,
		"section_id", sectionID,
	)

	return &DeleteSectionResponse{
		Body: struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
		}{
			Success: true,
			Message: "Section deleted",
		},
	}, nil
}

// GetPercentileRankingsHandler handles GET /users/{username}/rankings.
func (h *ContributorProfileHandler) GetPercentileRankingsHandler(ctx context.Context, req *GetPercentileRankingsRequest) (*GetPercentileRankingsResponse, error) {
	requestID := logger.GetRequestID(ctx)

	targetUser, err := h.userService.GetUserByUsername(req.Username)
	if err != nil {
		logger.FromContext(ctx).Error("get_rankings_user_lookup_failed",
			"username", req.Username,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to look up user (request_id: %s)", requestID),
		)
	}
	if targetUser == nil {
		return nil, huma.Error404NotFound("User not found")
	}

	viewer := middleware.GetUserFromContext(ctx)
	isOwner := viewer != nil && viewer.ID == targetUser.ID

	// Master privacy check
	if targetUser.ProfileVisibility == "private" && !isOwner {
		return nil, huma.Error404NotFound("User not found")
	}

	// Granular privacy check for contributions (rankings are part of contribution data)
	if !isOwner {
		privacy := contracts.DefaultPrivacySettings()
		if targetUser.PrivacySettings != nil {
			_ = json.Unmarshal(*targetUser.PrivacySettings, &privacy)
		}

		switch privacy.Contributions {
		case contracts.PrivacyHidden:
			return nil, huma.Error404NotFound("User not found")
		case contracts.PrivacyCountOnly:
			// For count_only, return just the overall score without individual rankings
			rankings, err := h.profileService.GetPercentileRankings(targetUser.ID)
			if err != nil {
				logger.FromContext(ctx).Error("get_rankings_failed",
					"user_id", targetUser.ID,
					"error", err.Error(),
					"request_id", requestID,
				)
				return nil, huma.Error500InternalServerError(
					fmt.Sprintf("Failed to get rankings (request_id: %s)", requestID),
				)
			}
			if rankings == nil {
				return nil, huma.Error404NotFound("Rankings not available")
			}
			return &GetPercentileRankingsResponse{
				Body: &contracts.PercentileRankings{
					Rankings:     []contracts.PercentileRanking{},
					OverallScore: rankings.OverallScore,
				},
			}, nil
		}
	}

	rankings, err := h.profileService.GetPercentileRankings(targetUser.ID)
	if err != nil {
		logger.FromContext(ctx).Error("get_rankings_failed",
			"user_id", targetUser.ID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to get rankings (request_id: %s)", requestID),
		)
	}

	if rankings == nil {
		return nil, huma.Error404NotFound("Rankings not available")
	}

	return &GetPercentileRankingsResponse{Body: rankings}, nil
}
