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

type ReleaseHandler struct {
	releaseService  services.ReleaseServiceInterface
	artistService   services.ArtistServiceInterface
	auditLogService services.AuditLogServiceInterface
}

func NewReleaseHandler(releaseService services.ReleaseServiceInterface, artistService services.ArtistServiceInterface, auditLogService services.AuditLogServiceInterface) *ReleaseHandler {
	return &ReleaseHandler{
		releaseService:  releaseService,
		artistService:   artistService,
		auditLogService: auditLogService,
	}
}

// ============================================================================
// Search Releases
// ============================================================================

// SearchReleasesRequest represents the autocomplete search request
type SearchReleasesRequest struct {
	Query string `query:"q" doc:"Search query for release autocomplete" example:"nevermind"`
}

// SearchReleasesResponse represents the autocomplete search response
type SearchReleasesResponse struct {
	Body struct {
		Releases []*services.ReleaseListResponse `json:"releases" doc:"Matching releases"`
		Count    int                              `json:"count" doc:"Number of results"`
	}
}

// SearchReleasesHandler handles GET /releases/search?q=query
func (h *ReleaseHandler) SearchReleasesHandler(ctx context.Context, req *SearchReleasesRequest) (*SearchReleasesResponse, error) {
	releases, err := h.releaseService.SearchReleases(req.Query)
	if err != nil {
		return nil, err
	}

	resp := &SearchReleasesResponse{}
	resp.Body.Releases = releases
	resp.Body.Count = len(releases)

	return resp, nil
}

// ============================================================================
// List Releases
// ============================================================================

// ListReleasesRequest represents the request for listing releases
type ListReleasesRequest struct {
	ArtistID    uint   `query:"artist_id" required:"false" doc:"Filter by artist ID" example:"1"`
	ReleaseType string `query:"release_type" required:"false" doc:"Filter by release type" example:"lp"`
	Year        int    `query:"year" required:"false" doc:"Filter by release year" example:"2024"`
}

// ListReleasesResponse represents the response for listing releases
type ListReleasesResponse struct {
	Body struct {
		Releases []*services.ReleaseListResponse `json:"releases" doc:"List of releases"`
		Count    int                              `json:"count" doc:"Number of releases"`
	}
}

// ListReleasesHandler handles GET /releases
func (h *ReleaseHandler) ListReleasesHandler(ctx context.Context, req *ListReleasesRequest) (*ListReleasesResponse, error) {
	filters := make(map[string]interface{})

	if req.ArtistID > 0 {
		filters["artist_id"] = req.ArtistID
	}
	if req.ReleaseType != "" {
		filters["release_type"] = req.ReleaseType
	}
	if req.Year > 0 {
		filters["year"] = req.Year
	}

	releases, err := h.releaseService.ListReleases(filters)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to fetch releases", err)
	}

	resp := &ListReleasesResponse{}
	resp.Body.Releases = releases
	resp.Body.Count = len(releases)

	return resp, nil
}

// ============================================================================
// Get Release
// ============================================================================

// GetReleaseRequest represents the request for getting a single release
type GetReleaseRequest struct {
	ReleaseID string `path:"release_id" doc:"Release ID or slug" example:"nevermind"`
}

// GetReleaseResponse represents the response for the get release endpoint
type GetReleaseResponse struct {
	Body *services.ReleaseDetailResponse
}

// GetReleaseHandler handles GET /releases/{release_id}
func (h *ReleaseHandler) GetReleaseHandler(ctx context.Context, req *GetReleaseRequest) (*GetReleaseResponse, error) {
	var release *services.ReleaseDetailResponse
	var err error

	// Try to parse as numeric ID first
	if id, parseErr := strconv.ParseUint(req.ReleaseID, 10, 32); parseErr == nil {
		release, err = h.releaseService.GetRelease(uint(id))
	} else {
		// Fall back to slug lookup
		release, err = h.releaseService.GetReleaseBySlug(req.ReleaseID)
	}

	if err != nil {
		var releaseErr *apperrors.ReleaseError
		if errors.As(err, &releaseErr) && releaseErr.Code == apperrors.CodeReleaseNotFound {
			return nil, huma.Error404NotFound("Release not found")
		}
		return nil, huma.Error500InternalServerError("Failed to fetch release", err)
	}

	return &GetReleaseResponse{Body: release}, nil
}

// ============================================================================
// Create Release
// ============================================================================

