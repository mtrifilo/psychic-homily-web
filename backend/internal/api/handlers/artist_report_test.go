package handlers

import (
	"context"
	"fmt"
	"testing"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services"
)

func testArtistReportHandler() *ArtistReportHandler {
	return NewArtistReportHandler(nil, nil, nil, nil)
}

// --- NewArtistReportHandler ---

func TestNewArtistReportHandler(t *testing.T) {
	h := testArtistReportHandler()
	if h == nil {
		t.Fatal("expected non-nil ArtistReportHandler")
	}
}

// --- ReportArtistHandler ---

func TestReportArtistHandler_NoAuth(t *testing.T) {
	h := testArtistReportHandler()
	req := &ReportArtistRequest{ArtistID: "1"}

	_, err := h.ReportArtistHandler(context.Background(), req)
	assertHumaError(t, err, 401)
}

func TestReportArtistHandler_InvalidID(t *testing.T) {
	h := testArtistReportHandler()
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &ReportArtistRequest{ArtistID: "abc"}

	_, err := h.ReportArtistHandler(ctx, req)
	assertHumaError(t, err, 400)
}

func TestReportArtistHandler_Success(t *testing.T) {
	report := &services.ArtistReportResponse{ID: 10, ArtistID: 7, ReportType: "inaccurate", Status: "pending"}
	mock := &mockArtistReportService{
		createReportFn: func(userID, artistID uint, reportType string, details *string) (*services.ArtistReportResponse, error) {
			if userID != 1 || artistID != 7 {
				t.Errorf("unexpected args: userID=%d, artistID=%d", userID, artistID)
			}
			if reportType != "inaccurate" {
				t.Errorf("unexpected reportType=%s", reportType)
			}
			return report, nil
		},
		getReportByIDFn: func(reportID uint) (*models.ArtistReport, error) {
			return &models.ArtistReport{ID: reportID}, nil
		},
	}
	email := "user@test.com"
	h := NewArtistReportHandler(mock, &mockDiscordService{}, &mockUserService{}, &mockAuditLogService{})
	ctx := ctxWithUser(&models.User{ID: 1, Email: &email})

	req := &ReportArtistRequest{ArtistID: "7"}
	req.Body.ReportType = "inaccurate"
	resp, err := h.ReportArtistHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.ID != 10 {
		t.Errorf("expected report ID=10, got %d", resp.Body.ID)
	}
}

func TestReportArtistHandler_ServiceError(t *testing.T) {
	mock := &mockArtistReportService{
		createReportFn: func(_, _ uint, _ string, _ *string) (*services.ArtistReportResponse, error) {
			return nil, fmt.Errorf("duplicate report")
		},
	}
	h := NewArtistReportHandler(mock, &mockDiscordService{}, &mockUserService{}, &mockAuditLogService{})
	ctx := ctxWithUser(&models.User{ID: 1})

	req := &ReportArtistRequest{ArtistID: "7"}
	req.Body.ReportType = "inaccurate"
	_, err := h.ReportArtistHandler(ctx, req)
	assertHumaError(t, err, 422)
}

// --- GetMyArtistReportHandler ---

func TestGetMyArtistReportHandler_NoAuth(t *testing.T) {
	h := testArtistReportHandler()
	req := &GetMyArtistReportRequest{ArtistID: "1"}

	_, err := h.GetMyArtistReportHandler(context.Background(), req)
	assertHumaError(t, err, 401)
}

func TestGetMyArtistReportHandler_InvalidID(t *testing.T) {
	h := testArtistReportHandler()
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &GetMyArtistReportRequest{ArtistID: "abc"}

	_, err := h.GetMyArtistReportHandler(ctx, req)
	assertHumaError(t, err, 400)
}

