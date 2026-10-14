package auth

import (
	"errors"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/testutil/softauthn"
)

func TestSignInLookupError(t *testing.T) {
	var refusal *apperrors.AuthError
	missing := signInLookupError("credential", gorm.ErrRecordNotFound)
	if !errors.As(missing, &refusal) || refusal.Code != apperrors.CodeInvalidCredentials {
		t.Errorf("a row that does not exist is a refusal, got %v", missing)
	}

	failed := signInLookupError("credential", fmt.Errorf("connection reset"))
	if errors.As(failed, &refusal) {
		t.Errorf("a lookup that could not run is not a refusal, got %v", failed)
	}
}

// requireRefusal asserts err is a refusal with code and user message.
func (suite *WebAuthnServiceIntegrationTestSuite) requireRefusal(err error, code, message string) {
	suite.T().Helper()
	var refusal *apperrors.AuthError
	suite.Require().True(errors.As(err, &refusal), "expected an AuthError refusal, got %v", err)
	suite.Equal(code, refusal.Code)
	suite.Equal(message, refusal.UserMessage())
}

// emailLogin runs an email-specific login for userID with device.
func (suite *WebAuthnServiceIntegrationTestSuite) emailLogin(userID uint, device *softauthn.Authenticator) (*authm.WebAuthnCredential, error) {
	options, session, err := suite.service.BeginLogin(&authm.User{ID: userID})
	suite.Require().NoError(err)
	body, err := device.Assert(options)
	suite.Require().NoError(err)
	parsed, err := protocol.ParseCredentialRequestResponseBytes(body)
	suite.Require().NoError(err)
	return suite.service.FinishLogin(&authm.User{ID: userID}, session, parsed)
}

// The account gate runs in the service on both ceremonies, after a valid
// assertion, and a refused sign-in records no use of the credential.
func (suite *WebAuthnServiceIntegrationTestSuite) TestSignInRefusesAccountsThatMayNotSignIn() {
	states := map[string]map[string]any{
		"deactivated":     {"is_active": false},
		"soft-deleted":    {"is_active": false, "deleted_at": time.Now()},
		"deleted-at-only": {"deleted_at": time.Now()},
	}
	logins := map[string]func(*authm.User, *softauthn.Authenticator) error{
		"email": func(user *authm.User, device *softauthn.Authenticator) error {
			_, err := suite.emailLogin(user.ID, device)
			return err
		},
		"discoverable": func(_ *authm.User, device *softauthn.Authenticator) error {
			_, err := suite.discoverableLogin(device)
			return err
		},
	}
	for loginName, login := range logins {
		for stateName, updates := range states {
			suite.Run(loginName+"/"+stateName, func() {
				user := suite.createUser(loginName + "-" + stateName + "@test.com")
				device := suite.registeredDevice(user)
				suite.Require().NoError(suite.db.Model(&authm.User{}).Where("id = ?", user.ID).Updates(updates).Error)

				suite.requireRefusal(login(user, device), apperrors.CodeAccountInactive, apperrors.ErrAccountInactive().UserMessage())

				var stored authm.WebAuthnCredential
				suite.Require().NoError(suite.db.Where("user_id = ?", user.ID).First(&stored).Error)
				suite.Nil(stored.LastUsedAt)
				suite.Zero(stored.SignCount)
			})
		}
	}
}

// A credential or account that does not exist is refused as invalid
// credentials. Validation refusals are in webauthn_ceremony_test.go.
func (suite *WebAuthnServiceIntegrationTestSuite) TestSignInRefusesWhatDoesNotExist() {
	suite.Run("credential the server does not hold", func() {
		device := softauthn.New(ceremonyOrigin)
		stranger := suite.createUser("unregistered@test.com")
		options, _, err := suite.service.BeginRegistration(stranger)
		suite.Require().NoError(err)
		_, err = device.Register(options)
		suite.Require().NoError(err)

		_, err = suite.discoverableLogin(device)
		suite.requireRefusal(err, apperrors.CodeInvalidCredentials, invalidPasskeyMessage)
	})

	suite.Run("account that does not exist", func() {
		user := suite.createUser("vanished@test.com")
		device := suite.registeredDevice(user)
		options, session, err := suite.service.BeginLogin(&authm.User{ID: user.ID})
		suite.Require().NoError(err)
		body, err := device.Assert(options)
		suite.Require().NoError(err)
		parsed, err := protocol.ParseCredentialRequestResponseBytes(body)
		suite.Require().NoError(err)
		suite.Require().NoError(suite.db.Exec("DELETE FROM users WHERE id = ?", user.ID).Error)

		_, err = suite.service.FinishLogin(&authm.User{ID: user.ID}, session, parsed)
		suite.requireRefusal(err, apperrors.CodeInvalidCredentials, invalidPasskeyMessage)
	})
}

