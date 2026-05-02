package admin

import (
	"context"
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	"psychic-homily-backend/internal/api/handlers/pipeline"
	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// ============================================================================
// Test helpers: create domain-specific handlers with nil services
// ============================================================================

func testAdminShowHandler() *AdminShowHandler {
	return NewAdminShowHandler(nil, nil, nil, nil, nil, nil, nil)
}

func testAdminDiscoveryHandler() *pipeline.AdminDiscoveryHandler {
	return pipeline.NewAdminDiscoveryHandler(nil)
}

func testAdminVenueHandler() *AdminVenueHandler {
	return NewAdminVenueHandler(nil, nil)
}

func testAdminTokenHandler() *AdminTokenHandler {
	return NewAdminTokenHandler(nil)
}

func testAdminDataHandler() *AdminDataHandler {
	return NewAdminDataHandler(nil)
}

func testAdminUserHandler() *AdminUserHandler {
	return NewAdminUserHandler(nil)
}

func testAdminStatsHandler() *AdminStatsHandler {
	return NewAdminStatsHandler(nil)
}

func adminCtx() context.Context {
	return testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
}

// ============================================================================
// Admin Guard: all handlers require admin access
// Tests both nil-user and non-admin user scenarios for every admin handler.
// ============================================================================

func TestAdminHandler_RequiresAdmin(t *testing.T) {
	showH := testAdminShowHandler()
	venueH := testAdminVenueHandler()
	tokenH := testAdminTokenHandler()
	dataH := testAdminDataHandler()
	userH := testAdminUserHandler()
	statsH := testAdminStatsHandler()
	discoveryH := testAdminDiscoveryHandler()

	tests := []struct {
		name string
		fn   func(ctx context.Context) error
	}{
		{"GetPendingShows", func(ctx context.Context) error {
			_, err := showH.GetPendingShowsHandler(ctx, &GetPendingShowsRequest{})
			return err
		}},
		{"GetRejectedShows", func(ctx context.Context) error {
			_, err := showH.GetRejectedShowsHandler(ctx, &GetRejectedShowsRequest{})
			return err
		}},
		{"ApproveShow", func(ctx context.Context) error {
			_, err := showH.ApproveShowHandler(ctx, &ApproveShowRequest{})
			return err
		}},
		{"RejectShow", func(ctx context.Context) error {
			_, err := showH.RejectShowHandler(ctx, &RejectShowRequest{})
			return err
		}},
		{"VerifyVenue", func(ctx context.Context) error {
			_, err := venueH.VerifyVenueHandler(ctx, &VerifyVenueRequest{})
			return err
		}},
		{"GetUnverifiedVenues", func(ctx context.Context) error {
			_, err := venueH.GetUnverifiedVenuesHandler(ctx, &GetUnverifiedVenuesRequest{})
			return err
		}},
		{"ImportShowPreview", func(ctx context.Context) error {
			_, err := showH.ImportShowPreviewHandler(ctx, &ImportShowPreviewRequest{})
			return err
		}},
		{"ImportShowConfirm", func(ctx context.Context) error {
			_, err := showH.ImportShowConfirmHandler(ctx, &ImportShowConfirmRequest{})
			return err
		}},
		{"GetAdminShows", func(ctx context.Context) error {
			_, err := showH.GetAdminShowsHandler(ctx, &GetAdminShowsRequest{})
			return err
		}},
		{"BulkExportShows", func(ctx context.Context) error {
			_, err := showH.BulkExportShowsHandler(ctx, &BulkExportShowsRequest{})
			return err
		}},
		{"BulkImportPreview", func(ctx context.Context) error {
			_, err := showH.BulkImportPreviewHandler(ctx, &BulkImportPreviewRequest{})
			return err
		}},
		{"BulkImportConfirm", func(ctx context.Context) error {
			_, err := showH.BulkImportConfirmHandler(ctx, &BulkImportConfirmRequest{})
			return err
		}},
		{"DiscoveryImport", func(ctx context.Context) error {
			_, err := discoveryH.DiscoveryImportHandler(ctx, &pipeline.DiscoveryImportRequest{})
			return err
		}},
		{"DiscoveryCheck", func(ctx context.Context) error {
			_, err := discoveryH.DiscoveryCheckHandler(ctx, &pipeline.DiscoveryCheckRequest{})
			return err
		}},
		{"CreateAPIToken", func(ctx context.Context) error {
			_, err := tokenH.CreateAPITokenHandler(ctx, &CreateAPITokenRequest{})
			return err
		}},
		{"ListAPITokens", func(ctx context.Context) error {
			_, err := tokenH.ListAPITokensHandler(ctx, &ListAPITokensRequest{})
			return err
		}},
		{"RevokeAPIToken", func(ctx context.Context) error {
			_, err := tokenH.RevokeAPITokenHandler(ctx, &RevokeAPITokenRequest{})
			return err
		}},
		{"ExportShows", func(ctx context.Context) error {
			_, err := dataH.ExportShowsHandler(ctx, &ExportShowsRequest{})
			return err
		}},
		{"ExportArtists", func(ctx context.Context) error {
			_, err := dataH.ExportArtistsHandler(ctx, &ExportArtistsRequest{})
			return err
		}},
		{"ExportVenues", func(ctx context.Context) error {
			_, err := dataH.ExportVenuesHandler(ctx, &ExportVenuesRequest{})
			return err
		}},
		{"DataImport", func(ctx context.Context) error {
			_, err := dataH.DataImportHandler(ctx, &DataImportRequest{})
			return err
		}},
		{"GetAdminUsers", func(ctx context.Context) error {
			_, err := userH.GetAdminUsersHandler(ctx, &GetAdminUsersRequest{})
			return err
		}},
		{"GetAdminStats", func(ctx context.Context) error {
			_, err := statsH.GetAdminStatsHandler(ctx, &GetAdminStatsRequest{})
			return err
		}},
		{"BatchApproveShows", func(ctx context.Context) error {
			_, err := showH.BatchApproveShowsHandler(ctx, &BatchApproveShowsRequest{})
			return err
		}},
		{"BatchRejectShows", func(ctx context.Context) error {
			_, err := showH.BatchRejectShowsHandler(ctx, &BatchRejectShowsRequest{})
			return err
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name+"_NoUser", func(t *testing.T) {
			err := tc.fn(context.Background())
			testhelpers.AssertHumaError(t, err, 403)
		})
		t.Run(tc.name+"_NonAdmin", func(t *testing.T) {
			ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: false})
			err := tc.fn(ctx)
			testhelpers.AssertHumaError(t, err, 403)
		})
	}
}

// ============================================================================
// Specific validation tests (require admin context to pass the guard)
// ============================================================================

// ApproveShowHandler — invalid show ID
func TestApproveShowHandler_InvalidID(t *testing.T) {
	h := testAdminShowHandler()
	req := &ApproveShowRequest{ShowID: "abc"}

	_, err := h.ApproveShowHandler(adminCtx(), req)
	testhelpers.AssertHumaError(t, err, 400)
}

// RejectShowHandler — invalid show ID
func TestRejectShowHandler_InvalidID(t *testing.T) {
	h := testAdminShowHandler()
	req := &RejectShowRequest{ShowID: "abc"}

	_, err := h.RejectShowHandler(adminCtx(), req)
	testhelpers.AssertHumaError(t, err, 400)
}

// RejectShowHandler — empty reason
func TestRejectShowHandler_EmptyReason(t *testing.T) {
	h := testAdminShowHandler()
	req := &RejectShowRequest{ShowID: "1"}
	// Body.Reason is empty

	_, err := h.RejectShowHandler(adminCtx(), req)
	testhelpers.AssertHumaError(t, err, 400)
}

// VerifyVenueHandler — invalid venue ID
func TestVerifyVenueHandler_InvalidID(t *testing.T) {
	h := testAdminVenueHandler()
	req := &VerifyVenueRequest{VenueID: "abc"}

	_, err := h.VerifyVenueHandler(adminCtx(), req)
	testhelpers.AssertHumaError(t, err, 400)
}

