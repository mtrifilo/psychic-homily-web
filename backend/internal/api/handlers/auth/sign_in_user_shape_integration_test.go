package auth

import (
	"encoding/json"
	"maps"
	"net/http/httptest"
	"slices"

	"github.com/danielgtaylor/huma/v2/humatest"

	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/testutil/softauthn"
)

// signedInUser returns the user object of a successful sign-in response.
func (s *AuthHandlerIntegrationSuite) signedInUser(resp *httptest.ResponseRecorder) map[string]any {
	requireWireSuccess(s.T(), resp)
	user, ok := decodeWireBody(s.T(), resp.Body.Bytes())["user"].(map[string]any)
	s.Require().True(ok, "sign-in response carries no user object: %s", resp.Body.String())
	return user
}

// passkeySignIn runs a passkey login with device, begun with beginBody, and
// returns the user object the finish answers with.
func (s *AuthHandlerIntegrationSuite) passkeySignIn(api humatest.TestAPI, device *softauthn.Authenticator, beginBody map[string]any) map[string]any {
	challengeID, options := s.beginPasskeyLogin(api, beginBody)
	assertion, err := device.Assert(options)
	s.Require().NoError(err)
	return s.signedInUser(api.Post("/auth/passkey/login/finish", map[string]any{
		"challenge_id": challengeID,
		"response":     json.RawMessage(assertion),
	}))
}

// The password login, the email-specific passkey login and the discoverable
// passkey login answer with the same account in one shape: the same user
// keys, carrying the account's OAuth accounts and preferences.
func (s *AuthHandlerIntegrationSuite) TestSignInPathsReturnTheUserInOneShape() {
	email, password := "one-shape@test.com", "strong-password-123!"
	user := s.createUserWithPassword(email, password)
	s.Require().NoError(s.deps.DB.Create(&authm.OAuthAccount{
		UserID: user.ID, Provider: "google", ProviderUserID: "one-shape-subject",
	}).Error)

	webauthnService := newTestWebAuthnService(s.T(), s.deps.DB)
	device := registerSoftPasskey(s.T(), webauthnService, user)
	passkeyAPI := newPasskeyWireAPI(s.T(), s.newPasskeyHandlerOver(webauthnService))
	passwordAPI := newWireAPI(s.T(), s.newAuthHandler(false))

	reference := s.signedInUser(passwordAPI.Post("/auth/login", map[string]any{"email": email, "password": password}))
	s.Require().Contains(reference, "oauth_accounts", "the password sign-in is the reference shape")
	s.Require().Contains(reference, "preferences", "the password sign-in is the reference shape")

	passkeyPaths := []struct {
		name string
		user map[string]any
	}{
		{"email passkey", s.passkeySignIn(passkeyAPI, device, map[string]any{"email": email})},
		{"discoverable passkey", s.passkeySignIn(passkeyAPI, device, map[string]any{})},
	}
	for _, path := range passkeyPaths {
		s.Equal(slices.Sorted(maps.Keys(reference)), slices.Sorted(maps.Keys(path.user)), "%s: user keys", path.name)
		s.Equal(reference["id"], path.user["id"], "%s: account", path.name)
		s.Equal(reference["oauth_accounts"], path.user["oauth_accounts"], "%s: OAuth accounts", path.name)
		s.Equal(reference["preferences"], path.user["preferences"], "%s: preferences", path.name)
	}
}
