package admin

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
	adminm "psychic-homily-backend/internal/models/admin"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// PendingEditHandler handles pending entity edit API endpoints.
type PendingEditHandler struct {
	pendingEditService contracts.PendingEditServiceInterface
	auditLogService    contracts.AuditLogServiceInterface
}

// NewPendingEditHandler creates a new pending edit handler.
func NewPendingEditHandler(
	pendingEditService contracts.PendingEditServiceInterface,
	auditLogService contracts.AuditLogServiceInterface,
) *PendingEditHandler {
	return &PendingEditHandler{
		pendingEditService: pendingEditService,
		auditLogService:    auditLogService,
	}
}

// Allowed fields per entity type for user-submitted edits.
// Admin-only fields (status, verified, auto_approve, etc.) are excluded.
var allowedEditFields = map[string]map[string]bool{
	"artist": {
		"name": true, "city": true, "state": true, "country": true,
		"description": true, "bandcamp_embed_url": true, "image_url": true,
		"instagram": true, "facebook": true, "twitter": true,
		"youtube": true, "spotify": true, "soundcloud": true,
		"bandcamp": true, "website": true,
	},
	"venue": {
		"name": true, "address": true, "city": true, "state": true,
		"country": true, "zipcode": true, "description": true, "image_url": true,
		"instagram": true, "facebook": true, "twitter": true,
		"youtube": true, "spotify": true, "soundcloud": true,
		"bandcamp": true, "website": true,
	},
	"festival": {
		"name": true, "description": true, "location_name": true,
		"city": true, "state": true, "country": true,
		"website": true, "ticket_url": true, "flyer_url": true,
	},
	"release": {
		"title": true, "release_year": true, "release_date": true,
		"release_type": true, "cover_art_url": true, "description": true,
	},
	"label": {
		"name": true, "founded_year": true,
		"city": true, "state": true, "country": true, "description": true,
		"image_url": true,
		"instagram": true, "facebook": true, "twitter": true,
		"youtube": true, "spotify": true, "soundcloud": true,
		"bandcamp": true, "website": true,
	},
}

// canEditDirectly returns true if the user can bypass the pending queue.
func canEditDirectly(user *authm.User) bool {
	if user.IsAdmin {
		return true
	}
	switch user.UserTier {
	case "trusted_contributor", "local_ambassador":
		return true
	}
	return false
}

// --- Suggest Edit ---

// SuggestEntityEditRequest is the Huma request for PUT /{entity_type}/{entity_id}/suggest-edit
type SuggestEntityEditRequest struct {
	EntityID string `path:"entity_id" doc:"Entity ID"`
	Body     struct {
		Changes []adminm.FieldChange `json:"changes" doc:"Field changes to propose"`
		Summary string               `json:"summary" doc:"Why you are making this change"`
	}
}

// SuggestEntityEditResponse is the Huma response for suggest-edit endpoints.
type SuggestEntityEditResponse struct {
	Body struct {
		PendingEdit *contracts.PendingEditResponse `json:"pending_edit,omitempty"`
		Applied     bool                           `json:"applied"`
		Message     string                         `json:"message"`
	}
}

// SuggestArtistEditHandler handles PUT /artists/{entity_id}/suggest-edit
func (h *PendingEditHandler) SuggestArtistEditHandler(ctx context.Context, req *SuggestEntityEditRequest) (*SuggestEntityEditResponse, error) {
	return h.suggestEdit(ctx, "artist", req)
}

// SuggestVenueEditHandler handles PUT /venues/{entity_id}/suggest-edit
func (h *PendingEditHandler) SuggestVenueEditHandler(ctx context.Context, req *SuggestEntityEditRequest) (*SuggestEntityEditResponse, error) {
	return h.suggestEdit(ctx, "venue", req)
}

// SuggestFestivalEditHandler handles PUT /festivals/{entity_id}/suggest-edit
func (h *PendingEditHandler) SuggestFestivalEditHandler(ctx context.Context, req *SuggestEntityEditRequest) (*SuggestEntityEditResponse, error) {
	return h.suggestEdit(ctx, "festival", req)
}

