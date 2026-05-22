package admin

import (
	"fmt"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// =============================================================================
// NewAutoPromotionHandler
// =============================================================================

func TestNewAutoPromotionHandler_WiresService(t *testing.T) {
	svc := &testhelpers.MockAutoPromotionService{}
	h := NewAutoPromotionHandler(svc)
	if h == nil {
		t.Fatal("NewAutoPromotionHandler returned nil")
	}
	if h.autoPromotionService != svc {
		t.Error("handler did not retain the injected auto-promotion service")
	}
}

// =============================================================================
// EvaluateAllUsersHandler — admin-gated; middleware guarantees a user.
// =============================================================================

func TestEvaluateAllUsersHandler_Success(t *testing.T) {
	mock := &testhelpers.MockAutoPromotionService{
		EvaluateAllUsersFn: func() (*contracts.AutoPromotionResult, error) {
			return &contracts.AutoPromotionResult{
				Promoted:  []contracts.UserTierChange{{UserID: 2, NewTier: "contributor"}},
				Demoted:   nil,
				Unchanged: 5,
				Errors:    0,
			}, nil
		},
	}
	h := NewAutoPromotionHandler(mock)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})

	resp, err := h.EvaluateAllUsersHandler(ctx, &EvaluateAllUsersRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Body.Promoted) != 1 || resp.Body.Unchanged != 5 {
		t.Errorf("unexpected result body: %+v", resp.Body)
	}
}

func TestEvaluateAllUsersHandler_ServiceError(t *testing.T) {
	mock := &testhelpers.MockAutoPromotionService{
		EvaluateAllUsersFn: func() (*contracts.AutoPromotionResult, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := NewAutoPromotionHandler(mock)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})

	_, err := h.EvaluateAllUsersHandler(ctx, &EvaluateAllUsersRequest{})
	testhelpers.AssertHumaError(t, err, 500)
}

// =============================================================================
// EvaluateUserHandler — distinguishes "user not found" (404) from other 500s.
// =============================================================================

func TestEvaluateUserHandler_Success(t *testing.T) {
	mock := &testhelpers.MockAutoPromotionService{
		EvaluateUserFn: func(userID uint) (*contracts.UserEvaluationResult, error) {
			if userID != 42 {
				t.Errorf("unexpected userID=%d", userID)
			}
			return &contracts.UserEvaluationResult{
				UserID:      42,
				CurrentTier: "new",
				Changed:     true,
				NewTier:     "contributor",
			}, nil
		},
	}
	h := NewAutoPromotionHandler(mock)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})

	resp, err := h.EvaluateUserHandler(ctx, &EvaluateUserRequest{UserID: 42})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.UserID != 42 || resp.Body.NewTier != "contributor" {
		t.Errorf("unexpected result body: %+v", resp.Body)
	}
}

func TestEvaluateUserHandler_NotFound(t *testing.T) {
	mock := &testhelpers.MockAutoPromotionService{
		EvaluateUserFn: func(_ uint) (*contracts.UserEvaluationResult, error) {
			// The handler maps the typed user-not-found error to 404.
			return nil, apperrors.ErrAutoPromotionUserNotFound()
		},
	}
	h := NewAutoPromotionHandler(mock)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})

	_, err := h.EvaluateUserHandler(ctx, &EvaluateUserRequest{UserID: 999})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestEvaluateUserHandler_ServiceError(t *testing.T) {
	mock := &testhelpers.MockAutoPromotionService{
		EvaluateUserFn: func(_ uint) (*contracts.UserEvaluationResult, error) {
			return nil, fmt.Errorf("connection reset")
		},
	}
	h := NewAutoPromotionHandler(mock)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})

	_, err := h.EvaluateUserHandler(ctx, &EvaluateUserRequest{UserID: 7})
	testhelpers.AssertHumaError(t, err, 500)
}
