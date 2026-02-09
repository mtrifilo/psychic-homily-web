package handlers

import (
	"context"
	"testing"

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
