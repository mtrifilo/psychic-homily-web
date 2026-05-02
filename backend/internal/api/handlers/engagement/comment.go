package handlers

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
)

// ============================================================================
// Focused interfaces for dependency injection
// ============================================================================

// CommentReader defines the read operations for comments.
type CommentReader interface {
	GetComment(commentID uint) (*contracts.CommentResponse, error)
	ListCommentsForEntity(entityType string, entityID uint, filters contracts.CommentListFilters) (*contracts.CommentListResponse, error)
	GetThread(rootID uint) ([]*contracts.CommentResponse, error)
}

// CommentWriter defines the write operations for comments.
type CommentWriter interface {
	CreateComment(userID uint, req *contracts.CreateCommentRequest) (*contracts.CommentResponse, error)
	UpdateComment(userID uint, commentID uint, req *contracts.UpdateCommentRequest) (*contracts.CommentResponse, error)
	UpdateReplyPermission(userID uint, commentID uint, permission string) (*contracts.CommentResponse, error)
	DeleteComment(userID uint, commentID uint, isAdmin bool) error
}

// CommentVoteReader supplies per-user vote lookups for populating
// `user_vote` on responses. Nil is acceptable — handlers treat a nil
// reader or nil user as "anonymous; don't populate".
type CommentVoteReader interface {
	GetUserVotesForComments(userID uint, commentIDs []uint) (map[uint]int, error)
}

// CommentHandler handles comment-related API requests.
type CommentHandler struct {
	reader          CommentReader
	writer          CommentWriter
	voteReader      CommentVoteReader
	auditLogService contracts.AuditLogServiceInterface
}

// NewCommentHandler creates a new CommentHandler. voteReader may be nil
// in tests that don't exercise the authenticated-read path.
func NewCommentHandler(reader CommentReader, writer CommentWriter, voteReader CommentVoteReader, auditLogService contracts.AuditLogServiceInterface) *CommentHandler {
	return &CommentHandler{
		reader:          reader,
		writer:          writer,
		voteReader:      voteReader,
		auditLogService: auditLogService,
	}
}

// populateUserVotes mutates the provided responses to set user_vote based
// on the authenticated user's existing votes. No-op if the user is
// anonymous, the voteReader is nil, or the response set is empty. Errors
// from the vote lookup are logged and swallowed — vote state is a
// decoration, not a critical path.
func (h *CommentHandler) populateUserVotes(ctx context.Context, user *models.User, responses []*contracts.CommentResponse) {
	if user == nil || h.voteReader == nil || len(responses) == 0 {
		return
	}
	ids := make([]uint, 0, len(responses))
	for _, r := range responses {
		if r != nil {
			ids = append(ids, r.ID)
		}
	}
	votes, err := h.voteReader.GetUserVotesForComments(user.ID, ids)
	if err != nil {
		logger.FromContext(ctx).Warn("failed_to_populate_user_votes",
			"user_id", user.ID,
			"error", err.Error(),
		)
		return
	}
	for _, r := range responses {
		if r == nil {
			continue
		}
		if dir, ok := votes[r.ID]; ok {
			d := dir
			r.UserVote = &d
		}
	}
}

// ============================================================================
// List Comments for Entity (public, optional auth)
// ============================================================================

// ListCommentsRequest represents the request for listing comments on an entity.
type ListCommentsRequest struct {
	EntityType string `path:"entity_type" doc:"Entity type (artist, venue, show, release, label, festival, collection)" example:"show"`
	EntityID   string `path:"entity_id" doc:"Entity ID" example:"1"`
	Sort       string `query:"sort" required:"false" doc:"Sort order: best, new, top, controversial (default: best)" example:"best"`
	Limit      int    `query:"limit" required:"false" doc:"Page size (default 25, max 100)" example:"25"`
	Offset     int    `query:"offset" required:"false" doc:"Pagination offset" example:"0"`
}

// ListCommentsResponse represents the response for listing comments.
type ListCommentsResponse struct {
	Body *contracts.CommentListResponse
}

// ListCommentsHandler handles GET /entities/{entity_type}/{entity_id}/comments
func (h *CommentHandler) ListCommentsHandler(ctx context.Context, req *ListCommentsRequest) (*ListCommentsResponse, error) {
	entityID, err := strconv.ParseUint(req.EntityID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid entity ID")
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 25
	}
	if limit > 100 {
		limit = 100
	}

	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	filters := contracts.CommentListFilters{
		Sort:   req.Sort,
		Limit:  limit,
		Offset: offset,
	}

	result, err := h.reader.ListCommentsForEntity(req.EntityType, uint(entityID), filters)
	if err != nil {
		if strings.Contains(err.Error(), "unsupported entity type") {
			return nil, huma.Error400BadRequest(err.Error())
		}
		return nil, huma.Error500InternalServerError("Failed to fetch comments")
	}

	// Populate user_vote for the authenticated viewer. Route is on
	// optionalAuthGroup, so user may be nil for anonymous requests.
	h.populateUserVotes(ctx, middleware.GetUserFromContext(ctx), result.Comments)

	return &ListCommentsResponse{Body: result}, nil
}

