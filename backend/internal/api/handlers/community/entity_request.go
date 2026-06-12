package community

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
	communitym "psychic-homily-backend/internal/models/community"
	"psychic-homily-backend/internal/services/contracts"
	servicesshared "psychic-homily-backend/internal/services/shared"
)

// PSY-997: HTTP endpoints for the polymorphic entity_requests moderation queue
// (built on PSY-869's service in services/community/entityrequest.go). Mirrors
// the polymorphic shape of entity_report.go: one user-facing create endpoint +
// admin list/decide endpoints, with audit-log writes fire-and-forget.
//
// These endpoints are intentionally SEPARATE from the pending_entity_edits
// admin endpoints (PSY-871's frontend unifies the two queues into one page;
// the backend keeps them parallel and independently testable).

// EntityRequestHandler handles entity-request queue API endpoints.
type EntityRequestHandler struct {
	entityRequestService contracts.EntityRequestServiceInterface
	fulfiller            contracts.EntityRequestFulfillerInterface
	auditLogService      contracts.AuditLogServiceInterface
}

// NewEntityRequestHandler creates a new entity-request handler. fulfiller is
// used only on the admin decide-approve path to create the actual entity from
// the payload; it may be nil in unit tests that don't exercise approval.
func NewEntityRequestHandler(
	entityRequestService contracts.EntityRequestServiceInterface,
	fulfiller contracts.EntityRequestFulfillerInterface,
	auditLogService contracts.AuditLogServiceInterface,
) *EntityRequestHandler {
	return &EntityRequestHandler{
		entityRequestService: entityRequestService,
		fulfiller:            fulfiller,
		auditLogService:      auditLogService,
	}
}

// ============================================================================
// User: Queue an entity-creation request — POST /entity-requests
// ============================================================================

// CreateEntityRequestRequest is the Huma request for POST /entity-requests.
//
// Payload is the typed, per-entity_type creation payload (the shapes in
// communitym/entity_request_payloads.go). It is carried as a raw JSON object
// and validated against the entity_type's registered struct on the read side;
// the handler only enforces it is present + non-empty here.
type CreateEntityRequestRequest struct {
	Body struct {
		EntityType    string                                `json:"entity_type" doc:"Entity type to request (artist, venue, label, release, show, festival)"`
		Payload       json.RawMessage                       `json:"payload" doc:"Typed creation payload for the entity_type"`
		SourceContext string                                `json:"source_context" required:"false" doc:"How the request originated (ai_extraction, paste_mode, manual); defaults to manual"`
		SourceDetail  *communitym.EntityRequestSourceDetail `json:"source_detail" required:"false" doc:"Optional origin context (source URL + excerpt), chiefly for AI extraction; shown in the admin moderation queue"`
		Confirmed     bool                                  `json:"confirmed" required:"false" doc:"FE-side confirm step (only relevant to trusted_contributor tier)"`
	}
}

// Defensive caps for the optional source_detail fields at the trust boundary.
const (
	maxSourceURLLen     = 2048
	maxSourceExcerptLen = 10000
)

// CreateEntityRequestResponse is the Huma response for POST /entity-requests.
type CreateEntityRequestResponse struct {
	Body *communitym.EntityRequest
}

