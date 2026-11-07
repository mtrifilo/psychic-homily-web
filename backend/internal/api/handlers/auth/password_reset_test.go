package auth

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/config"
	autherrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// resetOutbox records what the reset handlers queue.
type resetOutbox struct {
	kinds   []authm.AuthEmailKind
	userIDs []uint
	err     error
}

func (o *resetOutbox) mock() *testhelpers.MockAuthEmailOutbox {
	return &testhelpers.MockAuthEmailOutbox{
		EnqueueFn: func(_ context.Context, kind authm.AuthEmailKind, userID uint) error {
			o.kinds = append(o.kinds, kind)
			o.userIDs = append(o.userIDs, userID)
			return o.err
		},
	}
}

func configuredEmail() *testhelpers.MockEmailService {
	return &testhelpers.MockEmailService{IsConfiguredFn: func() bool { return true }}
}

func resetRequest(email string) *RequestPasswordResetRequest {
	input := &RequestPasswordResetRequest{}
	input.Body.Email = email
	return input
}

func requireAuthErrorCode(t *testing.T, err error, code string) {
	t.Helper()
	var authErr *autherrors.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("got %v, want an AuthError %s", err, code)
	}
	if authErr.Code != code {
		t.Fatalf("got code %s, want %s", authErr.Code, code)
	}
}

// Every well-formed address gets the same answer; only an account that may
// sign in gets a row queued.
func TestRequestPasswordResetHandler_SameAnswerWhateverTheAccount(t *testing.T) {
	open := &authm.User{ID: 11, Email: strPtr("open@example.com"), IsActive: true}
	closed := &authm.User{ID: 12, Email: strPtr("closed@example.com"), IsActive: false}
	cases := []struct {
		name      string
		email     string
		user      *authm.User
		lookupErr error
		enqErr    error
		wantQueue []uint
	}{
		{"an account that may sign in", "open@example.com", open, nil, nil, []uint{11}},
		{"no account", "nobody@example.com", nil, nil, nil, nil},
		{"a closed account", "closed@example.com", closed, nil, nil, nil},
		{"a failed lookup", "open@example.com", nil, errors.New("db down"), nil, nil},
		{"a failed enqueue", "open@example.com", open, nil, errors.New("outbox off"), []uint{11}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			outbox := &resetOutbox{err: tc.enqErr}
			h := authHandler(func(ah *AuthHandler) {
				ah.emailService = configuredEmail()
				ah.emailOutbox = outbox.mock()
				ah.userService = &testhelpers.MockUserService{
					GetUserByEmailFn: func(string) (*authm.User, error) { return tc.user, tc.lookupErr },
				}
			})

			resp, err := h.RequestPasswordResetHandler(context.Background(), resetRequest(tc.email))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !resp.Body.Success || resp.Body.Message != passwordResetSentMessage {
				t.Errorf("answer = %v %q, want the one enumeration-safe body", resp.Body.Success, resp.Body.Message)
			}
			if fmt.Sprint(outbox.userIDs) != fmt.Sprint(tc.wantQueue) {
				t.Errorf("queued for %v, want %v", outbox.userIDs, tc.wantQueue)
			}
			for _, kind := range outbox.kinds {
				if kind != authm.AuthEmailKindPasswordReset {
					t.Errorf("queued kind %s, want %s", kind, authm.AuthEmailKindPasswordReset)
				}
			}
		})
	}
}

// Refusals that depend on no account: a malformed address and a server with
// no email provider. Neither looks the address up.
func TestRequestPasswordResetHandler_RefusesBeforeTheLookup(t *testing.T) {
	cases := []struct {
		name       string
		email      string
		configured bool
		wantCode   string
	}{
		{"no address", "", true, autherrors.CodeValidationFailed},
		{"a malformed address", "not-an-address", true, autherrors.CodeValidationFailed},
		{"a padded address", " open@example.com", true, autherrors.CodeValidationFailed},
		{"no email provider", "open@example.com", false, autherrors.CodeServiceUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			looked := false
			h := authHandler(func(ah *AuthHandler) {
				ah.emailService = &testhelpers.MockEmailService{IsConfiguredFn: func() bool { return tc.configured }}
				ah.userService = &testhelpers.MockUserService{
					GetUserByEmailFn: func(string) (*authm.User, error) { looked = true; return nil, nil },
				}
			})
			_, err := h.RequestPasswordResetHandler(context.Background(), resetRequest(tc.email))
			requireAuthErrorCode(t, err, tc.wantCode)
			if looked {
				t.Error("the address was looked up before a refusal that needs no account")
			}
		})
	}
}

