package auth

import (
	"testing"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/config"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil/softauthn"
)

// These drive whole ceremonies through the service with a software
// authenticator, so every credential here is one the WebAuthn library itself
// verified on the way in and on the way out.

// ceremonyOrigin is the origin the integration suite's service accepts and
// the software authenticator reports.
const ceremonyOrigin = "http://localhost:3000"

func TestNewWebAuthnService_OffersEveryAuthenticatorAttachment(t *testing.T) {
	cfg := &config.Config{
		WebAuthn: config.WebAuthnConfig{RPDisplayName: "Test App", RPID: "localhost", RPOrigins: []string{ceremonyOrigin}},
	}
	svc, err := NewWebAuthnService(&gorm.DB{}, cfg)
	require.NoError(t, err)

	selection := svc.webAuthn.Config.AuthenticatorSelection
	assert.Empty(t, selection.AuthenticatorAttachment,
		"an attachment preference hides platform authenticators or security keys from the ceremony")
}

func (suite *WebAuthnServiceIntegrationTestSuite) parseCreation(body []byte) *protocol.ParsedCredentialCreationData {
	parsed, err := protocol.ParseCredentialCreationResponseBytes(body)
	suite.Require().NoError(err)
	return parsed
}

func (suite *WebAuthnServiceIntegrationTestSuite) discoverableLogin(device *softauthn.Authenticator) (*authm.User, error) {
	options, session, err := suite.service.BeginDiscoverableLogin()
	suite.Require().NoError(err)
	body, err := device.Assert(options)
	suite.Require().NoError(err)
	parsed, err := protocol.ParseCredentialRequestResponseBytes(body)
	suite.Require().NoError(err)
	user, _, err := suite.service.FinishDiscoverableLogin(session, parsed)
	return user, err
}

func (suite *WebAuthnServiceIntegrationTestSuite) TestRegisteredPasskeySignsIn() {
	user := suite.createUser("settings-add@test.com")
	device := softauthn.New(ceremonyOrigin)

	options, session, err := suite.service.BeginRegistration(user)
	suite.Require().NoError(err)
	body, err := device.Register(options)
	suite.Require().NoError(err)

	cred, err := suite.service.FinishRegistration(user, session, suite.parseCreation(body), "Laptop")
	suite.Require().NoError(err)
	suite.Equal(uuid.UUID(softauthn.AAGUID).String(), cred.AAGUID)

	signedIn, err := suite.discoverableLogin(device)
	suite.Require().NoError(err)
	suite.Equal(user.ID, signedIn.ID)

	var stored authm.WebAuthnCredential
	suite.Require().NoError(suite.db.First(&stored, cred.ID).Error)
	suite.NotNil(stored.LastUsedAt)
	suite.Equal(uint32(1), stored.SignCount)
}

// The email-specific login receives the user as the user service loads it,
// without its credentials, and must still validate against them.
func (suite *WebAuthnServiceIntegrationTestSuite) TestRegisteredPasskeySignsInByEmail() {
	user := suite.createUser("by-email@test.com")
	device := softauthn.New(ceremonyOrigin)

	options, session, err := suite.service.BeginRegistration(user)
	suite.Require().NoError(err)
	body, err := device.Register(options)
	suite.Require().NoError(err)
	cred, err := suite.service.FinishRegistration(user, session, suite.parseCreation(body), "Laptop")
	suite.Require().NoError(err)

	loginOptions, loginSession, err := suite.service.BeginLogin(&authm.User{ID: user.ID})
	suite.Require().NoError(err)
	assertion, err := device.Assert(loginOptions)
	suite.Require().NoError(err)
	parsed, err := protocol.ParseCredentialRequestResponseBytes(assertion)
	suite.Require().NoError(err)

	var asLoadedByUserService authm.User
	suite.Require().NoError(suite.db.First(&asLoadedByUserService, user.ID).Error)
	suite.Require().Empty(asLoadedByUserService.PasskeyCredentials)

	used, err := suite.service.FinishLogin(&asLoadedByUserService, loginSession, parsed)
	suite.Require().NoError(err)
	suite.Equal(cred.ID, used.ID)
	suite.NotNil(used.LastUsedAt)
	suite.Equal(uint32(1), used.SignCount)
}

func (suite *WebAuthnServiceIntegrationTestSuite) TestSignupPasskeySignsInAfterwards() {
	device := softauthn.New(ceremonyOrigin)

	options, session, err := suite.service.BeginRegistrationForEmail("passkey-first@test.com")
	suite.Require().NoError(err)
	body, err := device.Register(options)
	suite.Require().NoError(err)

	user, err := suite.service.FinishSignupRegistrationWithLegal(
		"passkey-first@test.com", session, suite.parseCreation(body), "My Passkey",
		contracts.LegalAcceptance{TermsVersion: "v1", PrivacyVersion: "v1", MinAgeAttested: 16},
	)
	suite.Require().NoError(err)
	suite.Equal(user.WebAuthnID(), device.UserHandle(),
		"the handle minted at signup must be the one login compares against")

	signedIn, err := suite.discoverableLogin(device)
	suite.Require().NoError(err)
	suite.Equal(user.ID, signedIn.ID)
}

func (suite *WebAuthnServiceIntegrationTestSuite) TestSignupHandleCarriesNoEmail() {
	options, _, err := suite.service.BeginRegistrationForEmail("private@test.com")
	suite.Require().NoError(err)

	handle, ok := options.Response.User.ID.(protocol.URLEncodedBase64)
	suite.Require().True(ok, "user.id type %T", options.Response.User.ID)
	_, isUserID := authm.UserIDFromWebAuthnID(handle)
	suite.True(isUserID)
	suite.NotContains(string(handle), "private@test.com")
}

func (suite *WebAuthnServiceIntegrationTestSuite) TestAbandonedSignupBeginLeavesNoRow() {
	var before int64
	suite.Require().NoError(suite.db.Model(&authm.User{}).Count(&before).Error)

	_, session, err := suite.service.BeginRegistrationForEmail("abandoned@test.com")
	suite.Require().NoError(err)
	reservedID, ok := authm.UserIDFromWebAuthnID(session.UserID)
	suite.Require().True(ok)

	var after int64
	suite.Require().NoError(suite.db.Model(&authm.User{}).Count(&after).Error)
	suite.Equal(before, after)

	// A later account never collides with the id the abandoned begin reserved.
	later := suite.createUser("later@test.com")
	suite.Greater(later.ID, reservedID)
}

func (suite *WebAuthnServiceIntegrationTestSuite) TestSignupFinishRejectsSessionWithoutReservedID() {
	device := softauthn.New(ceremonyOrigin)
	options, session, err := suite.service.BeginRegistrationForEmail("legacy@test.com")
	suite.Require().NoError(err)
	body, err := device.Register(options)
	suite.Require().NoError(err)

	session.UserID = []byte("legacy@test.com")
	_, err = suite.service.FinishSignupRegistrationWithLegal(
		"legacy@test.com", session, suite.parseCreation(body), "My Passkey", contracts.LegalAcceptance{},
	)
	suite.Require().Error(err)

	var count int64
	suite.Require().NoError(suite.db.Model(&authm.User{}).Where("email = ?", "legacy@test.com").Count(&count).Error)
	suite.Zero(count)
}