// CreateReleaseRequest represents the request for creating a release
type CreateReleaseRequest struct {
	Body struct {
		Title       string `json:"title" doc:"Release title" example:"Nevermind"`
		ReleaseType string `json:"release_type,omitempty" required:"false" doc:"Release type (lp, ep, single, compilation, live, remix, demo)" example:"lp"`
		ReleaseYear *int   `json:"release_year,omitempty" required:"false" doc:"Release year" example:"1991"`
		ReleaseDate *string `json:"release_date,omitempty" required:"false" doc:"Release date (YYYY-MM-DD)" example:"1991-09-24"`
		CoverArtURL *string `json:"cover_art_url,omitempty" required:"false" doc:"Cover art URL"`
		Description *string `json:"description,omitempty" required:"false" doc:"Description"`
		Artists     []CreateReleaseArtistInput `json:"artists,omitempty" required:"false" doc:"Artists with roles"`
		ExternalLinks []CreateReleaseLinkInput `json:"external_links,omitempty" required:"false" doc:"External links (Bandcamp, Spotify, etc.)"`
	}
}

// CreateReleaseArtistInput represents artist input for release creation
type CreateReleaseArtistInput struct {
	ArtistID uint   `json:"artist_id" doc:"Artist ID"`
	Role     string `json:"role,omitempty" required:"false" doc:"Role (main, featured, producer, remixer, composer, dj)" example:"main"`
}

// CreateReleaseLinkInput represents external link input for release creation
type CreateReleaseLinkInput struct {
	Platform string `json:"platform" doc:"Platform name (bandcamp, spotify, discogs, etc.)" example:"bandcamp"`
	URL      string `json:"url" doc:"URL to the release on the platform"`
}

// CreateReleaseResponse represents the response for creating a release
type CreateReleaseResponse struct {
	Body *services.ReleaseDetailResponse
}

