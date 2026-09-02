package engagement

import (
	"context"
	"fmt"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/api/middleware"
	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/services/contracts"
)

// CommentVoteHandler handles comment vote API requests.
type CommentVoteHandler struct {
	voteService contracts.CommentVoteServiceInterface
	// reader resolves the comment's parent entity, because a vote route is
	// addressed by a COMMENT id and the visibility rule belongs to the entity
	// the comment hangs off.
	reader CommentReader
	// showVisibility gates both vote routes on the parent entity's rule.
	// Required; a nil checker refuses every gated parent.
	showVisibility contracts.ShowVisibilityInterface
}

// NewCommentVoteHandler creates a new handler.
func NewCommentVoteHandler(
	voteService contracts.CommentVoteServiceInterface,
	reader CommentReader,
	showVisibility contracts.ShowVisibilityInterface,
) *CommentVoteHandler {
	return &CommentVoteHandler{
		voteService:    voteService,
		reader:         reader,
		showVisibility: showVisibility,
	}
}

// refuseVoteAsMissingComment is the answer both vote routes give for a comment
// whose parent the caller may not see: the vote service's own
// comment-not-found error, so a gated parent and a comment id nobody has used
// are one response.
func refuseVoteAsMissingComment() error {
	if mapped := shared.MapCommentVoteError(apperrors.ErrCommentVoteCommentNotFound()); mapped != nil {
		return mapped
	}
	return huma.Error404NotFound("Comment not found")
}

// voteParentVisible reports whether the caller may see the entity comment
// commentID hangs off.
//
// A vote is a WRITE on a comment inside that entity's discussion and the
// response carries the comment's live score, so it takes the same viewer the
// thread does. Gated after the load, because the comment is what names its
// entity; comment ids are dense, so a caller refused the listing would
// otherwise walk them.
func (h *CommentVoteHandler) voteParentVisible(ctx context.Context, commentID uint) bool {
	if h.reader == nil {
		return false
	}
	comment, err := h.reader.GetComment(commentID)
	if err != nil || comment == nil {
		return false
	}
	return shared.EntitySubResourceVisible(
		h.showVisibility, comment.EntityType, comment.EntityID, middleware.GetShowViewerFromContext(ctx))
}

// ============================================================================
// Vote on Comment (protected)
// ============================================================================

// VoteCommentRequest is the request for casting a vote on a comment.
type VoteCommentRequest struct {
	CommentID string `path:"comment_id" doc:"Comment ID" example:"1"`
	Body      struct {
		Direction int `json:"direction" doc:"Vote direction: 1 for upvote, -1 for downvote" example:"1"`
	}
}

// VoteCommentResponse contains the updated vote counts after a vote.
type VoteCommentResponse struct {
	Body contracts.CommentVoteResponse
}

// VoteCommentHandler handles upvoting or downvoting a comment.
func (h *CommentVoteHandler) VoteCommentHandler(ctx context.Context, req *VoteCommentRequest) (*VoteCommentResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	commentID, err := strconv.ParseUint(req.CommentID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid comment ID")
	}

	if req.Body.Direction != 1 && req.Body.Direction != -1 {
		return nil, huma.Error400BadRequest("Direction must be 1 (upvote) or -1 (downvote)")
	}

	if !h.voteParentVisible(ctx, uint(commentID)) {
		return nil, refuseVoteAsMissingComment()
	}

	err = h.voteService.Vote(user.ID, uint(commentID), req.Body.Direction)
	if err != nil {
		if mapped := shared.MapCommentVoteError(err); mapped != nil {
			return nil, mapped
		}
		return nil, huma.Error500InternalServerError(fmt.Sprintf("Failed to vote: %v", err))
	}

	return h.buildVoteResponse(user.ID, uint(commentID))
}

// ============================================================================
// Remove Vote (protected)
// ============================================================================

// UnvoteCommentRequest is the request for removing a vote on a comment.
type UnvoteCommentRequest struct {
	CommentID string `path:"comment_id" doc:"Comment ID" example:"1"`
}

// UnvoteCommentResponse contains the updated vote counts after removing a vote.
type UnvoteCommentResponse struct {
	Body contracts.CommentVoteResponse
}

// UnvoteCommentHandler removes a user's vote on a comment.
func (h *CommentVoteHandler) UnvoteCommentHandler(ctx context.Context, req *UnvoteCommentRequest) (*UnvoteCommentResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	commentID, err := strconv.ParseUint(req.CommentID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid comment ID")
	}

	if !h.voteParentVisible(ctx, uint(commentID)) {
		return nil, refuseVoteAsMissingComment()
	}

	err = h.voteService.Unvote(user.ID, uint(commentID))
	if err != nil {
		if mapped := shared.MapCommentVoteError(err); mapped != nil {
			return nil, mapped
		}
		return nil, huma.Error500InternalServerError(fmt.Sprintf("Failed to remove vote: %v", err))
	}

	return h.buildUnvoteResponse(uint(commentID))
}

// buildVoteResponse fetches current vote counts and user vote for the response.
func (h *CommentVoteHandler) buildVoteResponse(userID uint, commentID uint) (*VoteCommentResponse, error) {
	ups, downs, score, err := h.voteService.GetCommentVoteCounts(commentID)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to get vote counts")
	}

	userVote, _ := h.voteService.GetUserVote(userID, commentID)

	resp := &VoteCommentResponse{}
	resp.Body.Ups = ups
	resp.Body.Downs = downs
	resp.Body.Score = score
	resp.Body.UserVote = userVote
	return resp, nil
}

// buildUnvoteResponse fetches current vote counts for the unvote response.
func (h *CommentVoteHandler) buildUnvoteResponse(commentID uint) (*UnvoteCommentResponse, error) {
	ups, downs, score, err := h.voteService.GetCommentVoteCounts(commentID)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to get vote counts")
	}

	resp := &UnvoteCommentResponse{}
	resp.Body.Ups = ups
	resp.Body.Downs = downs
	resp.Body.Score = score
	resp.Body.UserVote = nil
	return resp, nil
}