// ============================================================================
// Get Comment (public)
// ============================================================================

// GetCommentRequest represents the request for getting a single comment.
type GetCommentRequest struct {
	CommentID string `path:"comment_id" doc:"Comment ID" example:"1"`
}

// GetCommentResponse represents the response for a single comment.
type GetCommentResponse struct {
	Body *contracts.CommentResponse
}

// GetCommentHandler handles GET /comments/{comment_id}
func (h *CommentHandler) GetCommentHandler(ctx context.Context, req *GetCommentRequest) (*GetCommentResponse, error) {
	commentID, err := strconv.ParseUint(req.CommentID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid comment ID")
	}

	comment, err := h.reader.GetComment(uint(commentID))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, huma.Error404NotFound("Comment not found")
		}
		return nil, huma.Error500InternalServerError("Failed to fetch comment")
	}

	h.populateUserVotes(ctx, middleware.GetUserFromContext(ctx), []*contracts.CommentResponse{comment})

	return &GetCommentResponse{Body: comment}, nil
}

// ============================================================================
// Get Thread (public)
// ============================================================================

// GetThreadRequest represents the request for getting a full comment thread.
type GetThreadRequest struct {
	CommentID string `path:"comment_id" doc:"Root comment ID" example:"1"`
}

// GetThreadResponse represents the response for a comment thread.
type GetThreadResponse struct {
	Body struct {
		Comments []*contracts.CommentResponse `json:"comments" doc:"All comments in the thread, ordered by created_at"`
	}
}

// GetThreadHandler handles GET /comments/{comment_id}/thread
func (h *CommentHandler) GetThreadHandler(ctx context.Context, req *GetThreadRequest) (*GetThreadResponse, error) {
	commentID, err := strconv.ParseUint(req.CommentID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid comment ID")
	}

	comments, err := h.reader.GetThread(uint(commentID))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, huma.Error404NotFound("Thread not found")
		}
		if strings.Contains(err.Error(), "not a thread root") {
			return nil, huma.Error400BadRequest("Comment is not a thread root")
		}
		return nil, huma.Error500InternalServerError("Failed to fetch thread")
	}

	h.populateUserVotes(ctx, middleware.GetUserFromContext(ctx), comments)

	resp := &GetThreadResponse{}
	resp.Body.Comments = comments
	return resp, nil
}

// ============================================================================
// Create Comment (protected)
// ============================================================================

// CreateCommentRequest represents the request for creating a top-level comment.
type CreateCommentRequest struct {
	EntityType string `path:"entity_type" doc:"Entity type (artist, venue, show, release, label, festival, collection)" example:"show"`
	EntityID   string `path:"entity_id" doc:"Entity ID" example:"1"`
	Body       struct {
		Body            string `json:"body" doc:"Comment body (Markdown)" example:"Great show last night!"`
		ReplyPermission string `json:"reply_permission,omitempty" required:"false" doc:"Who can reply: anyone, author_only (default: anyone)" example:"anyone"`
	}
}

// CreateCommentResponse represents the response for creating a comment.
type CreateCommentResponse struct {
	Body *contracts.CommentResponse
}