// CreateEntityRequestHandler handles POST /entity-requests.
//
// Tier policy lives in PSY-869's service (autoApproves): contributor/new_user
// file a PENDING request (never autonomously create the entity, per
// feedback_human_verify_ai_entity_data); admin/local_ambassador (and confirmed
// trusted_contributor) auto-approve. The service stamps decided_by/at on
// auto-approve. This handler is a thin validator + pass-through.
func (h *EntityRequestHandler) CreateEntityRequestHandler(ctx context.Context, req *CreateEntityRequestRequest) (*CreateEntityRequestResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	entityType := strings.TrimSpace(req.Body.EntityType)
	if !communitym.IsValidEntityRequestType(entityType) {
		return nil, huma.Error422UnprocessableEntity("Invalid entity type '" + entityType + "'")
	}

	// source_context defaults to manual when omitted; any provided value must
	// be a recognized source.
	sourceContext := strings.TrimSpace(req.Body.SourceContext)
	if sourceContext == "" {
		sourceContext = communitym.EntityRequestSourceManual
	}
	if !communitym.IsValidEntityRequestSource(sourceContext) {
		return nil, huma.Error422UnprocessableEntity("Invalid source context '" + sourceContext + "'")
	}

	if len(strings.TrimSpace(string(req.Body.Payload))) == 0 {
		return nil, huma.Error422UnprocessableEntity("Payload is required")
	}

	// Validate the payload decodes cleanly into its typed struct (rejects
	// unknown fields / wrong shape / missing required fields) at the trust
	// boundary, so a malformed contributor payload is rejected here rather than
	// stored as junk in the queue and failing confusingly on admin approve.
	if err := communitym.ValidateEntityRequestPayload(entityType, req.Body.Payload); err != nil {
		return nil, huma.Error422UnprocessableEntity("Invalid payload for " + entityType + ": " + err.Error())
	}

	// Normalize the optional source detail (trim, drop empties) and cap its
	// fields at the trust boundary. An all-empty detail becomes nil so the row
	// stores NULL rather than an empty object.
	sourceDetail, err := normalizeSourceDetail(req.Body.SourceDetail)
	if err != nil {
		return nil, err
	}

	created, err := h.entityRequestService.CreateRequest(user, entityType, req.Body.Payload, sourceContext, sourceDetail, req.Body.Confirmed)
	if err != nil {
		if mapped := shared.MapEntityRequestError(err); mapped != nil {
			return nil, mapped
		}
		logger.FromContext(ctx).Error("entity_request_create_failed",
			"user_id", user.ID,
			"entity_type", entityType,
			"source_context", sourceContext,
			"error", err.Error(),
		)
		return nil, huma.Error500InternalServerError("Failed to create entity request")
	}

	// Auto-approve fulfillment (PSY-1008): when a trusted tier's request lands
	// already-approved (the service stamped it), create the catalog entity now
	// and stamp created_entity_id onto the returned row so the frontend can
	// stage the new entity in the same step (true inline create-and-add). The
	// CreatedEntityID == nil guard skips the idempotent-dedup path, which only
	// ever returns an existing PENDING row (never approved/fulfilled).
	if created.DecisionState == communitym.EntityRequestStateApproved && created.CreatedEntityID == nil {
		// nil show-associations: only the admin decide endpoint can supply them
		// (PSY-1037), so an auto-approved show defers below.
		if _, ferr := h.fulfillAndRecord(ctx, created, nil); ferr != nil {
			if isFulfillUnsupported(ferr) {
				// show auto-approve: the request is filed-and-approved, but a
				// show's catalog Create needs admin-supplied venue + artist
				// associations (PSY-1037's decide endpoint collects them). Leave
				// it approved-but-unfulfilled (created_entity_id NULL → surfaced by
				// the admin queue) instead of failing the whole request.
				// (Festival now fulfills inline, so it never reaches here.)
				logger.FromContext(ctx).Warn("entity_request_autoapprove_fulfill_deferred",
					"request_id", created.ID,
					"entity_type", created.EntityType,
				)
			} else {
				// Real fulfillment failure. The row is already approved; surface
				// so the requester knows the entity was NOT created (and the
				// staging step won't happen) rather than returning a misleading
				// success with no created_entity_id.
				logger.FromContext(ctx).Error("entity_request_autoapprove_fulfill_failed",
					"request_id", created.ID,
					"entity_type", created.EntityType,
					"error", ferr.Error(),
				)
				// A duplicate catalog entity (e.g. ArtistExists) maps to 409 here,
				// not 500 — inline create-and-add of an already-existing entity is
				// a benign conflict, not a server fault.
				if mapped := mapFulfillmentError(ferr); mapped != nil {
					return nil, mapped
				}
				return nil, huma.Error500InternalServerError("Request approved but creating the entity failed: " + ferr.Error())
			}
		}
	}

	// Fire-and-forget audit log. Distinguish auto-approved (trusted tiers) from
	// queued so the activity feed reads correctly.
	if h.auditLogService != nil {
		action := "queue_entity_request"
		if created.DecisionState == communitym.EntityRequestStateApproved {
			action = "auto_approve_entity_request"
		}
		reqID := created.ID
		state := string(created.DecisionState)
		metadata := map[string]interface{}{
			"request_id":     reqID,
			"source_context": sourceContext,
			"decision_state": state,
		}
		if created.CreatedEntityID != nil {
			metadata["created_entity_id"] = *created.CreatedEntityID
		}
		servicesshared.GoSafe(ctx, "audit_log", func() {
			h.auditLogService.LogAction(user.ID, action, entityType, reqID, metadata)
		})
	}

	return &CreateEntityRequestResponse{Body: created}, nil
}

