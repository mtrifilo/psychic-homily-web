package handlers

import (
	"context"
	"fmt"
	"testing"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services"
)

func testAuditLogHandler() *AuditLogHandler {
	return NewAuditLogHandler(nil)
}

// --- NewAuditLogHandler ---

func TestNewAuditLogHandler(t *testing.T) {
	h := testAuditLogHandler()
	if h == nil {
		t.Fatal("expected non-nil AuditLogHandler")
	}
}

// --- GetAuditLogsHandler ---

func TestGetAuditLogsHandler_NoAuth(t *testing.T) {
	h := testAuditLogHandler()
	req := &GetAuditLogsRequest{}

	_, err := h.GetAuditLogsHandler(context.Background(), req)
	assertHumaError(t, err, 403)
}

func TestGetAuditLogsHandler_NonAdmin(t *testing.T) {
	h := testAuditLogHandler()
	ctx := ctxWithUser(&models.User{ID: 1, IsAdmin: false})
	req := &GetAuditLogsRequest{}

	_, err := h.GetAuditLogsHandler(ctx, req)
	assertHumaError(t, err, 403)
}

func TestGetAuditLogsHandler_Success(t *testing.T) {
	logs := []*services.AuditLogResponse{{ID: 1, Action: "approve_show"}}
	mock := &mockAuditLogService{
		getAuditLogsFn: func(limit, offset int, filters services.AuditLogFilters) ([]*services.AuditLogResponse, int64, error) {
			return logs, 1, nil
		},
	}
	h := NewAuditLogHandler(mock)
	ctx := ctxWithUser(&models.User{ID: 1, IsAdmin: true})

	resp, err := h.GetAuditLogsHandler(ctx, &GetAuditLogsRequest{Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Total != 1 {
		t.Errorf("expected total=1, got %d", resp.Body.Total)
	}
	if len(resp.Body.Logs) != 1 {
		t.Errorf("expected 1 log, got %d", len(resp.Body.Logs))
	}
}

func TestGetAuditLogsHandler_ServiceError(t *testing.T) {
	mock := &mockAuditLogService{
		getAuditLogsFn: func(_, _ int, _ services.AuditLogFilters) ([]*services.AuditLogResponse, int64, error) {
			return nil, 0, fmt.Errorf("db error")
		},
	}
	h := NewAuditLogHandler(mock)
	ctx := ctxWithUser(&models.User{ID: 1, IsAdmin: true})

	_, err := h.GetAuditLogsHandler(ctx, &GetAuditLogsRequest{Limit: 10})
	assertHumaError(t, err, 500)
}

func TestGetAuditLogsHandler_LimitClamping(t *testing.T) {
	var capturedLimit int
	mock := &mockAuditLogService{
		getAuditLogsFn: func(limit, _ int, _ services.AuditLogFilters) ([]*services.AuditLogResponse, int64, error) {
			capturedLimit = limit
			return nil, 0, nil
		},
	}
	h := NewAuditLogHandler(mock)
	ctx := ctxWithUser(&models.User{ID: 1, IsAdmin: true})

	// limit=0 → 50
	h.GetAuditLogsHandler(ctx, &GetAuditLogsRequest{Limit: 0})
	if capturedLimit != 50 {
		t.Errorf("expected limit clamped to 50, got %d", capturedLimit)
	}

	// limit=200 → 100
	h.GetAuditLogsHandler(ctx, &GetAuditLogsRequest{Limit: 200})
	if capturedLimit != 100 {
		t.Errorf("expected limit clamped to 100, got %d", capturedLimit)
	}
}

func TestGetAuditLogsHandler_FiltersPassedThrough(t *testing.T) {
	var capturedFilters services.AuditLogFilters
	mock := &mockAuditLogService{
		getAuditLogsFn: func(_ int, _ int, filters services.AuditLogFilters) ([]*services.AuditLogResponse, int64, error) {
			capturedFilters = filters
			return nil, 0, nil
		},
	}
	h := NewAuditLogHandler(mock)
	ctx := ctxWithUser(&models.User{ID: 1, IsAdmin: true})

	h.GetAuditLogsHandler(ctx, &GetAuditLogsRequest{
		Limit:      10,
		EntityType: "show",
		Action:     "approve_show",
	})
	if capturedFilters.EntityType != "show" {
		t.Errorf("expected entity_type=show, got %s", capturedFilters.EntityType)
	}
	if capturedFilters.Action != "approve_show" {
		t.Errorf("expected action=approve_show, got %s", capturedFilters.Action)
	}
}