// SuggestReleaseEditHandler handles PUT /releases/{entity_id}/suggest-edit
func (h *PendingEditHandler) SuggestReleaseEditHandler(ctx context.Context, req *SuggestEntityEditRequest) (*SuggestEntityEditResponse, error) {
	return h.suggestEdit(ctx, "release", req)
}

// SuggestLabelEditHandler handles PUT /labels/{entity_id}/suggest-edit
func (h *PendingEditHandler) SuggestLabelEditHandler(ctx context.Context, req *SuggestEntityEditRequest) (*SuggestEntityEditResponse, error) {
	return h.suggestEdit(ctx, "label", req)
}

// suggestEdit is the shared implementation for all suggest-edit endpoints.
func (h *PendingEditHandler) suggestEdit(ctx context.Context, entityType string, req *SuggestEntityEditRequest) (*SuggestEntityEditResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	entityID, err := strconv.ParseUint(req.EntityID, 10, 64)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid entity ID")
	}

	if len(req.Body.Changes) == 0 {
		return nil, huma.Error422UnprocessableEntity("No changes provided")
	}

	summary := strings.TrimSpace(req.Body.Summary)
	if summary == "" {
		return nil, huma.Error422UnprocessableEntity("Summary is required — explain why you are making this change")
	}

	// Validate fields against allowed list
	allowed := allowedEditFields[entityType]
	for _, change := range req.Body.Changes {
		if !allowed[change.Field] {
			return nil, huma.Error422UnprocessableEntity(fmt.Sprintf("Field '%s' is not editable on %s entities", change.Field, entityType))
		}
	}

	// Create the pending edit
	resp, err := h.pendingEditService.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: entityType,
		EntityID:   uint(entityID),
		UserID:     user.ID,
		Changes:    req.Body.Changes,
		Summary:    summary,
	})
	if err != nil {
		if strings.Contains(err.Error(), "entity not found") {
			return nil, huma.Error404NotFound(err.Error())
		}
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			return nil, huma.Error409Conflict("You already have a pending edit for this entity")
		}
		logger.FromContext(ctx).Error("pending_edit_create_failed",
			"user_id", user.ID,
			"entity_type", entityType,
			"entity_id", entityID,
			"error", err.Error(),
		)
		return nil, huma.Error500InternalServerError("Failed to create pending edit")
	}

	// If trusted user, auto-approve immediately
	if canEditDirectly(user) {
		approved, approveErr := h.pendingEditService.ApprovePendingEdit(resp.ID, user.ID)
		if approveErr != nil {
			logger.FromContext(ctx).Error("pending_edit_auto_approve_failed",
				"edit_id", resp.ID,
				"user_id", user.ID,
				"error", approveErr.Error(),
			)
			// Fall through — the pending edit was still created
		} else {
			// Fire-and-forget audit log
			if h.auditLogService != nil {
				go h.auditLogService.LogAction(user.ID, "edit_"+entityType, entityType, uint(entityID), map[string]interface{}{
					"edit_id": approved.ID,
					"direct":  true,
					"summary": summary,
				})
			}

			out := &SuggestEntityEditResponse{}
			out.Body.PendingEdit = approved
			out.Body.Applied = true
			out.Body.Message = "Changes applied directly"
			return out, nil
		}
	}

	// Fire-and-forget audit log for pending edit
	if h.auditLogService != nil {
		go h.auditLogService.LogAction(user.ID, "suggest_edit_"+entityType, entityType, uint(entityID), map[string]interface{}{
			"edit_id": resp.ID,
			"summary": summary,
		})
	}

	out := &SuggestEntityEditResponse{}
	out.Body.PendingEdit = resp
	out.Body.Applied = false
	out.Body.Message = "Edit submitted for review"
	return out, nil
}

// --- User's Own Edits ---

// GetMyPendingEditsRequest is the Huma request for GET /my/pending-edits
type GetMyPendingEditsRequest struct {
	Limit  int `query:"limit" required:"false" doc:"Max results (default 20, max 100)"`
	Offset int `query:"offset" required:"false" doc:"Offset for pagination"`
}

