package auth

import (
	"encoding/json"
	"net/http/httptest"
	"time"

	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/go-webauthn/webauthn/protocol"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	autherrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	authsvc "psychic-homily-backend/internal/services/auth"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil/softauthn"
)

// These present a challenge again after a finish request spent it, over the
// real WebAuthn service and the suite's database. The refused-finish cases
// send the response the challenge was issued for second, so they pass only
// when the refused first finish already spent the challenge.

// malformedCredential is a finish response no WebAuthn parser accepts.
var malformedCredential = json.RawMessage(`{"id":"not-a-credential","type":"public-key"}`)

// signUpWithPasskey creates an account for email through signup begin and
// finish and returns the authenticator holding its passkey.
func (s *AuthHandlerIntegrationSuite) signUpWithPasskey(api humatest.TestAPI, email string) *softauthn.Authenticator {
	challengeID, registration, device := s.beginPasskeySignup(api, email)
	requireWireSuccess(s.T(), api.Post("/auth/passkey/signup/finish", signupFinishBody(challengeID, registration)))
	return device
}

// signupFinishBody is a signup/finish request for challengeID carrying
// response.
func signupFinishBody(challengeID string, response json.RawMessage) map[string]any {
	return map[string]any{
		"challenge_id":   challengeID,
		"response":       response,
		"terms_accepted": true,
	}
}

// beginDiscoverableLogin starts a discoverable login and returns its
// challenge id and the options the authenticator answers.
func (s *AuthHandlerIntegrationSuite) beginDiscoverableLogin(api humatest.TestAPI) (string, *protocol.CredentialAssertion) {
	begin := api.Post("/auth/passkey/login/begin", map[string]any{})
	requireWireSuccess(s.T(), begin)
	var body struct {
		ChallengeID string                        `json:"challenge_id"`
		Options     *protocol.CredentialAssertion `json:"options"`
	}
	s.Require().NoError(json.Unmarshal(begin.Body.Bytes(), &body))
	s.Require().NotEmpty(body.ChallengeID)
	s.Require().NotNil(body.Options)
	return body.ChallengeID, body.Options
}

// requireSpentChallengeRefusal asserts the answer a finish gets for a
// challenge that no longer exists.
func (s *AuthHandlerIntegrationSuite) requireSpentChallengeRefusal(resp *httptest.ResponseRecorder) {
	s.Equal(200, resp.Code, resp.Body.String())
	body := decodeWireBody(s.T(), resp.Body.Bytes())
	s.Equal(false, body["success"])
	s.Equal(autherrors.CodeValidationFailed, body["error_code"])
	s.Equal("Invalid or expired challenge", body["message"])
}

// A successful finish spends its challenge, so the same request sent again
// is refused.
func (s *AuthHandlerIntegrationSuite) TestPasskeyLoginFinishReplayIsRefused() {
	api := newPasskeyWireAPI(s.T(), s.newRealPasskeyHandler())
	device := s.signUpWithPasskey(api, "login-replay@example.com")

	challengeID, options := s.beginDiscoverableLogin(api)
	assertion, err := device.Assert(options)
	s.Require().NoError(err)
	finish := map[string]any{"challenge_id": challengeID, "response": json.RawMessage(assertion)}

	requireWireSuccess(s.T(), api.Post("/auth/passkey/login/finish", finish))
	s.requireSpentChallengeRefusal(api.Post("/auth/passkey/login/finish", finish))
}

func (s *AuthHandlerIntegrationSuite) TestPasskeyLoginRefusedAssertionSpendsChallenge() {
	api := newPasskeyWireAPI(s.T(), s.newRealPasskeyHandler())
	device := s.signUpWithPasskey(api, "login-refused@example.com")

	challengeID, options := s.beginDiscoverableLogin(api)
	// softAssertion signs a challenge of its own, so this assertion is
	// refused for answering the wrong challenge.
	refused := api.Post("/auth/passkey/login/finish", map[string]any{
		"challenge_id": challengeID,
		"response":     softAssertion(s.T(), device),
	})
	s.Equal(200, refused.Code, refused.Body.String())
	s.Equal(autherrors.CodeInvalidCredentials, decodeWireBody(s.T(), refused.Body.Bytes())["error_code"])

	assertion, err := device.Assert(options)
	s.Require().NoError(err)
	s.requireSpentChallengeRefusal(api.Post("/auth/passkey/login/finish", map[string]any{
		"challenge_id": challengeID,
		"response":     json.RawMessage(assertion),
	}))
}

