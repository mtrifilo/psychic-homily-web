package engagement

import (
	"context"
	"fmt"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// Uses auto-generated testhelpers.MockCommentVoteService from handler_unit_mock_helpers_test.go

func testCommentVoteHandler() *CommentVoteHandler {
	return NewCommentVoteHandler(nil, visibleCommentParentReader(), testhelpers.AllShowsVisible())
}

// visibleCommentParentReader resolves every comment id to a parent of an
// always-visible entity type, so a test whose subject is NOT the vote gate is
// not deciding a visibility rule by accident.
func visibleCommentParentReader() *testhelpers.MockCommentService {
	return &testhelpers.MockCommentService{
		GetCommentFn: func(commentID uint) (*contracts.CommentResponse, error) {
			return &contracts.CommentResponse{ID: commentID, EntityType: "artist", EntityID: 1}, nil
		},
	}
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
	}, visibleCommentParentReader(), testhelpers.AllShowsVisible())

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
			return apperrors.ErrCommentVoteCommentNotFound()
		},
	}, visibleCommentParentReader(), testhelpers.AllShowsVisible())

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
	}, visibleCommentParentReader(), testhelpers.AllShowsVisible())

	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &VoteCommentRequest{CommentID: "1"}
	req.Body.Direction = 1

	_, err := h.VoteCommentHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 500)
}

// PSY-593: self-vote rejection surfaces as 403 from the handler. Covers both
// up and down so the guard cannot regress on one direction.
func TestVoteComment_SelfVoteUpForbidden(t *testing.T) {
	h := NewCommentVoteHandler(&testhelpers.MockCommentVoteService{
		VoteFn: func(userID uint, commentID uint, direction int) error {
			return apperrors.ErrCommentVoteSelfVote()
		},
	}, visibleCommentParentReader(), testhelpers.AllShowsVisible())

	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &VoteCommentRequest{CommentID: "42"}
	req.Body.Direction = 1

	_, err := h.VoteCommentHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 403)
}

func TestVoteComment_SelfVoteDownForbidden(t *testing.T) {
	h := NewCommentVoteHandler(&testhelpers.MockCommentVoteService{
		VoteFn: func(userID uint, commentID uint, direction int) error {
			return apperrors.ErrCommentVoteSelfVote()
		},
	}, visibleCommentParentReader(), testhelpers.AllShowsVisible())

	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &VoteCommentRequest{CommentID: "42"}
	req.Body.Direction = -1

	_, err := h.VoteCommentHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 403)
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
	}, visibleCommentParentReader(), testhelpers.AllShowsVisible())

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
			return apperrors.ErrCommentVoteCommentNotFound()
		},
	}, visibleCommentParentReader(), testhelpers.AllShowsVisible())

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
	}, visibleCommentParentReader(), testhelpers.AllShowsVisible())

	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &UnvoteCommentRequest{CommentID: "1"}

	_, err := h.UnvoteCommentHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// The vote gate
// ============================================================================

// A VOTE IS A WRITE INSIDE THE ENTITY'S DISCUSSION, and the response carries the
// comment's live score, so both routes take the same viewer the thread does.
//
// The refusal is the vote service's own comment-not-found error, so a comment on
// a gated parent and a comment id nobody has used are one response. The vote
// service is asserted UNREACHED, because a later edit could restore the
// default-open and a status-only assertion would still pass.
func TestCommentVote_RefusesAParentTheCallerCannotSee(t *testing.T) {
	deniesEverything := &testhelpers.MockShowVisibility{
		ShowVisibleToFn:       func(uint, contracts.ShowViewer) bool { return false },
		CollectionVisibleToFn: func(uint, contracts.ShowViewer) bool { return false },
	}
	privateCollectionParent := &testhelpers.MockCommentService{
		GetCommentFn: func(commentID uint) (*contracts.CommentResponse, error) {
			return &contracts.CommentResponse{ID: commentID, EntityType: "collection", EntityID: 7}, nil
		},
	}
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	t.Run("vote", func(t *testing.T) {
		h := NewCommentVoteHandler(&testhelpers.MockCommentVoteService{
			VoteFn: func(uint, uint, int) error {
				t.Error("the vote service was reached for a comment the caller cannot see")
				return nil
			},
		}, privateCollectionParent, deniesEverything)
		req := &VoteCommentRequest{CommentID: "42"}
		req.Body.Direction = 1
		_, err := h.VoteCommentHandler(ctx, req)
		testhelpers.AssertHumaError(t, err, 404)
	})

	t.Run("unvote", func(t *testing.T) {
		h := NewCommentVoteHandler(&testhelpers.MockCommentVoteService{
			UnvoteFn: func(uint, uint) error {
				t.Error("the vote service was reached for a comment the caller cannot see")
				return nil
			},
		}, privateCollectionParent, deniesEverything)
		_, err := h.UnvoteCommentHandler(ctx, &UnvoteCommentRequest{CommentID: "42"})
		testhelpers.AssertHumaError(t, err, 404)
	})

	// A comment id that resolves to nothing answers the same, which is what
	// collapses the pair instead of leaving it as an oracle over a dense space.
	t.Run("a comment that does not exist", func(t *testing.T) {
		h := NewCommentVoteHandler(&testhelpers.MockCommentVoteService{},
			&testhelpers.MockCommentService{
				GetCommentFn: func(uint) (*contracts.CommentResponse, error) { return nil, nil },
			}, testhelpers.AllShowsVisible())
		req := &VoteCommentRequest{CommentID: "42"}
		req.Body.Direction = 1
		_, err := h.VoteCommentHandler(ctx, req)
		testhelpers.AssertHumaError(t, err, 404)
	})
}