func TestGetMyArtistReportHandler_Success(t *testing.T) {
	report := &services.ArtistReportResponse{ID: 10, ArtistID: 7}
	mock := &mockArtistReportService{
		getUserReportFn: func(userID, artistID uint) (*services.ArtistReportResponse, error) {
			if userID != 1 || artistID != 7 {
				t.Errorf("unexpected args: userID=%d, artistID=%d", userID, artistID)
			}
			return report, nil
		},
	}
	h := NewArtistReportHandler(mock, nil, nil, nil)
	ctx := ctxWithUser(&models.User{ID: 1})

	resp, err := h.GetMyArtistReportHandler(ctx, &GetMyArtistReportRequest{ArtistID: "7"})
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

func TestGetMyArtistReportHandler_NoReport(t *testing.T) {
	mock := &mockArtistReportService{
		getUserReportFn: func(_, _ uint) (*services.ArtistReportResponse, error) {
			return nil, nil
		},
	}
	h := NewArtistReportHandler(mock, nil, nil, nil)
	ctx := ctxWithUser(&models.User{ID: 1})

	resp, err := h.GetMyArtistReportHandler(ctx, &GetMyArtistReportRequest{ArtistID: "7"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Report != nil {
		t.Errorf("expected nil report, got %+v", resp.Body.Report)
	}
}

func TestGetMyArtistReportHandler_ServiceError(t *testing.T) {
	mock := &mockArtistReportService{
		getUserReportFn: func(_, _ uint) (*services.ArtistReportResponse, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := NewArtistReportHandler(mock, nil, nil, nil)
	ctx := ctxWithUser(&models.User{ID: 1})

	_, err := h.GetMyArtistReportHandler(ctx, &GetMyArtistReportRequest{ArtistID: "7"})
	assertHumaError(t, err, 500)
}

// --- GetPendingArtistReportsHandler ---

func TestGetPendingArtistReportsHandler_NoAuth(t *testing.T) {
	h := testArtistReportHandler()
	req := &GetPendingArtistReportsRequest{}

	_, err := h.GetPendingArtistReportsHandler(context.Background(), req)
	assertHumaError(t, err, 403)
}

func TestGetPendingArtistReportsHandler_NonAdmin(t *testing.T) {
	h := testArtistReportHandler()
	ctx := ctxWithUser(&models.User{ID: 1, IsAdmin: false})
	req := &GetPendingArtistReportsRequest{}

	_, err := h.GetPendingArtistReportsHandler(ctx, req)
	assertHumaError(t, err, 403)
}

func TestGetPendingArtistReportsHandler_Success(t *testing.T) {
	reports := []*services.ArtistReportResponse{{ID: 1}, {ID: 2}}
	mock := &mockArtistReportService{
		getPendingReportsFn: func(limit, offset int) ([]*services.ArtistReportResponse, int64, error) {
			return reports, 2, nil
		},
	}
	h := NewArtistReportHandler(mock, nil, nil, nil)
	ctx := ctxWithUser(&models.User{ID: 1, IsAdmin: true})

	resp, err := h.GetPendingArtistReportsHandler(ctx, &GetPendingArtistReportsRequest{Limit: 10})
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

func TestGetPendingArtistReportsHandler_ServiceError(t *testing.T) {
	mock := &mockArtistReportService{
		getPendingReportsFn: func(_, _ int) ([]*services.ArtistReportResponse, int64, error) {
			return nil, 0, fmt.Errorf("db error")
		},
	}
	h := NewArtistReportHandler(mock, nil, nil, nil)
	ctx := ctxWithUser(&models.User{ID: 1, IsAdmin: true})

	_, err := h.GetPendingArtistReportsHandler(ctx, &GetPendingArtistReportsRequest{Limit: 10})
	assertHumaError(t, err, 500)
}

// --- DismissArtistReportHandler ---

func TestDismissArtistReportHandler_NoAuth(t *testing.T) {
	h := testArtistReportHandler()
	req := &DismissArtistReportRequest{ReportID: "1"}

	_, err := h.DismissArtistReportHandler(context.Background(), req)
	assertHumaError(t, err, 403)
}

func TestDismissArtistReportHandler_NonAdmin(t *testing.T) {
	h := testArtistReportHandler()
	ctx := ctxWithUser(&models.User{ID: 1, IsAdmin: false})
	req := &DismissArtistReportRequest{ReportID: "1"}

	_, err := h.DismissArtistReportHandler(ctx, req)
	assertHumaError(t, err, 403)
}

func TestDismissArtistReportHandler_InvalidID(t *testing.T) {
	h := testArtistReportHandler()
	ctx := ctxWithUser(&models.User{ID: 1, IsAdmin: true})
	req := &DismissArtistReportRequest{ReportID: "abc"}

	_, err := h.DismissArtistReportHandler(ctx, req)
	assertHumaError(t, err, 400)
}

func TestDismissArtistReportHandler_Success(t *testing.T) {
	report := &services.ArtistReportResponse{ID: 5, ArtistID: 7, Status: "dismissed"}
	var auditLogged bool
	mock := &mockArtistReportService{
		dismissReportFn: func(reportID, adminID uint, notes *string) (*services.ArtistReportResponse, error) {
			if reportID != 5 || adminID != 99 {
				t.Errorf("unexpected args: reportID=%d, adminID=%d", reportID, adminID)
			}
			return report, nil
		},
	}
	auditMock := &mockAuditLogService{
		logActionFn: func(actorID uint, action, entityType string, entityID uint, metadata map[string]interface{}) {
			auditLogged = true
			if action != "dismiss_artist_report" {
				t.Errorf("expected action=dismiss_artist_report, got %s", action)
			}
			if entityType != "artist_report" {
				t.Errorf("expected entityType=artist_report, got %s", entityType)
			}
		},
	}
	h := NewArtistReportHandler(mock, nil, nil, auditMock)
	ctx := ctxWithUser(&models.User{ID: 99, IsAdmin: true})

	resp, err := h.DismissArtistReportHandler(ctx, &DismissArtistReportRequest{ReportID: "5"})
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

func TestDismissArtistReportHandler_ServiceError(t *testing.T) {
	mock := &mockArtistReportService{
		dismissReportFn: func(_, _ uint, _ *string) (*services.ArtistReportResponse, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	h := NewArtistReportHandler(mock, nil, nil, &mockAuditLogService{})
	ctx := ctxWithUser(&models.User{ID: 1, IsAdmin: true})

	_, err := h.DismissArtistReportHandler(ctx, &DismissArtistReportRequest{ReportID: "5"})
	assertHumaError(t, err, 422)
}

// --- ResolveArtistReportHandler ---

func TestResolveArtistReportHandler_NoAuth(t *testing.T) {
	h := testArtistReportHandler()
	req := &ResolveArtistReportRequest{ReportID: "1"}

	_, err := h.ResolveArtistReportHandler(context.Background(), req)
	assertHumaError(t, err, 403)
}

func TestResolveArtistReportHandler_NonAdmin(t *testing.T) {
	h := testArtistReportHandler()
	ctx := ctxWithUser(&models.User{ID: 1, IsAdmin: false})
	req := &ResolveArtistReportRequest{ReportID: "1"}

	_, err := h.ResolveArtistReportHandler(ctx, req)
	assertHumaError(t, err, 403)
}

func TestResolveArtistReportHandler_InvalidID(t *testing.T) {
	h := testArtistReportHandler()
	ctx := ctxWithUser(&models.User{ID: 1, IsAdmin: true})
	req := &ResolveArtistReportRequest{ReportID: "abc"}

	_, err := h.ResolveArtistReportHandler(ctx, req)
	assertHumaError(t, err, 400)
}

func TestResolveArtistReportHandler_Success(t *testing.T) {
	report := &services.ArtistReportResponse{ID: 5, ArtistID: 7, Status: "resolved"}
	var auditAction string
	mock := &mockArtistReportService{
		resolveReportFn: func(reportID, adminID uint, notes *string) (*services.ArtistReportResponse, error) {
			if reportID != 5 || adminID != 99 {
				t.Errorf("unexpected args: reportID=%d, adminID=%d", reportID, adminID)
			}
			return report, nil
		},
	}
	auditMock := &mockAuditLogService{
		logActionFn: func(_ uint, action, _ string, _ uint, _ map[string]interface{}) {
			auditAction = action
		},
	}
	h := NewArtistReportHandler(mock, nil, nil, auditMock)
	ctx := ctxWithUser(&models.User{ID: 99, IsAdmin: true})

	resp, err := h.ResolveArtistReportHandler(ctx, &ResolveArtistReportRequest{ReportID: "5"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Status != "resolved" {
		t.Errorf("expected status=resolved, got %s", resp.Body.Status)
	}
	if auditAction != "resolve_artist_report" {
		t.Errorf("expected audit action=resolve_artist_report, got %s", auditAction)
	}
}

func TestResolveArtistReportHandler_ServiceError(t *testing.T) {
	mock := &mockArtistReportService{
		resolveReportFn: func(_, _ uint, _ *string) (*services.ArtistReportResponse, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	h := NewArtistReportHandler(mock, nil, nil, &mockAuditLogService{})
	ctx := ctxWithUser(&models.User{ID: 1, IsAdmin: true})

	_, err := h.ResolveArtistReportHandler(ctx, &ResolveArtistReportRequest{ReportID: "5"})
	assertHumaError(t, err, 422)
}
