package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
	adminm "psychic-homily-backend/internal/models/admin"
	"psychic-homily-backend/internal/services/contracts"
	servicesshared "psychic-homily-backend/internal/services/shared"
)

// RevisionHandler handles revision history API endpoints.
type RevisionHandler struct {
	revisionService contracts.RevisionServiceInterface
	auditLogService contracts.AuditLogServiceInterface
}

// NewRevisionHandler creates a new revision handler.
func NewRevisionHandler(
	revisionService contracts.RevisionServiceInterface,
	auditLogService contracts.AuditLogServiceInterface,
) *RevisionHandler {
	return &RevisionHandler{
		revisionService: revisionService,
		auditLogService: auditLogService,
	}
}

// validEntityTypes lists the allowed entity type values.
var validEntityTypes = map[string]bool{
	"artist":   true,
	"venue":    true,
	"show":     true,
	"release":  true,
	"label":    true,
	"festival": true,
}

// --- Response Types ---

// RevisionResponseItem represents a single revision in API responses.
//
// UserName is ABSENT — not empty, not a placeholder — when the author may not
// be named on this view: they hid their contributions, or their only resolvable
// name would come from their email address. See mapRevisionToResponse. Clients
// must render the row without a byline in that case ("edited Jul 12", no "by"),
// never substitute "Anonymous": the absence means "we may not say", and a
// placeholder would assert a person.
//
// UserUsername is nil when there is no profile to link to — no username set, a
// private profile, or a suppressed credit. Distinct from UserName so the
// frontend can decide between a /users/:username link and plain text.
//
// UserID GOES WITH THE NAME. A suppressed credit omits the numeric id too,
// because on its own the id is not anonymity — it is a lookup key. Several
// public, unauthenticated payloads publish a user id and a display name in the
// same object (a comment carries user_id + author_name; a collection carries
// creator_id + creator_name), so one anonymous request builds an id-to-name
// directory and the withheld byline is recovered in the next lookup.
//
// Withholding it RAISES THE COST of recovery; it is not the whole control. The
// author id of a suppressed revision was also recoverable by scanning
// GET /users/{id}/revisions for a page containing that revision's id, which is
// why that route now refuses a hidden contributor's listing outright
// (requireAuthorContributionsVisible). Both halves are needed: this one so the
// id is not simply handed over, that one so it cannot be searched for.
//
// This deliberately does NOT match the show submitter byline, which suppresses
// submitted_by_name while keeping submitted_by. The difference is that
// submitted_by is LOAD-BEARING there: the frontend runs ownership checks on it
// (ShowDetail, ShowCard, VenueDetail, the submissions console). A revision's
// user_id has no consumer at all, so withholding it costs nothing and closes
// the recovery. Do not "restore consistency" by re-publishing it.
type RevisionResponseItem struct {
	ID           uint                 `json:"id"`
	EntityType   string               `json:"entity_type"`
	EntityID     uint                 `json:"entity_id"`
	UserID       *uint                `json:"user_id,omitempty"`
	UserName     string               `json:"user_name,omitempty"`
	UserUsername *string              `json:"user_username"`
	Changes      []adminm.FieldChange `json:"changes"`
	Summary      string               `json:"summary,omitempty"`
	CreatedAt    string               `json:"created_at"`
}

