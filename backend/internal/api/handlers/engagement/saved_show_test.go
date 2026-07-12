package engagement

import (
	"context"
	"fmt"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

func testSavedShowHandler() *SavedShowHandler {
	return NewSavedShowHandler(nil)
}

// --- SaveShowHandler ---

func TestSaveShowHandler_NoAuth(t *testing.T) {
	h := testSavedShowHandler()
	req := &SaveShowRequest{ShowID: "1"}

	_, err := h.SaveShowHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 401)
}

func TestSaveShowHandler_InvalidID(t *testing.T) {
	h := testSavedShowHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &SaveShowRequest{ShowID: "abc"}

	_, err := h.SaveShowHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestSaveShowHandler_Success(t *testing.T) {
	mock := &testhelpers.MockSavedShowService{
		SaveShowFn: func(userID, showID uint) error {
			if userID != 1 || showID != 42 {
				t.Errorf("unexpected args: userID=%d, showID=%d", userID, showID)
			}
			return nil
		},
	}
	h := NewSavedShowHandler(mock)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	resp, err := h.SaveShowHandler(ctx, &SaveShowRequest{ShowID: "42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true")
	}
}

func TestSaveShowHandler_ServiceError(t *testing.T) {
	mock := &testhelpers.MockSavedShowService{
		SaveShowFn: func(_, _ uint) error {
			return fmt.Errorf("already saved")
		},
	}
	h := NewSavedShowHandler(mock)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	_, err := h.SaveShowHandler(ctx, &SaveShowRequest{ShowID: "42"})
	testhelpers.AssertHumaError(t, err, 422)
}

// --- UnsaveShowHandler ---

func TestUnsaveShowHandler_NoAuth(t *testing.T) {
	h := testSavedShowHandler()
	req := &UnsaveShowRequest{ShowID: "1"}

	_, err := h.UnsaveShowHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 401)
}

func TestUnsaveShowHandler_InvalidID(t *testing.T) {
	h := testSavedShowHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &UnsaveShowRequest{ShowID: "abc"}

	_, err := h.UnsaveShowHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestUnsaveShowHandler_Success(t *testing.T) {
	mock := &testhelpers.MockSavedShowService{
		UnsaveShowFn: func(userID, showID uint) error {
			if userID != 1 || showID != 42 {
				t.Errorf("unexpected args: userID=%d, showID=%d", userID, showID)
			}
			return nil
		},
	}
	h := NewSavedShowHandler(mock)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	resp, err := h.UnsaveShowHandler(ctx, &UnsaveShowRequest{ShowID: "42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true")
	}
}

func TestUnsaveShowHandler_ServiceError(t *testing.T) {
	mock := &testhelpers.MockSavedShowService{
		UnsaveShowFn: func(_, _ uint) error {
			return fmt.Errorf("not saved")
		},
	}
	h := NewSavedShowHandler(mock)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	_, err := h.UnsaveShowHandler(ctx, &UnsaveShowRequest{ShowID: "42"})
	testhelpers.AssertHumaError(t, err, 422)
}

// --- GetSavedShowsHandler ---

func TestGetSavedShowsHandler_NoAuth(t *testing.T) {
	h := testSavedShowHandler()
	req := &GetSavedShowsRequest{}

	_, err := h.GetSavedShowsHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 401)
}

