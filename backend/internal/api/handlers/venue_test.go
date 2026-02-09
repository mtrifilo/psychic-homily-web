package handlers

import (
	"context"
	"testing"

	"psychic-homily-backend/internal/models"
)

func testVenueHandler() *VenueHandler {
	return NewVenueHandler(nil, nil)
}

// --- NewVenueHandler ---

func TestNewVenueHandler(t *testing.T) {
	h := testVenueHandler()
	if h == nil {
		t.Fatal("expected non-nil VenueHandler")
	}
}

// --- UpdateVenueHandler ---

func TestUpdateVenueHandler_NoAuth(t *testing.T) {
	h := testVenueHandler()
	req := &UpdateVenueRequest{VenueID: "1"}

	_, err := h.UpdateVenueHandler(context.Background(), req)
	assertHumaError(t, err, 401)
}

func TestUpdateVenueHandler_InvalidID(t *testing.T) {
	h := testVenueHandler()
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &UpdateVenueRequest{VenueID: "abc"}

	_, err := h.UpdateVenueHandler(ctx, req)
	assertHumaError(t, err, 400)
}

// --- GetMyPendingEditHandler ---

func TestGetMyPendingEditHandler_NoAuth(t *testing.T) {
	h := testVenueHandler()
	req := &GetMyPendingEditRequest{VenueID: "1"}

	_, err := h.GetMyPendingEditHandler(context.Background(), req)
	assertHumaError(t, err, 401)
}

func TestGetMyPendingEditHandler_InvalidID(t *testing.T) {
	h := testVenueHandler()
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &GetMyPendingEditRequest{VenueID: "abc"}

	_, err := h.GetMyPendingEditHandler(ctx, req)
	assertHumaError(t, err, 400)
}

// --- CancelMyPendingEditHandler ---

func TestCancelMyPendingEditHandler_NoAuth(t *testing.T) {
	h := testVenueHandler()
	req := &CancelMyPendingEditRequest{VenueID: "1"}

	_, err := h.CancelMyPendingEditHandler(context.Background(), req)
	assertHumaError(t, err, 401)
}

func TestCancelMyPendingEditHandler_InvalidID(t *testing.T) {
	h := testVenueHandler()
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &CancelMyPendingEditRequest{VenueID: "abc"}

	_, err := h.CancelMyPendingEditHandler(ctx, req)
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
