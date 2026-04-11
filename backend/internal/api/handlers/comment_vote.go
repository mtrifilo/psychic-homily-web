package handlers

import (
	"context"
	"fmt"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/services/contracts"
)

// CommentVoteHandler handles comment vote API requests.
type CommentVoteHandler struct {
	voteService contracts.CommentVoteServiceInterface
}

// NewCommentVoteHandler creates a new handler.
func NewCommentVoteHandler(
	voteService contracts.CommentVoteServiceInterface,
) *CommentVoteHandler {
	return &CommentVoteHandler{
		voteService: voteService,
	}
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

	err = h.voteService.Vote(user.ID, uint(commentID), req.Body.Direction)
	if err != nil {
		if err.Error() == "comment not found" {
			return nil, huma.Error404NotFound("Comment not found")
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

	err = h.voteService.Unvote(user.ID, uint(commentID))
	if err != nil {
		if err.Error() == "comment not found" {
			return nil, huma.Error404NotFound("Comment not found")
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
