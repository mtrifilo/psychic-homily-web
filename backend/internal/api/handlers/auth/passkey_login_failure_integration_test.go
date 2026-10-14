package auth

import (
	"encoding/json"
	"net/http"

	autherrors "psychic-homily-backend/internal/errors"
)

// A sign-in whose assertion is valid but whose credential update fails is a
// failure, not a refusal: it answers a 5xx, never INVALID_CREDENTIALS, and
// mints no session.
func (s *AuthHandlerIntegrationSuite) TestPasskeyLoginUpdateFailureAnswersServiceUnavailable() {
	api := newPasskeyWireAPI(s.T(), s.newRealPasskeyHandler())
	device := s.signUpWithPasskey(api, "login-update-fails@example.com")

	s.Require().NoError(s.deps.DB.Exec(`CREATE OR REPLACE FUNCTION refuse_credential_update() RETURNS trigger AS $$
		BEGIN RAISE EXCEPTION 'credential update refused by test trigger'; END $$ LANGUAGE plpgsql`).Error)
	s.Require().NoError(s.deps.DB.Exec(`CREATE TRIGGER refuse_credential_update BEFORE UPDATE ON webauthn_credentials
		FOR EACH ROW EXECUTE FUNCTION refuse_credential_update()`).Error)
	defer func() {
		s.Require().NoError(s.deps.DB.Exec(`DROP TRIGGER refuse_credential_update ON webauthn_credentials`).Error)
		s.Require().NoError(s.deps.DB.Exec(`DROP FUNCTION refuse_credential_update()`).Error)
	}()

	challengeID, options := s.beginDiscoverableLogin(api)
	assertion, err := device.Assert(options)
	s.Require().NoError(err)
	resp := api.Post("/auth/passkey/login/finish", map[string]any{
		"challenge_id": challengeID,
		"response":     json.RawMessage(assertion),
	})

	s.Equal(http.StatusServiceUnavailable, resp.Code, resp.Body.String())
	body := decodeWireBody(s.T(), resp.Body.Bytes())
	s.Equal(autherrors.CodeServiceUnavailable, body["error_code"])
	s.Equal(false, body["success"])
	s.Empty(resp.Header().Get("Set-Cookie"))
	assertNoInternalText(s.T(), resp.Body.String(), "test trigger", "failed to update credential")
}
