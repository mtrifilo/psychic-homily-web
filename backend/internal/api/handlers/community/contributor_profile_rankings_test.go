package community

import (
	"context"
	"fmt"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// ============================================================================
// GetPercentileRankingsHandler
//
// The full privacy matrix is exercised by the integration suite; these unit
// tests cover the handler-level branches (public happy path, user-not-found,
// private-profile gate, and the two service-error paths) without a database.
// ============================================================================

func TestGetPercentileRankings_PublicSuccess(t *testing.T) {
	mockUsers := &testhelpers.MockUserService{
		GetUserByUsernameFn: func(username string) (*authm.User, error) {
			// Public profile (default visibility + default privacy settings).
			return &authm.User{ID: 5, ProfileVisibility: "public"}, nil
		},
	}
	mockProfile := &testhelpers.MockContributorProfileService{
		GetPercentileRankingsFn: func(userID uint) (*contracts.PercentileRankings, error) {
			if userID != 5 {
				t.Errorf("expected userID=5, got %d", userID)
			}
			return &contracts.PercentileRankings{
				Rankings:     []contracts.PercentileRanking{},
				OverallScore: 72,
			}, nil
		},
	}
	h := NewContributorProfileHandler(mockProfile, mockUsers, nil, nil)

	// Anonymous viewer — public profile falls through to full rankings.
	resp, err := h.GetPercentileRankingsHandler(context.Background(), &GetPercentileRankingsRequest{Username: "johndoe"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.OverallScore != 72 {
		t.Errorf("expected overall score=72, got %d", resp.Body.OverallScore)
	}
}

func TestGetPercentileRankings_UserNotFound(t *testing.T) {
	mockUsers := &testhelpers.MockUserService{
		GetUserByUsernameFn: func(_ string) (*authm.User, error) {
			return nil, nil // not found → handler returns 404
		},
	}
	h := NewContributorProfileHandler(&testhelpers.MockContributorProfileService{}, mockUsers, nil, nil)

	_, err := h.GetPercentileRankingsHandler(context.Background(), &GetPercentileRankingsRequest{Username: "ghost"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestGetPercentileRankings_PrivateProfileHiddenFromOthers(t *testing.T) {
	mockUsers := &testhelpers.MockUserService{
		GetUserByUsernameFn: func(_ string) (*authm.User, error) {
			return &authm.User{ID: 5, ProfileVisibility: "private"}, nil
		},
	}
	h := NewContributorProfileHandler(&testhelpers.MockContributorProfileService{}, mockUsers, nil, nil)

	// Anonymous (non-owner) viewer → private profile masked as 404.
	_, err := h.GetPercentileRankingsHandler(context.Background(), &GetPercentileRankingsRequest{Username: "private-user"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestGetPercentileRankings_UserLookupError(t *testing.T) {
	mockUsers := &testhelpers.MockUserService{
		GetUserByUsernameFn: func(_ string) (*authm.User, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := NewContributorProfileHandler(&testhelpers.MockContributorProfileService{}, mockUsers, nil, nil)

	_, err := h.GetPercentileRankingsHandler(context.Background(), &GetPercentileRankingsRequest{Username: "johndoe"})
	testhelpers.AssertHumaError(t, err, 500)
}

func TestGetPercentileRankings_RankingsServiceError(t *testing.T) {
	mockUsers := &testhelpers.MockUserService{
		GetUserByUsernameFn: func(_ string) (*authm.User, error) {
			return &authm.User{ID: 5, ProfileVisibility: "public"}, nil
		},
	}
	mockProfile := &testhelpers.MockContributorProfileService{
		GetPercentileRankingsFn: func(_ uint) (*contracts.PercentileRankings, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := NewContributorProfileHandler(mockProfile, mockUsers, nil, nil)

	_, err := h.GetPercentileRankingsHandler(context.Background(), &GetPercentileRankingsRequest{Username: "johndoe"})
	testhelpers.AssertHumaError(t, err, 500)
}
