package catalog

import (
	"context"
	"fmt"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

func testVenueHandler() *VenueHandler {
	return NewVenueHandler(nil, nil, nil, nil)
}

func TestUpdateVenueHandler_InvalidID(t *testing.T) {
	h := testVenueHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	req := &UpdateVenueRequest{VenueID: "abc"}

	_, err := h.UpdateVenueHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 400)
}

// --- DeleteVenueHandler ---

func TestDeleteVenueHandler_NoAuth(t *testing.T) {
	h := testVenueHandler()
	req := &DeleteVenueRequest{VenueID: "1"}

	_, err := h.DeleteVenueHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 401)
}

func TestDeleteVenueHandler_InvalidID(t *testing.T) {
	h := testVenueHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &DeleteVenueRequest{VenueID: "abc"}

	_, err := h.DeleteVenueHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 400)
}

// ============================================================================
// ID Parsing Boundary Tests
// ============================================================================

func TestGetVenueHandler_ZeroID(t *testing.T) {
	mock := &testhelpers.MockVenueService{
		GetVenueFn: func(venueID uint) (*contracts.VenueDetailResponse, error) {
			return nil, apperrors.ErrVenueNotFound(0)
		},
	}
	h := NewVenueHandler(mock, nil, nil, nil)
	_, err := h.GetVenueHandler(context.Background(), &GetVenueRequest{VenueID: "0"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestGetVenueHandler_VeryLargeID(t *testing.T) {
	mock := &testhelpers.MockVenueService{
		GetVenueFn: func(venueID uint) (*contracts.VenueDetailResponse, error) {
			return nil, apperrors.ErrVenueNotFound(venueID)
		},
	}
	h := NewVenueHandler(mock, nil, nil, nil)
	_, err := h.GetVenueHandler(context.Background(), &GetVenueRequest{VenueID: "4294967295"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestGetVenueHandler_OverflowID(t *testing.T) {
	mock := &testhelpers.MockVenueService{
		GetVenueBySlugFn: func(slug string) (*contracts.VenueDetailResponse, error) {
			return nil, apperrors.ErrVenueNotFound(0)
		},
	}
	h := NewVenueHandler(mock, nil, nil, nil)
	_, err := h.GetVenueHandler(context.Background(), &GetVenueRequest{VenueID: "99999999999"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestUpdateVenueHandler_ZeroID(t *testing.T) {
	mock := &testhelpers.MockVenueService{
		UpdateVenueFn: func(venueID uint, _ *contracts.UpdateVenueRequest) (*contracts.VenueDetailResponse, error) {
			return nil, apperrors.ErrVenueNotFound(venueID)
		},
	}
	h := NewVenueHandler(mock, nil, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	_, err := h.UpdateVenueHandler(ctx, &UpdateVenueRequest{VenueID: "0"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestDeleteVenueHandler_ZeroID(t *testing.T) {
	mock := &testhelpers.MockVenueService{
		GetVenueModelFn: func(venueID uint) (*catalogm.Venue, error) {
			return nil, apperrors.ErrVenueNotFound(venueID)
		},
	}
	h := NewVenueHandler(mock, nil, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	_, err := h.DeleteVenueHandler(ctx, &DeleteVenueRequest{VenueID: "0"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestDeleteVenueHandler_OverflowID(t *testing.T) {
	h := testVenueHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	_, err := h.DeleteVenueHandler(ctx, &DeleteVenueRequest{VenueID: "99999999999"})
	testhelpers.AssertHumaError(t, err, 400)
}

// ============================================================================
// GetVenueGenresHandler
// ============================================================================

func TestGetVenueGenres_ByID(t *testing.T) {
	mock := &testhelpers.MockVenueService{
		GetVenueGenreProfileFn: func(venueID uint) ([]contracts.GenreCount, error) {
			if venueID != 5 {
				t.Errorf("expected venueID=5, got %d", venueID)
			}
			return []contracts.GenreCount{{TagID: 1, Name: "punk", Slug: "punk", Count: 10}}, nil
		},
	}
	h := NewVenueHandler(mock, nil, nil, nil)
	resp, err := h.GetVenueGenresHandler(context.Background(), &GetVenueGenresRequest{VenueID: "5"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Body.Genres) != 1 || resp.Body.Genres[0].Name != "punk" {
		t.Errorf("unexpected body: %+v", resp.Body)
	}
}

func TestGetVenueGenres_BySlug(t *testing.T) {
	mock := &testhelpers.MockVenueService{
		GetVenueBySlugFn: func(slug string) (*contracts.VenueDetailResponse, error) {
			return &contracts.VenueDetailResponse{ID: 10}, nil
		},
		GetVenueGenreProfileFn: func(venueID uint) ([]contracts.GenreCount, error) {
			if venueID != 10 {
				t.Errorf("expected resolved venueID=10, got %d", venueID)
			}
			// nil genres → handler coerces to empty (non-nil) slice.
			return nil, nil
		},
	}
	h := NewVenueHandler(mock, nil, nil, nil)
	resp, err := h.GetVenueGenresHandler(context.Background(), &GetVenueGenresRequest{VenueID: "valley-bar"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Genres == nil {
		t.Error("expected non-nil empty genres slice")
	}
}

func TestGetVenueGenres_SlugNotFound(t *testing.T) {
	mock := &testhelpers.MockVenueService{
		GetVenueBySlugFn: func(_ string) (*contracts.VenueDetailResponse, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	h := NewVenueHandler(mock, nil, nil, nil)
	_, err := h.GetVenueGenresHandler(context.Background(), &GetVenueGenresRequest{VenueID: "ghost"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestGetVenueGenres_ServiceError(t *testing.T) {
	mock := &testhelpers.MockVenueService{
		GetVenueGenreProfileFn: func(_ uint) ([]contracts.GenreCount, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := NewVenueHandler(mock, nil, nil, nil)
	_, err := h.GetVenueGenresHandler(context.Background(), &GetVenueGenresRequest{VenueID: "5"})
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// GetVenueBillNetworkHandler
// ============================================================================

func TestGetVenueBillNetwork_Success(t *testing.T) {
	mock := &testhelpers.MockVenueService{
		GetVenueBillNetworkFn: func(venueID uint, window string, year *int) (*contracts.VenueBillNetworkResponse, error) {
			if venueID != 5 || window != "all" || year != nil {
				t.Errorf("unexpected params venueID=%d window=%q year=%v", venueID, window, year)
			}
			return &contracts.VenueBillNetworkResponse{}, nil
		},
	}
	h := NewVenueHandler(mock, nil, nil, nil)
	_, err := h.GetVenueBillNetworkHandler(context.Background(), &GetVenueBillNetworkRequest{VenueID: "5", Window: "all"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetVenueBillNetwork_YearWindowRequiresYear(t *testing.T) {
	h := NewVenueHandler(&testhelpers.MockVenueService{}, nil, nil, nil)
	// window=year with no year → 422 before the service is consulted.
	_, err := h.GetVenueBillNetworkHandler(context.Background(), &GetVenueBillNetworkRequest{VenueID: "5", Window: "year"})
	testhelpers.AssertHumaError(t, err, 422)
}

func TestGetVenueBillNetwork_NotFound(t *testing.T) {
	mock := &testhelpers.MockVenueService{
		GetVenueBillNetworkFn: func(_ uint, _ string, _ *int) (*contracts.VenueBillNetworkResponse, error) {
			return nil, apperrors.ErrVenueNotFound(99)
		},
	}
	h := NewVenueHandler(mock, nil, nil, nil)
	_, err := h.GetVenueBillNetworkHandler(context.Background(), &GetVenueBillNetworkRequest{VenueID: "99", Window: "all"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestGetVenueBillNetwork_ServiceError(t *testing.T) {
	mock := &testhelpers.MockVenueService{
		GetVenueBillNetworkFn: func(_ uint, _ string, _ *int) (*contracts.VenueBillNetworkResponse, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := NewVenueHandler(mock, nil, nil, nil)
	_, err := h.GetVenueBillNetworkHandler(context.Background(), &GetVenueBillNetworkRequest{VenueID: "5", Window: "all"})
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// AdminCreateVenueHandler
// ============================================================================

func TestAdminCreateVenue_Success(t *testing.T) {
	mock := &testhelpers.MockVenueService{
		CreateVenueFn: func(req *contracts.CreateVenueRequest, isAdmin bool) (*contracts.VenueDetailResponse, error) {
			if !isAdmin {
				t.Error("expected isAdmin=true for admin create")
			}
			if req.Name != "Valley Bar" || req.SubmittedBy == nil || *req.SubmittedBy != 1 {
				t.Errorf("unexpected service request: %+v", req)
			}
			return &contracts.VenueDetailResponse{ID: 1, Name: req.Name}, nil
		},
	}
	h := NewVenueHandler(mock, nil, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	req := &AdminCreateVenueRequest{}
	req.Body.Name = "Valley Bar"
	req.Body.City = "Phoenix"
	req.Body.State = "AZ"

	resp, err := h.AdminCreateVenueHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Name != "Valley Bar" {
		t.Errorf("expected name='Valley Bar', got %q", resp.Body.Name)
	}
}

func TestAdminCreateVenue_InvalidSocialURL(t *testing.T) {
	// Social-URL validation runs before the service call; a non-http scheme
	// is rejected without ever reaching CreateVenue.
	h := NewVenueHandler(&testhelpers.MockVenueService{}, nil, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	bad := "javascript:alert(1)"
	req := &AdminCreateVenueRequest{}
	req.Body.Name = "Valley Bar"
	req.Body.City = "Phoenix"
	req.Body.State = "AZ"
	req.Body.Instagram = &bad

	_, err := h.AdminCreateVenueHandler(ctx, req)
	if err == nil {
		t.Fatal("expected error for javascript: URL, got nil")
	}
}

func TestAdminCreateVenue_ServiceError(t *testing.T) {
	mock := &testhelpers.MockVenueService{
		CreateVenueFn: func(_ *contracts.CreateVenueRequest, _ bool) (*contracts.VenueDetailResponse, error) {
			return nil, fmt.Errorf("duplicate venue")
		},
	}
	h := NewVenueHandler(mock, nil, nil, nil)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
	req := &AdminCreateVenueRequest{}
	req.Body.Name = "Valley Bar"
	req.Body.City = "Phoenix"
	req.Body.State = "AZ"

	_, err := h.AdminCreateVenueHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 422)
}
