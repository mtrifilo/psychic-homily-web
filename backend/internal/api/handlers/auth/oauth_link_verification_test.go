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

// The same request with the provider vouching for the address. A vouched
// address is still only evidence about the mailbox, so the callback refuses it
// on the same terms and by the same route.
func (s *OAuthHandlerIntegrationSuite) TestCallback_VerifiedEmailMatch_RedirectsWithRefusalAndNoSession() {
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
	location := w.Header().Get("Location")
	s.True(strings.HasPrefix(location, "http://localhost:3000/auth?error="),
		"expected redirect to the frontend auth page, got %s", location)

	parsed, err := url.Parse(location)
	s.Require().NoError(err)
	s.Equal(autherrors.ErrOAuthLinkRefused("callback-verified@test.com").UserMessage(), parsed.Query().Get("error"))

	for _, c := range w.Result().Cookies() {
		if c.Name == "auth_token" && c.Value != "" {
			s.Fail("a refused sign-in must not issue a session")
		}
	}

	var oauthRows int64
	s.Require().NoError(s.deps.DB.Model(&authm.OAuthAccount{}).
		Where("user_id = ?", existing.ID).Count(&oauthRows).Error)
	s.Equal(int64(0), oauthRows, "a refused sign-in must attach no identity to the account")

	// The refusal must not fall through to the create path either.
	var users int64
	s.Require().NoError(s.deps.DB.Model(&authm.User{}).
		Where(authm.EmailIdentityWhere, "callback-verified@test.com").Count(&users).Error)
	s.Equal(int64(1), users)
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
