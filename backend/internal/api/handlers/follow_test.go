package handlers

import (
	"context"
	"fmt"
	"testing"
	"time"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
)

func testFollowHandler() *FollowHandler {
	return NewFollowHandler(nil)
}

// --- NewFollowHandler ---

// --- FollowEntityHandler ---

func TestFollowEntityHandler_NoAuth(t *testing.T) {
	h := testFollowHandler()
	req := &FollowRequest{EntityType: "artists", EntityID: "1"}

	_, err := h.FollowEntityHandler(context.Background(), req)
	assertHumaError(t, err, 401)
}

func TestFollowEntityHandler_InvalidEntityType(t *testing.T) {
	h := NewFollowHandler(&mockFollowService{})
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &FollowRequest{EntityType: "shows", EntityID: "1"}

	_, err := h.FollowEntityHandler(ctx, req)
	assertHumaError(t, err, 400)
}

func TestFollowEntityHandler_InvalidID(t *testing.T) {
	h := NewFollowHandler(&mockFollowService{})
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &FollowRequest{EntityType: "artists", EntityID: "abc"}

	_, err := h.FollowEntityHandler(ctx, req)
	assertHumaError(t, err, 400)
}

func TestFollowEntityHandler_Success(t *testing.T) {
	mock := &mockFollowService{
		followFn: func(userID uint, entityType string, entityID uint) error {
			if userID != 1 || entityType != "artist" || entityID != 42 {
				t.Errorf("unexpected args: userID=%d, entityType=%s, entityID=%d", userID, entityType, entityID)
			}
			return nil
		},
	}
	h := NewFollowHandler(mock)
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &FollowRequest{EntityType: "artists", EntityID: "42"}

	resp, err := h.FollowEntityHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true")
	}
}

func TestFollowEntityHandler_ServiceError(t *testing.T) {
	mock := &mockFollowService{
		followFn: func(_ uint, _ string, _ uint) error {
			return fmt.Errorf("db error")
		},
	}
	h := NewFollowHandler(mock)
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &FollowRequest{EntityType: "artists", EntityID: "42"}

	_, err := h.FollowEntityHandler(ctx, req)
	assertHumaError(t, err, 422)
}

func TestFollowEntityHandler_AllEntityTypes(t *testing.T) {
	for _, entityType := range []string{"artists", "venues", "labels", "festivals"} {
		t.Run(entityType, func(t *testing.T) {
			var capturedType string
			mock := &mockFollowService{
				followFn: func(_ uint, et string, _ uint) error {
					capturedType = et
					return nil
				},
			}
			h := NewFollowHandler(mock)
			ctx := ctxWithUser(&models.User{ID: 1})
			req := &FollowRequest{EntityType: entityType, EntityID: "1"}

			_, err := h.FollowEntityHandler(ctx, req)
			if err != nil {
				t.Fatalf("unexpected error for %s: %v", entityType, err)
			}
			// Verify the plural was converted to singular
			expected := entityType[:len(entityType)-1] // strip trailing 's'
			if capturedType != expected {
				t.Errorf("expected entity type %s, got %s", expected, capturedType)
			}
		})
	}
}

// --- UnfollowEntityHandler ---

func TestUnfollowEntityHandler_NoAuth(t *testing.T) {
	h := testFollowHandler()
	req := &UnfollowRequest{EntityType: "artists", EntityID: "1"}

	_, err := h.UnfollowEntityHandler(context.Background(), req)
	assertHumaError(t, err, 401)
}

func TestUnfollowEntityHandler_InvalidEntityType(t *testing.T) {
	h := NewFollowHandler(&mockFollowService{})
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &UnfollowRequest{EntityType: "bananas", EntityID: "1"}

	_, err := h.UnfollowEntityHandler(ctx, req)
	assertHumaError(t, err, 400)
}

func TestUnfollowEntityHandler_InvalidID(t *testing.T) {
	h := NewFollowHandler(&mockFollowService{})
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &UnfollowRequest{EntityType: "artists", EntityID: "xyz"}

	_, err := h.UnfollowEntityHandler(ctx, req)
	assertHumaError(t, err, 400)
}

