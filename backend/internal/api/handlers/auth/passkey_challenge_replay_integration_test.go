package auth

import (
	"encoding/json"
	"net/http/httptest"

	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/go-webauthn/webauthn/protocol"

	autherrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/testutil/softauthn"
)

// These replay a finish request with a challenge an earlier finish already
// received, over the real WebAuthn service and the suite's database.

// signUpWithPasskey creates an account for email through signup begin and
// finish and returns the authenticator holding its passkey and the finish
// request body.
func (s *AuthHandlerIntegrationSuite) signUpWithPasskey(api humatest.TestAPI, email string) (*softauthn.Authenticator, map[string]any) {
	challengeID, registration, device := s.beginPasskeySignup(api, email)
	finish := map[string]any{
		"challenge_id":   challengeID,
		"response":       registration,
		"terms_accepted": true,
		"age_confirmed":  true,
	}
	requireWireSuccess(s.T(), api.Post("/auth/passkey/signup/finish", finish))
	return device, finish
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

func (s *AuthHandlerIntegrationSuite) TestPasskeySignupFinishReplayIsRefused() {
	api := newPasskeyWireAPI(s.T(), s.newRealPasskeyHandler())
	_, finish := s.signUpWithPasskey(api, "signup-replay@example.com")

	s.requireSpentChallengeRefusal(api.Post("/auth/passkey/signup/finish", finish))
}

func (s *AuthHandlerIntegrationSuite) TestPasskeyLoginFinishReplayIsRefused() {
	api := newPasskeyWireAPI(s.T(), s.newRealPasskeyHandler())
	device, _ := s.signUpWithPasskey(api, "login-replay@example.com")

	challengeID, options := s.beginDiscoverableLogin(api)
	assertion, err := device.Assert(options)
	s.Require().NoError(err)
	finish := map[string]any{"challenge_id": challengeID, "response": json.RawMessage(assertion)}

	requireWireSuccess(s.T(), api.Post("/auth/passkey/login/finish", finish))
	s.requireSpentChallengeRefusal(api.Post("/auth/passkey/login/finish", finish))
}

// A finish whose assertion is refused still spends the challenge, so the
// assertion the challenge was issued for is refused afterwards too.
func (s *AuthHandlerIntegrationSuite) TestPasskeyLoginRefusedAssertionSpendsChallenge() {
	api := newPasskeyWireAPI(s.T(), s.newRealPasskeyHandler())
	device, _ := s.signUpWithPasskey(api, "login-refused@example.com")

	challengeID, options := s.beginDiscoverableLogin(api)
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