// ============================================================================
// Admin: List entity requests — GET /admin/entity-requests
// ============================================================================

// AdminListEntityRequestsRequest is the Huma request for GET /admin/entity-requests.
type AdminListEntityRequestsRequest struct {
	State         string `query:"state" required:"false" doc:"Filter by decision state (pending, approved, rejected); defaults to pending"`
	EntityType    string `query:"entity_type" required:"false" doc:"Filter by entity type (artist, venue, label, release, show, festival)"`
	SourceContext string `query:"source_context" required:"false" doc:"Filter by source context (ai_extraction, paste_mode, manual)"`
	Limit         int    `query:"limit" required:"false" minimum:"1" maximum:"100" doc:"Max results (default 20, max 100)"`
	Offset        int    `query:"offset" required:"false" minimum:"0" doc:"Offset for pagination"`
}

// AdminEntityRequestView is the admin-queue projection of an EntityRequest with
// the requester's display name/username resolved (PSY-871). The raw model
// serializes Requester as json:"-", so the moderation UI can't attribute a
// request from the model alone; this view resolves it via the canonical
// user_resolver (mirroring the PendingEdit / EntityReport admin responses).
// It carries exactly what the moderation card needs: the typed payload for the
// preview, source context (+ AI source_detail), requester attribution, and the
// decision/fulfillment fields for non-pending views.
type AdminEntityRequestView struct {
	ID                uint             `json:"id"`
	EntityType        string           `json:"entity_type"`
	Payload           *json.RawMessage `json:"payload"`
	SourceContext     string           `json:"source_context"`
	SourceDetail      *json.RawMessage `json:"source_detail,omitempty"`
	RequesterID       uint             `json:"requester_id"`
	RequesterName     string           `json:"requester_name"`
	RequesterUsername *string          `json:"requester_username"`
	DecisionState     string           `json:"decision_state"`
	DecisionNote      *string          `json:"decision_note,omitempty"`
	CreatedEntityID   *uint            `json:"created_entity_id,omitempty"`
	CreatedAt         time.Time        `json:"created_at"`
}

// toAdminEntityRequestView projects a model row onto the admin view, resolving
// the requester display via the user_resolver. The Requester relation MUST be
// preloaded by the caller (ListRequests preloads it).
func toAdminEntityRequestView(r *communitym.EntityRequest) AdminEntityRequestView {
	return AdminEntityRequestView{
		ID:                r.ID,
		EntityType:        r.EntityType,
		Payload:           r.Payload,
		SourceContext:     r.SourceContext,
		SourceDetail:      r.SourceDetail,
		RequesterID:       r.RequesterID,
		RequesterName:     servicesshared.ResolveUserName(&r.Requester),
		RequesterUsername: servicesshared.ResolveUserUsername(&r.Requester),
		DecisionState:     string(r.DecisionState),
		DecisionNote:      r.DecisionNote,
		CreatedEntityID:   r.CreatedEntityID,
		CreatedAt:         r.CreatedAt,
	}
}

