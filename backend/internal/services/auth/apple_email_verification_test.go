package auth

import (
	"github.com/golang-jwt/jwt/v5"

	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// Apple's email_verified claim arrives as a JSON bool or as the string "true".
// Both spellings record a verified address on an account this path creates;
// every other value records an unverified one. The claim opens nothing: an
// address that already belongs to an account refuses the sign-in whatever the
// claim says.

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
	s.Equal(apperrors.CodeOAuthLinkRefused, authErr.Code)

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
	s.Equal(apperrors.CodeOAuthLinkRefused, authErr.Code)

	s.assertNoAppleAccountFor(existing.ID)
}

// The strongest spelling of the claim is still not evidence about who holds
// the account, so it refuses like the weakest one.
func (s *AppleAuthIntegrationTestSuite) TestFindOrCreateAppleUser_StringVerifiedClaim_RefusesAddressMatch() {
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

	s.Require().Error(err)
	s.Require().Nil(user)

	var authErr *apperrors.AuthError
	s.Require().ErrorAs(err, &authErr)
	s.Equal(apperrors.CodeOAuthLinkRefused, authErr.Code)

	s.assertNoAppleAccountFor(existing.ID)
	s.assertSingleUserForAddress("apple-string-verified@example.com")
}

// The create path is the only one that still reads the claim, so it is where
// the string spelling has to be honoured end to end.
func (s *AppleAuthIntegrationTestSuite) TestFindOrCreateAppleUser_StringVerifiedClaim_NoExistingAccount_CreatesVerified() {
	svc := s.newService()
	user, err := svc.FindOrCreateAppleUser(&contracts.AppleIdentityTokenClaims{
		Email:         "apple-fresh-string-verified@example.com",
		EmailVerified: "true",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "apple-sub-fresh-string-verified",
		},
	}, "Apple", "Name")

	s.Require().NoError(err)
	s.Require().NotNil(user)
	s.True(user.EmailVerified)
	s.assertStoredEmailVerified(user.ID, true)
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
// it creates records what the claim actually said.
func (s *AppleAuthIntegrationTestSuite) TestFindOrCreateAppleUser_UnverifiedEmail_NoExistingAccount_CreatesUnverified() {
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
	s.False(user.EmailVerified, "a claim Apple did not make must not be recorded as verification")
	s.assertStoredEmailVerified(user.ID, false)
}

// A token with no claim at all says no more than a claim of false.
func (s *AppleAuthIntegrationTestSuite) TestFindOrCreateAppleUser_AbsentClaim_NoExistingAccount_CreatesUnverified() {
	svc := s.newService()
	user, err := svc.FindOrCreateAppleUser(&contracts.AppleIdentityTokenClaims{
		Email: "apple-fresh-absent@example.com",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "apple-sub-fresh-absent",
		},
	}, "Apple", "Name")

	s.Require().NoError(err)
	s.Require().NotNil(user)
	s.False(user.EmailVerified)
	s.assertStoredEmailVerified(user.ID, false)
}

func (s *AppleAuthIntegrationTestSuite) TestFindOrCreateAppleUser_VerifiedClaim_NoExistingAccount_CreatesVerified() {
	svc := s.newService()
	user, err := svc.FindOrCreateAppleUser(&contracts.AppleIdentityTokenClaims{
		Email:         "apple-fresh-verified@example.com",
		EmailVerified: true,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "apple-sub-fresh-verified",
		},
	}, "Apple", "Name")

	s.Require().NoError(err)
	s.Require().NotNil(user)
	s.True(user.EmailVerified)
	s.assertStoredEmailVerified(user.ID, true)
}

