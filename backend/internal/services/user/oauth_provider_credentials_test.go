package user

import (
	"github.com/markbates/goth"

	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/testutil/signupconsent"
)

// No path that writes an oauth_accounts row stores the provider's access
// token, refresh token or their expiry, even when the provider sent all three.

// requireNoProviderCredential asserts that account's row holds NULL in every
// provider-credential column. It reads the columns by name rather than through
// the model, so a model change cannot hide a stored value.
func (suite *UserServiceIntegrationTestSuite) requireNoProviderCredential(account authm.OAuthAccount) {
	suite.T().Helper()
	var stored struct {
		AccessTokenNull  bool
		RefreshTokenNull bool
		ExpiresAtNull    bool
	}
	suite.Require().NoError(suite.db.Raw(
		`SELECT access_token IS NULL AS access_token_null,
		        refresh_token IS NULL AS refresh_token_null,
		        expires_at IS NULL AS expires_at_null
		   FROM oauth_accounts WHERE id = ?`, account.ID).Scan(&stored).Error)
	suite.True(stored.AccessTokenNull, "access_token is stored for oauth_accounts row %d", account.ID)
	suite.True(stored.RefreshTokenNull, "refresh_token is stored for oauth_accounts row %d", account.ID)
	suite.True(stored.ExpiresAtNull, "expires_at is stored for oauth_accounts row %d", account.ID)
}

func (suite *UserServiceIntegrationTestSuite) TestOAuthSignIn_NewAccountStoresNoProviderCredential() {
	identity := providerCredentials(goth.User{
		UserID:  "credentials-new-account-subject",
		Email:   "credentials.new.account@example.com",
		RawData: map[string]any{"verified_email": true},
	}, "new-account")

	user, err := suite.userService.FindOrCreateUserWithConsent(identity, "google", signupconsent.CurrentOAuthSignupConsent())
	suite.Require().NoError(err)

	suite.requireNoProviderCredential(suite.storedGoogleAccount(user.ID))
}

func (suite *UserServiceIntegrationTestSuite) TestOAuthSignIn_ReturningSignInStoresNoProviderCredential() {
	identity := goth.User{
		UserID:  "credentials-returning-subject",
		Email:   "credentials.returning@example.com",
		RawData: map[string]any{"verified_email": true},
	}
	created, err := suite.userService.FindOrCreateUserWithConsent(providerCredentials(identity, "first"), "google", signupconsent.CurrentOAuthSignupConsent())
	suite.Require().NoError(err)

	returning, err := suite.userService.FindOrCreateUserWithConsent(providerCredentials(identity, "returning"), "google", nil)
	suite.Require().NoError(err)
	suite.Equal(created.ID, returning.ID)

	suite.requireNoProviderCredential(suite.storedGoogleAccount(created.ID))
}

func (suite *UserServiceIntegrationTestSuite) TestLinkOAuthAccountToUser_FirstConnectStoresNoProviderCredential() {
	owner := suite.createLinkableAccount("credentials.first.connect@example.com")

	_, err := suite.userService.LinkOAuthAccountToUser(owner.ID, owner.SessionsValidAfter, providerCredentials(goth.User{
		UserID: "credentials-first-connect-subject",
		Email:  "credentials.first.connect@example.com",
	}, "first-connect"), "google")
	suite.Require().NoError(err)

	suite.requireNoProviderCredential(suite.storedGoogleAccount(owner.ID))
}

// AttachOAuthIdentityTx is the write every attach to an existing account goes
// through, the pending-link proofs included; both its insert and its refresh
// leave the credential columns NULL.
func (suite *UserServiceIntegrationTestSuite) TestAttachOAuthIdentityTx_StoresNoProviderCredential() {
	owner := suite.createLinkableAccount("credentials.attach@example.com")
	identity := goth.User{UserID: "credentials-attach-subject", Email: "credentials.attach@example.com"}

	suite.Require().NoError(AttachOAuthIdentityTx(suite.db, owner.ID, "google", providerCredentials(identity, "insert")))
	suite.requireNoProviderCredential(suite.storedGoogleAccount(owner.ID))

	suite.Require().NoError(AttachOAuthIdentityTx(suite.db, owner.ID, "google", providerCredentials(identity, "refresh")))
	suite.requireNoProviderCredential(suite.storedGoogleAccount(owner.ID))
}
