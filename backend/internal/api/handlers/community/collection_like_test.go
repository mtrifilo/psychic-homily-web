package community

import (
	"context"
	"fmt"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
)

// Uses auto-generated testhelpers.MockCollectionService from handler_unit_mock_helpers_test.go.

func testCollectionLikeHandler(svc *testhelpers.MockCollectionService) *CollectionLikeHandler {
	if svc == nil {
		svc = &testhelpers.MockCollectionService{}
	}
	return NewCollectionLikeHandler(svc)
}

// ============================================================================
// LikeCollectionHandler
// ============================================================================

func TestLikeCollection_NoAuth(t *testing.T) {
	h := testCollectionLikeHandler(nil)
	req := &LikeCollectionRequest{Slug: "my-collection"}

	_, err := h.LikeCollectionHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 401)
}

func TestLikeCollection_Success(t *testing.T) {
	h := testCollectionLikeHandler(&testhelpers.MockCollectionService{
		LikeFn: func(slug string, userID uint) (*contracts.CollectionLikeResponse, error) {
			if slug != "my-collection" || userID != 7 {
				return nil, fmt.Errorf("unexpected args: %s, %d", slug, userID)
			}
			return &contracts.CollectionLikeResponse{LikeCount: 4, UserLikesThis: true}, nil
		},
	})

	ctx := testhelpers.CtxWithUser(&models.User{ID: 7})
	req := &LikeCollectionRequest{Slug: "my-collection"}

	resp, err := h.LikeCollectionHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.LikeCount != 4 {
		t.Errorf("expected like_count=4, got %d", resp.Body.LikeCount)
	}
	if !resp.Body.UserLikesThis {
		t.Errorf("expected user_likes_this=true")
	}
}

func TestLikeCollection_Idempotent(t *testing.T) {
	calls := 0
	h := testCollectionLikeHandler(&testhelpers.MockCollectionService{
		LikeFn: func(slug string, userID uint) (*contracts.CollectionLikeResponse, error) {
			calls++
			// Same response — service guarantees the count doesn't double.
			return &contracts.CollectionLikeResponse{LikeCount: 1, UserLikesThis: true}, nil
		},
	})

	ctx := testhelpers.CtxWithUser(&models.User{ID: 1})
	req := &LikeCollectionRequest{Slug: "x"}

	for i := 0; i < 2; i++ {
		resp, err := h.LikeCollectionHandler(ctx, req)
		if err != nil {
			t.Fatalf("call %d unexpected error: %v", i, err)
		}
		if resp.Body.LikeCount != 1 {
			t.Errorf("call %d expected like_count=1, got %d", i, resp.Body.LikeCount)
		}
	}
	if calls != 2 {
		t.Errorf("expected 2 service calls, got %d", calls)
	}
}

func TestLikeCollection_NotFound(t *testing.T) {
	h := testCollectionLikeHandler(&testhelpers.MockCollectionService{
		LikeFn: func(slug string, userID uint) (*contracts.CollectionLikeResponse, error) {
			return nil, apperrors.ErrCollectionNotFound(slug)
		},
	})

	ctx := testhelpers.CtxWithUser(&models.User{ID: 1})
	req := &LikeCollectionRequest{Slug: "missing"}

	_, err := h.LikeCollectionHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 404)
}

func TestLikeCollection_Forbidden(t *testing.T) {
	h := testCollectionLikeHandler(&testhelpers.MockCollectionService{
		LikeFn: func(slug string, userID uint) (*contracts.CollectionLikeResponse, error) {
			return nil, apperrors.ErrCollectionForbidden(slug)
		},
	})

	ctx := testhelpers.CtxWithUser(&models.User{ID: 1})
	req := &LikeCollectionRequest{Slug: "private"}

	_, err := h.LikeCollectionHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 403)
}

func TestLikeCollection_ServiceError(t *testing.T) {
	h := testCollectionLikeHandler(&testhelpers.MockCollectionService{
		LikeFn: func(slug string, userID uint) (*contracts.CollectionLikeResponse, error) {
			return nil, fmt.Errorf("database error")
		},
	})

	ctx := testhelpers.CtxWithUser(&models.User{ID: 1})
	req := &LikeCollectionRequest{Slug: "x"}

	_, err := h.LikeCollectionHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// UnlikeCollectionHandler
// ============================================================================

func TestUnlikeCollection_NoAuth(t *testing.T) {
	h := testCollectionLikeHandler(nil)
	req := &UnlikeCollectionRequest{Slug: "my-collection"}

	_, err := h.UnlikeCollectionHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 401)
}

func TestUnlikeCollection_Success(t *testing.T) {
	h := testCollectionLikeHandler(&testhelpers.MockCollectionService{
		UnlikeFn: func(slug string, userID uint) (*contracts.CollectionLikeResponse, error) {
			return &contracts.CollectionLikeResponse{LikeCount: 0, UserLikesThis: false}, nil
		},
	})

	ctx := testhelpers.CtxWithUser(&models.User{ID: 1})
	req := &UnlikeCollectionRequest{Slug: "x"}

	resp, err := h.UnlikeCollectionHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.LikeCount != 0 {
		t.Errorf("expected like_count=0, got %d", resp.Body.LikeCount)
	}
	if resp.Body.UserLikesThis {
		t.Errorf("expected user_likes_this=false")
	}
}

func TestUnlikeCollection_NotLiked_NoOp(t *testing.T) {
	h := testCollectionLikeHandler(&testhelpers.MockCollectionService{
		UnlikeFn: func(slug string, userID uint) (*contracts.CollectionLikeResponse, error) {
			return &contracts.CollectionLikeResponse{LikeCount: 0, UserLikesThis: false}, nil
		},
	})

	ctx := testhelpers.CtxWithUser(&models.User{ID: 1})
	req := &UnlikeCollectionRequest{Slug: "never-liked"}

	resp, err := h.UnlikeCollectionHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.LikeCount != 0 || resp.Body.UserLikesThis {
		t.Errorf("expected zero/false response on no-op unlike")
	}
}

func TestUnlikeCollection_NotFound(t *testing.T) {
	h := testCollectionLikeHandler(&testhelpers.MockCollectionService{
		UnlikeFn: func(slug string, userID uint) (*contracts.CollectionLikeResponse, error) {
			return nil, apperrors.ErrCollectionNotFound(slug)
		},
	})

	ctx := testhelpers.CtxWithUser(&models.User{ID: 1})
	req := &UnlikeCollectionRequest{Slug: "gone"}

	_, err := h.UnlikeCollectionHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 404)
}

func TestUnlikeCollection_ServiceError(t *testing.T) {
	h := testCollectionLikeHandler(&testhelpers.MockCollectionService{
		UnlikeFn: func(slug string, userID uint) (*contracts.CollectionLikeResponse, error) {
			return nil, fmt.Errorf("database error")
		},
	})

	ctx := testhelpers.CtxWithUser(&models.User{ID: 1})
	req := &UnlikeCollectionRequest{Slug: "x"}

	_, err := h.UnlikeCollectionHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 500)
}