func (s *AuthHandlerIntegrationSuite) TestPasskeySignupRefusedFinishSpendsChallenge() {
	api := newPasskeyWireAPI(s.T(), s.newRealPasskeyHandler())
	email := "signup-refused@example.com"
	challengeID, registration, _ := s.beginPasskeySignup(api, email)

	refused := api.Post("/auth/passkey/signup/finish", signupFinishBody(challengeID, malformedCredential))
	s.Equal(200, refused.Code, refused.Body.String())
	s.Equal("Invalid credential response", decodeWireBody(s.T(), refused.Body.Bytes())["message"])

	s.requireSpentChallengeRefusal(api.Post("/auth/passkey/signup/finish", signupFinishBody(challengeID, registration)))
	var accounts int64
	s.Require().NoError(s.deps.DB.Model(&authm.User{}).Where("email = ?", email).Count(&accounts).Error)
	s.Zero(accounts)
}

func (s *AuthHandlerIntegrationSuite) TestPasskeyRegisterRefusedFinishSpendsChallenge() {
	email := "register-refused@example.com"
	user := &authm.User{Email: &email, IsActive: true}
	s.Require().NoError(s.deps.DB.Create(user).Error)
	h := s.newRealPasskeyHandler()
	ctx := testhelpers.CtxWithSessionAuthTime(user, time.Now())

	begin, err := h.BeginRegisterHandler(ctx, &BeginRegisterRequest{})
	s.Require().NoError(err)
	s.Require().True(begin.Body.Success, begin.Body.Message)
	registration, err := softauthn.New(wireOrigin).Register(begin.Body.Options)
	s.Require().NoError(err)

	refusedInput := &FinishRegisterRequest{}
	refusedInput.Body.ChallengeID = begin.Body.ChallengeID
	refusedInput.Body.Response = malformedCredential
	refused, err := h.FinishRegisterHandler(ctx, refusedInput)
	s.Require().NoError(err)
	s.Equal("Invalid credential response", refused.Body.Message)

	retryInput := &FinishRegisterRequest{}
	retryInput.Body.ChallengeID = begin.Body.ChallengeID
	retryInput.Body.Response = registration
	retry, err := h.FinishRegisterHandler(ctx, retryInput)
	s.Require().NoError(err)
	s.False(retry.Body.Success)
	s.Equal(autherrors.CodeValidationFailed, retry.Body.ErrorCode)
	s.Equal("Invalid or expired challenge", retry.Body.Message)
	var credentials int64
	s.Require().NoError(s.deps.DB.Model(&authm.WebAuthnCredential{}).Where("user_id = ?", user.ID).Count(&credentials).Error)
	s.Zero(credentials)

	// The same steps on a challenge no finish has spent register the passkey.
	fresh, err := h.BeginRegisterHandler(ctx, &BeginRegisterRequest{})
	s.Require().NoError(err)
	freshRegistration, err := softauthn.New(wireOrigin).Register(fresh.Body.Options)
	s.Require().NoError(err)
	freshInput := &FinishRegisterRequest{}
	freshInput.Body.ChallengeID = fresh.Body.ChallengeID
	freshInput.Body.Response = freshRegistration
	registered, err := h.FinishRegisterHandler(ctx, freshInput)
	s.Require().NoError(err)
	s.True(registered.Body.Success, registered.Body.Message)
}

// storedSignupChallenge stores a signup challenge carrying signup, as
// signup/begin stores one, and returns its id and the registration a fresh
// software authenticator answers it with.
func (s *AuthHandlerIntegrationSuite) storedSignupChallenge(webauthnService *authsvc.WebAuthnService, signup contracts.SignupChallenge) (string, json.RawMessage) {
	options, session, err := webauthnService.BeginRegistrationForEmail(signup.Email)
	s.Require().NoError(err)
	challengeID, err := webauthnService.StoreSignupChallenge(signup, session, "signup")
	s.Require().NoError(err)
	registration, err := softauthn.New(wireOrigin).Register(options)
	s.Require().NoError(err)
	return challengeID, registration
}

func (s *AuthHandlerIntegrationSuite) requireNoAccount(email string) {
	var accounts int64
	s.Require().NoError(s.deps.DB.Model(&authm.User{}).Where("email = ?", email).Count(&accounts).Error)
	s.Zero(accounts)
}