// CreateCommentHandler handles POST /entities/{entity_type}/{entity_id}/comments
func (h *CommentHandler) CreateCommentHandler(ctx context.Context, req *CreateCommentRequest) (*CreateCommentResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	entityID, err := strconv.ParseUint(req.EntityID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid entity ID")
	}

	if strings.TrimSpace(req.Body.Body) == "" {
		return nil, huma.Error400BadRequest("Comment body is required")
	}

	serviceReq := &contracts.CreateCommentRequest{
		EntityType:      req.EntityType,
		EntityID:        uint(entityID),
		Body:            req.Body.Body,
		ReplyPermission: req.Body.ReplyPermission,
	}

	comment, err := h.writer.CreateComment(user.ID, serviceReq)
	if err != nil {
		if strings.Contains(err.Error(), "unsupported entity type") {
			return nil, huma.Error400BadRequest(err.Error())
		}
		if strings.Contains(err.Error(), "invalid reply_permission") {
			return nil, huma.Error400BadRequest(err.Error())
		}
		if strings.Contains(err.Error(), "not found") {
			return nil, huma.Error404NotFound(err.Error())
		}
		if strings.Contains(err.Error(), "body is required") || strings.Contains(err.Error(), "exceeds maximum length") {
			return nil, huma.Error400BadRequest(err.Error())
		}
		if strings.Contains(err.Error(), "please wait") || strings.Contains(err.Error(), "hourly comment limit") {
			return nil, huma.Error429TooManyRequests(err.Error())
		}
		requestID := logger.GetRequestID(ctx)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to create comment (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		go func() {
			h.auditLogService.LogAction(user.ID, "create_comment", req.EntityType, comment.ID, map[string]interface{}{
				"entity_id": uint(entityID),
			})
		}()
	}

	return &CreateCommentResponse{Body: comment}, nil
}

// ============================================================================
// Create Reply (protected)
// ============================================================================

// CreateReplyRequest represents the request for replying to a comment.
type CreateReplyRequest struct {
	CommentID string `path:"comment_id" doc:"Parent comment ID" example:"1"`
	Body      struct {
		Body            string `json:"body" doc:"Reply body (Markdown)" example:"I agree, the opener was amazing!"`
		ReplyPermission string `json:"reply_permission,omitempty" required:"false" doc:"Who can reply: anyone, author_only (default: anyone)" example:"anyone"`
	}
}

// CreateReplyResponse represents the response for creating a reply.
type CreateReplyResponse struct {
	Body *contracts.CommentResponse
}

// CreateReplyHandler handles POST /comments/{comment_id}/replies
func (h *CommentHandler) CreateReplyHandler(ctx context.Context, req *CreateReplyRequest) (*CreateReplyResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	parentID, err := strconv.ParseUint(req.CommentID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid comment ID")
	}

	if strings.TrimSpace(req.Body.Body) == "" {
		return nil, huma.Error400BadRequest("Reply body is required")
	}

	// Look up parent comment to get entity_type and entity_id
	parent, err := h.reader.GetComment(uint(parentID))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, huma.Error404NotFound("Parent comment not found")
		}
		return nil, huma.Error500InternalServerError("Failed to fetch parent comment")
	}

	parentIDUint := uint(parentID)
	serviceReq := &contracts.CreateCommentRequest{
		EntityType:      parent.EntityType,
		EntityID:        parent.EntityID,
		Body:            req.Body.Body,
		ParentID:        &parentIDUint,
		ReplyPermission: req.Body.ReplyPermission,
	}

	comment, err := h.writer.CreateComment(user.ID, serviceReq)
	if err != nil {
		if strings.Contains(err.Error(), "maximum reply depth") {
			return nil, huma.Error400BadRequest(err.Error())
		}
		if strings.Contains(err.Error(), "invalid reply_permission") ||
			strings.Contains(err.Error(), "unsupported reply_permission") {
			return nil, huma.Error400BadRequest(err.Error())
		}
		// PSY-296: reply-permission gate rejections.
		if strings.Contains(err.Error(), "replies to this comment are disabled") ||
			strings.Contains(err.Error(), "only followers of the author can reply") {
			return nil, huma.Error403Forbidden(err.Error())
		}
		if strings.Contains(err.Error(), "body is required") || strings.Contains(err.Error(), "exceeds maximum length") {
			return nil, huma.Error400BadRequest(err.Error())
		}
		if strings.Contains(err.Error(), "parent comment belongs to a different entity") {
			return nil, huma.Error400BadRequest(err.Error())
		}
		if strings.Contains(err.Error(), "parent comment not found") {
			return nil, huma.Error404NotFound(err.Error())
		}
		if strings.Contains(err.Error(), "please wait") || strings.Contains(err.Error(), "hourly comment limit") {
			return nil, huma.Error429TooManyRequests(err.Error())
		}
		requestID := logger.GetRequestID(ctx)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to create reply (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		go func() {
			h.auditLogService.LogAction(user.ID, "create_comment", parent.EntityType, comment.ID, map[string]interface{}{
				"entity_id": parent.EntityID,
				"parent_id": parentIDUint,
			})
		}()
	}

	return &CreateReplyResponse{Body: comment}, nil
}

// ============================================================================
// Update Comment (protected)
// ============================================================================

// UpdateCommentRequest represents the request for updating a comment.
type UpdateCommentRequest struct {
	CommentID string `path:"comment_id" doc:"Comment ID" example:"1"`
	Body      struct {
		Body string `json:"body" doc:"Updated comment body (Markdown)" example:"Updated: Great show last night!"`
	}
}