// AdminListEntityRequestsResponse is the Huma response for GET /admin/entity-requests.
type AdminListEntityRequestsResponse struct {
	Body struct {
		Requests []AdminEntityRequestView `json:"requests"`
		Total    int64                    `json:"total"`
	}
}

// AdminListEntityRequestsHandler handles GET /admin/entity-requests.
// Admin-gated via rc.Admin middleware (no inline admin check, per PSY-423).
func (h *EntityRequestHandler) AdminListEntityRequestsHandler(ctx context.Context, req *AdminListEntityRequestsRequest) (*AdminListEntityRequestsResponse, error) {
	if req.EntityType != "" && !communitym.IsValidEntityRequestType(req.EntityType) {
		return nil, huma.Error422UnprocessableEntity("Invalid entity type '" + req.EntityType + "'")
	}
	if req.State != "" && !communitym.IsValidEntityRequestState(req.State) {
		return nil, huma.Error422UnprocessableEntity("Invalid state '" + req.State + "'")
	}
	if req.SourceContext != "" && !communitym.IsValidEntityRequestSource(req.SourceContext) {
		return nil, huma.Error422UnprocessableEntity("Invalid source context '" + req.SourceContext + "'")
	}

	requests, total, err := h.entityRequestService.ListRequests(&contracts.EntityRequestFilters{
		EntityType:    req.EntityType,
		State:         req.State,
		SourceContext: req.SourceContext,
		Limit:         req.Limit,
		Offset:        req.Offset,
	})
	if err != nil {
		logger.FromContext(ctx).Error("entity_request_list_failed", "error", err.Error())
		return nil, huma.Error500InternalServerError("Failed to list entity requests")
	}

	views := make([]AdminEntityRequestView, 0, len(requests))
	for i := range requests {
		views = append(views, toAdminEntityRequestView(&requests[i]))
	}

	resp := &AdminListEntityRequestsResponse{}
	resp.Body.Requests = views
	resp.Body.Total = total
	return resp, nil
}

// ============================================================================
// Admin: Decide an entity request — POST /admin/entity-requests/{id}/decide
// ============================================================================

// ShowVenueInput is the admin-supplied venue for fulfilling a show request at
// approve time (PSY-1037). ID attaches an existing venue; otherwise
// Name+City+State find-or-create one (admin-created venues are auto-verified
// by the show service).
type ShowVenueInput struct {
	ID      *uint   `json:"id,omitempty" required:"false" doc:"Existing venue ID (optional)"`
	Name    string  `json:"name" doc:"Venue name"`
	City    string  `json:"city" doc:"Venue city"`
	State   string  `json:"state" doc:"Venue state"`
	Address *string `json:"address,omitempty" required:"false" doc:"Street address (optional)"`
}

// ShowArtistInput is one admin-supplied artist for fulfilling a show request
// at approve time (PSY-1037). Name is always required (the show service's
// duplicate-headliner pre-check matches on name); ID optionally pins an
// existing artist, otherwise Name find-or-creates one (case-insensitive).
type ShowArtistInput struct {
	ID          *uint  `json:"id,omitempty" required:"false" doc:"Existing artist ID (optional)"`
	Name        string `json:"name" doc:"Artist name (required)"`
	IsHeadliner *bool  `json:"is_headliner,omitempty" required:"false" doc:"Headliner flag (first artist defaults to headliner when unset)"`
}

// AdminDecideEntityRequestRequest is the Huma request for
// POST /admin/entity-requests/{id}/decide.
type AdminDecideEntityRequestRequest struct {
	ID   string `path:"id" doc:"Entity request ID to decide"`
	Body struct {
		Decision string  `json:"decision" doc:"Decision: approved or rejected"`
		Note     *string `json:"note" required:"false" doc:"Optional decision note (shown to the requester)"`
		// PSY-1037: required when approving a show request (its payload lacks
		// the venue + artist associations CreateShow needs); ignored for every
		// other entity type and for rejections.
		ShowVenue   *ShowVenueInput   `json:"show_venue,omitempty" required:"false" doc:"Venue for fulfilling a show request (required when approving a show)"`
		ShowArtists []ShowArtistInput `json:"show_artists,omitempty" required:"false" doc:"Artists for fulfilling a show request (required when approving a show; at least one)"`
	}
}

