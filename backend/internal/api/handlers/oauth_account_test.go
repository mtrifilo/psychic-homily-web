package handlers

import (
	"context"
	"testing"

	"psychic-homily-backend/internal/models"
)

func testOAuthAccountHandler() *OAuthAccountHandler {
	return NewOAuthAccountHandler(nil)
}

// --- NewOAuthAccountHandler ---

func TestNewOAuthAccountHandler(t *testing.T) {
	h := testOAuthAccountHandler()
	if h == nil {
		t.Fatal("expected non-nil OAuthAccountHandler")
	}
}

// --- GetOAuthAccountsHandler ---

func TestGetOAuthAccountsHandler_NoAuth(t *testing.T) {
	h := testOAuthAccountHandler()
	req := &GetOAuthAccountsRequest{}

	_, err := h.GetOAuthAccountsHandler(context.Background(), req)
	assertHumaError(t, err, 401)
}

// --- UnlinkOAuthAccountHandler ---

func TestUnlinkOAuthAccountHandler_NoAuth(t *testing.T) {
	h := testOAuthAccountHandler()
	req := &UnlinkOAuthAccountRequest{Provider: "google"}

	_, err := h.UnlinkOAuthAccountHandler(context.Background(), req)
	assertHumaError(t, err, 401)
}

func TestUnlinkOAuthAccountHandler_InvalidProvider(t *testing.T) {
	h := testOAuthAccountHandler()
	ctx := ctxWithUser(&models.User{ID: 1})
	req := &UnlinkOAuthAccountRequest{Provider: "facebook"}

	_, err := h.UnlinkOAuthAccountHandler(ctx, req)
	assertHumaError(t, err, 400)
}