func TestUnfollowEntityHandler_Success(t *testing.T) {
	mock := &mockFollowService{
		unfollowFn: func(userID uint, entityType string, entityID uint) error {
			if userID != 1 || entityType != "venue" || entityID != 42 {
				t.Errorf("unexpected args: userID=%d, entityType=%s, entityID=%d", userID, entityType, entityID)
			}
			return nil
		},
	}
	h := NewFollowHandler(mock)
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &UnfollowRequest{EntityType: "venues", EntityID: "42"}

	resp, err := h.UnfollowEntityHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true")
	}
}

func TestUnfollowEntityHandler_ServiceError(t *testing.T) {
	mock := &mockFollowService{
		unfollowFn: func(_ uint, _ string, _ uint) error {
			return fmt.Errorf("db error")
		},
	}
	h := NewFollowHandler(mock)
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &UnfollowRequest{EntityType: "artists", EntityID: "42"}

	_, err := h.UnfollowEntityHandler(ctx, req)
	assertHumaError(t, err, 422)
}

// --- GetFollowersHandler ---

func TestGetFollowersHandler_InvalidEntityType(t *testing.T) {
	h := NewFollowHandler(&mockFollowService{})
	req := &GetFollowersRequest{EntityType: "shows", EntityID: "1"}

	_, err := h.GetFollowersHandler(context.Background(), req)
	assertHumaError(t, err, 400)
}

func TestGetFollowersHandler_InvalidID(t *testing.T) {
	h := NewFollowHandler(&mockFollowService{})
	req := &GetFollowersRequest{EntityType: "artists", EntityID: "abc"}

	_, err := h.GetFollowersHandler(context.Background(), req)
	assertHumaError(t, err, 400)
}

func TestGetFollowersHandler_Success_NoAuth(t *testing.T) {
	mock := &mockFollowService{
		getFollowerCountFn: func(entityType string, entityID uint) (int64, error) {
			return 42, nil
		},
	}
	h := NewFollowHandler(mock)
	req := &GetFollowersRequest{EntityType: "artists", EntityID: "5"}

	resp, err := h.GetFollowersHandler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.FollowerCount != 42 {
		t.Errorf("expected follower_count=42, got %d", resp.Body.FollowerCount)
	}
	if resp.Body.IsFollowing {
		t.Error("expected is_following=false for unauthenticated")
	}
	if resp.Body.EntityType != "artist" {
		t.Errorf("expected entity_type=artist, got %s", resp.Body.EntityType)
	}
}

func TestGetFollowersHandler_Success_WithAuth(t *testing.T) {
	mock := &mockFollowService{
		getFollowerCountFn: func(_ string, _ uint) (int64, error) {
			return 10, nil
		},
		isFollowingFn: func(userID uint, _ string, _ uint) (bool, error) {
			return true, nil
		},
	}
	h := NewFollowHandler(mock)
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &GetFollowersRequest{EntityType: "artists", EntityID: "5"}

	resp, err := h.GetFollowersHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.FollowerCount != 10 {
		t.Errorf("expected follower_count=10, got %d", resp.Body.FollowerCount)
	}
	if !resp.Body.IsFollowing {
		t.Error("expected is_following=true")
	}
}

func TestGetFollowersHandler_CountError(t *testing.T) {
	mock := &mockFollowService{
		getFollowerCountFn: func(_ string, _ uint) (int64, error) {
			return 0, fmt.Errorf("db error")
		},
	}
	h := NewFollowHandler(mock)
	req := &GetFollowersRequest{EntityType: "artists", EntityID: "5"}

	_, err := h.GetFollowersHandler(context.Background(), req)
	assertHumaError(t, err, 500)
}