// A challenge whose consent fails the finish checks is spent by the refusal.
// The consent it carries would be refused again, so the second finish
// answering as a spent challenge shows the first spent it.
func (s *AuthHandlerIntegrationSuite) TestPasskeySignupConsentRefusalSpendsChallenge() {
	webauthnService := newTestWebAuthnService(s.T(), s.deps.DB)
	api := newPasskeyWireAPI(s.T(), s.newPasskeyHandlerOver(webauthnService))

	cases := []struct {
		name     string
		email    string
		alter    func(*contracts.LegalAcceptance)
		wantCode string
	}{
		{
			name:     "terms version no longer current",
			email:    "signup-stale-terms@example.com",
			alter:    func(a *contracts.LegalAcceptance) { a.TermsVersion = "2000-01-01" },
			wantCode: autherrors.CodeTermsAcceptanceRequired,
		},
		{
			name:  "no age confirmation",
			email: "signup-no-age@example.com",
			alter: func(a *contracts.LegalAcceptance) {
				a.AgeConfirmedAt = time.Time{}
				a.MinAgeAttested = 0
			},
			wantCode: autherrors.CodeAgeConfirmationRequired,
		},
		{
			name:     "age attested without a date of birth",
			email:    "signup-no-date@example.com",
			alter:    func(a *contracts.LegalAcceptance) { a.DateOfBirth = nil },
			wantCode: autherrors.CodeAgeConfirmationRequired,
		},
	}
	for _, tc := range cases {
		s.Run(tc.name, func() {
			signup := currentSignupChallenge(tc.email)
			tc.alter(&signup.Acceptance)
			challengeID, registration := s.storedSignupChallenge(webauthnService, signup)

			refused := api.Post("/auth/passkey/signup/finish", signupFinishBody(challengeID, registration))
			s.Equal(200, refused.Code, refused.Body.String())
			s.Equal(tc.wantCode, decodeWireBody(s.T(), refused.Body.Bytes())["error_code"])

			s.requireSpentChallengeRefusal(api.Post("/auth/passkey/signup/finish", signupFinishBody(challengeID, registration)))
			s.requireNoAccount(tc.email)
		})
	}
}

// A finish refused because the address was taken after begin spends the
// challenge: once the address is free again, the same finish is refused as a
// spent challenge instead of creating the account.
func (s *AuthHandlerIntegrationSuite) TestPasskeySignupEmailTakenSpendsChallenge() {
	api := newPasskeyWireAPI(s.T(), s.newRealPasskeyHandler())
	email := "signup-taken@example.com"
	challengeID, registration, _ := s.beginPasskeySignup(api, email)

	taken := &authm.User{Email: &email, IsActive: true}
	s.Require().NoError(s.deps.DB.Create(taken).Error)
	refused := api.Post("/auth/passkey/signup/finish", signupFinishBody(challengeID, registration))
	s.Equal(200, refused.Code, refused.Body.String())
	s.Equal(autherrors.CodeUserExists, decodeWireBody(s.T(), refused.Body.Bytes())["error_code"])

	s.Require().NoError(s.deps.DB.Delete(taken).Error)
	s.requireSpentChallengeRefusal(api.Post("/auth/passkey/signup/finish", signupFinishBody(challengeID, registration)))
	s.requireNoAccount(email)
}

// A registration challenge is spent only by the user it was issued to:
// another signed-in user's finish is refused and leaves it for its owner.
func (s *AuthHandlerIntegrationSuite) TestPasskeyRegisterFinishByAnotherUserLeavesChallenge() {
	ownerEmail, otherEmail := "register-owner@example.com", "register-other@example.com"
	owner := &authm.User{Email: &ownerEmail, IsActive: true}
	other := &authm.User{Email: &otherEmail, IsActive: true}
	s.Require().NoError(s.deps.DB.Create(owner).Error)
	s.Require().NoError(s.deps.DB.Create(other).Error)
	h := s.newRealPasskeyHandler()
	ownerCtx := testhelpers.CtxWithSessionAuthTime(owner, time.Now())
	otherCtx := testhelpers.CtxWithSessionAuthTime(other, time.Now())

	begin, err := h.BeginRegisterHandler(ownerCtx, &BeginRegisterRequest{})
	s.Require().NoError(err)
	s.Require().True(begin.Body.Success, begin.Body.Message)
	registration, err := softauthn.New(wireOrigin).Register(begin.Body.Options)
	s.Require().NoError(err)
	input := &FinishRegisterRequest{}
	input.Body.ChallengeID = begin.Body.ChallengeID
	input.Body.Response = registration

	refused, err := h.FinishRegisterHandler(otherCtx, input)
	s.Require().NoError(err)
	s.False(refused.Body.Success)
	s.Equal(autherrors.CodeUnauthorized, refused.Body.ErrorCode)
	s.Equal("Challenge belongs to different user", refused.Body.Message)

	registered, err := h.FinishRegisterHandler(ownerCtx, input)
	s.Require().NoError(err)
	s.True(registered.Body.Success, registered.Body.Message)

	var credentials []authm.WebAuthnCredential
	s.Require().NoError(s.deps.DB.Find(&credentials).Error)
	s.Require().Len(credentials, 1)
	s.Equal(owner.ID, credentials[0].UserID)
}
