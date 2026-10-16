package auth

import (
	"errors"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"

	"psychic-homily-backend/internal/config"
	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
	"psychic-homily-backend/internal/testutil/signupconsent"
	"psychic-homily-backend/internal/testutil/softauthn"
)

// These split registration, login begin and signup errors into refusals of
// what the client presented and failures to reach a decision.

// requireFailure asserts err is an error that is not a refusal.
func (suite *WebAuthnServiceIntegrationTestSuite) requireFailure(err error) {
	suite.T().Helper()
	suite.Require().Error(err)
	var refusal *apperrors.AuthError
	suite.False(errors.As(err, &refusal), "a failure is not a refusal: %v", err)
}

// unavailableService is the suite's service over a database every query to
// which fails.
func (suite *WebAuthnServiceIntegrationTestSuite) unavailableService() *WebAuthnService {
	svc, err := NewWebAuthnService(testutil.UnavailableDB(suite.T()), &config.Config{
		WebAuthn: config.WebAuthnConfig{RPDisplayName: "Test App", RPID: "localhost", RPOrigins: []string{ceremonyOrigin}},
	})
	suite.Require().NoError(err)
	return svc
}

func (suite *WebAuthnServiceIntegrationTestSuite) credentialRows(userID uint) int64 {
	var count int64
	suite.Require().NoError(suite.db.Model(&authm.WebAuthnCredential{}).Where("user_id = ?", userID).Count(&count).Error)
	return count
}

func (suite *WebAuthnServiceIntegrationTestSuite) accountRows(email string) int64 {
	var count int64
	suite.Require().NoError(suite.db.Model(&authm.User{}).Where("email = ?", email).Count(&count).Error)
	return count
}

// A registration response that fails verification is refused and stores
// nothing.
func (suite *WebAuthnServiceIntegrationTestSuite) TestFinishRegistrationRefusesUnverifiedResponses() {
	user := suite.createUser("register-refused@test.com")
	responses := map[string]func() ([]byte, *webauthn.SessionData){
		"another origin": func() ([]byte, *webauthn.SessionData) {
			options, session, err := suite.service.BeginRegistration(user)
			suite.Require().NoError(err)
			body, err := softauthn.New("https://elsewhere.example").Register(options)
			suite.Require().NoError(err)
			return body, session
		},
		"another challenge": func() ([]byte, *webauthn.SessionData) {
			_, session, err := suite.service.BeginRegistration(user)
			suite.Require().NoError(err)
			otherOptions, _, err := suite.service.BeginRegistration(user)
			suite.Require().NoError(err)
			body, err := softauthn.New(ceremonyOrigin).Register(otherOptions)
			suite.Require().NoError(err)
			return body, session
		},
	}
	for name, respond := range responses {
		suite.Run(name, func() {
			body, session := respond()
			_, err := suite.service.FinishRegistration(user, session, suite.parseCreation(body), "Key")
			suite.requireRefusal(err, apperrors.CodeValidationFailed, "Invalid credential response")
			suite.Zero(suite.credentialRows(user.ID))
		})
	}
}

// A verified registration of a credential id an account already holds is
// refused and stores nothing.
func (suite *WebAuthnServiceIntegrationTestSuite) TestFinishRegistrationRefusesHeldCredential() {
	user := suite.createUser("register-held@test.com")
	device := suite.registeredDevice(user)
	options, session, err := suite.service.BeginRegistration(user)
	suite.Require().NoError(err)
	body, err := device.RegisterAgain(options)
	suite.Require().NoError(err)

	_, err = suite.service.FinishRegistration(user, session, suite.parseCreation(body), "Key")

	suite.requireRefusal(err, apperrors.CodeValidationFailed, "Invalid credential response")
	suite.Equal(int64(1), suite.credentialRows(user.ID))
}

// A verified registration whose credential cannot be stored is a failure.
func (suite *WebAuthnServiceIntegrationTestSuite) TestFinishRegistrationStoreFailureIsNotARefusal() {
	user := suite.createUser("register-store-fails@test.com")
	options, session, err := suite.service.BeginRegistration(user)
	suite.Require().NoError(err)
	body, err := softauthn.New(ceremonyOrigin).Register(options)
	suite.Require().NoError(err)

	testutil.FailInserts(suite.T(), suite.db, "webauthn_credentials")

	_, err = suite.service.FinishRegistration(user, session, suite.parseCreation(body), "Key")
	suite.requireFailure(err)
	suite.Contains(err.Error(), "failed to save credential")
}

// An email-specific login begin refuses an account that does not exist or
// holds no passkey, and fails when the account cannot be read.
func (suite *WebAuthnServiceIntegrationTestSuite) TestBeginLoginRefusalsAndFailures() {
	suite.Run("account that does not exist", func() {
		user := suite.createUser("begin-vanished@test.com")
		suite.Require().NoError(suite.db.Exec("DELETE FROM users WHERE id = ?", user.ID).Error)

		_, _, err := suite.service.BeginLogin(&authm.User{ID: user.ID})
		suite.requireRefusal(err, apperrors.CodeInvalidCredentials, "No passkeys registered for this account")
	})

	suite.Run("account without a passkey", func() {
		user := suite.createUser("begin-passkeyless@test.com")
		_, _, err := suite.service.BeginLogin(&authm.User{ID: user.ID})
		suite.requireRefusal(err, apperrors.CodeInvalidCredentials, "No passkeys registered for this account")
	})

	suite.Run("account that cannot be read", func() {
		_, _, err := suite.unavailableService().BeginLogin(&authm.User{ID: 1})
		suite.requireFailure(err)
		suite.Contains(err.Error(), "failed to load user")
	})
}

