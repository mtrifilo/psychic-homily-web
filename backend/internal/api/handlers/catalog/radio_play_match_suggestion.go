package catalog

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/engagement"
	servicesshared "psychic-homily-backend/internal/services/shared"
)

// RadioPlayMatchSuggestionHandler owns community submit + admin review for
// radio play match suggestions (PSY-1494).
type RadioPlayMatchSuggestionHandler struct {
	service         contracts.RadioPlayMatchSuggestionServiceInterface
	auditLogService contracts.AuditLogServiceInterface
	// Optional approval-email deps (nil-safe). Wired so accept can mirror
	// SendEditApprovedEmail without pulling engagement into the catalog service.
	db           *gorm.DB
	emailService contracts.EmailServiceInterface
	frontendURL  string
	backendURL   string
	jwtSecret    string
}

// NewRadioPlayMatchSuggestionHandler wires the suggestion service + audit log.
func NewRadioPlayMatchSuggestionHandler(
	service contracts.RadioPlayMatchSuggestionServiceInterface,
	auditLogService contracts.AuditLogServiceInterface,
) *RadioPlayMatchSuggestionHandler {
	return &RadioPlayMatchSuggestionHandler{
		service:         service,
		auditLogService: auditLogService,
	}
}

// SetApprovalEmailDeps wires optional edit-approved email on accept (PSY-1494).
// Nil-safe: when unset, accept still links + audits but skips email.
func (h *RadioPlayMatchSuggestionHandler) SetApprovalEmailDeps(
	db *gorm.DB,
	emailService contracts.EmailServiceInterface,
	frontendURL, backendURL, jwtSecret string,
) {
	h.db = db
	h.emailService = emailService
	h.frontendURL = frontendURL
	h.backendURL = backendURL
	h.jwtSecret = jwtSecret
}

// ──────────────────────────────────────────────
// POST /radio/plays/{id}/match-suggestions (community, authed)
// ──────────────────────────────────────────────

// CreateRadioPlayMatchSuggestionRequest is the Huma request for community submit.
type CreateRadioPlayMatchSuggestionRequest struct {
	PlayID uint `path:"id" doc:"Radio play ID"`
	Body   contracts.CreateRadioPlayMatchSuggestionRequest
}

// CreateRadioPlayMatchSuggestionResponse wraps the created pending suggestion.
type CreateRadioPlayMatchSuggestionResponse struct {
	Body contracts.RadioPlayMatchSuggestionEntry
}

// CreateRadioPlayMatchSuggestionHandler handles POST /radio/plays/{id}/match-suggestions.
// Creates a pending row only — never mutates radio_plays.
func (h *RadioPlayMatchSuggestionHandler) CreateRadioPlayMatchSuggestionHandler(
	ctx context.Context,
	req *CreateRadioPlayMatchSuggestionRequest,
) (*CreateRadioPlayMatchSuggestionResponse, error) {
	requestID := logger.GetRequestID(ctx)
	user := middleware.GetUserFromContext(ctx)

	logger.FromContext(ctx).Debug("radio_play_match_suggestion_create_attempt",
		"play_id", req.PlayID,
		"artist_id", req.Body.ArtistID,
		"user_id", user.ID,
	)

	entry, err := h.service.CreateSuggestion(req.PlayID, user.ID, &req.Body)
	if err != nil {
		return nil, mapMatchSuggestionError(ctx, err, requestID, "create")
	}

	if h.auditLogService != nil {
		servicesshared.GoSafe(ctx, "audit_log", func() {
			h.auditLogService.LogAction(user.ID, "submit_radio_play_match_suggestion", "radio_play", req.PlayID, map[string]interface{}{
				"suggestion_id":       entry.ID,
				"suggested_artist_id": entry.SuggestedArtistID,
			})
		})
	}

	return &CreateRadioPlayMatchSuggestionResponse{Body: *entry}, nil
}

// ──────────────────────────────────────────────
// GET /radio/plays/{id}/match-suggestions/mine (community, authed)
// ──────────────────────────────────────────────

// GetOwnRadioPlayMatchSuggestionRequest is the Huma request for own-pending GET.
type GetOwnRadioPlayMatchSuggestionRequest struct {
	PlayID uint `path:"id" doc:"Radio play ID"`
}

// GetOwnRadioPlayMatchSuggestionResponse wraps the caller's pending suggestion,
// or an empty body when none exists (200 + suggestion:null).
type GetOwnRadioPlayMatchSuggestionResponse struct {
	Body struct {
		Suggestion *contracts.RadioPlayMatchSuggestionEntry `json:"suggestion"`
	}
}

// GetOwnRadioPlayMatchSuggestionHandler handles GET /radio/plays/{id}/match-suggestions/mine.
func (h *RadioPlayMatchSuggestionHandler) GetOwnRadioPlayMatchSuggestionHandler(
	ctx context.Context,
	req *GetOwnRadioPlayMatchSuggestionRequest,
) (*GetOwnRadioPlayMatchSuggestionResponse, error) {
	requestID := logger.GetRequestID(ctx)
	user := middleware.GetUserFromContext(ctx)

	entry, err := h.service.GetOwnPendingSuggestion(req.PlayID, user.ID)
	if err != nil {
		return nil, mapMatchSuggestionError(ctx, err, requestID, "get_own")
	}

	resp := &GetOwnRadioPlayMatchSuggestionResponse{}
	resp.Body.Suggestion = entry
	return resp, nil
}

