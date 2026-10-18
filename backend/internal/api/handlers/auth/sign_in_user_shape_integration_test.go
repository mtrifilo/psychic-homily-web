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

// The password login and the passkey login answer with the same account in
// one shape: the same user keys, carrying the account's OAuth accounts and
// preferences.
func (s *AuthHandlerIntegrationSuite) TestSignInPathsReturnTheUserInOneShape() {
	email, password := "one-shape@test.com", "strong-password-123!"
	user := s.createUserWithPassword(email, password)
	s.Require().NoError(s.deps.DB.Create(&authm.OAuthAccount{
		UserID: user.ID, Provider: "google", ProviderUserID: "one-shape-subject",
	}).Error)

	passkeyHandler := s.newRealPasskeyHandler()
	device := softauthn.New(wireOrigin)
	registered := s.registerPasskey(passkeyHandler, user, device.Register)
	s.Require().True(registered.Body.Success, registered.Body.Message)
	passkeyAPI := newPasskeyWireAPI(s.T(), passkeyHandler)
	passwordAPI := newWireAPI(s.T(), s.newAuthHandler(false))

	reference := s.signedInUser(passwordAPI.Post("/auth/login", map[string]any{"email": email, "password": password}))
	s.Require().Contains(reference, "oauth_accounts", "the password sign-in is the reference shape")
	s.Require().Contains(reference, "preferences", "the password sign-in is the reference shape")

	passkey := s.passkeySignIn(passkeyAPI, device, map[string]any{})
	s.Equal(slices.Sorted(maps.Keys(reference)), slices.Sorted(maps.Keys(passkey)), "user keys")
	s.Equal(reference["id"], passkey["id"], "account")
	s.Equal(reference["oauth_accounts"], passkey["oauth_accounts"], "OAuth accounts")
	s.Equal(reference["preferences"], passkey["preferences"], "preferences")
}
