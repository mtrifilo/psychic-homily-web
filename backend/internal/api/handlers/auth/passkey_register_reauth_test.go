package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
)

// Attaching a passkey gives the holder a permanent, independent way to sign in
// as the account, and signing in that way stamps the freshness every other gate
// reads. A stale session that could attach one would therefore defeat the gate
// on every mint, not just this endpoint.

func passkeyUser() *authm.User {
	hash := "$2a$not-a-real-hash"
	return &authm.User{ID: 4, IsActive: true, PasswordHash: &hash}
}

func TestPasskeyRegistration_FreshSessionProceeds(t *testing.T) {
	var began, finished bool
	mockWA := &testhelpers.MockWebAuthnService{
		BeginRegistrationFn: func(*authm.User) (*protocol.CredentialCreation, *webauthn.SessionData, error) {
			began = true
			return &protocol.CredentialCreation{}, &webauthn.SessionData{}, nil
		},
		GetChallengeFn: func(string, string) (*webauthn.SessionData, uint, error) {
			finished = true
			return nil, 0, errors.New("challenge lookup is past the gate")
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, &testhelpers.MockUserService{})
	ctx := testhelpers.CtxWithSessionAuthTime(passkeyUser(), time.Now().Add(-1*time.Minute))

	if _, err := h.BeginRegisterHandler(ctx, &BeginRegisterRequest{}); err != nil {
		t.Fatalf("a fresh session must be allowed to begin: %v", err)
	}
	if !began {
		t.Error("expected the webauthn service to be reached")
	}

	if _, err := h.FinishRegisterHandler(ctx, &FinishRegisterRequest{}); err != nil {
		t.Fatalf("a fresh session must be allowed to finish: %v", err)
	}
	if !finished {
		t.Error("expected finish to reach the challenge lookup")
	}
}

func TestPasskeyRegistration_StaleSessionRefused(t *testing.T) {
	for name, authAt := range map[string]time.Time{
		"stale session, refreshed or not": time.Now().Add(-2 * time.Hour),
		"api-token principal":             {},
	} {
		t.Run(name, func(t *testing.T) {
			var reached bool
			mockWA := &testhelpers.MockWebAuthnService{
				BeginRegistrationFn: func(*authm.User) (*protocol.CredentialCreation, *webauthn.SessionData, error) {
					reached = true
					return &protocol.CredentialCreation{}, &webauthn.SessionData{}, nil
				},
				GetChallengeFn: func(string, string) (*webauthn.SessionData, uint, error) {
					reached = true
					return &webauthn.SessionData{}, 4, nil
				},
			}
			h := testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, &testhelpers.MockUserService{})
			ctx := testhelpers.CtxWithSessionAuthTime(passkeyUser(), authAt)

			var refusal *shared.ReauthRequiredError

			_, beginErr := h.BeginRegisterHandler(ctx, &BeginRegisterRequest{})
			if !errors.As(beginErr, &refusal) {
				t.Fatalf("begin: expected a re-authentication refusal, got %v", beginErr)
			}

			_, finishErr := h.FinishRegisterHandler(ctx, &FinishRegisterRequest{})
			if !errors.As(finishErr, &refusal) {
				t.Fatalf("finish: expected a re-authentication refusal, got %v", finishErr)
			}

			if reached {
				t.Error("a refused request must not reach the webauthn service")
			}
		})
	}
}

// Signing up with a passkey is not attaching one to an existing account: the
// registration IS the factor, and there is no session to have proven anything.
func TestPasskeySignup_NotGated(t *testing.T) {
	var began bool
	mockWA := &testhelpers.MockWebAuthnService{
		BeginRegistrationForEmailFn: func(string) (*protocol.CredentialCreation, *webauthn.SessionData, error) {
			began = true
			return &protocol.CredentialCreation{}, &webauthn.SessionData{}, nil
		},
		StoreChallengeWithEmailFn: func(string, *webauthn.SessionData, string) (string, error) {
			return "challenge-1", nil
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, &testhelpers.MockUserService{
		GetUserByEmailFn: func(string) (*authm.User, error) { return nil, nil },
	})

	input := &BeginSignupRequest{}
	input.Body.Email = "new-user@example.com"
	input.Body.TermsAccepted = true
	input.Body.TermsVersion = "2026-01-01"
	input.Body.AgeConfirmed = true
	input.Body.MinAgeAttested = MinSignupAge
	if _, err := h.BeginSignupHandler(t.Context(), input); err != nil {
		t.Fatalf("passkey signup must not require a prior session: %v", err)
	}
	if !began {
		t.Error("expected signup registration to reach the webauthn service")
	}
}
