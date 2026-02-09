package handlers

import (
	"context"
	"testing"

	"psychic-homily-backend/internal/models"
)

func testFavoriteVenueHandler() *FavoriteVenueHandler {
	return NewFavoriteVenueHandler(nil)
}

// --- NewFavoriteVenueHandler ---

func TestNewFavoriteVenueHandler(t *testing.T) {
	h := testFavoriteVenueHandler()
	if h == nil {
		t.Fatal("expected non-nil FavoriteVenueHandler")
	}
}

// --- FavoriteVenueHandler (method) ---

func TestFavoriteVenueHandler_FavoriteVenue_NoAuth(t *testing.T) {
	h := testFavoriteVenueHandler()
	req := &FavoriteVenueRequest{VenueID: "1"}

	_, err := h.FavoriteVenueHandler(context.Background(), req)
	assertHumaError(t, err, 401)
}

func TestFavoriteVenueHandler_FavoriteVenue_InvalidID(t *testing.T) {
	h := testFavoriteVenueHandler()
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &FavoriteVenueRequest{VenueID: "abc"}

	_, err := h.FavoriteVenueHandler(ctx, req)
	assertHumaError(t, err, 400)
}

// --- UnfavoriteVenueHandler ---

func TestFavoriteVenueHandler_UnfavoriteVenue_NoAuth(t *testing.T) {
	h := testFavoriteVenueHandler()
	req := &UnfavoriteVenueRequest{VenueID: "1"}

	_, err := h.UnfavoriteVenueHandler(context.Background(), req)
	assertHumaError(t, err, 401)
}

func TestFavoriteVenueHandler_UnfavoriteVenue_InvalidID(t *testing.T) {
	h := testFavoriteVenueHandler()
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &UnfavoriteVenueRequest{VenueID: "abc"}

	_, err := h.UnfavoriteVenueHandler(ctx, req)
	assertHumaError(t, err, 400)
}

// --- GetFavoriteVenuesHandler ---

func TestFavoriteVenueHandler_GetFavoriteVenues_NoAuth(t *testing.T) {
	h := testFavoriteVenueHandler()
	req := &GetFavoriteVenuesRequest{}

	_, err := h.GetFavoriteVenuesHandler(context.Background(), req)
	assertHumaError(t, err, 401)
}

// --- CheckFavoritedHandler ---

func TestFavoriteVenueHandler_CheckFavorited_NoAuth(t *testing.T) {
	h := testFavoriteVenueHandler()
	req := &CheckFavoritedRequest{VenueID: "1"}

	_, err := h.CheckFavoritedHandler(context.Background(), req)
	assertHumaError(t, err, 401)
}

func TestFavoriteVenueHandler_CheckFavorited_InvalidID(t *testing.T) {
	h := testFavoriteVenueHandler()
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &CheckFavoritedRequest{VenueID: "abc"}

	_, err := h.CheckFavoritedHandler(ctx, req)
	assertHumaError(t, err, 400)
}

// --- GetFavoriteVenueShowsHandler ---

func TestFavoriteVenueHandler_GetFavoriteVenueShows_NoAuth(t *testing.T) {
	h := testFavoriteVenueHandler()
	req := &GetFavoriteVenueShowsRequest{}

	_, err := h.GetFavoriteVenueShowsHandler(context.Background(), req)
	assertHumaError(t, err, 401)
}
