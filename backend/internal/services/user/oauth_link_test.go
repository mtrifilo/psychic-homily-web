package user

import (
	"github.com/markbates/goth"

	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
)

// The authenticated link resolves the account from the caller's session, so
// the provider's address is not consulted at all: it may match nothing, match
// another account, or be absent, and the link lands on the session's account
// either way.

func (suite *UserServiceIntegrationTestSuite) TestLinkOAuthAccountToUser_AttachesRegardlessOfAddress() {
	owner := &authm.User{
		Email:         stringPtr("link.session.owner@example.com"),
		IsActive:      true,
		EmailVerified: false,
	}
	suite.Require().NoError(suite.db.Create(owner).Error)

	other := &authm.User{
		Email:         stringPtr("link.other.mailbox@example.com"),
		IsActive:      true,
		EmailVerified: true,
	}
	suite.Require().NoError(suite.db.Create(other).Error)

	linked, err := suite.userService.LinkOAuthAccountToUser(owner.ID, goth.User{
		UserID: "link-session-subject",
		// Another account's address, and no verification signal: neither is
		// consulted, because the session already named the account.
		Email: "link.other.mailbox@example.com",
	}, "google")

	suite.Require().NoError(err)
	suite.Require().NotNil(linked)
	suite.Equal(owner.ID, linked.ID)
	suite.Require().Len(linked.OAuthAccounts, 1)
	suite.Equal("link-session-subject", linked.OAuthAccounts[0].ProviderUserID)

	suite.assertNoOAuthAccountFor(other.ID)
}

// A link proves possession of the provider account, not of the mailbox on the
// account it attaches to, so it must not advance verification.
func (suite *UserServiceIntegrationTestSuite) TestLinkOAuthAccountToUser_LeavesEmailVerificationAlone() {
	owner := &authm.User{
		Email:         stringPtr("link.stays.unverified@example.com"),
		IsActive:      true,
		EmailVerified: false,
	}
	suite.Require().NoError(suite.db.Create(owner).Error)

	_, err := suite.userService.LinkOAuthAccountToUser(owner.ID, goth.User{
		UserID:  "link-unverified-subject",
		Email:   "link.stays.unverified@example.com",
		RawData: map[string]any{"verified_email": true},
	}, "google")
	suite.Require().NoError(err)

	suite.assertStoredEmailVerified(owner.ID, false)
	var stored authm.User
	suite.Require().NoError(suite.db.First(&stored, owner.ID).Error)
	suite.Equal("link.stays.unverified@example.com", *stored.Email)
}

// Connecting the account you already connected is a no-op, not an error: a
// double submit or a back button must not tell the user something broke.
func (suite *UserServiceIntegrationTestSuite) TestLinkOAuthAccountToUser_SameIdentityIsIdempotent() {
	owner := &authm.User{
		Email:         stringPtr("link.idempotent@example.com"),
		IsActive:      true,
		EmailVerified: true,
	}
	suite.Require().NoError(suite.db.Create(owner).Error)

	gothUser := goth.User{UserID: "link-idempotent-subject", Email: "link.idempotent@example.com"}
	_, err := suite.userService.LinkOAuthAccountToUser(owner.ID, gothUser, "google")
	suite.Require().NoError(err)

	again, err := suite.userService.LinkOAuthAccountToUser(owner.ID, gothUser, "google")
	suite.Require().NoError(err)
	suite.Require().Len(again.OAuthAccounts, 1)

	var rows int64
	suite.Require().NoError(
		suite.db.Model(&authm.OAuthAccount{}).Where("user_id = ?", owner.ID).Count(&rows).Error)
	suite.Equal(int64(1), rows)
}

// Moving an identity between accounts would take sign-in access away from the
// account that holds it, so it is refused rather than resolved.
func (suite *UserServiceIntegrationTestSuite) TestLinkOAuthAccountToUser_IdentityHeldByAnotherAccount_Refused() {
	holder := &authm.User{
		Email:         stringPtr("link.holder@example.com"),
		IsActive:      true,
		EmailVerified: true,
	}
	suite.Require().NoError(suite.db.Create(holder).Error)
	suite.Require().NoError(suite.db.Create(&authm.OAuthAccount{
		UserID:         holder.ID,
		Provider:       "google",
		ProviderUserID: "link-contested-subject",
	}).Error)

	claimant := &authm.User{
		Email:         stringPtr("link.claimant@example.com"),
		IsActive:      true,
		EmailVerified: true,
	}
	suite.Require().NoError(suite.db.Create(claimant).Error)

	linked, err := suite.userService.LinkOAuthAccountToUser(claimant.ID, goth.User{
		UserID: "link-contested-subject",
		Email:  "link.claimant@example.com",
	}, "google")

	suite.Require().Error(err)
	suite.Require().Nil(linked)

	var authErr *apperrors.AuthError
	suite.Require().ErrorAs(err, &authErr)
	suite.Equal(apperrors.CodeOAuthIdentityInUse, authErr.Code)

	suite.assertNoOAuthAccountFor(claimant.ID)
	var stillHolders int64
	suite.Require().NoError(
		suite.db.Model(&authm.OAuthAccount{}).
			Where("provider = ? AND provider_user_id = ? AND user_id = ?", "google", "link-contested-subject", holder.ID).
			Count(&stillHolders).Error)
	suite.Equal(int64(1), stillHolders)
}

