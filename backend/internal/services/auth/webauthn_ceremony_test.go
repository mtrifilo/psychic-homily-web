package auth

import (
	"encoding/json"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"

	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/testutil/signupconsent"
	"psychic-homily-backend/internal/testutil/softauthn"
)

// These drive whole ceremonies through the service with a software
// authenticator, so every credential here is one the WebAuthn library itself
// verified on the way in and on the way out.

func (suite *WebAuthnServiceIntegrationTestSuite) parseCreation(body []byte) *protocol.ParsedCredentialCreationData {
	parsed, err := protocol.ParseCredentialCreationResponseBytes(body)
	suite.Require().NoError(err)
	return parsed
}

func (suite *WebAuthnServiceIntegrationTestSuite) discoverableLogin(device *softauthn.Authenticator) (*authm.User, error) {
	return suite.discoverableLoginAltered(device, nil)
}

// discoverableLoginAltered runs a discoverable login with device, letting
// alter change the session and the parsed assertion before the finish.
func (suite *WebAuthnServiceIntegrationTestSuite) discoverableLoginAltered(device *softauthn.Authenticator, alter func(*webauthn.SessionData, *protocol.ParsedCredentialAssertionData)) (*authm.User, error) {
	options, session, err := suite.service.BeginDiscoverableLogin()
	suite.Require().NoError(err)
	body, err := device.Assert(options)
	suite.Require().NoError(err)
	parsed, err := protocol.ParseCredentialRequestResponseBytes(body)
	suite.Require().NoError(err)
	if alter != nil {
		alter(session, parsed)
	}
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

	suite.Require().NoError(suite.db.Create(&authm.UserPreferences{UserID: user.ID}).Error)
	oauth := authm.OAuthAccount{UserID: user.ID, Provider: "google", ProviderUserID: "discoverable-subject"}
	suite.Require().NoError(suite.db.Create(&oauth).Error)
	defer suite.db.Delete(&oauth)

	signedIn, err := suite.discoverableLogin(device)
	suite.Require().NoError(err)
	suite.Equal(user.ID, signedIn.ID)
	suite.Empty(signedIn.PasskeyCredentials, "the signed-in user is returned without its credentials")
	suite.Require().NotNil(signedIn.Preferences, "the signed-in user is returned with its preferences")
	suite.Require().Len(signedIn.OAuthAccounts, 1, "the signed-in user is returned with its OAuth accounts")
	suite.Equal(oauth.ID, signedIn.OAuthAccounts[0].ID)

	var stored authm.WebAuthnCredential
	suite.Require().NoError(suite.db.First(&stored, cred.ID).Error)
	suite.NotNil(stored.LastUsedAt)
	suite.Equal(uint32(1), stored.SignCount)
}

func (suite *WebAuthnServiceIntegrationTestSuite) TestSignupPasskeySignsInAfterwards() {
	device := softauthn.New(ceremonyOrigin)

	options, session, err := suite.service.BeginRegistrationForEmail("passkey-first@test.com")
	suite.Require().NoError(err)
	body, err := device.Register(options)
	suite.Require().NoError(err)

	user, err := suite.service.FinishSignupRegistrationWithLegal(
		"passkey-first@test.com", session, suite.parseCreation(body), "My Passkey",
		signupconsent.CurrentLegalAcceptance(),
	)
	suite.Require().NoError(err)
	suite.Equal(user.WebAuthnID(), device.UserHandle(),
		"the handle minted at signup must be the one login compares against")

	signedIn, err := suite.discoverableLogin(device)
	suite.Require().NoError(err)
	suite.Equal(user.ID, signedIn.ID)
}

