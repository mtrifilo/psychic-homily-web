package engagement

import (
	"context"
	"fmt"
	"testing"
	"time"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
)

func testAttendanceHandler() *AttendanceHandler {
	return NewAttendanceHandler(nil)
}

// --- SetAttendanceHandler ---

func TestSetAttendanceHandler_NoAuth(t *testing.T) {
	h := testAttendanceHandler()
	req := &SetAttendanceRequest{ShowID: "1"}
	req.Body.Status = "going"

	_, err := h.SetAttendanceHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 401)
}

func TestSetAttendanceHandler_InvalidID(t *testing.T) {
	h := testAttendanceHandler()
	ctx := testhelpers.CtxWithUser(&models.User{ID: 1})
	req := &SetAttendanceRequest{ShowID: "abc"}
	req.Body.Status = "going"

	_, err := h.SetAttendanceHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestSetAttendanceHandler_InvalidStatus(t *testing.T) {
	h := NewAttendanceHandler(&testhelpers.MockAttendanceService{})
	ctx := testhelpers.CtxWithUser(&models.User{ID: 1})
	req := &SetAttendanceRequest{ShowID: "42"}
	req.Body.Status = "maybe"

	_, err := h.SetAttendanceHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestSetAttendanceHandler_Going_Success(t *testing.T) {
	mock := &testhelpers.MockAttendanceService{
		SetAttendanceFn: func(userID, showID uint, status string) error {
			if userID != 1 || showID != 42 || status != "going" {
				t.Errorf("unexpected args: userID=%d, showID=%d, status=%s", userID, showID, status)
			}
			return nil
		},
	}
	h := NewAttendanceHandler(mock)
	ctx := testhelpers.CtxWithUser(&models.User{ID: 1})
	req := &SetAttendanceRequest{ShowID: "42"}
	req.Body.Status = "going"

	resp, err := h.SetAttendanceHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true")
	}
}

func TestSetAttendanceHandler_Interested_Success(t *testing.T) {
	mock := &testhelpers.MockAttendanceService{
		SetAttendanceFn: func(_, _ uint, status string) error {
			if status != "interested" {
				t.Errorf("expected status=interested, got %s", status)
			}
			return nil
		},
	}
	h := NewAttendanceHandler(mock)
	ctx := testhelpers.CtxWithUser(&models.User{ID: 1})
	req := &SetAttendanceRequest{ShowID: "42"}
	req.Body.Status = "interested"

	resp, err := h.SetAttendanceHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true")
	}
}

func TestSetAttendanceHandler_Clear_Success(t *testing.T) {
	mock := &testhelpers.MockAttendanceService{
		SetAttendanceFn: func(_, _ uint, status string) error {
			if status != "" {
				t.Errorf("expected empty status, got %s", status)
			}
			return nil
		},
	}
	h := NewAttendanceHandler(mock)
	ctx := testhelpers.CtxWithUser(&models.User{ID: 1})
	req := &SetAttendanceRequest{ShowID: "42"}
	req.Body.Status = ""

	resp, err := h.SetAttendanceHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true")
	}
}

func TestSetAttendanceHandler_ShowNotFound(t *testing.T) {
	mock := &testhelpers.MockAttendanceService{
		SetAttendanceFn: func(_, _ uint, _ string) error {
			return fmt.Errorf("show not found")
		},
	}
	h := NewAttendanceHandler(mock)
	ctx := testhelpers.CtxWithUser(&models.User{ID: 1})
	req := &SetAttendanceRequest{ShowID: "999"}
	req.Body.Status = "going"

	_, err := h.SetAttendanceHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 404)
}

func TestSetAttendanceHandler_ServiceError(t *testing.T) {
	mock := &testhelpers.MockAttendanceService{
		SetAttendanceFn: func(_, _ uint, _ string) error {
			return fmt.Errorf("db error")
		},
	}
	h := NewAttendanceHandler(mock)
	ctx := testhelpers.CtxWithUser(&models.User{ID: 1})
	req := &SetAttendanceRequest{ShowID: "42"}
	req.Body.Status = "going"

	_, err := h.SetAttendanceHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 422)
}

// --- RemoveAttendanceHandler ---

