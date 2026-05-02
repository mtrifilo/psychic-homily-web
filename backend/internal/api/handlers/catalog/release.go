package handlers

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
)

type ReleaseHandler struct {
	releaseService  contracts.ReleaseServiceInterface
	artistService   contracts.ArtistServiceInterface
	auditLogService contracts.AuditLogServiceInterface
	revisionService contracts.RevisionServiceInterface
}

func NewReleaseHandler(releaseService contracts.ReleaseServiceInterface, artistService contracts.ArtistServiceInterface, auditLogService contracts.AuditLogServiceInterface, revisionService contracts.RevisionServiceInterface) *ReleaseHandler {
	return &ReleaseHandler{
		releaseService:  releaseService,
		artistService:   artistService,
		auditLogService: auditLogService,
		revisionService: revisionService,
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
		Releases []*contracts.ReleaseListResponse `json:"releases" doc:"Matching releases"`
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
	Search      string `query:"search" required:"false" doc:"Search by release title or artist name" example:"nevermind"`
	Sort        string `query:"sort" required:"false" doc:"Sort order: newest, oldest, title_asc, title_desc, recently_added" example:"newest"`
	LabelID     uint   `query:"label_id" required:"false" doc:"Filter by label ID" example:"1"`
	Limit       int    `query:"limit" required:"false" doc:"Page size (default 50, max 200)" example:"50"`
	Offset      int    `query:"offset" required:"false" doc:"Pagination offset" example:"0"`
	Tags        string `query:"tags" required:"false" doc:"Comma-separated tag slugs. Multi-tag filter (PSY-309): AND by default; set tag_match=any for OR." example:"shoegaze,dream-pop"`
	TagMatch    string `query:"tag_match" required:"false" doc:"Tag matching mode: 'all' (default, AND) or 'any' (OR)" example:"all" enum:"all,any"`
}

// ListReleasesResponse represents the response for listing releases
type ListReleasesResponse struct {
	Body struct {
		Releases []*contracts.ReleaseListResponse `json:"releases" doc:"List of releases"`
		Total    int64                            `json:"total" doc:"Total number of matching releases"`
		Limit    int                              `json:"limit" doc:"Limit used in query"`
		Offset   int                              `json:"offset" doc:"Offset used in query"`
	}
}

// ListReleasesHandler handles GET /releases
func (h *ReleaseHandler) ListReleasesHandler(ctx context.Context, req *ListReleasesRequest) (*ListReleasesResponse, error) {
	filters := contracts.ReleaseListFilters{
		ArtistID:    req.ArtistID,
		ReleaseType: req.ReleaseType,
		Year:        req.Year,
		Search:      req.Search,
		Sort:        req.Sort,
		LabelID:     req.LabelID,
		Limit:       req.Limit,
		Offset:      req.Offset,
	}
	if tf := parseTagFilter(req.Tags, req.TagMatch); tf.HasTags() {
		filters.TagSlugs = tf.TagSlugs
		filters.TagMatchAny = tf.MatchAny
	}

	releases, total, err := h.releaseService.ListReleases(filters)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to fetch releases", err)
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}

	resp := &ListReleasesResponse{}
	resp.Body.Releases = releases
	resp.Body.Total = total
	resp.Body.Limit = limit
	resp.Body.Offset = req.Offset

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
	Body *contracts.ReleaseDetailResponse
}

// GetReleaseHandler handles GET /releases/{release_id}
func (h *ReleaseHandler) GetReleaseHandler(ctx context.Context, req *GetReleaseRequest) (*GetReleaseResponse, error) {
	var release *contracts.ReleaseDetailResponse
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
	Body *contracts.ReleaseDetailResponse
}

// CreateReleaseHandler handles POST /releases
func (h *ReleaseHandler) CreateReleaseHandler(ctx context.Context, req *CreateReleaseRequest) (*CreateReleaseResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	if req.Body.Title == "" {
		return nil, huma.Error400BadRequest("Title is required")
	}

	// Convert handler types to service types
	artists := make([]contracts.CreateReleaseArtistEntry, len(req.Body.Artists))
	for i, a := range req.Body.Artists {
		artists[i] = contracts.CreateReleaseArtistEntry{
			ArtistID: a.ArtistID,
			Role:     a.Role,
		}
	}
	links := make([]contracts.CreateReleaseLinkEntry, len(req.Body.ExternalLinks))
	for i, l := range req.Body.ExternalLinks {
		links[i] = contracts.CreateReleaseLinkEntry{
			Platform: l.Platform,
			URL:      l.URL,
		}
	}

	serviceReq := &contracts.CreateReleaseRequest{
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
		Summary     *string `json:"summary,omitempty" required:"false" doc:"Revision summary describing the change"`
	}
}

