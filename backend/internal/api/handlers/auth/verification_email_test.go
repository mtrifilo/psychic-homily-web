package auth

import (
	"context"
	"errors"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// TestSendVerificationEmailBestEffort covers every branch of the helper shared
// by password registration, passkey signup, and the magic-link fallback. It is
// the one place the branch logic lives, so the call sites only have to assert
// that they reach it.
//
// The invariant under test in every case: the helper never panics and never
// returns anything, because its callers have already completed a user-visible
// operation and cannot act on a failure.
func TestSendVerificationEmailBestEffort(t *testing.T) {
	email := "user@example.com"
	verifiedUser := &authm.User{ID: 5, Email: &email}
	emptyEmail := ""

	tests := []struct {
		name          string
		user          *authm.User
		configured    bool
		nilJWT        bool
		nilEmail      bool
		tokenErr      error
		sendErr       error
		wantTokenMint bool
		wantSend      bool
	}{
		{
			name:          "sends for a configured service and an addressable user",
			user:          verifiedUser,
			configured:    true,
			wantTokenMint: true,
			wantSend:      true,
		},
		{
			name: "no-op for a nil user",
			user: nil,
		},
		{
			name: "no-op for a user with no email",
			user: &authm.User{ID: 6, Email: nil},
		},
		{
			name: "no-op for a user with an empty email",
			user: &authm.User{ID: 7, Email: &emptyEmail},
		},
		{
			name:       "no-op when the email service is unconfigured",
			user:       verifiedUser,
			configured: false,
		},
		{
			name:     "no-op when the email service is absent",
			user:     verifiedUser,
			nilEmail: true,
		},
		{
			name:       "no-op when the jwt service is absent",
			user:       verifiedUser,
			configured: true,
			nilJWT:     true,
		},
		{
			// A failed mint must not send an empty token: the resulting link
			// would land on /verify-email and fail validation, training the
			// user to distrust the link.
			name:          "does not send when the token mint fails",
			user:          verifiedUser,
			configured:    true,
			tokenErr:      errors.New("jwt down"),
			wantTokenMint: true,
			wantSend:      false,
		},
		{
			name:          "swallows a send failure",
			user:          verifiedUser,
			configured:    true,
			sendErr:       errors.New("resend 503"),
			wantTokenMint: true,
			wantSend:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var tokenMinted, sent bool
			var sentTo, sentToken string

			jwtMock := &testhelpers.MockJWTService{
				CreateVerificationTokenFn: func(userID uint, e string) (string, error) {
					tokenMinted = true
					if tt.tokenErr != nil {
						return "", tt.tokenErr
					}
					return "verify-token", nil
				},
			}
			emailMock := &testhelpers.MockEmailService{
				IsConfiguredFn: func() bool { return tt.configured },
				SendVerificationEmailFn: func(to, token string) error {
					sent = true
					sentTo, sentToken = to, token
					return tt.sendErr
				},
			}

			jwtService := jwtServiceOrNil(jwtMock, tt.nilJWT)
			emailService := emailServiceOrNil(emailMock, tt.nilEmail)

			sendVerificationEmailBestEffort(
				context.Background(), jwtService, emailService, tt.user, verificationTriggerSignup,
			)

			if tokenMinted != tt.wantTokenMint {
				t.Errorf("token minted = %v, want %v", tokenMinted, tt.wantTokenMint)
			}
			if sent != tt.wantSend {
				t.Errorf("email sent = %v, want %v", sent, tt.wantSend)
			}
			if tt.wantSend {
				if sentTo != *tt.user.Email {
					t.Errorf("sent to %q, want %q", sentTo, *tt.user.Email)
				}
				if sentToken != "verify-token" {
					t.Errorf("sent token %q, want the minted token", sentToken)
				}
			}
		})
	}
}

// jwtServiceOrNil returns a typed-nil-free nil interface when asked, so the
// helper's `jwtService == nil` guard is actually exercised. Passing a nil
// *MockJWTService would produce a non-nil interface holding a nil pointer and
// silently skip the guard.
func jwtServiceOrNil(mock *testhelpers.MockJWTService, wantNil bool) contracts.JWTServiceInterface {
	if wantNil {
		return nil
	}
	return mock
}

func emailServiceOrNil(mock *testhelpers.MockEmailService, wantNil bool) contracts.EmailServiceInterface {
	if wantNil {
		return nil
	}
	return mock
}
