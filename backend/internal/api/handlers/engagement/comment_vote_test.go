package engagement

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/danielgtaylor/huma/v2"

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

// ============================================================================
// The vote gate, tier by tier and entity type by entity type
// ============================================================================

// commentVoteTier is one caller in the matrix below.
//
// A gate proven only against a checker that refuses everything is proven only
// against itself: it would pass with the entity type ignored and with the viewer
// discarded. These cases carry the REAL rules instead, so the entity type and
// the viewer both have to reach the checker for the expected answer to come out.
type commentVoteTier struct {
	name string
	// user is nil for the anonymous caller, who never reaches the gate: the
	// route needs a session before it has anything to decide.
	user *authm.User
	want int
}

// commentVoteOwnerID is the user who submitted the show and created the
// collection both parents in this matrix hang off.
const commentVoteOwnerID = uint(2)

// showParentTiers is the show rule: an admin and the show's own submitter see a
// gated show, nobody else does.
var showParentTiers = []commentVoteTier{
	{"anonymous", nil, 401},
	{"an authenticated stranger", &authm.User{ID: 3}, 404},
	{"the show's submitter", &authm.User{ID: commentVoteOwnerID}, 200},
	{"an admin", &authm.User{ID: 6, IsAdmin: true}, 200},
}

// privateCollectionParentTiers is the collection rule, which has NO admin term:
// a private collection is its creator's alone. The two lists differing on that
// one row is the point of running both.
var privateCollectionParentTiers = []commentVoteTier{
	{"anonymous", nil, 401},
	{"an authenticated stranger", &authm.User{ID: 3}, 404},
	{"the collection's creator", &authm.User{ID: commentVoteOwnerID}, 200},
	{"an admin", &authm.User{ID: 6, IsAdmin: true}, 404},
}

// gatedParentVisibility answers the two real rules for a parent owned by
// commentVoteOwnerID.
func gatedParentVisibility() *testhelpers.MockShowVisibility {
	return &testhelpers.MockShowVisibility{
		ShowVisibleToFn: func(_ uint, viewer contracts.ShowViewer) bool {
			return viewer.IsAdmin || viewer.UserID == commentVoteOwnerID
		},
		CollectionVisibleToFn: func(_ uint, viewer contracts.ShowViewer) bool {
			return viewer.UserID == commentVoteOwnerID
		},
	}
}

// commentParentReader resolves every comment id to a parent of one entity type.
func commentParentReader(entityType string) *testhelpers.MockCommentService {
	return &testhelpers.MockCommentService{
		GetCommentFn: func(commentID uint) (*contracts.CommentResponse, error) {
			return &contracts.CommentResponse{ID: commentID, EntityType: entityType, EntityID: 7}, nil
		},
	}
}

// voteServiceRecordingReach answers successfully and records that it was
// reached, so a refusal can assert the write never ran and a grant can assert it
// did.
func voteServiceRecordingReach(reached *int) *testhelpers.MockCommentVoteService {
	return &testhelpers.MockCommentVoteService{
		VoteFn:   func(uint, uint, int) error { *reached++; return nil },
		UnvoteFn: func(uint, uint) error { *reached++; return nil },
		GetCommentVoteCountsFn: func(uint) (int, int, float64, error) {
			return 1, 0, 1, nil
		},
	}
}

