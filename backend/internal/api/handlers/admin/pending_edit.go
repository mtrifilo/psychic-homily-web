package admin

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared"
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

// allowedEditFields delegates to the per-entity allowlists co-located with
// each catalog model (PSY-572). The same maps are consulted by the
// ApprovePendingEdit gate in services/admin/pending_edit.go, so a malformed
// pending_entity_edits row that bypassed this handler's suggest-edit
// validator still cannot land non-allowlisted columns.
//
// MUST stay in sync with the frontend EDITABLE_FIELDS map in
// frontend/features/contributions/types.ts. Per-entity sources of truth
// live in internal/models/catalog/{entity}_allowlist.go.
//
// Admin-only fields (status, verified, auto_approve, etc.) are intentionally
// excluded — those go through the typed admin Edit handlers.
func allowedEditFields(entityType string) map[string]bool {
	allowed, _ := adminm.AllowedEditFields(entityType)
	return allowed
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
	// PSY-605: summary is markdown-rendered on read via utils.MarkdownRenderer,
	// so cap it at the same 10k-char ceiling comments and collection
	// descriptions use.
	if len(summary) > contracts.MaxPendingEditSummaryLength {
		return nil, huma.Error422UnprocessableEntity(fmt.Sprintf("Summary exceeds maximum length of %d characters", contracts.MaxPendingEditSummaryLength))
	}

	// Validate fields against allowed list, then validate URL field values
	// (PSY-549) so contributors can't land non-http/https URLs or oversize
	// strings in the pending queue. Without this gate, the field-name
	// allowlist controls *which* fields can be edited but not *what values*
	// they take — and ApprovePendingEdit applies values blindly.
	allowed := allowedEditFields(entityType)
	for _, change := range req.Body.Changes {
		if !allowed[change.Field] {
			return nil, huma.Error422UnprocessableEntity(fmt.Sprintf("Field '%s' is not editable on %s entities", change.Field, entityType))
		}
		if err := shared.ValidateFieldChangeValue(change.Field, change.NewValue); err != nil {
			return nil, err
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
			// PSY-618: do NOT emit a "edit_<type>" audit row here. The
			// pending_entity_edits row above is the canonical user-facing
			// event; ApprovePendingEdit also writes a revisions row. A
			// parallel audit row dual-rendered in the contributor activity
			// feed (UNION over audit_logs + pending_entity_edits) and
			// double-counted in stats (ArtistsEdited via audit_log +
			// RevisionsMade via revisions). Direct admin edits via the
			// catalog handlers continue to log via LogEntityEdit; this
			// path's edit is already represented by pending_entity_edits +
			// revisions.

			out := &SuggestEntityEditResponse{}
			out.Body.PendingEdit = approved
			out.Body.Applied = true
			out.Body.Message = "Changes applied directly"
			return out, nil
		}
	}

	// No audit_log emit on the pending path: the pending_entity_edits row
	// above is the user-facing event, surfaced as "submit_<type>_edit" by
	// the UNION in ContributorProfileService.GetContributionHistory. A
	// parallel "suggest_edit_*" audit row would double-render in Recent
	// Activity (no other reader consumes that audit action).

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
		// PSY-572: pending_edit carried fields not on the per-entity allowlist.
		// The service has already auto-marked the row 'rejected' and logged
		// the rejected fields with the edit ID + admin user ID; surface a 400
		// so the admin UI can show which fields blocked the approval.
		if errors.Is(err, adminm.ErrPendingEditDisallowedFields) {
			rejected := strings.TrimPrefix(err.Error(), adminm.ErrPendingEditDisallowedFields.Error()+": ")
			return nil, huma.Error400BadRequest(fmt.Sprintf(
				"This edit was auto-rejected: it includes field(s) the contributor UI does not allow (%s). The pending edit has been marked rejected.",
				rejected,
			))
		}
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
	// PSY-605: rejection reason is markdown-rendered on the contributor view
	// (when PSY-600 ships) via utils.MarkdownRenderer; cap matches summary.
	if len(reason) > contracts.MaxPendingEditSummaryLength {
		return nil, huma.Error422UnprocessableEntity(fmt.Sprintf("Rejection reason exceeds maximum length of %d characters", contracts.MaxPendingEditSummaryLength))
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