// revisionAuthorCredit resolves the byline a revision may carry for its author,
// for the tier this caller is served.
//
// ADMIN reads the stored identity through the canonical chain
// (servicesshared.ResolveUserName), the same whole-view unmasking PSY-1717
// grants over field values and summaries: a moderator deciding on a rollback
// needs to know whose edit it is, and the admin surfaces already resolve
// contributors this way (the moderation queue, the audit log, entity reports).
//
// EVERYONE ELSE gets the public contribution credit, which fails closed on
// privacy_settings.contributions and never publishes an email-derived name
// (servicesshared.ResolvePublicContributorCredit — the gates are argued there).
// A suppressed credit is an ABSENT name, not "Anonymous"; see
// RevisionResponseItem.
//
// What the admin tier unmasks is the NAME, and only the name. The profile link
// runs through the same ContributorProfileLink both tiers use, because a private
// profile 404s for admins too — withholding that link is not a privacy rule to
// waive, it is the difference between a working href and a broken one.
//
// This is the ONLY place the two tiers diverge for identity. Content redaction
// is decided upstream in the service (applyPrivacyRedaction) off the same
// viewer, so the two policies read the same fact and cannot disagree about who
// is asking.
//
// Do not narrow this route's Preload("User") with a Select. The two tiers here
// need DIFFERENT columns — the public one needs privacy_settings and
// profile_visibility, the admin one needs email — so neither helper's own
// column contract is the contract for this route. The union, and what each
// omission silently breaks, is written beside the queries themselves: see the
// AUTHOR COLUMN CONTRACT note at the top of services/admin/revision.go.
func revisionAuthorCredit(r *adminm.Revision, viewer contracts.RevisionViewer) servicesshared.PublicContributorCredit {
	if viewer.IsAdmin {
		return servicesshared.PublicContributorCredit{
			Name:     servicesshared.ResolveUserName(&r.User),
			Username: servicesshared.ContributorProfileLink(&r.User),
		}
	}
	return servicesshared.ResolvePublicContributorCredit(&r.User)
}

// revisionViewer resolves the caller the three read routes serve.
//
// The routes are PUBLIC but optionally authenticated: they sit behind
// OptionalHumaJWTMiddleware, which attaches a user when it can validate a
// session JWT or an API token and otherwise lets the request through with no
// user at all. So a nil user is the normal anonymous case, not an error, and
// every way of failing to prove identity — no credential, an invalid or expired
// one, an inactive or deleted account — lands on the zero viewer, which is
// neither an admin nor anybody's submitter. Both gates that read it fail
// closed on exactly that value.
//
// Identity is read off the user the middleware attached, which JWTService
// loads from the database during validation rather than trusting a claim. So a
// demoted admin, a deactivated account and a deleted user all lose their tier
// on the next request rather than carrying it in a live token.
//
// Every optional-auth read that gates on a show reduces its caller the same way,
// so the reduction lives in one place (PSY-1939) and this is the name the three
// revision handlers reach it by. contracts.RevisionViewer is an alias of the
// ShowViewer that resolver returns, so this is a rename, not a conversion.
func revisionViewer(ctx context.Context) contracts.RevisionViewer {
	return middleware.GetShowViewerFromContext(ctx)
}

