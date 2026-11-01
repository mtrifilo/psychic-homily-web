package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/config"
	autherrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
)

func setPasswordInput(password string) *SetPasswordRequest {
	input := &SetPasswordRequest{}
	input.Body.NewPassword = password
	return input
}

// passwordlessUser is an account that signs in with a provider and has no
// password, the only shape the route serves.
func passwordlessUser() *authm.User {
	stamp := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	return &authm.User{ID: 42, IsActive: true, SessionsValidAfter: &stamp}
}

// setPasswordRecorder records what the handler hands the service.
type setPasswordRecorder struct {
	calls    int
	userID   uint
	password string
	stamp    *time.Time
	err      error
}

func (r *setPasswordRecorder) fn(userID uint, password string, stamp *time.Time) error {
	r.calls++
	r.userID, r.password, r.stamp = userID, password, stamp
	return r.err
}

func setPasswordHandler(rec *setPasswordRecorder, opts ...func(*AuthHandler)) *AuthHandler {
	base := func(ah *AuthHandler) {
		ah.userService = &testhelpers.MockUserService{SetPasswordFn: rec.fn}
		ah.jwtService = &testhelpers.MockJWTService{
			RenewSessionTokenFn: func(*authm.User, time.Time) (string, error) { return "renewed-token", nil },
		}
	}
	return authHandler(append([]func(*AuthHandler){base}, opts...)...)
}

func TestSetPasswordHandler_FreshSessionSetsThePassword(t *testing.T) {
	rec := &setPasswordRecorder{}
	var renewedAt time.Time
	h := setPasswordHandler(rec, func(ah *AuthHandler) {
		ah.jwtService = &testhelpers.MockJWTService{
			RenewSessionTokenFn: func(_ *authm.User, authAt time.Time) (string, error) {
				renewedAt = authAt
				return "renewed-token", nil
			},
		}
	})
	user := passwordlessUser()
	authAt := time.Now().Add(-1 * time.Minute).Truncate(time.Second)

	resp, err := h.SetPasswordHandler(testhelpers.CtxWithSessionAuthTime(user, authAt), setPasswordInput("a-new-strong-password-1!"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Fatalf("expected success, got %q (%s)", resp.Body.Message, resp.Body.ErrorCode)
	}
	if rec.calls != 1 || rec.userID != user.ID || rec.password != "a-new-strong-password-1!" {
		t.Errorf("service got calls=%d user=%d password=%q", rec.calls, rec.userID, rec.password)
	}
	// The stamp the session was validated against, so the service can refuse a
	// session revoked since.
	if rec.stamp != user.SessionsValidAfter {
		t.Errorf("service got stamp %v, want the session's %v", rec.stamp, user.SessionsValidAfter)
	}
	// Renewed with the time the session already had: setting a password proves
	// no factor, so it must not buy freshness.
	if !renewedAt.Equal(authAt) {
		t.Errorf("session renewed with auth time %v, want the arriving %v", renewedAt, authAt)
	}
	if resp.SetCookie.Name != config.AuthCookieName || resp.SetCookie.Value != "renewed-token" {
		t.Errorf("cookie = %s=%q, want the renewed session", resp.SetCookie.Name, resp.SetCookie.Value)
	}
}

// A stale session is refused before anything is validated or written, and the
// log names the factor a passwordless account is asked for.
func TestSetPasswordHandler_StaleSessionRefusedForReauth(t *testing.T) {
	for name, authAt := range map[string]time.Time{
		"stale session":                 time.Now().Add(-2 * time.Hour),
		"no authentication time at all": {},
	} {
		t.Run(name, func(t *testing.T) {
			rec := &setPasswordRecorder{}
			validated := false
			h := setPasswordHandler(rec, func(ah *AuthHandler) {
				ah.passwordValidator = &testhelpers.MockPasswordValidator{
					ValidatePasswordFn: func(string) (*contracts.PasswordValidationResult, error) {
						validated = true
						return &contracts.PasswordValidationResult{Valid: true}, nil
					},
				}
			})

			var buf bytes.Buffer
			ctx := logger.NewContext(
				testhelpers.CtxWithSessionAuthTime(passwordlessUser(), authAt),
				slog.New(slog.NewJSONHandler(&buf, nil)),
			)
			_, err := h.SetPasswordHandler(ctx, setPasswordInput("a-new-strong-password-1!"))

			var refusal *middleware.ReauthRequiredError
			if !errors.As(err, &refusal) {
				t.Fatalf("expected a re-authentication refusal, got %v", err)
			}
			if refusal.GetStatus() != http.StatusForbidden || refusal.ErrorCode != autherrors.CodeReauthRequired {
				t.Errorf("refusal = %d %s, want 403 %s", refusal.GetStatus(), refusal.ErrorCode, autherrors.CodeReauthRequired)
			}
			if rec.calls != 0 || validated {
				t.Errorf("a refused request reached the service (%d calls) or the validator (%v)", rec.calls, validated)
			}

			var line map[string]any
			if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
				t.Fatalf("want exactly one JSON log line: %v: %s", err, buf.String())
			}
			if line["operation"] != setPasswordOperation {
				t.Errorf("logged operation %v, want %q", line["operation"], setPasswordOperation)
			}
			if line["required_factor"] != string(middleware.ReauthMagicLink) {
				t.Errorf("logged required_factor %v, want %q for an account with no password",
					line["required_factor"], middleware.ReauthMagicLink)
			}
		})
	}
}

