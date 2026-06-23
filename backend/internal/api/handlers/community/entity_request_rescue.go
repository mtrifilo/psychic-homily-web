package community

import (
	"context"
	"strconv"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/api/middleware"
	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
	communitym "psychic-homily-backend/internal/models/community"
	servicesshared "psychic-homily-backend/internal/services/shared"
)

// ============================================================================
// Admin: Rescue an approved-but-unfulfilled request —
//   POST /admin/entity-requests/{id}/fulfill  (PSY-1088)
// ============================================================================
//
// A request can sit at decision_state='approved' AND created_entity_id IS NULL
// with no Decide rescue (Decide only claims PENDING rows). The two by-design
// routes here are a trusted-tier auto-approved SHOW (the auto-approve path
// can't supply the venue+artist associations CreateShow needs — PSY-1037) and
// a post-claim fulfillment failure on the decide path. This endpoint lets an
// admin close the loop on such a row directly: either FULFILL it (re-run the
// catalog create, supplying show associations in the body for a show) or VOID
// it (reject the orphan). Both guard against double-action via the service's
// conditional updates scoped to the orphan state.

// rescueActionFulfill / rescueActionVoid are the two admin choices.
const (
	rescueActionFulfill = "fulfill"
	rescueActionVoid    = "void"
)

// AdminFulfillEntityRequestRequest is the Huma request for
// POST /admin/entity-requests/{id}/fulfill.
type AdminFulfillEntityRequestRequest struct {
	ID   string `path:"id" doc:"Approved-but-unfulfilled entity request ID to rescue"`
	Body struct {
		// Action selects the rescue: "fulfill" re-runs the catalog create;
		// "void" rejects the orphan. Defaults to "fulfill" when omitted.
		Action string  `json:"action,omitempty" required:"false" doc:"Rescue action: fulfill (default) or void"`
		Note   *string `json:"note,omitempty" required:"false" doc:"Optional note (recorded as the decision note when voiding)"`
		// PSY-1037 reuse: required when fulfilling a SHOW request (its payload
		// lacks the venue + artist associations CreateShow needs); ignored for
		// every other type and for void.
		ShowVenue   *ShowVenueInput   `json:"show_venue,omitempty" required:"false" doc:"Venue for fulfilling a show request (required when fulfilling a show)"`
		ShowArtists []ShowArtistInput `json:"show_artists,omitempty" required:"false" doc:"Artists for fulfilling a show request (required when fulfilling a show; at least one)"`
	}
}

// AdminFulfillEntityRequestResponse mirrors the decide response: on fulfill it
// reports the created catalog entity; on void it just returns the rejected row.
type AdminFulfillEntityRequestResponse struct {
	Body struct {
		Request           *communitym.EntityRequest `json:"request"`
		CreatedEntityID   *uint                     `json:"created_entity_id,omitempty"`
		CreatedEntityType *string                   `json:"created_entity_type,omitempty"`
	}
}