// ImportShowPreviewHandler — invalid base64
func TestImportShowPreviewHandler_InvalidBase64(t *testing.T) {
	h := testAdminShowHandler()
	req := &ImportShowPreviewRequest{}
	req.Body.Content = "not-valid-base64!!!"

	_, err := h.ImportShowPreviewHandler(adminCtx(), req)
	testhelpers.AssertHumaError(t, err, 400)
}

// ImportShowConfirmHandler — invalid base64
func TestImportShowConfirmHandler_InvalidBase64(t *testing.T) {
	h := testAdminShowHandler()
	req := &ImportShowConfirmRequest{}
	req.Body.Content = "not-valid-base64!!!"

	_, err := h.ImportShowConfirmHandler(adminCtx(), req)
	testhelpers.AssertHumaError(t, err, 400)
}

// BulkExportShowsHandler — empty show IDs
func TestBulkExportShowsHandler_EmptyIDs(t *testing.T) {
	h := testAdminShowHandler()
	req := &BulkExportShowsRequest{}
	// Body.ShowIDs is nil

	_, err := h.BulkExportShowsHandler(adminCtx(), req)
	testhelpers.AssertHumaError(t, err, 400)
}

// BulkExportShowsHandler — too many show IDs
func TestBulkExportShowsHandler_TooMany(t *testing.T) {
	h := testAdminShowHandler()
	req := &BulkExportShowsRequest{}
	req.Body.ShowIDs = make([]uint, 51)

	_, err := h.BulkExportShowsHandler(adminCtx(), req)
	testhelpers.AssertHumaError(t, err, 400)
}

// BulkImportPreviewHandler — empty shows
func TestBulkImportPreviewHandler_EmptyShows(t *testing.T) {
	h := testAdminShowHandler()
	req := &BulkImportPreviewRequest{}

	_, err := h.BulkImportPreviewHandler(adminCtx(), req)
	testhelpers.AssertHumaError(t, err, 400)
}

// BulkImportPreviewHandler — too many shows
func TestBulkImportPreviewHandler_TooMany(t *testing.T) {
	h := testAdminShowHandler()
	req := &BulkImportPreviewRequest{}
	req.Body.Shows = make([]string, 51)

	_, err := h.BulkImportPreviewHandler(adminCtx(), req)
	testhelpers.AssertHumaError(t, err, 400)
}

// BulkImportConfirmHandler — empty shows
func TestBulkImportConfirmHandler_EmptyShows(t *testing.T) {
	h := testAdminShowHandler()
	req := &BulkImportConfirmRequest{}

	_, err := h.BulkImportConfirmHandler(adminCtx(), req)
	testhelpers.AssertHumaError(t, err, 400)
}

// BulkImportConfirmHandler — too many shows
func TestBulkImportConfirmHandler_TooMany(t *testing.T) {
	h := testAdminShowHandler()
	req := &BulkImportConfirmRequest{}
	req.Body.Shows = make([]string, 51)

	_, err := h.BulkImportConfirmHandler(adminCtx(), req)
	testhelpers.AssertHumaError(t, err, 400)
}

// Discovery handler tests live in pipeline/admin_discovery_test.go; the
// admin-guard checks for DiscoveryImport/DiscoveryCheck remain here in the
// TestAdminHandler_RequiresAdmin sweep above, since the admin gate is the
// shared concern.

// CreateAPITokenHandler — expiration too long
func TestCreateAPITokenHandler_ExpirationTooLong(t *testing.T) {
	h := testAdminTokenHandler()
	req := &CreateAPITokenRequest{}
	req.Body.ExpirationDays = 400

	_, err := h.CreateAPITokenHandler(adminCtx(), req)
	testhelpers.AssertHumaError(t, err, 400)
}

// RevokeAPITokenHandler — invalid token ID
func TestRevokeAPITokenHandler_InvalidID(t *testing.T) {
	h := testAdminTokenHandler()
	req := &RevokeAPITokenRequest{TokenID: "abc"}

	_, err := h.RevokeAPITokenHandler(adminCtx(), req)
	testhelpers.AssertHumaError(t, err, 400)
}

// DataImportHandler — empty items
func TestDataImportHandler_EmptyItems(t *testing.T) {
	h := testAdminDataHandler()
	req := &DataImportRequest{}
	// All slices are nil/empty, totalItems == 0

	_, err := h.DataImportHandler(adminCtx(), req)
	testhelpers.AssertHumaError(t, err, 400)
}

// DataImportHandler — too many items
func TestDataImportHandler_TooMany(t *testing.T) {
	h := testAdminDataHandler()
	req := &DataImportRequest{}
	req.Body.Shows = make([]contracts.ExportedShow, 501)

	_, err := h.DataImportHandler(adminCtx(), req)
	testhelpers.AssertHumaError(t, err, 400)
}

// ExportShowsHandler — invalid date format
func TestExportShowsHandler_InvalidDate(t *testing.T) {
	h := testAdminDataHandler()
	req := &ExportShowsRequest{FromDate: "not-a-date"}

	_, err := h.ExportShowsHandler(adminCtx(), req)
	testhelpers.AssertHumaError(t, err, 400)
}

// ============================================================================
// Mock-based test helpers: build domain-specific handlers with mocks
// ============================================================================

