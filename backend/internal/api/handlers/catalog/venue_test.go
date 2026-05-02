package catalog

import (
	"context"
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
		UpdateVenueFn: func(venueID uint, _ map[string]interface{}) (*contracts.VenueDetailResponse, error) {
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