// AdminFulfillEntityRequestHandler handles POST /admin/entity-requests/{id}/fulfill.
// Admin-gated via rc.Admin middleware (no inline admin check, per PSY-423).
//
// Fulfill flow: load the row, verify it is approved-but-unfulfilled, build +
// validate show associations BEFORE any write (a malformed show body is a clean
// 422, never a half-rescue), create the catalog entity, then ATOMICALLY claim
// created_entity_id (the conditional update is the double-fulfill guard — a
// concurrent rescue that already won leaves this one's entity a recoverable
// stray rather than corrupting the link).
//
// Void flow: atomically reject the orphan (scoped so a fulfilled row can never
// be voided out from under its entity).
func (h *EntityRequestHandler) AdminFulfillEntityRequestHandler(ctx context.Context, req *AdminFulfillEntityRequestRequest) (*AdminFulfillEntityRequestResponse, error) {
	admin := middleware.GetUserFromContext(ctx)

	requestID, err := strconv.ParseUint(req.ID, 10, 64)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid request ID")
	}

	action := strings.TrimSpace(req.Body.Action)
	if action == "" {
		action = rescueActionFulfill
	}
	if action != rescueActionFulfill && action != rescueActionVoid {
		return nil, huma.Error422UnprocessableEntity("action must be 'fulfill' or 'void'")
	}

	var note *string
	if req.Body.Note != nil {
		if trimmed := strings.TrimSpace(*req.Body.Note); trimmed != "" {
			note = &trimmed
		}
	}

	if action == rescueActionVoid {
		return h.voidApproved(ctx, uint(requestID), admin.ID, note)
	}

	// Fulfill: load the row first so we can validate it's rescuable and decode
	// its type before doing any catalog work.
	existing, gerr := h.entityRequestService.GetRequest(uint(requestID))
	if gerr != nil {
		if mapped := shared.MapEntityRequestError(gerr); mapped != nil {
			return nil, mapped
		}
		logger.FromContext(ctx).Error("entity_request_rescue_load_failed",
			"request_id", requestID, "error", gerr.Error())
		return nil, huma.Error500InternalServerError("Failed to load request")
	}
	if existing == nil {
		return nil, huma.Error404NotFound("Entity request not found")
	}
	// Only an approved-but-unfulfilled row is rescuable. This pre-check gives a
	// readable 409 before any catalog write; the atomic claim below is the
	// authoritative guard against the concurrent-rescue race.
	if existing.DecisionState != communitym.EntityRequestStateApproved || existing.CreatedEntityID != nil {
		if mapped := shared.MapEntityRequestError(apperrors.ErrEntityRequestNotRescuable(uint(requestID))); mapped != nil {
			return nil, mapped
		}
		return nil, huma.Error409Conflict("Entity request is not approved-but-unfulfilled")
	}

	// Show associations are meaningful ONLY for a show. Build + validate them
	// BEFORE creating anything (so a bad show body is a clean 422, never a
	// half-rescue), but only when the row IS a show — otherwise a non-show
	// fulfill that incidentally carries a stray show_venue/show_artists would
	// get a misleading show-specific 422 for the wrong entity type. For
	// non-show types the fields are simply ignored, as the request doc states.
	var showAssoc *showAssociations
	if existing.EntityType == communitym.EntityRequestShow {
		var aerr error
		showAssoc, aerr = buildShowAssociations(req.Body.ShowVenue, req.Body.ShowArtists)
		if aerr != nil {
			return nil, aerr
		}
		// A show MUST carry associations on the rescue path — the payload alone
		// can't be fulfilled (the same requirement the decide endpoint enforces).
		if showAssoc == nil {
			return nil, huma.Error422UnprocessableEntity("Fulfilling a show requires show_venue and show_artists")
		}
	}

	// Create the catalog entity from the stored payload (reusing the shared
	// per-type dispatcher). showAssoc supplies the show's venue + artists.
	// Attribution is inherited from fulfillEntity (entity_request_fulfill.go):
	// a rescued show credits the original REQUESTER, not the rescuing admin.
	createdID, ferr := h.fulfillEntity(existing, showAssoc)
	if ferr != nil {
		logger.FromContext(ctx).Error("entity_request_rescue_fulfill_failed",
			"request_id", requestID,
			"admin_id", admin.ID,
			"entity_type", existing.EntityType,
			"error", ferr.Error(),
		)
		if mapped := mapFulfillmentError(ferr); mapped != nil {
			return nil, mapped
		}
		return nil, huma.Error500InternalServerError("Creating the entity failed: " + ferr.Error())
	}

	// Atomically claim the link. The conditional update matches only an
	// approved-but-unfulfilled row, so a concurrent rescue that already won
	// (or a row decided/voided between our pre-check and here) yields
	// claimed=false — the entity we just created is then a recoverable stray.
	claimed, cerr := h.entityRequestService.ClaimRescueFulfillment(uint(requestID), createdID)
	if cerr != nil {
		// The entity WAS created; only the link write erred. Surface a 500 so
		// the admin knows the row's created_entity_id may be unset (it's
		// reconcilable — the entity exists and the id is in the response/log).
		logger.FromContext(ctx).Error("entity_request_rescue_claim_failed",
			"request_id", requestID,
			"created_entity_id", createdID,
			"entity_type", existing.EntityType,
			"error", cerr.Error(),
		)
		return nil, huma.Error500InternalServerError("Entity created but recording the link failed")
	}
	if !claimed {
		// Lost the race (or the row stopped qualifying): the entity we created
		// is a stray. Report the conflict so the admin can reconcile rather than
		// silently 200-ing with a created_entity_id that isn't on the row.
		logger.FromContext(ctx).Warn("entity_request_rescue_lost_race",
			"request_id", requestID,
			"created_entity_id", createdID,
			"entity_type", existing.EntityType,
		)
		return nil, huma.Error409Conflict("Request was already fulfilled by a concurrent rescue; the entity created here is a stray to reconcile")
	}

	// Reflect the link on the returned row.
	idCopy := createdID
	existing.CreatedEntityID = &idCopy

	resp := &AdminFulfillEntityRequestResponse{}
	resp.Body.Request = existing
	resp.Body.CreatedEntityID = &idCopy
	et := existing.EntityType
	resp.Body.CreatedEntityType = &et

	if h.auditLogService != nil {
		reqID := existing.ID
		entityType := existing.EntityType
		metadata := map[string]interface{}{
			"request_id":        reqID,
			"requester_id":      existing.RequesterID,
			"created_entity_id": createdID,
		}
		servicesshared.GoSafe(ctx, "audit_log", func() {
			h.auditLogService.LogAction(admin.ID, "rescue_fulfill_entity_request", entityType, reqID, metadata)
		})
	}

	return resp, nil
}

