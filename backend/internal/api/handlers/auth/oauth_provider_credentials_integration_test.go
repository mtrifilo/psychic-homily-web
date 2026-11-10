package auth

import (
	"time"

	"github.com/markbates/goth"

	authm "psychic-homily-backend/internal/models/auth"
)

// The provider hands the callback an access token, a refresh token and their
// expiry with every identity. A sign-in, a returning sign-in and a Settings
// link each store none of the three.

// withProviderCredentials is identity as a real provider's callback returns
// it: carrying tokens and an expiry beside the profile.
func withProviderCredentials(identity goth.User) goth.User {
	identity.AccessToken = "callback-access-token"
	identity.RefreshToken = "callback-refresh-token"
	identity.ExpiresAt = time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	return identity
}

// requireNoStoredProviderCredential asserts that no oauth_accounts row for
// subject holds a provider credential, read by column name so a model change
// cannot hide a stored value, and that the row exists.
func (s *OAuthHandlerIntegrationSuite) requireNoStoredProviderCredential(subject string) {
	s.T().Helper()
	var rows, holding int64
	s.Require().NoError(s.deps.DB.Model(&authm.OAuthAccount{}).Where("provider_user_id = ?", subject).Count(&rows).Error)
	s.Require().Equal(int64(1), rows, "the identity was attached")
	s.Require().NoError(s.deps.DB.Raw(
		`SELECT count(*) FROM oauth_accounts
		  WHERE provider_user_id = ?
		    AND (access_token IS NOT NULL OR refresh_token IS NOT NULL OR expires_at IS NOT NULL)`,
		subject).Scan(&holding).Error)
	s.Zero(holding, "a provider credential is stored for %s", subject)
}

func (s *OAuthHandlerIntegrationSuite) TestCallback_NewAccountStoresNoProviderCredential() {
	identity := withProviderCredentials(s.stepIdentity("credentials"))
	token := s.startStep(identity)

	resp := s.stepAPI().Post(OAuthFinishSignupRoute, pendingCookieHeader(token), validStepBody(dateOfBirthAged(30)))
	s.Equal(true, s.decode(resp)["success"], resp.Body.String())

	s.requireNoStoredProviderCredential(identity.UserID)
}

func (s *OAuthHandlerIntegrationSuite) TestCallback_ReturningSignInStoresNoProviderCredential() {
	identity := goth.User{
		Provider: "google",
		UserID:   "google-credentials-returning",
		Email:    "credentials.returning@test.com",
		RawData:  map[string]any{"verified_email": true},
	}
	s.signUp(identity)

	w, req := oauthCallbackRequest("google")
	s.newHandler(&mockOAuthCompleter{user: withProviderCredentials(identity)}).OAuthCallbackHTTPHandler(w, req)
	s.assertSignedIn(w)

	s.requireNoStoredProviderCredential(identity.UserID)
}

func (s *OAuthHandlerIntegrationSuite) TestLinkCallback_StoresNoProviderCredential() {
	user := s.createLinkUser("credentials-link@test.com")
	handler := s.newHandler(&mockOAuthCompleter{user: withProviderCredentials(goth.User{
		Provider: "google",
		UserID:   "google-credentials-link",
		Email:    "credentials-link@test.com",
	})})

	w, req := oauthCallbackRequest("google")
	s.armLinkIntentOn(req, user.ID, "google")
	s.addSessionCookie(req, user)
	handler.OAuthCallbackHTTPHandler(w, req)
	s.Equal("connected", s.parseLinkRedirect(w.Header().Get("Location")).Get(oauthLinkResultParam))

	s.requireNoStoredProviderCredential("google-credentials-link")
}