// UpdateCommentResponse represents the response for updating a comment.
type UpdateCommentResponse struct {
	Body *contracts.CommentResponse
}

// UpdateCommentHandler handles PUT /comments/{comment_id}
func (h *CommentHandler) UpdateCommentHandler(ctx context.Context, req *UpdateCommentRequest) (*UpdateCommentResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	commentID, err := strconv.ParseUint(req.CommentID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid comment ID")
	}

	if strings.TrimSpace(req.Body.Body) == "" {
		return nil, huma.Error400BadRequest("Comment body is required")
	}

	serviceReq := &contracts.UpdateCommentRequest{
		Body: req.Body.Body,
	}

	comment, err := h.writer.UpdateComment(user.ID, uint(commentID), serviceReq)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, huma.Error404NotFound("Comment not found")
		}
		if strings.Contains(err.Error(), "only the comment author") {
			return nil, huma.Error403Forbidden("You can only edit your own comments")
		}
		if strings.Contains(err.Error(), "body is required") || strings.Contains(err.Error(), "exceeds maximum length") {
			return nil, huma.Error400BadRequest(err.Error())
		}
		requestID := logger.GetRequestID(ctx)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to update comment (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		go func() {
			h.auditLogService.LogAction(user.ID, "edit_comment", "comment", uint(commentID), nil)
		}()
	}

	return &UpdateCommentResponse{Body: comment}, nil
}

// ============================================================================
// Update Reply Permission (protected, owner-only) — PSY-296
// ============================================================================

// UpdateReplyPermissionRequest represents the request to change a comment's reply permission.
type UpdateReplyPermissionRequest struct {
	CommentID string `path:"comment_id" doc:"Comment ID" example:"1"`
	Body      struct {
		Permission string `json:"permission" doc:"Reply permission: anyone, followers, or author_only" example:"followers"`
	}
}

// UpdateReplyPermissionResponse wraps the updated comment.
type UpdateReplyPermissionResponse struct {
	Body *contracts.CommentResponse
}

// UpdateReplyPermissionHandler handles PUT /comments/{comment_id}/reply-permission
func (h *CommentHandler) UpdateReplyPermissionHandler(ctx context.Context, req *UpdateReplyPermissionRequest) (*UpdateReplyPermissionResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	commentID, err := strconv.ParseUint(req.CommentID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid comment ID")
	}

	perm := strings.TrimSpace(req.Body.Permission)
	if perm == "" {
		return nil, huma.Error400BadRequest("permission is required")
	}

	comment, err := h.writer.UpdateReplyPermission(user.ID, uint(commentID), perm)
	if err != nil {
		if strings.Contains(err.Error(), "invalid reply_permission") {
			return nil, huma.Error400BadRequest(err.Error())
		}
		if strings.Contains(err.Error(), "not found") {
			return nil, huma.Error404NotFound("Comment not found")
		}
		if strings.Contains(err.Error(), "only the comment author") {
			return nil, huma.Error403Forbidden("Only the comment author can change reply permission")
		}
		requestID := logger.GetRequestID(ctx)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to update reply permission (request_id: %s)", requestID),
		)
	}

	if h.auditLogService != nil {
		go func() {
			h.auditLogService.LogAction(user.ID, "update_reply_permission", "comment", uint(commentID), map[string]interface{}{
				"permission": perm,
			})
		}()
	}

	return &UpdateReplyPermissionResponse{Body: comment}, nil
}

// ============================================================================
// Delete Comment (protected)
// ============================================================================

// DeleteCommentRequest represents the request for deleting a comment.
type DeleteCommentRequest struct {
	CommentID string `path:"comment_id" doc:"Comment ID" example:"1"`
}

// DeleteCommentHandler handles DELETE /comments/{comment_id}
func (h *CommentHandler) DeleteCommentHandler(ctx context.Context, req *DeleteCommentRequest) (*struct{}, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	commentID, err := strconv.ParseUint(req.CommentID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid comment ID")
	}

	err = h.writer.DeleteComment(user.ID, uint(commentID), user.IsAdmin)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, huma.Error404NotFound("Comment not found")
		}
		if strings.Contains(err.Error(), "only the comment author or an admin") {
			return nil, huma.Error403Forbidden("You can only delete your own comments")
		}
		requestID := logger.GetRequestID(ctx)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to delete comment (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		go func() {
			h.auditLogService.LogAction(user.ID, "delete_comment", "comment", uint(commentID), nil)
		}()
	}

	return nil, nil
}
