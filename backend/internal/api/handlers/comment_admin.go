package handlers

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/contracts"
)

// ============================================================================
// Focused interface for dependency injection
// ============================================================================

// CommentAdmin defines the admin moderation operations for comments.
type CommentAdmin interface {
	HideComment(adminUserID uint, commentID uint, reason string) error
	RestoreComment(adminUserID uint, commentID uint) error
	ListPendingComments(limit, offset int) ([]*contracts.CommentResponse, int64, error)
	ApproveComment(adminUserID uint, commentID uint) error
	RejectComment(adminUserID uint, commentID uint, reason string) error
	GetCommentEditHistory(requesterID uint, commentID uint) (*contracts.CommentEditHistoryResponse, error)
}

// CommentAdminHandler handles admin comment moderation API requests.
type CommentAdminHandler struct {
	admin           CommentAdmin
	auditLogService contracts.AuditLogServiceInterface
}

// NewCommentAdminHandler creates a new CommentAdminHandler.
func NewCommentAdminHandler(admin CommentAdmin, auditLogService contracts.AuditLogServiceInterface) *CommentAdminHandler {
	return &CommentAdminHandler{
		admin:           admin,
		auditLogService: auditLogService,
	}
}

// ============================================================================
// Admin: Hide Comment — POST /admin/comments/{comment_id}/hide
// ============================================================================

// AdminHideCommentRequest represents the request for hiding a comment.
type AdminHideCommentRequest struct {
	CommentID string `path:"comment_id" doc:"Comment ID" example:"1"`
	Body      struct {
		Reason string `json:"reason" doc:"Reason for hiding the comment" example:"Violates community guidelines"`
	}
}

// AdminHideCommentHandler handles POST /admin/comments/{comment_id}/hide
func (h *CommentAdminHandler) AdminHideCommentHandler(ctx context.Context, req *AdminHideCommentRequest) (*struct{}, error) {
	user, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	commentID, err := strconv.ParseUint(req.CommentID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid comment ID")
	}

	reason := strings.TrimSpace(req.Body.Reason)
	if reason == "" {
		return nil, huma.Error400BadRequest("Reason is required when hiding a comment")
	}

	if err := h.admin.HideComment(user.ID, uint(commentID), reason); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, huma.Error404NotFound("Comment not found")
		}
		requestID := logger.GetRequestID(ctx)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to hide comment (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		go func() {
			h.auditLogService.LogAction(user.ID, "hide_comment", "comment", uint(commentID), map[string]interface{}{
				"reason": reason,
			})
		}()
	}

	return nil, nil
}

// ============================================================================
// Admin: Restore Comment — POST /admin/comments/{comment_id}/restore
// ============================================================================

// AdminRestoreCommentRequest represents the request for restoring a comment.
type AdminRestoreCommentRequest struct {
	CommentID string `path:"comment_id" doc:"Comment ID" example:"1"`
}

// AdminRestoreCommentHandler handles POST /admin/comments/{comment_id}/restore
func (h *CommentAdminHandler) AdminRestoreCommentHandler(ctx context.Context, req *AdminRestoreCommentRequest) (*struct{}, error) {
	user, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	commentID, err := strconv.ParseUint(req.CommentID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid comment ID")
	}

	if err := h.admin.RestoreComment(user.ID, uint(commentID)); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, huma.Error404NotFound("Comment not found")
		}
		if strings.Contains(err.Error(), "already visible") {
			return nil, huma.Error409Conflict("Comment is already visible")
		}
		requestID := logger.GetRequestID(ctx)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to restore comment (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		go func() {
			h.auditLogService.LogAction(user.ID, "restore_comment", "comment", uint(commentID), nil)
		}()
	}

	return nil, nil
}

// ============================================================================
// Admin: List Pending Comments — GET /admin/comments/pending
// ============================================================================

// AdminListPendingCommentsRequest represents the request for listing pending comments.
type AdminListPendingCommentsRequest struct {
	Limit  int `query:"limit" required:"false" doc:"Page size (default 20, max 100)" example:"20"`
	Offset int `query:"offset" required:"false" doc:"Pagination offset" example:"0"`
}

// AdminListPendingCommentsResponse represents the response for listing pending comments.
type AdminListPendingCommentsResponse struct {
	Body struct {
		Comments []*contracts.CommentResponse `json:"comments" doc:"Pending comments awaiting review"`
		Total    int64                        `json:"total" doc:"Total number of pending comments"`
	}
}

