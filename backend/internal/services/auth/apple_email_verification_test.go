package auth

import (
	"github.com/golang-jwt/jwt/v5"

	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// Apple's email_verified claim arrives as a JSON bool or as the string "true".
// Both spellings open the link; every other value refuses it.

func (s *AppleAuthIntegrationTestSuite) TestFindOrCreateAppleUser_UnverifiedEmail_RefusesLink() {
	existing := &authm.User{
		Email:         stringPtr("apple-unverified@example.com"),
		IsActive:      true,
		EmailVerified: true,
	}
	s.Require().NoError(s.db.Create(existing).Error)

	svc := s.newService()
	user, err := svc.FindOrCreateAppleUser(&contracts.AppleIdentityTokenClaims{
		Email:         "apple-unverified@example.com",
		EmailVerified: false,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "apple-sub-unverified",
		},
	}, "Apple", "Name")

	s.Require().Error(err)
	s.Require().Nil(user)

	var authErr *apperrors.AuthError
	s.Require().ErrorAs(err, &authErr)
	s.Equal(apperrors.CodeUserExists, authErr.Code)

	s.assertNoAppleAccountFor(existing.ID)
}

func (s *AppleAuthIntegrationTestSuite) TestFindOrCreateAppleUser_AbsentVerificationClaim_RefusesLink() {
	existing := &authm.User{
		Email:         stringPtr("apple-absent@example.com"),
		IsActive:      true,
		EmailVerified: true,
	}
	s.Require().NoError(s.db.Create(existing).Error)

	svc := s.newService()
	// EmailVerified left nil: the token carried no claim.
	user, err := svc.FindOrCreateAppleUser(&contracts.AppleIdentityTokenClaims{
		Email: "apple-absent@example.com",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "apple-sub-absent",
		},
	}, "Apple", "Name")

	s.Require().Error(err)
	s.Require().Nil(user)

	var authErr *apperrors.AuthError
	s.Require().ErrorAs(err, &authErr)
	s.Equal(apperrors.CodeUserExists, authErr.Code)

	s.assertNoAppleAccountFor(existing.ID)
}

func (s *AppleAuthIntegrationTestSuite) TestFindOrCreateAppleUser_StringVerifiedClaim_LinksAccount() {
	existing := &authm.User{
		Email:         stringPtr("apple-string-verified@example.com"),
		IsActive:      true,
		EmailVerified: true,
	}
	s.Require().NoError(s.db.Create(existing).Error)

	svc := s.newService()
	user, err := svc.FindOrCreateAppleUser(&contracts.AppleIdentityTokenClaims{
		Email:         "apple-string-verified@example.com",
		EmailVerified: "true",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "apple-sub-string-verified",
		},
	}, "Apple", "Name")

	s.Require().NoError(err)
	s.Require().NotNil(user)
	s.Equal(existing.ID, user.ID)
	s.Require().Len(user.OAuthAccounts, 1)
	s.Equal("apple-sub-string-verified", user.OAuthAccounts[0].ProviderUserID)
}

// A returning Apple user resolves by subject before the address is consulted,
// so a token that stops carrying the claim cannot break an established sign-in.
func (s *AppleAuthIntegrationTestSuite) TestFindOrCreateAppleUser_ExistingAppleAccount_UnaffectedByVerification() {
	svc := s.newService()
	first, err := svc.FindOrCreateAppleUser(&contracts.AppleIdentityTokenClaims{
		Email:         "apple-returning@example.com",
		EmailVerified: true,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "apple-sub-returning",
		},
	}, "Apple", "Name")
	s.Require().NoError(err)

	second, err := svc.FindOrCreateAppleUser(&contracts.AppleIdentityTokenClaims{
		Email: "apple-returning@example.com",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "apple-sub-returning",
		},
	}, "Apple", "Name")

	s.Require().NoError(err)
	s.Equal(first.ID, second.ID)
}

// The gate guards the link branch. An address matching nobody is not a
// takeover, so a first-time Apple signup still creates an account, and the row
// it creates records email_verified whatever the claim said.
func (s *AppleAuthIntegrationTestSuite) TestFindOrCreateAppleUser_UnverifiedEmail_NoExistingAccount_StillCreates() {
	svc := s.newService()
	user, err := svc.FindOrCreateAppleUser(&contracts.AppleIdentityTokenClaims{
		Email:         "apple-fresh-unverified@example.com",
		EmailVerified: false,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "apple-sub-fresh-unverified",
		},
	}, "Apple", "Name")

	s.Require().NoError(err)
	s.Require().NotNil(user)
	s.Equal("apple-fresh-unverified@example.com", *user.Email)
	s.True(user.EmailVerified)
}

func (s *AppleAuthIntegrationTestSuite) assertNoAppleAccountFor(userID uint) {
	s.T().Helper()
	var rows int64
	s.Require().NoError(
		s.db.Model(&authm.OAuthAccount{}).
			Where("user_id = ? AND provider = ?", userID, "apple").
			Count(&rows).Error)
	s.Equal(int64(0), rows, "a refused link must leave no apple oauth_accounts row")
}