func TestGetFollowersHandler_IsFollowingError_GracefulDegradation(t *testing.T) {
	mock := &mockFollowService{
		getFollowerCountFn: func(_ string, _ uint) (int64, error) {
			return 7, nil
		},
		isFollowingFn: func(_ uint, _ string, _ uint) (bool, error) {
			return false, fmt.Errorf("is_following query failed")
		},
	}
	h := NewFollowHandler(mock)
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &GetFollowersRequest{EntityType: "artists", EntityID: "5"}

	resp, err := h.GetFollowersHandler(ctx, req)
	if err != nil {
		t.Fatalf("expected no error (graceful degradation), got: %v", err)
	}
	if resp.Body.FollowerCount != 7 {
		t.Errorf("expected follower_count=7, got %d", resp.Body.FollowerCount)
	}
	if resp.Body.IsFollowing {
		t.Error("expected is_following=false on error")
	}
}

// --- BatchFollowHandler ---

func TestBatchFollowHandler_EmptyList(t *testing.T) {
	h := NewFollowHandler(&mockFollowService{})
	req := &BatchFollowRequest{}
	req.Body.EntityType = "artist"
	req.Body.EntityIDs = []int{}

	resp, err := h.BatchFollowHandler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Body.Follows) != 0 {
		t.Errorf("expected empty map, got %v", resp.Body.Follows)
	}
}

func TestBatchFollowHandler_TooMany(t *testing.T) {
	h := NewFollowHandler(&mockFollowService{})
	req := &BatchFollowRequest{}
	req.Body.EntityType = "artist"
	req.Body.EntityIDs = make([]int, 101)
	for i := range req.Body.EntityIDs {
		req.Body.EntityIDs[i] = i + 1
	}

	_, err := h.BatchFollowHandler(context.Background(), req)
	assertHumaError(t, err, 400)
}

func TestBatchFollowHandler_InvalidEntityType(t *testing.T) {
	h := NewFollowHandler(&mockFollowService{})
	req := &BatchFollowRequest{}
	req.Body.EntityType = "banana"
	req.Body.EntityIDs = []int{1}

	_, err := h.BatchFollowHandler(context.Background(), req)
	assertHumaError(t, err, 400)
}

func TestBatchFollowHandler_NegativeID(t *testing.T) {
	h := NewFollowHandler(&mockFollowService{})
	req := &BatchFollowRequest{}
	req.Body.EntityType = "artist"
	req.Body.EntityIDs = []int{1, -5, 3}

	_, err := h.BatchFollowHandler(context.Background(), req)
	assertHumaError(t, err, 400)
}

func TestBatchFollowHandler_Success_NoAuth(t *testing.T) {
	mock := &mockFollowService{
		getBatchFollowerCountsFn: func(entityType string, entityIDs []uint) (map[uint]int64, error) {
			result := make(map[uint]int64)
			for _, id := range entityIDs {
				result[id] = 5
			}
			return result, nil
		},
	}
	h := NewFollowHandler(mock)
	req := &BatchFollowRequest{}
	req.Body.EntityType = "artist"
	req.Body.EntityIDs = []int{1, 2}

	resp, err := h.BatchFollowHandler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Body.Follows) != 2 {
		t.Errorf("expected 2 entries, got %d", len(resp.Body.Follows))
	}
	entry := resp.Body.Follows["1"]
	if entry == nil {
		t.Fatal("expected entry for entity 1")
	}
	if entry.FollowerCount != 5 {
		t.Errorf("expected follower_count=5, got %d", entry.FollowerCount)
	}
	if entry.IsFollowing {
		t.Error("expected is_following=false for unauthenticated")
	}
}

func TestBatchFollowHandler_Success_WithAuth(t *testing.T) {
	mock := &mockFollowService{
		getBatchFollowerCountsFn: func(_ string, entityIDs []uint) (map[uint]int64, error) {
			result := make(map[uint]int64)
			for _, id := range entityIDs {
				result[id] = 0
			}
			return result, nil
		},
		getBatchUserFollowingFn: func(userID uint, _ string, _ []uint) (map[uint]bool, error) {
			return map[uint]bool{1: true, 3: true}, nil
		},
	}
	h := NewFollowHandler(mock)
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &BatchFollowRequest{}
	req.Body.EntityType = "artist"
	req.Body.EntityIDs = []int{1, 2, 3}

	resp, err := h.BatchFollowHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Follows["1"].IsFollowing {
		t.Error("expected is_following=true for entity 1")
	}
	if resp.Body.Follows["2"].IsFollowing {
		t.Error("expected is_following=false for entity 2")
	}
	if !resp.Body.Follows["3"].IsFollowing {
		t.Error("expected is_following=true for entity 3")
	}
}

