package user

import (
	"github.com/markbates/goth"

	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/testutil/signupconsent"
)

// The authenticated link resolves the account from the caller's session, so
// the provider's address is not consulted at all: it may match nothing, match
// another account, or be absent, and the link lands on the session's account
// either way.

func (suite *UserServiceIntegrationTestSuite) TestLinkOAuthAccountToUser_AttachesRegardlessOfAddress() {
	owner := &authm.User{
		Email:         stringPtr("link.session.owner@example.com"),
		IsActive:      true,
		EmailVerified: true,
	}
	suite.Require().NoError(suite.db.Create(owner).Error)

	other := &authm.User{
		Email:         stringPtr("link.other.mailbox@example.com"),
		IsActive:      true,
		EmailVerified: true,
	}
	suite.Require().NoError(suite.db.Create(other).Error)

	linked, err := suite.userService.LinkOAuthAccountToUser(owner.ID, owner.SessionsValidAfter, goth.User{
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

// An account whose address is unverified is refused with
// CodeEmailVerificationRequired and nothing is attached, even when the
// provider says it verified the same address.
func (suite *UserServiceIntegrationTestSuite) TestLinkOAuthAccountToUser_UnverifiedAddress_Refused() {
	owner := &authm.User{
		Email:         stringPtr("link.stays.unverified@example.com"),
		IsActive:      true,
		EmailVerified: false,
	}
	suite.Require().NoError(suite.db.Create(owner).Error)

	linked, err := suite.userService.LinkOAuthAccountToUser(owner.ID, owner.SessionsValidAfter, goth.User{
		UserID:  "link-unverified-subject",
		Email:   "link.stays.unverified@example.com",
		RawData: map[string]any{"verified_email": true},
	}, "google")

	suite.Require().Nil(linked)
	var authErr *apperrors.AuthError
	suite.Require().ErrorAs(err, &authErr)
	suite.Equal(apperrors.CodeEmailVerificationRequired, authErr.Code)
	suite.assertNoOAuthAccountFor(owner.ID)
}

// An account with no address (a provider sign-up that supplied none) has no
// mailbox to verify, and links.
func (suite *UserServiceIntegrationTestSuite) TestLinkOAuthAccountToUser_NoAddress_Links() {
	owner := &authm.User{IsActive: true}
	suite.Require().NoError(suite.db.Create(owner).Error)

	linked, err := suite.userService.LinkOAuthAccountToUser(owner.ID, owner.SessionsValidAfter, goth.User{
		UserID: "link-no-address-subject",
		Email:  "provider.address@example.com",
	}, "google")
	suite.Require().NoError(err)
	suite.Require().Len(linked.OAuthAccounts, 1)
	suite.assertStoredEmailVerified(owner.ID, false)
	var stored authm.User
	suite.Require().NoError(suite.db.First(&stored, owner.ID).Error)
	suite.Nil(stored.Email, "the link does not give the account the provider's address")
}

// A verified account links, and the link leaves its address as it was.
func (suite *UserServiceIntegrationTestSuite) TestLinkOAuthAccountToUser_VerifiedAddress_LinksAndLeavesTheAddressAlone() {
	owner := &authm.User{
		Email:         stringPtr("link.verified@example.com"),
		IsActive:      true,
		EmailVerified: true,
	}
	suite.Require().NoError(suite.db.Create(owner).Error)

	linked, err := suite.userService.LinkOAuthAccountToUser(owner.ID, owner.SessionsValidAfter, goth.User{
		UserID: "link-verified-subject",
		Email:  "someone.else@example.com",
	}, "google")
	suite.Require().NoError(err)
	suite.Require().Len(linked.OAuthAccounts, 1)

	suite.assertStoredEmailVerified(owner.ID, true)
	var stored authm.User
	suite.Require().NoError(suite.db.First(&stored, owner.ID).Error)
	suite.Equal("link.verified@example.com", *stored.Email)
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
	_, err := suite.userService.LinkOAuthAccountToUser(owner.ID, owner.SessionsValidAfter, gothUser, "google")
	suite.Require().NoError(err)

	again, err := suite.userService.LinkOAuthAccountToUser(owner.ID, owner.SessionsValidAfter, gothUser, "google")
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

	linked, err := suite.userService.LinkOAuthAccountToUser(claimant.ID, claimant.SessionsValidAfter, goth.User{
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

	linked, err := suite.userService.LinkOAuthAccountToUser(owner.ID, owner.SessionsValidAfter, goth.User{
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

	linked, err := suite.userService.LinkOAuthAccountToUser(owner.ID, owner.SessionsValidAfter, goth.User{
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
// from a session of the verified account.
func (suite *UserServiceIntegrationTestSuite) TestLinkOAuthAccountToUser_CompletesWhatTheAddressRefusal_SendsUserTo() {
	account := &authm.User{
		Email:         stringPtr("link.remediation@example.com"),
		IsActive:      true,
		EmailVerified: true,
	}
	suite.Require().NoError(suite.db.Create(account).Error)

	gothUser := goth.User{
		UserID:  "link-remediation-subject",
		Email:   "link.remediation@example.com",
		RawData: map[string]any{"verified_email": true},
	}

	refused, err := suite.userService.FindOrCreateUserWithConsent(gothUser, "google", signupconsent.CurrentOAuthSignupConsent())
	suite.Require().Error(err)
	suite.Require().Nil(refused)

	linked, err := suite.userService.LinkOAuthAccountToUser(account.ID, account.SessionsValidAfter, gothUser, "google")
	suite.Require().NoError(err)
	suite.Equal(account.ID, linked.ID)
	suite.Require().Len(linked.OAuthAccounts, 1)

	// And the identity now resolves by subject, so the next sign-in is a
	// sign-in rather than the refused address comparison.
	resolved, err := suite.userService.FindOrCreateUserWithConsent(gothUser, "google", signupconsent.CurrentOAuthSignupConsent())
	suite.Require().NoError(err)
	suite.Equal(account.ID, resolved.ID)
}

// On an account whose address is unverified the same remediation takes one
// more step: the link is refused with CodeEmailVerificationRequired until the
// address is verified, and then it links.
func (suite *UserServiceIntegrationTestSuite) TestLinkOAuthAccountToUser_AddressRefusalOnAnUnverifiedAccount_NeedsVerificationFirst() {
	account := &authm.User{Email: stringPtr("link.remediation.unverified@example.com"), IsActive: true}
	suite.Require().NoError(suite.db.Create(account).Error)
	gothUser := goth.User{
		UserID:  "link-remediation-unverified-subject",
		Email:   "link.remediation.unverified@example.com",
		RawData: map[string]any{"verified_email": true},
	}

	_, err := suite.userService.FindOrCreateUserWithConsent(gothUser, "google", signupconsent.CurrentOAuthSignupConsent())
	suite.Require().Error(err)

	_, err = suite.userService.LinkOAuthAccountToUser(account.ID, account.SessionsValidAfter, gothUser, "google")
	var authErr *apperrors.AuthError
	suite.Require().ErrorAs(err, &authErr)
	suite.Equal(apperrors.CodeEmailVerificationRequired, authErr.Code)

	suite.Require().NoError(suite.userService.SetEmailVerified(account.ID, true))
	linked, err := suite.userService.LinkOAuthAccountToUser(account.ID, account.SessionsValidAfter, gothUser, "google")
	suite.Require().NoError(err)
	suite.Require().Len(linked.OAuthAccounts, 1)
}

func (suite *UserServiceIntegrationTestSuite) TestLinkOAuthAccountToUser_UnknownUser_Refused() {
	linked, err := suite.userService.LinkOAuthAccountToUser(9_000_001, nil, goth.User{
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
