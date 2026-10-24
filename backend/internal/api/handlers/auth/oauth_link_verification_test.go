package auth

import (
	"net/http"
	"net/http/httptest"
	"net/url"

	"github.com/markbates/goth"

	autherrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
)

// assertSignInRefusal pins the shape of a refused OAuth sign-in and returns the
// redirect's query: back to the auth page, carrying the sign-in refusal's own
// copy, with no session, and without the refused address. Parsed rather than
// prefix-matched, because the query is encoded from a map and parameter order
// is not part of the contract.
//
// refused is the address the caller presented. The message is a constant and
// does not interpolate it, which is the property the last assertion holds in
// place: this redirect becomes a browser URL, so an address interpolated into
// the copy would land in history, referrers and access logs.
func (s *OAuthHandlerIntegrationSuite) assertSignInRefusal(w *httptest.ResponseRecorder, refused string) url.Values {
	s.T().Helper()
	query := s.assertAuthPageRefusal(w, autherrors.ErrOAuthLinkRefused(refused))

	s.NotContains(query.Get("error"), refused)
	s.NotContains(w.Header().Get("Location"), refused)
	return query
}

// assertAuthPageRefusal pins a sign-in callback refused with refusal: back to
// the auth page carrying exactly the refusal's own copy and code, with no
// session. It returns the redirect's query.
func (s *OAuthHandlerIntegrationSuite) assertAuthPageRefusal(w *httptest.ResponseRecorder, refusal *autherrors.AuthError) url.Values {
	s.T().Helper()
	s.Equal(http.StatusTemporaryRedirect, w.Code)

	parsed, err := url.Parse(w.Header().Get("Location"))
	s.Require().NoError(err)
	s.Equal("http://localhost:3000/auth", parsed.Scheme+"://"+parsed.Host+parsed.Path)
	// UserMessage() is what the handler emits. ToExternalMessage is a separate
	// table that happens to agree for these codes, so asserting against it
	// would point a maintainer at the wrong function.
	s.Equal(url.Values{
		"error":      {refusal.UserMessage()},
		"error_code": {refusal.Code},
	}, parsed.Query())

	s.assertNoSessionCookie(w)
	return parsed.Query()
}

func (s *OAuthHandlerIntegrationSuite) assertNoSessionCookie(w *httptest.ResponseRecorder) {
	s.T().Helper()
	for _, c := range w.Result().Cookies() {
		if c.Name == "auth_token" && c.Value != "" {
			s.Fail("a refused sign-in must not issue a session")
		}
	}
}

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
	handler.OAuthCallbackHTTPHandler(w, req)

	query := s.assertSignInRefusal(w, "callback-unverified@test.com")
	// The remediation has to survive the trip into the URL, or the refusal
	// tells a user nothing they can act on.
	s.Contains(query.Get("error"), "Settings")

	var oauthRows int64
	s.Require().NoError(s.deps.DB.Model(&authm.OAuthAccount{}).
		Where("user_id = ?", existing.ID).Count(&oauthRows).Error)
	s.Equal(int64(0), oauthRows)
}

// The same request with the provider vouching for the address. A vouched
// address is still only evidence about the mailbox, so nothing is attached
// and no session issued; the identity waits as a pending link for a proof of
// the account (oauth_link_step_integration_test.go).
func (s *OAuthHandlerIntegrationSuite) TestCallback_VerifiedEmailMatch_PendsTheLinkWithNoSession() {
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
	handler.OAuthCallbackHTTPHandler(w, req)

	s.Equal(http.StatusTemporaryRedirect, w.Code)
	s.Equal("http://localhost:3000/auth?error_code="+autherrors.CodeOAuthLinkPending, w.Header().Get("Location"))
	s.assertNoSessionCookie(w)

	var oauthRows int64
	s.Require().NoError(s.deps.DB.Model(&authm.OAuthAccount{}).
		Where("user_id = ?", existing.ID).Count(&oauthRows).Error)
	s.Equal(int64(0), oauthRows, "a refused sign-in must attach no identity to the account")
}

// The allowlist itself. A code absent from it is reported generically on all
// four of the surfaces the predicate governs.
func (s *OAuthHandlerIntegrationSuite) TestAuthRefusalCarriesItsOwnCopy() {
	actionable := []string{
		autherrors.CodeTermsAcceptanceRequired,
		autherrors.CodeAgeConfirmationRequired,
		autherrors.CodeUserExists,
		autherrors.CodeOAuthLinkRefused,
		autherrors.CodeOAuthIdentityInUse,
		autherrors.CodeOAuthProviderAlreadyLinked,
		autherrors.CodeOAuthLinkExpired,
		autherrors.CodeAccountInactive,
		autherrors.CodeAccountRecoverable,
		// The refusal of a provider email claim no account can store reaches
		// the caller as a refusal, not as a fault.
		autherrors.CodeValidationFailed,
	}
	for _, code := range actionable {
		s.True(authRefusalCarriesItsOwnCopy(code), "expected %s to carry its own copy", code)
	}

	generic := []string{
		autherrors.CodeServiceUnavailable,
		autherrors.CodeInvalidCredentials,
		autherrors.CodeUnknown,
		"",
	}
	for _, code := range generic {
		s.False(authRefusalCarriesItsOwnCopy(code), "expected %s to stay generic", code)
	}
}
