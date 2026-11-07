package auth

import (
	"context"
	"net/http"
	"testing"
	"time"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/config"
	authm "psychic-homily-backend/internal/models/auth"
)

func profileHandlerFor(user *authm.User) *AuthHandler {
	return authHandler(func(ah *AuthHandler) {
		ah.authService = &testhelpers.MockAuthService{
			GetUserProfileFn: func(userID uint) (*authm.User, error) { return user, nil },
		}
	})
}

// The profile asks for a renewal only once the session is older than
// SessionRenewalAfter: one request a day per returning viewer, a clock that
// lags their last visit by less than that.
func TestGetProfileHandler_SessionRenewalDue(t *testing.T) {
	user := &authm.User{ID: 1, Email: strPtr("test@example.com")}
	now := time.Now()
	cases := map[string]struct {
		issuedAt time.Time
		want     bool
	}{
		"minted just now":                 {now, false},
		"just inside the threshold":       {now.Add(-SessionRenewalAfter + time.Minute), false},
		"just past the threshold":         {now.Add(-SessionRenewalAfter - time.Minute), true},
		"twenty days old":                 {now.Add(-20 * 24 * time.Hour), true},
		"age unknown (no believable iat)": {time.Time{}, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ctx := testhelpers.CtxWithSessionIssuedAt(&authm.User{ID: 1}, now.Add(-time.Hour), tc.issuedAt)
			resp, err := profileHandlerFor(user).GetProfileHandler(ctx, &struct{}{})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.Body.SessionRenewalDue != tc.want {
				t.Errorf("session_renewal_due = %v, want %v", resp.Body.SessionRenewalDue, tc.want)
			}
		})
	}
}

// An API token is never asked to renew, even if a context somehow carried an
// age for it: the session check stands on its own.
func TestGetProfileHandler_APITokenIsNeverDueForRenewal(t *testing.T) {
	user := &authm.User{ID: 1, Email: strPtr("test@example.com")}
	ctx := context.WithValue(testhelpers.CtxWithUser(&authm.User{ID: 1}),
		middleware.SessionIssuedAtContextKey, time.Now().Add(-20*24*time.Hour))
	resp, err := profileHandlerFor(user).GetProfileHandler(ctx, &struct{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.SessionRenewalDue {
		t.Error("an API-token principal must not be asked to renew")
	}
}

func refreshHandler() *AuthHandler {
	return authHandler(func(ah *AuthHandler) {
		ah.authService = &testhelpers.MockAuthService{
			GetUserProfileFn: func(userID uint) (*authm.User, error) {
				return &authm.User{ID: userID, IsActive: true}, nil
			},
			RefreshUserTokenFn: func(user *authm.User, authAt time.Time) (string, error) {
				return "renewed-token", nil
			},
		}
	})
}

// A browser holds the session as an HttpOnly cookie, so the renewal goes back
// as Set-Cookie and never into the body, where script could read it.
func TestRefreshTokenHandler_CookieSessionIsRenewedInTheCookieOnly(t *testing.T) {
	h := refreshHandler()
	ctx := testhelpers.CtxWithCookieSession(&authm.User{ID: 1}, time.Now().Add(-time.Hour), time.Now().Add(-25*time.Hour))

	resp, err := h.RefreshTokenHandler(ctx, &struct{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true")
	}
	if resp.Body.Token != "" {
		t.Errorf("body token = %q, want none for a cookie credential", resp.Body.Token)
	}
	cookie, err := http.ParseSetCookie(resp.SetCookie)
	if err != nil {
		t.Fatalf("Set-Cookie %q does not parse: %v", resp.SetCookie, err)
	}
	if cookie.Name != config.AuthCookieName || cookie.Value != "renewed-token" {
		t.Fatalf("Set-Cookie = %q, want the renewed session", resp.SetCookie)
	}
	if !cookie.HttpOnly {
		t.Error("the renewed cookie must stay HttpOnly")
	}
	want := h.config.JWT.SessionLifetime()
	if lifetime := time.Until(cookie.Expires); lifetime < want-time.Minute || lifetime > want {
		t.Errorf("cookie lifetime = %v, want the configured session lifetime %v (the clock restarts)", lifetime, want)
	}
}
