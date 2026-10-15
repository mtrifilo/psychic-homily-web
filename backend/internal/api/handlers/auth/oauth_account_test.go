package auth

import (
	"context"
	"testing"
	"time"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
)

func testOAuthAccountHandler() *OAuthAccountHandler {
	return NewOAuthAccountHandler(nil, "test-secret", "http://localhost:3000")
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

	h := NewOAuthAccountHandler(mockUserService, "test-secret", "http://localhost:3000")
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

// --- StartOAuthLinkHandler ---

func TestStartOAuthLinkHandler_NoAuth(t *testing.T) {
	h := testOAuthAccountHandler()

	_, err := h.StartOAuthLinkHandler(context.Background(), &StartOAuthLinkRequest{Origin: "http://localhost:3000"})
	testhelpers.AssertHumaError(t, err, 401)
}

// The mint answers only a request that names the configured frontend, by
// Origin or, when Origin is absent or opaque, by Referer. A request naming
// neither is refused: nothing then says where it came from.
func TestStartOAuthLinkHandler_OriginGate(t *testing.T) {
	const frontend = "http://localhost:3000"

	cases := []struct {
		name     string
		origin   string
		referer  string
		wantMint bool
	}{
		{"no Origin and no Referer", "", "", false},
		{"our Origin", frontend, "", true},
		{"our Referer alone", "", frontend + "/profile?tab=settings", true},
		{"opaque Origin with our Referer", "null", frontend + "/profile", true},
		{"another Origin", "https://evil.example", "", false},
		{"another Referer alone", "", "https://evil.example/profile", false},
		// Origin outranks Referer: a foreign Origin is not rescued by ours.
		{"another Origin with our Referer", "https://evil.example", frontend + "/profile", false},
		{"opaque Origin and nothing else", "null", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := testOAuthAccountHandler()
			user := &authm.User{ID: 7}
			ctx := testhelpers.CtxWithUser(user)

			resp, err := h.StartOAuthLinkHandler(ctx, &StartOAuthLinkRequest{Origin: tc.origin, Referer: tc.referer})
			if !tc.wantMint {
				testhelpers.AssertHumaErrorWithDetail(t, err, 403, "Start the connection from Settings")
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !resp.Body.Success || resp.Body.Token == "" {
				t.Fatalf("expected a minted token, got success=%v token=%q", resp.Body.Success, resp.Body.Token)
			}
			if !consumeOAuthLinkToken("test-secret", resp.Body.Token, user.ID) {
				t.Error("minted token does not spend for the user it was minted for")
			}
		})
	}
}