// Signup returns the account as it was stored, with its preferences and the
// values their columns default to, so a caller has nothing to read back.
func (suite *WebAuthnServiceIntegrationTestSuite) TestSignupReturnsTheAccountAsStored() {
	device := softauthn.New(ceremonyOrigin)
	options, session, err := suite.service.BeginRegistrationForEmail("as-stored@test.com")
	suite.Require().NoError(err)
	body, err := device.Register(options)
	suite.Require().NoError(err)

	created, err := suite.service.FinishSignupRegistrationWithLegal(
		"as-stored@test.com", session, suite.parseCreation(body), "My Passkey",
		signupconsent.CurrentLegalAcceptance(),
	)
	suite.Require().NoError(err)

	var stored authm.User
	suite.Require().NoError(suite.db.Preload("Preferences").First(&stored, created.ID).Error)
	suite.Require().NotNil(created.Preferences)
	suite.Require().NotNil(stored.Preferences)
	// The database keeps microseconds; the account in hand keeps what the
	// clock read.
	for _, ts := range []*time.Time{
		&created.CreatedAt, &created.UpdatedAt, &created.Preferences.CreatedAt, &created.Preferences.UpdatedAt,
		&stored.CreatedAt, &stored.UpdatedAt, &stored.Preferences.CreatedAt, &stored.Preferences.UpdatedAt,
	} {
		*ts = ts.UTC().Truncate(time.Microsecond)
	}
	createdJSON, err := json.Marshal(created)
	suite.Require().NoError(err)
	storedJSON, err := json.Marshal(&stored)
	suite.Require().NoError(err)
	suite.JSONEq(string(storedJSON), string(createdJSON))
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
		"legacy@test.com", session, suite.parseCreation(body), "My Passkey", signupconsent.CurrentLegalAcceptance(),
	)
	suite.Require().Error(err)

	var count int64
	suite.Require().NoError(suite.db.Model(&authm.User{}).Where("email = ?", "legacy@test.com").Count(&count).Error)
	suite.Zero(count)
}

// registeredDevice registers a fresh software authenticator for user.
func (suite *WebAuthnServiceIntegrationTestSuite) registeredDevice(user *authm.User) *softauthn.Authenticator {
	device := softauthn.New(ceremonyOrigin)
	options, session, err := suite.service.BeginRegistration(user)
	suite.Require().NoError(err)
	body, err := device.Register(options)
	suite.Require().NoError(err)
	_, err = suite.service.FinishRegistration(user, session, suite.parseCreation(body), "Key")
	suite.Require().NoError(err)
	return device
}

// A credential signs in only as the account that owns it: an assertion whose
// user handle names another account is refused.
func (suite *WebAuthnServiceIntegrationTestSuite) TestDiscoverableLoginRefusesAnotherAccountsHandle() {
	owner := suite.createUser("owner@test.com")
	other := suite.createUser("other@test.com")
	device := suite.registeredDevice(owner)

	_, err := suite.discoverableLoginAltered(device, func(_ *webauthn.SessionData, parsed *protocol.ParsedCredentialAssertionData) {
		parsed.Response.UserHandle = other.WebAuthnID()
	})
	suite.requireRefusal(err, apperrors.CodeInvalidCredentials, "Invalid passkey")
}

// A session that names an account is not a discoverable login's, so a valid
// assertion from that account's own credential is refused with it.
func (suite *WebAuthnServiceIntegrationTestSuite) TestDiscoverableLoginRefusesSessionNamingAnAccount() {
	owner := suite.createUser("named-session@test.com")
	device := suite.registeredDevice(owner)

	_, err := suite.discoverableLoginAltered(device, func(session *webauthn.SessionData, _ *protocol.ParsedCredentialAssertionData) {
		session.UserID = owner.WebAuthnID()
	})
	suite.requireRefusal(err, apperrors.CodeInvalidCredentials, "Invalid passkey")

	var stored authm.WebAuthnCredential
	suite.Require().NoError(suite.db.Where("user_id = ?", owner.ID).First(&stored).Error)
	suite.Nil(stored.LastUsedAt, "a refused sign-in records no use of the credential")
}

// A sign-in records the backup state the authenticator reports now, so a
// passkey that becomes synced after registration reads as synced.
func (suite *WebAuthnServiceIntegrationTestSuite) TestSignInRecordsCurrentBackupState() {
	user := suite.createUser("syncs-later@test.com")
	device := softauthn.New(ceremonyOrigin)
	device.BackupEligible = true

	options, session, err := suite.service.BeginRegistration(user)
	suite.Require().NoError(err)
	body, err := device.Register(options)
	suite.Require().NoError(err)
	cred, err := suite.service.FinishRegistration(user, session, suite.parseCreation(body), "Phone")
	suite.Require().NoError(err)
	suite.False(cred.BackupState)

	device.BackedUp = true
	_, err = suite.discoverableLogin(device)
	suite.Require().NoError(err)

	var stored authm.WebAuthnCredential
	suite.Require().NoError(suite.db.First(&stored, cred.ID).Error)
	suite.True(stored.BackupState)
}
