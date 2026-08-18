package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
	adminm "psychic-homily-backend/internal/models/admin"
	authm "psychic-homily-backend/internal/models/auth"
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
// UserName is never empty (resolveRevisionUserName chain).
// UserUsername is nil when no username is set — distinct from UserName so
// the frontend can decide between a /users/:username link and plain text.
type RevisionResponseItem struct {
	ID           uint                 `json:"id"`
	EntityType   string               `json:"entity_type"`
	EntityID     uint                 `json:"entity_id"`
	UserID       uint                 `json:"user_id"`
	UserName     string               `json:"user_name,omitempty"`
	UserUsername *string              `json:"user_username"`
	Changes      []adminm.FieldChange `json:"changes"`
	Summary      string               `json:"summary,omitempty"`
	CreatedAt    string               `json:"created_at"`
}

// resolveRevisionUserName returns the display name for a revision's author,
// never empty. Resolution chain: username → first/last → email-prefix →
// "Anonymous". Mirrors resolveCommentAuthorName (PSY-552) and
// CollectionService.resolveUserName (PSY-353); operates on the preloaded
// User so there's no extra query per revision.
func resolveRevisionUserName(u *authm.User) string {
	if u == nil || u.ID == 0 {
		return "Anonymous"
	}
	if u.Username != nil && *u.Username != "" {
		return *u.Username
	}
	if u.FirstName != nil && *u.FirstName != "" {
		name := *u.FirstName
		if u.LastName != nil && *u.LastName != "" {
			name += " " + *u.LastName
		}
		return name
	}
	if u.Email != nil && *u.Email != "" {
		if idx := strings.Index(*u.Email, "@"); idx > 0 {
			return (*u.Email)[:idx]
		}
	}
	return "Anonymous"
}

// resolveRevisionUserUsername returns the URL-safe username slug, or nil
// when the user has no username set. Distinct from resolveRevisionUserName,
// whose fallback to first/last/email can't be used in a /users/:username
// link. Mirrors resolveCommentAuthorUsername (PSY-552).
func resolveRevisionUserUsername(u *authm.User) *string {
	if u == nil || u.ID == 0 {
		return nil
	}
	if u.Username == nil || *u.Username == "" {
		return nil
	}
	username := *u.Username
	return &username
}

// revisionViewerIsAdmin resolves the caller tier the three read routes serve.
//
// The routes are PUBLIC but optionally authenticated: they sit behind
// OptionalHumaJWTMiddleware, which attaches a user when it can validate a
// session JWT or an API token and otherwise lets the request through with no
// user at all. So a nil user is the normal anonymous case, not an error, and
// every way of failing to prove admin — no credential, an invalid or expired
// one, an inactive or deleted account, a valid credential for a non-admin —
// lands on the same false. That is the masked tier, so the gate fails closed.
//
// IsAdmin is read off the user the middleware attached, which JWTService
// loads from the database during validation rather than trusting a claim.
//
// Mirrors the shape used for optional-auth reads elsewhere — the closest
// exemplar is catalog.ShowHandler.GetShowHandler, whose route is likewise
// registered on an optional-auth group. Named here rather than re-spelled
// because three handlers need it, and an inline `user != nil && user.IsAdmin`
// is the shape a later edit drops the nil check from.
func revisionViewerIsAdmin(ctx context.Context) bool {
	user := middleware.GetUserFromContext(ctx)
	return user != nil && user.IsAdmin
}

// mapRevisionToResponse converts a adminm.Revision to a RevisionResponseItem.
//
// All three read routes are public, and optionally authenticated: anonymous and
// non-admin callers see the redacted view, admins see the stored one
// (PSY-1717). What may appear in Changes AND in Summary is decided upstream:
// RevisionService applies the read-time privacy redaction for the tier the
// handler passes it (see applyPrivacyRedaction), and this function publishes the
// result verbatim. Do not read revisions.field_changes or revisions.summary into
// a response through any other path, and do not add a tier check here — this
// function cannot see the caller, which is what keeps the policy in one place.
//
// Summary is covered by that gate too, but differently from the diff: a gated
// revision arrives here with a nil Summary, which Deref turns into "" and
// omitempty drops from the payload. There is no mask string to recognise, so an
// empty summary here means EITHER the contributor wrote none or the revision is
// gated. Do not add a branch that tries to tell them apart. The whole point is
// that the response cannot.
func mapRevisionToResponse(r adminm.Revision) RevisionResponseItem {
	item := RevisionResponseItem{
		ID:           r.ID,
		EntityType:   r.EntityType,
		EntityID:     r.EntityID,
		UserID:       r.UserID,
		UserName:     resolveRevisionUserName(&r.User),
		UserUsername: resolveRevisionUserUsername(&r.User),
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

	revisions, total, err := h.revisionService.GetEntityHistory(
		req.EntityType, uint(entityID), limit, req.Offset, revisionViewerIsAdmin(ctx))
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
		items = append(items, mapRevisionToResponse(r))
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

	revision, err := h.revisionService.GetRevision(uint(revisionID), revisionViewerIsAdmin(ctx))
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

	item := mapRevisionToResponse(*revision)
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

	revisions, total, err := h.revisionService.GetUserRevisions(
		uint(userID), limit, req.Offset, revisionViewerIsAdmin(ctx))
	if err != nil {
		logger.FromContext(ctx).Error("revision_get_user_revisions_failed",
			"user_id", userID,
			"error", err.Error(),
		)
		return nil, huma.Error500InternalServerError("Failed to get user revisions")
	}

	items := make([]RevisionResponseItem, 0, len(revisions))
	for _, r := range revisions {
		items = append(items, mapRevisionToResponse(r))
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
type RollbackRevisionResponse struct {
	Body struct {
		Success bool `json:"success"`
	}
}

// RollbackRevisionHandler handles POST /admin/revisions/{revision_id}/rollback
func (h *RevisionHandler) RollbackRevisionHandler(ctx context.Context, req *RollbackRevisionRequest) (*RollbackRevisionResponse, error) {
	user := middleware.GetUserFromContext(ctx)

	revisionID, err := strconv.ParseUint(req.RevisionID, 10, 64)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid revision ID")
	}

	if err := h.revisionService.Rollback(uint(revisionID), user.ID); err != nil {
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
	)

	// Fire-and-forget audit log
	if h.auditLogService != nil {
		servicesshared.GoSafe(ctx, "audit_log", func() {
			h.auditLogService.LogAction(user.ID, "revision_rollback", "revision", uint(revisionID), map[string]interface{}{
				"revision_id": revisionID,
			})
		})
	}

	resp := &RollbackRevisionResponse{}
	resp.Body.Success = true
	return resp, nil
}
