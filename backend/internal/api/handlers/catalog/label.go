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

type LabelHandler struct {
	labelService    contracts.LabelServiceInterface
	auditLogService contracts.AuditLogServiceInterface
	revisionService contracts.RevisionServiceInterface
}

func NewLabelHandler(labelService contracts.LabelServiceInterface, auditLogService contracts.AuditLogServiceInterface, revisionService contracts.RevisionServiceInterface) *LabelHandler {
	return &LabelHandler{
		labelService:    labelService,
		auditLogService: auditLogService,
		revisionService: revisionService,
	}
}

// ============================================================================
// Search Labels
// ============================================================================

// SearchLabelsRequest represents the autocomplete search request
type SearchLabelsRequest struct {
	Query string `query:"q" doc:"Search query for label autocomplete" example:"sub pop"`
}

// SearchLabelsResponse represents the autocomplete search response
type SearchLabelsResponse struct {
	Body struct {
		Labels []*contracts.LabelListResponse `json:"labels" doc:"Matching labels"`
		Count  int                           `json:"count" doc:"Number of results"`
	}
}

// SearchLabelsHandler handles GET /labels/search?q=query
func (h *LabelHandler) SearchLabelsHandler(ctx context.Context, req *SearchLabelsRequest) (*SearchLabelsResponse, error) {
	labels, err := h.labelService.SearchLabels(req.Query)
	if err != nil {
		return nil, err
	}

	resp := &SearchLabelsResponse{}
	resp.Body.Labels = labels
	resp.Body.Count = len(labels)

	return resp, nil
}

// ============================================================================
// List Labels
// ============================================================================

// ListLabelsRequest represents the request for listing labels
type ListLabelsRequest struct {
	Status   string `query:"status" required:"false" doc:"Filter by status (active, inactive, defunct)" example:"active"`
	City     string `query:"city" required:"false" doc:"Filter by city" example:"Phoenix"`
	State    string `query:"state" required:"false" doc:"Filter by state" example:"AZ"`
	Tags     string `query:"tags" required:"false" doc:"Comma-separated tag slugs. Multi-tag filter (PSY-309): AND by default; set tag_match=any for OR." example:"indie,punk"`
	TagMatch string `query:"tag_match" required:"false" doc:"Tag matching mode: 'all' (default, AND) or 'any' (OR)" example:"all" enum:"all,any"`
}

