package handlers

import (
	"context"
	"fmt"
	"testing"
	"time"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
)

// ============================================================================
// Test helpers
// ============================================================================

func testEntityReportHandler() *EntityReportHandler {
	return NewEntityReportHandler(nil, nil)
}

func entityReportAdminCtx() context.Context {
	return ctxWithUser(&models.User{ID: 1, IsAdmin: true})
}

func entityReportUserCtx() context.Context {
	return ctxWithUser(&models.User{ID: 2, IsAdmin: false})
}

func makeEntityReportResponse(id uint, entityType, reportType string) *contracts.EntityReportResponse {
	return &contracts.EntityReportResponse{
		ID:           id,
		EntityType:   entityType,
		EntityID:     10,
		ReportedBy:   2,
		ReporterName: "testuser",
		ReportType:   reportType,
		Status:       "pending",
		CreatedAt:    time.Now(),
	}
}

// ============================================================================
// Tests: NewEntityReportHandler
// ============================================================================

func TestNewEntityReportHandler(t *testing.T) {
	h := testEntityReportHandler()
	if h == nil {
		t.Fatal("expected non-nil EntityReportHandler")
	}
}

// ============================================================================
// Tests: Report Entity — Auth & Validation
// ============================================================================

func TestReportEntity_NoUser(t *testing.T) {
	h := testEntityReportHandler()
	_, err := h.ReportArtistHandler(context.Background(), &ReportEntityRequest{EntityID: "1"})
	assertHumaError(t, err, 401)
}

func TestReportEntity_InvalidEntityID(t *testing.T) {
	h := testEntityReportHandler()
	req := &ReportEntityRequest{EntityID: "abc"}
	req.Body.ReportType = "inaccurate"
	_, err := h.ReportArtistHandler(entityReportUserCtx(), req)
	assertHumaError(t, err, 400)
}

func TestReportEntity_EmptyReportType(t *testing.T) {
	h := testEntityReportHandler()
	req := &ReportEntityRequest{EntityID: "1"}
	req.Body.ReportType = ""
	_, err := h.ReportArtistHandler(entityReportUserCtx(), req)
	assertHumaError(t, err, 400)
}

func TestReportEntity_InvalidReportType(t *testing.T) {
	h := testEntityReportHandler()
	req := &ReportEntityRequest{EntityID: "1"}
	req.Body.ReportType = "cancelled" // not valid for artist
	_, err := h.ReportArtistHandler(entityReportUserCtx(), req)
	assertHumaError(t, err, 400)
}

// ============================================================================
// Tests: Report Entity — Success
// ============================================================================

