package engagement

import (
	"context"
	"fmt"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/contracts"
)

// SavedReleaseHandler exposes release bookmarks as the user-facing Save/Saved
// relationship. Counts are public aggregates; the user's own state is only
// populated by optional auth.
type SavedReleaseHandler struct {
	service contracts.SavedReleaseServiceInterface
}

func NewSavedReleaseHandler(service contracts.SavedReleaseServiceInterface) *SavedReleaseHandler {
	return &SavedReleaseHandler{service: service}
}

type SaveReleaseRequest struct {
	ReleaseID string `path:"release_id" validate:"required" doc:"Release ID"`
}

type SaveReleaseResponse struct {
	Body struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
}

type GetSavedReleasesRequest struct {
	Limit  int `query:"limit" default:"50" minimum:"1" maximum:"100" doc:"Number of releases per page"`
	Offset int `query:"offset" default:"0" minimum:"0" doc:"Offset for pagination"`
}

type GetSavedReleasesResponse struct {
	Body struct {
		Releases []*contracts.SavedReleaseResponse `json:"releases"`
		Total    int64                             `json:"total"`
		Limit    int                               `json:"limit"`
		Offset   int                               `json:"offset"`
	}
}

type GetReleaseSaveCountRequest struct {
	ReleaseID string `path:"release_id" validate:"required" doc:"Release ID"`
}

type GetReleaseSaveCountResponse struct {
	Body struct {
		ReleaseID uint `json:"release_id"`
		SaveCount int  `json:"save_count"`
		IsSaved   bool `json:"is_saved"`
	}
}

type BatchReleaseSaveCountsRequest struct {
	Body struct {
		ReleaseIDs []int `json:"release_ids" doc:"List of release IDs (max 200)"`
	}
}

type BatchReleaseSaveCountsEntry struct {
	SaveCount int  `json:"save_count"`
	IsSaved   bool `json:"is_saved"`
}

type BatchReleaseSaveCountsResponse struct {
	Body struct {
		Saves map[string]*BatchReleaseSaveCountsEntry `json:"saves"`
	}
}

func parseReleaseID(raw string) (uint, error) {
	id, err := strconv.ParseUint(raw, 10, 32)
	if err != nil || id == 0 {
		return 0, huma.Error400BadRequest("Invalid release ID")
	}
	return uint(id), nil
}

func (h *SavedReleaseHandler) SaveReleaseHandler(ctx context.Context, req *SaveReleaseRequest) (*SaveReleaseResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}
	releaseID, err := parseReleaseID(req.ReleaseID)
	if err != nil {
		return nil, err
	}
	if err := h.service.SaveRelease(user.ID, releaseID); err != nil {
		requestID := logger.GetRequestID(ctx)
		logger.FromContext(ctx).Error("save_release_failed", "user_id", user.ID, "release_id", releaseID, "error", err.Error())
		return nil, huma.Error422UnprocessableEntity(fmt.Sprintf("Failed to save release (request_id: %s)", requestID))
	}
	resp := &SaveReleaseResponse{}
	resp.Body.Success = true
	resp.Body.Message = "Release saved successfully"
	return resp, nil
}

func (h *SavedReleaseHandler) UnsaveReleaseHandler(ctx context.Context, req *SaveReleaseRequest) (*SaveReleaseResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}
	releaseID, err := parseReleaseID(req.ReleaseID)
	if err != nil {
		return nil, err
	}
	if err := h.service.UnsaveRelease(user.ID, releaseID); err != nil {
		requestID := logger.GetRequestID(ctx)
		logger.FromContext(ctx).Error("unsave_release_failed", "user_id", user.ID, "release_id", releaseID, "error", err.Error())
		return nil, huma.Error422UnprocessableEntity(fmt.Sprintf("Failed to unsave release (request_id: %s)", requestID))
	}
	resp := &SaveReleaseResponse{}
	resp.Body.Success = true
	resp.Body.Message = "Release unsaved successfully"
	return resp, nil
}

