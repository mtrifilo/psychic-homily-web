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
		EntityType string `json:"entity_type" doc:"Entity type to request (artist, venue, label, release, show, festival)"`
		// The payload is json.RawMessage, so NOTHING about its shape reaches the
		// generated OpenAPI document: the doc string is the only contract a
		// producer author sees. The show bill's headliner rule is stated there
		// for that reason (PSY-1858) — a producer that assumes bill order names
		// the headliner, as most sources do, ships shows with none.
		//
		// TestCreateEntityRequestPayloadDocMatchesTheRules pins the cap and the
		// vocabulary this string restates, since a doc tag cannot be built from
		// constants.
		Payload       json.RawMessage                       `json:"payload" doc:"Typed creation payload for the entity_type. A show payload may carry the bill as artists: [{name, set_type?}], name only, no id, at most 50 acts. A payload bill NEVER infers a headliner from list order: an act with no set_type is stored as 'performer', so a bill naming no 'headliner' creates a show with no headliner row. State set_type 'headliner' explicitly when the source names one. When set_type is present it must be one of: headliner,direct_support,opener,special_guest,dj,performer."`
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
	Body *CreateEntityRequestResponseBody
}

// EntityRequestFields is communitym.EntityRequest with its methods stripped, so
// it can be EMBEDDED in a response body. Huma's schema-link transformer rebuilds
// every body struct with reflect.StructOf to prepend $schema, and
// reflect.StructOf refuses an embedded type that HAS METHODS unless it is the
// first field — which it no longer is once $schema goes in front. EntityRequest
// has TableName(); a defined type does not inherit it. Huma only warns on that
// failure, so embedding the model directly silently drops $schema and the
// describedBy Link header from this one endpoint while the generated types still
// declare them. Same fields, same json tags, same schema.
type EntityRequestFields communitym.EntityRequest

// CreateEntityRequestResponseBody is the queue-create response body: the request
// row, plus the one fact the row itself cannot carry — whether this submission
// opened a new request or corrected a queued one.
//
// Named rather than anonymous so the embed below survives; the ...ResponseBody
// suffix is the one Huma would have generated, and the sibling schema
// CreateEntityRequestRequestBody is one token away in the generated document.
//
// The request is EMBEDDED so its fields stay at the top level of the JSON, which
// is the shape clients already read (PSY-1008's created_entity_id / PSY-1858's
// payload are read straight off the body).
type CreateEntityRequestResponseBody struct {
	*EntityRequestFields
	// A client that reports "queued" for both a fresh request and a replacement
	// leaves a contributor unable to tell their correction landed.
	Replaced bool `json:"replaced" doc:"True when this submission replaced the requester's existing pending request (a correction) rather than filing a new one. A request matches an existing one when the name (or title) AND the occurrence date match; a show or festival on a different date is a different request and files its own row. The returned id is the queued request's. Only a PENDING request is ever replaced; read decision_state for the row's state, which an admin can decide the moment the replacement lands."`
}

