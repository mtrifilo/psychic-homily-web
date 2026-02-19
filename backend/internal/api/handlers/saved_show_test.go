package handlers

import (
	"context"
	"fmt"
	"testing"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services"
)

func testSavedShowHandler() *SavedShowHandler {
	return NewSavedShowHandler(nil)
}

// --- NewSavedShowHandler ---

func TestNewSavedShowHandler(t *testing.T) {
	h := testSavedShowHandler()
	if h == nil {
		t.Fatal("expected non-nil SavedShowHandler")
	}
}

// --- SaveShowHandler ---

func TestSaveShowHandler_NoAuth(t *testing.T) {
	h := testSavedShowHandler()
	req := &SaveShowRequest{ShowID: "1"}

	_, err := h.SaveShowHandler(context.Background(), req)
	assertHumaError(t, err, 401)
}

func TestSaveShowHandler_InvalidID(t *testing.T) {
	h := testSavedShowHandler()
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &SaveShowRequest{ShowID: "abc"}

	_, err := h.SaveShowHandler(ctx, req)
	assertHumaError(t, err, 400)
}

func TestSaveShowHandler_Success(t *testing.T) {
	mock := &mockSavedShowService{
		saveShowFn: func(userID, showID uint) error {
			if userID != 1 || showID != 42 {
				t.Errorf("unexpected args: userID=%d, showID=%d", userID, showID)
			}
			return nil
		},
	}
	h := NewSavedShowHandler(mock)
	ctx := ctxWithUser(&models.User{ID: 1})

	resp, err := h.SaveShowHandler(ctx, &SaveShowRequest{ShowID: "42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true")
	}
}

func TestSaveShowHandler_ServiceError(t *testing.T) {
	mock := &mockSavedShowService{
		saveShowFn: func(_, _ uint) error {
			return fmt.Errorf("already saved")
		},
	}
	h := NewSavedShowHandler(mock)
	ctx := ctxWithUser(&models.User{ID: 1})

	_, err := h.SaveShowHandler(ctx, &SaveShowRequest{ShowID: "42"})
	assertHumaError(t, err, 422)
}

// --- UnsaveShowHandler ---

func TestUnsaveShowHandler_NoAuth(t *testing.T) {
	h := testSavedShowHandler()
	req := &UnsaveShowRequest{ShowID: "1"}

	_, err := h.UnsaveShowHandler(context.Background(), req)
	assertHumaError(t, err, 401)
}

func TestUnsaveShowHandler_InvalidID(t *testing.T) {
	h := testSavedShowHandler()
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &UnsaveShowRequest{ShowID: "abc"}

	_, err := h.UnsaveShowHandler(ctx, req)
	assertHumaError(t, err, 400)
}

func TestUnsaveShowHandler_Success(t *testing.T) {
	mock := &mockSavedShowService{
		unsaveShowFn: func(userID, showID uint) error {
			if userID != 1 || showID != 42 {
				t.Errorf("unexpected args: userID=%d, showID=%d", userID, showID)
			}
			return nil
		},
	}
	h := NewSavedShowHandler(mock)
	ctx := ctxWithUser(&models.User{ID: 1})

	resp, err := h.UnsaveShowHandler(ctx, &UnsaveShowRequest{ShowID: "42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true")
	}
}

func TestUnsaveShowHandler_ServiceError(t *testing.T) {
	mock := &mockSavedShowService{
		unsaveShowFn: func(_, _ uint) error {
			return fmt.Errorf("not saved")
		},
	}
	h := NewSavedShowHandler(mock)
	ctx := ctxWithUser(&models.User{ID: 1})

	_, err := h.UnsaveShowHandler(ctx, &UnsaveShowRequest{ShowID: "42"})
	assertHumaError(t, err, 422)
}

// --- GetSavedShowsHandler ---

func TestGetSavedShowsHandler_NoAuth(t *testing.T) {
	h := testSavedShowHandler()
	req := &GetSavedShowsRequest{}

	_, err := h.GetSavedShowsHandler(context.Background(), req)
	assertHumaError(t, err, 401)
}

