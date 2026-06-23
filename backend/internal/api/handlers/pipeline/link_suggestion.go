package pipeline

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/contracts"
)

// LinkSuggestionHandler owns the admin music-link suggestion review queue
// surface (PSY-1199): list pending suggestions, accept (write the link), reject.
// Admin-gated via the rc.Admin middleware (PSY-423); these handlers carry no
// inline admin check.
type LinkSuggestionHandler struct {
	suggestionService contracts.LinkSuggestionServiceInterface
	auditLogService   contracts.AuditLogServiceInterface
}

// NewLinkSuggestionHandler wires the suggestion service + audit-log dependency.
func NewLinkSuggestionHandler(
	suggestionService contracts.LinkSuggestionServiceInterface,
	auditLogService contracts.AuditLogServiceInterface,
) *LinkSuggestionHandler {
	return &LinkSuggestionHandler{
		suggestionService: suggestionService,
		auditLogService:   auditLogService,
	}
}

// ──────────────────────────────────────────────
// GET /admin/link-suggestions
// ──────────────────────────────────────────────

// ListLinkSuggestionsRequest is the Huma request shape for the paginated list.
type ListLinkSuggestionsRequest struct {
	Limit  int `query:"limit" default:"50" minimum:"1" maximum:"200" doc:"Number of suggestions to return (1–200; default 50)"`
	Offset int `query:"offset" default:"0" minimum:"0" doc:"Pagination offset"`
}

// ListLinkSuggestionsResponse wraps the service result for OpenAPI.
type ListLinkSuggestionsResponse struct {
	Body contracts.LinkSuggestionListResult `json:"body"`
}

// ListLinkSuggestionsHandler handles GET /admin/link-suggestions. Returns
// pending suggestions joined to their artist, high-confidence first.
func (h *LinkSuggestionHandler) ListLinkSuggestionsHandler(ctx context.Context, req *ListLinkSuggestionsRequest) (*ListLinkSuggestionsResponse, error) {
	requestID := logger.GetRequestID(ctx)

	logger.FromContext(ctx).Debug("admin_link_suggestions_list_attempt",
		"limit", req.Limit,
		"offset", req.Offset,
	)

	result, err := h.suggestionService.ListPendingSuggestions(req.Limit, req.Offset)
	if err != nil {
		logger.FromContext(ctx).Error("admin_link_suggestions_list_failed",
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to list link suggestions (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Debug("admin_link_suggestions_list_success",
		"count", len(result.Suggestions),
		"total", result.Total,
	)

	return &ListLinkSuggestionsResponse{Body: *result}, nil
}

// ──────────────────────────────────────────────
// POST /admin/link-suggestions/{id}/accept
// ──────────────────────────────────────────────

// AcceptLinkSuggestionRequest is the Huma request shape. There is no body.
type AcceptLinkSuggestionRequest struct {
	ID string `path:"id" validate:"required" doc:"Suggestion ID"`
}

// LinkSuggestionReviewResponse wraps the review result (shared by accept/reject).
type LinkSuggestionReviewResponse struct {
	Body contracts.LinkSuggestionReviewResult `json:"body"`
}

// AcceptLinkSuggestionHandler handles POST /admin/link-suggestions/{id}/accept.
// Writes the suggested link via the existing artist update path (Spotify →
// social.spotify; Bandcamp → social.bandcamp + the PSY-1190 resolver), marks the
// row accepted, and stamps the reviewer. Idempotent on replay.
func (h *LinkSuggestionHandler) AcceptLinkSuggestionHandler(ctx context.Context, req *AcceptLinkSuggestionRequest) (*LinkSuggestionReviewResponse, error) {
	return h.review(ctx, req.ID, true)
}

// ──────────────────────────────────────────────
// POST /admin/link-suggestions/{id}/reject
// ──────────────────────────────────────────────

// RejectLinkSuggestionRequest is the Huma request shape. There is no body.
type RejectLinkSuggestionRequest struct {
	ID string `path:"id" validate:"required" doc:"Suggestion ID"`
}

// RejectLinkSuggestionHandler handles POST /admin/link-suggestions/{id}/reject.
// Marks the row rejected and stamps the reviewer. Idempotent on replay.
func (h *LinkSuggestionHandler) RejectLinkSuggestionHandler(ctx context.Context, req *RejectLinkSuggestionRequest) (*LinkSuggestionReviewResponse, error) {
	return h.review(ctx, req.ID, false)
}

// review is the shared accept/reject body: parse the ID, call the service,
// translate typed errors to HTTP codes, and audit the decision. accept selects
// AcceptSuggestion vs RejectSuggestion so the two thin handlers don't duplicate
// the error-mapping + audit logic.
func (h *LinkSuggestionHandler) review(ctx context.Context, idStr string, accept bool) (*LinkSuggestionReviewResponse, error) {
	requestID := logger.GetRequestID(ctx)
	user := middleware.GetUserFromContext(ctx)

	suggestionID, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid suggestion ID")
	}

	action := "reject_link_suggestion"
	if accept {
		action = "accept_link_suggestion"
	}

	logger.FromContext(ctx).Debug("admin_link_suggestion_review_attempt",
		"suggestion_id", suggestionID,
		"action", action,
		"admin_id", user.ID,
	)

	var result *contracts.LinkSuggestionReviewResult
	if accept {
		result, err = h.suggestionService.AcceptSuggestion(uint(suggestionID), user.ID)
	} else {
		result, err = h.suggestionService.RejectSuggestion(uint(suggestionID), user.ID)
	}
	if err != nil {
		if errors.Is(err, contracts.ErrLinkSuggestionNotFound) {
			return nil, huma.Error404NotFound("Link suggestion not found")
		}
		if errors.Is(err, contracts.ErrLinkSuggestionAlreadyReviewed) {
			return nil, huma.Error409Conflict("Link suggestion has already been reviewed with a different verdict")
		}
		if errors.Is(err, contracts.ErrLinkSuggestionInvalidURL) {
			return nil, huma.Error422UnprocessableEntity("Link suggestion URL failed validation (not a valid Spotify artist / Bandcamp profile URL)")
		}
		logger.FromContext(ctx).Error("admin_link_suggestion_review_failed",
			"suggestion_id", suggestionID,
			"action", action,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to %s (request_id: %s)", action, requestID),
		)
	}

	logger.FromContext(ctx).Info("admin_link_suggestion_review_success",
		"suggestion_id", suggestionID,
		"action", action,
		"artist_id", result.ArtistID,
		"new_status", result.Status,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	// Audit the decision for contributor-history + "who reviewed X" reporting.
	if h.auditLogService != nil {
		h.auditLogService.LogAction(user.ID, action, "artist", result.ArtistID, map[string]interface{}{
			"suggestion_id": result.ID,
			"new_status":    result.Status,
		})
	}

	return &LinkSuggestionReviewResponse{Body: *result}, nil
}