// ──────────────────────────────────────────────
// GET /admin/radio/match-suggestions
// ──────────────────────────────────────────────

// ListRadioPlayMatchSuggestionsRequest is the admin pending-list request.
type ListRadioPlayMatchSuggestionsRequest struct {
	Limit  int `query:"limit" default:"50" minimum:"1" maximum:"200" doc:"Page size (1–200; default 50)"`
	Offset int `query:"offset" default:"0" minimum:"0" doc:"Pagination offset"`
}

// ListRadioPlayMatchSuggestionsResponse wraps the pending list.
type ListRadioPlayMatchSuggestionsResponse struct {
	Body contracts.RadioPlayMatchSuggestionListResult
}

// ListRadioPlayMatchSuggestionsHandler handles GET /admin/radio/match-suggestions.
func (h *RadioPlayMatchSuggestionHandler) ListRadioPlayMatchSuggestionsHandler(
	ctx context.Context,
	req *ListRadioPlayMatchSuggestionsRequest,
) (*ListRadioPlayMatchSuggestionsResponse, error) {
	requestID := logger.GetRequestID(ctx)

	result, err := h.service.ListPendingSuggestions(req.Limit, req.Offset)
	if err != nil {
		logger.FromContext(ctx).Error("admin_radio_match_suggestions_list_failed",
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to list match suggestions (request_id: %s)", requestID),
		)
	}
	return &ListRadioPlayMatchSuggestionsResponse{Body: *result}, nil
}

// ──────────────────────────────────────────────
// POST /admin/radio/match-suggestions/{id}/accept
// ──────────────────────────────────────────────

// AcceptRadioPlayMatchSuggestionRequest is the admin accept request.
type AcceptRadioPlayMatchSuggestionRequest struct {
	ID   string `path:"id" validate:"required" doc:"Suggestion ID"`
	Body contracts.AcceptRadioPlayMatchSuggestionRequest
}

// RadioPlayMatchSuggestionReviewResponse wraps accept/reject results.
type RadioPlayMatchSuggestionReviewResponse struct {
	Body contracts.RadioPlayMatchSuggestionReviewResult
}

// AcceptRadioPlayMatchSuggestionHandler handles POST .../accept.
// Calls LinkPlay; optional also_bulk_link_name → BulkLinkPlays. Audited.
func (h *RadioPlayMatchSuggestionHandler) AcceptRadioPlayMatchSuggestionHandler(
	ctx context.Context,
	req *AcceptRadioPlayMatchSuggestionRequest,
) (*RadioPlayMatchSuggestionReviewResponse, error) {
	requestID := logger.GetRequestID(ctx)
	user := middleware.GetUserFromContext(ctx)

	suggestionID, err := strconv.ParseUint(req.ID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid suggestion ID")
	}

	result, err := h.service.AcceptSuggestion(uint(suggestionID), user.ID, &req.Body)
	if err != nil {
		return nil, mapMatchSuggestionError(ctx, err, requestID, "accept")
	}

	if result.NewlyReviewed && h.auditLogService != nil {
		servicesshared.GoSafe(ctx, "audit_log", func() {
			h.auditLogService.LogAction(user.ID, "accept_radio_play_match_suggestion", "radio_play", result.PlayID, map[string]interface{}{
				"suggestion_id":       result.ID,
				"suggested_artist_id": result.SuggestedArtistID,
				"submitted_by":        result.SubmittedBy,
				"also_bulk_link_name": req.Body.AlsoBulkLinkName,
				"bulk_updated":        result.BulkUpdated,
			})
			// Contributor credit: attribute acceptance to the submitter.
			h.auditLogService.LogAction(result.SubmittedBy, "radio_play_match_accepted", "radio_play", result.PlayID, map[string]interface{}{
				"suggestion_id":       result.ID,
				"suggested_artist_id": result.SuggestedArtistID,
				"reviewed_by":         user.ID,
			})
		})
	}

	if result.NewlyReviewed {
		servicesshared.GoSafe(ctx, "radio_play_match_suggestion_approval_email", func() {
			h.sendApprovalEmail(result)
		})
	}

	return &RadioPlayMatchSuggestionReviewResponse{Body: *result}, nil
}

// ──────────────────────────────────────────────
// POST /admin/radio/match-suggestions/{id}/reject
// ──────────────────────────────────────────────

// RejectRadioPlayMatchSuggestionRequest is the admin reject request.
type RejectRadioPlayMatchSuggestionRequest struct {
	ID   string `path:"id" validate:"required" doc:"Suggestion ID"`
	Body contracts.RejectRadioPlayMatchSuggestionRequest
}