// mapRevisionToResponse converts a adminm.Revision to a RevisionResponseItem
// for the given caller.
//
// All three read routes are public, and optionally authenticated: anonymous and
// non-admin callers see the redacted view, admins see the stored one
// (PSY-1717). What may appear in Changes AND in Summary is decided upstream:
// RevisionService applies the read-time privacy redaction for the tier the
// handler passes it (see applyPrivacyRedaction), and this function publishes the
// result verbatim. Do not read revisions.field_changes or revisions.summary into
// a response through any other path, and do not add a CONTENT tier check here —
// that policy lives in one place, in the service.
//
// The viewer parameter decides ONE thing here and nothing else: how the author
// is credited (revisionAuthorCredit, PSY-1940). It is passed explicitly rather
// than read off a context so this function stays a pure mapping that a test can
// drive both tiers of, and so a future reader cannot mistake it for a place
// where content policy may also be re-decided.
//
// The show gate (PSY-1715) never reaches this function at all: a revision the
// caller may not see is not passed here to be masked, it is not returned. So
// everything below describes rows that have already been cleared for serving.
//
// Summary is covered by that gate too, but differently from the diff: a gated
// revision arrives here with a nil Summary, which Deref turns into "" and
// omitempty drops from the payload. There is no mask string to recognise, so an
// empty summary here means EITHER the contributor wrote none or the revision is
// gated. Do not add a branch that tries to tell them apart. The whole point is
// that the response cannot.
func mapRevisionToResponse(r adminm.Revision, viewer contracts.RevisionViewer) RevisionResponseItem {
	credit := revisionAuthorCredit(&r, viewer)
	// One decision, three fields. The id is part of the credit, not metadata
	// beside it — see RevisionResponseItem on why publishing it would undo the
	// suppression.
	var userID *uint
	if credit.Renderable() {
		id := r.UserID
		userID = &id
	}
	item := RevisionResponseItem{
		ID:           r.ID,
		EntityType:   r.EntityType,
		EntityID:     r.EntityID,
		UserID:       userID,
		UserName:     credit.Name,
		UserUsername: credit.Username,
		// PSY-604: must convert to UTC before formatting — the literal "Z"
		// in the layout asserts the value is UTC but Format does not convert.
		// A local time.Time would otherwise be stamped with "Z" while still
		// carrying the local clock reading, drifting relative-time renders by
		// the local UTC offset (e.g. 7h on Phoenix MST).
		CreatedAt: r.CreatedAt.UTC().Format(time.RFC3339),
	}

	item.Summary = shared.Deref(r.Summary)

	// Unmarshal field changes from JSONB
	if r.FieldChanges != nil {
		var changes []adminm.FieldChange
		if err := json.Unmarshal(*r.FieldChanges, &changes); err == nil {
			item.Changes = changes
		}
	}
	if item.Changes == nil {
		item.Changes = []adminm.FieldChange{}
	}

	return item
}

// --- GetEntityHistory ---

// GetEntityHistoryRequest is the Huma request for GET /revisions/{entity_type}/{entity_id}
type GetEntityHistoryRequest struct {
	EntityType string `path:"entity_type" doc:"Entity type (artist, venue, show, release, label, festival)"`
	EntityID   string `path:"entity_id" doc:"Entity ID"`
	Limit      int    `query:"limit" required:"false" minimum:"1" maximum:"100" doc:"Max results (default 20, max 100)"`
	Offset     int    `query:"offset" required:"false" minimum:"0" doc:"Offset for pagination"`
}

// GetEntityHistoryResponse is the Huma response for GET /revisions/{entity_type}/{entity_id}
type GetEntityHistoryResponse struct {
	Body struct {
		Revisions []RevisionResponseItem `json:"revisions"`
		Total     int64                  `json:"total"`
	}
}

// GetEntityHistoryHandler handles GET /revisions/{entity_type}/{entity_id}
func (h *RevisionHandler) GetEntityHistoryHandler(ctx context.Context, req *GetEntityHistoryRequest) (*GetEntityHistoryResponse, error) {
	// Validate entity type
	if !validEntityTypes[req.EntityType] {
		return nil, huma.Error422UnprocessableEntity(fmt.Sprintf("Invalid entity type: %s. Must be one of: artist, venue, show, release, label, festival", req.EntityType))
	}

	entityID, err := strconv.ParseUint(req.EntityID, 10, 64)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid entity ID")
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}

	// Resolved once and reused for the response mapping: the tier that decides
	// what content the service serves must be the same one that decides how the
	// author is credited, or a single request could redact on one reading of the
	// caller and attribute on another.
	viewer := revisionViewer(ctx)

	revisions, total, err := h.revisionService.GetEntityHistory(
		req.EntityType, uint(entityID), limit, req.Offset, viewer)
	// An entity this caller may not see answers 404, mirroring the detail route
	// the gate is copied from (PSY-1715). The message says nothing about WHY,
	// and is the same one an entity that does not exist would produce, so the
	// response cannot be used to enumerate unpublished shows. Logged at Info,
	// not Error: a public route refusing a public request is routine.
	if errors.Is(err, contracts.ErrRevisionEntityHidden) {
		logger.FromContext(ctx).Info("revision_history_access_denied",
			"entity_type", req.EntityType,
			"entity_id", entityID,
		)
		return nil, huma.Error404NotFound("Revision history not found")
	}
	if err != nil {
		logger.FromContext(ctx).Error("revision_get_entity_history_failed",
			"entity_type", req.EntityType,
			"entity_id", entityID,
			"error", err.Error(),
		)
		return nil, huma.Error500InternalServerError("Failed to get revision history")
	}

	items := make([]RevisionResponseItem, 0, len(revisions))
	for _, r := range revisions {
		items = append(items, mapRevisionToResponse(r, viewer))
	}

	resp := &GetEntityHistoryResponse{}
	resp.Body.Revisions = items
	resp.Body.Total = total
	return resp, nil
}