func TestGetSavedShowsHandler_Success(t *testing.T) {
	shows := []*services.SavedShowResponse{{}}
	mock := &mockSavedShowService{
		getUserSavedFn: func(userID uint, limit, offset int) ([]*services.SavedShowResponse, int64, error) {
			if userID != 1 {
				t.Errorf("unexpected userID=%d", userID)
			}
			return shows, 1, nil
		},
	}
	h := NewSavedShowHandler(mock)
	ctx := ctxWithUser(&models.User{ID: 1})

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

func TestGetSavedShowsHandler_ServiceError(t *testing.T) {
	mock := &mockSavedShowService{
		getUserSavedFn: func(_ uint, _, _ int) ([]*services.SavedShowResponse, int64, error) {
			return nil, 0, fmt.Errorf("db error")
		},
	}
	h := NewSavedShowHandler(mock)
	ctx := ctxWithUser(&models.User{ID: 1})

	_, err := h.GetSavedShowsHandler(ctx, &GetSavedShowsRequest{Limit: 10})
	assertHumaError(t, err, 500)
}

func TestGetSavedShowsHandler_PaginationClamping(t *testing.T) {
	var capturedLimit, capturedOffset int
	mock := &mockSavedShowService{
		getUserSavedFn: func(_ uint, limit, offset int) ([]*services.SavedShowResponse, int64, error) {
			capturedLimit = limit
			capturedOffset = offset
			return nil, 0, nil
		},
	}
	h := NewSavedShowHandler(mock)
	ctx := ctxWithUser(&models.User{ID: 1})

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
	h.GetSavedShowsHandler(ctx, &GetSavedShowsRequest{Limit: 999})
	if capturedLimit != 200 {
		t.Errorf("expected limit clamped to 200, got %d", capturedLimit)
	}
}

// --- CheckSavedHandler ---

func TestCheckSavedHandler_NoAuth(t *testing.T) {
	h := testSavedShowHandler()
	req := &CheckSavedRequest{ShowID: "1"}

	_, err := h.CheckSavedHandler(context.Background(), req)
	assertHumaError(t, err, 401)
}

func TestCheckSavedHandler_InvalidID(t *testing.T) {
	h := testSavedShowHandler()
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &CheckSavedRequest{ShowID: "abc"}

	_, err := h.CheckSavedHandler(ctx, req)
	assertHumaError(t, err, 400)
}

func TestCheckSavedHandler_Saved(t *testing.T) {
	mock := &mockSavedShowService{
		isShowSavedFn: func(userID, showID uint) (bool, error) {
			return true, nil
		},
	}
	h := NewSavedShowHandler(mock)
	ctx := ctxWithUser(&models.User{ID: 1})

	resp, err := h.CheckSavedHandler(ctx, &CheckSavedRequest{ShowID: "42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.IsSaved {
		t.Error("expected is_saved=true")
	}
}

func TestCheckSavedHandler_NotSaved(t *testing.T) {
	mock := &mockSavedShowService{
		isShowSavedFn: func(_, _ uint) (bool, error) {
			return false, nil
		},
	}
	h := NewSavedShowHandler(mock)
	ctx := ctxWithUser(&models.User{ID: 1})

	resp, err := h.CheckSavedHandler(ctx, &CheckSavedRequest{ShowID: "42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.IsSaved {
		t.Error("expected is_saved=false")
	}
}

func TestCheckSavedHandler_ServiceError(t *testing.T) {
	mock := &mockSavedShowService{
		isShowSavedFn: func(_, _ uint) (bool, error) {
			return false, fmt.Errorf("db error")
		},
	}
	h := NewSavedShowHandler(mock)
	ctx := ctxWithUser(&models.User{ID: 1})

	_, err := h.CheckSavedHandler(ctx, &CheckSavedRequest{ShowID: "42"})
	assertHumaError(t, err, 500)
}

// --- CheckBatchSavedHandler ---

func TestCheckBatchSavedHandler_NoAuth(t *testing.T) {
	h := testSavedShowHandler()
	req := &CheckBatchSavedRequest{}
	req.Body.ShowIDs = []int{1, 2}

	_, err := h.CheckBatchSavedHandler(context.Background(), req)
	assertHumaError(t, err, 401)
}

func TestCheckBatchSavedHandler_EmptyList(t *testing.T) {
	h := NewSavedShowHandler(&mockSavedShowService{})
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &CheckBatchSavedRequest{}
	req.Body.ShowIDs = []int{}

	resp, err := h.CheckBatchSavedHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Body.SavedShowIDs) != 0 {
		t.Errorf("expected empty list, got %v", resp.Body.SavedShowIDs)
	}
}

func TestCheckBatchSavedHandler_Success(t *testing.T) {
	mock := &mockSavedShowService{
		getSavedShowIDFn: func(userID uint, showIDs []uint) (map[uint]bool, error) {
			return map[uint]bool{1: true, 3: true}, nil
		},
	}
	h := NewSavedShowHandler(mock)
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &CheckBatchSavedRequest{}
	req.Body.ShowIDs = []int{1, 2, 3}

	resp, err := h.CheckBatchSavedHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Body.SavedShowIDs) != 2 {
		t.Errorf("expected 2 saved IDs, got %d", len(resp.Body.SavedShowIDs))
	}
}

func TestCheckBatchSavedHandler_NegativeID(t *testing.T) {
	mock := &mockSavedShowService{}
	h := NewSavedShowHandler(mock)
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &CheckBatchSavedRequest{}
	req.Body.ShowIDs = []int{1, -5, 3}

	_, err := h.CheckBatchSavedHandler(ctx, req)
	assertHumaError(t, err, 400)
}

func TestCheckBatchSavedHandler_ServiceError(t *testing.T) {
	mock := &mockSavedShowService{
		getSavedShowIDFn: func(_ uint, _ []uint) (map[uint]bool, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := NewSavedShowHandler(mock)
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &CheckBatchSavedRequest{}
	req.Body.ShowIDs = []int{1, 2}

	_, err := h.CheckBatchSavedHandler(ctx, req)
	assertHumaError(t, err, 500)
}
