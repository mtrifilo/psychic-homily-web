package auth

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/markbates/goth"

	autherrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
)

// The refusal over HTTP: no auth cookie, and the refusal's own copy in the
// redirect rather than the generic failure.
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
	// UserMessage() is what the handler emits. ToExternalMessage is a separate
	// table that happens to agree for this code, so asserting against it would
	// point a maintainer at the wrong function.
	s.Equal(autherrors.ErrOAuthLinkRefused("callback-unverified@test.com").UserMessage(), parsed.Query().Get("error"))
	// The remediation has to survive the trip into the URL, or the refusal
	// tells a user nothing they can act on.
	s.Contains(parsed.Query().Get("error"), "Settings")

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

// The same request with the provider vouching for the address.
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

// The allowlist itself. A code absent from it is reported generically on all
// four of the surfaces the predicate governs.
func (s *OAuthHandlerIntegrationSuite) TestAuthRefusalCarriesItsOwnCopy() {
	actionable := []string{
		autherrors.CodeTermsAcceptanceRequired,
		autherrors.CodeUserExists,
		autherrors.CodeOAuthLinkRefused,
		autherrors.CodeOAuthIdentityInUse,
		autherrors.CodeOAuthProviderAlreadyLinked,
		autherrors.CodeOAuthLinkExpired,
	}
	for _, code := range actionable {
		s.True(authRefusalCarriesItsOwnCopy(code), "expected %s to carry its own copy", code)
	}

	generic := []string{
		autherrors.CodeServiceUnavailable,
		autherrors.CodeInvalidCredentials,
		autherrors.CodeUnknown,
		autherrors.CodeValidationFailed,
		"",
	}
	for _, code := range generic {
		s.False(authRefusalCarriesItsOwnCopy(code), "expected %s to stay generic", code)
	}
}
