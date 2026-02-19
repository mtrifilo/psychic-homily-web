package handlers

import (
	"context"
	"fmt"
	"testing"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services"
)

func testShowReportHandler() *ShowReportHandler {
	return NewShowReportHandler(nil, nil, nil, nil)
}

// --- NewShowReportHandler ---

func TestNewShowReportHandler(t *testing.T) {
	h := testShowReportHandler()
	if h == nil {
		t.Fatal("expected non-nil ShowReportHandler")
	}
}

// --- ReportShowHandler ---

func TestReportShowHandler_NoAuth(t *testing.T) {
	h := testShowReportHandler()
	req := &ReportShowRequest{ShowID: "1"}

	_, err := h.ReportShowHandler(context.Background(), req)
	assertHumaError(t, err, 401)
}

func TestReportShowHandler_InvalidID(t *testing.T) {
	h := testShowReportHandler()
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &ReportShowRequest{ShowID: "abc"}

	_, err := h.ReportShowHandler(ctx, req)
	assertHumaError(t, err, 400)
}

func TestReportShowHandler_Success(t *testing.T) {
	report := &services.ShowReportResponse{ID: 10, ShowID: 42, ReportType: "cancelled", Status: "pending"}
	mock := &mockShowReportService{
		createReportFn: func(userID, showID uint, reportType string, details *string) (*services.ShowReportResponse, error) {
			if userID != 1 || showID != 42 {
				t.Errorf("unexpected args: userID=%d, showID=%d", userID, showID)
			}
			if reportType != "cancelled" {
				t.Errorf("unexpected reportType=%s", reportType)
			}
			return report, nil
		},
		getReportByIDFn: func(reportID uint) (*models.ShowReport, error) {
			return &models.ShowReport{ID: reportID}, nil
		},
	}
	email := "user@test.com"
	h := NewShowReportHandler(mock, &mockDiscordService{}, &mockUserService{}, &mockAuditLogService{})
	ctx := ctxWithUser(&models.User{ID: 1, Email: &email})

	req := &ReportShowRequest{ShowID: "42"}
	req.Body.ReportType = "cancelled"
	resp, err := h.ReportShowHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.ID != 10 {
		t.Errorf("expected report ID=10, got %d", resp.Body.ID)
	}
}

func TestReportShowHandler_ServiceError(t *testing.T) {
	mock := &mockShowReportService{
		createReportFn: func(_, _ uint, _ string, _ *string) (*services.ShowReportResponse, error) {
			return nil, fmt.Errorf("duplicate report")
		},
	}
	h := NewShowReportHandler(mock, &mockDiscordService{}, &mockUserService{}, &mockAuditLogService{})
	ctx := ctxWithUser(&models.User{ID: 1})

	req := &ReportShowRequest{ShowID: "42"}
	req.Body.ReportType = "cancelled"
	_, err := h.ReportShowHandler(ctx, req)
	assertHumaError(t, err, 422)
}

// --- GetMyReportHandler ---

func TestGetMyReportHandler_NoAuth(t *testing.T) {
	h := testShowReportHandler()
	req := &GetMyReportRequest{ShowID: "1"}

	_, err := h.GetMyReportHandler(context.Background(), req)
	assertHumaError(t, err, 401)
}

func TestGetMyReportHandler_InvalidID(t *testing.T) {
	h := testShowReportHandler()
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &GetMyReportRequest{ShowID: "abc"}

	_, err := h.GetMyReportHandler(ctx, req)
	assertHumaError(t, err, 400)
}

