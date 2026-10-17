package auth

import (
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/markbates/goth"

	"psychic-homily-backend/internal/config"
	autherrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/auth"
	usersvc "psychic-homily-backend/internal/services/user"
)

// assertLinkIntentSpent checks that a refused link consumed its intent and
// cleared the intent cookie, so the next callback on this browser is not armed.
func (s *OAuthHandlerIntegrationSuite) assertLinkIntentSpent(w *httptest.ResponseRecorder, intentCookie *http.Cookie) {
	s.T().Helper()
	_, _, stillHeld := peekOAuthLinkIntent(intentCookie.Value)
	s.False(stillHeld, "a refused link must consume its intent")
	s.assertNoSessionCookie(w)
	var intentCleared bool
	for _, c := range w.Result().Cookies() {
		if c.Name == oauthLinkIntentCookieName && c.MaxAge < 0 {
			intentCleared = true
		}
	}
	s.True(intentCleared, "a refused link must clear the intent cookie")
}

func (s *OAuthHandlerIntegrationSuite) createLinkUser(address string) *authm.User {
	s.T().Helper()
	user := &authm.User{Email: strPtr(address), IsActive: true, EmailVerified: true}
	s.Require().NoError(s.deps.DB.Create(user).Error)
	return user
}

// The live session has to name the account the intent names. Every way it can
// fail to is refused as "sign in again", before the provider is contacted and
// without saying anything about any account's state; in particular a closed
// account that is NOT the intent's is a mismatch, not ACCOUNT_INACTIVE.
func (s *OAuthHandlerIntegrationSuite) TestLinkCallback_SessionThatDoesNotNameTheIntentAccountIsRefused() {
	otherKey := auth.NewJWTService(s.deps.DB, &config.Config{JWT: config.JWTConfig{
		SecretKey: "a-different-secret-key-of-32-chars!!",
		Expiry:    24,
	}}, s.deps.UserService)

	cases := []struct {
		name string
		// withSession puts the session (if any) on req for a link armed for
		// owner, and may change account rows to set the case up.
		withSession func(req *http.Request, owner *authm.User)
	}{
		{"no session", func(*http.Request, *authm.User) {}},
		{"an open session for another account", func(req *http.Request, _ *authm.User) {
			s.addSessionCookie(req, s.createLinkUser("gate-other-open@test.com"))
		}},
		{"a closed session for another account", func(req *http.Request, _ *authm.User) {
			other := s.createLinkUser("gate-other-closed@test.com")
			s.addSessionCookie(req, other)
			s.Require().NoError(s.deps.DB.Model(&authm.User{}).
				Where("id = ?", other.ID).Update("is_active", false).Error)
		}},
		{"a session signed with another key", func(req *http.Request, owner *authm.User) {
			token, err := otherKey.CreateToken(owner)
			s.Require().NoError(err)
			req.AddCookie(&http.Cookie{Name: config.AuthCookieName, Value: token})
		}},
	}

	for i, tc := range cases {
		s.Run(tc.name, func() {
			owner := s.createLinkUser(fmt.Sprintf("gate-owner-%d@test.com", i))
			completer := &mockOAuthCompleter{user: goth.User{
				Provider: "google", UserID: fmt.Sprintf("google-gate-subject-%d", i), Email: "gate@test.com",
			}}
			handler := s.newHandler(completer)

			w, req := oauthCallbackRequest("google")
			cookie, _ := s.armLinkIntentOn(req, owner.ID, "google")
			tc.withSession(req, owner)

			handler.OAuthCallbackHTTPHandler(w, req)

			s.assertLinkRefusal(w, oauthLinkErrorReauthRequired)
			s.Zero(completer.calls, "a mismatched session must be refused before the provider token exchange")
			s.assertNoIdentityLinked(owner.ID)
			s.assertLinkIntentSpent(w, cookie)
		})
	}
}

// A session for the intent's account that no longer exists is a credential
// that names nobody: a mismatch, not a closed account.
func (s *OAuthHandlerIntegrationSuite) TestLinkCallback_SessionForAVanishedAccountIsRefused() {
	owner := s.createLinkUser("gate-vanished@test.com")
	completer := &mockOAuthCompleter{user: goth.User{
		Provider: "google", UserID: "google-gate-vanished", Email: "gate-vanished@test.com",
	}}
	handler := s.newHandler(completer)

	w, req := oauthCallbackRequest("google")
	cookie, _ := s.armLinkIntentOn(req, owner.ID, "google")
	s.addSessionCookie(req, owner)
	s.Require().NoError(s.deps.DB.Exec("DELETE FROM users WHERE id = ?", owner.ID).Error)

	handler.OAuthCallbackHTTPHandler(w, req)

	s.assertLinkRefusal(w, oauthLinkErrorReauthRequired)
	s.Zero(completer.calls)
	s.assertLinkIntentSpent(w, cookie)
}

// A fault in resolving the session is not a verdict on it: the reader is not
// told to sign in again, and nothing is linked.
func (s *OAuthHandlerIntegrationSuite) TestLinkCallback_SessionStoreFaultIsNotReportedAsReauth() {
	owner := s.createLinkUser("gate-store-fault@test.com")
	completer := &mockOAuthCompleter{user: goth.User{
		Provider: "google", UserID: "google-gate-store-fault", Email: "gate-store-fault@test.com",
	}}
	authService := auth.NewAuthService(s.deps.DB, s.cfg, s.deps.UserService)
	authService.SetOAuthCompleter(completer)
	unavailable := auth.NewJWTService(nil, s.cfg, usersvc.NewUserService(nil))
	handler := NewOAuthHTTPHandler(authService, unavailable, s.cfg)

	w, req := oauthCallbackRequest("google")
	cookie, _ := s.armLinkIntentOn(req, owner.ID, "google")
	s.addSessionCookie(req, owner)

	handler.OAuthCallbackHTTPHandler(w, req)

	s.assertLinkRefusal(w, autherrors.CodeUnknown)
	s.Zero(completer.calls)
	s.assertNoIdentityLinked(owner.ID)
	s.assertLinkIntentSpent(w, cookie)
}
