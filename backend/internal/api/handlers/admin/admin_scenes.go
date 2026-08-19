package admin

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/contracts"
)

// MaxSceneTaglineLength caps the authored scene tagline (PSY-1848).
//
// Counted in RUNES, not bytes, to match the VARCHAR(80) column: Postgres
// measures VARCHAR in characters, so a byte-based guard would reject taglines
// the column accepts and let no over-long one through — the wrong half of the
// pair to disagree on. The line is a headline of roughly four to eight words;
// the cap exists so it cannot outgrow the two lines the scene mock allows.
const MaxSceneTaglineLength = 80

// AdminSceneHandler handles admin scene curation.
type AdminSceneHandler struct {
	sceneService    contracts.SceneServiceInterface
	auditLogService contracts.AuditLogServiceInterface
}

// NewAdminSceneHandler creates a new admin scene handler.
func NewAdminSceneHandler(
	sceneService contracts.SceneServiceInterface,
	auditLogService contracts.AuditLogServiceInterface,
) *AdminSceneHandler {
	return &AdminSceneHandler{
		sceneService:    sceneService,
		auditLogService: auditLogService,
	}
}

// UpdateSceneTaglineRequest is the HTTP request for authoring a scene tagline.
//
// Tagline is a POINTER so the two intents stay distinguishable on the wire:
// a string sets it, an explicit `null` clears it. Huma requires body fields by
// default even when they are pointers, so a caller must send the key either
// way — there is no third "leave it alone" state to mistake a clear for.
type UpdateSceneTaglineRequest struct {
	Slug string `path:"slug" validate:"required" doc:"Scene slug (e.g. phoenix-az)" example:"phoenix-az"`
	Body struct {
		Tagline *string `json:"tagline" doc:"Authored tagline shown under the scene heading, max 80 characters. Send null or an empty string to clear it."`
	}
}

// UpdateSceneTaglineResponse is the HTTP response for authoring a scene tagline.
type UpdateSceneTaglineResponse struct {
	Body struct {
		Slug    string  `json:"slug"`
		Tagline *string `json:"tagline"`
	}
}

// UpdateSceneTaglineHandler handles PATCH /admin/scenes/{slug}/tagline.
//
// Admin-only: registered on rc.Admin, so the middleware chain has already
// rejected non-admins before this runs (PSY-423) — no in-handler
// RequireAdmin. Authoring is deliberately admin-only for now; opening it to
// the trusted tier is a separate decision, not an oversight here.
func (h *AdminSceneHandler) UpdateSceneTaglineHandler(ctx context.Context, req *UpdateSceneTaglineRequest) (*UpdateSceneTaglineResponse, error) {
	requestID := logger.GetRequestID(ctx)
	user := middleware.GetUserFromContext(ctx)

	// Normalize at the boundary: trim, then treat blank as "clear". Storing a
	// whitespace-only tagline would render an invisible non-empty line on the
	// page, which is neither the present state nor the absent one.
	tagline := normalizeSceneTagline(req.Body.Tagline)

	if tagline != nil && utf8.RuneCountInString(*tagline) > MaxSceneTaglineLength {
		return nil, huma.Error422UnprocessableEntity(
			fmt.Sprintf("Tagline must be %d characters or fewer", MaxSceneTaglineLength),
		)
	}

	logger.FromContext(ctx).Debug("admin_update_scene_tagline_attempt",
		"scene_slug", req.Slug,
		"admin_id", user.ID,
		"clearing", tagline == nil,
	)

	sceneID, err := h.sceneService.UpdateSceneTagline(req.Slug, tagline)
	if err != nil {
		// An unresolvable slug is a 404, not a 500 — you cannot author a
		// tagline for a city that is not a scene.
		if mapped := shared.MapSceneError(err); mapped != nil {
			return nil, mapped
		}
		logger.FromContext(ctx).Error("admin_update_scene_tagline_failed",
			"scene_slug", req.Slug,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to update scene tagline (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Info("admin_update_scene_tagline_success",
		"scene_slug", req.Slug,
		"scene_id", sceneID,
		"admin_id", user.ID,
		"clearing", tagline == nil,
		"request_id", requestID,
	)

	// LogEntityEdit, not LogAction: this is a direct content edit on a
	// knowledge-graph entity, which entity_edit_audit_logs is the canonical
	// writer for. The tagline text itself is not recorded — the audit answers
	// who changed what and when, and the current value is one read away.
	h.auditLogService.LogEntityEdit(user.ID, "scene", sceneID, map[string]interface{}{
		"field":    "tagline",
		"slug":     req.Slug,
		"cleared":  tagline == nil,
		"scene_id": sceneID,
	})

	resp := &UpdateSceneTaglineResponse{}
	resp.Body.Slug = req.Slug
	resp.Body.Tagline = tagline
	return resp, nil
}

// normalizeSceneTagline trims the incoming tagline and collapses the blank
// forms — nil, "", and whitespace-only — into a single nil meaning "clear".
func normalizeSceneTagline(raw *string) *string {
	if raw == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*raw)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
