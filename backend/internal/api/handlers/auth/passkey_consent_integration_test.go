package auth

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/legal"
	authm "psychic-homily-backend/internal/models/auth"
	authsvc "psychic-homily-backend/internal/services/auth"
	usersvc "psychic-homily-backend/internal/services/user"
	"psychic-homily-backend/internal/testutil"
	"psychic-homily-backend/internal/testutil/softauthn"
)

// These run the passkey endpoints over the real user and WebAuthn services
// and a real database, with a software authenticator for the ceremonies.
type PasskeyConsentIntegrationSuite struct {
	suite.Suite
	testDB  *testutil.TestDatabase
	db      *gorm.DB
	handler *PasskeyHandler
}

func TestPasskeyConsentIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(PasskeyConsentIntegrationSuite))
}

func (s *PasskeyConsentIntegrationSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
	webauthnService, err := authsvc.NewWebAuthnService(s.db, &config.Config{
		WebAuthn: config.WebAuthnConfig{RPDisplayName: "Test", RPID: "localhost", RPOrigins: []string{wireOrigin}},
	})
	s.Require().NoError(err)
	jwt := &testhelpers.MockJWTService{CreateTokenFn: func(*authm.User) (string, error) { return "token", nil }}
	s.handler = NewPasskeyHandler(webauthnService, jwt, usersvc.NewUserService(s.db), &testhelpers.MockEmailService{}, testConfig())
}

func (s *PasskeyConsentIntegrationSuite) TearDownTest() {
	testhelpers.CleanupTables(s.db)
}

func (s *PasskeyConsentIntegrationSuite) TearDownSuite() {
	s.testDB.Cleanup()
}

// creationOptions decodes a signup/begin response's options into what the
// software authenticator answers.
func (s *PasskeyConsentIntegrationSuite) creationOptions(raw []byte) *protocol.CredentialCreation {
	var body struct {
		Options     json.RawMessage `json:"options"`
		ChallengeID string          `json:"challenge_id"`
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

// The account records the consent signup/begin validated, not the consent
// fields the finish request carries.
func (s *PasskeyConsentIntegrationSuite) TestSignupRecordsTheConsentBeginValidated() {
	api := newPasskeyWireAPI(s.T(), s.handler)
	email := "consent-carried@example.com"

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

	registration, err := softauthn.New(wireOrigin).Register(s.creationOptions(begin.Body.Bytes()))
	s.Require().NoError(err)

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
	s.Require().NoError(s.db.Where("email = ?", email).First(&stored).Error)
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
func (s *PasskeyConsentIntegrationSuite) TestEmailLoginBeginDoesNotRevealAccounts() {
	known := "no-passkey@example.com"
	s.Require().NoError(s.db.Create(&authm.User{Email: &known, IsActive: true}).Error)
	api := newPasskeyWireAPI(s.T(), s.handler)

	unknown := api.Post("/auth/passkey/login/begin", map[string]any{"email": "no-account@example.com"})
	passkeyless := api.Post("/auth/passkey/login/begin", map[string]any{"email": known})

	requireIdenticalResponses(s.T(), unknown, passkeyless)
	s.NotEmpty(unknown.Body.Bytes(), "the refusal has a body")
}