// voidApproved rejects an approved-but-unfulfilled row. Atomic on the service
// side (scoped to the orphan state); a row that no longer qualifies yields a
// 409 rather than a silent no-op.
func (h *EntityRequestHandler) voidApproved(ctx context.Context, requestID, adminID uint, note *string) (*AdminFulfillEntityRequestResponse, error) {
	voided, err := h.entityRequestService.VoidApprovedUnfulfilled(requestID, adminID, note)
	if err != nil {
		logger.FromContext(ctx).Error("entity_request_rescue_void_failed",
			"request_id", requestID, "admin_id", adminID, "error", err.Error())
		return nil, huma.Error500InternalServerError("Failed to void request")
	}
	if !voided {
		if mapped := shared.MapEntityRequestError(apperrors.ErrEntityRequestNotRescuable(requestID)); mapped != nil {
			return nil, mapped
		}
		return nil, huma.Error409Conflict("Entity request is not approved-but-unfulfilled")
	}

	// Re-read so the response reflects the rejected row (decided_by/at/note).
	fresh, gerr := h.entityRequestService.GetRequest(requestID)
	if gerr != nil {
		logger.FromContext(ctx).Error("entity_request_rescue_void_reload_failed",
			"request_id", requestID, "error", gerr.Error())
		// The void succeeded; only the re-read failed. A 500 here would wrongly
		// imply the void didn't happen. Synthesize a minimal row so the 200 body
		// always carries a usable, non-null request (matching the fulfill path,
		// which never returns a null request on success) rather than stranding a
		// consumer with {"request": null}.
		decider := adminID
		fresh = &communitym.EntityRequest{
			ID:            requestID,
			DecisionState: communitym.EntityRequestStateRejected,
			DecidedBy:     &decider,
		}
	}

	resp := &AdminFulfillEntityRequestResponse{}
	resp.Body.Request = fresh

	if h.auditLogService != nil {
		// Log the row's real entity type (artist/venue/...) so a by-entity-type
		// audit query sees the void, matching the fulfill + decide audit rows.
		// fresh is always non-nil here (re-read result or the synthesized
		// fallback above); the fallback's EntityType is "" — log "entity_request"
		// then rather than an empty type.
		entityType := fresh.EntityType
		if entityType == "" {
			entityType = "entity_request"
		}
		servicesshared.GoSafe(ctx, "audit_log", func() {
			// rescue_void_* shares the rescue_ prefix with rescue_fulfill_* so a
			// single prefix query surfaces both halves of the rescue feature.
			h.auditLogService.LogAction(adminID, "rescue_void_entity_request", entityType, requestID,
				map[string]interface{}{"request_id": requestID})
		})
	}

	return resp, nil
}
