package handlers

import (
	"context"
	"errors"
	"testing"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/engagement"
)

// --- SetFavoriteCitiesHandler ---

func TestSetFavoriteCitiesHandler_NoAuth(t *testing.T) {
	h := NewUserPreferencesHandler(&mockUserService{}, "secret")
	req := &SetFavoriteCitiesRequest{}

	_, err := h.SetFavoriteCitiesHandler(context.Background(), req)
	assertHumaError(t, err, 401)
}

func TestSetFavoriteCitiesHandler_Success(t *testing.T) {
	var calledWith []models.FavoriteCity
	mock := &mockUserService{
		setFavoriteCitiesFn: func(userID uint, cities []models.FavoriteCity) error {
			calledWith = cities
			return nil
		},
	}
	h := NewUserPreferencesHandler(mock, "secret")
	user := &models.User{ID: 1, IsActive: true}
	ctx := ctxWithUser(user)

	req := &SetFavoriteCitiesRequest{}
	req.Body.Cities = []models.FavoriteCity{{City: "Phoenix", State: "AZ"}}

	resp, err := h.SetFavoriteCitiesHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Fatal("expected success=true")
	}
	if len(calledWith) != 1 || calledWith[0].City != "Phoenix" {
		t.Fatalf("expected Phoenix, got %+v", calledWith)
	}
}

func TestSetFavoriteCitiesHandler_NilCities(t *testing.T) {
	mock := &mockUserService{
		setFavoriteCitiesFn: func(userID uint, cities []models.FavoriteCity) error {
			if cities == nil {
				return errors.New("expected non-nil slice")
			}
			return nil
		},
	}
	h := NewUserPreferencesHandler(mock, "secret")
	ctx := ctxWithUser(&models.User{ID: 1})

	req := &SetFavoriteCitiesRequest{}
	// Body.Cities is nil — handler should default to empty slice

	resp, err := h.SetFavoriteCitiesHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Fatal("expected success=true")
	}
}

func TestSetFavoriteCitiesHandler_ServiceError(t *testing.T) {
	mock := &mockUserService{
		setFavoriteCitiesFn: func(userID uint, cities []models.FavoriteCity) error {
			return errors.New("db error")
		},
	}
	h := NewUserPreferencesHandler(mock, "secret")
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &SetFavoriteCitiesRequest{}

	_, err := h.SetFavoriteCitiesHandler(ctx, req)
	assertHumaError(t, err, 422)
}

// --- SetShowRemindersHandler ---

func TestSetShowRemindersHandler_NoAuth(t *testing.T) {
	h := NewUserPreferencesHandler(&mockUserService{}, "secret")
	req := &SetShowRemindersRequest{}

	_, err := h.SetShowRemindersHandler(context.Background(), req)
	assertHumaError(t, err, 401)
}

func TestSetShowRemindersHandler_Success_Enable(t *testing.T) {
	var calledEnabled bool
	mock := &mockUserService{
		setShowRemindersFn: func(userID uint, enabled bool) error {
			calledEnabled = enabled
			return nil
		},
	}
	h := NewUserPreferencesHandler(mock, "secret")
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &SetShowRemindersRequest{}
	req.Body.Enabled = true

	resp, err := h.SetShowRemindersHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success || !resp.Body.ShowReminders {
		t.Fatal("expected success=true and show_reminders=true")
	}
	if !calledEnabled {
		t.Fatal("expected service called with enabled=true")
	}
}

func TestSetShowRemindersHandler_Success_Disable(t *testing.T) {
	mock := &mockUserService{
		setShowRemindersFn: func(userID uint, enabled bool) error {
			return nil
		},
	}
	h := NewUserPreferencesHandler(mock, "secret")
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &SetShowRemindersRequest{}
	req.Body.Enabled = false

	resp, err := h.SetShowRemindersHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success || resp.Body.ShowReminders {
		t.Fatal("expected success=true and show_reminders=false")
	}
}

func TestSetShowRemindersHandler_ServiceError(t *testing.T) {
	mock := &mockUserService{
		setShowRemindersFn: func(userID uint, enabled bool) error {
			return errors.New("db error")
		},
	}
	h := NewUserPreferencesHandler(mock, "secret")
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &SetShowRemindersRequest{}
	req.Body.Enabled = true

	_, err := h.SetShowRemindersHandler(ctx, req)
	assertHumaError(t, err, 422)
}

// --- UnsubscribeShowRemindersHandler ---

func TestUnsubscribeShowRemindersHandler_InvalidSignature(t *testing.T) {
	h := NewUserPreferencesHandler(&mockUserService{}, "secret")
	req := &UnsubscribeShowRemindersRequest{}
	req.Body.UID = 1
	req.Body.Sig = "invalid-sig"

	_, err := h.UnsubscribeShowRemindersHandler(context.Background(), req)
	assertHumaError(t, err, 403)
}

func TestUnsubscribeShowRemindersHandler_ServiceError(t *testing.T) {
	// Use the real HMAC function to generate a valid signature
	secret := "test-jwt-secret"
	uid := uint(42)
	sig := computeTestUnsubscribeSig(uid, secret)

	mock := &mockUserService{
		setShowRemindersFn: func(userID uint, enabled bool) error {
			return errors.New("db error")
		},
	}
	h := NewUserPreferencesHandler(mock, secret)
	req := &UnsubscribeShowRemindersRequest{}
	req.Body.UID = uid
	req.Body.Sig = sig

	_, err := h.UnsubscribeShowRemindersHandler(context.Background(), req)
	assertHumaError(t, err, 500)
}

// computeTestUnsubscribeSig generates a valid HMAC signature for testing
func computeTestUnsubscribeSig(uid uint, secret string) string {
	return engagement.ComputeUnsubscribeSignature(uid, secret)
}