func TestBatchFollowHandler_AcceptsPluralForm(t *testing.T) {
	var capturedType string
	mock := &mockFollowService{
		getBatchFollowerCountsFn: func(entityType string, entityIDs []uint) (map[uint]int64, error) {
			capturedType = entityType
			result := make(map[uint]int64)
			for _, id := range entityIDs {
				result[id] = 0
			}
			return result, nil
		},
	}
	h := NewFollowHandler(mock)
	req := &BatchFollowRequest{}
	req.Body.EntityType = "artists" // plural form
	req.Body.EntityIDs = []int{1}

	_, err := h.BatchFollowHandler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedType != "artist" {
		t.Errorf("expected entity type 'artist', got '%s'", capturedType)
	}
}

func TestBatchFollowHandler_UserFollowingError_GracefulDegradation(t *testing.T) {
	mock := &mockFollowService{
		getBatchFollowerCountsFn: func(_ string, entityIDs []uint) (map[uint]int64, error) {
			result := make(map[uint]int64)
			for _, id := range entityIDs {
				result[id] = 3
			}
			return result, nil
		},
		getBatchUserFollowingFn: func(_ uint, _ string, _ []uint) (map[uint]bool, error) {
			return nil, fmt.Errorf("user following query failed")
		},
	}
	h := NewFollowHandler(mock)
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &BatchFollowRequest{}
	req.Body.EntityType = "artist"
	req.Body.EntityIDs = []int{1, 2}

	resp, err := h.BatchFollowHandler(ctx, req)
	if err != nil {
		t.Fatalf("expected no error (graceful degradation), got: %v", err)
	}
	// Counts still returned despite user following error
	if resp.Body.Follows["1"].FollowerCount != 3 {
		t.Errorf("expected follower_count=3, got %d", resp.Body.Follows["1"].FollowerCount)
	}
	// IsFollowing defaults to false when error
	if resp.Body.Follows["1"].IsFollowing {
		t.Error("expected is_following=false on error")
	}
}

func TestBatchFollowHandler_ZeroEntityID(t *testing.T) {
	h := NewFollowHandler(&mockFollowService{})
	req := &BatchFollowRequest{}
	req.Body.EntityType = "artist"
	req.Body.EntityIDs = []int{1, 0, 3}

	_, err := h.BatchFollowHandler(context.Background(), req)
	assertHumaError(t, err, 400)
}

