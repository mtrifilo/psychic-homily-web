package errors

import (
	"fmt"
)

// Comment-vote error codes.
//
// Produced by CommentVoteService (Vote / Unvote) and consumed by
// `engagement/comment_vote.go`. The self-vote rejection is its own
// forbidden code so it can be distinguished from a generic 403.
//
// Status mapping (see shared.MapCommentVoteError):
//   - CommentNotFound  → 404
//   - SelfVote         → 403
//   - InvalidDirection → 400 (the handler short-circuits before the service
//     in the production path; included for service-layer defensive use)
//   - Internal         → 500
const (
	// CodeCommentVoteCommentNotFound indicates the target comment does
	// not exist.
	CodeCommentVoteCommentNotFound = "COMMENT_VOTE_COMMENT_NOT_FOUND"
	// CodeCommentVoteSelfVote indicates the caller attempted to vote on
	// their own comment (HN/Lobsters convention).
	CodeCommentVoteSelfVote = "COMMENT_VOTE_SELF_VOTE"
	// CodeCommentVoteInvalidDirection indicates the direction is not 1 or -1.
	CodeCommentVoteInvalidDirection = "COMMENT_VOTE_INVALID_DIRECTION"
	// CodeCommentVoteInternal indicates a database or infrastructure failure.
	CodeCommentVoteInternal = "COMMENT_VOTE_INTERNAL"
)

// CommentVoteError represents a comment-vote-related error.
type CommentVoteError struct {
	Code     string
	Message  string
	Internal error
}

// Error implements the error interface.
func (e *CommentVoteError) Error() string {
	if e.Internal != nil {
		return fmt.Sprintf("%s: %s (internal: %v)", e.Code, e.Message, e.Internal)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the internal error for errors.Is/As compatibility.
func (e *CommentVoteError) Unwrap() error {
	return e.Internal
}

// ErrCommentVoteCommentNotFound creates a comment-not-found error.
func ErrCommentVoteCommentNotFound() *CommentVoteError {
	return &CommentVoteError{
		Code:    CodeCommentVoteCommentNotFound,
		Message: "comment not found",
	}
}

// ErrCommentVoteSelfVote creates a self-vote-forbidden error.
func ErrCommentVoteSelfVote() *CommentVoteError {
	return &CommentVoteError{
		Code:    CodeCommentVoteSelfVote,
		Message: "cannot vote on your own comment",
	}
}

// ErrCommentVoteInvalidDirection creates an invalid-direction error.
func ErrCommentVoteInvalidDirection() *CommentVoteError {
	return &CommentVoteError{
		Code:    CodeCommentVoteInvalidDirection,
		Message: "invalid vote direction: must be 1 or -1",
	}
}

// ErrCommentVoteInternal wraps a database or infrastructure failure.
func ErrCommentVoteInternal(message string, internal error) *CommentVoteError {
	return &CommentVoteError{
		Code:     CodeCommentVoteInternal,
		Message:  message,
		Internal: internal,
	}
}
