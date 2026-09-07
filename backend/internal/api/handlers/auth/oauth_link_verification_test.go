package auth

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/markbates/goth"

	autherrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
)

// TestCallback_UnverifiedEmailMatch_RedirectsWithRefusalAndNoSession is the
// end-to-end shape of the refusal over HTTP: no auth cookie is set, and the
// person is told why rather than being handed the generic failure, which reads
// as a transient fault and offers nothing to do about it.
func (s *OAuthHandlerIntegrationSuite) TestCallback_UnverifiedEmailMatch_RedirectsWithRefusalAndNoSession() {
	existing := &authm.User{
		Email:         strPtr("callback-unverified@test.com"),
		IsActive:      true,
		EmailVerified: true,
	}
	s.Require().NoError(s.deps.DB.Create(existing).Error)

	handler := s.newHandler(&mockOAuthCompleter{
		user: goth.User{
			Email:    "callback-unverified@test.com",
			Provider: "google",
			UserID:   "google-unverified-callback",
			Name:     "Unverified Caller",
			RawData:  map[string]any{"verified_email": false},
		},
	})

	w, req := oauthCallbackRequest("google")
	s.addSignupConsentCookie(req)
	handler.OAuthCallbackHTTPHandler(w, req)

	s.Equal(http.StatusTemporaryRedirect, w.Code)
	location := w.Header().Get("Location")
	s.True(strings.HasPrefix(location, "http://localhost:3000/auth?error="),
		"expected redirect to the frontend auth page, got %s", location)

	parsed, err := url.Parse(location)
	s.Require().NoError(err)
	s.Equal(autherrors.ToExternalMessage(autherrors.CodeUserExists), parsed.Query().Get("error"))

	for _, c := range w.Result().Cookies() {
		if c.Name == "auth_token" && c.Value != "" {
			s.Fail("a refused link must not issue a session")
		}
	}

	var oauthRows int64
	s.Require().NoError(s.deps.DB.Model(&authm.OAuthAccount{}).
		Where("user_id = ?", existing.ID).Count(&oauthRows).Error)
	s.Equal(int64(0), oauthRows)
}

// TestCallback_VerifiedEmailMatch_LinksAndSetsCookie is the same request with
// the provider vouching for the address.
func (s *OAuthHandlerIntegrationSuite) TestCallback_VerifiedEmailMatch_LinksAndSetsCookie() {
	existing := &authm.User{
		Email:         strPtr("callback-verified@test.com"),
		IsActive:      true,
		EmailVerified: true,
	}
	s.Require().NoError(s.deps.DB.Create(existing).Error)

	handler := s.newHandler(&mockOAuthCompleter{
		user: goth.User{
			Email:    "callback-verified@test.com",
			Provider: "google",
			UserID:   "google-verified-callback",
			Name:     "Verified Caller",
			RawData:  map[string]any{"verified_email": true},
		},
	})

	w, req := oauthCallbackRequest("google")
	s.addSignupConsentCookie(req)
	handler.OAuthCallbackHTTPHandler(w, req)

	s.Equal(http.StatusTemporaryRedirect, w.Code)
	s.Equal("http://localhost:3000", w.Header().Get("Location"))

	sessionIssued := false
	for _, c := range w.Result().Cookies() {
		if c.Name == "auth_token" && c.Value != "" {
			sessionIssued = true
		}
	}
	s.True(sessionIssued, "a verified link must issue a session")

	var oauthRows int64
	s.Require().NoError(s.deps.DB.Model(&authm.OAuthAccount{}).
		Where("user_id = ? AND provider_user_id = ?", existing.ID, "google-verified-callback").
		Count(&oauthRows).Error)
	s.Equal(int64(1), oauthRows)
}

// TestOAuthCallbackErrorIsUserActionable pins the allowlist itself. Anything
// not named here reaches the query string as the generic failure, which is what
// keeps a backend fault out of a URL the browser will render.
func (s *OAuthHandlerIntegrationSuite) TestOAuthCallbackErrorIsUserActionable() {
	actionable := []string{
		autherrors.CodeTermsAcceptanceRequired,
		autherrors.CodeUserExists,
	}
	for _, code := range actionable {
		s.True(oauthCallbackErrorIsUserActionable(code), "expected %s to carry its own copy", code)
	}

	generic := []string{
		autherrors.CodeServiceUnavailable,
		autherrors.CodeInvalidCredentials,
		autherrors.CodeUnknown,
		autherrors.CodeValidationFailed,
		"",
	}
	for _, code := range generic {
		s.False(oauthCallbackErrorIsUserActionable(code), "expected %s to stay generic", code)
	}
}