func TestSetPasswordHandler_AccountWithPasswordAnswersAlreadySet(t *testing.T) {
	rec := &setPasswordRecorder{}
	h := setPasswordHandler(rec)
	hash := "$2a$not-a-real-hash"
	user := &authm.User{ID: 7, IsActive: true, PasswordHash: &hash}

	resp, err := h.SetPasswordHandler(testhelpers.CtxWithSessionAuthTime(user, time.Now()), setPasswordInput("a-new-strong-password-1!"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertSetPasswordRefused(t, resp, autherrors.CodePasswordAlreadySet,
		"This account already has a password. Reload the page to change it.")
	if rec.calls != 0 {
		t.Error("an account that holds a password must not reach the service")
	}
	if resp.SetCookie.Value != "" {
		t.Error("a refusal must not set a session cookie")
	}
}

// The race the service decides: the session's principal had no password when
// it was loaded, and another request set one first.
func TestSetPasswordHandler_ServiceAlreadySetIsTheRefusal(t *testing.T) {
	rec := &setPasswordRecorder{err: autherrors.ErrPasswordAlreadySet()}
	h := setPasswordHandler(rec)

	resp, err := h.SetPasswordHandler(testhelpers.CtxWithSessionAuthTime(passwordlessUser(), time.Now()), setPasswordInput("a-new-strong-password-1!"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertSetPasswordRefused(t, resp, autherrors.CodePasswordAlreadySet, "")
	if resp.SetCookie.Value != "" {
		t.Error("a refusal must not set a session cookie")
	}
}

func TestSetPasswordHandler_ValidatorRefusalIsTheValidatorsWords(t *testing.T) {
	for _, message := range []string{
		"Password must be at least 12 characters",
		"This password has been exposed in a data breach and should not be used",
	} {
		t.Run(message, func(t *testing.T) {
			rec := &setPasswordRecorder{}
			h := setPasswordHandler(rec, func(ah *AuthHandler) {
				ah.passwordValidator = &testhelpers.MockPasswordValidator{
					ValidatePasswordFn: func(string) (*contracts.PasswordValidationResult, error) {
						return &contracts.PasswordValidationResult{Valid: false, Errors: []string{message, "a second reason"}}, nil
					},
				}
			})

			resp, err := h.SetPasswordHandler(testhelpers.CtxWithSessionAuthTime(passwordlessUser(), time.Now()), setPasswordInput("password-value"))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assertSetPasswordRefused(t, resp, autherrors.CodeValidationFailed, message)
			if rec.calls != 0 {
				t.Error("a refused password must not be written")
			}
		})
	}
}

func TestSetPasswordHandler_EmptyPasswordRefused(t *testing.T) {
	rec := &setPasswordRecorder{}
	h := setPasswordHandler(rec)

	resp, err := h.SetPasswordHandler(testhelpers.CtxWithSessionAuthTime(passwordlessUser(), time.Now()), setPasswordInput(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertSetPasswordRefused(t, resp, autherrors.CodeValidationFailed, "New password is required")
	if rec.calls != 0 {
		t.Error("an empty password must not be written")
	}
}

// The validator is the route's only strength check, so its absence or failure
// refuses with a 5xx rather than writing an unchecked password.
func TestSetPasswordHandler_NoValidatorFailsClosed(t *testing.T) {
	cases := map[string]contracts.PasswordValidatorInterface{
		"no validator": nil,
		"validator error": &testhelpers.MockPasswordValidator{
			ValidatePasswordFn: func(string) (*contracts.PasswordValidationResult, error) {
				return nil, fmt.Errorf("validator outage")
			},
		},
	}
	for name, validator := range cases {
		t.Run(name, func(t *testing.T) {
			rec := &setPasswordRecorder{}
			h := setPasswordHandler(rec, func(ah *AuthHandler) { ah.passwordValidator = validator })

			_, err := h.SetPasswordHandler(testhelpers.CtxWithSessionAuthTime(passwordlessUser(), time.Now()), setPasswordInput("a-new-strong-password-1!"))
			assertServiceUnavailable(t, err)
			if rec.calls != 0 {
				t.Error("an unvalidated password must not be written")
			}
		})
	}
}

// A session revoked after its validation is refused the way the JWT middleware
// refuses it on its next request: 401 TOKEN_INVALID, clearing the cookie.
func TestSetPasswordHandler_RevokedSessionIsRefused(t *testing.T) {
	rec := &setPasswordRecorder{err: autherrors.ErrTokenInvalid(fmt.Errorf("user 42: %w", shared.ErrSessionRevoked))}
	h := setPasswordHandler(rec)

	_, err := h.SetPasswordHandler(testhelpers.CtxWithSessionAuthTime(passwordlessUser(), time.Now()), setPasswordInput("a-new-strong-password-1!"))
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
}

func TestSetPasswordHandler_ClosedAccountIsRefused(t *testing.T) {
	refusal := autherrors.ErrAccountInactive()
	refusal.Internal = fmt.Errorf("user 42: %w", shared.ErrSessionAccountClosed)
	rec := &setPasswordRecorder{err: refusal}
	h := setPasswordHandler(rec)

	resp, err := h.SetPasswordHandler(testhelpers.CtxWithSessionAuthTime(passwordlessUser(), time.Now()), setPasswordInput("a-new-strong-password-1!"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertSetPasswordRefused(t, resp, autherrors.CodeAccountInactive, "")
}

func TestSetPasswordHandler_UnexpectedFailureIsA5xx(t *testing.T) {
	rec := &setPasswordRecorder{err: errors.New("connection reset")}
	h := setPasswordHandler(rec)

	_, err := h.SetPasswordHandler(testhelpers.CtxWithSessionAuthTime(passwordlessUser(), time.Now()), setPasswordInput("a-new-strong-password-1!"))
	assertServiceUnavailable(t, err)
}

// The password is set by the time the session is renewed, so a renewal
// failure is not reported as a failed set.
func TestSetPasswordHandler_RenewalFailureDoesNotFailTheSet(t *testing.T) {
	rec := &setPasswordRecorder{}
	h := setPasswordHandler(rec, func(ah *AuthHandler) {
		ah.jwtService = &testhelpers.MockJWTService{
			RenewSessionTokenFn: func(*authm.User, time.Time) (string, error) { return "", errors.New("jwt outage") },
		}
	})

	resp, err := h.SetPasswordHandler(testhelpers.CtxWithSessionAuthTime(passwordlessUser(), time.Now()), setPasswordInput("a-new-strong-password-1!"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("the password was set, so the response must say so")
	}
	if resp.SetCookie.Value != "" {
		t.Error("a failed renewal must not set a cookie")
	}
}

func assertSetPasswordRefused(t *testing.T, resp *SetPasswordResponse, code, message string) {
	t.Helper()
	if resp.Body.Success {
		t.Fatal("expected a refusal")
	}
	if resp.Body.ErrorCode != code {
		t.Errorf("error_code = %q, want %q", resp.Body.ErrorCode, code)
	}
	if message != "" && resp.Body.Message != message {
		t.Errorf("message = %q, want %q", resp.Body.Message, message)
	}
}

func assertServiceUnavailable(t *testing.T, err error) {
	t.Helper()
	var authErr *autherrors.AuthError
	if !errors.As(err, &authErr) || authErr.Code != autherrors.CodeServiceUnavailable {
		t.Fatalf("want a SERVICE_UNAVAILABLE error, got %v", err)
	}
	if authErr.HTTPStatus() != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", authErr.HTTPStatus())
	}
}
