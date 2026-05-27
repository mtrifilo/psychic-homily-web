package admin

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
	adminm "psychic-homily-backend/internal/models/admin"
	adminsvc "psychic-homily-backend/internal/services/admin"
	"psychic-homily-backend/internal/services/contracts"
)

// FeaturedSlotResponse is the wire shape for a single featured-slot row.
// CuratorNoteHTML carries the rendered + sanitized HTML; CuratorNote (the
// raw markdown source) is also exposed so the admin UI can re-edit a
// pick without round-tripping through the rendered form.
type FeaturedSlotResponse struct {
	ID              uint       `json:"id"`
	SlotType        string     `json:"slot_type"`
	EntityID        uint       `json:"entity_id"`
	CuratorNote     *string    `json:"curator_note,omitempty"`
	CuratorNoteHTML string     `json:"curator_note_html,omitempty"`
	ActiveFrom      time.Time  `json:"active_from"`
	ActiveUntil     *time.Time `json:"active_until,omitempty"`
	CreatedBy       uint       `json:"created_by"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// FeaturedSlotHandler exposes the admin-only featured-slot CRUD surface.
// Routes register on rc.Admin so the middleware enforces auth + IsAdmin
// upstream; handlers do not call shared.RequireAdmin themselves.
type FeaturedSlotHandler struct {
	featuredSlotService contracts.FeaturedSlotServiceInterface
	auditLogService     contracts.AuditLogServiceInterface
}

// NewFeaturedSlotHandler wires the featured-slot service + audit log.
func NewFeaturedSlotHandler(
	featuredSlotService contracts.FeaturedSlotServiceInterface,
	auditLogService contracts.AuditLogServiceInterface,
) *FeaturedSlotHandler {
	return &FeaturedSlotHandler{
		featuredSlotService: featuredSlotService,
		auditLogService:     auditLogService,
	}
}

// --- List / Get ---

// ListFeaturedSlotsRequest is the Huma request for GET /admin/featured-slots.
// HistoryLimit is per-slot — admin UI typically wants a small window
// (default 5, capped at 50).
type ListFeaturedSlotsRequest struct {
	HistoryLimit int `query:"history_limit" default:"5" minimum:"1" maximum:"50" doc:"Number of historical rows to include per slot_type"`
}

// FeaturedSlotsPerType bundles the active row (or nil if none) plus the
// recent-history rows for a single slot_type. Keeping the shape per-type
// rather than a flat list lets the admin UI render two parallel columns
// without re-grouping.
type FeaturedSlotsPerType struct {
	SlotType string                 `json:"slot_type"`
	Active   *FeaturedSlotResponse  `json:"active,omitempty"`
	History  []FeaturedSlotResponse `json:"history"`
}

// ListFeaturedSlotsResponse is the wire shape for GET /admin/featured-slots.
type ListFeaturedSlotsResponse struct {
	Body struct {
		Slots []FeaturedSlotsPerType `json:"slots"`
	}
}

// ListFeaturedSlotsHandler handles GET /admin/featured-slots — returns
// the current active row + recent history for both slot types.
func (h *FeaturedSlotHandler) ListFeaturedSlotsHandler(ctx context.Context, req *ListFeaturedSlotsRequest) (*ListFeaturedSlotsResponse, error) {
	requestID := logger.GetRequestID(ctx)

	limit := req.HistoryLimit
	if limit <= 0 {
		limit = 5
	}

	slotTypes := []string{adminm.FeaturedSlotTypeBill, adminm.FeaturedSlotTypeCollection}
	out := make([]FeaturedSlotsPerType, 0, len(slotTypes))

	for _, slotType := range slotTypes {
		entry := FeaturedSlotsPerType{SlotType: slotType, History: []FeaturedSlotResponse{}}

		active, err := h.featuredSlotService.GetActiveSlot(slotType)
		if err != nil && !errors.Is(err, adminsvc.ErrFeaturedSlotNotFound) {
			logger.FromContext(ctx).Error("admin_list_featured_slots_get_active_failed",
				"slot_type", slotType,
				"error", err.Error(),
				"request_id", requestID,
			)
			return nil, huma.Error500InternalServerError(
				fmt.Sprintf("Failed to load featured slots (request_id: %s)", requestID),
			)
		}
		if active != nil {
			r := h.toResponse(active)
			entry.Active = &r
		}

		history, err := h.featuredSlotService.ListRecent(slotType, limit)
		if err != nil {
			logger.FromContext(ctx).Error("admin_list_featured_slots_history_failed",
				"slot_type", slotType,
				"error", err.Error(),
				"request_id", requestID,
			)
			return nil, huma.Error500InternalServerError(
				fmt.Sprintf("Failed to load featured slots (request_id: %s)", requestID),
			)
		}
		for i := range history {
			entry.History = append(entry.History, h.toResponse(&history[i]))
		}

		out = append(out, entry)
	}

	resp := &ListFeaturedSlotsResponse{}
	resp.Body.Slots = out
	return resp, nil
}

// --- Set active slot ---

// SetFeaturedSlotRequest is the Huma request for POST /admin/featured-slots.
// EntityID is the show ID (slot_type=bill) or collection ID
// (slot_type=collection); the service does not validate the referent
// exists — that's the calling admin tool's job, mirroring how
// pending_entity_edits trusts the referenced entity_id.
type SetFeaturedSlotRequest struct {
	Body struct {
		SlotType    string  `json:"slot_type" doc:"One of 'bill' or 'collection'"`
		EntityID    uint    `json:"entity_id" doc:"Show ID (for bill) or Collection ID (for collection)"`
		CuratorNote *string `json:"curator_note,omitempty" doc:"Optional markdown curator note"`
	}
}

// SetFeaturedSlotResponse wraps the new active row.
type SetFeaturedSlotResponse struct {
	Body FeaturedSlotResponse `json:"body"`
}

// SetFeaturedSlotHandler handles POST /admin/featured-slots — atomic
// retire + insert. The previous active row's active_until is set to
// NOW() and a new active row is inserted, both inside one transaction.
func (h *FeaturedSlotHandler) SetFeaturedSlotHandler(ctx context.Context, req *SetFeaturedSlotRequest) (*SetFeaturedSlotResponse, error) {
	requestID := logger.GetRequestID(ctx)
	user := middleware.GetUserFromContext(ctx)

	if !adminm.IsValidFeaturedSlotType(req.Body.SlotType) {
		return nil, huma.Error400BadRequest("slot_type must be 'bill' or 'collection'")
	}
	if req.Body.EntityID == 0 {
		return nil, huma.Error400BadRequest("entity_id is required")
	}
	if req.Body.CuratorNote != nil && len(*req.Body.CuratorNote) > adminm.MaxFeaturedSlotCuratorNoteLength {
		return nil, huma.Error400BadRequest(
			fmt.Sprintf("curator_note exceeds maximum length of %d characters", adminm.MaxFeaturedSlotCuratorNoteLength),
		)
	}

	logger.FromContext(ctx).Debug("admin_set_featured_slot_attempt",
		"slot_type", req.Body.SlotType,
		"entity_id", req.Body.EntityID,
		"admin_id", user.ID,
	)

	slot, err := h.featuredSlotService.SetActiveSlot(req.Body.SlotType, req.Body.EntityID, req.Body.CuratorNote, user.ID)
	if err != nil {
		// Referent-validation sentinels translate to specific 4xx codes
		// so the admin UI can show a useful inline error instead of the
		// generic "phantom save" the consumer endpoint produced before
		// validation existed. Order matters: check the typed sentinels
		// BEFORE falling through to the generic 422.
		switch {
		case errors.Is(err, adminsvc.ErrFeaturedSlotReferentNotFound):
			switch req.Body.SlotType {
			case adminm.FeaturedSlotTypeBill:
				return nil, huma.Error404NotFound("show not found")
			case adminm.FeaturedSlotTypeCollection:
				return nil, huma.Error404NotFound("collection not found")
			default:
				return nil, huma.Error404NotFound("featured slot referent not found")
			}
		case errors.Is(err, adminsvc.ErrFeaturedSlotReferentNotApproved):
			return nil, huma.Error400BadRequest("show is not approved; only approved shows can be featured")
		case errors.Is(err, adminsvc.ErrFeaturedSlotReferentNotPublic):
			return nil, huma.Error400BadRequest("collection is private; only public collections can be featured")
		}
		logger.FromContext(ctx).Error("admin_set_featured_slot_failed",
			"slot_type", req.Body.SlotType,
			"entity_id", req.Body.EntityID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error422UnprocessableEntity(
			fmt.Sprintf("Failed to set featured slot (request_id: %s)", requestID),
		)
	}

	h.auditLogService.LogAction(user.ID, "set_featured_slot", "featured_slot", slot.ID, map[string]interface{}{
		"slot_type": slot.SlotType,
		"entity_id": slot.EntityID,
	})

	logger.FromContext(ctx).Info("admin_set_featured_slot_success",
		"slot_id", slot.ID,
		"slot_type", slot.SlotType,
		"entity_id", slot.EntityID,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	return &SetFeaturedSlotResponse{Body: h.toResponse(slot)}, nil
}

// --- Retire active slot ---

// DeleteFeaturedSlotRequest is the Huma request for DELETE /admin/featured-slots/{slot_type}.
type DeleteFeaturedSlotRequest struct {
	SlotType string `path:"slot_type" doc:"One of 'bill' or 'collection'"`
}

// DeleteFeaturedSlotResponse is the wire shape for the retire response.
type DeleteFeaturedSlotResponse struct {
	Body struct {
		SlotType string `json:"slot_type"`
		Message  string `json:"message"`
	}
}

// DeleteFeaturedSlotHandler handles DELETE /admin/featured-slots/{slot_type} —
// retires the active row without replacement. Idempotent at the API
// surface: a second DELETE returns 404 (no active row to retire).
func (h *FeaturedSlotHandler) DeleteFeaturedSlotHandler(ctx context.Context, req *DeleteFeaturedSlotRequest) (*DeleteFeaturedSlotResponse, error) {
	requestID := logger.GetRequestID(ctx)
	user := middleware.GetUserFromContext(ctx)

	if !adminm.IsValidFeaturedSlotType(req.SlotType) {
		return nil, huma.Error400BadRequest("slot_type must be 'bill' or 'collection'")
	}

	logger.FromContext(ctx).Debug("admin_retire_featured_slot_attempt",
		"slot_type", req.SlotType,
		"admin_id", user.ID,
	)

	if err := h.featuredSlotService.RetireActiveSlot(req.SlotType, user.ID); err != nil {
		if errors.Is(err, adminsvc.ErrFeaturedSlotNotFound) {
			return nil, huma.Error404NotFound(
				fmt.Sprintf("No active %s slot to retire", req.SlotType),
			)
		}
		logger.FromContext(ctx).Error("admin_retire_featured_slot_failed",
			"slot_type", req.SlotType,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error422UnprocessableEntity(
			fmt.Sprintf("Failed to retire featured slot (request_id: %s)", requestID),
		)
	}

	h.auditLogService.LogAction(user.ID, "retire_featured_slot", "featured_slot", 0, map[string]interface{}{
		"slot_type": req.SlotType,
	})

	logger.FromContext(ctx).Info("admin_retire_featured_slot_success",
		"slot_type", req.SlotType,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	resp := &DeleteFeaturedSlotResponse{}
	resp.Body.SlotType = req.SlotType
	resp.Body.Message = "Featured slot retired"
	return resp, nil
}

// toResponse maps a FeaturedSlot model to the wire shape, rendering the
// curator_note markdown to sanitized HTML in the same step. Keeps the
// rendering boundary single — the markdown source on disk stays the
// source of truth.
func (h *FeaturedSlotHandler) toResponse(slot *adminm.FeaturedSlot) FeaturedSlotResponse {
	return FeaturedSlotResponse{
		ID:              slot.ID,
		SlotType:        slot.SlotType,
		EntityID:        slot.EntityID,
		CuratorNote:     slot.CuratorNote,
		CuratorNoteHTML: h.featuredSlotService.RenderCuratorNote(slot.CuratorNote),
		ActiveFrom:      slot.ActiveFrom,
		ActiveUntil:     slot.ActiveUntil,
		CreatedBy:       slot.CreatedBy,
		CreatedAt:       slot.CreatedAt,
		UpdatedAt:       slot.UpdatedAt,
	}
}