// ListLabelsResponse represents the response for listing labels
type ListLabelsResponse struct {
	Body struct {
		Labels []*contracts.LabelListResponse `json:"labels" doc:"List of labels"`
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
	if tf := parseTagFilter(req.Tags, req.TagMatch); tf.HasTags() {
		filters["tag_filter"] = tf
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
	Body *contracts.LabelDetailResponse
}

// GetLabelHandler handles GET /labels/{label_id}
func (h *LabelHandler) GetLabelHandler(ctx context.Context, req *GetLabelRequest) (*GetLabelResponse, error) {
	var label *contracts.LabelDetailResponse
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
	Body *contracts.LabelDetailResponse
}

// CreateLabelHandler handles POST /labels
func (h *LabelHandler) CreateLabelHandler(ctx context.Context, req *CreateLabelRequest) (*CreateLabelResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	if req.Body.Name == "" {
		return nil, huma.Error400BadRequest("Name is required")
	}

	serviceReq := &contracts.CreateLabelRequest{
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
		Summary     *string `json:"summary,omitempty" required:"false" doc:"Revision summary describing the change"`
	}
}

// UpdateLabelResponse represents the response for updating a label
type UpdateLabelResponse struct {
	Body *contracts.LabelDetailResponse
}

// UpdateLabelHandler handles PUT /labels/{label_id}
func (h *LabelHandler) UpdateLabelHandler(ctx context.Context, req *UpdateLabelRequest) (*UpdateLabelResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	// Resolve label ID
	labelID, err := h.resolveLabelID(req.LabelID)
	if err != nil {
		return nil, err
	}

	// Capture old values for revision diff (fire-and-forget safe)
	var oldLabel *contracts.LabelDetailResponse
	if h.revisionService != nil {
		oldLabel, _ = h.labelService.GetLabel(labelID)
	}

	serviceReq := &contracts.UpdateLabelRequest{
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

	// Record revision (fire and forget)
	if h.revisionService != nil && oldLabel != nil {
		go func() {
			changes := computeLabelChanges(oldLabel, label)
			if len(changes) > 0 {
				summary := ""
				if req.Body.Summary != nil {
					summary = *req.Body.Summary
				}
				if err := h.revisionService.RecordRevision("label", labelID, user.ID, changes, summary); err != nil {
					logger.Default().Error("record_label_revision_failed",
						"label_id", labelID,
						"error", err.Error(),
					)
				}
			}
		}()
	}

	logger.FromContext(ctx).Info("label_updated",
		"label_id", labelID,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	return &UpdateLabelResponse{Body: label}, nil
}

// computeLabelChanges compares old and new label detail responses and returns field-level diffs.
func computeLabelChanges(old, new *contracts.LabelDetailResponse) []models.FieldChange {
	var changes []models.FieldChange

	if old.Name != new.Name {
		changes = append(changes, models.FieldChange{Field: "name", OldValue: old.Name, NewValue: new.Name})
	}
	if ptrToStr(old.City) != ptrToStr(new.City) {
		changes = append(changes, models.FieldChange{Field: "city", OldValue: ptrToStr(old.City), NewValue: ptrToStr(new.City)})
	}
	if ptrToStr(old.State) != ptrToStr(new.State) {
		changes = append(changes, models.FieldChange{Field: "state", OldValue: ptrToStr(old.State), NewValue: ptrToStr(new.State)})
	}
	if ptrToStr(old.Country) != ptrToStr(new.Country) {
		changes = append(changes, models.FieldChange{Field: "country", OldValue: ptrToStr(old.Country), NewValue: ptrToStr(new.Country)})
	}
	if !intPtrEq(old.FoundedYear, new.FoundedYear) {
		changes = append(changes, models.FieldChange{Field: "founded_year", OldValue: intPtrVal(old.FoundedYear), NewValue: intPtrVal(new.FoundedYear)})
	}
	if old.Status != new.Status {
		changes = append(changes, models.FieldChange{Field: "status", OldValue: old.Status, NewValue: new.Status})
	}
	if ptrToStr(old.Description) != ptrToStr(new.Description) {
		changes = append(changes, models.FieldChange{Field: "description", OldValue: ptrToStr(old.Description), NewValue: ptrToStr(new.Description)})
	}
	if ptrToStr(old.Social.Instagram) != ptrToStr(new.Social.Instagram) {
		changes = append(changes, models.FieldChange{Field: "instagram", OldValue: ptrToStr(old.Social.Instagram), NewValue: ptrToStr(new.Social.Instagram)})
	}
	if ptrToStr(old.Social.Facebook) != ptrToStr(new.Social.Facebook) {
		changes = append(changes, models.FieldChange{Field: "facebook", OldValue: ptrToStr(old.Social.Facebook), NewValue: ptrToStr(new.Social.Facebook)})
	}
	if ptrToStr(old.Social.Twitter) != ptrToStr(new.Social.Twitter) {
		changes = append(changes, models.FieldChange{Field: "twitter", OldValue: ptrToStr(old.Social.Twitter), NewValue: ptrToStr(new.Social.Twitter)})
	}
	if ptrToStr(old.Social.YouTube) != ptrToStr(new.Social.YouTube) {
		changes = append(changes, models.FieldChange{Field: "youtube", OldValue: ptrToStr(old.Social.YouTube), NewValue: ptrToStr(new.Social.YouTube)})
	}
	if ptrToStr(old.Social.Spotify) != ptrToStr(new.Social.Spotify) {
		changes = append(changes, models.FieldChange{Field: "spotify", OldValue: ptrToStr(old.Social.Spotify), NewValue: ptrToStr(new.Social.Spotify)})
	}
	if ptrToStr(old.Social.SoundCloud) != ptrToStr(new.Social.SoundCloud) {
		changes = append(changes, models.FieldChange{Field: "soundcloud", OldValue: ptrToStr(old.Social.SoundCloud), NewValue: ptrToStr(new.Social.SoundCloud)})
	}
	if ptrToStr(old.Social.Bandcamp) != ptrToStr(new.Social.Bandcamp) {
		changes = append(changes, models.FieldChange{Field: "bandcamp", OldValue: ptrToStr(old.Social.Bandcamp), NewValue: ptrToStr(new.Social.Bandcamp)})
	}
	if ptrToStr(old.Social.Website) != ptrToStr(new.Social.Website) {
		changes = append(changes, models.FieldChange{Field: "website", OldValue: ptrToStr(old.Social.Website), NewValue: ptrToStr(new.Social.Website)})
	}

	return changes
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

	user, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
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
		Artists []*contracts.LabelArtistResponse `json:"artists" doc:"List of artists"`
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
		Releases []*contracts.LabelReleaseResponse `json:"releases" doc:"List of releases"`
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
// Add Artist to Label
// ============================================================================

// AddArtistToLabelRequest represents the request for linking an artist to a label
type AddArtistToLabelRequest struct {
	LabelID string `path:"label_id" doc:"Label ID or slug" example:"sub-pop"`
	Body    struct {
		ArtistID uint `json:"artist_id" doc:"Artist ID to link" example:"42"`
	}
}

// AddArtistToLabelResponse represents the response for linking an artist to a label
type AddArtistToLabelResponse struct {
	Body struct {
		Success bool `json:"success" doc:"Whether the link was created"`
	}
}

// AddArtistToLabelHandler handles POST /admin/labels/{label_id}/artists
func (h *LabelHandler) AddArtistToLabelHandler(ctx context.Context, req *AddArtistToLabelRequest) (*AddArtistToLabelResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	// Resolve label ID
	labelID, err := h.resolveLabelID(req.LabelID)
	if err != nil {
		return nil, err
	}

	if req.Body.ArtistID == 0 {
		return nil, huma.Error400BadRequest("artist_id is required")
	}

	err = h.labelService.AddArtistToLabel(labelID, req.Body.ArtistID)
	if err != nil {
		var labelErr *apperrors.LabelError
		if errors.As(err, &labelErr) && labelErr.Code == apperrors.CodeLabelNotFound {
			return nil, huma.Error404NotFound("Label not found")
		}
		logger.FromContext(ctx).Error("add_artist_to_label_failed",
			"label_id", labelID,
			"artist_id", req.Body.ArtistID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to link artist to label (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		go func() {
			h.auditLogService.LogAction(user.ID, "add_artist_to_label", "label", labelID, nil)
		}()
	}

	logger.FromContext(ctx).Info("artist_added_to_label",
		"label_id", labelID,
		"artist_id", req.Body.ArtistID,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	resp := &AddArtistToLabelResponse{}
	resp.Body.Success = true
	return resp, nil
}

// ============================================================================
// Add Release to Label
// ============================================================================

// AddReleaseToLabelRequest represents the request for linking a release to a label
type AddReleaseToLabelRequest struct {
	LabelID string `path:"label_id" doc:"Label ID or slug" example:"sub-pop"`
	Body    struct {
		ReleaseID     uint    `json:"release_id" doc:"Release ID to link" example:"42"`
		CatalogNumber *string `json:"catalog_number,omitempty" required:"false" doc:"Catalog number for this release on this label"`
	}
}

// AddReleaseToLabelResponse represents the response for linking a release to a label
type AddReleaseToLabelResponse struct {
	Body struct {
		Success bool `json:"success" doc:"Whether the link was created"`
	}
}

// AddReleaseToLabelHandler handles POST /admin/labels/{label_id}/releases
func (h *LabelHandler) AddReleaseToLabelHandler(ctx context.Context, req *AddReleaseToLabelRequest) (*AddReleaseToLabelResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	// Resolve label ID
	labelID, err := h.resolveLabelID(req.LabelID)
	if err != nil {
		return nil, err
	}

	if req.Body.ReleaseID == 0 {
		return nil, huma.Error400BadRequest("release_id is required")
	}

	err = h.labelService.AddReleaseToLabel(labelID, req.Body.ReleaseID, req.Body.CatalogNumber)
	if err != nil {
		var labelErr *apperrors.LabelError
		if errors.As(err, &labelErr) && labelErr.Code == apperrors.CodeLabelNotFound {
			return nil, huma.Error404NotFound("Label not found")
		}
		logger.FromContext(ctx).Error("add_release_to_label_failed",
			"label_id", labelID,
			"release_id", req.Body.ReleaseID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to link release to label (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		go func() {
			h.auditLogService.LogAction(user.ID, "add_release_to_label", "label", labelID, nil)
		}()
	}

	logger.FromContext(ctx).Info("release_added_to_label",
		"label_id", labelID,
		"release_id", req.Body.ReleaseID,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	resp := &AddReleaseToLabelResponse{}
	resp.Body.Success = true
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