func TestSendOwnPasswordResetHandler_QueuesForTheSignedInAccount(t *testing.T) {
	outbox := &resetOutbox{}
	h := authHandler(func(ah *AuthHandler) {
		ah.emailService = configuredEmail()
		ah.emailOutbox = outbox.mock()
	})
	user := &authm.User{ID: 21, Email: strPtr("owner@example.com"), IsActive: true}

	resp, err := h.SendOwnPasswordResetHandler(testhelpers.CtxWithUser(user), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success")
	}
	if fmt.Sprint(outbox.userIDs) != "[21]" || outbox.kinds[0] != authm.AuthEmailKindPasswordReset {
		t.Errorf("queued %v for %v, want one reset for 21", outbox.kinds, outbox.userIDs)
	}
}

// The caller is the account, so a failure to queue is reported, never
// confirmed as sent.
func TestSendOwnPasswordResetHandler_ReportsAFailureToQueue(t *testing.T) {
	outbox := &resetOutbox{err: errors.New("outbox off")}
	h := authHandler(func(ah *AuthHandler) {
		ah.emailService = configuredEmail()
		ah.emailOutbox = outbox.mock()
	})
	user := &authm.User{ID: 21, Email: strPtr("owner@example.com"), IsActive: true}

	_, err := h.SendOwnPasswordResetHandler(testhelpers.CtxWithUser(user), nil)
	requireAuthErrorCode(t, err, autherrors.CodeServiceUnavailable)
}

func TestSendOwnPasswordResetHandler_Refusals(t *testing.T) {
	h := authHandler(func(ah *AuthHandler) { ah.emailService = configuredEmail() })
	if _, err := h.SendOwnPasswordResetHandler(context.Background(), nil); err == nil {
		t.Error("no session must be refused")
	}
	_, err := h.SendOwnPasswordResetHandler(testhelpers.CtxWithUser(&authm.User{ID: 22, IsActive: true}), nil)
	requireAuthErrorCode(t, err, autherrors.CodeValidationFailed)
}

// resetClaims is a valid link for account 31.
func resetClaims() *contracts.PasswordResetTokenClaims {
	return &contracts.PasswordResetTokenClaims{UserID: 31, Email: "owner@example.com", PasswordState: "state"}
}

func resetJWT() *testhelpers.MockJWTService {
	return &testhelpers.MockJWTService{
		ValidatePasswordResetTokenFn: func(token string) (*contracts.PasswordResetTokenClaims, error) {
			if token != "good-link" {
				return nil, errors.New("invalid password reset token")
			}
			return resetClaims(), nil
		},
	}
}

func TestCheckPasswordResetHandler(t *testing.T) {
	owner := &authm.User{ID: 31, Email: strPtr("owner@example.com"), IsActive: true}
	cases := []struct {
		name       string
		token      string
		accountErr error
		wantCode   string
	}{
		{"a live link", "good-link", nil, ""},
		{"no token", "", nil, autherrors.CodeValidationFailed},
		{"an expired or forged link", "bad-link", nil, passwordResetLinkInvalidCode},
		{"a used link", "good-link", fmt.Errorf("user 31: %w", contracts.ErrPasswordResetLinkSpent), passwordResetLinkInvalidCode},
		{"a closed account", "good-link", autherrors.ErrAccountInactive(), autherrors.CodeAccountInactive},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := authHandler(func(ah *AuthHandler) {
				ah.jwtService = resetJWT()
				ah.authService = &testhelpers.MockAuthService{
					PasswordResetAccountFn: func(*contracts.PasswordResetTokenClaims) (*authm.User, error) {
						if tc.accountErr != nil {
							return nil, tc.accountErr
						}
						return owner, nil
					},
				}
			})
			input := &CheckPasswordResetRequest{}
			input.Body.Token = tc.token
			resp, err := h.CheckPasswordResetHandler(context.Background(), input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantCode == "" {
				if !resp.Body.Success || resp.Body.Email != "owner@example.com" {
					t.Errorf("got %v %q, want success for owner@example.com", resp.Body.Success, resp.Body.Email)
				}
				return
			}
			if resp.Body.Success || resp.Body.ErrorCode != tc.wantCode || resp.Body.Email != "" {
				t.Errorf("got success=%v code=%q email=%q, want refusal %s and no address", resp.Body.Success, resp.Body.ErrorCode, resp.Body.Email, tc.wantCode)
			}
		})
	}
}