func adminShowHandler(opts ...func(*AdminShowHandler)) *AdminShowHandler {
	h := &AdminShowHandler{
		discordService:        &testhelpers.MockDiscordService{},
		auditLogService:       &testhelpers.MockAuditLogService{},
		musicDiscoveryService: &testhelpers.MockMusicDiscoveryService{},
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// AdminDiscoveryHandler factory + Mock-driven tests live in
// pipeline/admin_discovery_test.go (PSY-420 sub-package split). Constructing
// pipeline.AdminDiscoveryHandler with mocked services requires assigning to
// its unexported `discoveryService` field, which is only legal from inside
// the pipeline package.

func adminVenueHandler(opts ...func(*AdminVenueHandler)) *AdminVenueHandler {
	h := &AdminVenueHandler{
		auditLogService: &testhelpers.MockAuditLogService{},
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

func adminTokenHandler(opts ...func(*AdminTokenHandler)) *AdminTokenHandler {
	h := &AdminTokenHandler{}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

func adminDataHandler(opts ...func(*AdminDataHandler)) *AdminDataHandler {
	h := &AdminDataHandler{}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

func adminUserHandler(opts ...func(*AdminUserHandler)) *AdminUserHandler {
	h := &AdminUserHandler{}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

func adminStatsHandler(opts ...func(*AdminStatsHandler)) *AdminStatsHandler {
	h := &AdminStatsHandler{}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// ============================================================================
// Mock-based tests: Simple read handlers (Success + ServiceError)
// ============================================================================

func TestGetPendingShowsHandler_Success(t *testing.T) {
	h := adminShowHandler(func(ah *AdminShowHandler) {
		ah.showAdminService = &testhelpers.MockShowAdminService{
			GetPendingShowsFn: func(limit, offset int, filters *contracts.PendingShowsFilter) ([]*contracts.ShowResponse, int64, error) {
				if limit != 50 {
					t.Errorf("expected limit=50, got %d", limit)
				}
				if offset != 0 {
					t.Errorf("expected offset=0, got %d", offset)
				}
				if filters != nil {
					t.Errorf("expected nil filters, got %v", filters)
				}
				return []*contracts.ShowResponse{{ID: 1}}, 1, nil
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
	h := adminShowHandler(func(ah *AdminShowHandler) {
		ah.showAdminService = &testhelpers.MockShowAdminService{
			GetPendingShowsFn: func(_, _ int, _ *contracts.PendingShowsFilter) ([]*contracts.ShowResponse, int64, error) {
				return nil, 0, fmt.Errorf("db error")
			},
		}
	})
	_, err := h.GetPendingShowsHandler(adminCtx(), &GetPendingShowsRequest{Limit: 50})
	testhelpers.AssertHumaError(t, err, 500)
}

func TestGetRejectedShowsHandler_Success(t *testing.T) {
	h := adminShowHandler(func(ah *AdminShowHandler) {
		ah.showAdminService = &testhelpers.MockShowAdminService{
			GetRejectedShowsFn: func(limit, offset int, search string) ([]*contracts.ShowResponse, int64, error) {
				if limit != 50 {
					t.Errorf("expected limit=50, got %d", limit)
				}
				if offset != 0 {
					t.Errorf("expected offset=0, got %d", offset)
				}
				return []*contracts.ShowResponse{{ID: 1}}, 1, nil
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
	h := adminShowHandler(func(ah *AdminShowHandler) {
		ah.showAdminService = &testhelpers.MockShowAdminService{
			GetRejectedShowsFn: func(_, _ int, _ string) ([]*contracts.ShowResponse, int64, error) {
				return nil, 0, fmt.Errorf("db error")
			},
		}
	})
	_, err := h.GetRejectedShowsHandler(adminCtx(), &GetRejectedShowsRequest{Limit: 50})
	testhelpers.AssertHumaError(t, err, 500)
}

func TestGetUnverifiedVenuesHandler_Success(t *testing.T) {
	h := adminVenueHandler(func(ah *AdminVenueHandler) {
		ah.venueService = &testhelpers.MockVenueService{
			GetUnverifiedVenuesFn: func(limit, offset int) ([]*contracts.UnverifiedVenueResponse, int64, error) {
				if limit != 50 {
					t.Errorf("expected limit=50, got %d", limit)
				}
				if offset != 0 {
					t.Errorf("expected offset=0, got %d", offset)
				}
				return []*contracts.UnverifiedVenueResponse{{}}, 1, nil
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
	h := adminVenueHandler(func(ah *AdminVenueHandler) {
		ah.venueService = &testhelpers.MockVenueService{
			GetUnverifiedVenuesFn: func(_, _ int) ([]*contracts.UnverifiedVenueResponse, int64, error) {
				return nil, 0, fmt.Errorf("db error")
			},
		}
	})
	_, err := h.GetUnverifiedVenuesHandler(adminCtx(), &GetUnverifiedVenuesRequest{Limit: 50})
	testhelpers.AssertHumaError(t, err, 500)
}

func TestGetAdminShowsHandler_Success(t *testing.T) {
	h := adminShowHandler(func(ah *AdminShowHandler) {
		ah.showAdminService = &testhelpers.MockShowAdminService{
			GetAdminShowsFn: func(limit, offset int, filters contracts.AdminShowFilters) ([]*contracts.ShowResponse, int64, error) {
				if limit != 50 {
					t.Errorf("expected limit=50, got %d", limit)
				}
				if offset != 0 {
					t.Errorf("expected offset=0, got %d", offset)
				}
				return []*contracts.ShowResponse{{ID: 1}}, 1, nil
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
	h := adminShowHandler(func(ah *AdminShowHandler) {
		ah.showAdminService = &testhelpers.MockShowAdminService{
			GetAdminShowsFn: func(_, _ int, _ contracts.AdminShowFilters) ([]*contracts.ShowResponse, int64, error) {
				return nil, 0, fmt.Errorf("db error")
			},
		}
	})
	_, err := h.GetAdminShowsHandler(adminCtx(), &GetAdminShowsRequest{Limit: 50})
	testhelpers.AssertHumaError(t, err, 500)
}

func TestListAPITokensHandler_Success(t *testing.T) {
	h := adminTokenHandler(func(ah *AdminTokenHandler) {
		ah.apiTokenService = &testhelpers.MockAPITokenService{
			ListTokensFn: func(userID uint) ([]contracts.APITokenResponse, error) {
				if userID != 1 {
					t.Errorf("expected userID=1, got %d", userID)
				}
				return []contracts.APITokenResponse{{ID: 1}}, nil
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
	h := adminTokenHandler(func(ah *AdminTokenHandler) {
		ah.apiTokenService = &testhelpers.MockAPITokenService{
			ListTokensFn: func(_ uint) ([]contracts.APITokenResponse, error) {
				return nil, fmt.Errorf("db error")
			},
		}
	})
	_, err := h.ListAPITokensHandler(adminCtx(), &ListAPITokensRequest{})
	testhelpers.AssertHumaError(t, err, 500)
}

func TestExportShowsHandler_Success(t *testing.T) {
	h := adminDataHandler(func(ah *AdminDataHandler) {
		ah.dataSyncService = &testhelpers.MockDataSyncService{
			ExportShowsFn: func(params contracts.ExportShowsParams) (*contracts.ExportShowsResult, error) {
				if params.Limit != 50 {
					t.Errorf("expected params.Limit=50, got %d", params.Limit)
				}
				return &contracts.ExportShowsResult{Total: 5}, nil
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
	h := adminDataHandler(func(ah *AdminDataHandler) {
		ah.dataSyncService = &testhelpers.MockDataSyncService{
			ExportShowsFn: func(_ contracts.ExportShowsParams) (*contracts.ExportShowsResult, error) {
				return nil, fmt.Errorf("db error")
			},
		}
	})
	_, err := h.ExportShowsHandler(adminCtx(), &ExportShowsRequest{Limit: 50})
	testhelpers.AssertHumaError(t, err, 500)
}

func TestExportArtistsHandler_Success(t *testing.T) {
	h := adminDataHandler(func(ah *AdminDataHandler) {
		ah.dataSyncService = &testhelpers.MockDataSyncService{
			ExportArtistsFn: func(params contracts.ExportArtistsParams) (*contracts.ExportArtistsResult, error) {
				return &contracts.ExportArtistsResult{Total: 3}, nil
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
	h := adminDataHandler(func(ah *AdminDataHandler) {
		ah.dataSyncService = &testhelpers.MockDataSyncService{
			ExportArtistsFn: func(_ contracts.ExportArtistsParams) (*contracts.ExportArtistsResult, error) {
				return nil, fmt.Errorf("db error")
			},
		}
	})
	_, err := h.ExportArtistsHandler(adminCtx(), &ExportArtistsRequest{Limit: 50})
	testhelpers.AssertHumaError(t, err, 500)
}

func TestExportVenuesHandler_Success(t *testing.T) {
	h := adminDataHandler(func(ah *AdminDataHandler) {
		ah.dataSyncService = &testhelpers.MockDataSyncService{
			ExportVenuesFn: func(params contracts.ExportVenuesParams) (*contracts.ExportVenuesResult, error) {
				return &contracts.ExportVenuesResult{Total: 2}, nil
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
	h := adminDataHandler(func(ah *AdminDataHandler) {
		ah.dataSyncService = &testhelpers.MockDataSyncService{
			ExportVenuesFn: func(_ contracts.ExportVenuesParams) (*contracts.ExportVenuesResult, error) {
				return nil, fmt.Errorf("db error")
			},
		}
	})
	_, err := h.ExportVenuesHandler(adminCtx(), &ExportVenuesRequest{Limit: 50})
	testhelpers.AssertHumaError(t, err, 500)
}

func TestGetAdminUsersHandler_Success(t *testing.T) {
	h := adminUserHandler(func(ah *AdminUserHandler) {
		ah.userService = &testhelpers.MockUserService{
			ListUsersFn: func(limit, offset int, filters contracts.AdminUserFilters) ([]*contracts.AdminUserResponse, int64, error) {
				if limit != 50 {
					t.Errorf("expected limit=50, got %d", limit)
				}
				if offset != 0 {
					t.Errorf("expected offset=0, got %d", offset)
				}
				return []*contracts.AdminUserResponse{{}}, 1, nil
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
	h := adminUserHandler(func(ah *AdminUserHandler) {
		ah.userService = &testhelpers.MockUserService{
			ListUsersFn: func(_, _ int, _ contracts.AdminUserFilters) ([]*contracts.AdminUserResponse, int64, error) {
				return nil, 0, fmt.Errorf("db error")
			},
		}
	})
	_, err := h.GetAdminUsersHandler(adminCtx(), &GetAdminUsersRequest{Limit: 50})
	testhelpers.AssertHumaError(t, err, 500)
}

func TestGetAdminStatsHandler_Success(t *testing.T) {
	h := adminStatsHandler(func(ah *AdminStatsHandler) {
		ah.adminStatsService = &testhelpers.MockAdminStatsService{
			GetDashboardStatsFn: func() (*contracts.AdminDashboardStats, error) {
				return &contracts.AdminDashboardStats{
					PendingShows: 5,
					TotalShows:   42,
					TotalArtists: 100,
				}, nil
			},
		}
	})
	resp, err := h.GetAdminStatsHandler(adminCtx(), &GetAdminStatsRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.PendingShows != 5 {
		t.Errorf("expected PendingShows=5, got %d", resp.Body.PendingShows)
	}
	if resp.Body.TotalShows != 42 {
		t.Errorf("expected TotalShows=42, got %d", resp.Body.TotalShows)
	}
	if resp.Body.TotalArtists != 100 {
		t.Errorf("expected TotalArtists=100, got %d", resp.Body.TotalArtists)
	}
}

func TestGetAdminStatsHandler_ServiceError(t *testing.T) {
	h := adminStatsHandler(func(ah *AdminStatsHandler) {
		ah.adminStatsService = &testhelpers.MockAdminStatsService{
			GetDashboardStatsFn: func() (*contracts.AdminDashboardStats, error) {
				return nil, fmt.Errorf("db error")
			},
		}
	})
	_, err := h.GetAdminStatsHandler(adminCtx(), &GetAdminStatsRequest{})
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// Mock-based tests: Write handlers with audit log
// ============================================================================

func TestApproveShowHandler_Success(t *testing.T) {
	var auditCalled bool
	h := adminShowHandler(func(ah *AdminShowHandler) {
		ah.showAdminService = &testhelpers.MockShowAdminService{
			ApproveShowFn: func(showID uint, verifyVenues bool) (*contracts.ShowResponse, error) {
				if showID != 42 {
					t.Errorf("expected showID=42, got %d", showID)
				}
				return &contracts.ShowResponse{ID: showID, Status: "approved"}, nil
			},
		}
		ah.auditLogService = &testhelpers.MockAuditLogService{
			LogActionFn: func(actorID uint, action string, _ string, entityID uint, _ map[string]interface{}) {
				auditCalled = true
				if action != "approve_show" {
					t.Errorf("expected action='approve_show', got %q", action)
				}
				if entityID != 42 {
					t.Errorf("expected audit entityID=42, got %d", entityID)
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
	h := adminShowHandler(func(ah *AdminShowHandler) {
		ah.showAdminService = &testhelpers.MockShowAdminService{
			ApproveShowFn: func(_ uint, _ bool) (*contracts.ShowResponse, error) {
				return nil, fmt.Errorf("not found")
			},
		}
	})
	_, err := h.ApproveShowHandler(adminCtx(), &ApproveShowRequest{ShowID: "42"})
	testhelpers.AssertHumaError(t, err, 422)
}

func TestRejectShowHandler_Success(t *testing.T) {
	var auditCalled bool
	h := adminShowHandler(func(ah *AdminShowHandler) {
		ah.showAdminService = &testhelpers.MockShowAdminService{
			RejectShowFn: func(showID uint, reason string) (*contracts.ShowResponse, error) {
				if showID != 42 {
					t.Errorf("expected showID=42, got %d", showID)
				}
				if reason != "duplicate" {
					t.Errorf("expected reason='duplicate', got %q", reason)
				}
				return &contracts.ShowResponse{ID: showID, Status: "rejected"}, nil
			},
		}
		ah.auditLogService = &testhelpers.MockAuditLogService{
			LogActionFn: func(_ uint, action string, _ string, entityID uint, _ map[string]interface{}) {
				auditCalled = true
				if action != "reject_show" {
					t.Errorf("expected action='reject_show', got %q", action)
				}
				if entityID != 42 {
					t.Errorf("expected audit entityID=42, got %d", entityID)
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
	h := adminShowHandler(func(ah *AdminShowHandler) {
		ah.showAdminService = &testhelpers.MockShowAdminService{
			RejectShowFn: func(_ uint, _ string) (*contracts.ShowResponse, error) {
				return nil, fmt.Errorf("not found")
			},
		}
	})
	req := &RejectShowRequest{ShowID: "42"}
	req.Body.Reason = "spam"
	_, err := h.RejectShowHandler(adminCtx(), req)
	testhelpers.AssertHumaError(t, err, 422)
}

func TestVerifyVenueHandler_Success(t *testing.T) {
	var auditCalled bool
	h := adminVenueHandler(func(ah *AdminVenueHandler) {
		ah.venueService = &testhelpers.MockVenueService{
			VerifyVenueFn: func(venueID uint) (*contracts.VenueDetailResponse, error) {
				if venueID != 10 {
					t.Errorf("expected venueID=10, got %d", venueID)
				}
				return &contracts.VenueDetailResponse{ID: venueID, Verified: true}, nil
			},
		}
		ah.auditLogService = &testhelpers.MockAuditLogService{
			LogActionFn: func(_ uint, action string, _ string, entityID uint, _ map[string]interface{}) {
				auditCalled = true
				if action != "verify_venue" {
					t.Errorf("expected action='verify_venue', got %q", action)
				}
				if entityID != 10 {
					t.Errorf("expected audit entityID=10, got %d", entityID)
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
	h := adminVenueHandler(func(ah *AdminVenueHandler) {
		ah.venueService = &testhelpers.MockVenueService{
			VerifyVenueFn: func(_ uint) (*contracts.VenueDetailResponse, error) {
				return nil, fmt.Errorf("not found")
			},
		}
	})
	_, err := h.VerifyVenueHandler(adminCtx(), &VerifyVenueRequest{VenueID: "10"})
	testhelpers.AssertHumaError(t, err, 422)
}

func TestCreateAPITokenHandler_Success(t *testing.T) {
	h := adminTokenHandler(func(ah *AdminTokenHandler) {
		ah.apiTokenService = &testhelpers.MockAPITokenService{
			CreateTokenFn: func(userID uint, description *string, expirationDays int) (*contracts.APITokenCreateResponse, error) {
				if userID != 1 {
					t.Errorf("expected userID=1, got %d", userID)
				}
				if expirationDays != 90 {
					t.Errorf("expected expirationDays=90, got %d", expirationDays)
				}
				return &contracts.APITokenCreateResponse{ID: 1, ExpiresAt: time.Now().Add(24 * time.Hour)}, nil
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
	h := adminTokenHandler(func(ah *AdminTokenHandler) {
		ah.apiTokenService = &testhelpers.MockAPITokenService{
			CreateTokenFn: func(_ uint, _ *string, _ int) (*contracts.APITokenCreateResponse, error) {
				return nil, fmt.Errorf("db error")
			},
		}
	})
	req := &CreateAPITokenRequest{}
	req.Body.ExpirationDays = 90
	_, err := h.CreateAPITokenHandler(adminCtx(), req)
	testhelpers.AssertHumaError(t, err, 500)
}

func TestRevokeAPITokenHandler_Success(t *testing.T) {
	h := adminTokenHandler(func(ah *AdminTokenHandler) {
		ah.apiTokenService = &testhelpers.MockAPITokenService{
			RevokeTokenFn: func(userID, tokenID uint) error {
				if tokenID != 42 {
					t.Errorf("expected tokenID=42, got %d", tokenID)
				}
				if userID != 1 {
					t.Errorf("expected userID=1, got %d", userID)
				}
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
	h := adminTokenHandler(func(ah *AdminTokenHandler) {
		ah.apiTokenService = &testhelpers.MockAPITokenService{
			RevokeTokenFn: func(_, _ uint) error {
				return fmt.Errorf("not found")
			},
		}
	})
	_, err := h.RevokeAPITokenHandler(adminCtx(), &RevokeAPITokenRequest{TokenID: "42"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestDataImportHandler_Success(t *testing.T) {
	h := adminDataHandler(func(ah *AdminDataHandler) {
		ah.dataSyncService = &testhelpers.MockDataSyncService{
			ImportDataFn: func(req contracts.DataImportRequest) (*contracts.DataImportResult, error) {
				result := &contracts.DataImportResult{}
				result.Shows.Total = 1
				result.Shows.Imported = 1
				return result, nil
			},
		}
	})
	req := &DataImportRequest{}
	req.Body.Shows = []contracts.ExportedShow{{}}
	resp, err := h.DataImportHandler(adminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Shows.Total != 1 {
		t.Errorf("expected Shows.Total=1, got %d", resp.Body.Shows.Total)
	}
	if resp.Body.Shows.Imported != 1 {
		t.Errorf("expected Shows.Imported=1, got %d", resp.Body.Shows.Imported)
	}
}

func TestDataImportHandler_ServiceError(t *testing.T) {
	h := adminDataHandler(func(ah *AdminDataHandler) {
		ah.dataSyncService = &testhelpers.MockDataSyncService{
			ImportDataFn: func(_ contracts.DataImportRequest) (*contracts.DataImportResult, error) {
				return nil, fmt.Errorf("import failed")
			},
		}
	})
	req := &DataImportRequest{}
	req.Body.Shows = []contracts.ExportedShow{{}}
	_, err := h.DataImportHandler(adminCtx(), req)
	testhelpers.AssertHumaError(t, err, 500)
}

// Discovery success/error tests for AdminDiscoveryHandler now live in
// pipeline/admin_discovery_test.go — they construct the handler via its
// unexported `discoveryService` field, which is only addressable inside the
// pipeline package.

// ============================================================================
// Mock-based tests: Complex multi-service handlers
// ============================================================================

func TestImportShowPreviewHandler_Success(t *testing.T) {
	h := adminShowHandler(func(ah *AdminShowHandler) {
		ah.showImportService = &testhelpers.MockShowImportService{
			PreviewShowImportFn: func(content []byte) (*contracts.ImportPreviewResponse, error) {
				return &contracts.ImportPreviewResponse{CanImport: true}, nil
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
	h := adminShowHandler(func(ah *AdminShowHandler) {
		ah.showImportService = &testhelpers.MockShowImportService{
			PreviewShowImportFn: func(_ []byte) (*contracts.ImportPreviewResponse, error) {
				return nil, fmt.Errorf("parse error")
			},
		}
	})
	req := &ImportShowPreviewRequest{}
	req.Body.Content = base64.StdEncoding.EncodeToString([]byte("bad"))
	_, err := h.ImportShowPreviewHandler(adminCtx(), req)
	testhelpers.AssertHumaError(t, err, 422)
}

func TestImportShowConfirmHandler_Success(t *testing.T) {
	h := adminShowHandler(func(ah *AdminShowHandler) {
		ah.showImportService = &testhelpers.MockShowImportService{
			ConfirmShowImportFn: func(content []byte, verifyVenues bool) (*contracts.ShowResponse, error) {
				return &contracts.ShowResponse{ID: 100, Title: "Imported Show"}, nil
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
	h := adminShowHandler(func(ah *AdminShowHandler) {
		ah.showImportService = &testhelpers.MockShowImportService{
			ConfirmShowImportFn: func(_ []byte, _ bool) (*contracts.ShowResponse, error) {
				return nil, fmt.Errorf("import failed")
			},
		}
	})
	req := &ImportShowConfirmRequest{}
	req.Body.Content = base64.StdEncoding.EncodeToString([]byte("bad"))
	_, err := h.ImportShowConfirmHandler(adminCtx(), req)
	testhelpers.AssertHumaError(t, err, 422)
}

func TestBulkExportShowsHandler_Success(t *testing.T) {
	h := adminShowHandler(func(ah *AdminShowHandler) {
		ah.showImportService = &testhelpers.MockShowImportService{
			ExportShowToMarkdownFn: func(showID uint) ([]byte, string, error) {
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
	h := adminShowHandler(func(ah *AdminShowHandler) {
		ah.showImportService = &testhelpers.MockShowImportService{
			ExportShowToMarkdownFn: func(showID uint) ([]byte, string, error) {
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
	testhelpers.AssertHumaError(t, err, 422)
}

func TestBulkImportPreviewHandler_Success(t *testing.T) {
	h := adminShowHandler(func(ah *AdminShowHandler) {
		ah.showImportService = &testhelpers.MockShowImportService{
			PreviewShowImportFn: func(_ []byte) (*contracts.ImportPreviewResponse, error) {
				return &contracts.ImportPreviewResponse{CanImport: true}, nil
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
	h := adminShowHandler(func(ah *AdminShowHandler) {
		ah.showImportService = &testhelpers.MockShowImportService{
			ConfirmShowImportFn: func(_ []byte, _ bool) (*contracts.ShowResponse, error) {
				return &contracts.ShowResponse{ID: 1}, nil
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
	h := adminShowHandler(func(ah *AdminShowHandler) {
		ah.showImportService = &testhelpers.MockShowImportService{
			ConfirmShowImportFn: func(_ []byte, _ bool) (*contracts.ShowResponse, error) {
				callCount++
				if callCount == 2 {
					return nil, fmt.Errorf("import error")
				}
				return &contracts.ShowResponse{ID: uint(callCount)}, nil
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

// ============================================================================
// Batch approve/reject handler tests
// ============================================================================

func TestBatchApproveShowsHandler_Success(t *testing.T) {
	h := adminShowHandler(func(ah *AdminShowHandler) {
		ah.showAdminService = &testhelpers.MockShowAdminService{
			BatchApproveShowsFn: func(showIDs []uint) (*contracts.BatchShowResult, error) {
				if len(showIDs) != 3 {
					t.Errorf("expected 3 show IDs, got %d", len(showIDs))
				}
				if showIDs[0] != 1 || showIDs[1] != 2 || showIDs[2] != 3 {
					t.Errorf("expected showIDs=[1,2,3], got %v", showIDs)
				}
				return &contracts.BatchShowResult{
					Succeeded: showIDs,
					Errors:    []contracts.BatchShowError{},
				}, nil
			},
		}
	})

	req := &BatchApproveShowsRequest{}
	req.Body.ShowIDs = []uint{1, 2, 3}
	resp, err := h.BatchApproveShowsHandler(adminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Approved != 3 {
		t.Errorf("expected approved=3, got %d", resp.Body.Approved)
	}
	if len(resp.Body.Errors) != 0 {
		t.Errorf("expected 0 errors, got %d", len(resp.Body.Errors))
	}
}

func TestBatchApproveShowsHandler_AdminRequired(t *testing.T) {
	h := testAdminShowHandler()
	req := &BatchApproveShowsRequest{}
	req.Body.ShowIDs = []uint{1}

	// No user context
	_, err := h.BatchApproveShowsHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 403)

	// Non-admin user
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: false})
	_, err = h.BatchApproveShowsHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 403)
}

func TestBatchRejectShowsHandler_Success(t *testing.T) {
	h := adminShowHandler(func(ah *AdminShowHandler) {
		ah.showAdminService = &testhelpers.MockShowAdminService{
			BatchRejectShowsFn: func(showIDs []uint, reason string, category string) (*contracts.BatchShowResult, error) {
				if len(showIDs) != 2 {
					t.Errorf("expected 2 show IDs, got %d", len(showIDs))
				}
				if showIDs[0] != 1 || showIDs[1] != 2 {
					t.Errorf("expected showIDs=[1,2], got %v", showIDs)
				}
				if reason != "Not a music event" {
					t.Errorf("expected reason='Not a music event', got %q", reason)
				}
				if category != "non_music" {
					t.Errorf("expected category='non_music', got %q", category)
				}
				return &contracts.BatchShowResult{
					Succeeded: showIDs,
					Errors:    []contracts.BatchShowError{},
				}, nil
			},
		}
	})

	req := &BatchRejectShowsRequest{}
	req.Body.ShowIDs = []uint{1, 2}
	req.Body.Reason = "Not a music event"
	req.Body.Category = "non_music"
	resp, err := h.BatchRejectShowsHandler(adminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Rejected != 2 {
		t.Errorf("expected rejected=2, got %d", resp.Body.Rejected)
	}
	if len(resp.Body.Errors) != 0 {
		t.Errorf("expected 0 errors, got %d", len(resp.Body.Errors))
	}
}

func TestBatchRejectShowsHandler_AdminRequired(t *testing.T) {
	h := testAdminShowHandler()
	req := &BatchRejectShowsRequest{}
	req.Body.ShowIDs = []uint{1}
	req.Body.Reason = "bad data"

	// No user context
	_, err := h.BatchRejectShowsHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 403)

	// Non-admin user
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: false})
	_, err = h.BatchRejectShowsHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 403)
}

func TestBatchRejectShowsHandler_RequiresReason(t *testing.T) {
	h := adminShowHandler(func(ah *AdminShowHandler) {
		ah.showAdminService = &testhelpers.MockShowAdminService{}
	})

	req := &BatchRejectShowsRequest{}
	req.Body.ShowIDs = []uint{1}
	req.Body.Reason = ""

	_, err := h.BatchRejectShowsHandler(adminCtx(), req)
	testhelpers.AssertHumaError(t, err, 400)
}

// ============================================================================
// Pending shows with filters (admin_shows.go — filter path coverage)
// ============================================================================

func TestGetPendingShowsHandler_WithVenueIDFilter(t *testing.T) {
	var capturedFilter *contracts.PendingShowsFilter
	h := adminShowHandler(func(ah *AdminShowHandler) {
		ah.showAdminService = &testhelpers.MockShowAdminService{
			GetPendingShowsFn: func(limit, offset int, filters *contracts.PendingShowsFilter) ([]*contracts.ShowResponse, int64, error) {
				capturedFilter = filters
				return []*contracts.ShowResponse{}, 0, nil
			},
		}
	})
	resp, err := h.GetPendingShowsHandler(adminCtx(), &GetPendingShowsRequest{Limit: 50, VenueID: 42})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = resp
	if capturedFilter == nil {
		t.Fatal("expected non-nil filter when VenueID is set")
	}
	if capturedFilter.VenueID == nil || *capturedFilter.VenueID != 42 {
		t.Errorf("expected VenueID=42 in filter, got %v", capturedFilter.VenueID)
	}
	if capturedFilter.Source != nil {
		t.Errorf("expected nil Source in filter, got %v", capturedFilter.Source)
	}
}

func TestGetPendingShowsHandler_WithSourceFilter(t *testing.T) {
	var capturedFilter *contracts.PendingShowsFilter
	h := adminShowHandler(func(ah *AdminShowHandler) {
		ah.showAdminService = &testhelpers.MockShowAdminService{
			GetPendingShowsFn: func(limit, offset int, filters *contracts.PendingShowsFilter) ([]*contracts.ShowResponse, int64, error) {
				capturedFilter = filters
				return []*contracts.ShowResponse{}, 0, nil
			},
		}
	})
	_, err := h.GetPendingShowsHandler(adminCtx(), &GetPendingShowsRequest{Limit: 50, Source: "discovery"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedFilter == nil {
		t.Fatal("expected non-nil filter when Source is set")
	}
	if capturedFilter.Source == nil || *capturedFilter.Source != "discovery" {
		t.Errorf("expected Source='discovery' in filter, got %v", capturedFilter.Source)
	}
}

func TestGetPendingShowsHandler_WithBothFilters(t *testing.T) {
	var capturedFilter *contracts.PendingShowsFilter
	h := adminShowHandler(func(ah *AdminShowHandler) {
		ah.showAdminService = &testhelpers.MockShowAdminService{
			GetPendingShowsFn: func(limit, offset int, filters *contracts.PendingShowsFilter) ([]*contracts.ShowResponse, int64, error) {
				capturedFilter = filters
				return []*contracts.ShowResponse{}, 0, nil
			},
		}
	})
	_, err := h.GetPendingShowsHandler(adminCtx(), &GetPendingShowsRequest{Limit: 50, VenueID: 10, Source: "user"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedFilter == nil {
		t.Fatal("expected non-nil filter")
	}
	if capturedFilter.VenueID == nil || *capturedFilter.VenueID != 10 {
		t.Errorf("expected VenueID=10, got %v", capturedFilter.VenueID)
	}
	if capturedFilter.Source == nil || *capturedFilter.Source != "user" {
		t.Errorf("expected Source='user', got %v", capturedFilter.Source)
	}
}

func TestGetPendingShowsHandler_NoFilters(t *testing.T) {
	var capturedFilter *contracts.PendingShowsFilter
	h := adminShowHandler(func(ah *AdminShowHandler) {
		ah.showAdminService = &testhelpers.MockShowAdminService{
			GetPendingShowsFn: func(limit, offset int, filters *contracts.PendingShowsFilter) ([]*contracts.ShowResponse, int64, error) {
				capturedFilter = filters
				return []*contracts.ShowResponse{}, 0, nil
			},
		}
	})
	_, err := h.GetPendingShowsHandler(adminCtx(), &GetPendingShowsRequest{Limit: 50})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedFilter != nil {
		t.Error("expected nil filter when no VenueID or Source set")
	}
}

// ============================================================================
// Limit/offset clamping tests (admin_shows.go)
// ============================================================================

func TestGetPendingShowsHandler_LimitClamping(t *testing.T) {
	var capturedLimit, capturedOffset int
	h := adminShowHandler(func(ah *AdminShowHandler) {
		ah.showAdminService = &testhelpers.MockShowAdminService{
			GetPendingShowsFn: func(limit, offset int, _ *contracts.PendingShowsFilter) ([]*contracts.ShowResponse, int64, error) {
				capturedLimit = limit
				capturedOffset = offset
				return []*contracts.ShowResponse{}, 0, nil
			},
		}
	})

	// Limit < 1 => defaults to 50
	_, err := h.GetPendingShowsHandler(adminCtx(), &GetPendingShowsRequest{Limit: 0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedLimit != 50 {
		t.Errorf("expected limit clamped to 50, got %d", capturedLimit)
	}

	// Limit > 100 => capped to 100
	_, err = h.GetPendingShowsHandler(adminCtx(), &GetPendingShowsRequest{Limit: 200})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedLimit != 100 {
		t.Errorf("expected limit clamped to 100, got %d", capturedLimit)
	}

	// Negative offset => 0
	_, err = h.GetPendingShowsHandler(adminCtx(), &GetPendingShowsRequest{Limit: 50, Offset: -5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedOffset != 0 {
		t.Errorf("expected offset clamped to 0, got %d", capturedOffset)
	}
}

func TestGetRejectedShowsHandler_LimitClamping(t *testing.T) {
	var capturedLimit, capturedOffset int
	h := adminShowHandler(func(ah *AdminShowHandler) {
		ah.showAdminService = &testhelpers.MockShowAdminService{
			GetRejectedShowsFn: func(limit, offset int, _ string) ([]*contracts.ShowResponse, int64, error) {
				capturedLimit = limit
				capturedOffset = offset
				return []*contracts.ShowResponse{}, 0, nil
			},
		}
	})

	// Limit < 1 => 50
	_, err := h.GetRejectedShowsHandler(adminCtx(), &GetRejectedShowsRequest{Limit: -1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedLimit != 50 {
		t.Errorf("expected limit=50, got %d", capturedLimit)
	}

	// Limit > 100 => 100
	_, err = h.GetRejectedShowsHandler(adminCtx(), &GetRejectedShowsRequest{Limit: 150})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedLimit != 100 {
		t.Errorf("expected limit=100, got %d", capturedLimit)
	}

	// Negative offset => 0
	_, err = h.GetRejectedShowsHandler(adminCtx(), &GetRejectedShowsRequest{Limit: 50, Offset: -10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedOffset != 0 {
		t.Errorf("expected offset=0, got %d", capturedOffset)
	}
}

func TestGetAdminShowsHandler_LimitClamping(t *testing.T) {
	var capturedLimit, capturedOffset int
	h := adminShowHandler(func(ah *AdminShowHandler) {
		ah.showAdminService = &testhelpers.MockShowAdminService{
			GetAdminShowsFn: func(limit, offset int, _ contracts.AdminShowFilters) ([]*contracts.ShowResponse, int64, error) {
				capturedLimit = limit
				capturedOffset = offset
				return []*contracts.ShowResponse{}, 0, nil
			},
		}
	})

	_, err := h.GetAdminShowsHandler(adminCtx(), &GetAdminShowsRequest{Limit: 0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedLimit != 50 {
		t.Errorf("expected limit=50, got %d", capturedLimit)
	}

	_, err = h.GetAdminShowsHandler(adminCtx(), &GetAdminShowsRequest{Limit: 999})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedLimit != 100 {
		t.Errorf("expected limit=100, got %d", capturedLimit)
	}

	_, err = h.GetAdminShowsHandler(adminCtx(), &GetAdminShowsRequest{Limit: 50, Offset: -3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedOffset != 0 {
		t.Errorf("expected offset=0, got %d", capturedOffset)
	}
}

// ============================================================================
// Batch handler service error paths (admin_shows.go)
// ============================================================================

func TestBatchApproveShowsHandler_ServiceError(t *testing.T) {
	h := adminShowHandler(func(ah *AdminShowHandler) {
		ah.showAdminService = &testhelpers.MockShowAdminService{
			BatchApproveShowsFn: func(_ []uint) (*contracts.BatchShowResult, error) {
				return nil, fmt.Errorf("db error")
			},
		}
	})
	req := &BatchApproveShowsRequest{}
	req.Body.ShowIDs = []uint{1, 2}
	_, err := h.BatchApproveShowsHandler(adminCtx(), req)
	testhelpers.AssertHumaError(t, err, 500)
}

func TestBatchRejectShowsHandler_ServiceError(t *testing.T) {
	h := adminShowHandler(func(ah *AdminShowHandler) {
		ah.showAdminService = &testhelpers.MockShowAdminService{
			BatchRejectShowsFn: func(_ []uint, _ string, _ string) (*contracts.BatchShowResult, error) {
				return nil, fmt.Errorf("db error")
			},
		}
	})
	req := &BatchRejectShowsRequest{}
	req.Body.ShowIDs = []uint{1}
	req.Body.Reason = "test reason"
	_, err := h.BatchRejectShowsHandler(adminCtx(), req)
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// Bulk import preview edge cases (admin_shows.go)
// ============================================================================

func TestBulkImportPreviewHandler_InvalidBase64(t *testing.T) {
	h := adminShowHandler(func(ah *AdminShowHandler) {
		ah.showImportService = &testhelpers.MockShowImportService{}
	})
	req := &BulkImportPreviewRequest{}
	req.Body.Shows = []string{"not-valid-base64!!!"}
	_, err := h.BulkImportPreviewHandler(adminCtx(), req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestBulkImportPreviewHandler_ServiceError(t *testing.T) {
	h := adminShowHandler(func(ah *AdminShowHandler) {
		ah.showImportService = &testhelpers.MockShowImportService{
			PreviewShowImportFn: func(_ []byte) (*contracts.ImportPreviewResponse, error) {
				return nil, fmt.Errorf("parse error")
			},
		}
	})
	req := &BulkImportPreviewRequest{}
	req.Body.Shows = []string{base64.StdEncoding.EncodeToString([]byte("# Show"))}
	_, err := h.BulkImportPreviewHandler(adminCtx(), req)
	testhelpers.AssertHumaError(t, err, 422)
}

func TestBulkImportPreviewHandler_SummaryAccumulation(t *testing.T) {
	callCount := 0
	h := adminShowHandler(func(ah *AdminShowHandler) {
		ah.showImportService = &testhelpers.MockShowImportService{
			PreviewShowImportFn: func(_ []byte) (*contracts.ImportPreviewResponse, error) {
				callCount++
				if callCount == 1 {
					return &contracts.ImportPreviewResponse{
						CanImport: true,
						Venues:    []contracts.VenueMatchResult{{WillCreate: true}},
						Artists:   []contracts.ArtistMatchResult{{WillCreate: true}, {WillCreate: false}},
						Warnings:  []string{"warning1"},
					}, nil
				}
				return &contracts.ImportPreviewResponse{
					CanImport: false,
					Venues:    []contracts.VenueMatchResult{{WillCreate: false}},
					Artists:   []contracts.ArtistMatchResult{{WillCreate: true}},
					Warnings:  []string{"warning2", "warning3"},
				}, nil
			},
		}
	})
	req := &BulkImportPreviewRequest{}
	req.Body.Shows = []string{
		base64.StdEncoding.EncodeToString([]byte("# Show 1")),
		base64.StdEncoding.EncodeToString([]byte("# Show 2")),
	}
	resp, err := h.BulkImportPreviewHandler(adminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Summary.TotalShows != 2 {
		t.Errorf("expected total_shows=2, got %d", resp.Body.Summary.TotalShows)
	}
	if resp.Body.Summary.NewVenues != 1 {
		t.Errorf("expected new_venues=1, got %d", resp.Body.Summary.NewVenues)
	}
	if resp.Body.Summary.ExistingVenues != 1 {
		t.Errorf("expected existing_venues=1, got %d", resp.Body.Summary.ExistingVenues)
	}
	if resp.Body.Summary.NewArtists != 2 {
		t.Errorf("expected new_artists=2, got %d", resp.Body.Summary.NewArtists)
	}
	if resp.Body.Summary.ExistingArtists != 1 {
		t.Errorf("expected existing_artists=1, got %d", resp.Body.Summary.ExistingArtists)
	}
	if resp.Body.Summary.WarningCount != 3 {
		t.Errorf("expected warning_count=3, got %d", resp.Body.Summary.WarningCount)
	}
	if resp.Body.Summary.CanImportAll {
		t.Error("expected can_import_all=false when one show cannot be imported")
	}
}

// ============================================================================
// Bulk import confirm edge cases (admin_shows.go)
// ============================================================================

func TestBulkImportConfirmHandler_InvalidBase64InArray(t *testing.T) {
	h := adminShowHandler(func(ah *AdminShowHandler) {
		ah.showImportService = &testhelpers.MockShowImportService{
			ConfirmShowImportFn: func(_ []byte, _ bool) (*contracts.ShowResponse, error) {
				return &contracts.ShowResponse{ID: 1}, nil
			},
		}
	})
	req := &BulkImportConfirmRequest{}
	req.Body.Shows = []string{
		base64.StdEncoding.EncodeToString([]byte("# Good Show")),
		"not-valid-base64!!!",
	}
	resp, err := h.BulkImportConfirmHandler(adminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.SuccessCount != 1 {
		t.Errorf("expected success_count=1, got %d", resp.Body.SuccessCount)
	}
	if resp.Body.ErrorCount != 1 {
		t.Errorf("expected error_count=1, got %d", resp.Body.ErrorCount)
	}
	// Verify the error result
	found := false
	for _, r := range resp.Body.Results {
		if !r.Success && r.Error == "Invalid base64 content" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'Invalid base64 content' error in results")
	}
}

// ============================================================================
// Export venues verified filter paths (admin_data.go)
// ============================================================================

func TestExportVenuesHandler_VerifiedFilterTrue(t *testing.T) {
	var capturedParams contracts.ExportVenuesParams
	h := adminDataHandler(func(ah *AdminDataHandler) {
		ah.dataSyncService = &testhelpers.MockDataSyncService{
			ExportVenuesFn: func(params contracts.ExportVenuesParams) (*contracts.ExportVenuesResult, error) {
				capturedParams = params
				return &contracts.ExportVenuesResult{Total: 1}, nil
			},
		}
	})
	_, err := h.ExportVenuesHandler(adminCtx(), &ExportVenuesRequest{Limit: 50, Verified: "true"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedParams.Verified == nil || *capturedParams.Verified != true {
		t.Error("expected Verified=true in params")
	}
}

func TestExportVenuesHandler_VerifiedFilterFalse(t *testing.T) {
	var capturedParams contracts.ExportVenuesParams
	h := adminDataHandler(func(ah *AdminDataHandler) {
		ah.dataSyncService = &testhelpers.MockDataSyncService{
			ExportVenuesFn: func(params contracts.ExportVenuesParams) (*contracts.ExportVenuesResult, error) {
				capturedParams = params
				return &contracts.ExportVenuesResult{Total: 1}, nil
			},
		}
	})
	_, err := h.ExportVenuesHandler(adminCtx(), &ExportVenuesRequest{Limit: 50, Verified: "false"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedParams.Verified == nil || *capturedParams.Verified != false {
		t.Error("expected Verified=false in params")
	}
}

func TestExportVenuesHandler_NegativeOffset(t *testing.T) {
	var capturedOffset int
	h := adminDataHandler(func(ah *AdminDataHandler) {
		ah.dataSyncService = &testhelpers.MockDataSyncService{
			ExportVenuesFn: func(params contracts.ExportVenuesParams) (*contracts.ExportVenuesResult, error) {
				capturedOffset = params.Offset
				return &contracts.ExportVenuesResult{Total: 0}, nil
			},
		}
	})
	_, err := h.ExportVenuesHandler(adminCtx(), &ExportVenuesRequest{Limit: 50, Offset: -5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedOffset != 0 {
		t.Errorf("expected offset clamped to 0, got %d", capturedOffset)
	}
}

// ============================================================================
// Export shows with valid date and negative offset (admin_data.go)
// ============================================================================

func TestExportShowsHandler_WithValidDate(t *testing.T) {
	var capturedParams contracts.ExportShowsParams
	h := adminDataHandler(func(ah *AdminDataHandler) {
		ah.dataSyncService = &testhelpers.MockDataSyncService{
			ExportShowsFn: func(params contracts.ExportShowsParams) (*contracts.ExportShowsResult, error) {
				capturedParams = params
				return &contracts.ExportShowsResult{Total: 0}, nil
			},
		}
	})
	_, err := h.ExportShowsHandler(adminCtx(), &ExportShowsRequest{Limit: 50, FromDate: "2025-06-15"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedParams.FromDate == nil {
		t.Fatal("expected FromDate to be set")
	}
	expected := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
	if !capturedParams.FromDate.Equal(expected) {
		t.Errorf("expected FromDate=%v, got %v", expected, *capturedParams.FromDate)
	}
}

func TestExportShowsHandler_NegativeOffset(t *testing.T) {
	var capturedOffset int
	h := adminDataHandler(func(ah *AdminDataHandler) {
		ah.dataSyncService = &testhelpers.MockDataSyncService{
			ExportShowsFn: func(params contracts.ExportShowsParams) (*contracts.ExportShowsResult, error) {
				capturedOffset = params.Offset
				return &contracts.ExportShowsResult{Total: 0}, nil
			},
		}
	})
	_, err := h.ExportShowsHandler(adminCtx(), &ExportShowsRequest{Limit: 50, Offset: -10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedOffset != 0 {
		t.Errorf("expected offset clamped to 0, got %d", capturedOffset)
	}
}

func TestExportArtistsHandler_NegativeOffset(t *testing.T) {
	var capturedOffset int
	h := adminDataHandler(func(ah *AdminDataHandler) {
		ah.dataSyncService = &testhelpers.MockDataSyncService{
			ExportArtistsFn: func(params contracts.ExportArtistsParams) (*contracts.ExportArtistsResult, error) {
				capturedOffset = params.Offset
				return &contracts.ExportArtistsResult{Total: 0}, nil
			},
		}
	})
	_, err := h.ExportArtistsHandler(adminCtx(), &ExportArtistsRequest{Limit: 50, Offset: -3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedOffset != 0 {
		t.Errorf("expected offset clamped to 0, got %d", capturedOffset)
	}
}

// ============================================================================
// Venue handler limit clamping tests (admin_venues.go)
// ============================================================================

func TestGetUnverifiedVenuesHandler_LimitClamping(t *testing.T) {
	var capturedLimit, capturedOffset int
	h := adminVenueHandler(func(ah *AdminVenueHandler) {
		ah.venueService = &testhelpers.MockVenueService{
			GetUnverifiedVenuesFn: func(limit, offset int) ([]*contracts.UnverifiedVenueResponse, int64, error) {
				capturedLimit = limit
				capturedOffset = offset
				return []*contracts.UnverifiedVenueResponse{}, 0, nil
			},
		}
	})

	_, err := h.GetUnverifiedVenuesHandler(adminCtx(), &GetUnverifiedVenuesRequest{Limit: 0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedLimit != 50 {
		t.Errorf("expected limit=50, got %d", capturedLimit)
	}

	_, err = h.GetUnverifiedVenuesHandler(adminCtx(), &GetUnverifiedVenuesRequest{Limit: 200})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedLimit != 100 {
		t.Errorf("expected limit=100, got %d", capturedLimit)
	}

	_, err = h.GetUnverifiedVenuesHandler(adminCtx(), &GetUnverifiedVenuesRequest{Limit: 50, Offset: -1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedOffset != 0 {
		t.Errorf("expected offset=0, got %d", capturedOffset)
	}
}

// ============================================================================
// User handler limit clamping tests (admin_users.go)
// ============================================================================

func TestGetAdminUsersHandler_LimitClamping(t *testing.T) {
	var capturedLimit, capturedOffset int
	h := adminUserHandler(func(ah *AdminUserHandler) {
		ah.userService = &testhelpers.MockUserService{
			ListUsersFn: func(limit, offset int, _ contracts.AdminUserFilters) ([]*contracts.AdminUserResponse, int64, error) {
				capturedLimit = limit
				capturedOffset = offset
				return []*contracts.AdminUserResponse{}, 0, nil
			},
		}
	})

	_, err := h.GetAdminUsersHandler(adminCtx(), &GetAdminUsersRequest{Limit: 0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedLimit != 50 {
		t.Errorf("expected limit=50, got %d", capturedLimit)
	}

	_, err = h.GetAdminUsersHandler(adminCtx(), &GetAdminUsersRequest{Limit: 101})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedLimit != 100 {
		t.Errorf("expected limit=100, got %d", capturedLimit)
	}

	_, err = h.GetAdminUsersHandler(adminCtx(), &GetAdminUsersRequest{Limit: 50, Offset: -1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedOffset != 0 {
		t.Errorf("expected offset=0, got %d", capturedOffset)
	}
}

// ============================================================================
// Helpers tests (helpers.go)
// ============================================================================

func TestParseDate_Valid(t *testing.T) {
	d, err := shared.ParseDate("2025-06-15")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Year() != 2025 || d.Month() != 6 || d.Day() != 15 {
		t.Errorf("unexpected date: %v", d)
	}
}

func TestParseDate_Invalid(t *testing.T) {
	_, err := shared.ParseDate("not-a-date")
	if err == nil {
		t.Error("expected error for invalid date")
	}
}

func TestGetUserID_Nil(t *testing.T) {
	id := shared.GetUserID(nil)
	if id != 0 {
		t.Errorf("expected 0, got %d", id)
	}
}

func TestGetUserID_NonNil(t *testing.T) {
	id := shared.GetUserID(&authm.User{ID: 42})
	if id != 42 {
		t.Errorf("expected 42, got %d", id)
	}
}

func TestPtrString(t *testing.T) {
	p := shared.PtrString("hello")
	if p == nil || *p != "hello" {
		t.Errorf("expected pointer to 'hello', got %v", p)
	}
}

// ============================================================================
// Data import handler edge cases (admin_data.go)
// ============================================================================

func TestDataImportHandler_DryRun(t *testing.T) {
	var capturedDryRun bool
	h := adminDataHandler(func(ah *AdminDataHandler) {
		ah.dataSyncService = &testhelpers.MockDataSyncService{
			ImportDataFn: func(req contracts.DataImportRequest) (*contracts.DataImportResult, error) {
				capturedDryRun = req.DryRun
				return &contracts.DataImportResult{}, nil
			},
		}
	})
	req := &DataImportRequest{}
	req.Body.Shows = []contracts.ExportedShow{{}}
	req.Body.DryRun = true
	_, err := h.DataImportHandler(adminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !capturedDryRun {
		t.Error("expected dry_run=true to be passed to service")
	}
}
