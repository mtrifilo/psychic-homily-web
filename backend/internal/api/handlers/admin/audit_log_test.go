package admin

import (
	"context"
	"fmt"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

func testAuditLogHandler() *AuditLogHandler {
	return NewAuditLogHandler(nil)
}

// --- GetAuditLogsHandler ---

func TestGetAuditLogsHandler_NoAuth(t *testing.T) {
	h := testAuditLogHandler()
	req := &GetAuditLogsRequest{}

	_, err := h.GetAuditLogsHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 403)
}

func TestGetAuditLogsHandler_NonAdmin(t *testing.T) {
	h := testAuditLogHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: false})
	req := &GetAuditLogsRequest{}

	_, err := h.GetAuditLogsHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 403)
}

func TestGetAuditLogsHandler_Success(t *testing.T) {
	logs := []*contracts.AuditLogResponse{{ID: 1, Action: "approve_show"}}
	mock := &testhelpers.MockAuditLogService{
		GetAuditLogsFn: func(limit, offset int, filters contracts.AuditLogFilters) ([]*contracts.AuditLogResponse, int64, error) {
			return logs, 1, nil
		},
	}
	h := NewAuditLogHandler(mock)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})

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
	mock := &testhelpers.MockAuditLogService{
		GetAuditLogsFn: func(_, _ int, _ contracts.AuditLogFilters) ([]*contracts.AuditLogResponse, int64, error) {
			return nil, 0, fmt.Errorf("db error")
		},
	}
	h := NewAuditLogHandler(mock)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})

	_, err := h.GetAuditLogsHandler(ctx, &GetAuditLogsRequest{Limit: 10})
	testhelpers.AssertHumaError(t, err, 500)
}

func TestGetAuditLogsHandler_LimitClamping(t *testing.T) {
	var capturedLimit int
	mock := &testhelpers.MockAuditLogService{
		GetAuditLogsFn: func(limit, _ int, _ contracts.AuditLogFilters) ([]*contracts.AuditLogResponse, int64, error) {
			capturedLimit = limit
			return nil, 0, nil
		},
	}
	h := NewAuditLogHandler(mock)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})

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
	var capturedFilters contracts.AuditLogFilters
	mock := &testhelpers.MockAuditLogService{
		GetAuditLogsFn: func(_ int, _ int, filters contracts.AuditLogFilters) ([]*contracts.AuditLogResponse, int64, error) {
			capturedFilters = filters
			return nil, 0, nil
		},
	}
	h := NewAuditLogHandler(mock)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})

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
