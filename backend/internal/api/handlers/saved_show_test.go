package handlers

import (
	"context"
	"testing"

	"psychic-homily-backend/internal/models"
)

func testSavedShowHandler() *SavedShowHandler {
	return NewSavedShowHandler(nil)
}

// --- NewSavedShowHandler ---

func TestNewSavedShowHandler(t *testing.T) {
	h := testSavedShowHandler()
	if h == nil {
		t.Fatal("expected non-nil SavedShowHandler")
	}
}

// --- SaveShowHandler ---

func TestSaveShowHandler_NoAuth(t *testing.T) {
	h := testSavedShowHandler()
	req := &SaveShowRequest{ShowID: "1"}

	_, err := h.SaveShowHandler(context.Background(), req)
	assertHumaError(t, err, 401)
}

func TestSaveShowHandler_InvalidID(t *testing.T) {
	h := testSavedShowHandler()
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &SaveShowRequest{ShowID: "abc"}

	_, err := h.SaveShowHandler(ctx, req)
	assertHumaError(t, err, 400)
}

// --- UnsaveShowHandler ---

func TestUnsaveShowHandler_NoAuth(t *testing.T) {
	h := testSavedShowHandler()
	req := &UnsaveShowRequest{ShowID: "1"}

	_, err := h.UnsaveShowHandler(context.Background(), req)
	assertHumaError(t, err, 401)
}

func TestUnsaveShowHandler_InvalidID(t *testing.T) {
	h := testSavedShowHandler()
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &UnsaveShowRequest{ShowID: "abc"}

	_, err := h.UnsaveShowHandler(ctx, req)
	assertHumaError(t, err, 400)
}

// --- GetSavedShowsHandler ---

func TestGetSavedShowsHandler_NoAuth(t *testing.T) {
	h := testSavedShowHandler()
	req := &GetSavedShowsRequest{}

	_, err := h.GetSavedShowsHandler(context.Background(), req)
	assertHumaError(t, err, 401)
}

// --- CheckSavedHandler ---

func TestCheckSavedHandler_NoAuth(t *testing.T) {
	h := testSavedShowHandler()
	req := &CheckSavedRequest{ShowID: "1"}

	_, err := h.CheckSavedHandler(context.Background(), req)
	assertHumaError(t, err, 401)
}

func TestCheckSavedHandler_InvalidID(t *testing.T) {
	h := testSavedShowHandler()
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &CheckSavedRequest{ShowID: "abc"}

	_, err := h.CheckSavedHandler(ctx, req)
	assertHumaError(t, err, 400)
}
