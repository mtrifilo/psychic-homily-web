package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/config"
	autherrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/shared"
)

// signOutRecorder records what the handler hands the service.
type signOutRecorder struct {
	calls  int
	user   *authm.User
	authAt time.Time
	token  string
	err    error
}

func (r *signOutRecorder) fn(user *authm.User, authAt time.Time) (string, error) {
	r.calls++
	r.user, r.authAt = user, authAt
	return r.token, r.err
}

func signOutHandler(rec *signOutRecorder) *AuthHandler {
	return authHandler(func(ah *AuthHandler) {
		ah.authService = &testhelpers.MockAuthService{SignOutOtherSessionsFn: rec.fn}
	})
}

func signedInUser() *authm.User {
	stamp := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	email := "signed-in@example.com"
	return &authm.User{ID: 42, Email: &email, IsActive: true, SessionsValidAfter: &stamp}
}

func TestSignOutEverywhereElseHandler_RenewsTheCallersSession(t *testing.T) {
	rec := &signOutRecorder{token: "renewed-token"}
	h := signOutHandler(rec)
	user := signedInUser()
	authAt := time.Now().Add(-3 * time.Hour).Truncate(time.Second)

	resp, err := h.SignOutEverywhereElseHandler(testhelpers.CtxWithSessionAuthTime(user, authAt), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Fatal("expected success")
	}
	// The principal validation loaded, whose stamp is the session's, and the
	// session's own authentication time: the control proves no factor, so the
	// renewal must not buy freshness, and it is not refused for lacking it.
	if rec.calls != 1 || rec.user != user || !rec.authAt.Equal(authAt) {
		t.Errorf("service got calls=%d user=%p authAt=%v, want 1, %p, %v", rec.calls, rec.user, rec.authAt, user, authAt)
	}
	if resp.SetCookie.Name != config.AuthCookieName || resp.SetCookie.Value != "renewed-token" {
		t.Errorf("cookie = %s=%q, want the renewed session", resp.SetCookie.Name, resp.SetCookie.Value)
	}
	if lifetime := time.Until(resp.SetCookie.Expires); lifetime < 23*time.Hour || lifetime > 25*time.Hour {
		t.Errorf("cookie lifetime = %v, want 24 hours, as change-password sets", lifetime)
	}
}

// A refusal that leaves the caller's session revoked, or its account closed,
// is answered as the JWT middleware answers that session next: 401
// TOKEN_INVALID, clearing the cookie.
func TestSignOutEverywhereElseHandler_SessionRefusalsClearTheCookie(t *testing.T) {
	closed := autherrors.ErrAccountInactive()
	closed.Internal = fmt.Errorf("user 42: %w", shared.ErrSessionAccountClosed)
	for name, refusal := range map[string]error{
		"revoked since validation": autherrors.ErrTokenInvalid(fmt.Errorf("user 42: %w", shared.ErrSessionRevoked)),
		"closed since validation":  closed,
	} {
		t.Run(name, func(t *testing.T) {
			h := signOutHandler(&signOutRecorder{err: refusal})

			resp, err := h.SignOutEverywhereElseHandler(testhelpers.CtxWithSessionAuthTime(signedInUser(), time.Now()), nil)
			if resp != nil {
				t.Errorf("a refusal must carry no response, got %+v", resp)
			}
			var statusErr huma.StatusError
			if !errors.As(err, &statusErr) || statusErr.GetStatus() != http.StatusUnauthorized {
				t.Fatalf("want a 401 refusal, got %v", err)
			}
			var refused *middleware.SessionRefusedError
			if !errors.As(err, &refused) || refused.ErrorCode != autherrors.CodeTokenInvalid {
				t.Fatalf("want TOKEN_INVALID, got %v", err)
			}
			var headers huma.HeadersError
			if !errors.As(err, &headers) || !strings.Contains(headers.GetHeaders().Get("Set-Cookie"), config.AuthCookieName+"=") {
				t.Error("the refusal must clear the auth cookie")
			}
		})
	}
}

// Any other failure is a 5xx with no cookie: nothing was revoked, so the
// caller keeps the session it came with.
func TestSignOutEverywhereElseHandler_ServiceFailureIsUnavailable(t *testing.T) {
	h := signOutHandler(&signOutRecorder{err: errors.New("database unreachable")})

	resp, err := h.SignOutEverywhereElseHandler(testhelpers.CtxWithSessionAuthTime(signedInUser(), time.Now()), nil)
	if resp != nil {
		t.Errorf("a failure must carry no response, got %+v", resp)
	}
	assertServiceUnavailable(t, err)
}

func TestSignOutEverywhereElseHandler_NoUserIsUnauthorized(t *testing.T) {
	rec := &signOutRecorder{}
	h := signOutHandler(rec)

	_, err := h.SignOutEverywhereElseHandler(context.Background(), nil)
	var statusErr huma.StatusError
	if !errors.As(err, &statusErr) || statusErr.GetStatus() != http.StatusUnauthorized {
		t.Fatalf("want a 401, got %v", err)
	}
	if rec.calls != 0 {
		t.Error("no session may be revoked without a principal")
	}
}