// RejectRadioPlayMatchSuggestionHandler handles POST .../reject.
// Stamps rejection_reason. Reject email is optional day-one (not sent).
func (h *RadioPlayMatchSuggestionHandler) RejectRadioPlayMatchSuggestionHandler(
	ctx context.Context,
	req *RejectRadioPlayMatchSuggestionRequest,
) (*RadioPlayMatchSuggestionReviewResponse, error) {
	requestID := logger.GetRequestID(ctx)
	user := middleware.GetUserFromContext(ctx)

	suggestionID, err := strconv.ParseUint(req.ID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid suggestion ID")
	}

	result, err := h.service.RejectSuggestion(uint(suggestionID), user.ID, &req.Body)
	if err != nil {
		return nil, mapMatchSuggestionError(ctx, err, requestID, "reject")
	}

	if result.NewlyReviewed && h.auditLogService != nil {
		servicesshared.GoSafe(ctx, "audit_log", func() {
			h.auditLogService.LogAction(user.ID, "reject_radio_play_match_suggestion", "radio_play", result.PlayID, map[string]interface{}{
				"suggestion_id":    result.ID,
				"submitted_by":     result.SubmittedBy,
				"rejection_reason": result.RejectionReason,
			})
		})
	}

	return &RadioPlayMatchSuggestionReviewResponse{Body: *result}, nil
}

func mapMatchSuggestionError(ctx context.Context, err error, requestID, action string) error {
	switch {
	case errors.Is(err, contracts.ErrRadioPlayMatchSuggestionNotFound):
		return huma.Error404NotFound("Match suggestion not found")
	case errors.Is(err, contracts.ErrRadioPlayMatchSuggestionArtistNotFound):
		return huma.Error404NotFound("Suggested artist not found")
	case errors.Is(err, contracts.ErrRadioPlayMatchSuggestionAlreadyReviewed):
		return huma.Error409Conflict("Match suggestion has already been reviewed with a different verdict")
	case errors.Is(err, contracts.ErrRadioPlayMatchSuggestionDuplicatePending):
		return huma.Error409Conflict("You already have a pending match suggestion for this play")
	case errors.Is(err, contracts.ErrRadioPlayMatchSuggestionPlayNotSuggestable):
		return huma.Error422UnprocessableEntity("Play is not suggestable (must have null artist_id and match_state in unmatched/ambiguous/no_match)")
	case errors.Is(err, contracts.ErrRadioPlayMatchSuggestionRejectReasonRequired):
		return huma.Error422UnprocessableEntity("rejection reason is required")
	case errors.Is(err, contracts.ErrRadioPlayMatchSuggestionNoteTooLong):
		return huma.Error422UnprocessableEntity("note must be at most 2000 characters")
	default:
		logger.FromContext(ctx).Error("radio_play_match_suggestion_failed",
			"action", action,
			"error", err.Error(),
			"request_id", requestID,
		)
		return huma.Error500InternalServerError(
			fmt.Sprintf("Failed to %s match suggestion (request_id: %s)", action, requestID),
		)
	}
}

// sendApprovalEmail mirrors PendingEditService.sendApprovalEmail: fire-and-
// forget, respects notify_on_edit_notifications (opt-OUT), uses
// SendEditApprovedEmail. Reject email is intentionally skipped day-one.
func (h *RadioPlayMatchSuggestionHandler) sendApprovalEmail(result *contracts.RadioPlayMatchSuggestionReviewResult) {
	if h.emailService == nil || !h.emailService.IsConfigured() || h.db == nil || result == nil {
		return
	}

	var user authm.User
	if err := h.db.First(&user, result.SubmittedBy).Error; err != nil {
		log.Printf("radio match suggestion approval email: look up submitter %d: %v", result.SubmittedBy, err)
		return
	}
	if user.Email == nil || *user.Email == "" {
		return
	}
	if !h.editNotificationsEnabled(user.ID) {
		return
	}

	entityName := fmt.Sprintf("artist #%d", result.SuggestedArtistID)
	entityURL := h.frontendURL
	var artist catalogm.Artist
	if err := h.db.Select("id", "name", "slug").First(&artist, result.SuggestedArtistID).Error; err == nil {
		entityName = artist.Name
		if artist.Slug != nil && *artist.Slug != "" {
			entityURL = strings.TrimRight(h.frontendURL, "/") + "/artists/" + *artist.Slug
		}
	}

	username := servicesshared.ResolveUserName(&user)
	unsubURL := engagement.GenerateScopedUnsubscribeURL(
		h.backendURL, user.ID, engagement.UnsubscribeScopeEditNotifications, h.jwtSecret,
	)
	if err := h.emailService.SendEditApprovedEmail(
		*user.Email, username, "artist", entityName, entityURL, unsubURL,
	); err != nil {
		log.Printf("radio match suggestion approval email: send to %s: %v", *user.Email, err)
	}
}

func (h *RadioPlayMatchSuggestionHandler) editNotificationsEnabled(userID uint) bool {
	var prefs authm.UserPreferences
	if err := h.db.Select("notify_on_edit_notifications").
		Where("user_id = ?", userID).First(&prefs).Error; err != nil {
		return true
	}
	return prefs.NotifyOnEditNotifications
}
