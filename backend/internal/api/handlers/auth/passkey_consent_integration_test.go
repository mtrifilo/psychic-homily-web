package auth

import (
	"encoding/base64"
	"encoding/json"

	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/go-webauthn/webauthn/protocol"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/legal"
	authm "psychic-homily-backend/internal/models/auth"
	authsvc "psychic-homily-backend/internal/services/auth"
	"psychic-homily-backend/internal/testutil/softauthn"
)

// These run the passkey endpoints over the real user and WebAuthn services
// and the suite's database, with a software authenticator for the ceremonies.

func (s *AuthHandlerIntegrationSuite) newRealPasskeyHandler() *PasskeyHandler {
	return s.newPasskeyHandlerOver(s.newRealWebAuthnService())
}

func (s *AuthHandlerIntegrationSuite) newRealWebAuthnService() *authsvc.WebAuthnService {
	webauthnService, err := authsvc.NewWebAuthnService(s.deps.DB, &config.Config{
		WebAuthn: config.WebAuthnConfig{RPDisplayName: "Test", RPID: "localhost", RPOrigins: []string{wireOrigin}},
	})
	s.Require().NoError(err)
	return webauthnService
}

func (s *AuthHandlerIntegrationSuite) newPasskeyHandlerOver(webauthnService *authsvc.WebAuthnService) *PasskeyHandler {
	jwt := &testhelpers.MockJWTService{CreateTokenFn: func(*authm.User) (string, error) { return "token", nil }}
	return NewPasskeyHandler(webauthnService, jwt, s.deps.UserService, &testhelpers.MockEmailService{}, testConfig())
}

// creationOptions decodes a signup/begin response's options into what the
// software authenticator answers; the wire carries user.id as base64url.
func (s *AuthHandlerIntegrationSuite) creationOptions(raw []byte) *protocol.CredentialCreation {
	var body struct {
		Options json.RawMessage `json:"options"`
	}
	s.Require().NoError(json.Unmarshal(raw, &body))
	var creation protocol.CredentialCreation
	s.Require().NoError(json.Unmarshal(body.Options, &creation))
	encoded, ok := creation.Response.User.ID.(string)
	s.Require().True(ok, "user.id type %T", creation.Response.User.ID)
	handle, err := base64.RawURLEncoding.DecodeString(encoded)
	s.Require().NoError(err)
	creation.Response.User.ID = protocol.URLEncodedBase64(handle)
	return &creation
}

// beginPasskeySignup runs signup/begin for email with the current documents
// accepted and returns the challenge id, the registration a fresh software
// authenticator answers it with, and that authenticator.
func (s *AuthHandlerIntegrationSuite) beginPasskeySignup(api humatest.TestAPI, email string) (string, json.RawMessage, *softauthn.Authenticator) {
	begin := api.Post("/auth/passkey/signup/begin", map[string]any{
		"email":            email,
		"terms_accepted":   true,
		"terms_version":    legal.CurrentTermsVersion,
		"privacy_version":  legal.CurrentPrivacyVersion,
		"age_confirmed":    true,
		"min_age_attested": MinSignupAge,
	})
	requireWireSuccess(s.T(), begin)
	challengeID, _ := decodeWireBody(s.T(), begin.Body.Bytes())["challenge_id"].(string)
	s.Require().NotEmpty(challengeID)

	device := softauthn.New(wireOrigin)
	registration, err := device.Register(s.creationOptions(begin.Body.Bytes()))
	s.Require().NoError(err)
	return challengeID, registration, device
}

// The account records the consent signup/begin validated, not the consent
// fields the finish request carries.
func (s *AuthHandlerIntegrationSuite) TestPasskeySignupRecordsTheConsentBeginValidated() {
	api := newPasskeyWireAPI(s.T(), s.newRealPasskeyHandler())
	email := "consent-carried@example.com"

	challengeID, registration, _ := s.beginPasskeySignup(api, email)

	finish := api.Post("/auth/passkey/signup/finish", map[string]any{
		"challenge_id":     challengeID,
		"response":         json.RawMessage(registration),
		"terms_accepted":   false,
		"terms_version":    "2025-01-01",
		"privacy_version":  "",
		"age_confirmed":    false,
		"min_age_attested": 10,
	})
	requireWireSuccess(s.T(), finish)

	var stored authm.User
	s.Require().NoError(s.deps.DB.Where("email = ?", email).First(&stored).Error)
	s.Require().NotNil(stored.TermsVersion)
	s.Require().NotNil(stored.PrivacyVersion)
	s.Require().NotNil(stored.MinAgeAttested)
	s.Equal(legal.CurrentTermsVersion, *stored.TermsVersion)
	s.Equal(legal.CurrentPrivacyVersion, *stored.PrivacyVersion)
	s.Equal(MinSignupAge, *stored.MinAgeAttested)
	s.NotNil(stored.TermsAcceptedAt)
	s.NotNil(stored.AgeConfirmedAt)
}

// An address with no account and an account with no passkey answer the same
// bytes from the email-specific login begin.
func (s *AuthHandlerIntegrationSuite) TestPasskeyEmailLoginBeginAnswersUnknownAndPasskeylessAlike() {
	known := "no-passkey@example.com"
	s.Require().NoError(s.deps.DB.Create(&authm.User{Email: &known, IsActive: true}).Error)
	api := newPasskeyWireAPI(s.T(), s.newRealPasskeyHandler())

	unknown := api.Post("/auth/passkey/login/begin", map[string]any{"email": "no-account@example.com"})
	passkeyless := api.Post("/auth/passkey/login/begin", map[string]any{"email": known})

	requireIdenticalResponses(s.T(), unknown, passkeyless)
	s.NotEmpty(unknown.Body.Bytes(), "the refusal has a body")
}