// GetMyPendingEditsResponse is the Huma response for GET /my/pending-edits
type GetMyPendingEditsResponse struct {
	Body struct {
		Edits []contracts.PendingEditResponse `json:"edits"`
		Total int64                           `json:"total"`
	}
}

// GetMyPendingEditsHandler handles GET /my/pending-edits
func (h *PendingEditHandler) GetMyPendingEditsHandler(ctx context.Context, req *GetMyPendingEditsRequest) (*GetMyPendingEditsResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	edits, total, err := h.pendingEditService.GetUserPendingEdits(user.ID, req.Limit, req.Offset)
	if err != nil {
		logger.FromContext(ctx).Error("pending_edit_get_user_edits_failed",
			"user_id", user.ID,
			"error", err.Error(),
		)
		return nil, huma.Error500InternalServerError("Failed to get your pending edits")
	}

	resp := &GetMyPendingEditsResponse{}
	resp.Body.Edits = edits
	resp.Body.Total = total
	return resp, nil
}

// --- Cancel Own Edit ---

// CancelMyPendingEntityEditRequest is the Huma request for DELETE /my/pending-edits/{edit_id}
type CancelMyPendingEntityEditRequest struct {
	EditID string `path:"edit_id" doc:"Pending edit ID to cancel"`
}

// CancelMyPendingEntityEditResponse is the Huma response for DELETE /my/pending-edits/{edit_id}
type CancelMyPendingEntityEditResponse struct {
	Body struct {
		Success bool `json:"success"`
	}
}

// CancelMyPendingEditHandler handles DELETE /my/pending-edits/{edit_id}
func (h *PendingEditHandler) CancelMyPendingEditHandler(ctx context.Context, req *CancelMyPendingEntityEditRequest) (*CancelMyPendingEntityEditResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	editID, err := strconv.ParseUint(req.EditID, 10, 64)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid edit ID")
	}

	if err := h.pendingEditService.CancelPendingEdit(uint(editID), user.ID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, huma.Error404NotFound("Pending edit not found")
		}
		if strings.Contains(err.Error(), "only the submitter") {
			return nil, huma.Error403Forbidden("You can only cancel your own pending edits")
		}
		if strings.Contains(err.Error(), "not pending") {
			return nil, huma.Error409Conflict("This edit has already been reviewed")
		}
		logger.FromContext(ctx).Error("pending_edit_cancel_failed",
			"user_id", user.ID,
			"edit_id", editID,
			"error", err.Error(),
		)
		return nil, huma.Error500InternalServerError("Failed to cancel pending edit")
	}

	resp := &CancelMyPendingEntityEditResponse{}
	resp.Body.Success = true
	return resp, nil
}

// --- Admin: List Pending Edits ---

// AdminListPendingEditsRequest is the Huma request for GET /admin/pending-edits
type AdminListPendingEditsRequest struct {
	Status     string `query:"status" required:"false" doc:"Filter by status (pending, approved, rejected)"`
	EntityType string `query:"entity_type" required:"false" doc:"Filter by entity type (artist, venue, festival, release, label)"`
	Limit      int    `query:"limit" required:"false" doc:"Max results (default 20, max 100)"`
	Offset     int    `query:"offset" required:"false" doc:"Offset for pagination"`
}

// AdminListPendingEditsResponse is the Huma response for GET /admin/pending-edits
type AdminListPendingEditsResponse struct {
	Body struct {
		Edits []contracts.PendingEditResponse `json:"edits"`
		Total int64                           `json:"total"`
	}
}

// AdminListPendingEditsHandler handles GET /admin/pending-edits
func (h *PendingEditHandler) AdminListPendingEditsHandler(ctx context.Context, req *AdminListPendingEditsRequest) (*AdminListPendingEditsResponse, error) {
	edits, total, err := h.pendingEditService.ListPendingEdits(&contracts.PendingEditFilters{
		Status:     req.Status,
		EntityType: req.EntityType,
		Limit:      req.Limit,
		Offset:     req.Offset,
	})
	if err != nil {
		logger.FromContext(ctx).Error("pending_edit_list_failed", "error", err.Error())
		return nil, huma.Error500InternalServerError("Failed to list pending edits")
	}

	resp := &AdminListPendingEditsResponse{}
	resp.Body.Edits = edits
	resp.Body.Total = total
	return resp, nil
}