// UpdateReleaseResponse represents the response for updating a release
type UpdateReleaseResponse struct {
	Body *contracts.ReleaseDetailResponse
}

// UpdateReleaseHandler handles PUT /releases/{release_id}
func (h *ReleaseHandler) UpdateReleaseHandler(ctx context.Context, req *UpdateReleaseRequest) (*UpdateReleaseResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	// Resolve release ID
	releaseID, err := h.resolveReleaseID(req.ReleaseID)
	if err != nil {
		return nil, err
	}

	// Capture old values for revision diff (fire-and-forget safe)
	var oldRelease *contracts.ReleaseDetailResponse
	if h.revisionService != nil {
		oldRelease, _ = h.releaseService.GetRelease(releaseID)
	}

	serviceReq := &contracts.UpdateReleaseRequest{
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

	// Record revision (fire and forget)
	if h.revisionService != nil && oldRelease != nil {
		go func() {
			changes := computeReleaseChanges(oldRelease, release)
			if len(changes) > 0 {
				summary := ""
				if req.Body.Summary != nil {
					summary = *req.Body.Summary
				}
				if err := h.revisionService.RecordRevision("release", releaseID, user.ID, changes, summary); err != nil {
					logger.Default().Error("record_release_revision_failed",
						"release_id", releaseID,
						"error", err.Error(),
					)
				}
			}
		}()
	}

	logger.FromContext(ctx).Info("release_updated",
		"release_id", releaseID,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	return &UpdateReleaseResponse{Body: release}, nil
}

// computeReleaseChanges compares old and new release detail responses and returns field-level diffs.
func computeReleaseChanges(old, new *contracts.ReleaseDetailResponse) []models.FieldChange {
	var changes []models.FieldChange

	if old.Title != new.Title {
		changes = append(changes, models.FieldChange{Field: "title", OldValue: old.Title, NewValue: new.Title})
	}
	if old.ReleaseType != new.ReleaseType {
		changes = append(changes, models.FieldChange{Field: "release_type", OldValue: old.ReleaseType, NewValue: new.ReleaseType})
	}
	if !intPtrEq(old.ReleaseYear, new.ReleaseYear) {
		changes = append(changes, models.FieldChange{Field: "release_year", OldValue: intPtrVal(old.ReleaseYear), NewValue: intPtrVal(new.ReleaseYear)})
	}
	if ptrToStr(old.ReleaseDate) != ptrToStr(new.ReleaseDate) {
		changes = append(changes, models.FieldChange{Field: "release_date", OldValue: ptrToStr(old.ReleaseDate), NewValue: ptrToStr(new.ReleaseDate)})
	}
	if ptrToStr(old.CoverArtURL) != ptrToStr(new.CoverArtURL) {
		changes = append(changes, models.FieldChange{Field: "cover_art_url", OldValue: ptrToStr(old.CoverArtURL), NewValue: ptrToStr(new.CoverArtURL)})
	}
	if ptrToStr(old.Description) != ptrToStr(new.Description) {
		changes = append(changes, models.FieldChange{Field: "description", OldValue: ptrToStr(old.Description), NewValue: ptrToStr(new.Description)})
	}

	return changes
}

// intPtrEq returns true if two *int pointers refer to equal values (both nil is equal).
func intPtrEq(a, b *int) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

// intPtrVal returns the pointed-to int or 0 if nil.
func intPtrVal(a *int) int {
	if a == nil {
		return 0
	}
	return *a
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

	user, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
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
		Releases []*contracts.ArtistReleaseListResponse `json:"releases" doc:"List of releases with artist roles"`
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
	Body *contracts.ReleaseExternalLinkResponse
}

// AddExternalLinkHandler handles POST /releases/{release_id}/links
func (h *ReleaseHandler) AddExternalLinkHandler(ctx context.Context, req *AddExternalLinkRequest) (*AddExternalLinkResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
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

	user, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
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
