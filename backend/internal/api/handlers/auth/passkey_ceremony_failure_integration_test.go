package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"time"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	autherrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	usersvc "psychic-homily-backend/internal/services/user"
	"psychic-homily-backend/internal/testutil"
	"psychic-homily-backend/internal/testutil/softauthn"
)

// These drive register finish, email login begin and signup finish over the
// real services and the suite's database, with a software authenticator, a
// write the database refuses, or a database that cannot be reached.

const elsewhereOrigin = "https://elsewhere.example"

func (s *AuthHandlerIntegrationSuite) requireServiceUnavailable(resp *httptest.ResponseRecorder) {
	s.T().Helper()
	requireWireOutcome(s.T(), resp, unavailable, testutil.FailedInsertMessage, "unavailable.invalid", "database is closed")
}

func (s *AuthHandlerIntegrationSuite) credentialCount(userID uint) int64 {
	var count int64
	s.Require().NoError(s.deps.DB.Model(&authm.WebAuthnCredential{}).Where("user_id = ?", userID).Count(&count).Error)
	return count
}

// A registration the relying party does not accept is refused; one whose
// credential cannot be stored fails.
func (s *AuthHandlerIntegrationSuite) TestPasskeyRegisterFinishRefusalAndFailure() {
	email := "register-split@example.com"
	user := &authm.User{Email: &email, IsActive: true}
	s.Require().NoError(s.deps.DB.Create(user).Error)
	h := s.newRealPasskeyHandler()
	ctx := testhelpers.CtxWithSessionAuthTime(user, time.Now())
	finish := func(origin string) (*FinishRegisterResponse, error) {
		begin, err := h.BeginRegisterHandler(ctx, &BeginRegisterRequest{})
		s.Require().NoError(err)
		s.Require().True(begin.Body.Success, begin.Body.Message)
		registration, err := softauthn.New(origin).Register(begin.Body.Options)
		s.Require().NoError(err)
		input := &FinishRegisterRequest{}
		input.Body.ChallengeID = begin.Body.ChallengeID
		input.Body.Response = registration
		return h.FinishRegisterHandler(ctx, input)
	}

	s.Run("registration from another origin is refused", func() {
		refused, err := finish(elsewhereOrigin)
		s.Require().NoError(err)
		s.False(refused.Body.Success)
		s.Equal(autherrors.CodeValidationFailed, refused.Body.ErrorCode)
		s.Equal("Invalid credential response", refused.Body.Message)
		s.Zero(s.credentialCount(user.ID))
	})

	s.Run("credential that cannot be stored fails", func() {
		testutil.FailInserts(s.T(), s.deps.DB, "webauthn_credentials")
		_, err := finish(wireOrigin)
		requireServiceUnavailableError(s.T(), err)
		s.Zero(s.credentialCount(user.ID))
	})
}

// An email login begin that cannot read the account or its credentials fails
// instead of answering as an address that cannot sign in.
func (s *AuthHandlerIntegrationSuite) TestPasskeyEmailLoginBeginFailures() {
	email := "begin-split@example.com"
	s.signUpWithPasskey(newPasskeyWireAPI(s.T(), s.newRealPasskeyHandler()), email)
	jwt := &testhelpers.MockJWTService{}
	unavailable := testutil.UnavailableDB(s.T())

	handlers := map[string]*PasskeyHandler{
		"account lookup fails": NewPasskeyHandler(newTestWebAuthnService(s.T(), s.deps.DB), jwt,
			usersvc.NewUserService(unavailable), &testhelpers.MockEmailService{}, testConfig()),
		"credential load fails": NewPasskeyHandler(newTestWebAuthnService(s.T(), unavailable), jwt,
			s.deps.UserService, &testhelpers.MockEmailService{}, testConfig()),
	}
	for name, h := range handlers {
		s.Run(name, func() {
			s.requireServiceUnavailable(newPasskeyWireAPI(s.T(), h).Post("/auth/passkey/login/begin", map[string]any{"email": email}))
		})
	}
}