// The squat, on the Apple path: an unvouched claim creates an unverified
// account under an address nobody held, and the real owner arriving later with
// a vouched claim is refused rather than joined into it.
func (s *AppleAuthIntegrationTestSuite) TestFindOrCreateAppleUser_SquattedUnverifiedAccount_RefusesVerifiedOwner() {
	svc := s.newService()
	squatted, err := svc.FindOrCreateAppleUser(&contracts.AppleIdentityTokenClaims{
		Email:         "apple-squatted@example.com",
		EmailVerified: false,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "apple-sub-squatter",
		},
	}, "Apple", "Name")
	s.Require().NoError(err)
	s.Require().False(squatted.EmailVerified)

	owner, err := svc.FindOrCreateAppleUser(&contracts.AppleIdentityTokenClaims{
		Email:         "apple-squatted@example.com",
		EmailVerified: true,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "apple-sub-real-owner",
		},
	}, "Apple", "Name")

	s.Require().Error(err)
	s.Require().Nil(owner)

	var authErr *apperrors.AuthError
	s.Require().ErrorAs(err, &authErr)
	s.Equal(apperrors.CodeOAuthLinkRefused, authErr.Code)
	// Apple's own copy. It names no Settings control because there is none to
	// name: Apple arrives by a native-token POST, not the goth handshake that
	// /auth/link/{provider} drives.
	s.Equal(
		"An account already uses this email address. Sign in to that account with the method it already has.",
		authErr.UserMessage(),
	)
	s.NotContains(authErr.UserMessage(), "Settings")

	s.assertStoredEmailVerified(squatted.ID, false)
	var rows int64
	s.Require().NoError(
		s.db.Model(&authm.OAuthAccount{}).Where("user_id = ?", squatted.ID).Count(&rows).Error)
	s.Equal(int64(1), rows, "the refused owner must not have been added to the squatter's account")
}

// The account side of the rule, reached from the ordinary direction.
func (s *AppleAuthIntegrationTestSuite) TestFindOrCreateAppleUser_UnverifiedExistingAccount_RefusesVerifiedClaim() {
	existing := &authm.User{
		Email:         stringPtr("apple-never-verified@example.com"),
		IsActive:      true,
		EmailVerified: false,
	}
	s.Require().NoError(s.db.Create(existing).Error)

	svc := s.newService()
	user, err := svc.FindOrCreateAppleUser(&contracts.AppleIdentityTokenClaims{
		Email:         "apple-never-verified@example.com",
		EmailVerified: true,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "apple-sub-into-unverified",
		},
	}, "Apple", "Name")

	s.Require().Error(err)
	s.Require().Nil(user)

	var authErr *apperrors.AuthError
	s.Require().ErrorAs(err, &authErr)
	s.Equal(apperrors.CodeOAuthLinkRefused, authErr.Code)

	s.assertNoAppleAccountFor(existing.ID)
}

// Reads the column back rather than trusting the struct the service returned.
func (s *AppleAuthIntegrationTestSuite) assertStoredEmailVerified(userID uint, want bool) {
	s.T().Helper()
	var stored authm.User
	s.Require().NoError(s.db.First(&stored, userID).Error)
	s.Equal(want, stored.EmailVerified)
}

// provider_user_id permits the empty string, so a stored row with one would be
// matched by a token carrying no subject, ahead of the address gate.
func (s *AppleAuthIntegrationTestSuite) TestFindOrCreateAppleUser_EmptySubject_Refused() {
	owner := &authm.User{
		Email:         stringPtr("apple-empty-subject-owner@example.com"),
		IsActive:      true,
		EmailVerified: true,
	}
	s.Require().NoError(s.db.Create(owner).Error)
	s.Require().NoError(s.db.Create(&authm.OAuthAccount{
		UserID:         owner.ID,
		Provider:       "apple",
		ProviderUserID: "",
	}).Error)

	svc := s.newService()
	user, err := svc.FindOrCreateAppleUser(&contracts.AppleIdentityTokenClaims{
		Email:         "apple-someone-else@example.com",
		EmailVerified: true,
	}, "Apple", "Name")

	s.Require().Error(err)
	s.Require().Nil(user)
	s.Contains(err.Error(), "no subject")
}

// A refused sign-in must not fall through to the create path: the mailbox
// still resolves to exactly one row. Uses the same case-folding fragment the
// service looks addresses up with, so a second row differing only in case
// counts here too.
func (s *AppleAuthIntegrationTestSuite) assertSingleUserForAddress(email string) {
	s.T().Helper()
	var rows int64
	s.Require().NoError(
		s.db.Model(&authm.User{}).Where(authm.EmailIdentityWhere, email).Count(&rows).Error)
	s.Equal(int64(1), rows, "a refused sign-in must not mint a second account for the mailbox")
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