func TestRemoveAttendanceHandler_NoAuth(t *testing.T) {
	h := testAttendanceHandler()
	req := &RemoveAttendanceRequest{ShowID: "1"}

	_, err := h.RemoveAttendanceHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 401)
}

func TestRemoveAttendanceHandler_InvalidID(t *testing.T) {
	h := testAttendanceHandler()
	ctx := testhelpers.CtxWithUser(&models.User{ID: 1})
	req := &RemoveAttendanceRequest{ShowID: "abc"}

	_, err := h.RemoveAttendanceHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestRemoveAttendanceHandler_Success(t *testing.T) {
	mock := &testhelpers.MockAttendanceService{
		RemoveAttendanceFn: func(userID, showID uint) error {
			if userID != 1 || showID != 42 {
				t.Errorf("unexpected args: userID=%d, showID=%d", userID, showID)
			}
			return nil
		},
	}
	h := NewAttendanceHandler(mock)
	ctx := testhelpers.CtxWithUser(&models.User{ID: 1})
	req := &RemoveAttendanceRequest{ShowID: "42"}

	resp, err := h.RemoveAttendanceHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true")
	}
}

func TestRemoveAttendanceHandler_ServiceError(t *testing.T) {
	mock := &testhelpers.MockAttendanceService{
		RemoveAttendanceFn: func(_, _ uint) error {
			return fmt.Errorf("db error")
		},
	}
	h := NewAttendanceHandler(mock)
	ctx := testhelpers.CtxWithUser(&models.User{ID: 1})
	req := &RemoveAttendanceRequest{ShowID: "42"}

	_, err := h.RemoveAttendanceHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 422)
}

// --- GetAttendanceHandler ---

