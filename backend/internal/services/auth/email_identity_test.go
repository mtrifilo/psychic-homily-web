package auth

import (
	"github.com/golang-jwt/jwt/v5"

	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// TestFindOrCreateAppleUser_ExistingEmail_CaseVariant_LinksAppleAccount covers
// the Apple Sign In lookup. Apple returns the address from its own record, in
// whatever casing it holds, so a user who registered as Mixed.Case@Example.com
// and later signs in with Apple must land on that account rather than a second
// one for the same mailbox.
func (s *AppleAuthIntegrationTestSuite) TestFindOrCreateAppleUser_ExistingEmail_CaseVariant_LinksAppleAccount() {
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
		Email: "apple.case@example.com",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "apple-sub-case-variant",
		},
	}, "Apple", "Name")

	s.Require().NoError(err)
	s.Equal(existingUser.ID, user.ID)
	s.Equal("Apple.Case@Example.com", *user.Email)
	s.Require().Len(user.OAuthAccounts, 1)
	s.Equal("apple-sub-case-variant", user.OAuthAccounts[0].ProviderUserID)

	var rows int64
	s.Require().NoError(
		s.db.Model(&authm.User{}).
			Where(authm.EmailIdentityWhere, "apple.case@example.com").
			Count(&rows).Error)
	s.Equal(int64(1), rows)
}
