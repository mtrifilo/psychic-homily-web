package auth

import (
	"github.com/golang-jwt/jwt/v5"

	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// TestFindOrCreateAppleUser_ExistingEmail_CaseVariant_RefusesSignIn covers the
// Apple Sign In lookup. Identity is case-insensitive, so an address differing
// only in case from a stored one resolves to that account, and an address that
// resolves to an account refuses the sign-in.
//
// The refusal is what proves the fold ran: a case-sensitive lookup would miss
// the row and mint a second account for the same mailbox instead.
func (s *AppleAuthIntegrationTestSuite) TestFindOrCreateAppleUser_ExistingEmail_CaseVariant_RefusesSignIn() {
	existingUser := &authm.User{
		Email:         stringPtr("Apple.Case@Example.com"),
		FirstName:     stringPtr("Existing"),
		LastName:      stringPtr("User"),
		IsActive:      true,
		EmailVerified: true,
	}
	s.Require().NoError(s.db.Create(existingUser).Error)

	svc := s.newService()
	user, err := svc.FindOrCreateAppleUser(&contracts.AppleIdentityTokenClaims{
		Email:         "apple.case@example.com",
		EmailVerified: true,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "apple-sub-case-variant",
		},
	}, "Apple", "Name")

	s.Require().Error(err)
	s.Require().Nil(user)

	var authErr *apperrors.AuthError
	s.Require().ErrorAs(err, &authErr)
	s.Equal(apperrors.CodeOAuthLinkRefused, authErr.Code)

	s.assertNoAppleAccountFor(existingUser.ID)
	s.assertSingleUserForAddress("apple.case@example.com")
}
