package auth

import (
	"context"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
)

func testOAuthAccountHandler() *OAuthAccountHandler {
	return NewOAuthAccountHandler(nil)
}

// --- GetOAuthAccountsHandler ---

func TestGetOAuthAccountsHandler_NoAuth(t *testing.T) {
	h := testOAuthAccountHandler()
	req := &GetOAuthAccountsRequest{}

	_, err := h.GetOAuthAccountsHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 401)
}

// --- UnlinkOAuthAccountHandler ---

func TestUnlinkOAuthAccountHandler_NoAuth(t *testing.T) {
	h := testOAuthAccountHandler()
	req := &UnlinkOAuthAccountRequest{Provider: "google"}

	_, err := h.UnlinkOAuthAccountHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 401)
}

func TestUnlinkOAuthAccountHandler_InvalidProvider(t *testing.T) {
	h := testOAuthAccountHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &UnlinkOAuthAccountRequest{Provider: "facebook"}

	_, err := h.UnlinkOAuthAccountHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 400)
}
