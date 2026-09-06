package auth

import (
	"context"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// The two handlers below compare a token's address against the account's
// stored address. Both cases run the SAME pair through the handler: a case
// variant must be accepted as the same mailbox, and a genuinely different
// address must still be refused, so a comparison that always returned true
// would fail the second case.

func TestConfirmVerificationHandler_EmailIdentityIsCaseInsensitive(t *testing.T) {
	cases := []struct {
		name        string
		stored      string
		tokenEmail  string
		wantSuccess bool
	}{
		{"case variant is the same mailbox", "Sym.Case@Example.com", "sym.case@example.com", true},
		{"different mailbox is still refused", "Sym.Case@Example.com", "someone.else@example.com", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stored := tc.stored
			var verified bool
			h := authHandler(func(ah *AuthHandler) {
				ah.jwtService = &testhelpers.MockJWTService{
					ValidateVerificationTokenFn: func(string) (*contracts.VerificationTokenClaims, error) {
						return &contracts.VerificationTokenClaims{UserID: 1, Email: tc.tokenEmail}, nil
					},
				}
				ah.userService = &testhelpers.MockUserService{
					GetUserByIDFn: func(uint) (*authm.User, error) {
						return &authm.User{ID: 1, Email: &stored, EmailVerified: false}, nil
					},
					SetEmailVerifiedFn: func(uint, bool) error {
						verified = true
						return nil
					},
				}
			})

			input := &ConfirmVerificationRequest{}
			input.Body.Token = "valid-token"

			resp, err := h.ConfirmVerificationHandler(context.Background(), input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.Body.Success != tc.wantSuccess {
				t.Fatalf("success=%v, want %v (message=%q, code=%q)",
					resp.Body.Success, tc.wantSuccess, resp.Body.Message, resp.Body.ErrorCode)
			}
			if verified != tc.wantSuccess {
				t.Errorf("SetEmailVerified called=%v, want %v", verified, tc.wantSuccess)
			}
			if !tc.wantSuccess && resp.Body.ErrorCode != "EMAIL_MISMATCH" {
				t.Errorf("error_code=%q, want EMAIL_MISMATCH", resp.Body.ErrorCode)
			}
		})
	}
}

func TestVerifyMagicLinkHandler_EmailIdentityIsCaseInsensitive(t *testing.T) {
	cases := []struct {
		name        string
		stored      string
		tokenEmail  string
		wantSuccess bool
	}{
		{"case variant is the same mailbox", "Sym.Case@Example.com", "SYM.CASE@EXAMPLE.COM", true},
		{"different mailbox is still refused", "Sym.Case@Example.com", "someone.else@example.com", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stored := tc.stored
			h := authHandler(func(ah *AuthHandler) {
				ah.jwtService = &testhelpers.MockJWTService{
					ValidateMagicLinkTokenFn: func(string) (*contracts.MagicLinkTokenClaims, error) {
						return &contracts.MagicLinkTokenClaims{UserID: 1, Email: tc.tokenEmail}, nil
					},
					CreateTokenFn: func(*authm.User) (string, error) { return "session-token", nil },
				}
				ah.userService = &testhelpers.MockUserService{
					GetUserByIDFn: func(uint) (*authm.User, error) {
						return &authm.User{ID: 1, Email: &stored, EmailVerified: true, IsActive: true}, nil
					},
				}
			})

			input := &VerifyMagicLinkRequest{}
			input.Body.Token = "valid-token"

			resp, err := h.VerifyMagicLinkHandler(context.Background(), input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.Body.Success != tc.wantSuccess {
				t.Fatalf("success=%v, want %v (message=%q, code=%q)",
					resp.Body.Success, tc.wantSuccess, resp.Body.Message, resp.Body.ErrorCode)
			}
			if !tc.wantSuccess && resp.Body.ErrorCode != "INVALID_TOKEN" {
				t.Errorf("error_code=%q, want INVALID_TOKEN", resp.Body.ErrorCode)
			}
		})
	}
}