// A signup challenge that does not exist, has expired or has a malformed id is
// refused; one that cannot be read is a failure.
func (suite *WebAuthnServiceIntegrationTestSuite) TestConsumeSignupChallengeRefusalsAndFailures() {
	store := func() string {
		challengeID, err := suite.service.StoreSignupChallenge(
			contracts.SignupChallenge{Email: "consume@test.com"}, &webauthn.SessionData{Challenge: "c"}, "signup",
		)
		suite.Require().NoError(err)
		return challengeID
	}

	suite.Run("spent", func() {
		challengeID := store()
		_, _, err := suite.service.ConsumeSignupChallenge(challengeID, "signup")
		suite.Require().NoError(err)
		_, _, err = suite.service.ConsumeSignupChallenge(challengeID, "signup")
		suite.requireRefusal(err, apperrors.CodeValidationFailed, "Invalid or expired challenge")
		suite.Require().ErrorIs(err, errChallengeNotFound)
	})

	suite.Run("expired", func() {
		challengeID := store()
		suite.Require().NoError(suite.db.Exec("UPDATE webauthn_challenges SET expires_at = ? WHERE id = ?",
			time.Now().Add(-time.Hour), challengeID).Error)
		_, _, err := suite.service.ConsumeSignupChallenge(challengeID, "signup")
		suite.requireRefusal(err, apperrors.CodeValidationFailed, "Invalid or expired challenge")
		suite.Require().ErrorIs(err, errChallengeExpired)
	})

	suite.Run("malformed id", func() {
		// The unavailable service fails every query, so a refusal from it
		// shows the id was refused before any query ran.
		unavailable := suite.unavailableService()
		for _, challengeID := range []string{"", "not-a-uuid", "\x00"} {
			_, _, err := suite.service.ConsumeSignupChallenge(challengeID, "signup")
			suite.requireRefusal(err, apperrors.CodeValidationFailed, "Invalid or expired challenge")
			_, _, err = unavailable.ConsumeSignupChallenge(challengeID, "signup")
			suite.requireRefusal(err, apperrors.CodeValidationFailed, "Invalid or expired challenge")
		}
	})

	suite.Run("cannot be read", func() {
		challengeID := store()
		_, _, err := suite.unavailableService().ConsumeSignupChallenge(challengeID, "signup")
		suite.requireFailure(err)
		suite.Equal(int64(1), suite.challengeRows(challengeID))
	})
}

// signupRegistration begins a signup for email and returns the session and
// the registration a fresh software authenticator for origin answers with.
func (suite *WebAuthnServiceIntegrationTestSuite) signupRegistration(email, origin string) (*webauthn.SessionData, []byte) {
	options, session, err := suite.service.BeginRegistrationForEmail(email)
	suite.Require().NoError(err)
	body, err := softauthn.New(origin).Register(options)
	suite.Require().NoError(err)
	return session, body
}

func (suite *WebAuthnServiceIntegrationTestSuite) finishSignup(email string, session *webauthn.SessionData, body []byte) error {
	_, err := suite.service.FinishSignupRegistrationWithLegal(
		email, session, suite.parseCreation(body), "Key",
		signupconsent.CurrentLegalAcceptance(),
	)
	return err
}

// A signup is refused for a registration that fails verification or an
// address an account holds, and fails, creating no account, when a row
// cannot be written.
func (suite *WebAuthnServiceIntegrationTestSuite) TestFinishSignupRefusalsAndFailures() {
	suite.Run("registration from another origin", func() {
		email := "signup-elsewhere@test.com"
		session, body := suite.signupRegistration(email, "https://elsewhere.example")
		suite.requireRefusal(suite.finishSignup(email, session, body), apperrors.CodeValidationFailed, "Invalid credential response")
		suite.Zero(suite.accountRows(email))
	})

	suite.Run("address taken", func() {
		email := "signup-taken@test.com"
		session, body := suite.signupRegistration(email, ceremonyOrigin)
		suite.createUser(email)
		suite.requireRefusal(suite.finishSignup(email, session, body), apperrors.CodeUserExists, "An account with this email already exists")
		suite.Equal(int64(1), suite.accountRows(email))
	})

	for _, table := range []string{"users", "user_preferences", "webauthn_credentials"} {
		suite.Run("insert into "+table+" fails", func() {
			email := "signup-" + table + "-fails@test.com"
			session, body := suite.signupRegistration(email, ceremonyOrigin)
			testutil.FailInserts(suite.T(), suite.db, table)

			err := suite.finishSignup(email, session, body)
			suite.requireFailure(err)
			suite.Contains(err.Error(), testutil.FailedInsertMessage)
			suite.Zero(suite.accountRows(email))
		})
	}
}

// An acceptance that names no terms version records no consent, so the
// passkey sign-up refuses it and creates no account.
func (suite *WebAuthnServiceIntegrationTestSuite) TestFinishSignupRefusesEmptyAcceptance() {
	for _, tc := range []struct {
		email      string
		acceptance contracts.LegalAcceptance
	}{
		{"signup-zero-acceptance@test.com", contracts.LegalAcceptance{}},
		{"signup-no-terms-version@test.com", contracts.LegalAcceptance{PrivacyVersion: "v1", MinAgeAttested: 16}},
	} {
		suite.Run(tc.email, func() {
			session, body := suite.signupRegistration(tc.email, ceremonyOrigin)
			_, err := suite.service.FinishSignupRegistrationWithLegal(
				tc.email, session, suite.parseCreation(body), "Key", tc.acceptance,
			)
			suite.requireRefusal(err, apperrors.CodeTermsAcceptanceRequired,
				"Please accept the Terms of Service and Privacy Policy before creating an account.")
			suite.Zero(suite.accountRows(tc.email))
		})
	}
}
