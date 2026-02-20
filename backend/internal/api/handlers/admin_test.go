package handlers

import (
	"context"
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services"
)

func testAdminHandler() *AdminHandler {
	return NewAdminHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
}

func adminCtx() context.Context {
	return ctxWithUser(&models.User{ID: 1, IsAdmin: true})
}

// --- NewAdminHandler ---

func TestNewAdminHandler(t *testing.T) {
	h := testAdminHandler()
	if h == nil {
		t.Fatal("expected non-nil AdminHandler")
	}
}

// --- Admin Guard: all handlers require admin access ---
// Tests both nil-user and non-admin user scenarios for every admin handler.

func TestAdminHandler_RequiresAdmin(t *testing.T) {
	h := testAdminHandler()

	tests := []struct {
		name string
		fn   func(ctx context.Context) error
	}{
		{"GetPendingShows", func(ctx context.Context) error {
			_, err := h.GetPendingShowsHandler(ctx, &GetPendingShowsRequest{})
			return err
		}},
		{"GetRejectedShows", func(ctx context.Context) error {
			_, err := h.GetRejectedShowsHandler(ctx, &GetRejectedShowsRequest{})
			return err
		}},
		{"ApproveShow", func(ctx context.Context) error {
			_, err := h.ApproveShowHandler(ctx, &ApproveShowRequest{})
			return err
		}},
		{"RejectShow", func(ctx context.Context) error {
			_, err := h.RejectShowHandler(ctx, &RejectShowRequest{})
			return err
		}},
		{"VerifyVenue", func(ctx context.Context) error {
			_, err := h.VerifyVenueHandler(ctx, &VerifyVenueRequest{})
			return err
		}},
		{"GetUnverifiedVenues", func(ctx context.Context) error {
			_, err := h.GetUnverifiedVenuesHandler(ctx, &GetUnverifiedVenuesRequest{})
			return err
		}},
		{"GetPendingVenueEdits", func(ctx context.Context) error {
			_, err := h.GetPendingVenueEditsHandler(ctx, &GetPendingVenueEditsRequest{})
			return err
		}},
		{"ApproveVenueEdit", func(ctx context.Context) error {
			_, err := h.ApproveVenueEditHandler(ctx, &ApproveVenueEditRequest{})
			return err
		}},
		{"RejectVenueEdit", func(ctx context.Context) error {
			_, err := h.RejectVenueEditHandler(ctx, &RejectVenueEditRequest{})
			return err
		}},
		{"ImportShowPreview", func(ctx context.Context) error {
			_, err := h.ImportShowPreviewHandler(ctx, &ImportShowPreviewRequest{})
			return err
		}},
		{"ImportShowConfirm", func(ctx context.Context) error {
			_, err := h.ImportShowConfirmHandler(ctx, &ImportShowConfirmRequest{})
			return err
		}},
		{"GetAdminShows", func(ctx context.Context) error {
			_, err := h.GetAdminShowsHandler(ctx, &GetAdminShowsRequest{})
			return err
		}},
		{"BulkExportShows", func(ctx context.Context) error {
			_, err := h.BulkExportShowsHandler(ctx, &BulkExportShowsRequest{})
			return err
		}},
		{"BulkImportPreview", func(ctx context.Context) error {
			_, err := h.BulkImportPreviewHandler(ctx, &BulkImportPreviewRequest{})
			return err
		}},
		{"BulkImportConfirm", func(ctx context.Context) error {
			_, err := h.BulkImportConfirmHandler(ctx, &BulkImportConfirmRequest{})
			return err
		}},
		{"DiscoveryImport", func(ctx context.Context) error {
			_, err := h.DiscoveryImportHandler(ctx, &DiscoveryImportRequest{})
			return err
		}},
		{"DiscoveryCheck", func(ctx context.Context) error {
			_, err := h.DiscoveryCheckHandler(ctx, &DiscoveryCheckRequest{})
			return err
		}},
		{"CreateAPIToken", func(ctx context.Context) error {
			_, err := h.CreateAPITokenHandler(ctx, &CreateAPITokenRequest{})
			return err
		}},
		{"ListAPITokens", func(ctx context.Context) error {
			_, err := h.ListAPITokensHandler(ctx, &ListAPITokensRequest{})
			return err
		}},
		{"RevokeAPIToken", func(ctx context.Context) error {
			_, err := h.RevokeAPITokenHandler(ctx, &RevokeAPITokenRequest{})
			return err
		}},
		{"ExportShows", func(ctx context.Context) error {
			_, err := h.ExportShowsHandler(ctx, &ExportShowsRequest{})
			return err
		}},
		{"ExportArtists", func(ctx context.Context) error {
			_, err := h.ExportArtistsHandler(ctx, &ExportArtistsRequest{})
			return err
		}},
		{"ExportVenues", func(ctx context.Context) error {
			_, err := h.ExportVenuesHandler(ctx, &ExportVenuesRequest{})
			return err
		}},
		{"DataImport", func(ctx context.Context) error {
			_, err := h.DataImportHandler(ctx, &DataImportRequest{})
			return err
		}},
		{"GetAdminUsers", func(ctx context.Context) error {
			_, err := h.GetAdminUsersHandler(ctx, &GetAdminUsersRequest{})
			return err
		}},
		{"GetAdminStats", func(ctx context.Context) error {
			_, err := h.GetAdminStatsHandler(ctx, &GetAdminStatsRequest{})
			return err
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name+"_NoUser", func(t *testing.T) {
			err := tc.fn(context.Background())
			assertHumaError(t, err, 403)
		})
		t.Run(tc.name+"_NonAdmin", func(t *testing.T) {
			ctx := ctxWithUser(&models.User{ID: 1, IsAdmin: false})
			err := tc.fn(ctx)
			assertHumaError(t, err, 403)
		})
	}
}

// --- Specific validation tests (require admin context to pass the guard) ---

// ApproveShowHandler — invalid show ID
func TestApproveShowHandler_InvalidID(t *testing.T) {
	h := testAdminHandler()
	req := &ApproveShowRequest{ShowID: "abc"}

	_, err := h.ApproveShowHandler(adminCtx(), req)
	assertHumaError(t, err, 400)
}

// RejectShowHandler — invalid show ID
func TestRejectShowHandler_InvalidID(t *testing.T) {
	h := testAdminHandler()
	req := &RejectShowRequest{ShowID: "abc"}

	_, err := h.RejectShowHandler(adminCtx(), req)
	assertHumaError(t, err, 400)
}

// RejectShowHandler — empty reason
func TestRejectShowHandler_EmptyReason(t *testing.T) {
	h := testAdminHandler()
	req := &RejectShowRequest{ShowID: "1"}
	// Body.Reason is empty

	_, err := h.RejectShowHandler(adminCtx(), req)
	assertHumaError(t, err, 400)
}

// VerifyVenueHandler — invalid venue ID
func TestVerifyVenueHandler_InvalidID(t *testing.T) {
	h := testAdminHandler()
	req := &VerifyVenueRequest{VenueID: "abc"}

	_, err := h.VerifyVenueHandler(adminCtx(), req)
	assertHumaError(t, err, 400)
}

// ApproveVenueEditHandler — invalid edit ID
func TestApproveVenueEditHandler_InvalidID(t *testing.T) {
	h := testAdminHandler()
	req := &ApproveVenueEditRequest{EditID: "abc"}

	_, err := h.ApproveVenueEditHandler(adminCtx(), req)
	assertHumaError(t, err, 400)
}

// RejectVenueEditHandler — invalid edit ID
func TestRejectVenueEditHandler_InvalidID(t *testing.T) {
	h := testAdminHandler()
	req := &RejectVenueEditRequest{EditID: "abc"}

	_, err := h.RejectVenueEditHandler(adminCtx(), req)
	assertHumaError(t, err, 400)
}

// RejectVenueEditHandler — empty reason
func TestRejectVenueEditHandler_EmptyReason(t *testing.T) {
	h := testAdminHandler()
	req := &RejectVenueEditRequest{EditID: "1"}
	// Body.Reason is empty

	_, err := h.RejectVenueEditHandler(adminCtx(), req)
	assertHumaError(t, err, 400)
}

// ImportShowPreviewHandler — invalid base64
func TestImportShowPreviewHandler_InvalidBase64(t *testing.T) {
	h := testAdminHandler()
	req := &ImportShowPreviewRequest{}
	req.Body.Content = "not-valid-base64!!!"

	_, err := h.ImportShowPreviewHandler(adminCtx(), req)
	assertHumaError(t, err, 400)
}

// ImportShowConfirmHandler — invalid base64
func TestImportShowConfirmHandler_InvalidBase64(t *testing.T) {
	h := testAdminHandler()
	req := &ImportShowConfirmRequest{}
	req.Body.Content = "not-valid-base64!!!"

	_, err := h.ImportShowConfirmHandler(adminCtx(), req)
	assertHumaError(t, err, 400)
}

// BulkExportShowsHandler — empty show IDs
func TestBulkExportShowsHandler_EmptyIDs(t *testing.T) {
	h := testAdminHandler()
	req := &BulkExportShowsRequest{}
	// Body.ShowIDs is nil

	_, err := h.BulkExportShowsHandler(adminCtx(), req)
	assertHumaError(t, err, 400)
}

// BulkExportShowsHandler — too many show IDs
func TestBulkExportShowsHandler_TooMany(t *testing.T) {
	h := testAdminHandler()
	req := &BulkExportShowsRequest{}
	req.Body.ShowIDs = make([]uint, 51)

	_, err := h.BulkExportShowsHandler(adminCtx(), req)
	assertHumaError(t, err, 400)
}

// BulkImportPreviewHandler — empty shows
func TestBulkImportPreviewHandler_EmptyShows(t *testing.T) {
	h := testAdminHandler()
	req := &BulkImportPreviewRequest{}

	_, err := h.BulkImportPreviewHandler(adminCtx(), req)
	assertHumaError(t, err, 400)
}

// BulkImportPreviewHandler — too many shows
func TestBulkImportPreviewHandler_TooMany(t *testing.T) {
	h := testAdminHandler()
	req := &BulkImportPreviewRequest{}
	req.Body.Shows = make([]string, 51)

	_, err := h.BulkImportPreviewHandler(adminCtx(), req)
	assertHumaError(t, err, 400)
}

// BulkImportConfirmHandler — empty shows
func TestBulkImportConfirmHandler_EmptyShows(t *testing.T) {
	h := testAdminHandler()
	req := &BulkImportConfirmRequest{}

	_, err := h.BulkImportConfirmHandler(adminCtx(), req)
	assertHumaError(t, err, 400)
}

// BulkImportConfirmHandler — too many shows
func TestBulkImportConfirmHandler_TooMany(t *testing.T) {
	h := testAdminHandler()
	req := &BulkImportConfirmRequest{}
	req.Body.Shows = make([]string, 51)

	_, err := h.BulkImportConfirmHandler(adminCtx(), req)
	assertHumaError(t, err, 400)
}

// DiscoveryImportHandler — empty events
func TestDiscoveryImportHandler_EmptyEvents(t *testing.T) {
	h := testAdminHandler()
	req := &DiscoveryImportRequest{}

	_, err := h.DiscoveryImportHandler(adminCtx(), req)
	assertHumaError(t, err, 400)
}

// DiscoveryImportHandler — too many events
func TestDiscoveryImportHandler_TooMany(t *testing.T) {
	h := testAdminHandler()
	req := &DiscoveryImportRequest{}
	req.Body.Events = make([]DiscoveryImportEventInput, 101)

	_, err := h.DiscoveryImportHandler(adminCtx(), req)
	assertHumaError(t, err, 400)
}

// DiscoveryCheckHandler — empty events
func TestDiscoveryCheckHandler_EmptyEvents(t *testing.T) {
	h := testAdminHandler()
	req := &DiscoveryCheckRequest{}

	_, err := h.DiscoveryCheckHandler(adminCtx(), req)
	assertHumaError(t, err, 400)
}

// DiscoveryCheckHandler — too many events
func TestDiscoveryCheckHandler_TooMany(t *testing.T) {
	h := testAdminHandler()
	req := &DiscoveryCheckRequest{}
	req.Body.Events = make([]DiscoveryCheckEventInput, 201)

	_, err := h.DiscoveryCheckHandler(adminCtx(), req)
	assertHumaError(t, err, 400)
}

// CreateAPITokenHandler — expiration too long
func TestCreateAPITokenHandler_ExpirationTooLong(t *testing.T) {
	h := testAdminHandler()
	req := &CreateAPITokenRequest{}
	req.Body.ExpirationDays = 400

	_, err := h.CreateAPITokenHandler(adminCtx(), req)
	assertHumaError(t, err, 400)
}

// RevokeAPITokenHandler — invalid token ID
func TestRevokeAPITokenHandler_InvalidID(t *testing.T) {
	h := testAdminHandler()
	req := &RevokeAPITokenRequest{TokenID: "abc"}

	_, err := h.RevokeAPITokenHandler(adminCtx(), req)
	assertHumaError(t, err, 400)
}

// DataImportHandler — empty items
func TestDataImportHandler_EmptyItems(t *testing.T) {
	h := testAdminHandler()
	req := &DataImportRequest{}
	// All slices are nil/empty, totalItems == 0

	_, err := h.DataImportHandler(adminCtx(), req)
	assertHumaError(t, err, 400)
}

// DataImportHandler — too many items
func TestDataImportHandler_TooMany(t *testing.T) {
	h := testAdminHandler()
	req := &DataImportRequest{}
	req.Body.Shows = make([]services.ExportedShow, 501)

	_, err := h.DataImportHandler(adminCtx(), req)
	assertHumaError(t, err, 400)
}

// ExportShowsHandler — invalid date format
func TestExportShowsHandler_InvalidDate(t *testing.T) {
	h := testAdminHandler()
	req := &ExportShowsRequest{FromDate: "not-a-date"}

	_, err := h.ExportShowsHandler(adminCtx(), req)
	assertHumaError(t, err, 400)
}

// ============================================================================
// Mock-based tests: helper to build admin handler with specific mocks
// ============================================================================

func adminHandler(opts ...func(*AdminHandler)) *AdminHandler {
	h := &AdminHandler{
		discordService:        &mockDiscordService{},
		auditLogService:       &mockAuditLogService{},
		musicDiscoveryService: &mockMusicDiscoveryService{},
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// ============================================================================
// Mock-based tests: Simple read handlers (Success + ServiceError)
// ============================================================================

func TestGetPendingShowsHandler_Success(t *testing.T) {
	h := adminHandler(func(ah *AdminHandler) {
		ah.showService = &mockShowService{
			getPendingShowsFn: func(limit, offset int) ([]*services.ShowResponse, int64, error) {
				return []*services.ShowResponse{{ID: 1}}, 1, nil
			},
		}
	})
	resp, err := h.GetPendingShowsHandler(adminCtx(), &GetPendingShowsRequest{Limit: 50})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Total != 1 {
		t.Errorf("expected total=1, got %d", resp.Body.Total)
	}
}

func TestGetPendingShowsHandler_ServiceError(t *testing.T) {
	h := adminHandler(func(ah *AdminHandler) {
		ah.showService = &mockShowService{
			getPendingShowsFn: func(_, _ int) ([]*services.ShowResponse, int64, error) {
				return nil, 0, fmt.Errorf("db error")
			},
		}
	})
	_, err := h.GetPendingShowsHandler(adminCtx(), &GetPendingShowsRequest{Limit: 50})
	assertHumaError(t, err, 500)
}

func TestGetRejectedShowsHandler_Success(t *testing.T) {
	h := adminHandler(func(ah *AdminHandler) {
		ah.showService = &mockShowService{
			getRejectedShowsFn: func(limit, offset int, search string) ([]*services.ShowResponse, int64, error) {
				return []*services.ShowResponse{{ID: 1}}, 1, nil
			},
		}
	})
	resp, err := h.GetRejectedShowsHandler(adminCtx(), &GetRejectedShowsRequest{Limit: 50})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Total != 1 {
		t.Errorf("expected total=1, got %d", resp.Body.Total)
	}
}

func TestGetRejectedShowsHandler_ServiceError(t *testing.T) {
	h := adminHandler(func(ah *AdminHandler) {
		ah.showService = &mockShowService{
			getRejectedShowsFn: func(_, _ int, _ string) ([]*services.ShowResponse, int64, error) {
				return nil, 0, fmt.Errorf("db error")
			},
		}
	})
	_, err := h.GetRejectedShowsHandler(adminCtx(), &GetRejectedShowsRequest{Limit: 50})
	assertHumaError(t, err, 500)
}

func TestGetUnverifiedVenuesHandler_Success(t *testing.T) {
	h := adminHandler(func(ah *AdminHandler) {
		ah.venueService = &mockVenueService{
			getUnverifiedVenuesFn: func(limit, offset int) ([]*services.UnverifiedVenueResponse, int64, error) {
				return []*services.UnverifiedVenueResponse{{}}, 1, nil
			},
		}
	})
	resp, err := h.GetUnverifiedVenuesHandler(adminCtx(), &GetUnverifiedVenuesRequest{Limit: 50})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Total != 1 {
		t.Errorf("expected total=1, got %d", resp.Body.Total)
	}
}

func TestGetUnverifiedVenuesHandler_ServiceError(t *testing.T) {
	h := adminHandler(func(ah *AdminHandler) {
		ah.venueService = &mockVenueService{
			getUnverifiedVenuesFn: func(_, _ int) ([]*services.UnverifiedVenueResponse, int64, error) {
				return nil, 0, fmt.Errorf("db error")
			},
		}
	})
	_, err := h.GetUnverifiedVenuesHandler(adminCtx(), &GetUnverifiedVenuesRequest{Limit: 50})
	assertHumaError(t, err, 500)
}

func TestGetPendingVenueEditsHandler_Success(t *testing.T) {
	h := adminHandler(func(ah *AdminHandler) {
		ah.venueService = &mockVenueService{
			getPendingVenueEditsFn: func(limit, offset int) ([]*services.PendingVenueEditResponse, int64, error) {
				return []*services.PendingVenueEditResponse{{}}, 1, nil
			},
		}
	})
	resp, err := h.GetPendingVenueEditsHandler(adminCtx(), &GetPendingVenueEditsRequest{Limit: 50})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Total != 1 {
		t.Errorf("expected total=1, got %d", resp.Body.Total)
	}
}

func TestGetPendingVenueEditsHandler_ServiceError(t *testing.T) {
	h := adminHandler(func(ah *AdminHandler) {
		ah.venueService = &mockVenueService{
			getPendingVenueEditsFn: func(_, _ int) ([]*services.PendingVenueEditResponse, int64, error) {
				return nil, 0, fmt.Errorf("db error")
			},
		}
	})
	_, err := h.GetPendingVenueEditsHandler(adminCtx(), &GetPendingVenueEditsRequest{Limit: 50})
	assertHumaError(t, err, 500)
}

func TestGetAdminShowsHandler_Success(t *testing.T) {
	h := adminHandler(func(ah *AdminHandler) {
		ah.showService = &mockShowService{
			getAdminShowsFn: func(limit, offset int, filters services.AdminShowFilters) ([]*services.ShowResponse, int64, error) {
				return []*services.ShowResponse{{ID: 1}}, 1, nil
			},
		}
	})
	resp, err := h.GetAdminShowsHandler(adminCtx(), &GetAdminShowsRequest{Limit: 50})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Total != 1 {
		t.Errorf("expected total=1, got %d", resp.Body.Total)
	}
}

func TestGetAdminShowsHandler_ServiceError(t *testing.T) {
	h := adminHandler(func(ah *AdminHandler) {
		ah.showService = &mockShowService{
			getAdminShowsFn: func(_, _ int, _ services.AdminShowFilters) ([]*services.ShowResponse, int64, error) {
				return nil, 0, fmt.Errorf("db error")
			},
		}
	})
	_, err := h.GetAdminShowsHandler(adminCtx(), &GetAdminShowsRequest{Limit: 50})
	assertHumaError(t, err, 500)
}

func TestListAPITokensHandler_Success(t *testing.T) {
	h := adminHandler(func(ah *AdminHandler) {
		ah.apiTokenService = &mockAPITokenService{
			listTokensFn: func(userID uint) ([]services.APITokenResponse, error) {
				return []services.APITokenResponse{{ID: 1}}, nil
			},
		}
	})
	resp, err := h.ListAPITokensHandler(adminCtx(), &ListAPITokensRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Body.Tokens) != 1 {
		t.Errorf("expected 1 token, got %d", len(resp.Body.Tokens))
	}
}

func TestListAPITokensHandler_ServiceError(t *testing.T) {
	h := adminHandler(func(ah *AdminHandler) {
		ah.apiTokenService = &mockAPITokenService{
			listTokensFn: func(_ uint) ([]services.APITokenResponse, error) {
				return nil, fmt.Errorf("db error")
			},
		}
	})
	_, err := h.ListAPITokensHandler(adminCtx(), &ListAPITokensRequest{})
	assertHumaError(t, err, 500)
}

func TestExportShowsHandler_Success(t *testing.T) {
	h := adminHandler(func(ah *AdminHandler) {
		ah.dataSyncService = &mockDataSyncService{
			exportShowsFn: func(params services.ExportShowsParams) (*services.ExportShowsResult, error) {
				return &services.ExportShowsResult{Total: 5}, nil
			},
		}
	})
	resp, err := h.ExportShowsHandler(adminCtx(), &ExportShowsRequest{Limit: 50})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Total != 5 {
		t.Errorf("expected total=5, got %d", resp.Body.Total)
	}
}

func TestExportShowsHandler_ServiceError(t *testing.T) {
	h := adminHandler(func(ah *AdminHandler) {
		ah.dataSyncService = &mockDataSyncService{
			exportShowsFn: func(_ services.ExportShowsParams) (*services.ExportShowsResult, error) {
				return nil, fmt.Errorf("db error")
			},
		}
	})
	_, err := h.ExportShowsHandler(adminCtx(), &ExportShowsRequest{Limit: 50})
	assertHumaError(t, err, 500)
}

func TestExportArtistsHandler_Success(t *testing.T) {
	h := adminHandler(func(ah *AdminHandler) {
		ah.dataSyncService = &mockDataSyncService{
			exportArtistsFn: func(params services.ExportArtistsParams) (*services.ExportArtistsResult, error) {
				return &services.ExportArtistsResult{Total: 3}, nil
			},
		}
	})
	resp, err := h.ExportArtistsHandler(adminCtx(), &ExportArtistsRequest{Limit: 50})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Total != 3 {
		t.Errorf("expected total=3, got %d", resp.Body.Total)
	}
}

func TestExportArtistsHandler_ServiceError(t *testing.T) {
	h := adminHandler(func(ah *AdminHandler) {
		ah.dataSyncService = &mockDataSyncService{
			exportArtistsFn: func(_ services.ExportArtistsParams) (*services.ExportArtistsResult, error) {
				return nil, fmt.Errorf("db error")
			},
		}
	})
	_, err := h.ExportArtistsHandler(adminCtx(), &ExportArtistsRequest{Limit: 50})
	assertHumaError(t, err, 500)
}

func TestExportVenuesHandler_Success(t *testing.T) {
	h := adminHandler(func(ah *AdminHandler) {
		ah.dataSyncService = &mockDataSyncService{
			exportVenuesFn: func(params services.ExportVenuesParams) (*services.ExportVenuesResult, error) {
				return &services.ExportVenuesResult{Total: 2}, nil
			},
		}
	})
	resp, err := h.ExportVenuesHandler(adminCtx(), &ExportVenuesRequest{Limit: 50})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Total != 2 {
		t.Errorf("expected total=2, got %d", resp.Body.Total)
	}
}

func TestExportVenuesHandler_ServiceError(t *testing.T) {
	h := adminHandler(func(ah *AdminHandler) {
		ah.dataSyncService = &mockDataSyncService{
			exportVenuesFn: func(_ services.ExportVenuesParams) (*services.ExportVenuesResult, error) {
				return nil, fmt.Errorf("db error")
			},
		}
	})
	_, err := h.ExportVenuesHandler(adminCtx(), &ExportVenuesRequest{Limit: 50})
	assertHumaError(t, err, 500)
}

func TestGetAdminUsersHandler_Success(t *testing.T) {
	h := adminHandler(func(ah *AdminHandler) {
		ah.userService = &mockUserService{
			listUsersFn: func(limit, offset int, filters services.AdminUserFilters) ([]*services.AdminUserResponse, int64, error) {
				return []*services.AdminUserResponse{{}}, 1, nil
			},
		}
	})
	resp, err := h.GetAdminUsersHandler(adminCtx(), &GetAdminUsersRequest{Limit: 50})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Total != 1 {
		t.Errorf("expected total=1, got %d", resp.Body.Total)
	}
}

func TestGetAdminUsersHandler_ServiceError(t *testing.T) {
	h := adminHandler(func(ah *AdminHandler) {
		ah.userService = &mockUserService{
			listUsersFn: func(_, _ int, _ services.AdminUserFilters) ([]*services.AdminUserResponse, int64, error) {
				return nil, 0, fmt.Errorf("db error")
			},
		}
	})
	_, err := h.GetAdminUsersHandler(adminCtx(), &GetAdminUsersRequest{Limit: 50})
	assertHumaError(t, err, 500)
}

func TestGetAdminStatsHandler_Success(t *testing.T) {
	h := adminHandler(func(ah *AdminHandler) {
		ah.adminStatsService = &mockAdminStatsService{
			getDashboardStatsFn: func() (*services.AdminDashboardStats, error) {
				return &services.AdminDashboardStats{}, nil
			},
		}
	})
	_, err := h.GetAdminStatsHandler(adminCtx(), &GetAdminStatsRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetAdminStatsHandler_ServiceError(t *testing.T) {
	h := adminHandler(func(ah *AdminHandler) {
		ah.adminStatsService = &mockAdminStatsService{
			getDashboardStatsFn: func() (*services.AdminDashboardStats, error) {
				return nil, fmt.Errorf("db error")
			},
		}
	})
	_, err := h.GetAdminStatsHandler(adminCtx(), &GetAdminStatsRequest{})
	assertHumaError(t, err, 500)
}

// ============================================================================
// Mock-based tests: Write handlers with audit log
// ============================================================================

func TestApproveShowHandler_Success(t *testing.T) {
	var auditCalled bool
	h := adminHandler(func(ah *AdminHandler) {
		ah.showService = &mockShowService{
			approveShowFn: func(showID uint, verifyVenues bool) (*services.ShowResponse, error) {
				return &services.ShowResponse{ID: showID, Status: "approved"}, nil
			},
		}
		ah.auditLogService = &mockAuditLogService{
			logActionFn: func(actorID uint, action string, _ string, _ uint, _ map[string]interface{}) {
				auditCalled = true
				if action != "approve_show" {
					t.Errorf("expected action='approve_show', got %q", action)
				}
			},
		}
	})
	resp, err := h.ApproveShowHandler(adminCtx(), &ApproveShowRequest{ShowID: "42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Status != "approved" {
		t.Errorf("expected status='approved', got %q", resp.Body.Status)
	}
	if !auditCalled {
		t.Error("expected audit log to be called")
	}
}

func TestApproveShowHandler_ServiceError(t *testing.T) {
	h := adminHandler(func(ah *AdminHandler) {
		ah.showService = &mockShowService{
			approveShowFn: func(_ uint, _ bool) (*services.ShowResponse, error) {
				return nil, fmt.Errorf("not found")
			},
		}
	})
	_, err := h.ApproveShowHandler(adminCtx(), &ApproveShowRequest{ShowID: "42"})
	assertHumaError(t, err, 422)
}

func TestRejectShowHandler_Success(t *testing.T) {
	var auditCalled bool
	h := adminHandler(func(ah *AdminHandler) {
		ah.showService = &mockShowService{
			rejectShowFn: func(showID uint, reason string) (*services.ShowResponse, error) {
				if reason != "duplicate" {
					t.Errorf("expected reason='duplicate', got %q", reason)
				}
				return &services.ShowResponse{ID: showID, Status: "rejected"}, nil
			},
		}
		ah.auditLogService = &mockAuditLogService{
			logActionFn: func(_ uint, action string, _ string, _ uint, _ map[string]interface{}) {
				auditCalled = true
				if action != "reject_show" {
					t.Errorf("expected action='reject_show', got %q", action)
				}
			},
		}
	})
	req := &RejectShowRequest{ShowID: "42"}
	req.Body.Reason = "duplicate"
	resp, err := h.RejectShowHandler(adminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Status != "rejected" {
		t.Errorf("expected status='rejected', got %q", resp.Body.Status)
	}
	if !auditCalled {
		t.Error("expected audit log to be called")
	}
}

func TestRejectShowHandler_ServiceError(t *testing.T) {
	h := adminHandler(func(ah *AdminHandler) {
		ah.showService = &mockShowService{
			rejectShowFn: func(_ uint, _ string) (*services.ShowResponse, error) {
				return nil, fmt.Errorf("not found")
			},
		}
	})
	req := &RejectShowRequest{ShowID: "42"}
	req.Body.Reason = "spam"
	_, err := h.RejectShowHandler(adminCtx(), req)
	assertHumaError(t, err, 422)
}

func TestVerifyVenueHandler_Success(t *testing.T) {
	var auditCalled bool
	h := adminHandler(func(ah *AdminHandler) {
		ah.venueService = &mockVenueService{
			verifyVenueFn: func(venueID uint) (*services.VenueDetailResponse, error) {
				return &services.VenueDetailResponse{ID: venueID, Verified: true}, nil
			},
		}
		ah.auditLogService = &mockAuditLogService{
			logActionFn: func(_ uint, action string, _ string, _ uint, _ map[string]interface{}) {
				auditCalled = true
				if action != "verify_venue" {
					t.Errorf("expected action='verify_venue', got %q", action)
				}
			},
		}
	})
	resp, err := h.VerifyVenueHandler(adminCtx(), &VerifyVenueRequest{VenueID: "10"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Verified {
		t.Error("expected verified=true")
	}
	if !auditCalled {
		t.Error("expected audit log to be called")
	}
}

func TestVerifyVenueHandler_ServiceError(t *testing.T) {
	h := adminHandler(func(ah *AdminHandler) {
		ah.venueService = &mockVenueService{
			verifyVenueFn: func(_ uint) (*services.VenueDetailResponse, error) {
				return nil, fmt.Errorf("not found")
			},
		}
	})
	_, err := h.VerifyVenueHandler(adminCtx(), &VerifyVenueRequest{VenueID: "10"})
	assertHumaError(t, err, 422)
}

func TestApproveVenueEditHandler_Success(t *testing.T) {
	var auditCalled bool
	h := adminHandler(func(ah *AdminHandler) {
		ah.venueService = &mockVenueService{
			approveVenueEditFn: func(editID, adminID uint) (*services.VenueDetailResponse, error) {
				return &services.VenueDetailResponse{ID: 5}, nil
			},
		}
		ah.auditLogService = &mockAuditLogService{
			logActionFn: func(_ uint, action string, _ string, _ uint, _ map[string]interface{}) {
				auditCalled = true
				if action != "approve_venue_edit" {
					t.Errorf("expected action='approve_venue_edit', got %q", action)
				}
			},
		}
	})
	resp, err := h.ApproveVenueEditHandler(adminCtx(), &ApproveVenueEditRequest{EditID: "1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.ID != 5 {
		t.Errorf("expected venue ID=5, got %d", resp.Body.ID)
	}
	if !auditCalled {
		t.Error("expected audit log to be called")
	}
}

func TestApproveVenueEditHandler_ServiceError(t *testing.T) {
	h := adminHandler(func(ah *AdminHandler) {
		ah.venueService = &mockVenueService{
			approveVenueEditFn: func(_, _ uint) (*services.VenueDetailResponse, error) {
				return nil, fmt.Errorf("edit not found")
			},
		}
	})
	_, err := h.ApproveVenueEditHandler(adminCtx(), &ApproveVenueEditRequest{EditID: "1"})
	assertHumaError(t, err, 422)
}

func TestRejectVenueEditHandler_Success(t *testing.T) {
	var auditCalled bool
	h := adminHandler(func(ah *AdminHandler) {
		ah.venueService = &mockVenueService{
			rejectVenueEditFn: func(editID, adminID uint, reason string) (*services.PendingVenueEditResponse, error) {
				return &services.PendingVenueEditResponse{}, nil
			},
		}
		ah.auditLogService = &mockAuditLogService{
			logActionFn: func(_ uint, action string, _ string, _ uint, _ map[string]interface{}) {
				auditCalled = true
				if action != "reject_venue_edit" {
					t.Errorf("expected action='reject_venue_edit', got %q", action)
				}
			},
		}
	})
	req := &RejectVenueEditRequest{EditID: "1"}
	req.Body.Reason = "wrong info"
	_, err := h.RejectVenueEditHandler(adminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !auditCalled {
		t.Error("expected audit log to be called")
	}
}

func TestRejectVenueEditHandler_ServiceError(t *testing.T) {
	h := adminHandler(func(ah *AdminHandler) {
		ah.venueService = &mockVenueService{
			rejectVenueEditFn: func(_, _ uint, _ string) (*services.PendingVenueEditResponse, error) {
				return nil, fmt.Errorf("not found")
			},
		}
	})
	req := &RejectVenueEditRequest{EditID: "1"}
	req.Body.Reason = "wrong"
	_, err := h.RejectVenueEditHandler(adminCtx(), req)
	assertHumaError(t, err, 422)
}

func TestCreateAPITokenHandler_Success(t *testing.T) {
	h := adminHandler(func(ah *AdminHandler) {
		ah.apiTokenService = &mockAPITokenService{
			createTokenFn: func(userID uint, description *string, expirationDays int) (*services.APITokenCreateResponse, error) {
				return &services.APITokenCreateResponse{ID: 1, ExpiresAt: time.Now().Add(24 * time.Hour)}, nil
			},
		}
	})
	req := &CreateAPITokenRequest{}
	req.Body.ExpirationDays = 90
	resp, err := h.CreateAPITokenHandler(adminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.ID != 1 {
		t.Errorf("expected token ID=1, got %d", resp.Body.ID)
	}
}

func TestCreateAPITokenHandler_ServiceError(t *testing.T) {
	h := adminHandler(func(ah *AdminHandler) {
		ah.apiTokenService = &mockAPITokenService{
			createTokenFn: func(_ uint, _ *string, _ int) (*services.APITokenCreateResponse, error) {
				return nil, fmt.Errorf("db error")
			},
		}
	})
	req := &CreateAPITokenRequest{}
	req.Body.ExpirationDays = 90
	_, err := h.CreateAPITokenHandler(adminCtx(), req)
	assertHumaError(t, err, 500)
}

func TestRevokeAPITokenHandler_Success(t *testing.T) {
	h := adminHandler(func(ah *AdminHandler) {
		ah.apiTokenService = &mockAPITokenService{
			revokeTokenFn: func(userID, tokenID uint) error {
				return nil
			},
		}
	})
	resp, err := h.RevokeAPITokenHandler(adminCtx(), &RevokeAPITokenRequest{TokenID: "42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Message != "Token revoked successfully" {
		t.Errorf("unexpected message: %q", resp.Body.Message)
	}
}

func TestRevokeAPITokenHandler_ServiceError(t *testing.T) {
	h := adminHandler(func(ah *AdminHandler) {
		ah.apiTokenService = &mockAPITokenService{
			revokeTokenFn: func(_, _ uint) error {
				return fmt.Errorf("not found")
			},
		}
	})
	_, err := h.RevokeAPITokenHandler(adminCtx(), &RevokeAPITokenRequest{TokenID: "42"})
	assertHumaError(t, err, 404)
}

func TestDataImportHandler_Success(t *testing.T) {
	h := adminHandler(func(ah *AdminHandler) {
		ah.dataSyncService = &mockDataSyncService{
			importDataFn: func(req services.DataImportRequest) (*services.DataImportResult, error) {
				return &services.DataImportResult{}, nil
			},
		}
	})
	req := &DataImportRequest{}
	req.Body.Shows = []services.ExportedShow{{}}
	resp, err := h.DataImportHandler(adminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = resp // success
}

func TestDataImportHandler_ServiceError(t *testing.T) {
	h := adminHandler(func(ah *AdminHandler) {
		ah.dataSyncService = &mockDataSyncService{
			importDataFn: func(_ services.DataImportRequest) (*services.DataImportResult, error) {
				return nil, fmt.Errorf("import failed")
			},
		}
	})
	req := &DataImportRequest{}
	req.Body.Shows = []services.ExportedShow{{}}
	_, err := h.DataImportHandler(adminCtx(), req)
	assertHumaError(t, err, 500)
}

func TestDiscoveryImportHandler_Success(t *testing.T) {
	h := adminHandler(func(ah *AdminHandler) {
		ah.discoveryService = &mockDiscoveryService{
			importEventsFn: func(events []services.DiscoveredEvent, dryRun, allowUpdates bool) (*services.ImportResult, error) {
				return &services.ImportResult{Total: len(events), Imported: len(events)}, nil
			},
		}
	})
	req := &DiscoveryImportRequest{}
	req.Body.Events = []DiscoveryImportEventInput{{ID: "ev1", VenueSlug: "test"}}
	resp, err := h.DiscoveryImportHandler(adminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Imported != 1 {
		t.Errorf("expected imported=1, got %d", resp.Body.Imported)
	}
}

func TestDiscoveryImportHandler_ServiceError(t *testing.T) {
	h := adminHandler(func(ah *AdminHandler) {
		ah.discoveryService = &mockDiscoveryService{
			importEventsFn: func(_ []services.DiscoveredEvent, _, _ bool) (*services.ImportResult, error) {
				return nil, fmt.Errorf("import failed")
			},
		}
	})
	req := &DiscoveryImportRequest{}
	req.Body.Events = []DiscoveryImportEventInput{{ID: "ev1"}}
	_, err := h.DiscoveryImportHandler(adminCtx(), req)
	assertHumaError(t, err, 500)
}

func TestDiscoveryCheckHandler_Success(t *testing.T) {
	h := adminHandler(func(ah *AdminHandler) {
		ah.discoveryService = &mockDiscoveryService{
			checkEventsFn: func(events []services.CheckEventInput) (*services.CheckEventsResult, error) {
				return &services.CheckEventsResult{}, nil
			},
		}
	})
	req := &DiscoveryCheckRequest{}
	req.Body.Events = []DiscoveryCheckEventInput{{ID: "ev1", VenueSlug: "test"}}
	_, err := h.DiscoveryCheckHandler(adminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDiscoveryCheckHandler_ServiceError(t *testing.T) {
	h := adminHandler(func(ah *AdminHandler) {
		ah.discoveryService = &mockDiscoveryService{
			checkEventsFn: func(_ []services.CheckEventInput) (*services.CheckEventsResult, error) {
				return nil, fmt.Errorf("db error")
			},
		}
	})
	req := &DiscoveryCheckRequest{}
	req.Body.Events = []DiscoveryCheckEventInput{{ID: "ev1"}}
	_, err := h.DiscoveryCheckHandler(adminCtx(), req)
	assertHumaError(t, err, 500)
}

// ============================================================================
// Mock-based tests: Complex multi-service handlers
// ============================================================================

func TestImportShowPreviewHandler_Success(t *testing.T) {
	h := adminHandler(func(ah *AdminHandler) {
		ah.showService = &mockShowService{
			previewShowImportFn: func(content []byte) (*services.ImportPreviewResponse, error) {
				return &services.ImportPreviewResponse{CanImport: true}, nil
			},
		}
	})
	req := &ImportShowPreviewRequest{}
	req.Body.Content = base64.StdEncoding.EncodeToString([]byte("# Test Show"))
	resp, err := h.ImportShowPreviewHandler(adminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.CanImport {
		t.Error("expected can_import=true")
	}
}

func TestImportShowPreviewHandler_ServiceError(t *testing.T) {
	h := adminHandler(func(ah *AdminHandler) {
		ah.showService = &mockShowService{
			previewShowImportFn: func(_ []byte) (*services.ImportPreviewResponse, error) {
				return nil, fmt.Errorf("parse error")
			},
		}
	})
	req := &ImportShowPreviewRequest{}
	req.Body.Content = base64.StdEncoding.EncodeToString([]byte("bad"))
	_, err := h.ImportShowPreviewHandler(adminCtx(), req)
	assertHumaError(t, err, 422)
}

func TestImportShowConfirmHandler_Success(t *testing.T) {
	h := adminHandler(func(ah *AdminHandler) {
		ah.showService = &mockShowService{
			confirmShowImportFn: func(content []byte, verifyVenues bool) (*services.ShowResponse, error) {
				return &services.ShowResponse{ID: 100, Title: "Imported Show"}, nil
			},
		}
	})
	req := &ImportShowConfirmRequest{}
	req.Body.Content = base64.StdEncoding.EncodeToString([]byte("# Test Show"))
	resp, err := h.ImportShowConfirmHandler(adminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.ID != 100 {
		t.Errorf("expected ID=100, got %d", resp.Body.ID)
	}
}

func TestImportShowConfirmHandler_ServiceError(t *testing.T) {
	h := adminHandler(func(ah *AdminHandler) {
		ah.showService = &mockShowService{
			confirmShowImportFn: func(_ []byte, _ bool) (*services.ShowResponse, error) {
				return nil, fmt.Errorf("import failed")
			},
		}
	})
	req := &ImportShowConfirmRequest{}
	req.Body.Content = base64.StdEncoding.EncodeToString([]byte("bad"))
	_, err := h.ImportShowConfirmHandler(adminCtx(), req)
	assertHumaError(t, err, 422)
}

func TestBulkExportShowsHandler_Success(t *testing.T) {
	h := adminHandler(func(ah *AdminHandler) {
		ah.showService = &mockShowService{
			exportShowToMarkdownFn: func(showID uint) ([]byte, string, error) {
				return []byte("# Show"), "show.md", nil
			},
		}
	})
	req := &BulkExportShowsRequest{}
	req.Body.ShowIDs = []uint{1, 2}
	resp, err := h.BulkExportShowsHandler(adminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Body.Exports) != 2 {
		t.Errorf("expected 2 exports, got %d", len(resp.Body.Exports))
	}
}

func TestBulkExportShowsHandler_PartialFail(t *testing.T) {
	callCount := 0
	h := adminHandler(func(ah *AdminHandler) {
		ah.showService = &mockShowService{
			exportShowToMarkdownFn: func(showID uint) ([]byte, string, error) {
				callCount++
				if callCount == 2 {
					return nil, "", fmt.Errorf("not found")
				}
				return []byte("# Show"), "show.md", nil
			},
		}
	})
	req := &BulkExportShowsRequest{}
	req.Body.ShowIDs = []uint{1, 2, 3}
	_, err := h.BulkExportShowsHandler(adminCtx(), req)
	assertHumaError(t, err, 422)
}

func TestBulkImportPreviewHandler_Success(t *testing.T) {
	h := adminHandler(func(ah *AdminHandler) {
		ah.showService = &mockShowService{
			previewShowImportFn: func(_ []byte) (*services.ImportPreviewResponse, error) {
				return &services.ImportPreviewResponse{CanImport: true}, nil
			},
		}
	})
	req := &BulkImportPreviewRequest{}
	req.Body.Shows = []string{base64.StdEncoding.EncodeToString([]byte("# Show 1"))}
	resp, err := h.BulkImportPreviewHandler(adminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Body.Previews) != 1 {
		t.Errorf("expected 1 preview, got %d", len(resp.Body.Previews))
	}
	if !resp.Body.Summary.CanImportAll {
		t.Error("expected can_import_all=true")
	}
}

func TestBulkImportConfirmHandler_Success(t *testing.T) {
	h := adminHandler(func(ah *AdminHandler) {
		ah.showService = &mockShowService{
			confirmShowImportFn: func(_ []byte, _ bool) (*services.ShowResponse, error) {
				return &services.ShowResponse{ID: 1}, nil
			},
		}
	})
	req := &BulkImportConfirmRequest{}
	req.Body.Shows = []string{base64.StdEncoding.EncodeToString([]byte("# Show 1"))}
	resp, err := h.BulkImportConfirmHandler(adminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.SuccessCount != 1 {
		t.Errorf("expected success_count=1, got %d", resp.Body.SuccessCount)
	}
}

func TestBulkImportConfirmHandler_MixedResults(t *testing.T) {
	callCount := 0
	h := adminHandler(func(ah *AdminHandler) {
		ah.showService = &mockShowService{
			confirmShowImportFn: func(_ []byte, _ bool) (*services.ShowResponse, error) {
				callCount++
				if callCount == 2 {
					return nil, fmt.Errorf("import error")
				}
				return &services.ShowResponse{ID: uint(callCount)}, nil
			},
		}
	})
	req := &BulkImportConfirmRequest{}
	req.Body.Shows = []string{
		base64.StdEncoding.EncodeToString([]byte("# Show 1")),
		base64.StdEncoding.EncodeToString([]byte("# Show 2")),
		base64.StdEncoding.EncodeToString([]byte("# Show 3")),
	}
	resp, err := h.BulkImportConfirmHandler(adminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.SuccessCount != 2 {
		t.Errorf("expected success_count=2, got %d", resp.Body.SuccessCount)
	}
	if resp.Body.ErrorCount != 1 {
		t.Errorf("expected error_count=1, got %d", resp.Body.ErrorCount)
	}
}