// AdminListPendingCommentsHandler handles GET /admin/comments/pending
func (h *CommentAdminHandler) AdminListPendingCommentsHandler(ctx context.Context, req *AdminListPendingCommentsRequest) (*AdminListPendingCommentsResponse, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	comments, total, err := h.admin.ListPendingComments(limit, offset)
	if err != nil {
		requestID := logger.GetRequestID(ctx)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to list pending comments (request_id: %s)", requestID),
		)
	}

	resp := &AdminListPendingCommentsResponse{}
	resp.Body.Comments = comments
	resp.Body.Total = total
	return resp, nil
}

// ============================================================================
// Admin: Approve Comment — POST /admin/comments/{comment_id}/approve
// ============================================================================

// AdminApproveCommentRequest represents the request for approving a pending comment.
type AdminApproveCommentRequest struct {
	CommentID string `path:"comment_id" doc:"Comment ID" example:"1"`
}

// AdminApproveCommentHandler handles POST /admin/comments/{comment_id}/approve
func (h *CommentAdminHandler) AdminApproveCommentHandler(ctx context.Context, req *AdminApproveCommentRequest) (*struct{}, error) {
	user, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	commentID, err := strconv.ParseUint(req.CommentID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid comment ID")
	}

	if err := h.admin.ApproveComment(user.ID, uint(commentID)); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, huma.Error404NotFound("Comment not found")
		}
		if strings.Contains(err.Error(), "not pending review") {
			return nil, huma.Error409Conflict("Comment is not pending review")
		}
		requestID := logger.GetRequestID(ctx)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to approve comment (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		go func() {
			h.auditLogService.LogAction(user.ID, "approve_comment", "comment", uint(commentID), nil)
		}()
	}

	return nil, nil
}

// ============================================================================
// Admin: Reject Comment — POST /admin/comments/{comment_id}/reject
// ============================================================================

// AdminRejectCommentRequest represents the request for rejecting a pending comment.
type AdminRejectCommentRequest struct {
	CommentID string `path:"comment_id" doc:"Comment ID" example:"1"`
	Body      struct {
		Reason string `json:"reason" doc:"Reason for rejecting the comment" example:"Spam content"`
	}
}

// AdminRejectCommentHandler handles POST /admin/comments/{comment_id}/reject
func (h *CommentAdminHandler) AdminRejectCommentHandler(ctx context.Context, req *AdminRejectCommentRequest) (*struct{}, error) {
	user, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	commentID, err := strconv.ParseUint(req.CommentID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid comment ID")
	}

	reason := strings.TrimSpace(req.Body.Reason)
	if reason == "" {
		return nil, huma.Error400BadRequest("Reason is required when rejecting a comment")
	}

	if err := h.admin.RejectComment(user.ID, uint(commentID), reason); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, huma.Error404NotFound("Comment not found")
		}
		if strings.Contains(err.Error(), "not pending review") {
			return nil, huma.Error409Conflict("Comment is not pending review")
		}
		requestID := logger.GetRequestID(ctx)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to reject comment (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		go func() {
			h.auditLogService.LogAction(user.ID, "reject_comment", "comment", uint(commentID), map[string]interface{}{
				"reason": reason,
			})
		}()
	}

	return nil, nil
}

// ============================================================================
// Admin: Get Comment Edit History — GET /admin/comments/{comment_id}/edits
// ============================================================================

// AdminGetCommentEditHistoryRequest represents the request for fetching a comment's edit history.
type AdminGetCommentEditHistoryRequest struct {
	CommentID string `path:"comment_id" doc:"Comment ID" example:"1"`
}

// AdminGetCommentEditHistoryResponse represents the response for fetching a comment's edit history.
type AdminGetCommentEditHistoryResponse struct {
	Body contracts.CommentEditHistoryResponse
}

// AdminGetCommentEditHistoryHandler handles GET /admin/comments/{comment_id}/edits.
// Returns the chronological edit history (oldest first) plus the current body.
func (h *CommentAdminHandler) AdminGetCommentEditHistoryHandler(ctx context.Context, req *AdminGetCommentEditHistoryRequest) (*AdminGetCommentEditHistoryResponse, error) {
	user, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	commentID, err := strconv.ParseUint(req.CommentID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid comment ID")
	}

	history, err := h.admin.GetCommentEditHistory(user.ID, uint(commentID))
	if err != nil {
		if strings.Contains(err.Error(), "admin access required") {
			return nil, huma.Error403Forbidden("Admin access required")
		}
		if strings.Contains(err.Error(), "not found") {
			return nil, huma.Error404NotFound("Comment not found")
		}
		requestID := logger.GetRequestID(ctx)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to load comment edit history (request_id: %s)", requestID),
		)
	}

	return &AdminGetCommentEditHistoryResponse{Body: *history}, nil
}