// Replacing the stored subject silently retargets which provider account can
// sign in as this user, so a second identity from the same provider is
// refused. Disconnecting first is the explicit way to change it.
func (suite *UserServiceIntegrationTestSuite) TestLinkOAuthAccountToUser_SecondIdentityForSameProvider_Refused() {
	owner := &authm.User{
		Email:         stringPtr("link.second.identity@example.com"),
		IsActive:      true,
		EmailVerified: true,
	}
	suite.Require().NoError(suite.db.Create(owner).Error)
	suite.Require().NoError(suite.db.Create(&authm.OAuthAccount{
		UserID:         owner.ID,
		Provider:       "google",
		ProviderUserID: "link-first-subject",
	}).Error)

	linked, err := suite.userService.LinkOAuthAccountToUser(owner.ID, goth.User{
		UserID: "link-second-subject",
		Email:  "link.second.identity@example.com",
	}, "google")

	suite.Require().Error(err)
	suite.Require().Nil(linked)

	var authErr *apperrors.AuthError
	suite.Require().ErrorAs(err, &authErr)
	suite.Equal(apperrors.CodeOAuthProviderAlreadyLinked, authErr.Code)

	var stored authm.OAuthAccount
	suite.Require().NoError(
		suite.db.Where("user_id = ? AND provider = ?", owner.ID, "google").First(&stored).Error)
	suite.Equal("link-first-subject", stored.ProviderUserID,
		"a refused link must leave the stored subject untouched")
}

// provider_user_id permits the empty string and is the value this resolves on,
// so a provider returning no subject must be refused before anything is
// written or matched.
func (suite *UserServiceIntegrationTestSuite) TestLinkOAuthAccountToUser_EmptyProviderUserID_Refused() {
	owner := &authm.User{
		Email:         stringPtr("link.empty.subject@example.com"),
		IsActive:      true,
		EmailVerified: true,
	}
	suite.Require().NoError(suite.db.Create(owner).Error)

	linked, err := suite.userService.LinkOAuthAccountToUser(owner.ID, goth.User{
		UserID: "",
		Email:  "link.empty.subject@example.com",
	}, "google")

	suite.Require().Error(err)
	suite.Require().Nil(linked)
	suite.Contains(err.Error(), "no user id")
	suite.assertNoOAuthAccountFor(owner.ID)
}

// The link is the remediation the refusal names, so the sequence the refusal
// sends a user through has to actually work: refused by address, then linked
// from a session.
func (suite *UserServiceIntegrationTestSuite) TestLinkOAuthAccountToUser_CompletesWhatTheAddressRefusal_SendsUserTo() {
	account := &authm.User{
		Email:         stringPtr("link.remediation@example.com"),
		IsActive:      true,
		EmailVerified: false,
	}
	suite.Require().NoError(suite.db.Create(account).Error)

	gothUser := goth.User{
		UserID:  "link-remediation-subject",
		Email:   "link.remediation@example.com",
		RawData: map[string]any{"verified_email": true},
	}

	refused, err := suite.userService.FindOrCreateUser(gothUser, "google")
	suite.Require().Error(err)
	suite.Require().Nil(refused)

	linked, err := suite.userService.LinkOAuthAccountToUser(account.ID, gothUser, "google")
	suite.Require().NoError(err)
	suite.Equal(account.ID, linked.ID)
	suite.Require().Len(linked.OAuthAccounts, 1)

	// And the identity now resolves by subject, so the next sign-in is a
	// sign-in rather than the refused address comparison.
	resolved, err := suite.userService.FindOrCreateUser(gothUser, "google")
	suite.Require().NoError(err)
	suite.Equal(account.ID, resolved.ID)
}

func (suite *UserServiceIntegrationTestSuite) TestLinkOAuthAccountToUser_UnknownUser_Refused() {
	linked, err := suite.userService.LinkOAuthAccountToUser(9_000_001, goth.User{
		UserID: "link-unknown-user-subject",
		Email:  "link.unknown@example.com",
	}, "google")

	suite.Require().Error(err)
	suite.Require().Nil(linked)

	// Refused before the write, not by the write failing: an orphan row keyed
	// on a user id nothing holds would be an identity nobody can unlink.
	var rows int64
	suite.Require().NoError(
		suite.db.Model(&authm.OAuthAccount{}).
			Where("provider_user_id = ?", "link-unknown-user-subject").
			Count(&rows).Error)
	suite.Equal(int64(0), rows)
}