func (h *SavedReleaseHandler) GetSavedReleasesHandler(ctx context.Context, req *GetSavedReleasesRequest) (*GetSavedReleasesResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}
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
	releases, total, err := h.service.GetUserSavedReleases(user.ID, limit, offset)
	if err != nil {
		logger.FromContext(ctx).Error("get_saved_releases_failed", "user_id", user.ID, "error", err.Error())
		return nil, huma.Error500InternalServerError("Failed to get saved releases")
	}
	resp := &GetSavedReleasesResponse{}
	resp.Body.Releases = releases
	resp.Body.Total = total
	resp.Body.Limit = limit
	resp.Body.Offset = offset
	return resp, nil
}

func (h *SavedReleaseHandler) GetReleaseSaveCountHandler(ctx context.Context, req *GetReleaseSaveCountRequest) (*GetReleaseSaveCountResponse, error) {
	releaseID, err := parseReleaseID(req.ReleaseID)
	if err != nil {
		return nil, err
	}
	count, err := h.service.GetSaveCount(releaseID)
	if err != nil {
		logger.FromContext(ctx).Error("get_release_save_count_failed", "release_id", releaseID, "error", err.Error())
		return nil, huma.Error500InternalServerError("Failed to get release save count")
	}
	resp := &GetReleaseSaveCountResponse{}
	resp.Body.ReleaseID = releaseID
	resp.Body.SaveCount = count
	if user := middleware.GetUserFromContext(ctx); user != nil {
		isSaved, err := h.service.IsReleaseSaved(user.ID, releaseID)
		if err != nil {
			logger.FromContext(ctx).Warn("get_release_save_count_is_saved_failed", "user_id", user.ID, "release_id", releaseID, "error", err.Error())
		} else {
			resp.Body.IsSaved = isSaved
		}
	}
	return resp, nil
}

func (h *SavedReleaseHandler) BatchReleaseSaveCountsHandler(ctx context.Context, req *BatchReleaseSaveCountsRequest) (*BatchReleaseSaveCountsResponse, error) {
	resp := &BatchReleaseSaveCountsResponse{}
	resp.Body.Saves = make(map[string]*BatchReleaseSaveCountsEntry)
	if len(req.Body.ReleaseIDs) == 0 {
		return resp, nil
	}
	if len(req.Body.ReleaseIDs) > 200 {
		return nil, huma.Error400BadRequest("Maximum 200 release IDs allowed")
	}

	releaseIDs := make([]uint, len(req.Body.ReleaseIDs))
	for i, id := range req.Body.ReleaseIDs {
		if id <= 0 {
			return nil, huma.Error400BadRequest("Invalid release ID")
		}
		releaseIDs[i] = uint(id)
	}
	counts, err := h.service.GetBatchSaveCounts(releaseIDs)
	if err != nil {
		logger.FromContext(ctx).Error("batch_release_save_counts_failed", "count", len(releaseIDs), "error", err.Error())
		return nil, huma.Error500InternalServerError("Failed to get release save counts")
	}
	for releaseID, count := range counts {
		resp.Body.Saves[strconv.FormatUint(uint64(releaseID), 10)] = &BatchReleaseSaveCountsEntry{SaveCount: count}
	}
	if user := middleware.GetUserFromContext(ctx); user != nil {
		saved, err := h.service.GetSavedReleaseIDs(user.ID, releaseIDs)
		if err != nil {
			logger.FromContext(ctx).Warn("batch_release_save_counts_is_saved_failed", "user_id", user.ID, "count", len(releaseIDs), "error", err.Error())
		} else {
			for releaseID, isSaved := range saved {
				if entry := resp.Body.Saves[strconv.FormatUint(uint64(releaseID), 10)]; entry != nil {
					entry.IsSaved = isSaved
				}
			}
		}
	}
	return resp, nil
}
