package auth

import (
	"github.com/markbates/goth"

	autherrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
)

// The callback is a public route: the session it carries is checked for WHO it
// names, and the account named is read for whether it may still sign in. An
// account deactivated while its owner was at the provider still holds a
// correctly signed session cookie, so only the account's state refuses it.
func (s *OAuthHandlerIntegrationSuite) TestLinkCallback_AccountThatMayNotSignInIsRefused() {
	user := &authm.User{Email: strPtr("link-deactivated@test.com"), IsActive: true, EmailVerified: true}
	s.Require().NoError(s.deps.DB.Create(user).Error)

	handler := s.newHandler(&mockOAuthCompleter{user: goth.User{
		Provider: "google", UserID: "google-deactivated-subject", Email: "link-deactivated@test.com",
	}})

	w, req := oauthCallbackRequest("google")
	cookie, _ := s.armLinkIntentOn(req, user.ID, "google")
	s.addSessionCookie(req, user)
	s.Require().NoError(s.deps.DB.Model(&authm.User{}).Where("id = ?", user.ID).Update("is_active", false).Error)

	handler.OAuthCallbackHTTPHandler(w, req)

	s.assertLinkRefusal(w, autherrors.CodeAccountInactive)

	var rows int64
	s.Require().NoError(s.deps.DB.Model(&authm.OAuthAccount{}).
		Where("user_id = ?", user.ID).Count(&rows).Error)
	s.Equal(int64(0), rows, "a refused link must write no identity")

	// Spent and cleared, the same as any other refusal past the intent match.
	_, stillHeld := peekOAuthLinkIntent(cookie.Value)
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
