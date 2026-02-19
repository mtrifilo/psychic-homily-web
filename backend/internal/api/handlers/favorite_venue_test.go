package handlers

import (
	"context"
	"fmt"
	"testing"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services"
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

func TestFavoriteVenueHandler_FavoriteVenue_Success(t *testing.T) {
	mock := &mockFavoriteVenueService{
		favoriteVenueFn: func(userID, venueID uint) error {
			if userID != 1 || venueID != 5 {
				t.Errorf("unexpected args: userID=%d, venueID=%d", userID, venueID)
			}
			return nil
		},
	}
	h := NewFavoriteVenueHandler(mock)
	ctx := ctxWithUser(&models.User{ID: 1})

	resp, err := h.FavoriteVenueHandler(ctx, &FavoriteVenueRequest{VenueID: "5"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true")
	}
}

func TestFavoriteVenueHandler_FavoriteVenue_ServiceError(t *testing.T) {
	mock := &mockFavoriteVenueService{
		favoriteVenueFn: func(_, _ uint) error {
			return fmt.Errorf("already favorited")
		},
	}
	h := NewFavoriteVenueHandler(mock)
	ctx := ctxWithUser(&models.User{ID: 1})

	_, err := h.FavoriteVenueHandler(ctx, &FavoriteVenueRequest{VenueID: "5"})
	assertHumaError(t, err, 422)
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

func TestFavoriteVenueHandler_UnfavoriteVenue_Success(t *testing.T) {
	mock := &mockFavoriteVenueService{
		unfavoriteVenueFn: func(userID, venueID uint) error {
			if userID != 1 || venueID != 5 {
				t.Errorf("unexpected args: userID=%d, venueID=%d", userID, venueID)
			}
			return nil
		},
	}
	h := NewFavoriteVenueHandler(mock)
	ctx := ctxWithUser(&models.User{ID: 1})

	resp, err := h.UnfavoriteVenueHandler(ctx, &UnfavoriteVenueRequest{VenueID: "5"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true")
	}
}

func TestFavoriteVenueHandler_UnfavoriteVenue_ServiceError(t *testing.T) {
	mock := &mockFavoriteVenueService{
		unfavoriteVenueFn: func(_, _ uint) error {
			return fmt.Errorf("not favorited")
		},
	}
	h := NewFavoriteVenueHandler(mock)
	ctx := ctxWithUser(&models.User{ID: 1})

	_, err := h.UnfavoriteVenueHandler(ctx, &UnfavoriteVenueRequest{VenueID: "5"})
	assertHumaError(t, err, 422)
}

// --- GetFavoriteVenuesHandler ---

func TestFavoriteVenueHandler_GetFavoriteVenues_NoAuth(t *testing.T) {
	h := testFavoriteVenueHandler()
	req := &GetFavoriteVenuesRequest{}

	_, err := h.GetFavoriteVenuesHandler(context.Background(), req)
	assertHumaError(t, err, 401)
}

func TestFavoriteVenueHandler_GetFavoriteVenues_Success(t *testing.T) {
	venues := []*services.FavoriteVenueResponse{{}}
	mock := &mockFavoriteVenueService{
		getUserFavoritesFn: func(userID uint, limit, offset int) ([]*services.FavoriteVenueResponse, int64, error) {
			if userID != 1 {
				t.Errorf("unexpected userID=%d", userID)
			}
			return venues, 1, nil
		},
	}
	h := NewFavoriteVenueHandler(mock)
	ctx := ctxWithUser(&models.User{ID: 1})

	resp, err := h.GetFavoriteVenuesHandler(ctx, &GetFavoriteVenuesRequest{Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Total != 1 {
		t.Errorf("expected total=1, got %d", resp.Body.Total)
	}
	if len(resp.Body.Venues) != 1 {
		t.Errorf("expected 1 venue, got %d", len(resp.Body.Venues))
	}
}

func TestFavoriteVenueHandler_GetFavoriteVenues_ServiceError(t *testing.T) {
	mock := &mockFavoriteVenueService{
		getUserFavoritesFn: func(_ uint, _, _ int) ([]*services.FavoriteVenueResponse, int64, error) {
			return nil, 0, fmt.Errorf("db error")
		},
	}
	h := NewFavoriteVenueHandler(mock)
	ctx := ctxWithUser(&models.User{ID: 1})

	_, err := h.GetFavoriteVenuesHandler(ctx, &GetFavoriteVenuesRequest{Limit: 10})
	assertHumaError(t, err, 500)
}

func TestFavoriteVenueHandler_GetFavoriteVenues_PaginationClamping(t *testing.T) {
	var capturedLimit, capturedOffset int
	mock := &mockFavoriteVenueService{
		getUserFavoritesFn: func(_ uint, limit, offset int) ([]*services.FavoriteVenueResponse, int64, error) {
			capturedLimit = limit
			capturedOffset = offset
			return nil, 0, nil
		},
	}
	h := NewFavoriteVenueHandler(mock)
	ctx := ctxWithUser(&models.User{ID: 1})

	// limit=0 → 50, offset=-1 → 0
	resp, err := h.GetFavoriteVenuesHandler(ctx, &GetFavoriteVenuesRequest{Limit: 0, Offset: -1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedLimit != 50 {
		t.Errorf("expected limit clamped to 50, got %d", capturedLimit)
	}
	if capturedOffset != 0 {
		t.Errorf("expected offset clamped to 0, got %d", capturedOffset)
	}
	if resp.Body.Limit != 50 {
		t.Errorf("expected response limit=50, got %d", resp.Body.Limit)
	}

	// limit=999 → 200
	h.GetFavoriteVenuesHandler(ctx, &GetFavoriteVenuesRequest{Limit: 999})
	if capturedLimit != 200 {
		t.Errorf("expected limit clamped to 200, got %d", capturedLimit)
	}
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

func TestFavoriteVenueHandler_CheckFavorited_True(t *testing.T) {
	mock := &mockFavoriteVenueService{
		isVenueFavoritedFn: func(_, _ uint) (bool, error) {
			return true, nil
		},
	}
	h := NewFavoriteVenueHandler(mock)
	ctx := ctxWithUser(&models.User{ID: 1})

	resp, err := h.CheckFavoritedHandler(ctx, &CheckFavoritedRequest{VenueID: "5"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.IsFavorited {
		t.Error("expected is_favorited=true")
	}
}

func TestFavoriteVenueHandler_CheckFavorited_False(t *testing.T) {
	mock := &mockFavoriteVenueService{
		isVenueFavoritedFn: func(_, _ uint) (bool, error) {
			return false, nil
		},
	}
	h := NewFavoriteVenueHandler(mock)
	ctx := ctxWithUser(&models.User{ID: 1})

	resp, err := h.CheckFavoritedHandler(ctx, &CheckFavoritedRequest{VenueID: "5"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.IsFavorited {
		t.Error("expected is_favorited=false")
	}
}

func TestFavoriteVenueHandler_CheckFavorited_ServiceError(t *testing.T) {
	mock := &mockFavoriteVenueService{
		isVenueFavoritedFn: func(_, _ uint) (bool, error) {
			return false, fmt.Errorf("db error")
		},
	}
	h := NewFavoriteVenueHandler(mock)
	ctx := ctxWithUser(&models.User{ID: 1})

	_, err := h.CheckFavoritedHandler(ctx, &CheckFavoritedRequest{VenueID: "5"})
	assertHumaError(t, err, 500)
}

// --- GetFavoriteVenueShowsHandler ---

func TestFavoriteVenueHandler_GetFavoriteVenueShows_NoAuth(t *testing.T) {
	h := testFavoriteVenueHandler()
	req := &GetFavoriteVenueShowsRequest{}

	_, err := h.GetFavoriteVenueShowsHandler(context.Background(), req)
	assertHumaError(t, err, 401)
}

func TestFavoriteVenueHandler_GetFavoriteVenueShows_Success(t *testing.T) {
	shows := []*services.FavoriteVenueShowResponse{{}}
	mock := &mockFavoriteVenueService{
		getUpcomingShowsFn: func(userID uint, timezone string, limit, offset int) ([]*services.FavoriteVenueShowResponse, int64, error) {
			if userID != 1 {
				t.Errorf("unexpected userID=%d", userID)
			}
			if timezone != "US/Eastern" {
				t.Errorf("unexpected timezone=%s", timezone)
			}
			return shows, 1, nil
		},
	}
	h := NewFavoriteVenueHandler(mock)
	ctx := ctxWithUser(&models.User{ID: 1})

	resp, err := h.GetFavoriteVenueShowsHandler(ctx, &GetFavoriteVenueShowsRequest{Timezone: "US/Eastern", Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Total != 1 {
		t.Errorf("expected total=1, got %d", resp.Body.Total)
	}
	if resp.Body.Timezone != "US/Eastern" {
		t.Errorf("expected timezone=US/Eastern, got %s", resp.Body.Timezone)
	}
}

func TestFavoriteVenueHandler_GetFavoriteVenueShows_ServiceError(t *testing.T) {
	mock := &mockFavoriteVenueService{
		getUpcomingShowsFn: func(_ uint, _ string, _, _ int) ([]*services.FavoriteVenueShowResponse, int64, error) {
			return nil, 0, fmt.Errorf("db error")
		},
	}
	h := NewFavoriteVenueHandler(mock)
	ctx := ctxWithUser(&models.User{ID: 1})

	_, err := h.GetFavoriteVenueShowsHandler(ctx, &GetFavoriteVenueShowsRequest{Timezone: "US/Eastern", Limit: 10})
	assertHumaError(t, err, 500)
}

func TestFavoriteVenueHandler_GetFavoriteVenueShows_DefaultTimezone(t *testing.T) {
	var capturedTZ string
	mock := &mockFavoriteVenueService{
		getUpcomingShowsFn: func(_ uint, timezone string, _, _ int) ([]*services.FavoriteVenueShowResponse, int64, error) {
			capturedTZ = timezone
			return nil, 0, nil
		},
	}
	h := NewFavoriteVenueHandler(mock)
	ctx := ctxWithUser(&models.User{ID: 1})

	resp, err := h.GetFavoriteVenueShowsHandler(ctx, &GetFavoriteVenueShowsRequest{Timezone: "", Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedTZ != "America/Phoenix" {
		t.Errorf("expected default timezone America/Phoenix, got %s", capturedTZ)
	}
	if resp.Body.Timezone != "America/Phoenix" {
		t.Errorf("expected response timezone=America/Phoenix, got %s", resp.Body.Timezone)
	}
}