func TestGetAttendanceHandler_InvalidID(t *testing.T) {
	h := NewAttendanceHandler(&testhelpers.MockAttendanceService{})
	req := &GetAttendanceRequest{ShowID: "abc"}

	_, err := h.GetAttendanceHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestGetAttendanceHandler_Success_NoAuth(t *testing.T) {
	mock := &testhelpers.MockAttendanceService{
		GetAttendanceCountsFn: func(showID uint) (*contracts.AttendanceCountsResponse, error) {
			return &contracts.AttendanceCountsResponse{
				ShowID:          showID,
				GoingCount:      5,
				InterestedCount: 10,
			}, nil
		},
	}
	h := NewAttendanceHandler(mock)
	req := &GetAttendanceRequest{ShowID: "42"}

	resp, err := h.GetAttendanceHandler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.GoingCount != 5 {
		t.Errorf("expected going_count=5, got %d", resp.Body.GoingCount)
	}
	if resp.Body.InterestedCount != 10 {
		t.Errorf("expected interested_count=10, got %d", resp.Body.InterestedCount)
	}
	if resp.Body.UserStatus != "" {
		t.Errorf("expected empty user_status for unauthenticated, got %s", resp.Body.UserStatus)
	}
}

func TestGetAttendanceHandler_Success_WithAuth(t *testing.T) {
	mock := &testhelpers.MockAttendanceService{
		GetAttendanceCountsFn: func(showID uint) (*contracts.AttendanceCountsResponse, error) {
			return &contracts.AttendanceCountsResponse{
				ShowID:          showID,
				GoingCount:      3,
				InterestedCount: 7,
			}, nil
		},
		GetUserAttendanceFn: func(userID, showID uint) (string, error) {
			return "going", nil
		},
	}
	h := NewAttendanceHandler(mock)
	ctx := testhelpers.CtxWithUser(&models.User{ID: 1})
	req := &GetAttendanceRequest{ShowID: "42"}

	resp, err := h.GetAttendanceHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.GoingCount != 3 {
		t.Errorf("expected going_count=3, got %d", resp.Body.GoingCount)
	}
	if resp.Body.UserStatus != "going" {
		t.Errorf("expected user_status=going, got %s", resp.Body.UserStatus)
	}
}

func TestGetAttendanceHandler_CountsError(t *testing.T) {
	mock := &testhelpers.MockAttendanceService{
		GetAttendanceCountsFn: func(_ uint) (*contracts.AttendanceCountsResponse, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := NewAttendanceHandler(mock)
	req := &GetAttendanceRequest{ShowID: "42"}

	_, err := h.GetAttendanceHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 500)
}

// --- BatchAttendanceHandler ---

func TestBatchAttendanceHandler_EmptyList(t *testing.T) {
	h := NewAttendanceHandler(&testhelpers.MockAttendanceService{})
	req := &BatchAttendanceRequest{}
	req.Body.ShowIDs = []int{}

	resp, err := h.BatchAttendanceHandler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Body.Attendance) != 0 {
		t.Errorf("expected empty map, got %v", resp.Body.Attendance)
	}
}

func TestBatchAttendanceHandler_TooMany(t *testing.T) {
	h := NewAttendanceHandler(&testhelpers.MockAttendanceService{})
	req := &BatchAttendanceRequest{}
	req.Body.ShowIDs = make([]int, 101)
	for i := range req.Body.ShowIDs {
		req.Body.ShowIDs[i] = i + 1
	}

	_, err := h.BatchAttendanceHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestBatchAttendanceHandler_NegativeID(t *testing.T) {
	h := NewAttendanceHandler(&testhelpers.MockAttendanceService{})
	req := &BatchAttendanceRequest{}
	req.Body.ShowIDs = []int{1, -5, 3}

	_, err := h.BatchAttendanceHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestBatchAttendanceHandler_Success_NoAuth(t *testing.T) {
	mock := &testhelpers.MockAttendanceService{
		GetBatchAttendanceCountsFn: func(showIDs []uint) (map[uint]*contracts.AttendanceCountsResponse, error) {
			result := make(map[uint]*contracts.AttendanceCountsResponse)
			for _, id := range showIDs {
				result[id] = &contracts.AttendanceCountsResponse{
					ShowID:          id,
					GoingCount:      2,
					InterestedCount: 3,
				}
			}
			return result, nil
		},
	}
	h := NewAttendanceHandler(mock)
	req := &BatchAttendanceRequest{}
	req.Body.ShowIDs = []int{1, 2}

	resp, err := h.BatchAttendanceHandler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Body.Attendance) != 2 {
		t.Errorf("expected 2 entries, got %d", len(resp.Body.Attendance))
	}
	entry := resp.Body.Attendance["1"]
	if entry == nil {
		t.Fatal("expected entry for show 1")
	}
	if entry.GoingCount != 2 {
		t.Errorf("expected going_count=2, got %d", entry.GoingCount)
	}
	if entry.UserStatus != "" {
		t.Errorf("expected empty user_status for unauthenticated, got %s", entry.UserStatus)
	}
}

func TestBatchAttendanceHandler_Success_WithAuth(t *testing.T) {
	mock := &testhelpers.MockAttendanceService{
		GetBatchAttendanceCountsFn: func(showIDs []uint) (map[uint]*contracts.AttendanceCountsResponse, error) {
			result := make(map[uint]*contracts.AttendanceCountsResponse)
			for _, id := range showIDs {
				result[id] = &contracts.AttendanceCountsResponse{ShowID: id}
			}
			return result, nil
		},
		GetBatchUserAttendanceFn: func(userID uint, showIDs []uint) (map[uint]string, error) {
			return map[uint]string{1: "going", 2: "interested"}, nil
		},
	}
	h := NewAttendanceHandler(mock)
	ctx := testhelpers.CtxWithUser(&models.User{ID: 1})
	req := &BatchAttendanceRequest{}
	req.Body.ShowIDs = []int{1, 2, 3}

	resp, err := h.BatchAttendanceHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Attendance["1"].UserStatus != "going" {
		t.Errorf("expected user_status=going for show 1, got %s", resp.Body.Attendance["1"].UserStatus)
	}
	if resp.Body.Attendance["2"].UserStatus != "interested" {
		t.Errorf("expected user_status=interested for show 2, got %s", resp.Body.Attendance["2"].UserStatus)
	}
	if resp.Body.Attendance["3"].UserStatus != "" {
		t.Errorf("expected empty user_status for show 3, got %s", resp.Body.Attendance["3"].UserStatus)
	}
}

func TestBatchAttendanceHandler_CountsError(t *testing.T) {
	mock := &testhelpers.MockAttendanceService{
		GetBatchAttendanceCountsFn: func(_ []uint) (map[uint]*contracts.AttendanceCountsResponse, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := NewAttendanceHandler(mock)
	req := &BatchAttendanceRequest{}
	req.Body.ShowIDs = []int{1}

	_, err := h.BatchAttendanceHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 500)
}

// --- GetMyShowsHandler ---

func TestGetMyShowsHandler_NoAuth(t *testing.T) {
	h := testAttendanceHandler()
	req := &GetMyShowsRequest{}

	_, err := h.GetMyShowsHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 401)
}

func TestGetMyShowsHandler_InvalidStatus(t *testing.T) {
	h := NewAttendanceHandler(&testhelpers.MockAttendanceService{})
	ctx := testhelpers.CtxWithUser(&models.User{ID: 1})
	req := &GetMyShowsRequest{Status: "maybe"}

	_, err := h.GetMyShowsHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestGetMyShowsHandler_Success(t *testing.T) {
	now := time.Now().UTC()
	shows := []*contracts.AttendingShowResponse{
		{
			ShowID:    1,
			Title:     "Test Show",
			Slug:      "test-show",
			EventDate: now.AddDate(0, 0, 7),
			Status:    "going",
		},
	}
	mock := &testhelpers.MockAttendanceService{
		GetUserAttendingShowsFn: func(userID uint, status string, limit, offset int) ([]*contracts.AttendingShowResponse, int64, error) {
			if userID != 1 {
				t.Errorf("unexpected userID=%d", userID)
			}
			if status != "all" {
				t.Errorf("unexpected status=%s", status)
			}
			return shows, 1, nil
		},
	}
	h := NewAttendanceHandler(mock)
	ctx := testhelpers.CtxWithUser(&models.User{ID: 1})
	req := &GetMyShowsRequest{Status: "all", Limit: 20, Offset: 0}

	resp, err := h.GetMyShowsHandler(ctx, req)
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

func TestGetMyShowsHandler_ServiceError(t *testing.T) {
	mock := &testhelpers.MockAttendanceService{
		GetUserAttendingShowsFn: func(_ uint, _ string, _, _ int) ([]*contracts.AttendingShowResponse, int64, error) {
			return nil, 0, fmt.Errorf("db error")
		},
	}
	h := NewAttendanceHandler(mock)
	ctx := testhelpers.CtxWithUser(&models.User{ID: 1})
	req := &GetMyShowsRequest{Status: "all", Limit: 20}

	_, err := h.GetMyShowsHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 500)
}

func TestGetMyShowsHandler_PaginationClamping(t *testing.T) {
	var capturedLimit, capturedOffset int
	mock := &testhelpers.MockAttendanceService{
		GetUserAttendingShowsFn: func(_ uint, _ string, limit, offset int) ([]*contracts.AttendingShowResponse, int64, error) {
			capturedLimit = limit
			capturedOffset = offset
			return nil, 0, nil
		},
	}
	h := NewAttendanceHandler(mock)
	ctx := testhelpers.CtxWithUser(&models.User{ID: 1})

	// limit=0 -> 20, offset=-1 -> 0
	_, err := h.GetMyShowsHandler(ctx, &GetMyShowsRequest{Status: "all", Limit: 0, Offset: -1})
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
	h.GetMyShowsHandler(ctx, &GetMyShowsRequest{Status: "all", Limit: 999})
	if capturedLimit != 100 {
		t.Errorf("expected limit=100, got %d", capturedLimit)
	}
}

func TestGetMyShowsHandler_DefaultStatus(t *testing.T) {
	var capturedStatus string
	mock := &testhelpers.MockAttendanceService{
		GetUserAttendingShowsFn: func(_ uint, status string, _, _ int) ([]*contracts.AttendingShowResponse, int64, error) {
			capturedStatus = status
			return nil, 0, nil
		},
	}
	h := NewAttendanceHandler(mock)
	ctx := testhelpers.CtxWithUser(&models.User{ID: 1})

	// Empty status defaults to "all"
	_, err := h.GetMyShowsHandler(ctx, &GetMyShowsRequest{Status: "", Limit: 20})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedStatus != "all" {
		t.Errorf("expected status=all, got %s", capturedStatus)
	}
}
