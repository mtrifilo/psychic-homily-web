package auth

import (
	"fmt"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

func TestNewAdvancementHandler_WiresService(t *testing.T) {
	svc := &testhelpers.MockAutoPromotionService{}
	h := NewAdvancementHandler(svc)
	if h == nil || h.autoPromotionService != svc {
		t.Fatal("handler did not retain injected service")
	}
}

func TestGetAdvancementHandler_Success(t *testing.T) {
	mock := &testhelpers.MockAutoPromotionService{
		GetAdvancementProgressFn: func(userID uint) (*contracts.AdvancementProgress, error) {
			if userID != 7 {
				t.Errorf("unexpected userID=%d", userID)
			}
			current := 32.0
			threshold := 50.0
			return &contracts.AdvancementProgress{
				CurrentTier: "trusted_contributor",
				NextTier:    "local_ambassador",
				Requirements: []contracts.AdvancementRequirement{{
					Requirement: contracts.AdvancementReqApprovedEdits,
					Current:     &current,
					Threshold:   &threshold,
					Met:         false,
				}},
			}, nil
		},
	}
	h := NewAdvancementHandler(mock)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 7})

	resp, err := h.GetAdvancementHandler(ctx, &struct{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.CurrentTier != "trusted_contributor" || resp.Body.NextTier != "local_ambassador" {
		t.Errorf("unexpected body: %+v", resp.Body)
	}
	if len(resp.Body.Requirements) != 1 {
		t.Fatalf("want 1 requirement, got %d", len(resp.Body.Requirements))
	}
}

func TestGetAdvancementHandler_UsesCallerIDOnly(t *testing.T) {
	// Confirms the handler never accepts a path/query user_id — only the
	// JWT principal is evaluated (own-user scope).
	var seen uint
	mock := &testhelpers.MockAutoPromotionService{
		GetAdvancementProgressFn: func(userID uint) (*contracts.AdvancementProgress, error) {
			seen = userID
			return &contracts.AdvancementProgress{CurrentTier: "new_user"}, nil
		},
	}
	h := NewAdvancementHandler(mock)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 42})

	if _, err := h.GetAdvancementHandler(ctx, &struct{}{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if seen != 42 {
		t.Errorf("evaluated userID=%d, want 42", seen)
	}
}

func TestGetAdvancementHandler_NoUserContext(t *testing.T) {
	h := NewAdvancementHandler(&testhelpers.MockAutoPromotionService{})
	_, err := h.GetAdvancementHandler(t.Context(), &struct{}{})
	testhelpers.AssertHumaError(t, err, 401)
}

func TestGetAdvancementHandler_UserNotFound(t *testing.T) {
	mock := &testhelpers.MockAutoPromotionService{
		GetAdvancementProgressFn: func(_ uint) (*contracts.AdvancementProgress, error) {
			return nil, apperrors.ErrAutoPromotionUserNotFound()
		},
	}
	h := NewAdvancementHandler(mock)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	_, err := h.GetAdvancementHandler(ctx, &struct{}{})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestGetAdvancementHandler_ServiceError(t *testing.T) {
	mock := &testhelpers.MockAutoPromotionService{
		GetAdvancementProgressFn: func(_ uint) (*contracts.AdvancementProgress, error) {
			return nil, fmt.Errorf("connection reset")
		},
	}
	h := NewAdvancementHandler(mock)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	_, err := h.GetAdvancementHandler(ctx, &struct{}{})
	testhelpers.AssertHumaError(t, err, 500)
}
