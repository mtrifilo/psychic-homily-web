package handlers

import (
	"context"
	"testing"

	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
)

func testVenueHandler() *VenueHandler {
	return NewVenueHandler(nil, nil, nil, nil)
}

// --- AdminCreateVenueHandler ---

func TestAdminCreateVenueHandler_NoAuth(t *testing.T) {
	h := testVenueHandler()
	req := &AdminCreateVenueRequest{}
	req.Body.Name = "Test Venue"
	req.Body.City = "Phoenix"
	req.Body.State = "AZ"

	_, err := h.AdminCreateVenueHandler(context.Background(), req)
	assertHumaError(t, err, 403)
}

func TestAdminCreateVenueHandler_NonAdmin(t *testing.T) {
	h := testVenueHandler()
	ctx := ctxWithUser(&models.User{ID: 1, IsAdmin: false})
	req := &AdminCreateVenueRequest{}
	req.Body.Name = "Test Venue"
	req.Body.City = "Phoenix"
	req.Body.State = "AZ"

	_, err := h.AdminCreateVenueHandler(ctx, req)
	assertHumaError(t, err, 403)
}

// --- UpdateVenueHandler (admin-only post-PSY-503) ---

func TestUpdateVenueHandler_NoAuth(t *testing.T) {
	h := testVenueHandler()
	req := &UpdateVenueRequest{VenueID: "1"}

	// Admin-only handler: unauthenticated requests get 403 (requireAdmin
	// collapses "no user" and "non-admin" into the same response).
	_, err := h.UpdateVenueHandler(context.Background(), req)
	assertHumaError(t, err, 403)
}

func TestUpdateVenueHandler_NonAdmin(t *testing.T) {
	h := testVenueHandler()
	// Even a venue submitter can't direct-update via this endpoint; they
	// must use PUT /venues/{id}/suggest-edit through SuggestVenueEditHandler.
	ctx := ctxWithUser(&models.User{ID: 1, IsAdmin: false})
	req := &UpdateVenueRequest{VenueID: "1"}

	_, err := h.UpdateVenueHandler(ctx, req)
	assertHumaError(t, err, 403)
}

func TestUpdateVenueHandler_InvalidID(t *testing.T) {
	h := testVenueHandler()
	ctx := ctxWithUser(&models.User{ID: 1, IsAdmin: true})
	req := &UpdateVenueRequest{VenueID: "abc"}

	_, err := h.UpdateVenueHandler(ctx, req)
	assertHumaError(t, err, 400)
}

// --- DeleteVenueHandler ---

func TestDeleteVenueHandler_NoAuth(t *testing.T) {
	h := testVenueHandler()
	req := &DeleteVenueRequest{VenueID: "1"}

	_, err := h.DeleteVenueHandler(context.Background(), req)
	assertHumaError(t, err, 401)
}

func TestDeleteVenueHandler_InvalidID(t *testing.T) {
	h := testVenueHandler()
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &DeleteVenueRequest{VenueID: "abc"}

	_, err := h.DeleteVenueHandler(ctx, req)
	assertHumaError(t, err, 400)
}

// ============================================================================
// ID Parsing Boundary Tests
// ============================================================================

func TestGetVenueHandler_ZeroID(t *testing.T) {
	mock := &mockVenueService{
		getVenueFn: func(venueID uint) (*contracts.VenueDetailResponse, error) {
			return nil, apperrors.ErrVenueNotFound(0)
		},
	}
	h := NewVenueHandler(mock, nil, nil, nil)
	_, err := h.GetVenueHandler(context.Background(), &GetVenueRequest{VenueID: "0"})
	assertHumaError(t, err, 404)
}

func TestGetVenueHandler_VeryLargeID(t *testing.T) {
	mock := &mockVenueService{
		getVenueFn: func(venueID uint) (*contracts.VenueDetailResponse, error) {
			return nil, apperrors.ErrVenueNotFound(venueID)
		},
	}
	h := NewVenueHandler(mock, nil, nil, nil)
	_, err := h.GetVenueHandler(context.Background(), &GetVenueRequest{VenueID: "4294967295"})
	assertHumaError(t, err, 404)
}

func TestGetVenueHandler_OverflowID(t *testing.T) {
	mock := &mockVenueService{
		getVenueBySlugFn: func(slug string) (*contracts.VenueDetailResponse, error) {
			return nil, apperrors.ErrVenueNotFound(0)
		},
	}
	h := NewVenueHandler(mock, nil, nil, nil)
	_, err := h.GetVenueHandler(context.Background(), &GetVenueRequest{VenueID: "99999999999"})
	assertHumaError(t, err, 404)
}

func TestUpdateVenueHandler_ZeroID(t *testing.T) {
	mock := &mockVenueService{
		updateVenueFn: func(venueID uint, _ map[string]interface{}) (*contracts.VenueDetailResponse, error) {
			return nil, apperrors.ErrVenueNotFound(venueID)
		},
	}
	h := NewVenueHandler(mock, nil, nil, nil)
	ctx := ctxWithUser(&models.User{ID: 1, IsAdmin: true})
	_, err := h.UpdateVenueHandler(ctx, &UpdateVenueRequest{VenueID: "0"})
	assertHumaError(t, err, 404)
}

func TestDeleteVenueHandler_ZeroID(t *testing.T) {
	mock := &mockVenueService{
		getVenueModelFn: func(venueID uint) (*models.Venue, error) {
			return nil, apperrors.ErrVenueNotFound(venueID)
		},
	}
	h := NewVenueHandler(mock, nil, nil, nil)
	ctx := ctxWithUser(&models.User{ID: 1})
	_, err := h.DeleteVenueHandler(ctx, &DeleteVenueRequest{VenueID: "0"})
	assertHumaError(t, err, 404)
}

func TestDeleteVenueHandler_OverflowID(t *testing.T) {
	h := testVenueHandler()
	ctx := ctxWithUser(&models.User{ID: 1})
	_, err := h.DeleteVenueHandler(ctx, &DeleteVenueRequest{VenueID: "99999999999"})
	assertHumaError(t, err, 400)
}