// --- GetRevision ---

// GetRevisionRequest is the Huma request for GET /revisions/{revision_id}
type GetRevisionRequest struct {
	RevisionID string `path:"revision_id" doc:"Revision ID"`
}

// GetRevisionResponse is the Huma response for GET /revisions/{revision_id}
type GetRevisionResponse struct {
	Body RevisionResponseItem
}

// GetRevisionHandler handles GET /revisions/{revision_id}
func (h *RevisionHandler) GetRevisionHandler(ctx context.Context, req *GetRevisionRequest) (*GetRevisionResponse, error) {
	revisionID, err := strconv.ParseUint(req.RevisionID, 10, 64)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid revision ID")
	}

	// One reading of the caller for both the service's redaction and the
	// response's byline — see GetEntityHistoryHandler.
	viewer := revisionViewer(ctx)

	revision, err := h.revisionService.GetRevision(uint(revisionID), viewer)
	if err != nil {
		logger.FromContext(ctx).Error("revision_get_failed",
			"revision_id", revisionID,
			"error", err.Error(),
		)
		return nil, huma.Error500InternalServerError("Failed to get revision")
	}
	if revision == nil {
		return nil, huma.Error404NotFound("Revision not found")
	}

	item := mapRevisionToResponse(*revision, viewer)
	return &GetRevisionResponse{Body: item}, nil
}

// --- GetUserRevisions ---

// GetUserRevisionsRequest is the Huma request for GET /users/{user_id}/revisions
type GetUserRevisionsRequest struct {
	UserID string `path:"user_id" doc:"User ID"`
	Limit  int    `query:"limit" required:"false" minimum:"1" maximum:"100" doc:"Max results (default 20, max 100)"`
	Offset int    `query:"offset" required:"false" minimum:"0" doc:"Offset for pagination"`
}

// GetUserRevisionsResponse is the Huma response for GET /users/{user_id}/revisions
type GetUserRevisionsResponse struct {
	Body struct {
		Revisions []RevisionResponseItem `json:"revisions"`
		Total     int64                  `json:"total"`
	}
}

// GetUserRevisionsHandler handles GET /users/{user_id}/revisions
func (h *RevisionHandler) GetUserRevisionsHandler(ctx context.Context, req *GetUserRevisionsRequest) (*GetUserRevisionsResponse, error) {
	userID, err := strconv.ParseUint(req.UserID, 10, 64)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid user ID")
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}

	// One reading of the caller for both the service's redaction and the
	// response's byline — see GetEntityHistoryHandler.
	//
	// Note this route names the author in its PATH, so suppressing the byline
	// here withholds the NAME, not the fact that user {user_id} edited these
	// entities. That residual is the same one the show submitter byline leaves
	// by publishing submitted_by; the per-user counting and differencing family
	// is PSY-1939's.
	viewer := revisionViewer(ctx)

	revisions, total, err := h.revisionService.GetUserRevisions(
		uint(userID), limit, req.Offset, viewer)
	// A contributor who hid their contributions has no public contributions
	// page, and this route is one. Same 404, same message, as a user id that
	// does not exist — mirroring GET /users/{username}/contributions, and
	// leaving nothing to tell the two apart. Info, not Error: a public route
	// refusing a public request is routine.
	if errors.Is(err, contracts.ErrRevisionEntityHidden) {
		logger.FromContext(ctx).Info("revision_user_contributions_hidden",
			"user_id", userID,
		)
		return nil, huma.Error404NotFound("User not found")
	}
	if err != nil {
		logger.FromContext(ctx).Error("revision_get_user_revisions_failed",
			"user_id", userID,
			"error", err.Error(),
		)
		return nil, huma.Error500InternalServerError("Failed to get user revisions")
	}

	items := make([]RevisionResponseItem, 0, len(revisions))
	for _, r := range revisions {
		items = append(items, mapRevisionToResponse(r, viewer))
	}

	resp := &GetUserRevisionsResponse{}
	resp.Body.Revisions = items
	resp.Body.Total = total
	return resp, nil
}

