package engagement

import (
	"context"
	"errors"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
)

func ptrString(s string) *string { return &s }

func TestUserFollowHandler_NoAuth(t *testing.T) {
	h := NewUserFollowHandler(nil, nil)
	_, err := h.UserFollowHandler(context.Background(), &UserFollowRequest{Username: "alice"})
	testhelpers.AssertHumaError(t, err, 401)
}

func TestUserFollowHandler_UnknownUserIs404(t *testing.T) {
	users := &testhelpers.MockUserService{
		GetUserByUsernameFn: func(string) (*authm.User, error) { return nil, nil },
	}
	h := NewUserFollowHandler(&testhelpers.MockFollowService{}, users)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	_, err := h.UserFollowHandler(ctx, &UserFollowRequest{Username: "ghost"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestUserFollowHandler_PrivateTargetIs404(t *testing.T) {
	users := &testhelpers.MockUserService{
		GetUserByUsernameFn: func(string) (*authm.User, error) {
			return &authm.User{
				ID:                2,
				Username:          ptrString("secret"),
				ProfileVisibility: "private",
			}, nil
		},
	}
	followCalled := false
	follows := &testhelpers.MockFollowService{
		FollowFn: func(uint, string, uint) error {
			followCalled = true
			return nil
		},
	}
	h := NewUserFollowHandler(follows, users)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	_, err := h.UserFollowHandler(ctx, &UserFollowRequest{Username: "secret"})
	testhelpers.AssertHumaError(t, err, 404)
	if followCalled {
		t.Error("Follow must not be called for a private target")
	}
}

func TestUserFollowHandler_SelfFollowIs422(t *testing.T) {
	users := &testhelpers.MockUserService{
		GetUserByUsernameFn: func(string) (*authm.User, error) {
			return &authm.User{
				ID:                1,
				Username:          ptrString("alice"),
				ProfileVisibility: "public",
			}, nil
		},
	}
	followCalled := false
	follows := &testhelpers.MockFollowService{
		FollowFn: func(uint, string, uint) error {
			followCalled = true
			return nil
		},
	}
	h := NewUserFollowHandler(follows, users)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	_, err := h.UserFollowHandler(ctx, &UserFollowRequest{Username: "alice"})
	testhelpers.AssertHumaError(t, err, 422)
	if followCalled {
		t.Error("Follow must not be called for self-follow")
	}
}

func TestUserFollowHandler_Success(t *testing.T) {
	users := &testhelpers.MockUserService{
		GetUserByUsernameFn: func(username string) (*authm.User, error) {
			if username != "bob" {
				t.Errorf("unexpected username: %s", username)
			}
			return &authm.User{
				ID:                7,
				Username:          ptrString("bob"),
				ProfileVisibility: "public",
			}, nil
		},
	}
	follows := &testhelpers.MockFollowService{
		FollowFn: func(userID uint, entityType string, entityID uint) error {
			if userID != 1 || entityType != userEntityType || entityID != 7 {
				t.Errorf("unexpected args: userID=%d, entityType=%s, entityID=%d", userID, entityType, entityID)
			}
			return nil
		},
	}
	h := NewUserFollowHandler(follows, users)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	resp, err := h.UserFollowHandler(ctx, &UserFollowRequest{Username: "bob"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true")
	}
}

func TestUserUnfollowHandler_Success(t *testing.T) {
	users := &testhelpers.MockUserService{
		GetUserByUsernameIncludingClosedFn: func(string) (*authm.User, error) {
			return &authm.User{
				ID:                7,
				Username:          ptrString("bob"),
				ProfileVisibility: "public",
				IsActive:          true,
			}, nil
		},
	}
	follows := &testhelpers.MockFollowService{
		UnfollowFn: func(userID uint, entityType string, entityID uint) error {
			if userID != 1 || entityType != userEntityType || entityID != 7 {
				t.Errorf("unexpected args: userID=%d, entityType=%s, entityID=%d", userID, entityType, entityID)
			}
			return nil
		},
	}
	h := NewUserFollowHandler(follows, users)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	resp, err := h.UserUnfollowHandler(ctx, &UserUnfollowRequest{Username: "bob"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true")
	}
}

func TestUserUnfollowHandler_PrivateTargetNonFollowerIs404(t *testing.T) {
	users := &testhelpers.MockUserService{
		GetUserByUsernameIncludingClosedFn: func(string) (*authm.User, error) {
			return &authm.User{
				ID:                2,
				Username:          ptrString("secret"),
				ProfileVisibility: "private",
				IsActive:          true,
			}, nil
		},
	}
	unfollowCalled := false
	follows := &testhelpers.MockFollowService{
		UnfollowFn: func(uint, string, uint) error {
			unfollowCalled = true
			return nil
		},
	}
	h := NewUserFollowHandler(follows, users)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	_, err := h.UserUnfollowHandler(ctx, &UserUnfollowRequest{Username: "secret"})
	testhelpers.AssertHumaError(t, err, 404)
	if unfollowCalled {
		t.Error("Unfollow must not be called for a caller who does not follow the private target")
	}
}

func TestUserUnfollowHandler_PrivateTargetFollowerUnfollows(t *testing.T) {
	users := &testhelpers.MockUserService{
		GetUserByUsernameIncludingClosedFn: func(string) (*authm.User, error) {
			return &authm.User{
				ID:                2,
				Username:          ptrString("secret"),
				ProfileVisibility: "private",
				IsActive:          true,
			}, nil
		},
	}
	unfollowed := false
	follows := &testhelpers.MockFollowService{
		IsFollowingFn: func(userID uint, entityType string, entityID uint) (bool, error) {
			return userID == 1 && entityType == userEntityType && entityID == 2, nil
		},
		UnfollowFn: func(userID uint, entityType string, entityID uint) error {
			unfollowed = userID == 1 && entityType == userEntityType && entityID == 2
			return nil
		},
	}
	h := NewUserFollowHandler(follows, users)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	resp, err := h.UserUnfollowHandler(ctx, &UserUnfollowRequest{Username: "secret"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success || !unfollowed {
		t.Errorf("success=%v unfollowed=%v, want both", resp.Body.Success, unfollowed)
	}
}

// closedUnfollowTarget is a deactivated account, which CanSignIn refuses.
func closedUnfollowTarget(string) (*authm.User, error) {
	return &authm.User{ID: 9, Username: ptrString("gone"), ProfileVisibility: "public", IsActive: false}, nil
}

func TestUserUnfollowHandler_ClosedTargetFollowerUnfollows(t *testing.T) {
	users := &testhelpers.MockUserService{GetUserByUsernameIncludingClosedFn: closedUnfollowTarget}
	unfollowed := false
	follows := &testhelpers.MockFollowService{
		IsFollowingFn: func(userID uint, entityType string, entityID uint) (bool, error) {
			return userID == 1 && entityType == userEntityType && entityID == 9, nil
		},
		UnfollowFn: func(userID uint, entityType string, entityID uint) error {
			unfollowed = userID == 1 && entityType == userEntityType && entityID == 9
			return nil
		},
	}
	h := NewUserFollowHandler(follows, users)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	resp, err := h.UserUnfollowHandler(ctx, &UserUnfollowRequest{Username: "gone"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success || !unfollowed {
		t.Errorf("success=%v unfollowed=%v, want both", resp.Body.Success, unfollowed)
	}
}

func TestUserUnfollowHandler_ClosedTargetNonFollowerIs404(t *testing.T) {
	users := &testhelpers.MockUserService{GetUserByUsernameIncludingClosedFn: closedUnfollowTarget}
	unfollowCalled := false
	follows := &testhelpers.MockFollowService{
		IsFollowingFn: func(uint, string, uint) (bool, error) { return false, nil },
		UnfollowFn: func(uint, string, uint) error {
			unfollowCalled = true
			return nil
		},
	}
	h := NewUserFollowHandler(follows, users)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	_, err := h.UserUnfollowHandler(ctx, &UserUnfollowRequest{Username: "gone"})
	testhelpers.AssertHumaError(t, err, 404)
	if unfollowCalled {
		t.Error("Unfollow must not be called for a caller who does not follow the closed target")
	}
}

func TestUserUnfollowHandler_ClosedTargetFollowStatusFailureIs500(t *testing.T) {
	users := &testhelpers.MockUserService{GetUserByUsernameIncludingClosedFn: closedUnfollowTarget}
	follows := &testhelpers.MockFollowService{
		IsFollowingFn: func(uint, string, uint) (bool, error) { return false, errors.New("db down") },
	}
	h := NewUserFollowHandler(follows, users)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	_, err := h.UserUnfollowHandler(ctx, &UserUnfollowRequest{Username: "gone"})
	testhelpers.AssertHumaError(t, err, 500)
}

func TestUserFollowersHandler_UnknownUserIs404(t *testing.T) {
	users := &testhelpers.MockUserService{
		GetUserByUsernameFn: func(string) (*authm.User, error) { return nil, nil },
	}
	h := NewUserFollowHandler(&testhelpers.MockFollowService{}, users)

	_, err := h.UserFollowersHandler(context.Background(), &UserFollowersRequest{Username: "ghost"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestUserFollowersHandler_PrivateTargetIs404(t *testing.T) {
	users := &testhelpers.MockUserService{
		GetUserByUsernameFn: func(string) (*authm.User, error) {
			return &authm.User{
				ID:                2,
				Username:          ptrString("secret"),
				ProfileVisibility: "private",
			}, nil
		},
	}
	h := NewUserFollowHandler(&testhelpers.MockFollowService{}, users)

	_, err := h.UserFollowersHandler(context.Background(), &UserFollowersRequest{Username: "secret"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestUserFollowersHandler_OwnerCanReadPrivate(t *testing.T) {
	users := &testhelpers.MockUserService{
		GetUserByUsernameFn: func(string) (*authm.User, error) {
			return &authm.User{
				ID:                2,
				Username:          ptrString("secret"),
				ProfileVisibility: "private",
			}, nil
		},
	}
	follows := &testhelpers.MockFollowService{
		GetFollowerCountFn: func(entityType string, entityID uint) (int64, error) {
			if entityType != userEntityType || entityID != 2 {
				t.Errorf("unexpected args: %s/%d", entityType, entityID)
			}
			return 4, nil
		},
		IsFollowingFn: func(uint, string, uint) (bool, error) { return false, nil },
	}
	h := NewUserFollowHandler(follows, users)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 2})

	resp, err := h.UserFollowersHandler(ctx, &UserFollowersRequest{Username: "secret"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.FollowerCount != 4 {
		t.Errorf("expected count=4, got %d", resp.Body.FollowerCount)
	}
}

func TestUserFollowersHandler_CountAndStatus(t *testing.T) {
	users := &testhelpers.MockUserService{
		GetUserByUsernameFn: func(string) (*authm.User, error) {
			return &authm.User{
				ID:                7,
				Username:          ptrString("bob"),
				ProfileVisibility: "public",
			}, nil
		},
	}
	follows := &testhelpers.MockFollowService{
		GetFollowerCountFn: func(entityType string, entityID uint) (int64, error) {
			if entityType != userEntityType || entityID != 7 {
				t.Errorf("unexpected args: %s/%d", entityType, entityID)
			}
			return 3, nil
		},
		IsFollowingFn: func(userID uint, entityType string, entityID uint) (bool, error) {
			return userID == 1 && entityType == userEntityType && entityID == 7, nil
		},
	}
	h := NewUserFollowHandler(follows, users)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	resp, err := h.UserFollowersHandler(ctx, &UserFollowersRequest{Username: "bob"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.FollowerCount != 3 || !resp.Body.IsFollowing {
		t.Errorf("expected count=3 following=true, got %+v", resp.Body)
	}
	if resp.Body.Username != "bob" {
		t.Errorf("expected username echoed, got %q", resp.Body.Username)
	}
}

func TestUserFollowersHandler_UnauthedHasNoIsFollowing(t *testing.T) {
	users := &testhelpers.MockUserService{
		GetUserByUsernameFn: func(string) (*authm.User, error) {
			return &authm.User{
				ID:                7,
				Username:          ptrString("bob"),
				ProfileVisibility: "public",
			}, nil
		},
	}
	follows := &testhelpers.MockFollowService{
		GetFollowerCountFn: func(string, uint) (int64, error) { return 2, nil },
		IsFollowingFn: func(uint, string, uint) (bool, error) {
			t.Error("IsFollowing must not be called without auth")
			return true, nil
		},
	}
	h := NewUserFollowHandler(follows, users)

	resp, err := h.UserFollowersHandler(context.Background(), &UserFollowersRequest{Username: "bob"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.FollowerCount != 2 || resp.Body.IsFollowing {
		t.Errorf("expected count=2 following=false, got %+v", resp.Body)
	}
}

func TestUserUnfollowHandler_LookupFailureIs500(t *testing.T) {
	users := &testhelpers.MockUserService{
		GetUserByUsernameIncludingClosedFn: func(string) (*authm.User, error) { return nil, errors.New("db down") },
	}
	h := NewUserFollowHandler(&testhelpers.MockFollowService{}, users)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	_, err := h.UserUnfollowHandler(ctx, &UserUnfollowRequest{Username: "anyone"})
	testhelpers.AssertHumaError(t, err, 500)
}
