package auth

import (
	"net/http"

	"github.com/markbates/goth"
	"gorm.io/gorm"

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

	completer := &mockOAuthCompleter{user: goth.User{
		Provider: "google", UserID: "google-deactivated-subject", Email: "link-deactivated@test.com",
	}}
	handler := s.newHandler(completer)

	w, req := oauthCallbackRequest("google")
	cookie, _ := s.armLinkIntentOn(req, user.ID, "google")
	s.addSessionCookie(req, user)
	s.Require().NoError(s.deps.DB.Model(&authm.User{}).Where("id = ?", user.ID).Update("is_active", false).Error)

	handler.OAuthCallbackHTTPHandler(w, req)

	s.assertLinkRefusal(w, autherrors.CodeAccountInactive)
	s.Zero(completer.calls, "a closed account must be refused before the provider token exchange")

	var rows int64
	s.Require().NoError(s.deps.DB.Model(&authm.OAuthAccount{}).
		Where("user_id = ?", user.ID).Count(&rows).Error)
	s.Equal(int64(0), rows, "a refused link must write no identity")

	// Spent and cleared, the same as any other refusal past the intent match.
	_, _, stillHeld := peekOAuthLinkIntent(cookie.Value)
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

// closingCompleter closes the account while its owner is at the provider: the
// session gate has already passed, so only the reads after the token exchange
// can see the change.
type closingCompleter struct {
	db     *gorm.DB
	userID uint
	user   goth.User
	calls  int
}

func (c *closingCompleter) CompleteUserAuth(http.ResponseWriter, *http.Request) (goth.User, error) {
	c.calls++
	if err := c.db.Model(&authm.User{}).Where("id = ?", c.userID).Update("is_active", false).Error; err != nil {
		return goth.User{}, err
	}
	return c.user, nil
}

func (s *OAuthHandlerIntegrationSuite) TestLinkCallback_AccountClosedDuringHandshakeIsRefused() {
	user := &authm.User{Email: strPtr("link-closed-mid-handshake@test.com"), IsActive: true, EmailVerified: true}
	s.Require().NoError(s.deps.DB.Create(user).Error)

	completer := &closingCompleter{db: s.deps.DB, userID: user.ID, user: goth.User{
		Provider: "google", UserID: "google-closed-mid-handshake", Email: "link-closed-mid-handshake@test.com",
	}}
	handler := s.newHandler(completer)

	w, req := oauthCallbackRequest("google")
	s.armLinkIntentOn(req, user.ID, "google")
	s.addSessionCookie(req, user)

	handler.OAuthCallbackHTTPHandler(w, req)

	s.Equal(1, completer.calls, "an open account passes the session gate and reaches the provider")
	s.assertLinkRefusal(w, autherrors.CodeAccountInactive)
	var rows int64
	s.Require().NoError(s.deps.DB.Model(&authm.OAuthAccount{}).
		Where("user_id = ?", user.ID).Count(&rows).Error)
	s.Equal(int64(0), rows, "a refused link must write no identity")
}