// An address no account can hold answers as an unknown address, not as a
// failed lookup.
func (s *AuthHandlerIntegrationSuite) TestPasskeyEmailLoginBeginRefusesUnstorableAddress() {
	api := newPasskeyWireAPI(s.T(), s.newRealPasskeyHandler())

	unknown := api.Post("/auth/passkey/login/begin", map[string]any{"email": "no-account@example.com"})
	unstorable := api.Post("/auth/passkey/login/begin", map[string]any{"email": "no\x00account@example.com"})

	requireIdenticalResponses(s.T(), unknown, unstorable)
}

// A signup is refused for a registration the relying party does not accept,
// and fails, creating no account and no session, when the challenge cannot
// be read or a row cannot be written.
func (s *AuthHandlerIntegrationSuite) TestPasskeySignupFinishRefusalAndFailures() {
	api := newPasskeyWireAPI(s.T(), s.newRealPasskeyHandler())

	s.Run("registration from another origin is refused", func() {
		email := "signup-elsewhere@example.com"
		begin := api.Post("/auth/passkey/signup/begin", signupBeginBody(email))
		requireWireSuccess(s.T(), begin)
		challengeID, _ := decodeWireBody(s.T(), begin.Body.Bytes())["challenge_id"].(string)
		registration, err := softauthn.New(elsewhereOrigin).Register(s.creationOptions(begin.Body.Bytes()))
		s.Require().NoError(err)

		refused := api.Post("/auth/passkey/signup/finish", signupFinishBody(challengeID, json.RawMessage(registration)))
		s.Equal(http.StatusOK, refused.Code, refused.Body.String())
		body := decodeWireBody(s.T(), refused.Body.Bytes())
		s.Equal(autherrors.CodeValidationFailed, body["error_code"])
		s.Equal("Invalid credential response", body["message"])
		s.Empty(refused.Header().Get("Set-Cookie"))
		s.requireNoAccount(email)
	})

	s.Run("credential id another account holds is refused", func() {
		device := s.signUpWithPasskey(api, "signup-held-holder@example.com")
		email := "signup-held@example.com"
		begin := api.Post("/auth/passkey/signup/begin", signupBeginBody(email))
		requireWireSuccess(s.T(), begin)
		challengeID, _ := decodeWireBody(s.T(), begin.Body.Bytes())["challenge_id"].(string)
		registration, err := device.RegisterAgain(s.creationOptions(begin.Body.Bytes()))
		s.Require().NoError(err)
		var usersBefore int64
		s.Require().NoError(s.deps.DB.Model(&authm.User{}).Count(&usersBefore).Error)

		refused := api.Post("/auth/passkey/signup/finish", signupFinishBody(challengeID, json.RawMessage(registration)))

		s.Equal(http.StatusOK, refused.Code, refused.Body.String())
		body := decodeWireBody(s.T(), refused.Body.Bytes())
		s.Equal(false, body["success"])
		s.Equal(autherrors.CodeValidationFailed, body["error_code"])
		s.Equal("Invalid credential response", body["message"])
		s.Empty(refused.Header().Get("Set-Cookie"))
		var usersAfter int64
		s.Require().NoError(s.deps.DB.Model(&authm.User{}).Count(&usersAfter).Error)
		s.Equal(usersBefore, usersAfter, "a refused signup creates no account")
		s.requireNoAccount(email)
	})

	s.Run("account that cannot be created fails", func() {
		email := "signup-insert-fails@example.com"
		challengeID, registration, _ := s.beginPasskeySignup(api, email)
		testutil.FailInserts(s.T(), s.deps.DB, "user_preferences")

		s.requireServiceUnavailable(api.Post("/auth/passkey/signup/finish", signupFinishBody(challengeID, registration)))
		s.requireNoAccount(email)
	})

	s.Run("challenge that cannot be read fails", func() {
		email := "signup-consume-fails@example.com"
		challengeID, registration, _ := s.beginPasskeySignup(api, email)
		unreadable := newPasskeyWireAPI(s.T(), s.newPasskeyHandlerOver(newTestWebAuthnService(s.T(), testutil.UnavailableDB(s.T()))))

		s.requireServiceUnavailable(unreadable.Post("/auth/passkey/signup/finish", signupFinishBody(challengeID, registration)))
		s.requireNoAccount(email)
		requireWireSuccess(s.T(), api.Post("/auth/passkey/signup/finish", signupFinishBody(challengeID, registration)))
	})
}
