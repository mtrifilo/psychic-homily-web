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
// NewLeaderboardHandler
// ============================================================================

func TestNewLeaderboardHandler_WiresService(t *testing.T) {
	svc := &testhelpers.MockLeaderboardService{}
	h := NewLeaderboardHandler(svc)
	if h == nil {
		t.Fatal("NewLeaderboardHandler returned nil")
	}
	if h.leaderboardService != svc {
		t.Error("handler did not retain the injected leaderboard service")
	}
}

// ============================================================================
// GetLeaderboardHandler
// ============================================================================

func TestGetLeaderboard_DefaultsAndAnonymous(t *testing.T) {
	mock := &testhelpers.MockLeaderboardService{
		GetLeaderboardFn: func(dimension, period string, limit int) ([]contracts.LeaderboardEntry, error) {
			// Empty request → handler applies defaults.
			if dimension != "overall" || period != "all_time" || limit != 25 {
				t.Errorf("expected defaults overall/all_time/25, got %s/%s/%d", dimension, period, limit)
			}
			return []contracts.LeaderboardEntry{{Rank: 1, UserID: 2, Username: "top", Count: 99}}, nil
		},
	}
	h := NewLeaderboardHandler(mock)

	// Anonymous context → no user-rank lookup, UserRank stays nil.
	resp, err := h.GetLeaderboardHandler(context.Background(), &GetLeaderboardRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Dimension != "overall" || resp.Body.Period != "all_time" {
		t.Errorf("unexpected echoed dimension/period: %+v", resp.Body)
	}
	if len(resp.Body.Entries) != 1 || resp.Body.Entries[0].Username != "top" {
		t.Errorf("unexpected entries: %+v", resp.Body.Entries)
	}
	if resp.Body.UserRank != nil {
		t.Errorf("expected nil UserRank for anonymous viewer, got %v", *resp.Body.UserRank)
	}
}

func TestGetLeaderboard_AuthenticatedComputesUserRank(t *testing.T) {
	rank := 4
	mock := &testhelpers.MockLeaderboardService{
		GetLeaderboardFn: func(_, _ string, _ int) ([]contracts.LeaderboardEntry, error) {
			return []contracts.LeaderboardEntry{}, nil
		},
		GetUserRankFn: func(userID uint, dimension, period string) (*int, error) {
			if userID != 7 {
				t.Errorf("expected userID=7, got %d", userID)
			}
			return &rank, nil
		},
	}
	h := NewLeaderboardHandler(mock)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 7})

	resp, err := h.GetLeaderboardHandler(ctx, &GetLeaderboardRequest{Dimension: "shows", Period: "month", Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.UserRank == nil || *resp.Body.UserRank != 4 {
		t.Errorf("expected UserRank=4, got %v", resp.Body.UserRank)
	}
}

func TestGetLeaderboard_UserRankErrorIsNonFatal(t *testing.T) {
	// A GetUserRank failure must not fail the request — the board still returns.
	mock := &testhelpers.MockLeaderboardService{
		GetLeaderboardFn: func(_, _ string, _ int) ([]contracts.LeaderboardEntry, error) {
			return []contracts.LeaderboardEntry{}, nil
		},
		GetUserRankFn: func(_ uint, _, _ string) (*int, error) {
			return nil, fmt.Errorf("rank query failed")
		},
	}
	h := NewLeaderboardHandler(mock)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 7})

	resp, err := h.GetLeaderboardHandler(ctx, &GetLeaderboardRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.UserRank != nil {
		t.Errorf("expected nil UserRank after non-fatal error, got %v", *resp.Body.UserRank)
	}
}

func TestGetLeaderboard_InvalidDimension(t *testing.T) {
	mock := &testhelpers.MockLeaderboardService{
		GetLeaderboardFn: func(dimension, _ string, _ int) ([]contracts.LeaderboardEntry, error) {
			// Handler maps this exact message to 422.
			return nil, fmt.Errorf("invalid dimension: %s", dimension)
		},
	}
	h := NewLeaderboardHandler(mock)
	_, err := h.GetLeaderboardHandler(context.Background(), &GetLeaderboardRequest{Dimension: "bogus"})
	testhelpers.AssertHumaError(t, err, 422)
}

func TestGetLeaderboard_ServiceError(t *testing.T) {
	mock := &testhelpers.MockLeaderboardService{
		GetLeaderboardFn: func(_, _ string, _ int) ([]contracts.LeaderboardEntry, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := NewLeaderboardHandler(mock)
	_, err := h.GetLeaderboardHandler(context.Background(), &GetLeaderboardRequest{})
	testhelpers.AssertHumaError(t, err, 500)
}