// --- Admin: Get Single Pending Edit ---

// AdminGetPendingEditRequest is the Huma request for GET /admin/pending-edits/{edit_id}
type AdminGetPendingEditRequest struct {
	EditID string `path:"edit_id" doc:"Pending edit ID"`
}

// AdminGetPendingEditResponse is the Huma response for GET /admin/pending-edits/{edit_id}
type AdminGetPendingEditResponse struct {
	Body *contracts.PendingEditResponse
}

// AdminGetPendingEditHandler handles GET /admin/pending-edits/{edit_id}
func (h *PendingEditHandler) AdminGetPendingEditHandler(ctx context.Context, req *AdminGetPendingEditRequest) (*AdminGetPendingEditResponse, error) {
	editID, err := strconv.ParseUint(req.EditID, 10, 64)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid edit ID")
	}

	edit, err := h.pendingEditService.GetPendingEdit(uint(editID))
	if err != nil {
		logger.FromContext(ctx).Error("pending_edit_get_failed", "edit_id", editID, "error", err.Error())
		return nil, huma.Error500InternalServerError("Failed to get pending edit")
	}
	if edit == nil {
		return nil, huma.Error404NotFound("Pending edit not found")
	}

	return &AdminGetPendingEditResponse{Body: edit}, nil
}

// --- Admin: Approve ---

// AdminApprovePendingEditRequest is the Huma request for POST /admin/pending-edits/{edit_id}/approve
type AdminApprovePendingEditRequest struct {
	EditID string `path:"edit_id" doc:"Pending edit ID to approve"`
}

// AdminApprovePendingEditResponse is the Huma response for POST /admin/pending-edits/{edit_id}/approve
type AdminApprovePendingEditResponse struct {
	Body *contracts.PendingEditResponse
}

// AdminApprovePendingEditHandler handles POST /admin/pending-edits/{edit_id}/approve
func (h *PendingEditHandler) AdminApprovePendingEditHandler(ctx context.Context, req *AdminApprovePendingEditRequest) (*AdminApprovePendingEditResponse, error) {
	user := middleware.GetUserFromContext(ctx)

	editID, err := strconv.ParseUint(req.EditID, 10, 64)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid edit ID")
	}

	approved, err := h.pendingEditService.ApprovePendingEdit(uint(editID), user.ID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, huma.Error404NotFound("Pending edit not found")
		}
		if strings.Contains(err.Error(), "not pending") {
			return nil, huma.Error409Conflict(err.Error())
		}
		if strings.Contains(err.Error(), "entity not found") {
			return nil, huma.Error422UnprocessableEntity("Entity no longer exists — cannot apply edit")
		}
		logger.FromContext(ctx).Error("pending_edit_approve_failed",
			"edit_id", editID,
			"admin_id", user.ID,
			"error", err.Error(),
		)
		return nil, huma.Error500InternalServerError("Failed to approve pending edit")
	}

	logger.FromContext(ctx).Info("pending_edit_approved",
		"edit_id", editID,
		"admin_id", user.ID,
		"entity_type", approved.EntityType,
		"entity_id", approved.EntityID,
	)

	// Fire-and-forget audit log
	if h.auditLogService != nil {
		go h.auditLogService.LogAction(user.ID, "approve_edit_"+approved.EntityType, approved.EntityType, approved.EntityID, map[string]interface{}{
			"edit_id":      approved.ID,
			"submitted_by": approved.SubmittedBy,
		})
	}

	return &AdminApprovePendingEditResponse{Body: approved}, nil
}

// --- Admin: Reject ---