// AdminDecideEntityRequestResponse is the Huma response for the decide endpoint.
// On approve, CreatedEntityID/CreatedEntityType report the catalog entity that
// was created from the payload.
type AdminDecideEntityRequestResponse struct {
	Body struct {
		Request           *communitym.EntityRequest `json:"request"`
		CreatedEntityID   *uint                     `json:"created_entity_id,omitempty"`
		CreatedEntityType *string                   `json:"created_entity_type,omitempty"`
	}
}

// AdminDecideEntityRequestHandler handles POST /admin/entity-requests/{id}/decide.
// Admin-gated via rc.Admin middleware (no inline admin check, per PSY-423).
//
// Approve flow (claim-then-fulfill): the service's atomic Decide claims the
// pending→approved transition FIRST (so two concurrent approvals can't both
// win and double-create the entity), then the handler fulfills the payload into
// a real catalog entity. If fulfillment fails after the claim, the row is
// approved-but-unfulfilled and the error is logged loudly + surfaced — the
// admin can create the entity manually. This trades a rare orphaned-approval
// for never double-creating an entity.
//
// Reject flow: marks rejected + optional note. No entity is created.
func (h *EntityRequestHandler) AdminDecideEntityRequestHandler(ctx context.Context, req *AdminDecideEntityRequestRequest) (*AdminDecideEntityRequestResponse, error) {
	admin := middleware.GetUserFromContext(ctx)

	requestID, err := strconv.ParseUint(req.ID, 10, 64)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid request ID")
	}

	decision := strings.TrimSpace(req.Body.Decision)
	var newState communitym.EntityRequestDecisionState
	switch decision {
	case string(communitym.EntityRequestStateApproved):
		newState = communitym.EntityRequestStateApproved
	case string(communitym.EntityRequestStateRejected):
		newState = communitym.EntityRequestStateRejected
	default:
		return nil, huma.Error422UnprocessableEntity("Decision must be 'approved' or 'rejected'")
	}

	var note *string
	if req.Body.Note != nil {
		trimmed := strings.TrimSpace(*req.Body.Note)
		if trimmed != "" {
			note = &trimmed
		}
	}

	// PSY-1037: validate + convert admin-supplied show associations BEFORE the
	// row is claimed, so malformed input is a clean 422 instead of an
	// approved-but-unfulfilled row. (We can't know the request's entity_type
	// until Decide returns, so this validates whatever was supplied; the
	// associations are ignored for non-show types.)
	showAssoc, aerr := buildShowAssociations(req.Body.ShowVenue, req.Body.ShowArtists)
	if aerr != nil {
		return nil, aerr
	}

	// Claim the decision atomically before any side effect.
	decided, err := h.entityRequestService.Decide(uint(requestID), admin.ID, newState, note)
	if err != nil {
		if mapped := shared.MapEntityRequestError(err); mapped != nil {
			return nil, mapped
		}
		logger.FromContext(ctx).Error("entity_request_decide_failed",
			"request_id", requestID,
			"admin_id", admin.ID,
			"decision", decision,
			"error", err.Error(),
		)
		return nil, huma.Error500InternalServerError("Failed to record decision")
	}

	resp := &AdminDecideEntityRequestResponse{}
	resp.Body.Request = decided

	if newState == communitym.EntityRequestStateApproved {
		createdID, err := h.fulfillAndRecord(ctx, decided, showAssoc)
		if err != nil {
			// The row is already approved (claimed). Surface the fulfillment
			// failure so the admin knows the entity was NOT created and can act,
			// rather than silently returning success. FulfillUnsupported (show)
			// maps to 422 and a duplicate catalog entity to 409 via
			// mapFulfillmentError; only an unrecognized fault falls to 500.
			logger.FromContext(ctx).Error("entity_request_fulfill_failed",
				"request_id", requestID,
				"admin_id", admin.ID,
				"entity_type", decided.EntityType,
				"error", err.Error(),
			)
			if mapped := mapFulfillmentError(err); mapped != nil {
				return nil, mapped
			}
			return nil, huma.Error500InternalServerError("Request approved but creating the entity failed: " + err.Error())
		}
		resp.Body.CreatedEntityID = &createdID
		et := decided.EntityType
		resp.Body.CreatedEntityType = &et
	}

	// Fire-and-forget audit log.
	if h.auditLogService != nil {
		action := "approve_entity_request"
		if newState == communitym.EntityRequestStateRejected {
			action = "reject_entity_request"
		}
		reqID := decided.ID
		metadata := map[string]interface{}{
			"request_id":   reqID,
			"requester_id": decided.RequesterID,
		}
		if resp.Body.CreatedEntityID != nil {
			metadata["created_entity_id"] = *resp.Body.CreatedEntityID
		}
		entityType := decided.EntityType
		servicesshared.GoSafe(ctx, "audit_log", func() {
			h.auditLogService.LogAction(admin.ID, action, entityType, reqID, metadata)
		})
	}

	return resp, nil
}

