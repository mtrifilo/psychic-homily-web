package auth

import (
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// GetSignInFactors runs on the WebAuthn integration suite because that suite
// owns the credential fixtures and clears webauthn_credentials between tests.

// signInFactorsOf loads the account as the profile does, without its
// credentials, and returns its sign-in factors.
func (suite *WebAuthnServiceIntegrationTestSuite) signInFactorsOf(userID uint) contracts.SignInFactors {
	suite.T().Helper()
	var user authm.User
	suite.Require().NoError(suite.db.First(&user, userID).Error)
	factors, err := (&AuthService{db: suite.db}).GetSignInFactors(&user)
	suite.Require().NoError(err)
	suite.Nil(user.PasskeyCredentials, "the factors must count passkeys, not load them onto the user")
	return factors
}

func (suite *WebAuthnServiceIntegrationTestSuite) TestGetSignInFactors_ProviderOnlyAccount() {
	user := suite.createUser("provider-only@test.local")

	suite.Equal(contracts.SignInFactors{HasPassword: false, PasskeyCount: 0}, suite.signInFactorsOf(user.ID))
}

func (suite *WebAuthnServiceIntegrationTestSuite) TestGetSignInFactors_CountsOnlyThisAccountsPasskeys() {
	user := suite.createUser("two-passkeys@test.local")
	other := suite.createUser("someone-else@test.local")
	suite.insertCredential(user.ID, "laptop")
	suite.insertCredential(user.ID, "phone")
	suite.insertCredential(other.ID, "theirs")

	suite.Equal(2, suite.signInFactorsOf(user.ID).PasskeyCount)
	suite.Equal(1, suite.signInFactorsOf(other.ID).PasskeyCount)
}

func (suite *WebAuthnServiceIntegrationTestSuite) TestGetSignInFactors_ReadsThePasswordsPresence() {
	withPassword := suite.createUser("has-password@test.local")
	suite.Require().NoError(suite.db.Model(&authm.User{}).Where("id = ?", withPassword.ID).
		Update("password_hash", "$2a$10$placeholderhashplaceholderhashplaceholderhashpla").Error)
	emptyHash := suite.createUser("empty-hash@test.local")
	suite.Require().NoError(suite.db.Model(&authm.User{}).Where("id = ?", emptyHash.ID).
		Update("password_hash", "").Error)

	suite.True(suite.signInFactorsOf(withPassword.ID).HasPassword)
	suite.False(suite.signInFactorsOf(emptyHash.ID).HasPassword, "an empty hash is no password")
}
