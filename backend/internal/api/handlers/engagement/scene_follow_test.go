package engagement

import (
	"context"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
)

// PSY-1339: scene follows are slug-addressed; the handler resolves the slug
// through the registry (get-or-create on follow, lookup-only elsewhere) and
// delegates to the same FollowService as every other entity.

func TestSceneFollowHandler_NoAuth(t *testing.T) {
	h := NewSceneFollowHandler(nil, nil)
	_, err := h.SceneFollowHandler(context.Background(), &SceneFollowRequest{Slug: "phoenix-az"})
	testhelpers.AssertHumaError(t, err, 401)
}

func TestSceneFollowHandler_UnknownSlugIs404(t *testing.T) {
	scenes := &testhelpers.MockSceneService{
		GetOrCreateSceneIDFn: func(slug string) (uint, error) {
			return 0, apperrors.ErrSceneNotFound("scene not found for slug: " + slug)
		},
	}
	h := NewSceneFollowHandler(&testhelpers.MockFollowService{}, scenes)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	_, err := h.SceneFollowHandler(ctx, &SceneFollowRequest{Slug: "nowhere-zz"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestSceneFollowHandler_Success(t *testing.T) {
	scenes := &testhelpers.MockSceneService{
		GetOrCreateSceneIDFn: func(slug string) (uint, error) {
			if slug != "phoenix-az" {
				t.Errorf("unexpected slug: %s", slug)
			}
			return 7, nil
		},
	}
	follows := &testhelpers.MockFollowService{
		FollowFn: func(userID uint, entityType string, entityID uint) error {
			if userID != 1 || entityType != "scene" || entityID != 7 {
				t.Errorf("unexpected args: userID=%d, entityType=%s, entityID=%d", userID, entityType, entityID)
			}
			return nil
		},
	}
	h := NewSceneFollowHandler(follows, scenes)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	resp, err := h.SceneFollowHandler(ctx, &SceneFollowRequest{Slug: "phoenix-az"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true")
	}
}

func TestSceneFollowHandler_SetsNotifyMode(t *testing.T) {
	modeSet := ""
	scenes := &testhelpers.MockSceneService{
		GetOrCreateSceneIDFn: func(string) (uint, error) { return 7, nil },
	}
	follows := &testhelpers.MockFollowService{
		FollowFn: func(uint, string, uint) error { return nil },
		SetSceneNotifyModeFn: func(userID uint, sceneID uint, mode string) error {
			if userID != 1 || sceneID != 7 {
				t.Errorf("unexpected args: %d/%d", userID, sceneID)
			}
			modeSet = mode
			return nil
		},
	}
	h := NewSceneFollowHandler(follows, scenes)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	req := &SceneFollowRequest{
		Slug: "phoenix-az",
		Body: &SceneFollowBody{NotifyMode: "followed_bands_only"},
	}
	if _, err := h.SceneFollowHandler(ctx, req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if modeSet != "followed_bands_only" {
		t.Errorf("expected mode set, got %q", modeSet)
	}
}

func TestSceneFollowersHandler_IncludesMyMode(t *testing.T) {
	scenes := &testhelpers.MockSceneService{
		LookupSceneIDFn: func(string) (uint, bool, error) { return 7, true, nil },
	}
	follows := &testhelpers.MockFollowService{
		GetFollowerCountFn: func(string, uint) (int64, error) { return 1, nil },
		IsFollowingFn:      func(uint, string, uint) (bool, error) { return true, nil },
		SceneNotifyModeFn:  func(uint, uint) (string, error) { return "followed_bands_only", nil },
	}
	h := NewSceneFollowHandler(follows, scenes)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	resp, err := h.SceneFollowersHandler(ctx, &SceneFollowersRequest{Slug: "phoenix-az"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.NotifyMode != "followed_bands_only" {
		t.Errorf("expected mode in response, got %q", resp.Body.NotifyMode)
	}
}

func TestSceneUnfollowHandler_AbsentRowIsIdempotentSuccess(t *testing.T) {
	unfollowCalled := false
	scenes := &testhelpers.MockSceneService{
		LookupSceneIDFn: func(string) (uint, bool, error) { return 0, false, nil },
	}
	follows := &testhelpers.MockFollowService{
		UnfollowFn: func(uint, string, uint) error {
			unfollowCalled = true
			return nil
		},
	}
	h := NewSceneFollowHandler(follows, scenes)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	resp, err := h.SceneUnfollowHandler(ctx, &SceneUnfollowRequest{Slug: "phoenix-az"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true")
	}
	if unfollowCalled {
		// No registry row → nothing to unfollow; it must NOT materialize one.
		t.Error("Unfollow must not be called when no scene row exists")
	}
}

func TestSceneFollowersHandler_NoRowIsZeroState(t *testing.T) {
	scenes := &testhelpers.MockSceneService{
		LookupSceneIDFn: func(string) (uint, bool, error) { return 0, false, nil },
	}
	h := NewSceneFollowHandler(&testhelpers.MockFollowService{}, scenes)

	resp, err := h.SceneFollowersHandler(context.Background(), &SceneFollowersRequest{Slug: "phoenix-az"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.FollowerCount != 0 || resp.Body.IsFollowing {
		t.Errorf("expected zero-state, got %+v", resp.Body)
	}
	if resp.Body.Slug != "phoenix-az" {
		t.Errorf("expected slug echoed, got %q", resp.Body.Slug)
	}
}

func TestSceneFollowersHandler_CountAndStatus(t *testing.T) {
	scenes := &testhelpers.MockSceneService{
		LookupSceneIDFn: func(string) (uint, bool, error) { return 7, true, nil },
	}
	follows := &testhelpers.MockFollowService{
		GetFollowerCountFn: func(entityType string, entityID uint) (int64, error) {
			if entityType != "scene" || entityID != 7 {
				t.Errorf("unexpected args: %s/%d", entityType, entityID)
			}
			return 3, nil
		},
		IsFollowingFn: func(userID uint, entityType string, entityID uint) (bool, error) {
			return userID == 1 && entityType == "scene" && entityID == 7, nil
		},
	}
	h := NewSceneFollowHandler(follows, scenes)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	resp, err := h.SceneFollowersHandler(ctx, &SceneFollowersRequest{Slug: "phoenix-az"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.FollowerCount != 3 || !resp.Body.IsFollowing {
		t.Errorf("expected count=3 following=true, got %+v", resp.Body)
	}
}