// CreateReleaseHandler handles POST /releases
func (h *ReleaseHandler) CreateReleaseHandler(ctx context.Context, req *CreateReleaseRequest) (*CreateReleaseResponse, error) {
	requestID := logger.GetRequestID(ctx)

	// Verify admin access
	user := middleware.GetUserFromContext(ctx)
	if user == nil || !user.IsAdmin {
		return nil, huma.Error403Forbidden("Admin access required")
	}

	if req.Body.Title == "" {
		return nil, huma.Error400BadRequest("Title is required")
	}

	// Convert handler types to service types
	artists := make([]services.CreateReleaseArtistEntry, len(req.Body.Artists))
	for i, a := range req.Body.Artists {
		artists[i] = services.CreateReleaseArtistEntry{
			ArtistID: a.ArtistID,
			Role:     a.Role,
		}
	}
	links := make([]services.CreateReleaseLinkEntry, len(req.Body.ExternalLinks))
	for i, l := range req.Body.ExternalLinks {
		links[i] = services.CreateReleaseLinkEntry{
			Platform: l.Platform,
			URL:      l.URL,
		}
	}

	serviceReq := &services.CreateReleaseRequest{
		Title:         req.Body.Title,
		ReleaseType:   req.Body.ReleaseType,
		ReleaseYear:   req.Body.ReleaseYear,
		ReleaseDate:   req.Body.ReleaseDate,
		CoverArtURL:   req.Body.CoverArtURL,
		Description:   req.Body.Description,
		Artists:       artists,
		ExternalLinks: links,
	}

	release, err := h.releaseService.CreateRelease(serviceReq)
	if err != nil {
		logger.FromContext(ctx).Error("create_release_failed",
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to create release (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		go func() {
			h.auditLogService.LogAction(user.ID, "create_release", "release", release.ID, nil)
		}()
	}

	logger.FromContext(ctx).Info("release_created",
		"release_id", release.ID,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	return &CreateReleaseResponse{Body: release}, nil
}

// ============================================================================
// Update Release
// ============================================================================

// UpdateReleaseRequest represents the request for updating a release
type UpdateReleaseRequest struct {
	ReleaseID string `path:"release_id" doc:"Release ID or slug" example:"1"`
	Body      struct {
		Title       *string `json:"title,omitempty" required:"false" doc:"Release title"`
		ReleaseType *string `json:"release_type,omitempty" required:"false" doc:"Release type"`
		ReleaseYear *int    `json:"release_year,omitempty" required:"false" doc:"Release year"`
		ReleaseDate *string `json:"release_date,omitempty" required:"false" doc:"Release date (YYYY-MM-DD)"`
		CoverArtURL *string `json:"cover_art_url,omitempty" required:"false" doc:"Cover art URL"`
		Description *string `json:"description,omitempty" required:"false" doc:"Description"`
	}
}

// UpdateReleaseResponse represents the response for updating a release
type UpdateReleaseResponse struct {
	Body *services.ReleaseDetailResponse
}

// UpdateReleaseHandler handles PUT /releases/{release_id}
func (h *ReleaseHandler) UpdateReleaseHandler(ctx context.Context, req *UpdateReleaseRequest) (*UpdateReleaseResponse, error) {
	requestID := logger.GetRequestID(ctx)

	// Verify admin access
	user := middleware.GetUserFromContext(ctx)
	if user == nil || !user.IsAdmin {
		return nil, huma.Error403Forbidden("Admin access required")
	}

	// Resolve release ID
	releaseID, err := h.resolveReleaseID(req.ReleaseID)
	if err != nil {
		return nil, err
	}

	serviceReq := &services.UpdateReleaseRequest{
		Title:       req.Body.Title,
		ReleaseType: req.Body.ReleaseType,
		ReleaseYear: req.Body.ReleaseYear,
		ReleaseDate: req.Body.ReleaseDate,
		CoverArtURL: req.Body.CoverArtURL,
		Description: req.Body.Description,
	}

	release, err := h.releaseService.UpdateRelease(releaseID, serviceReq)
	if err != nil {
		var releaseErr *apperrors.ReleaseError
		if errors.As(err, &releaseErr) && releaseErr.Code == apperrors.CodeReleaseNotFound {
			return nil, huma.Error404NotFound("Release not found")
		}
		logger.FromContext(ctx).Error("update_release_failed",
			"release_id", releaseID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to update release (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		go func() {
			h.auditLogService.LogAction(user.ID, "edit_release", "release", releaseID, nil)
		}()
	}

	logger.FromContext(ctx).Info("release_updated",
		"release_id", releaseID,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	return &UpdateReleaseResponse{Body: release}, nil
}

// ============================================================================
// Delete Release
// ============================================================================

// DeleteReleaseRequest represents the request for deleting a release
type DeleteReleaseRequest struct {
	ReleaseID string `path:"release_id" doc:"Release ID" example:"1"`
}

// DeleteReleaseHandler handles DELETE /releases/{release_id}
func (h *ReleaseHandler) DeleteReleaseHandler(ctx context.Context, req *DeleteReleaseRequest) (*struct{}, error) {
	requestID := logger.GetRequestID(ctx)

	// Verify admin access
	user := middleware.GetUserFromContext(ctx)
	if user == nil || !user.IsAdmin {
		return nil, huma.Error403Forbidden("Admin access required")
	}

	// Resolve release ID
	releaseID, err := h.resolveReleaseID(req.ReleaseID)
	if err != nil {
		return nil, err
	}

	err = h.releaseService.DeleteRelease(releaseID)
	if err != nil {
		var releaseErr *apperrors.ReleaseError
		if errors.As(err, &releaseErr) && releaseErr.Code == apperrors.CodeReleaseNotFound {
			return nil, huma.Error404NotFound("Release not found")
		}
		logger.FromContext(ctx).Error("delete_release_failed",
			"release_id", releaseID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to delete release (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		go func() {
			h.auditLogService.LogAction(user.ID, "delete_release", "release", releaseID, nil)
		}()
	}

	logger.FromContext(ctx).Info("release_deleted",
		"release_id", releaseID,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	return nil, nil
}

// ============================================================================
// Artist Releases (Discography)
// ============================================================================

// GetArtistReleasesRequest represents the request for getting an artist's releases
type GetArtistReleasesRequest struct {
	ArtistID string `path:"artist_id" doc:"Artist ID or slug" example:"nirvana"`
}

// GetArtistReleasesResponse represents the response for the artist releases endpoint
type GetArtistReleasesResponse struct {
	Body struct {
		Releases []*services.ArtistReleaseListResponse `json:"releases" doc:"List of releases with artist roles"`
		Count    int                                    `json:"count" doc:"Number of releases"`
	}
}

// GetArtistReleasesHandler handles GET /artists/{artist_id}/releases
func (h *ReleaseHandler) GetArtistReleasesHandler(ctx context.Context, req *GetArtistReleasesRequest) (*GetArtistReleasesResponse, error) {
	// Resolve artist ID from numeric ID or slug
	var artistID uint
	if id, parseErr := strconv.ParseUint(req.ArtistID, 10, 32); parseErr == nil {
		artistID = uint(id)
	} else {
		// Look up by slug to get the ID
		artist, err := h.artistService.GetArtistBySlug(req.ArtistID)
		if err != nil {
			var artistErr *apperrors.ArtistError
			if errors.As(err, &artistErr) && artistErr.Code == apperrors.CodeArtistNotFound {
				return nil, huma.Error404NotFound("Artist not found")
			}
			return nil, huma.Error500InternalServerError("Failed to fetch artist", err)
		}
		artistID = artist.ID
	}

	releases, err := h.releaseService.GetReleasesForArtistWithRoles(artistID)
	if err != nil {
		var artistErr *apperrors.ArtistError
		if errors.As(err, &artistErr) && artistErr.Code == apperrors.CodeArtistNotFound {
			return nil, huma.Error404NotFound("Artist not found")
		}
		return nil, huma.Error500InternalServerError("Failed to fetch releases", err)
	}

	resp := &GetArtistReleasesResponse{}
	resp.Body.Releases = releases
	resp.Body.Count = len(releases)

	return resp, nil
}

// ============================================================================
// External Links
// ============================================================================

// AddExternalLinkRequest represents the request for adding an external link
type AddExternalLinkRequest struct {
	ReleaseID string `path:"release_id" doc:"Release ID" example:"1"`
	Body      struct {
		Platform string `json:"platform" doc:"Platform name (bandcamp, spotify, etc.)" example:"bandcamp"`
		URL      string `json:"url" doc:"URL to the release"`
	}
}

// AddExternalLinkResponse represents the response for adding an external link
type AddExternalLinkResponse struct {
	Body *services.ReleaseExternalLinkResponse
}

// AddExternalLinkHandler handles POST /releases/{release_id}/links
func (h *ReleaseHandler) AddExternalLinkHandler(ctx context.Context, req *AddExternalLinkRequest) (*AddExternalLinkResponse, error) {
	requestID := logger.GetRequestID(ctx)

	// Verify admin access
	user := middleware.GetUserFromContext(ctx)
	if user == nil || !user.IsAdmin {
		return nil, huma.Error403Forbidden("Admin access required")
	}

	releaseID, err := strconv.ParseUint(req.ReleaseID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid release ID")
	}

	if req.Body.Platform == "" || req.Body.URL == "" {
		return nil, huma.Error400BadRequest("Platform and URL are required")
	}

	link, err := h.releaseService.AddExternalLink(uint(releaseID), req.Body.Platform, req.Body.URL)
	if err != nil {
		var releaseErr *apperrors.ReleaseError
		if errors.As(err, &releaseErr) && releaseErr.Code == apperrors.CodeReleaseNotFound {
			return nil, huma.Error404NotFound("Release not found")
		}
		logger.FromContext(ctx).Error("add_external_link_failed",
			"release_id", releaseID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to add external link (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		go func() {
			h.auditLogService.LogAction(user.ID, "add_release_link", "release", uint(releaseID), nil)
		}()
	}

	return &AddExternalLinkResponse{Body: link}, nil
}

// RemoveExternalLinkRequest represents the request for removing an external link
type RemoveExternalLinkRequest struct {
	ReleaseID string `path:"release_id" doc:"Release ID" example:"1"`
	LinkID    string `path:"link_id" doc:"Link ID" example:"1"`
}

// RemoveExternalLinkHandler handles DELETE /releases/{release_id}/links/{link_id}
func (h *ReleaseHandler) RemoveExternalLinkHandler(ctx context.Context, req *RemoveExternalLinkRequest) (*struct{}, error) {
	requestID := logger.GetRequestID(ctx)

	// Verify admin access
	user := middleware.GetUserFromContext(ctx)
	if user == nil || !user.IsAdmin {
		return nil, huma.Error403Forbidden("Admin access required")
	}

	linkID, err := strconv.ParseUint(req.LinkID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid link ID")
	}

	err = h.releaseService.RemoveExternalLink(uint(linkID))
	if err != nil {
		logger.FromContext(ctx).Error("remove_external_link_failed",
			"link_id", linkID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to remove external link (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		releaseID, _ := strconv.ParseUint(req.ReleaseID, 10, 32)
		go func() {
			h.auditLogService.LogAction(user.ID, "remove_release_link", "release", uint(releaseID), nil)
		}()
	}

	return nil, nil
}

// ============================================================================
// Helpers
// ============================================================================

// resolveReleaseID tries to parse the ID as a number first, then falls back to slug lookup
func (h *ReleaseHandler) resolveReleaseID(idOrSlug string) (uint, error) {
	if id, parseErr := strconv.ParseUint(idOrSlug, 10, 32); parseErr == nil {
		return uint(id), nil
	}

	// Fall back to slug lookup
	release, err := h.releaseService.GetReleaseBySlug(idOrSlug)
	if err != nil {
		var releaseErr *apperrors.ReleaseError
		if errors.As(err, &releaseErr) && releaseErr.Code == apperrors.CodeReleaseNotFound {
			return 0, huma.Error404NotFound("Release not found")
		}
		return 0, huma.Error500InternalServerError("Failed to fetch release", err)
	}

	return release.ID, nil
}