// --- Rollback ---

// RollbackRevisionRequest is the Huma request for POST /admin/revisions/{revision_id}/rollback
type RollbackRevisionRequest struct {
	RevisionID string `path:"revision_id" doc:"Revision ID to rollback"`
}

// RollbackRevisionResponse is the Huma response for POST /admin/revisions/{revision_id}/rollback
//
// SkippedFields is a normal outcome, not an error branch: a rollback restores
// the fields the apply-side gates accept and refuses the rest, so a caller that
// renders only Success tells an admin an edit was undone when part of it was
// not. Both lists are always present.
type RollbackRevisionResponse struct {
	Body struct {
		Success       bool                             `json:"success"`
		AppliedFields []string                         `json:"applied_fields" doc:"Fields restored to their previous values"`
		SkippedFields []contracts.RollbackSkippedField `json:"skipped_fields" doc:"Fields left unchanged, with the reason for each"`
	}
}

// RollbackRevisionHandler handles POST /admin/revisions/{revision_id}/rollback
func (h *RevisionHandler) RollbackRevisionHandler(ctx context.Context, req *RollbackRevisionRequest) (*RollbackRevisionResponse, error) {
	user := middleware.GetUserFromContext(ctx)

	revisionID, err := strconv.ParseUint(req.RevisionID, 10, 64)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid revision ID")
	}

	result, err := h.revisionService.Rollback(ctx, uint(revisionID), user.ID)
	// A nil result with no error is not something the service produces, but the
	// interface does not forbid it and every field below dereferences it. Answer
	// like any other failed rollback rather than panicking the request.
	if err == nil && result == nil {
		err = fmt.Errorf("rollback returned no result")
	}
	if err != nil {
		logger.FromContext(ctx).Error("revision_rollback_failed",
			"revision_id", revisionID,
			"admin_id", user.ID,
			"error", err.Error(),
		)
		return nil, huma.Error422UnprocessableEntity(err.Error())
	}

	logger.FromContext(ctx).Info("revision_rolled_back",
		"revision_id", revisionID,
		"admin_id", user.ID,
		"applied_fields", result.AppliedFields,
		"skipped_fields", result.SkippedFieldNames(),
	)

	// Fire-and-forget audit log. The refused fields belong in the audit row for
	// the same reason they belong in the response: the row is the durable record
	// of what the admin's action did, and one that named only the revision would
	// record a full undo that did not happen.
	if h.auditLogService != nil {
		servicesshared.SubmitAuditWrite("audit_log", func() {
			h.auditLogService.LogAction(user.ID, "revision_rollback", "revision", uint(revisionID), map[string]interface{}{
				"revision_id":    revisionID,
				"applied_fields": result.AppliedFields,
				"skipped_fields": result.SkippedFields,
			})
		})
	}

	resp := &RollbackRevisionResponse{}
	resp.Body.Success = true
	resp.Body.AppliedFields = result.AppliedFields
	resp.Body.SkippedFields = result.SkippedFields
	return resp, nil
}