func TestReportArtist_Success(t *testing.T) {
	expected := makeEntityReportResponse(1, "artist", "inaccurate")
	h := NewEntityReportHandler(
		&mockEntityReportService{
			createEntityReportFn: func(req *contracts.CreateEntityReportRequest) (*contracts.EntityReportResponse, error) {
				if req.EntityType != "artist" || req.EntityID != 10 || req.UserID != 2 {
					t.Errorf("unexpected params: %+v", req)
				}
				if req.ReportType != "inaccurate" {
					t.Errorf("expected report_type=inaccurate, got %s", req.ReportType)
				}
				return expected, nil
			},
		},
		nil,
	)

	req := &ReportEntityRequest{EntityID: "10"}
	req.Body.ReportType = "inaccurate"

	resp, err := h.ReportArtistHandler(entityReportUserCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.ID != 1 {
		t.Errorf("expected ID=1, got %d", resp.Body.ID)
	}
}

func TestReportVenue_Success(t *testing.T) {
	expected := makeEntityReportResponse(2, "venue", "closed_permanently")
	h := NewEntityReportHandler(
		&mockEntityReportService{
			createEntityReportFn: func(req *contracts.CreateEntityReportRequest) (*contracts.EntityReportResponse, error) {
				if req.EntityType != "venue" {
					t.Errorf("expected entity_type=venue, got %s", req.EntityType)
				}
				return expected, nil
			},
		},
		nil,
	)

	req := &ReportEntityRequest{EntityID: "10"}
	req.Body.ReportType = "closed_permanently"

	resp, err := h.ReportVenueHandler(entityReportUserCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.EntityType != "venue" {
		t.Errorf("expected entity_type=venue, got %s", resp.Body.EntityType)
	}
}

func TestReportFestival_Success(t *testing.T) {
	expected := makeEntityReportResponse(3, "festival", "cancelled")
	h := NewEntityReportHandler(
		&mockEntityReportService{
			createEntityReportFn: func(req *contracts.CreateEntityReportRequest) (*contracts.EntityReportResponse, error) {
				return expected, nil
			},
		},
		nil,
	)

	req := &ReportEntityRequest{EntityID: "10"}
	req.Body.ReportType = "cancelled"

	resp, err := h.ReportFestivalHandler(entityReportUserCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.EntityType != "festival" {
		t.Errorf("expected entity_type=festival, got %s", resp.Body.EntityType)
	}
}

func TestReportShow_Success(t *testing.T) {
	expected := makeEntityReportResponse(4, "show", "wrong_venue")
	h := NewEntityReportHandler(
		&mockEntityReportService{
			createEntityReportFn: func(req *contracts.CreateEntityReportRequest) (*contracts.EntityReportResponse, error) {
				return expected, nil
			},
		},
		nil,
	)

	req := &ReportEntityRequest{EntityID: "10"}
	req.Body.ReportType = "wrong_venue"

	resp, err := h.ReportShowHandler(entityReportUserCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.EntityType != "show" {
		t.Errorf("expected entity_type=show, got %s", resp.Body.EntityType)
	}
}

// ============================================================================
// Tests: Report Entity — Error Cases
// ============================================================================

func TestReportEntity_EntityNotFound(t *testing.T) {
	h := NewEntityReportHandler(
		&mockEntityReportService{
			createEntityReportFn: func(req *contracts.CreateEntityReportRequest) (*contracts.EntityReportResponse, error) {
				return nil, fmt.Errorf("entity not found: artist 99999")
			},
		},
		nil,
	)

	req := &ReportEntityRequest{EntityID: "99999"}
	req.Body.ReportType = "inaccurate"

	_, err := h.ReportArtistHandler(entityReportUserCtx(), req)
	assertHumaError(t, err, 404)
}

func TestReportEntity_DuplicatePending(t *testing.T) {
	h := NewEntityReportHandler(
		&mockEntityReportService{
			createEntityReportFn: func(req *contracts.CreateEntityReportRequest) (*contracts.EntityReportResponse, error) {
				return nil, fmt.Errorf("you already have a pending report for this entity")
			},
		},
		nil,
	)

	req := &ReportEntityRequest{EntityID: "10"}
	req.Body.ReportType = "inaccurate"

	_, err := h.ReportArtistHandler(entityReportUserCtx(), req)
	assertHumaError(t, err, 409)
}

func TestReportEntity_ServiceError(t *testing.T) {
	h := NewEntityReportHandler(
		&mockEntityReportService{
			createEntityReportFn: func(req *contracts.CreateEntityReportRequest) (*contracts.EntityReportResponse, error) {
				return nil, fmt.Errorf("database error")
			},
		},
		nil,
	)

	req := &ReportEntityRequest{EntityID: "10"}
	req.Body.ReportType = "inaccurate"

	_, err := h.ReportArtistHandler(entityReportUserCtx(), req)
	assertHumaError(t, err, 500)
}

// ============================================================================
// Tests: Admin — List Entity Reports
// ============================================================================

func TestAdminListEntityReports_RequiresAdmin(t *testing.T) {
	h := testEntityReportHandler()

	t.Run("NoUser", func(t *testing.T) {
		_, err := h.AdminListEntityReportsHandler(context.Background(), &AdminListEntityReportsRequest{})
		assertHumaError(t, err, 403)
	})
	t.Run("NonAdmin", func(t *testing.T) {
		_, err := h.AdminListEntityReportsHandler(entityReportUserCtx(), &AdminListEntityReportsRequest{})
		assertHumaError(t, err, 403)
	})
}

func TestAdminListEntityReports_Success(t *testing.T) {
	reports := []contracts.EntityReportResponse{*makeEntityReportResponse(1, "artist", "inaccurate")}
	h := NewEntityReportHandler(
		&mockEntityReportService{
			listEntityReportsFn: func(filters *contracts.EntityReportFilters) ([]contracts.EntityReportResponse, int64, error) {
				if filters.Status != "pending" {
					t.Errorf("expected status=pending, got %s", filters.Status)
				}
				return reports, 1, nil
			},
		},
		nil,
	)

	resp, err := h.AdminListEntityReportsHandler(entityReportAdminCtx(), &AdminListEntityReportsRequest{
		Status: "pending",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Total != 1 {
		t.Errorf("expected total=1, got %d", resp.Body.Total)
	}
	if len(resp.Body.Reports) != 1 {
		t.Errorf("expected 1 report, got %d", len(resp.Body.Reports))
	}
}

func TestAdminListEntityReports_WithEntityTypeFilter(t *testing.T) {
	reports := []contracts.EntityReportResponse{*makeEntityReportResponse(1, "venue", "wrong_address")}
	h := NewEntityReportHandler(
		&mockEntityReportService{
			listEntityReportsFn: func(filters *contracts.EntityReportFilters) ([]contracts.EntityReportResponse, int64, error) {
				if filters.EntityType != "venue" {
					t.Errorf("expected entity_type=venue, got %s", filters.EntityType)
				}
				return reports, 1, nil
			},
		},
		nil,
	)

	resp, err := h.AdminListEntityReportsHandler(entityReportAdminCtx(), &AdminListEntityReportsRequest{
		EntityType: "venue",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Total != 1 {
		t.Errorf("expected total=1, got %d", resp.Body.Total)
	}
}

// ============================================================================
// Tests: Admin — Get Single Entity Report
// ============================================================================

func TestAdminGetEntityReport_RequiresAdmin(t *testing.T) {
	h := testEntityReportHandler()
	_, err := h.AdminGetEntityReportHandler(entityReportUserCtx(), &AdminGetEntityReportRequest{ReportID: "1"})
	assertHumaError(t, err, 403)
}

func TestAdminGetEntityReport_InvalidID(t *testing.T) {
	h := testEntityReportHandler()
	_, err := h.AdminGetEntityReportHandler(entityReportAdminCtx(), &AdminGetEntityReportRequest{ReportID: "abc"})
	assertHumaError(t, err, 400)
}

func TestAdminGetEntityReport_NotFound(t *testing.T) {
	h := NewEntityReportHandler(
		&mockEntityReportService{
			getEntityReportFn: func(reportID uint) (*contracts.EntityReportResponse, error) {
				return nil, nil
			},
		},
		nil,
	)

	_, err := h.AdminGetEntityReportHandler(entityReportAdminCtx(), &AdminGetEntityReportRequest{ReportID: "99"})
	assertHumaError(t, err, 404)
}

func TestAdminGetEntityReport_Success(t *testing.T) {
	expected := makeEntityReportResponse(1, "artist", "inaccurate")
	h := NewEntityReportHandler(
		&mockEntityReportService{
			getEntityReportFn: func(reportID uint) (*contracts.EntityReportResponse, error) {
				if reportID != 1 {
					t.Errorf("expected reportID=1, got %d", reportID)
				}
				return expected, nil
			},
		},
		nil,
	)

	resp, err := h.AdminGetEntityReportHandler(entityReportAdminCtx(), &AdminGetEntityReportRequest{ReportID: "1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.ID != 1 {
		t.Errorf("expected ID=1, got %d", resp.Body.ID)
	}
}

// ============================================================================
// Tests: Admin — Resolve Entity Report
// ============================================================================

func TestAdminResolveEntityReport_RequiresAdmin(t *testing.T) {
	h := testEntityReportHandler()
	req := &AdminResolveEntityReportRequest{ReportID: "1"}
	_, err := h.AdminResolveEntityReportHandler(entityReportUserCtx(), req)
	assertHumaError(t, err, 403)
}

func TestAdminResolveEntityReport_InvalidID(t *testing.T) {
	h := testEntityReportHandler()
	_, err := h.AdminResolveEntityReportHandler(entityReportAdminCtx(), &AdminResolveEntityReportRequest{ReportID: "abc"})
	assertHumaError(t, err, 400)
}

func TestAdminResolveEntityReport_Success(t *testing.T) {
	resolved := makeEntityReportResponse(1, "artist", "inaccurate")
	resolved.Status = "resolved"
	reviewerID := uint(1)
	resolved.ReviewedBy = &reviewerID
	notes := "Fixed the issue"
	resolved.AdminNotes = &notes

	h := NewEntityReportHandler(
		&mockEntityReportService{
			resolveEntityReportFn: func(reportID, rID uint, n string) (*contracts.EntityReportResponse, error) {
				if reportID != 1 || rID != 1 {
					t.Errorf("unexpected params: reportID=%d, reviewerID=%d", reportID, rID)
				}
				if n != "Fixed the issue" {
					t.Errorf("unexpected notes: %s", n)
				}
				return resolved, nil
			},
		},
		&mockAuditLogService{},
	)

	req := &AdminResolveEntityReportRequest{ReportID: "1"}
	req.Body.Notes = "Fixed the issue"

	resp, err := h.AdminResolveEntityReportHandler(entityReportAdminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Status != "resolved" {
		t.Errorf("expected resolved status, got %s", resp.Body.Status)
	}
}

func TestAdminResolveEntityReport_NotFound(t *testing.T) {
	h := NewEntityReportHandler(
		&mockEntityReportService{
			resolveEntityReportFn: func(reportID, rID uint, n string) (*contracts.EntityReportResponse, error) {
				return nil, fmt.Errorf("report not found")
			},
		},
		nil,
	)

	_, err := h.AdminResolveEntityReportHandler(entityReportAdminCtx(), &AdminResolveEntityReportRequest{ReportID: "99"})
	assertHumaError(t, err, 404)
}

func TestAdminResolveEntityReport_AlreadyReviewed(t *testing.T) {
	h := NewEntityReportHandler(
		&mockEntityReportService{
			resolveEntityReportFn: func(reportID, rID uint, n string) (*contracts.EntityReportResponse, error) {
				return nil, fmt.Errorf("report has already been reviewed (status: resolved)")
			},
		},
		nil,
	)

	_, err := h.AdminResolveEntityReportHandler(entityReportAdminCtx(), &AdminResolveEntityReportRequest{ReportID: "1"})
	assertHumaError(t, err, 409)
}

// ============================================================================
// Tests: Admin — Dismiss Entity Report
// ============================================================================

func TestAdminDismissEntityReport_RequiresAdmin(t *testing.T) {
	h := testEntityReportHandler()
	req := &AdminDismissEntityReportRequest{ReportID: "1"}
	_, err := h.AdminDismissEntityReportHandler(entityReportUserCtx(), req)
	assertHumaError(t, err, 403)
}

func TestAdminDismissEntityReport_InvalidID(t *testing.T) {
	h := testEntityReportHandler()
	_, err := h.AdminDismissEntityReportHandler(entityReportAdminCtx(), &AdminDismissEntityReportRequest{ReportID: "abc"})
	assertHumaError(t, err, 400)
}

func TestAdminDismissEntityReport_Success(t *testing.T) {
	dismissed := makeEntityReportResponse(1, "venue", "wrong_address")
	dismissed.Status = "dismissed"
	reviewerID := uint(1)
	dismissed.ReviewedBy = &reviewerID
	notes := "Not valid"
	dismissed.AdminNotes = &notes

	h := NewEntityReportHandler(
		&mockEntityReportService{
			dismissEntityReportFn: func(reportID, rID uint, n string) (*contracts.EntityReportResponse, error) {
				if reportID != 1 || rID != 1 {
					t.Errorf("unexpected params: reportID=%d, reviewerID=%d", reportID, rID)
				}
				return dismissed, nil
			},
		},
		&mockAuditLogService{},
	)

	req := &AdminDismissEntityReportRequest{ReportID: "1"}
	req.Body.Notes = "Not valid"

	resp, err := h.AdminDismissEntityReportHandler(entityReportAdminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Status != "dismissed" {
		t.Errorf("expected dismissed status, got %s", resp.Body.Status)
	}
}

func TestAdminDismissEntityReport_NotFound(t *testing.T) {
	h := NewEntityReportHandler(
		&mockEntityReportService{
			dismissEntityReportFn: func(reportID, rID uint, n string) (*contracts.EntityReportResponse, error) {
				return nil, fmt.Errorf("report not found")
			},
		},
		nil,
	)

	_, err := h.AdminDismissEntityReportHandler(entityReportAdminCtx(), &AdminDismissEntityReportRequest{ReportID: "99"})
	assertHumaError(t, err, 404)
}

func TestAdminDismissEntityReport_AlreadyReviewed(t *testing.T) {
	h := NewEntityReportHandler(
		&mockEntityReportService{
			dismissEntityReportFn: func(reportID, rID uint, n string) (*contracts.EntityReportResponse, error) {
				return nil, fmt.Errorf("report has already been reviewed (status: dismissed)")
			},
		},
		nil,
	)

	_, err := h.AdminDismissEntityReportHandler(entityReportAdminCtx(), &AdminDismissEntityReportRequest{ReportID: "1"})
	assertHumaError(t, err, 409)
}
