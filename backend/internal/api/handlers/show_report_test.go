package handlers

import (
	"context"
	"testing"

	"psychic-homily-backend/internal/models"
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
