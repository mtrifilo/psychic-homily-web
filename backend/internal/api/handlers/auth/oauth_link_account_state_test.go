package auth

import (
	"github.com/markbates/goth"

	autherrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
)

// The callback is a public route: the session it carries is checked for WHO it
// names, and the account named is read for whether it may still sign in. An
// account closed while its owner was at the provider still holds a correctly
// signed session cookie, so only the account's state refuses it, and it does
// so before the provider token exchange.
func (s *OAuthHandlerIntegrationSuite) TestLinkCallback_AccountThatMayNotSignInIsRefused() {
	for _, state := range signInAccountStates {
		if state.maySignIn {
			continue
		}
		address := "link-" + state.name + "@test.com"
		user := &authm.User{Email: strPtr(address), IsActive: true, EmailVerified: true}
		s.Require().NoError(s.deps.DB.Create(user).Error)

		completer := &mockOAuthCompleter{user: goth.User{
			Provider: "google", UserID: "google-" + state.name + "-subject", Email: address,
		}}
		handler := s.newHandler(completer)

		w, req := oauthCallbackRequest("google")
		cookie, _ := s.armLinkIntentOn(req, user.ID, "google")
		s.addSessionCookie(req, user)
		state.apply(s, user.ID)

		handler.OAuthCallbackHTTPHandler(w, req)

		s.assertLinkRefusal(w, autherrors.CodeAccountInactive)
		s.Zero(completer.calls, "%s: a closed account must be refused before the provider token exchange", state.name)
		s.assertNoIdentityLinked(user.ID)

		// Spent and cleared, the same as any other refusal past the intent match.
		_, _, stillHeld := peekOAuthLinkIntent(cookie.Value)
		s.False(stillHeld, "%s: a refused link must consume its intent", state.name)
		s.assertNoSessionCookie(w)
		var intentCleared bool
		for _, c := range w.Result().Cookies() {
			if c.Name == oauthLinkIntentCookieName && c.MaxAge < 0 {
				intentCleared = true
			}
		}
		s.True(intentCleared, "%s: a refused link must clear the intent cookie", state.name)
	}
}

// The session gate passes an open account, so an account closed while its
// owner is at the provider is seen only by the reads after the token exchange.
func (s *OAuthHandlerIntegrationSuite) TestLinkCallback_AccountClosedDuringHandshakeIsRefused() {
	for _, state := range signInAccountStates {
		if state.maySignIn {
			continue
		}
		address := "link-mid-handshake-" + state.name + "@test.com"
		user := &authm.User{Email: strPtr(address), IsActive: true, EmailVerified: true}
		s.Require().NoError(s.deps.DB.Create(user).Error)

		completer := &mockOAuthCompleter{
			user: goth.User{
				Provider: "google", UserID: "google-mid-handshake-" + state.name, Email: address,
			},
			onExchange: func() { state.apply(s, user.ID) },
		}
		handler := s.newHandler(completer)

		w, req := oauthCallbackRequest("google")
		s.armLinkIntentOn(req, user.ID, "google")
		s.addSessionCookie(req, user)

		handler.OAuthCallbackHTTPHandler(w, req)

		s.Equal(1, completer.calls, "%s: an open account passes the session gate and reaches the provider", state.name)
		s.assertLinkRefusal(w, autherrors.CodeAccountInactive)
		s.assertNoIdentityLinked(user.ID)
	}
}

func (s *OAuthHandlerIntegrationSuite) assertNoIdentityLinked(userID uint) {
	s.T().Helper()
	var rows int64
	s.Require().NoError(s.deps.DB.Model(&authm.OAuthAccount{}).
		Where("user_id = ?", userID).Count(&rows).Error)
	s.Equal(int64(0), rows, "a refused link must write no identity")
}