// ============================================================================
// Shared fulfillment + request-body helpers (PSY-1008)
// ============================================================================

// fulfillAndRecord creates the catalog entity from an approved request's payload
// (via the per-type dispatcher) and persists created_entity_id back onto the
// request row. It sets req.CreatedEntityID so the response body reflects the new
// entity even if the persistence write fails — best-effort: the entity WAS
// created, so surfacing a 500 there would wrongly imply it wasn't. The
// fulfillEntity error is returned verbatim (including the typed
// FulfillUnsupported for show) so callers classify it via
// isFulfillUnsupported. Used by both the auto-approve create path and the admin
// approve path so they record fulfillment identically.
func (h *EntityRequestHandler) fulfillAndRecord(ctx context.Context, req *communitym.EntityRequest, showAssoc *showAssociations) (uint, error) {
	createdID, err := h.fulfillEntity(req, showAssoc)
	if err != nil {
		return 0, err
	}
	idCopy := createdID
	req.CreatedEntityID = &idCopy
	if rerr := h.entityRequestService.RecordFulfillment(req.ID, createdID); rerr != nil {
		// The entity WAS created; only the link-back write failed. Log loudly
		// and continue — the response already carries created_entity_id (set
		// above), and the row's created_entity_id is reconcilable later.
		logger.FromContext(ctx).Error("entity_request_record_fulfillment_failed",
			"request_id", req.ID,
			"created_entity_id", createdID,
			"entity_type", req.EntityType,
			"error", rerr.Error(),
		)
	}
	return createdID, nil
}

// normalizeSourceDetail trims + length-caps the optional source detail and
// marshals it to JSONB bytes for storage. Returns (nil, nil) when there is no
// usable content (so the row stores NULL, not an empty object), or a 422 when a
// field exceeds its cap.
func normalizeSourceDetail(in *communitym.EntityRequestSourceDetail) ([]byte, error) {
	clean, ok := in.Normalize()
	if !ok {
		return nil, nil
	}
	if clean.URL != nil && len(*clean.URL) > maxSourceURLLen {
		return nil, huma.Error422UnprocessableEntity("source_detail.url exceeds maximum length")
	}
	if clean.Excerpt != nil && len(*clean.Excerpt) > maxSourceExcerptLen {
		return nil, huma.Error422UnprocessableEntity("source_detail.excerpt exceeds maximum length")
	}
	b, err := json.Marshal(clean)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to encode source detail")
	}
	return b, nil
}
