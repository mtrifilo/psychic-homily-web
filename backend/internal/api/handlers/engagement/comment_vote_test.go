package engagement

import (
	"context"
	"fmt"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
)

// Uses auto-generated testhelpers.MockCommentVoteService from handler_unit_mock_helpers_test.go

func testCommentVoteHandler() *CommentVoteHandler {
	return NewCommentVoteHandler(nil)
}

// ============================================================================
// VoteCommentHandler Tests
// ============================================================================

func TestVoteComment_NoAuth(t *testing.T) {
	h := testCommentVoteHandler()
	req := &VoteCommentRequest{CommentID: "1"}
	req.Body.Direction = 1

	_, err := h.VoteCommentHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 401)
}

func TestVoteComment_InvalidCommentID(t *testing.T) {
	h := testCommentVoteHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &VoteCommentRequest{CommentID: "abc"}
	req.Body.Direction = 1

	_, err := h.VoteCommentHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestVoteComment_InvalidDirectionZero(t *testing.T) {
	h := testCommentVoteHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &VoteCommentRequest{CommentID: "1"}
	req.Body.Direction = 0

	_, err := h.VoteCommentHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestVoteComment_InvalidDirectionTwo(t *testing.T) {
	h := testCommentVoteHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &VoteCommentRequest{CommentID: "1"}
	req.Body.Direction = 2

	_, err := h.VoteCommentHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestVoteComment_Success(t *testing.T) {
	upvote := 1
	h := NewCommentVoteHandler(&testhelpers.MockCommentVoteService{
		VoteFn: func(userID uint, commentID uint, direction int) error {
			if userID != 1 || commentID != 42 || direction != 1 {
				return fmt.Errorf("unexpected args: %d, %d, %d", userID, commentID, direction)
			}
			return nil
		},
		GetCommentVoteCountsFn: func(commentID uint) (int, int, float64, error) {
			return 5, 2, 0.55, nil
		},
		GetUserVoteFn: func(userID uint, commentID uint) (*int, error) {
			return &upvote, nil
		},
	})

	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &VoteCommentRequest{CommentID: "42"}
	req.Body.Direction = 1

	resp, err := h.VoteCommentHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Body.Ups != 5 {
		t.Errorf("expected ups=5, got %d", resp.Body.Ups)
	}
	if resp.Body.Downs != 2 {
		t.Errorf("expected downs=2, got %d", resp.Body.Downs)
	}
	if resp.Body.UserVote == nil || *resp.Body.UserVote != 1 {
		t.Errorf("expected user_vote=1, got %v", resp.Body.UserVote)
	}
}

func TestVoteComment_CommentNotFound(t *testing.T) {
	h := NewCommentVoteHandler(&testhelpers.MockCommentVoteService{
		VoteFn: func(userID uint, commentID uint, direction int) error {
			return fmt.Errorf("comment not found")
		},
	})

	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &VoteCommentRequest{CommentID: "99"}
	req.Body.Direction = 1

	_, err := h.VoteCommentHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 404)
}

func TestVoteComment_ServiceError(t *testing.T) {
	h := NewCommentVoteHandler(&testhelpers.MockCommentVoteService{
		VoteFn: func(userID uint, commentID uint, direction int) error {
			return fmt.Errorf("database error")
		},
	})

	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &VoteCommentRequest{CommentID: "1"}
	req.Body.Direction = 1

	_, err := h.VoteCommentHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// UnvoteCommentHandler Tests
// ============================================================================

func TestUnvoteComment_NoAuth(t *testing.T) {
	h := testCommentVoteHandler()
	req := &UnvoteCommentRequest{CommentID: "1"}

	_, err := h.UnvoteCommentHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 401)
}

func TestUnvoteComment_InvalidCommentID(t *testing.T) {
	h := testCommentVoteHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &UnvoteCommentRequest{CommentID: "abc"}

	_, err := h.UnvoteCommentHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestUnvoteComment_Success(t *testing.T) {
	h := NewCommentVoteHandler(&testhelpers.MockCommentVoteService{
		UnvoteFn: func(userID uint, commentID uint) error {
			return nil
		},
		GetCommentVoteCountsFn: func(commentID uint) (int, int, float64, error) {
			return 3, 1, 0.45, nil
		},
	})

	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &UnvoteCommentRequest{CommentID: "42"}

	resp, err := h.UnvoteCommentHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Body.Ups != 3 {
		t.Errorf("expected ups=3, got %d", resp.Body.Ups)
	}
	if resp.Body.UserVote != nil {
		t.Errorf("expected user_vote=nil, got %v", resp.Body.UserVote)
	}
}

func TestUnvoteComment_CommentNotFound(t *testing.T) {
	h := NewCommentVoteHandler(&testhelpers.MockCommentVoteService{
		UnvoteFn: func(userID uint, commentID uint) error {
			return fmt.Errorf("comment not found")
		},
	})

	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &UnvoteCommentRequest{CommentID: "99"}

	_, err := h.UnvoteCommentHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 404)
}

func TestUnvoteComment_ServiceError(t *testing.T) {
	h := NewCommentVoteHandler(&testhelpers.MockCommentVoteService{
		UnvoteFn: func(userID uint, commentID uint) error {
			return fmt.Errorf("database error")
		},
	})

	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &UnvoteCommentRequest{CommentID: "1"}

	_, err := h.UnvoteCommentHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 500)
}
