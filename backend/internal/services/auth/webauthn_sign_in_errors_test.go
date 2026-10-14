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
	"psychic-homily-backend/internal/testutil"
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
	_, used, err := suite.service.FinishLogin(&authm.User{ID: userID}, session, parsed)
	return used, err
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

				err := login(user, device)
				suite.requireRefusal(err, apperrors.CodeAccountInactive, apperrors.ErrAccountInactive().UserMessage())
				suite.Contains(err.Error(), fmt.Sprintf("user %d may not sign in", user.ID), "the logged error names the account")

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
		suite.requireRefusal(err, apperrors.CodeInvalidCredentials, "Invalid passkey")
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

		_, _, err = suite.service.FinishLogin(&authm.User{ID: user.ID}, session, parsed)
		suite.requireRefusal(err, apperrors.CodeInvalidCredentials, "User not found")
	})
}

// A valid assertion whose credential update fails is a failure, not a
// refusal, on both ceremonies.
func (suite *WebAuthnServiceIntegrationTestSuite) TestSignInUpdateFailureIsNotARefusal() {
	user := suite.createUser("update-fails@test.com")
	device := suite.registeredDevice(user)

	testutil.FailUpdates(suite.T(), suite.db, "webauthn_credentials")

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
		suite.requireRefusal(err, apperrors.CodeValidationFailed, "Invalid or expired challenge")
		suite.Require().ErrorIs(err, errChallengeNotFound)
		_, err = suite.service.ConsumeRegistrationChallenge(challengeID, other.ID)
		suite.requireRefusal(err, apperrors.CodeValidationFailed, "Invalid or expired challenge")
	})

	suite.Run("another operation's challenge is neither returned nor spent", func() {
		challengeID := store("authentication")
		_, err := suite.service.ConsumeRegistrationChallenge(challengeID, owner.ID)
		suite.requireRefusal(err, apperrors.CodeValidationFailed, "Invalid or expired challenge")
		suite.Equal(int64(1), suite.challengeRows(challengeID))
	})

	suite.Run("an expired challenge is removed and answers as expired", func() {
		challengeID := store("registration")
		suite.Require().NoError(suite.db.Exec("UPDATE webauthn_challenges SET expires_at = ? WHERE id = ?",
			time.Now().Add(-time.Hour), challengeID).Error)
		_, err := suite.service.ConsumeRegistrationChallenge(challengeID, owner.ID)
		suite.requireRefusal(err, apperrors.CodeValidationFailed, "Invalid or expired challenge")
		suite.Require().ErrorIs(err, errChallengeExpired)
		suite.Zero(suite.challengeRows(challengeID))
	})
}

// The account's state is decided only for an assertion that validated: an
// assertion that fails validation is refused as invalid credentials whatever
// state the account is in, so it learns nothing about that state.
func (suite *WebAuthnServiceIntegrationTestSuite) TestSignInRefusesInvalidAssertionBeforeAccountState() {
	logins := map[string]func(target *authm.User, device *softauthn.Authenticator) error{
		// The device belongs to another account, so the target's login
		// session does not allow its credential.
		"email": func(target *authm.User, device *softauthn.Authenticator) error {
			_, err := suite.emailLogin(target.ID, device)
			return err
		},
		// The assertion names the target's user handle, which is not the
		// handle of the credential's account.
		"discoverable": func(target *authm.User, device *softauthn.Authenticator) error {
			options, session, err := suite.service.BeginDiscoverableLogin()
			suite.Require().NoError(err)
			body, err := device.Assert(options)
			suite.Require().NoError(err)
			parsed, err := protocol.ParseCredentialRequestResponseBytes(body)
			suite.Require().NoError(err)
			parsed.Response.UserHandle = target.WebAuthnID()
			_, _, err = suite.service.FinishDiscoverableLogin(session, parsed)
			return err
		},
	}
	for name, login := range logins {
		suite.Run(name, func() {
			target := suite.createUser(name + "-inactive-target@test.com")
			suite.registeredDevice(target)
			holder := suite.createUser(name + "-holder@test.com")
			device := suite.registeredDevice(holder)
			for _, inactive := range []*authm.User{target, holder} {
				suite.Require().NoError(suite.db.Model(&authm.User{}).Where("id = ?", inactive.ID).Update("is_active", false).Error)
			}

			suite.requireRefusal(login(target, device), apperrors.CodeInvalidCredentials, "Invalid passkey")
		})
	}
}

// A challenge id StoreChallenge could not have issued is refused before any
// query, so a value the database would reject is a refusal, not a failure.
func (suite *WebAuthnServiceIntegrationTestSuite) TestConsumeRefusesMalformedChallengeIDs() {
	user := suite.createUser("malformed@test.com")
	for _, challengeID := range []string{"", "not-a-uuid", "\x00"} {
		_, _, err := suite.service.ConsumeChallenge(challengeID, "authentication")
		suite.requireRefusal(err, apperrors.CodeValidationFailed, "Invalid or expired challenge")
		_, err = suite.service.ConsumeRegistrationChallenge(challengeID, user.ID)
		suite.requireRefusal(err, apperrors.CodeValidationFailed, "Invalid or expired challenge")
	}
}

// No operation-generic consume spends a registration challenge.
func (suite *WebAuthnServiceIntegrationTestSuite) TestGenericConsumesRefuseRegistrationChallenges() {
	user := suite.createUser("generic@test.com")
	challengeID, err := suite.service.StoreChallenge(user.ID, &webauthn.SessionData{Challenge: "c"}, "registration")
	suite.Require().NoError(err)

	_, _, err = suite.service.ConsumeChallenge(challengeID, "registration")
	suite.Require().ErrorIs(err, errRegistrationOperation)
	_, _, err = suite.service.ConsumeSignupChallenge(challengeID, "registration")
	suite.Require().ErrorIs(err, errRegistrationOperation)
	suite.Equal(int64(1), suite.challengeRows(challengeID))
}

// Another user's expired registration challenge answers as expired, not as
// someone else's.
func (suite *WebAuthnServiceIntegrationTestSuite) TestExpiredRegistrationChallengeOfAnotherUser() {
	owner := suite.createUser("expired-owner@test.com")
	other := suite.createUser("expired-other@test.com")
	challengeID, err := suite.service.StoreChallenge(owner.ID, &webauthn.SessionData{Challenge: "c"}, "registration")
	suite.Require().NoError(err)
	suite.Require().NoError(suite.db.Exec("UPDATE webauthn_challenges SET expires_at = ? WHERE id = ?",
		time.Now().Add(-time.Hour), challengeID).Error)

	_, err = suite.service.ConsumeRegistrationChallenge(challengeID, other.ID)
	suite.requireRefusal(err, apperrors.CodeValidationFailed, "Invalid or expired challenge")
}

// A credential deleted between its lookup and the use being recorded no
// longer signs in.
func (suite *WebAuthnServiceIntegrationTestSuite) TestCredentialDeletedDuringSignInIsRefused() {
	user := suite.createUser("revoked@test.com")
	suite.registeredDevice(user)
	var stored authm.WebAuthnCredential
	suite.Require().NoError(suite.db.Where("user_id = ?", user.ID).First(&stored).Error)
	suite.Require().NoError(suite.db.Delete(&authm.WebAuthnCredential{}, stored.ID).Error)

	err := suite.service.recordCredentialUse(&stored, &webauthn.Credential{Authenticator: webauthn.Authenticator{SignCount: 2}})
	suite.requireRefusal(err, apperrors.CodeInvalidCredentials, "Invalid passkey")
}