// A valid assertion whose credential update fails is a failure, not a
// refusal, on both ceremonies.
func (suite *WebAuthnServiceIntegrationTestSuite) TestSignInUpdateFailureIsNotARefusal() {
	user := suite.createUser("update-fails@test.com")
	device := suite.registeredDevice(user)

	suite.Require().NoError(suite.db.Exec(`CREATE OR REPLACE FUNCTION refuse_credential_update() RETURNS trigger AS $$
		BEGIN RAISE EXCEPTION 'credential update refused by test trigger'; END $$ LANGUAGE plpgsql`).Error)
	suite.Require().NoError(suite.db.Exec(`CREATE TRIGGER refuse_credential_update BEFORE UPDATE ON webauthn_credentials
		FOR EACH ROW EXECUTE FUNCTION refuse_credential_update()`).Error)
	defer func() {
		suite.Require().NoError(suite.db.Exec(`DROP TRIGGER refuse_credential_update ON webauthn_credentials`).Error)
		suite.Require().NoError(suite.db.Exec(`DROP FUNCTION refuse_credential_update()`).Error)
	}()

	_, emailErr := suite.emailLogin(user.ID, device)
	_, discoverableErr := suite.discoverableLogin(device)
	for name, err := range map[string]error{"email": emailErr, "discoverable": discoverableErr} {
		suite.Require().Error(err, name)
		var refusal *apperrors.AuthError
		suite.False(errors.As(err, &refusal), "%s: an update failure is not a refusal: %v", name, err)
		suite.Contains(err.Error(), "failed to update credential", name)
	}
}

// sign_count holds every value an authenticator's 32-bit counter can take.
func (suite *WebAuthnServiceIntegrationTestSuite) TestSignCountStoresFullUint32Range() {
	user := suite.createUser("counter@test.com")
	suite.registeredDevice(user)
	var stored authm.WebAuthnCredential
	suite.Require().NoError(suite.db.Where("user_id = ?", user.ID).First(&stored).Error)

	validated := &webauthn.Credential{Authenticator: webauthn.Authenticator{SignCount: math.MaxUint32}}
	suite.Require().NoError(suite.service.recordCredentialUse(&stored, validated))

	var reread authm.WebAuthnCredential
	suite.Require().NoError(suite.db.First(&reread, stored.ID).Error)
	suite.Equal(uint32(math.MaxUint32), reread.SignCount)
}

// A registration challenge is spent only by the user it was issued to.
func (suite *WebAuthnServiceIntegrationTestSuite) TestConsumeRegistrationChallenge() {
	owner := suite.createUser("challenge-owner@test.com")
	other := suite.createUser("challenge-other@test.com")
	store := func(operation string) string {
		challengeID, err := suite.service.StoreChallenge(owner.ID, &webauthn.SessionData{Challenge: "registration-challenge"}, operation)
		suite.Require().NoError(err)
		return challengeID
	}

	suite.Run("another user is refused and the challenge stays", func() {
		challengeID := store("registration")
		_, err := suite.service.ConsumeRegistrationChallenge(challengeID, other.ID)
		suite.requireRefusal(err, apperrors.CodeUnauthorized, "Challenge belongs to different user")
		suite.Equal(int64(1), suite.challengeRows(challengeID))

		session, err := suite.service.ConsumeRegistrationChallenge(challengeID, owner.ID)
		suite.Require().NoError(err)
		suite.Equal("registration-challenge", session.Challenge)
		suite.Zero(suite.challengeRows(challengeID))
	})

	suite.Run("a spent challenge is not found", func() {
		challengeID := store("registration")
		_, err := suite.service.ConsumeRegistrationChallenge(challengeID, owner.ID)
		suite.Require().NoError(err)
		_, err = suite.service.ConsumeRegistrationChallenge(challengeID, owner.ID)
		suite.Require().ErrorIs(err, errChallengeNotFound)
		_, err = suite.service.ConsumeRegistrationChallenge(challengeID, other.ID)
		suite.Require().ErrorIs(err, errChallengeNotFound)
	})

	suite.Run("another operation's challenge is neither returned nor spent", func() {
		challengeID := store("authentication")
		_, err := suite.service.ConsumeRegistrationChallenge(challengeID, owner.ID)
		suite.Require().ErrorIs(err, errChallengeNotFound)
		suite.Equal(int64(1), suite.challengeRows(challengeID))
	})

	suite.Run("an expired challenge is removed and answers as expired", func() {
		challengeID := store("registration")
		suite.Require().NoError(suite.db.Exec("UPDATE webauthn_challenges SET expires_at = ? WHERE id = ?",
			time.Now().Add(-time.Hour), challengeID).Error)
		_, err := suite.service.ConsumeRegistrationChallenge(challengeID, owner.ID)
		suite.Require().Error(err)
		suite.Contains(err.Error(), "challenge expired")
		suite.Zero(suite.challengeRows(challengeID))
	})
}
