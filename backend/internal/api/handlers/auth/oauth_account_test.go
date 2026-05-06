package auth

import (
	"context"
	"testing"
	"time"

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

// TestGetOAuthAccountsHandler_ConnectedAtIsUTC is the PSY-616 regression
// guard (sibling of PSY-604's TestRevisionHandler_GetEntityHistory_CreatedAtIsUTC).
// Before the fix, an OAuth account whose CreatedAt was a local time.Time
// (e.g. served from a DB driver that returns timestamptz in the session
// TZ) was formatted via t.Format("2006-01-02T15:04:05Z") — Format does
// NOT convert to UTC, so the literal "Z" in the layout asserted UTC while
// the value still carried the local clock reading. The fix is to call
// .UTC() before .Format(time.RFC3339) on the field. This test asserts the
// response field reflects the UTC equivalent of the input time, not the
// local clock.
func TestGetOAuthAccountsHandler_ConnectedAtIsUTC(t *testing.T) {
	// 13:00 Phoenix MST (UTC-7) == 20:00 UTC
	phoenix, err := time.LoadLocation("America/Phoenix")
	if err != nil {
		t.Fatalf("failed to load Phoenix location: %v", err)
	}
	localTime := time.Date(2026, 5, 4, 13, 0, 0, 0, phoenix)

	mockUserService := &testhelpers.MockUserService{
		GetOAuthAccountsFn: func(userID uint) ([]authm.OAuthAccount, error) {
			return []authm.OAuthAccount{{
				ID:        1,
				UserID:    userID,
				Provider:  "google",
				CreatedAt: localTime,
			}}, nil
		},
	}

	h := NewOAuthAccountHandler(mockUserService)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	resp, err := h.GetOAuthAccountsHandler(ctx, &GetOAuthAccountsRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Body.Accounts) != 1 {
		t.Fatalf("expected 1 account, got %d", len(resp.Body.Accounts))
	}

	got := resp.Body.Accounts[0].ConnectedAt
	want := "2026-05-04T20:00:00Z"
	if got != want {
		t.Errorf("ConnectedAt timezone drift: got %q, want %q (input was 13:00 Phoenix == 20:00 UTC)", got, want)
	}
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
	testhelpers.AssertHumaError(t, err, 422)
}