func TestBatchFollowHandler_CountsError(t *testing.T) {
	mock := &mockFollowService{
		getBatchFollowerCountsFn: func(_ string, _ []uint) (map[uint]int64, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := NewFollowHandler(mock)
	req := &BatchFollowRequest{}
	req.Body.EntityType = "artist"
	req.Body.EntityIDs = []int{1}

	_, err := h.BatchFollowHandler(context.Background(), req)
	assertHumaError(t, err, 500)
}

// --- GetMyFollowingHandler ---

func TestGetMyFollowingHandler_NoAuth(t *testing.T) {
	h := testFollowHandler()
	req := &GetMyFollowingRequest{}

	_, err := h.GetMyFollowingHandler(context.Background(), req)
	assertHumaError(t, err, 401)
}

func TestGetMyFollowingHandler_InvalidType(t *testing.T) {
	h := NewFollowHandler(&mockFollowService{})
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &GetMyFollowingRequest{Type: "banana"}

	_, err := h.GetMyFollowingHandler(ctx, req)
	assertHumaError(t, err, 400)
}

func TestGetMyFollowingHandler_Success(t *testing.T) {
	now := time.Now().UTC()
	following := []*contracts.FollowingEntityResponse{
		{
			EntityType: "artist",
			EntityID:   1,
			Name:       "Test Artist",
			Slug:       "test-artist",
			FollowedAt: now,
		},
	}
	mock := &mockFollowService{
		getUserFollowingFn: func(userID uint, entityType string, limit, offset int) ([]*contracts.FollowingEntityResponse, int64, error) {
			if userID != 1 {
				t.Errorf("unexpected userID=%d", userID)
			}
			if entityType != "" {
				t.Errorf("expected empty entityType for 'all', got %s", entityType)
			}
			return following, 1, nil
		},
	}
	h := NewFollowHandler(mock)
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &GetMyFollowingRequest{Type: "all", Limit: 20, Offset: 0}

	resp, err := h.GetMyFollowingHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Total != 1 {
		t.Errorf("expected total=1, got %d", resp.Body.Total)
	}
	if len(resp.Body.Following) != 1 {
		t.Errorf("expected 1 following, got %d", len(resp.Body.Following))
	}
}

func TestGetMyFollowingHandler_WithTypeFilter(t *testing.T) {
	var capturedType string
	mock := &mockFollowService{
		getUserFollowingFn: func(_ uint, entityType string, _, _ int) ([]*contracts.FollowingEntityResponse, int64, error) {
			capturedType = entityType
			return nil, 0, nil
		},
	}
	h := NewFollowHandler(mock)
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &GetMyFollowingRequest{Type: "venue", Limit: 20}

	_, err := h.GetMyFollowingHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedType != "venue" {
		t.Errorf("expected type=venue, got %s", capturedType)
	}
}

func TestGetMyFollowingHandler_ServiceError(t *testing.T) {
	mock := &mockFollowService{
		getUserFollowingFn: func(_ uint, _ string, _, _ int) ([]*contracts.FollowingEntityResponse, int64, error) {
			return nil, 0, fmt.Errorf("db error")
		},
	}
	h := NewFollowHandler(mock)
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &GetMyFollowingRequest{Type: "all", Limit: 20}

	_, err := h.GetMyFollowingHandler(ctx, req)
	assertHumaError(t, err, 500)
}

func TestGetMyFollowingHandler_PaginationClamping(t *testing.T) {
	var capturedLimit, capturedOffset int
	mock := &mockFollowService{
		getUserFollowingFn: func(_ uint, _ string, limit, offset int) ([]*contracts.FollowingEntityResponse, int64, error) {
			capturedLimit = limit
			capturedOffset = offset
			return nil, 0, nil
		},
	}
	h := NewFollowHandler(mock)
	ctx := ctxWithUser(&models.User{ID: 1})

	// limit=0 -> 20, offset=-1 -> 0
	_, err := h.GetMyFollowingHandler(ctx, &GetMyFollowingRequest{Type: "all", Limit: 0, Offset: -1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedLimit != 20 {
		t.Errorf("expected limit=20, got %d", capturedLimit)
	}
	if capturedOffset != 0 {
		t.Errorf("expected offset=0, got %d", capturedOffset)
	}

	// limit=999 -> 100
	h.GetMyFollowingHandler(ctx, &GetMyFollowingRequest{Type: "all", Limit: 999})
	if capturedLimit != 100 {
		t.Errorf("expected limit=100, got %d", capturedLimit)
	}
}

func TestGetMyFollowingHandler_AllValidTypeFilters(t *testing.T) {
	for _, typeFilter := range []string{"artist", "venue", "label", "festival"} {
		t.Run(typeFilter, func(t *testing.T) {
			var capturedType string
			mock := &mockFollowService{
				getUserFollowingFn: func(_ uint, entityType string, _, _ int) ([]*contracts.FollowingEntityResponse, int64, error) {
					capturedType = entityType
					return nil, 0, nil
				},
			}
			h := NewFollowHandler(mock)
			ctx := ctxWithUser(&models.User{ID: 1})
			req := &GetMyFollowingRequest{Type: typeFilter, Limit: 20}

			_, err := h.GetMyFollowingHandler(ctx, req)
			if err != nil {
				t.Fatalf("unexpected error for type %s: %v", typeFilter, err)
			}
			if capturedType != typeFilter {
				t.Errorf("expected type=%s, got %s", typeFilter, capturedType)
			}
		})
	}
}

func TestGetMyFollowingHandler_DefaultType(t *testing.T) {
	var capturedType string
	mock := &mockFollowService{
		getUserFollowingFn: func(_ uint, entityType string, _, _ int) ([]*contracts.FollowingEntityResponse, int64, error) {
			capturedType = entityType
			return nil, 0, nil
		},
	}
	h := NewFollowHandler(mock)
	ctx := ctxWithUser(&models.User{ID: 1})

	// Empty type defaults to "" (all)
	_, err := h.GetMyFollowingHandler(ctx, &GetMyFollowingRequest{Type: "", Limit: 20})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedType != "" {
		t.Errorf("expected empty type for default, got %s", capturedType)
	}
}

// --- GetFollowersListHandler ---

func TestGetFollowersListHandler_InvalidEntityType(t *testing.T) {
	h := NewFollowHandler(&mockFollowService{})
	req := &GetFollowersListRequest{EntityType: "shows", EntityID: "1", Limit: 20}

	_, err := h.GetFollowersListHandler(context.Background(), req)
	assertHumaError(t, err, 400)
}

func TestGetFollowersListHandler_InvalidID(t *testing.T) {
	h := NewFollowHandler(&mockFollowService{})
	req := &GetFollowersListRequest{EntityType: "artists", EntityID: "abc", Limit: 20}

	_, err := h.GetFollowersListHandler(context.Background(), req)
	assertHumaError(t, err, 400)
}

func TestGetFollowersListHandler_Success(t *testing.T) {
	followers := []*contracts.FollowerResponse{
		{UserID: 1, Username: "user1", DisplayName: "User One"},
		{UserID: 2, Username: "user2"},
	}
	mock := &mockFollowService{
		getFollowersFn: func(entityType string, entityID uint, limit, offset int) ([]*contracts.FollowerResponse, int64, error) {
			if entityType != "artist" || entityID != 5 {
				t.Errorf("unexpected args: entityType=%s, entityID=%d", entityType, entityID)
			}
			return followers, 2, nil
		},
	}
	h := NewFollowHandler(mock)
	req := &GetFollowersListRequest{EntityType: "artists", EntityID: "5", Limit: 20}

	resp, err := h.GetFollowersListHandler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Total != 2 {
		t.Errorf("expected total=2, got %d", resp.Body.Total)
	}
	if len(resp.Body.Followers) != 2 {
		t.Errorf("expected 2 followers, got %d", len(resp.Body.Followers))
	}
}

func TestGetFollowersListHandler_ServiceError(t *testing.T) {
	mock := &mockFollowService{
		getFollowersFn: func(_ string, _ uint, _, _ int) ([]*contracts.FollowerResponse, int64, error) {
			return nil, 0, fmt.Errorf("db error")
		},
	}
	h := NewFollowHandler(mock)
	req := &GetFollowersListRequest{EntityType: "artists", EntityID: "5", Limit: 20}

	_, err := h.GetFollowersListHandler(context.Background(), req)
	assertHumaError(t, err, 500)
}

func TestGetFollowersListHandler_PaginationClamping(t *testing.T) {
	var capturedLimit, capturedOffset int
	mock := &mockFollowService{
		getFollowersFn: func(_ string, _ uint, limit, offset int) ([]*contracts.FollowerResponse, int64, error) {
			capturedLimit = limit
			capturedOffset = offset
			return nil, 0, nil
		},
	}
	h := NewFollowHandler(mock)

	// limit=0 -> 20, offset=-1 -> 0
	_, err := h.GetFollowersListHandler(context.Background(), &GetFollowersListRequest{
		EntityType: "artists", EntityID: "1", Limit: 0, Offset: -1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedLimit != 20 {
		t.Errorf("expected limit=20, got %d", capturedLimit)
	}
	if capturedOffset != 0 {
		t.Errorf("expected offset=0, got %d", capturedOffset)
	}

	// limit=999 -> 100
	h.GetFollowersListHandler(context.Background(), &GetFollowersListRequest{
		EntityType: "artists", EntityID: "1", Limit: 999,
	})
	if capturedLimit != 100 {
		t.Errorf("expected limit=100, got %d", capturedLimit)
	}
}