// CreateEntityRequestHandler handles POST /entity-requests.
//
// Tier policy lives in PSY-869's service (autoApproves): contributor/new_user
// file a PENDING request (never autonomously create the entity, per
// feedback_human_verify_ai_entity_data); admin/local_ambassador (and confirmed
// trusted_contributor) auto-approve. The service stamps decided_by/at on
// auto-approve. This handler is a thin validator + pass-through.
//
// PSY-1948 — RESUBMISSION REPLACES: a request matching an existing PENDING one
// on (entity_type, requester, normalized name, occurrence) overwrites that row's
// payload, source_context and source_detail instead of filing a second row, and
// the response reports replaced: true on the queued row's own id. A resubmission
// is how a contributor corrects a queued request, and returning the stored
// payload discarded the correction behind a 2xx.
//
// PSY-1977 — the occurrence is the payload's own date, so a recurring night
// queued twice is two requests; each payload type names its own occurrence field
// (EntityRequestPayload.dedupOccurrenceJSONKey).
//
// Only a PENDING row is ever written: the dedup index is pending-only and the
// UPDATE is conditional on the state, so a decided request is never rewritten.
//
// What that does NOT buy, stated here because it is the surprising half: a
// queued payload is now MUTABLE until it is decided, so an admin can review one
// payload in the queue and approve a later one. That is the intended shape of
// replace-on-resubmit (the queue is meant to show the correction).
// AdminEntityRequestView carries updated_at so a queue CAN tell a revised row
// from an untouched one, but nothing renders it yet (PSY-1975), so today the
// signal exists on the wire and not on the screen. The decide handler's claim
// defends the narrower window between its own read and its claim; see the note
// there.
//
// Only queueing tiers reach this path at all. An auto-approving tier's row is
// stamped 'approved' before the INSERT, so it never collides with the
// pending-only index and never replaces anything.
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

	// PSY-1858: a show payload may carry the bill the contributor knew. Its roles
	// are checked against the curated set_type vocabulary HERE, at submit, for
	// the same reason the admin paths check them pre-claim: a role rejected at
	// fulfillment is rejected after the row has been claimed, and a claimed row's
	// payload can no longer be corrected (PSY-1948's resubmission replaces PENDING
	// rows only). The check is not inside ValidateEntityRequestPayload only
	// because the vocabulary lives in a package that imports the payload models;
	// see validateShowPayloadBillRoles. The
	// bill's STRUCTURE was already checked by ValidateEntityRequestPayload above.
	// Ordered ahead of the image-URL guard because it is a pure in-memory check
	// and that one can resolve DNS.
	if err := validateShowPayloadBillRoles(entityType, req.Body.Payload); err != nil {
		return nil, err
	}

	// PSY-1675: the payload's image_url rides onto a real entity at fulfillment
	// and is then fetched server-side by the share-card renderer, so it clears
	// the same SSRF host guard the direct show/venue/label endpoints apply.
	// Enforced here at queue-create so a hostile value never reaches the queue.
	// NOTHING re-applies it at fulfillment — validatePayloadImageURL has exactly
	// two call sites, this one and the decide handler's pre-claim check — which is
	// why that check's read has to be the one the claim commits against.
	if err := validatePayloadImageURL(ctx, entityType, req.Body.Payload); err != nil {
		return nil, err
	}

	// Normalize the optional source detail (trim, drop empties) and cap its
	// fields at the trust boundary. An all-empty detail becomes nil so the row
	// stores NULL rather than an empty object.
	sourceDetail, err := normalizeSourceDetail(req.Body.SourceDetail)
	if err != nil {
		return nil, err
	}

	// Every check above this line runs before a replacement too, so a
	// resubmission that fails validation is a 422 that leaves the queued payload
	// exactly as it was (PSY-1948).
	created, replaced, err := h.entityRequestService.CreateRequest(user, entityType, req.Body.Payload, sourceContext, sourceDetail, req.Body.Confirmed)
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
	// CreatedEntityID == nil guard keeps an already-fulfilled row from being
	// fulfilled twice.
	//
	// !replaced states the invariant rather than relying on it: a replacement only
	// ever writes a PENDING row and the service builds the returned row from the
	// one it matched, so DecisionState is pending by construction and this branch
	// is already unreachable for a replacement. The guard is what keeps that true
	// if the service ever re-reads the row again — a read landing after an admin's
	// approval but before their RecordFulfillment returns 'approved' with no
	// created_entity_id, and the CONTRIBUTOR's request would fulfill the admin's
	// approval. A replacement is never an auto-approval, so it never fulfills.
	if !replaced && created.DecisionState == communitym.EntityRequestStateApproved && created.CreatedEntityID == nil {
		// nil show-associations: only the admin decide endpoint can supply them
		// (PSY-1037), so an auto-approved show defers below.
		if _, ferr := h.fulfillAndRecord(ctx, created, nil); ferr != nil {
			if isFulfillUnsupported(ferr) {
				// show auto-approve: the request is filed-and-approved, but a
				// show's catalog Create needs admin-supplied venue + artist
				// associations, which only the admin decide endpoint collects
				// (PSY-1037). Leave it approved-but-unfulfilled rather than fail
				// the whole request. The admin queue lists only PENDING rows and
				// Decide only re-processes pending rows, so the recovery path is
				// the PSY-1088 rescue endpoint (POST
				// /admin/entity-requests/{id}/fulfill), which names this exact row
				// shape as one of its two by-design sources and is the only route
				// that both creates the show and keeps the request linked to it.
				// Creating the show directly instead orphans the request. The Warn
				// below is the operational signal.
				// (Festival fulfills inline, so it never reaches here.)
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
	//
	// PSY-1948: a replacement gets its own action because it OVERWRITES a stored
	// payload. Nothing else records that the queued submission an admin is about
	// to moderate is not the one originally filed, and the row keeps no history of
	// what it replaced.
	if h.auditLogService != nil {
		action := "queue_entity_request"
		switch {
		case replaced:
			action = "replace_entity_request"
		case created.DecisionState == communitym.EntityRequestStateApproved:
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

	return &CreateEntityRequestResponse{Body: &CreateEntityRequestResponseBody{
		EntityRequestFields: (*EntityRequestFields)(created),
		Replaced:            replaced,
	}}, nil
}

// ============================================================================
// Admin: List entity requests — GET /admin/entity-requests
// ============================================================================

// AdminListEntityRequestsRequest is the Huma request for GET /admin/entity-requests.
type AdminListEntityRequestsRequest struct {
	State         string `query:"state" required:"false" doc:"Filter by decision state (pending, approved, rejected); defaults to pending"`
	EntityType    string `query:"entity_type" required:"false" doc:"Filter by entity type (artist, venue, label, release, show, festival)"`
	SourceContext string `query:"source_context" required:"false" doc:"Filter by source context (ai_extraction, paste_mode, manual)"`
	// Unfulfilled (PSY-1088), when true, narrows to approved-but-unfulfilled
	// rows (created_entity_id IS NULL) — the rescue "needs attention" queue.
	// Pair with state=approved. A bare bool query param (not a pointer — Huma
	// panics on pointer params); false / omitted = no narrowing.
	Unfulfilled bool `query:"unfulfilled" required:"false" doc:"Narrow to approved-but-unfulfilled rows (pair with state=approved)"`
	Limit       int  `query:"limit" required:"false" minimum:"1" maximum:"100" doc:"Max results (default 20, max 100)"`
	Offset      int  `query:"offset" required:"false" minimum:"0" doc:"Offset for pagination"`
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
	// UpdatedAt is the only field that distinguishes a request whose payload the
	// contributor has since corrected from one that still holds what they first
	// filed (PSY-1948). created_at does not move on a replacement, so a queue
	// that shows only "filed 2 hours ago" cannot tell the two apart.
	UpdatedAt time.Time `json:"updated_at"`
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
		UpdatedAt:         r.UpdatedAt,
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
		Unfulfilled:   req.Unfulfilled,
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
// approve time (PSY-1037). Name+City+State find-or-create the venue
// (admin-created venues are auto-verified by the show service); there is
// deliberately no ID field — the inline admin form has no venue picker, and
// find-or-create by name is idempotent against existing venues.
type ShowVenueInput struct {
	Name    string  `json:"name" doc:"Venue name"`
	City    string  `json:"city" doc:"Venue city"`
	State   string  `json:"state" doc:"Venue state"`
	Address *string `json:"address,omitempty" required:"false" doc:"Street address (optional)"`
}

// ShowArtistInput is one admin-supplied artist for fulfilling a show request
// at approve time (PSY-1037). Name is always required (the show service's
// duplicate-headliner pre-check matches on name); ID optionally pins an
// existing artist, otherwise Name find-or-creates one (case-insensitive).
//
// PSY-1705: set_type carries the curated bill role, so a request whose source
// states a support act or a DJ can be fulfilled without flattening that role.
// It is authoritative over is_headliner when present (the show service derives
// the headliner flag from it).
//
// The one place bill ORDER still has a say: on a bill where no entry states
// EITHER field, the first act is read as the headliner. Only a stated set_type
// arms the suppression, though -- as soon as any entry states one,
// buildShowAssociations suppresses that inference for the whole bill (see
// suppressPositionInference), because a stated bill is a complete statement and
// first-in-list is not a second opinion. So "omit set_type" means 'performer' on
// a curated bill.
//
// KNOWN GAP, described here rather than promised away: only set_type arms that
// suppression. billIsCurated never reads is_headliner, so a bill stated purely
// through the legacy flag -- [{Earth}, {Boris, is_headliner:true}] -- still has
// Earth inferred into the headline slot and writes TWO set_type='headliner' rows.
// That is the PSY-1860 defect in this endpoint's own spelling; PSY-1860 fixed it
// next door on PUT /shows/{show_id} and this one is reported as a follow-up.
type ShowArtistInput struct {
	ID          *uint   `json:"id,omitempty" required:"false" doc:"Existing artist ID (optional)"`
	Name        string  `json:"name" doc:"Artist name (required)"`
	IsHeadliner *bool   `json:"is_headliner,omitempty" required:"false" doc:"Headliner flag. Ignored when set_type is present, which is authoritative. On a bill where no entry states either field, the first entry is read as the headliner. Stating this flag alone does NOT stop that inference (known gap): only a stated set_type settles the bill."`
	SetType     *string `json:"set_type,omitempty" required:"false" enum:"headliner,direct_support,opener,special_guest,dj,performer" doc:"Curated bill role, authoritative over is_headliner. Omit the key when the slot is not known: the act then stores 'performer', meaning 'on the bill, slot unknown', which must not be rendered as a role. Do NOT send an empty string; only an absent key means unknown. Stating this field on any entry settles the whole bill, so no other entry is then inferred from list position; stating is_headliner alone does not."`
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
		ShowArtists []ShowArtistInput `json:"show_artists,omitempty" required:"false" doc:"Artists for fulfilling a show request (required when approving a show, unless use_payload_artists adopts the bill the request payload carries)"`
		// UsePayloadArtists is the admin's affirmative adoption of the bill the
		// CONTRIBUTOR recorded (PSY-1858). See resolveShowBill for the rule and
		// why the flag exists rather than an omitted show_artists meaning the
		// same thing.
		UsePayloadArtists bool `json:"use_payload_artists,omitempty" required:"false" doc:"Approve a show using the artists stored on the request's own payload. Mutually exclusive with show_artists: send one or the other, never both. Omitting both is still a 422, so a bill is never adopted by default. An adopted bill never designates a headliner by list order: an act with no set_type is stored as 'performer', so a bill naming no 'headliner' creates a show with no headliner row."`
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
	// approved-but-unfulfilled row. Rejections ignore the association fields
	// entirely (no spurious 422 for a reject that happens to carry them), and
	// skip the pre-claim read below along with them.
	//
	// SCOPE OF THESE CHECKS, since PSY-1948 made a queued payload mutable: they
	// describe the payload as of THIS read, and NOTHING re-runs them after the
	// claim — validatePayloadImageURL is deliberately not called from
	// fulfillEntity, and buildShowAssociations reads the bill resolved here. So
	// the claim is OPTIMISTIC: reviewedVersion carries this read's updated_at into
	// Decide, and a row the requester replaced in between refuses with a 409
	// instead of fulfilling a payload no check ever saw. Without that the SSRF
	// host guard is bypassable by resubmitting during the DNS resolution it
	// performs, and an adopted show bill can be fulfilled against scalar fields
	// from a different payload.
	//
	// This closes the window between THIS read and the claim. It does NOT close
	// the window between the admin READING the queue and pressing approve: that
	// body carries no version, so the payload can change under a human review.
	// PSY-1975 covers surfacing a revision in the queue.
	//
	// PSY-1037: approving a show REQUIRES the associations — guard before the
	// claim. Decide only operates on pending rows, so a post-claim failure
	// would leave an approved-but-unfulfilled row no decide call can ever
	// re-process. Costs one PK read, only on the approve path. Every check that
	// reads the stored row is scoped to PENDING rows, so nothing the row contains
	// can pre-empt Decide's 409 (invalid state) on an already-decided one.
	//
	// PSY-1675 rides on the same pre-claim read for the same reason: a stored
	// image_url pointing at an internal address must not be fulfilled, and a
	// post-claim rejection would strand the row (a CLAIMED row's payload can no
	// longer be corrected, and the rescue path re-enters the same fulfiller),
	// leaving void — which discards the contributor's request and their
	// attribution — as the only way out. Checked here, a hostile flyer is a clean
	// 422 on a row that is still pending.
	//
	// PSY-1858 is why the read now happens BEFORE the association build rather
	// than after it: an adopted bill is read off the stored payload, so the
	// conversion needs the row in hand. Nothing else moved. A
	// read that ERRORS still reports ahead of any complaint about the body, and a
	// read that finds nothing (GetRequest answers (nil, nil) for a missing row)
	// still falls through to Decide, which is what turns it into the 404.
	// nil for a rejection: it reads nothing off the row and creates nothing, so
	// there is no validated version to defend.
	var reviewedVersion *time.Time
	var showAssoc *showAssociations
	if newState == communitym.EntityRequestStateApproved {
		existing, gerr := h.entityRequestService.GetRequest(uint(requestID))
		if gerr != nil {
			if mapped := shared.MapEntityRequestError(gerr); mapped != nil {
				return nil, mapped
			}
			logger.FromContext(ctx).Error("entity_request_preclaim_load_failed",
				"request_id", requestID,
				"error", gerr.Error(),
			)
			return nil, huma.Error500InternalServerError("Failed to load request")
		}

		// eligible is the row when the checks that READ IT may act on it, and nil
		// otherwise. Decide claims only PENDING rows, so for anything else the
		// honest answer is its 409 (invalid state); a 422 about a stored payload
		// the admin never sent would report the wrong problem entirely and read as
		// though re-sending the request with a bill could fix it. Adoption is
		// scoped by the same value, which is the whole reason it is one variable.
		//
		// This does NOT make an already-decided row immune to every 422: the
		// admin's own body is still shape-checked first, exactly as it was before
		// any of this, so an approve supplying a venue and no bill is refused on
		// its own merits whatever state the row is in. What the gate buys is that
		// nothing the STORED row contains can produce that refusal.
		var eligible *communitym.EntityRequest
		if existing != nil && existing.DecisionState == communitym.EntityRequestStatePending {
			eligible = existing
			// The version every check below is about to run against. Passed to the
			// claim so a row the requester revised in between refuses instead of
			// fulfilling a payload none of these checks ever saw (PSY-1948).
			reviewedVersion = &eligible.UpdatedAt
		}

		// EVERY pre-claim check is inside this block, so a row Decide cannot act
		// on skips all of them and gets answered by its own state: 409 for an
		// already-decided row, 404 for one that is not there.
		//
		// The whole block is gated, not just the checks that read the payload,
		// because use_payload_artists made a body that omits show_artists a
		// COMPLETE request (PSY-1858). Validating the body's shape here would
		// refuse a complete adopting approve with "show_artists is missing" on a
		// row whose real problem is that it was decided a second ago, which is a
		// message that reads as fixable and is not.
		if eligible != nil {
			// PSY-1858: one shared pre-claim admission check with the rescue
			// endpoint (validate the stored bill, resolve body-vs-flag, build), so
			// the two admin paths cannot answer the same bill differently.
			var aerr error
			showAssoc, aerr = admitShowBill(eligible, req.Body.ShowVenue, req.Body.ShowArtists, req.Body.UsePayloadArtists)
			if aerr != nil {
				return nil, aerr
			}

			if showAssoc == nil && eligible.EntityType == communitym.EntityRequestShow {
				return nil, huma.Error422UnprocessableEntity(
					"Approving a show requires show_venue, and either show_artists or use_payload_artists")
			}
			// Last of the pre-claim checks because it is the only one that can
			// resolve DNS; the in-memory refusals above should not wait on it.
			if eligible.Payload != nil {
				if verr := validatePayloadImageURL(ctx, eligible.EntityType, *eligible.Payload); verr != nil {
					return nil, verr
				}
			}
		}
	}

	// Claim the decision atomically before any side effect.
	decided, err := h.entityRequestService.Decide(uint(requestID), admin.ID, newState, note, reviewedVersion)
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
		// PSY-1858: record WHICH bill was fulfilled, so an approve that adopted the
		// contributor's stored bill is distinguishable after the fact from one the
		// admin typed and vetted. Nothing else in the row carries that.
		//
		// Gated on the entity type as well as on showAssoc, because a non-show
		// approve carrying stray show_venue + show_artists still builds a
		// showAssoc that fulfillEntity then ignores. Recording a bill_source there
		// would answer "which bill was fulfilled" for an entity that has no bill,
		// which is worse than staying silent in the one field added to make that
		// question answerable.
		if showAssoc != nil && showAssoc.billSource != "" && decided.EntityType == communitym.EntityRequestShow {
			metadata["bill_source"] = string(showAssoc.billSource)
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
// FulfillUnsupported for show) so callers can classify it: the auto-approve
// create path checks isFulfillUnsupported (and swallows it), the admin decide
// path routes through mapFulfillmentError. Any NEW caller must do one of the
// two, or a typed fulfillment error degrades to a raw 500. Used by both paths
// so they record fulfillment identically.
func (h *EntityRequestHandler) fulfillAndRecord(ctx context.Context, req *communitym.EntityRequest, showAssoc *showAssociations) (uint, error) {
	createdID, err := h.fulfillEntity(ctx, req, showAssoc)
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