// AdminRejectPendingEditRequest is the Huma request for POST /admin/pending-edits/{edit_id}/reject
type AdminRejectPendingEditRequest struct {
	EditID string `path:"edit_id" doc:"Pending edit ID to reject"`
	Body   struct {
		Reason string `json:"reason" doc:"Specific rejection reason (educational — explain the standard)"`
	}
}

// AdminRejectPendingEditResponse is the Huma response for POST /admin/pending-edits/{edit_id}/reject
type AdminRejectPendingEditResponse struct {
	Body *contracts.PendingEditResponse
}

// AdminRejectPendingEditHandler handles POST /admin/pending-edits/{edit_id}/reject
func (h *PendingEditHandler) AdminRejectPendingEditHandler(ctx context.Context, req *AdminRejectPendingEditRequest) (*AdminRejectPendingEditResponse, error) {
	user := middleware.GetUserFromContext(ctx)

	editID, err := strconv.ParseUint(req.EditID, 10, 64)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid edit ID")
	}

	reason := strings.TrimSpace(req.Body.Reason)
	if reason == "" {
		return nil, huma.Error422UnprocessableEntity("Rejection reason is required — be specific to help the contributor learn")
	}

	rejected, err := h.pendingEditService.RejectPendingEdit(uint(editID), user.ID, reason)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, huma.Error404NotFound("Pending edit not found")
		}
		if strings.Contains(err.Error(), "not pending") {
			return nil, huma.Error409Conflict(err.Error())
		}
		logger.FromContext(ctx).Error("pending_edit_reject_failed",
			"edit_id", editID,
			"admin_id", user.ID,
			"error", err.Error(),
		)
		return nil, huma.Error500InternalServerError("Failed to reject pending edit")
	}

	logger.FromContext(ctx).Info("pending_edit_rejected",
		"edit_id", editID,
		"admin_id", user.ID,
		"reason", reason,
	)

	// Fire-and-forget audit log
	if h.auditLogService != nil {
		go h.auditLogService.LogAction(user.ID, "reject_edit_"+rejected.EntityType, rejected.EntityType, rejected.EntityID, map[string]interface{}{
			"edit_id":      rejected.ID,
			"submitted_by": rejected.SubmittedBy,
			"reason":       reason,
		})
	}

	return &AdminRejectPendingEditResponse{Body: rejected}, nil
}

// --- Admin: Get Pending Edits for Entity ---

// AdminGetEntityPendingEditsRequest is the Huma request for GET /admin/pending-edits/entity/{entity_type}/{entity_id}
type AdminGetEntityPendingEditsRequest struct {
	EntityType string `path:"entity_type" doc:"Entity type (artist, venue, festival, release, label)"`
	EntityID   string `path:"entity_id" doc:"Entity ID"`
}

// AdminGetEntityPendingEditsResponse is the Huma response
type AdminGetEntityPendingEditsResponse struct {
	Body struct {
		Edits []contracts.PendingEditResponse `json:"edits"`
	}
}

// AdminGetEntityPendingEditsHandler handles GET /admin/pending-edits/entity/{entity_type}/{entity_id}
func (h *PendingEditHandler) AdminGetEntityPendingEditsHandler(ctx context.Context, req *AdminGetEntityPendingEditsRequest) (*AdminGetEntityPendingEditsResponse, error) {
	if !adminm.IsValidPendingEditEntityType(req.EntityType) {
		return nil, huma.Error422UnprocessableEntity(fmt.Sprintf("Invalid entity type: %s", req.EntityType))
	}

	entityID, err := strconv.ParseUint(req.EntityID, 10, 64)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid entity ID")
	}

	edits, err := h.pendingEditService.GetPendingEditsForEntity(req.EntityType, uint(entityID))
	if err != nil {
		logger.FromContext(ctx).Error("pending_edit_get_entity_edits_failed",
			"entity_type", req.EntityType,
			"entity_id", entityID,
			"error", err.Error(),
		)
		return nil, huma.Error500InternalServerError("Failed to get pending edits for entity")
	}

	resp := &AdminGetEntityPendingEditsResponse{}
	resp.Body.Edits = edits
	return resp, nil
}