func TestGetSavedShowsHandler_Success(t *testing.T) {
	shows := []*contracts.SavedShowResponse{{}}
	mock := &testhelpers.MockSavedShowService{
		GetUserSavedShowsFn: func(userID uint, limit, offset int, timeFilter string) ([]*contracts.SavedShowResponse, int64, error) {
			if userID != 1 {
				t.Errorf("unexpected userID=%d", userID)
			}
			if timeFilter != "" {
				t.Errorf("expected empty timeFilter when not requested, got %q", timeFilter)
			}
			return shows, 1, nil
		},
	}
	h := NewSavedShowHandler(mock)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	resp, err := h.GetSavedShowsHandler(ctx, &GetSavedShowsRequest{Limit: 10, Offset: 0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Total != 1 {
		t.Errorf("expected total=1, got %d", resp.Body.Total)
	}
	if len(resp.Body.Shows) != 1 {
		t.Errorf("expected 1 show, got %d", len(resp.Body.Shows))
	}
}

func TestGetSavedShowsHandler_TimeFilterPassthrough(t *testing.T) {
	var capturedFilter string
	mock := &testhelpers.MockSavedShowService{
		GetUserSavedShowsFn: func(_ uint, _, _ int, timeFilter string) ([]*contracts.SavedShowResponse, int64, error) {
			capturedFilter = timeFilter
			return nil, 0, nil
		},
	}
	h := NewSavedShowHandler(mock)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	for _, filter := range []string{"upcoming", "past"} {
		if _, err := h.GetSavedShowsHandler(ctx, &GetSavedShowsRequest{Limit: 10, TimeFilter: filter}); err != nil {
			t.Fatalf("unexpected error for filter %q: %v", filter, err)
		}
		if capturedFilter != filter {
			t.Errorf("expected time filter %q passed to service, got %q", filter, capturedFilter)
		}
	}
}

func TestGetSavedShowsHandler_ServiceError(t *testing.T) {
	mock := &testhelpers.MockSavedShowService{
		GetUserSavedShowsFn: func(_ uint, _, _ int, _ string) ([]*contracts.SavedShowResponse, int64, error) {
			return nil, 0, fmt.Errorf("db error")
		},
	}
	h := NewSavedShowHandler(mock)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	_, err := h.GetSavedShowsHandler(ctx, &GetSavedShowsRequest{Limit: 10})
	testhelpers.AssertHumaError(t, err, 500)
}

func TestGetSavedShowsHandler_PaginationClamping(t *testing.T) {
	var capturedLimit, capturedOffset int
	mock := &testhelpers.MockSavedShowService{
		GetUserSavedShowsFn: func(_ uint, limit, offset int, _ string) ([]*contracts.SavedShowResponse, int64, error) {
			capturedLimit = limit
			capturedOffset = offset
			return nil, 0, nil
		},
	}
	h := NewSavedShowHandler(mock)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	// limit=0 should be clamped to 50, offset=-1 should be clamped to 0
	resp, err := h.GetSavedShowsHandler(ctx, &GetSavedShowsRequest{Limit: 0, Offset: -1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedLimit != 50 {
		t.Errorf("expected limit clamped to 50, got %d", capturedLimit)
	}
	if capturedOffset != 0 {
		t.Errorf("expected offset clamped to 0, got %d", capturedOffset)
	}
	if resp.Body.Limit != 50 {
		t.Errorf("expected response limit=50, got %d", resp.Body.Limit)
	}

	// limit=999 should be clamped to 200
	if _, err := h.GetSavedShowsHandler(ctx, &GetSavedShowsRequest{Limit: 999}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedLimit != 200 {
		t.Errorf("expected limit clamped to 200, got %d", capturedLimit)
	}
}

// --- CheckSavedHandler ---

func TestCheckSavedHandler_NoAuth(t *testing.T) {
	h := testSavedShowHandler()
	req := &CheckSavedRequest{ShowID: "1"}

	_, err := h.CheckSavedHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 401)
}

func TestCheckSavedHandler_InvalidID(t *testing.T) {
	h := testSavedShowHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &CheckSavedRequest{ShowID: "abc"}

	_, err := h.CheckSavedHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestCheckSavedHandler_Saved(t *testing.T) {
	mock := &testhelpers.MockSavedShowService{
		IsShowSavedFn: func(userID, showID uint) (bool, error) {
			return true, nil
		},
	}
	h := NewSavedShowHandler(mock)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	resp, err := h.CheckSavedHandler(ctx, &CheckSavedRequest{ShowID: "42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.IsSaved {
		t.Error("expected is_saved=true")
	}
}

func TestCheckSavedHandler_NotSaved(t *testing.T) {
	mock := &testhelpers.MockSavedShowService{
		IsShowSavedFn: func(_, _ uint) (bool, error) {
			return false, nil
		},
	}
	h := NewSavedShowHandler(mock)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	resp, err := h.CheckSavedHandler(ctx, &CheckSavedRequest{ShowID: "42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.IsSaved {
		t.Error("expected is_saved=false")
	}
}

func TestCheckSavedHandler_ServiceError(t *testing.T) {
	mock := &testhelpers.MockSavedShowService{
		IsShowSavedFn: func(_, _ uint) (bool, error) {
			return false, fmt.Errorf("db error")
		},
	}
	h := NewSavedShowHandler(mock)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	_, err := h.CheckSavedHandler(ctx, &CheckSavedRequest{ShowID: "42"})
	testhelpers.AssertHumaError(t, err, 500)
}

// --- GetSaveCountHandler (public, optional auth) ---
//
// The save COUNT is public; is_saved is per-caller. These tests pin that split:
// an anonymous caller must receive the count and is_saved=false, never another
// user's state.

func TestGetSaveCountHandler_Anonymous_ReturnsCountWithoutIsSaved(t *testing.T) {
	mock := &testhelpers.MockSavedShowService{
		GetSaveCountFn: func(showID uint) (int, error) { return 7, nil },
		IsShowSavedFn: func(_, _ uint) (bool, error) {
			t.Fatal("IsShowSaved must not be called for an anonymous request")
			return false, nil
		},
	}
	h := NewSavedShowHandler(mock)

	resp, err := h.GetSaveCountHandler(context.Background(), &GetSaveCountRequest{ShowID: "42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.ShowID != 42 {
		t.Errorf("expected show_id 42, got %d", resp.Body.ShowID)
	}
	if resp.Body.SaveCount != 7 {
		t.Errorf("expected save_count 7, got %d", resp.Body.SaveCount)
	}
	if resp.Body.IsSaved {
		t.Error("anonymous caller must never receive is_saved=true")
	}
}

func TestGetSaveCountHandler_Authenticated_IncludesOwnIsSaved(t *testing.T) {
	var sawUserID uint
	mock := &testhelpers.MockSavedShowService{
		GetSaveCountFn: func(_ uint) (int, error) { return 3, nil },
		IsShowSavedFn: func(userID, _ uint) (bool, error) {
			sawUserID = userID
			return true, nil
		},
	}
	h := NewSavedShowHandler(mock)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 9})

	resp, err := h.GetSaveCountHandler(ctx, &GetSaveCountRequest{ShowID: "42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.SaveCount != 3 {
		t.Errorf("expected save_count 3, got %d", resp.Body.SaveCount)
	}
	if !resp.Body.IsSaved {
		t.Error("expected is_saved=true for the authenticated saver")
	}
	// is_saved must be scoped to the caller from context, never a request param.
	if sawUserID != 9 {
		t.Errorf("expected IsShowSaved scoped to user 9, got %d", sawUserID)
	}
}

func TestGetSaveCountHandler_InvalidShowID(t *testing.T) {
	h := testSavedShowHandler()

	_, err := h.GetSaveCountHandler(context.Background(), &GetSaveCountRequest{ShowID: "abc"})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestGetSaveCountHandler_ServiceError(t *testing.T) {
	mock := &testhelpers.MockSavedShowService{
		GetSaveCountFn: func(_ uint) (int, error) { return 0, fmt.Errorf("db error") },
	}
	h := NewSavedShowHandler(mock)

	_, err := h.GetSaveCountHandler(context.Background(), &GetSaveCountRequest{ShowID: "42"})
	testhelpers.AssertHumaError(t, err, 500)
}

func TestGetSaveCountHandler_IsSavedFailureStillServesPublicCount(t *testing.T) {
	mock := &testhelpers.MockSavedShowService{
		GetSaveCountFn: func(_ uint) (int, error) { return 5, nil },
		IsShowSavedFn:  func(_, _ uint) (bool, error) { return false, fmt.Errorf("db error") },
	}
	h := NewSavedShowHandler(mock)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	resp, err := h.GetSaveCountHandler(ctx, &GetSaveCountRequest{ShowID: "42"})
	if err != nil {
		t.Fatalf("is_saved failure must be non-fatal, got %v", err)
	}
	if resp.Body.SaveCount != 5 {
		t.Errorf("expected the public count to survive, got %d", resp.Body.SaveCount)
	}
	if resp.Body.IsSaved {
		t.Error("is_saved must stay false when the lookup failed")
	}
}

// --- BatchSaveCountsHandler (public, optional auth) ---

func TestBatchSaveCountsHandler_EmptyList(t *testing.T) {
	h := NewSavedShowHandler(&testhelpers.MockSavedShowService{})
	req := &BatchSaveCountsRequest{}
	req.Body.ShowIDs = []int{}

	resp, err := h.BatchSaveCountsHandler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Body.Saves) != 0 {
		t.Errorf("expected empty map, got %v", resp.Body.Saves)
	}
}

func TestBatchSaveCountsHandler_Anonymous_ZeroFillsAndHidesIsSaved(t *testing.T) {
	mock := &testhelpers.MockSavedShowService{
		GetBatchSaveCountsFn: func(showIDs []uint) (map[uint]int, error) {
			return map[uint]int{1: 4, 2: 0}, nil
		},
		GetSavedShowIDsFn: func(_ uint, _ []uint) (map[uint]bool, error) {
			t.Fatal("GetSavedShowIDs must not be called for an anonymous request")
			return nil, nil
		},
	}
	h := NewSavedShowHandler(mock)
	req := &BatchSaveCountsRequest{}
	req.Body.ShowIDs = []int{1, 2}

	resp, err := h.BatchSaveCountsHandler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Saves["1"].SaveCount != 4 {
		t.Errorf("expected show 1 save_count 4, got %d", resp.Body.Saves["1"].SaveCount)
	}
	// A show nobody saved is present with a zero count, not omitted.
	entry, ok := resp.Body.Saves["2"]
	if !ok {
		t.Fatal("expected show 2 to be present with a zero count")
	}
	if entry.SaveCount != 0 || entry.IsSaved {
		t.Errorf("expected zero-filled, unsaved entry, got %+v", entry)
	}
	if resp.Body.Saves["1"].IsSaved {
		t.Error("anonymous caller must never receive is_saved=true")
	}
}

func TestBatchSaveCountsHandler_Authenticated_MergesOwnIsSaved(t *testing.T) {
	mock := &testhelpers.MockSavedShowService{
		GetBatchSaveCountsFn: func(_ []uint) (map[uint]int, error) {
			return map[uint]int{1: 4, 2: 2}, nil
		},
		GetSavedShowIDsFn: func(userID uint, _ []uint) (map[uint]bool, error) {
			return map[uint]bool{1: true, 2: false}, nil
		},
	}
	h := NewSavedShowHandler(mock)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &BatchSaveCountsRequest{}
	req.Body.ShowIDs = []int{1, 2}

	resp, err := h.BatchSaveCountsHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Saves["1"].IsSaved {
		t.Error("expected show 1 is_saved=true")
	}
	if resp.Body.Saves["2"].IsSaved {
		t.Error("expected show 2 is_saved=false")
	}
	if resp.Body.Saves["2"].SaveCount != 2 {
		t.Errorf("expected show 2 save_count 2, got %d", resp.Body.Saves["2"].SaveCount)
	}
}

func TestBatchSaveCountsHandler_NegativeID(t *testing.T) {
	h := NewSavedShowHandler(&testhelpers.MockSavedShowService{})
	req := &BatchSaveCountsRequest{}
	req.Body.ShowIDs = []int{1, -5, 3}

	_, err := h.BatchSaveCountsHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestBatchSaveCountsHandler_OverCap(t *testing.T) {
	h := NewSavedShowHandler(&testhelpers.MockSavedShowService{})
	req := &BatchSaveCountsRequest{}
	req.Body.ShowIDs = make([]int, 201)
	for i := range req.Body.ShowIDs {
		req.Body.ShowIDs[i] = i + 1
	}

	_, err := h.BatchSaveCountsHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestBatchSaveCountsHandler_ServiceError(t *testing.T) {
	mock := &testhelpers.MockSavedShowService{
		GetBatchSaveCountsFn: func(_ []uint) (map[uint]int, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := NewSavedShowHandler(mock)
	req := &BatchSaveCountsRequest{}
	req.Body.ShowIDs = []int{1, 2}

	_, err := h.BatchSaveCountsHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 500)
}

func TestBatchSaveCountsHandler_IsSavedFailureStillServesPublicCounts(t *testing.T) {
	mock := &testhelpers.MockSavedShowService{
		GetBatchSaveCountsFn: func(_ []uint) (map[uint]int, error) {
			return map[uint]int{1: 4}, nil
		},
		GetSavedShowIDsFn: func(_ uint, _ []uint) (map[uint]bool, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := NewSavedShowHandler(mock)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &BatchSaveCountsRequest{}
	req.Body.ShowIDs = []int{1}

	resp, err := h.BatchSaveCountsHandler(ctx, req)
	if err != nil {
		t.Fatalf("is_saved failure must be non-fatal, got %v", err)
	}
	if resp.Body.Saves["1"].SaveCount != 4 {
		t.Errorf("expected the public count to survive, got %d", resp.Body.Saves["1"].SaveCount)
	}
	if resp.Body.Saves["1"].IsSaved {
		t.Error("is_saved must stay false when the lookup failed")
	}
}