// BOTH VOTE ROUTES ANSWER THE PARENT'S OWN VISIBILITY RULE, for both gated
// entity types, across the four caller tiers.
//
// The response carries the comment's live score, so a caller refused the thread
// would otherwise watch its activity move; the write itself is attributed and
// lands inside a discussion they may not read. Comment ids are dense and
// sequential, so an answer that varied with the parent's existence is walkable.
//
// The show row and the collection row differ on the admin, and that difference
// is the reason both run: a gate that granted every admin would pass the show
// half and publish every private collection's comment scores.
func TestCommentVote_ParentVisibilityMatrix(t *testing.T) {
	for _, tc := range []struct {
		entityType string
		tiers      []commentVoteTier
	}{
		{"show", showParentTiers},
		{"collection", privateCollectionParentTiers},
	} {
		t.Run(tc.entityType, func(t *testing.T) {
			for _, tier := range tc.tiers {
				t.Run(tier.name, func(t *testing.T) {
					ctx := context.Background()
					if tier.user != nil {
						ctx = testhelpers.CtxWithUser(tier.user)
					}

					t.Run("vote", func(t *testing.T) {
						reached := 0
						h := NewCommentVoteHandler(voteServiceRecordingReach(&reached),
							commentParentReader(tc.entityType), gatedParentVisibility())
						req := &VoteCommentRequest{CommentID: "42"}
						req.Body.Direction = 1
						resp, err := h.VoteCommentHandler(ctx, req)
						assertVoteOutcome(t, tier.want, reached, err, resp == nil)
					})

					t.Run("unvote", func(t *testing.T) {
						reached := 0
						h := NewCommentVoteHandler(voteServiceRecordingReach(&reached),
							commentParentReader(tc.entityType), gatedParentVisibility())
						resp, err := h.UnvoteCommentHandler(ctx, &UnvoteCommentRequest{CommentID: "42"})
						assertVoteOutcome(t, tier.want, reached, err, resp == nil)
					})
				})
			}
		})
	}
}

// assertVoteOutcome checks the status AND whether the vote service ran, because
// a status-only assertion passes for a gate that refuses after writing.
func assertVoteOutcome(t *testing.T, want, reached int, err error, respNil bool) {
	t.Helper()
	if want == 200 {
		if err != nil {
			t.Fatalf("a caller entitled to the parent was refused: %v", err)
		}
		if respNil {
			t.Error("no response body for a granted vote")
		}
		if reached != 1 {
			t.Errorf("the vote service ran %d times for a granted caller, want 1", reached)
		}
		return
	}
	testhelpers.AssertHumaError(t, err, want)
	if reached != 0 {
		t.Errorf("the vote service ran %d times for a refused caller, want 0", reached)
	}
}

// THE REFUSALS ARE ONE RESPONSE, field for field.
//
// The whole value of the gate is that a gated parent and a comment id nobody has
// used are indistinguishable. A stranger's refusal on a gated show, on a private
// collection and on a comment that does not exist must therefore carry the same
// status, title and detail; a difference in any of the three is the oracle
// restated in the error body.
func TestCommentVote_EveryRefusalIsTheSameResponse(t *testing.T) {
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 3})

	refusal := func(t *testing.T, reader CommentReader, vis *testhelpers.MockShowVisibility) *huma.ErrorModel {
		t.Helper()
		h := NewCommentVoteHandler(&testhelpers.MockCommentVoteService{}, reader, vis)
		req := &VoteCommentRequest{CommentID: "42"}
		req.Body.Direction = 1
		_, err := h.VoteCommentHandler(ctx, req)
		var model *huma.ErrorModel
		if !errors.As(err, &model) {
			t.Fatalf("expected a *huma.ErrorModel, got %T: %v", err, err)
		}
		return model
	}

	absent := &testhelpers.MockCommentService{
		GetCommentFn: func(uint) (*contracts.CommentResponse, error) { return nil, nil },
	}

	want := refusal(t, absent, testhelpers.AllShowsVisible())
	for _, tc := range []struct {
		name   string
		reader CommentReader
	}{
		{"a comment on a gated show", commentParentReader("show")},
		{"a comment on a private collection", commentParentReader("collection")},
	} {
		got := refusal(t, tc.reader, gatedParentVisibility())
		if got.Status != want.Status || got.Title != want.Title || got.Detail != want.Detail {
			t.Errorf("%s answers %d/%q/%q, and a comment that does not exist answers %d/%q/%q",
				tc.name, got.Status, got.Title, got.Detail, want.Status, want.Title, want.Detail)
		}
	}
}