// A database failure is not a refusal the reader can act on.
func TestCheckPasswordResetHandler_FailureIs5xx(t *testing.T) {
	h := authHandler(func(ah *AuthHandler) {
		ah.jwtService = resetJWT()
		ah.authService = &testhelpers.MockAuthService{
			PasswordResetAccountFn: func(*contracts.PasswordResetTokenClaims) (*authm.User, error) {
				return nil, errors.New("db down")
			},
		}
	})
	input := &CheckPasswordResetRequest{}
	input.Body.Token = "good-link"
	_, err := h.CheckPasswordResetHandler(context.Background(), input)
	requireAuthErrorCode(t, err, autherrors.CodeServiceUnavailable)
}

func completeRequest(token, password string) *CompletePasswordResetRequest {
	input := &CompletePasswordResetRequest{}
	input.Body.Token, input.Body.NewPassword = token, password
	return input
}

func TestCompletePasswordResetHandler_SetsThePasswordAndSignsIn(t *testing.T) {
	owner := &authm.User{ID: 31, Email: strPtr("owner@example.com"), IsActive: true}
	var gotPassword string
	h := authHandler(func(ah *AuthHandler) {
		ah.jwtService = resetJWT()
		ah.authService = &testhelpers.MockAuthService{
			CompletePasswordResetFn: func(claims *contracts.PasswordResetTokenClaims, password string) (*contracts.PasswordResetCompletion, error) {
				gotPassword = password
				return &contracts.PasswordResetCompletion{User: owner, Token: "new-session"}, nil
			},
		}
	})

	resp, err := h.CompletePasswordResetHandler(context.Background(), completeRequest("good-link", "owners-new-password-1!"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success || resp.Body.User == nil || resp.Body.User.ID != 31 {
		t.Fatalf("got %+v, want success for account 31", resp.Body)
	}
	if gotPassword != "owners-new-password-1!" {
		t.Errorf("service got password %q", gotPassword)
	}
	if resp.SetCookie.Name != config.AuthCookieName || resp.SetCookie.Value != "new-session" {
		t.Errorf("cookie = %s=%q, want this browser's new session", resp.SetCookie.Name, resp.SetCookie.Value)
	}
	want := h.config.JWT.SessionLifetime()
	if lifetime := time.Until(resp.SetCookie.Expires); lifetime < want-time.Minute || lifetime > want {
		t.Errorf("cookie lifetime = %v, want the configured session lifetime %v", lifetime, want)
	}
}

// A weak or breached password is refused with the validator's message before
// the service runs, so the link stays unspent.
func TestCompletePasswordResetHandler_ValidatorRefusal(t *testing.T) {
	called := false
	h := authHandler(func(ah *AuthHandler) {
		ah.jwtService = resetJWT()
		ah.passwordValidator = &testhelpers.MockPasswordValidator{
			ValidatePasswordFn: func(string) (*contracts.PasswordValidationResult, error) {
				return &contracts.PasswordValidationResult{Valid: false, Errors: []string{"This password has appeared in a data breach. Please choose a different password."}}, nil
			},
		}
		ah.authService = &testhelpers.MockAuthService{
			CompletePasswordResetFn: func(*contracts.PasswordResetTokenClaims, string) (*contracts.PasswordResetCompletion, error) {
				called = true
				return nil, nil
			},
		}
	})

	resp, err := h.CompletePasswordResetHandler(context.Background(), completeRequest("good-link", "password123password"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success || resp.Body.ErrorCode != autherrors.CodeValidationFailed ||
		resp.Body.Message != "This password has appeared in a data breach. Please choose a different password." {
		t.Errorf("got %+v, want the validator's refusal", resp.Body)
	}
	if called {
		t.Error("a refused password must not reach the reset")
	}
	if resp.SetCookie.Value != "" {
		t.Error("a refusal sets no session")
	}
}

func TestCompletePasswordResetHandler_Refusals(t *testing.T) {
	cases := []struct {
		name       string
		token      string
		password   string
		serviceErr error
		wantCode   string
	}{
		{"no token", "", "owners-new-password-1!", nil, autherrors.CodeValidationFailed},
		{"an expired or forged link", "bad-link", "owners-new-password-1!", nil, passwordResetLinkInvalidCode},
		{"no password", "good-link", "", nil, autherrors.CodeValidationFailed},
		{"a used link", "good-link", "owners-new-password-1!", fmt.Errorf("user 31: %w", contracts.ErrPasswordResetLinkSpent), passwordResetLinkInvalidCode},
		{"a closed account", "good-link", "owners-new-password-1!", autherrors.ErrAccountRecoverable(), autherrors.CodeAccountRecoverable},
		{"a password bcrypt cannot hash", "good-link", "owners-new-password-1!", autherrors.ErrValidationFailed("Password is too long. Try a shorter one."), autherrors.CodeValidationFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := authHandler(func(ah *AuthHandler) {
				ah.jwtService = resetJWT()
				ah.authService = &testhelpers.MockAuthService{
					CompletePasswordResetFn: func(*contracts.PasswordResetTokenClaims, string) (*contracts.PasswordResetCompletion, error) {
						return nil, tc.serviceErr
					},
				}
			})
			resp, err := h.CompletePasswordResetHandler(context.Background(), completeRequest(tc.token, tc.password))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.Body.Success || resp.Body.ErrorCode != tc.wantCode {
				t.Errorf("got success=%v code=%q, want refusal %s", resp.Body.Success, resp.Body.ErrorCode, tc.wantCode)
			}
			if resp.SetCookie.Value != "" {
				t.Error("a refusal sets no session")
			}
		})
	}
}

// A link that can no longer set the password is refused before the validator's
// outbound breach check and the hash run.
func TestCompletePasswordResetHandler_SpentLinkSkipsTheValidator(t *testing.T) {
	validated, completed := false, false
	h := authHandler(func(ah *AuthHandler) {
		ah.jwtService = resetJWT()
		ah.passwordValidator = &testhelpers.MockPasswordValidator{
			ValidatePasswordFn: func(string) (*contracts.PasswordValidationResult, error) {
				validated = true
				return &contracts.PasswordValidationResult{Valid: true}, nil
			},
		}
		ah.authService = &testhelpers.MockAuthService{
			PasswordResetAccountFn: func(*contracts.PasswordResetTokenClaims) (*authm.User, error) {
				return nil, fmt.Errorf("user 31: %w", contracts.ErrPasswordResetLinkSpent)
			},
			CompletePasswordResetFn: func(*contracts.PasswordResetTokenClaims, string) (*contracts.PasswordResetCompletion, error) {
				completed = true
				return nil, nil
			},
		}
	})

	resp, err := h.CompletePasswordResetHandler(context.Background(), completeRequest("good-link", "owners-new-password-1!"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success || resp.Body.ErrorCode != passwordResetLinkInvalidCode {
		t.Errorf("got %+v, want INVALID_TOKEN", resp.Body)
	}
	if validated || completed {
		t.Errorf("validated=%v completed=%v, want neither for a spent link", validated, completed)
	}
}

// The reset page shows the "link no longer works" state for this code alone.
func TestPasswordResetLinkInvalidCodeMatchesTheFrontend(t *testing.T) {
	assertFrontendSpells(t, "frontend/lib/errors/authErrors.ts", "code",
		"The reset page shows its expired-link state for this code", []string{passwordResetLinkInvalidCode})
}

// A failure that is not the reader's is a 5xx, which the page answers with a
// retry of the same link.
func TestCompletePasswordResetHandler_FailureIs5xx(t *testing.T) {
	h := authHandler(func(ah *AuthHandler) {
		ah.jwtService = resetJWT()
		ah.authService = &testhelpers.MockAuthService{
			CompletePasswordResetFn: func(*contracts.PasswordResetTokenClaims, string) (*contracts.PasswordResetCompletion, error) {
				return nil, errors.New("db down")
			},
		}
	})
	_, err := h.CompletePasswordResetHandler(context.Background(), completeRequest("good-link", "owners-new-password-1!"))
	requireAuthErrorCode(t, err, autherrors.CodeServiceUnavailable)
}