func TestGetMyReportHandler_Success(t *testing.T) {
	report := &services.ShowReportResponse{ID: 10, ShowID: 42}
	mock := &mockShowReportService{
		getUserReportFn: func(userID, showID uint) (*services.ShowReportResponse, error) {
			if userID != 1 || showID != 42 {
				t.Errorf("unexpected args: userID=%d, showID=%d", userID, showID)
			}
			return report, nil
		},
	}
	h := NewShowReportHandler(mock, nil, nil, nil)
	ctx := ctxWithUser(&models.User{ID: 1})

	resp, err := h.GetMyReportHandler(ctx, &GetMyReportRequest{ShowID: "42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Report == nil {
		t.Fatal("expected non-nil report")
	}
	if resp.Body.Report.ID != 10 {
		t.Errorf("expected report ID=10, got %d", resp.Body.Report.ID)
	}
}

func TestGetMyReportHandler_NoReport(t *testing.T) {
	mock := &mockShowReportService{
		getUserReportFn: func(_, _ uint) (*services.ShowReportResponse, error) {
			return nil, nil
		},
	}
	h := NewShowReportHandler(mock, nil, nil, nil)
	ctx := ctxWithUser(&models.User{ID: 1})

	resp, err := h.GetMyReportHandler(ctx, &GetMyReportRequest{ShowID: "42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Report != nil {
		t.Errorf("expected nil report, got %+v", resp.Body.Report)
	}
}

func TestGetMyReportHandler_ServiceError(t *testing.T) {
	mock := &mockShowReportService{
		getUserReportFn: func(_, _ uint) (*services.ShowReportResponse, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := NewShowReportHandler(mock, nil, nil, nil)
	ctx := ctxWithUser(&models.User{ID: 1})

	_, err := h.GetMyReportHandler(ctx, &GetMyReportRequest{ShowID: "42"})
	assertHumaError(t, err, 500)
}

// --- GetPendingReportsHandler ---

func TestGetPendingReportsHandler_NoAuth(t *testing.T) {
	h := testShowReportHandler()
	req := &GetPendingReportsRequest{}

	_, err := h.GetPendingReportsHandler(context.Background(), req)
	assertHumaError(t, err, 403)
}

func TestGetPendingReportsHandler_NonAdmin(t *testing.T) {
	h := testShowReportHandler()
	ctx := ctxWithUser(&models.User{ID: 1, IsAdmin: false})
	req := &GetPendingReportsRequest{}

	_, err := h.GetPendingReportsHandler(ctx, req)
	assertHumaError(t, err, 403)
}

func TestGetPendingReportsHandler_Success(t *testing.T) {
	reports := []*services.ShowReportResponse{{ID: 1}, {ID: 2}}
	mock := &mockShowReportService{
		getPendingReportsFn: func(limit, offset int) ([]*services.ShowReportResponse, int64, error) {
			return reports, 2, nil
		},
	}
	h := NewShowReportHandler(mock, nil, nil, nil)
	ctx := ctxWithUser(&models.User{ID: 1, IsAdmin: true})

	resp, err := h.GetPendingReportsHandler(ctx, &GetPendingReportsRequest{Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Total != 2 {
		t.Errorf("expected total=2, got %d", resp.Body.Total)
	}
	if len(resp.Body.Reports) != 2 {
		t.Errorf("expected 2 reports, got %d", len(resp.Body.Reports))
	}
}

func TestGetPendingReportsHandler_ServiceError(t *testing.T) {
	mock := &mockShowReportService{
		getPendingReportsFn: func(_, _ int) ([]*services.ShowReportResponse, int64, error) {
			return nil, 0, fmt.Errorf("db error")
		},
	}
	h := NewShowReportHandler(mock, nil, nil, nil)
	ctx := ctxWithUser(&models.User{ID: 1, IsAdmin: true})

	_, err := h.GetPendingReportsHandler(ctx, &GetPendingReportsRequest{Limit: 10})
	assertHumaError(t, err, 500)
}

// --- DismissReportHandler ---

func TestDismissReportHandler_NoAuth(t *testing.T) {
	h := testShowReportHandler()
	req := &DismissReportRequest{ReportID: "1"}

	_, err := h.DismissReportHandler(context.Background(), req)
	assertHumaError(t, err, 403)
}

func TestDismissReportHandler_NonAdmin(t *testing.T) {
	h := testShowReportHandler()
	ctx := ctxWithUser(&models.User{ID: 1, IsAdmin: false})
	req := &DismissReportRequest{ReportID: "1"}

	_, err := h.DismissReportHandler(ctx, req)
	assertHumaError(t, err, 403)
}

func TestDismissReportHandler_InvalidID(t *testing.T) {
	h := testShowReportHandler()
	ctx := ctxWithUser(&models.User{ID: 1, IsAdmin: true})
	req := &DismissReportRequest{ReportID: "abc"}

	_, err := h.DismissReportHandler(ctx, req)
	assertHumaError(t, err, 400)
}

func TestDismissReportHandler_Success(t *testing.T) {
	report := &services.ShowReportResponse{ID: 5, ShowID: 42, Status: "dismissed"}
	var auditLogged bool
	mock := &mockShowReportService{
		dismissReportFn: func(reportID, adminID uint, notes *string) (*services.ShowReportResponse, error) {
			if reportID != 5 || adminID != 99 {
				t.Errorf("unexpected args: reportID=%d, adminID=%d", reportID, adminID)
			}
			return report, nil
		},
	}
	auditMock := &mockAuditLogService{
		logActionFn: func(actorID uint, action, entityType string, entityID uint, metadata map[string]interface{}) {
			auditLogged = true
			if action != "dismiss_report" {
				t.Errorf("expected action=dismiss_report, got %s", action)
			}
			if entityType != "show_report" {
				t.Errorf("expected entityType=show_report, got %s", entityType)
			}
		},
	}
	h := NewShowReportHandler(mock, nil, nil, auditMock)
	ctx := ctxWithUser(&models.User{ID: 99, IsAdmin: true})

	resp, err := h.DismissReportHandler(ctx, &DismissReportRequest{ReportID: "5"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Status != "dismissed" {
		t.Errorf("expected status=dismissed, got %s", resp.Body.Status)
	}
	if !auditLogged {
		t.Error("expected audit log to be called")
	}
}

func TestDismissReportHandler_ServiceError(t *testing.T) {
	mock := &mockShowReportService{
		dismissReportFn: func(_, _ uint, _ *string) (*services.ShowReportResponse, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	h := NewShowReportHandler(mock, nil, nil, &mockAuditLogService{})
	ctx := ctxWithUser(&models.User{ID: 1, IsAdmin: true})

	_, err := h.DismissReportHandler(ctx, &DismissReportRequest{ReportID: "5"})
	assertHumaError(t, err, 422)
}

// --- ResolveReportHandler ---

func TestResolveReportHandler_NoAuth(t *testing.T) {
	h := testShowReportHandler()
	req := &ResolveReportRequest{ReportID: "1"}

	_, err := h.ResolveReportHandler(context.Background(), req)
	assertHumaError(t, err, 403)
}

func TestResolveReportHandler_NonAdmin(t *testing.T) {
	h := testShowReportHandler()
	ctx := ctxWithUser(&models.User{ID: 1, IsAdmin: false})
	req := &ResolveReportRequest{ReportID: "1"}

	_, err := h.ResolveReportHandler(ctx, req)
	assertHumaError(t, err, 403)
}

func TestResolveReportHandler_InvalidID(t *testing.T) {
	h := testShowReportHandler()
	ctx := ctxWithUser(&models.User{ID: 1, IsAdmin: true})
	req := &ResolveReportRequest{ReportID: "abc"}

	_, err := h.ResolveReportHandler(ctx, req)
	assertHumaError(t, err, 400)
}

func TestResolveReportHandler_Success(t *testing.T) {
	report := &services.ShowReportResponse{ID: 5, ShowID: 42, Status: "resolved"}
	var auditAction string
	mock := &mockShowReportService{
		resolveWithFlagFn: func(reportID, adminID uint, notes *string, setShowFlag bool) (*services.ShowReportResponse, error) {
			if reportID != 5 || adminID != 99 {
				t.Errorf("unexpected args: reportID=%d, adminID=%d", reportID, adminID)
			}
			if setShowFlag {
				t.Error("expected setShowFlag=false")
			}
			return report, nil
		},
	}
	auditMock := &mockAuditLogService{
		logActionFn: func(_ uint, action, _ string, _ uint, _ map[string]interface{}) {
			auditAction = action
		},
	}
	h := NewShowReportHandler(mock, nil, nil, auditMock)
	ctx := ctxWithUser(&models.User{ID: 99, IsAdmin: true})

	resp, err := h.ResolveReportHandler(ctx, &ResolveReportRequest{ReportID: "5"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Status != "resolved" {
		t.Errorf("expected status=resolved, got %s", resp.Body.Status)
	}
	if auditAction != "resolve_report" {
		t.Errorf("expected audit action=resolve_report, got %s", auditAction)
	}
}

func TestResolveReportHandler_WithFlag(t *testing.T) {
	report := &services.ShowReportResponse{ID: 5, ShowID: 42, Status: "resolved"}
	var capturedFlag bool
	var auditAction string
	mock := &mockShowReportService{
		resolveWithFlagFn: func(_, _ uint, _ *string, setShowFlag bool) (*services.ShowReportResponse, error) {
			capturedFlag = setShowFlag
			return report, nil
		},
	}
	auditMock := &mockAuditLogService{
		logActionFn: func(_ uint, action, _ string, _ uint, meta map[string]interface{}) {
			auditAction = action
		},
	}
	h := NewShowReportHandler(mock, nil, nil, auditMock)
	ctx := ctxWithUser(&models.User{ID: 99, IsAdmin: true})

	req := &ResolveReportRequest{ReportID: "5"}
	flagTrue := true
	req.Body.SetShowFlag = &flagTrue

	_, err := h.ResolveReportHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !capturedFlag {
		t.Error("expected setShowFlag=true to be passed to service")
	}
	if auditAction != "resolve_report_with_flag" {
		t.Errorf("expected audit action=resolve_report_with_flag, got %s", auditAction)
	}
}

func TestResolveReportHandler_ServiceError(t *testing.T) {
	mock := &mockShowReportService{
		resolveWithFlagFn: func(_, _ uint, _ *string, _ bool) (*services.ShowReportResponse, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	h := NewShowReportHandler(mock, nil, nil, &mockAuditLogService{})
	ctx := ctxWithUser(&models.User{ID: 1, IsAdmin: true})

	_, err := h.ResolveReportHandler(ctx, &ResolveReportRequest{ReportID: "5"})
	assertHumaError(t, err, 422)
}
