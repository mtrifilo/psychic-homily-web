package user

import (
	"time"

	"github.com/markbates/goth"

	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/testutil/signupconsent"
)

// No path that writes an oauth_accounts row stores the provider's access
// token, refresh token or their expiry, even when the provider sent all three.

// providerCredentials is user as a real provider returns it: carrying an
// access token, a refresh token and the token's expiry beside the profile.
func providerCredentials(user goth.User) goth.User {
	user.AccessToken = "provider-access-token"
	user.RefreshToken = "provider-refresh-token"
	user.ExpiresAt = time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	return user
}

// requireNoProviderCredential asserts that account's row holds NULL in every
// provider-credential column. It reads the columns by name rather than through
// the model, so a model change cannot hide a stored value.
func (suite *UserServiceIntegrationTestSuite) requireNoProviderCredential(account authm.OAuthAccount) {
	suite.T().Helper()
	var holding int64
	suite.Require().NoError(suite.db.Raw(
		`SELECT count(*) FROM oauth_accounts
		  WHERE id = ?
		    AND (access_token IS NOT NULL OR refresh_token IS NOT NULL OR expires_at IS NOT NULL)`,
		account.ID).Scan(&holding).Error)
	suite.Zero(holding, "a provider credential is stored for oauth_accounts row %d", account.ID)
}

func (suite *UserServiceIntegrationTestSuite) TestOAuthSignIn_NewAccountStoresNoProviderCredential() {
	identity := providerCredentials(goth.User{
		UserID:  "credentials-new-account-subject",
		Email:   "credentials.new.account@example.com",
		RawData: map[string]any{"verified_email": true},
	})

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
	created, err := suite.userService.FindOrCreateUserWithConsent(providerCredentials(identity), "google", signupconsent.CurrentOAuthSignupConsent())
	suite.Require().NoError(err)

	returning, err := suite.userService.FindOrCreateUserWithConsent(providerCredentials(identity), "google", nil)
	suite.Require().NoError(err)
	suite.Equal(created.ID, returning.ID)

	suite.requireNoProviderCredential(suite.storedGoogleAccount(created.ID))
}

func (suite *UserServiceIntegrationTestSuite) TestLinkOAuthAccountToUser_FirstConnectStoresNoProviderCredential() {
	owner := suite.createLinkableAccount("credentials.first.connect@example.com")

	_, err := suite.userService.LinkOAuthAccountToUser(owner.ID, owner.SessionsValidAfter, providerCredentials(goth.User{
		UserID: "credentials-first-connect-subject",
		Email:  "credentials.first.connect@example.com",
	}), "google")
	suite.Require().NoError(err)

	suite.requireNoProviderCredential(suite.storedGoogleAccount(owner.ID))
}

// AttachOAuthIdentityTx is the write every attach to an existing account goes
// through, the pending-link proofs included; both its insert and its refresh
// leave the credential columns NULL.
func (suite *UserServiceIntegrationTestSuite) TestAttachOAuthIdentityTx_StoresNoProviderCredential() {
	owner := suite.createLinkableAccount("credentials.attach@example.com")
	identity := goth.User{UserID: "credentials-attach-subject", Email: "credentials.attach@example.com"}

	suite.Require().NoError(AttachOAuthIdentityTx(suite.db, owner.ID, "google", providerCredentials(identity)))
	suite.requireNoProviderCredential(suite.storedGoogleAccount(owner.ID))

	suite.Require().NoError(AttachOAuthIdentityTx(suite.db, owner.ID, "google", providerCredentials(identity)))
	suite.requireNoProviderCredential(suite.storedGoogleAccount(owner.ID))
}
