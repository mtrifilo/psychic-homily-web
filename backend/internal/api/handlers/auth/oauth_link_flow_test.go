package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/markbates/goth"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	autherrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
)

const linkSettingsPrefix = "http://localhost:3000/profile?tab=settings"

func oauthLinkRequest(provider string, user *authm.User) (*httptest.ResponseRecorder, *http.Request) {
	req := httptest.NewRequest("GET", "/auth/link/"+provider, nil)

	// The principal goes in the way the JWT middleware puts it there, so this
	// exercises the same read the handler makes in production.
	ctx := context.Background()
	if user != nil {
		ctx = testhelpers.CtxWithUser(user)
	}
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("provider", provider)
	ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)

	return httptest.NewRecorder(), req.WithContext(ctx)
}

func linkIntentCookie(w *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range w.Result().Cookies() {
		if c.Name == oauthLinkIntentCookieName && c.Value != "" {
			return c
		}
	}
	return nil
}

// --- initiation ---

func (s *OAuthHandlerIntegrationSuite) TestLink_ArmsAnIntentForTheSessionsAccount() {
	user := &authm.User{Email: strPtr("link-start@test.com"), IsActive: true, EmailVerified: true}
	s.Require().NoError(s.deps.DB.Create(user).Error)

	handler := s.newHandler(&mockOAuthCompleter{})
	w, req := oauthLinkRequest("google", user)
	handler.OAuthLinkHTTPHandler(w, req)

	cookie := linkIntentCookie(w)
	s.Require().NotNil(cookie, "the intent cookie is what tells the callback this is a link")
	s.True(cookie.HttpOnly)
	s.Equal(http.SameSiteLaxMode, cookie.SameSite,
		"Strict would be stripped on the provider's cross-site return")

	// The cookie names nothing on its own: the account lives in the store.
	s.NotContains(cookie.Value, "link-start@test.com")

	intent, ok := takeOAuthLinkIntent(cookie.Value)
	s.Require().True(ok)
	s.Equal(user.ID, intent.userID)
	s.Equal("google", intent.provider)
}

func (s *OAuthHandlerIntegrationSuite) TestLink_UnknownProviderRefused() {
	user := &authm.User{Email: strPtr("link-bad-provider@test.com"), IsActive: true, EmailVerified: true}
	s.Require().NoError(s.deps.DB.Create(user).Error)

	handler := s.newHandler(&mockOAuthCompleter{})
	w, req := oauthLinkRequest("evilcorp", user)
	handler.OAuthLinkHTTPHandler(w, req)

	s.Equal(http.StatusBadRequest, w.Code)
	s.Nil(linkIntentCookie(w))
}

// The route runs behind the JWT middleware, but the handler must not depend on
// that for correctness: with no principal there is no account to link to.
func (s *OAuthHandlerIntegrationSuite) TestLink_NoSessionRefused() {
	handler := s.newHandler(&mockOAuthCompleter{})
	w, req := oauthLinkRequest("google", nil)
	handler.OAuthLinkHTTPHandler(w, req)

	s.Equal(http.StatusUnauthorized, w.Code)
	s.Nil(linkIntentCookie(w))
}

// --- callback with an intent ---

func (s *OAuthHandlerIntegrationSuite) TestLinkCallback_AttachesIdentityAndIssuesNoSession() {
	user := &authm.User{Email: strPtr("link-complete@test.com"), IsActive: true, EmailVerified: false}
	s.Require().NoError(s.deps.DB.Create(user).Error)

	handler := s.newHandler(&mockOAuthCompleter{user: goth.User{
		Provider: "google",
		UserID:   "google-link-subject",
		// A DIFFERENT mailbox, and no verification signal: the link resolves
		// the account from the intent, so neither is consulted.
		Email: "someone.else@test.com",
	}})

	w, req := oauthCallbackRequest("google")
	req.AddCookie(s.armLinkIntent(user.ID, "google"))
	handler.OAuthCallbackHTTPHandler(w, req)

	s.Equal(http.StatusTemporaryRedirect, w.Code)
	location := w.Header().Get("Location")
	s.True(strings.HasPrefix(location, linkSettingsPrefix), "got %s", location)
	s.Contains(location, "oauth_link=connected")

	for _, c := range w.Result().Cookies() {
		if c.Name == "auth_token" && c.Value != "" {
			s.Fail("a link must not issue a session")
		}
	}

	var rows int64
	s.Require().NoError(s.deps.DB.Model(&authm.OAuthAccount{}).
		Where("user_id = ? AND provider_user_id = ?", user.ID, "google-link-subject").
		Count(&rows).Error)
	s.Equal(int64(1), rows)

	// Possession of a provider account says nothing about this mailbox.
	var stored authm.User
	s.Require().NoError(s.deps.DB.First(&stored, user.ID).Error)
	s.False(stored.EmailVerified)
	s.Equal("link-complete@test.com", *stored.Email)
}

// The intent is single use, so a replayed callback finds nothing armed and is
// refused rather than linking again.
func (s *OAuthHandlerIntegrationSuite) TestLinkCallback_IntentIsSingleUse() {
	user := &authm.User{Email: strPtr("link-replay@test.com"), IsActive: true, EmailVerified: true}
	s.Require().NoError(s.deps.DB.Create(user).Error)

	handler := s.newHandler(&mockOAuthCompleter{user: goth.User{
		Provider: "google", UserID: "google-replay-subject", Email: "link-replay@test.com",
	}})
	cookie := s.armLinkIntent(user.ID, "google")

	w, req := oauthCallbackRequest("google")
	req.AddCookie(cookie)
	handler.OAuthCallbackHTTPHandler(w, req)
	s.Contains(w.Header().Get("Location"), "oauth_link=connected")

	replayW, replayReq := oauthCallbackRequest("google")
	replayReq.AddCookie(cookie)
	handler.OAuthCallbackHTTPHandler(replayW, replayReq)

	s.assertLinkRefusal(replayW, autherrors.ErrOAuthLinkExpired().UserMessage())
}

// The fail-closed rule: an intent cookie with nothing behind it must NOT fall
// through to the sign-in path, which would resolve an account from the address
// the provider returned instead of from the session that started the attempt.
func (s *OAuthHandlerIntegrationSuite) TestLinkCallback_UnknownIntentDoesNotFallThroughToSignIn() {
	owner := &authm.User{Email: strPtr("link-no-fallthrough@test.com"), IsActive: true, EmailVerified: true}
	s.Require().NoError(s.deps.DB.Create(owner).Error)

	handler := s.newHandler(&mockOAuthCompleter{user: goth.User{
		Provider: "google",
		UserID:   "google-fallthrough-subject",
		Email:    "link-no-fallthrough@test.com",
		RawData:  map[string]any{"verified_email": true},
	}})

	w, req := oauthCallbackRequest("google")
	req.AddCookie(&http.Cookie{Name: oauthLinkIntentCookieName, Value: "an-id-nothing-armed"})
	handler.OAuthCallbackHTTPHandler(w, req)

	s.assertLinkRefusal(w, autherrors.ErrOAuthLinkExpired().UserMessage())

	// Had it fallen through, this address plus a vouching provider would have
	// linked and issued a session.
	for _, c := range w.Result().Cookies() {
		if c.Name == "auth_token" && c.Value != "" {
			s.Fail("an expired link must not issue a session")
		}
	}
	var rows int64
	s.Require().NoError(s.deps.DB.Model(&authm.OAuthAccount{}).
		Where("user_id = ?", owner.ID).Count(&rows).Error)
	s.Equal(int64(0), rows)
}

// An intent authorizes one provider's handshake, not any handshake.
func (s *OAuthHandlerIntegrationSuite) TestLinkCallback_ProviderMismatchRefused() {
	user := &authm.User{Email: strPtr("link-mismatch@test.com"), IsActive: true, EmailVerified: true}
	s.Require().NoError(s.deps.DB.Create(user).Error)

	handler := s.newHandler(&mockOAuthCompleter{user: goth.User{
		Provider: "google", UserID: "google-mismatch-subject", Email: "link-mismatch@test.com",
	}})

	w, req := oauthCallbackRequest("google")
	req.AddCookie(s.armLinkIntent(user.ID, "github"))
	handler.OAuthCallbackHTTPHandler(w, req)

	s.assertLinkRefusal(w, autherrors.ErrOAuthLinkExpired().UserMessage())
	var rows int64
	s.Require().NoError(s.deps.DB.Model(&authm.OAuthAccount{}).
		Where("user_id = ?", user.ID).Count(&rows).Error)
	s.Equal(int64(0), rows)
}

// A refusal from the service reaches the user in its own words, because it is
// the only place they are told what to do about it.
func (s *OAuthHandlerIntegrationSuite) TestLinkCallback_IdentityInUseCarriesItsOwnCopy() {
	holder := &authm.User{Email: strPtr("link-holder@test.com"), IsActive: true, EmailVerified: true}
	s.Require().NoError(s.deps.DB.Create(holder).Error)
	s.Require().NoError(s.deps.DB.Create(&authm.OAuthAccount{
		UserID: holder.ID, Provider: "google", ProviderUserID: "google-contested-subject",
	}).Error)

	claimant := &authm.User{Email: strPtr("link-claimant@test.com"), IsActive: true, EmailVerified: true}
	s.Require().NoError(s.deps.DB.Create(claimant).Error)

	handler := s.newHandler(&mockOAuthCompleter{user: goth.User{
		Provider: "google", UserID: "google-contested-subject", Email: "link-claimant@test.com",
	}})

	w, req := oauthCallbackRequest("google")
	req.AddCookie(s.armLinkIntent(claimant.ID, "google"))
	handler.OAuthCallbackHTTPHandler(w, req)

	s.assertLinkRefusal(w, autherrors.ErrOAuthIdentityInUse("google").UserMessage())
	var rows int64
	s.Require().NoError(s.deps.DB.Model(&authm.OAuthAccount{}).
		Where("user_id = ?", claimant.ID).Count(&rows).Error)
	s.Equal(int64(0), rows)
}

// A callback with no intent is a sign-in, which is what keeps the two flows on
// one registered redirect URI without either changing the other.
func (s *OAuthHandlerIntegrationSuite) TestCallback_WithoutIntentStillSignsIn() {
	existing := &authm.User{Email: strPtr("link-absent-intent@test.com"), IsActive: true, EmailVerified: true}
	s.Require().NoError(s.deps.DB.Create(existing).Error)

	handler := s.newHandler(&mockOAuthCompleter{user: goth.User{
		Provider: "google",
		UserID:   "google-no-intent-subject",
		Email:    "link-absent-intent@test.com",
		RawData:  map[string]any{"verified_email": true},
	}})

	w, req := oauthCallbackRequest("google")
	s.addSignupConsentCookie(req)
	handler.OAuthCallbackHTTPHandler(w, req)

	s.Equal("http://localhost:3000", w.Header().Get("Location"))
	sessionIssued := false
	for _, c := range w.Result().Cookies() {
		if c.Name == "auth_token" && c.Value != "" {
			sessionIssued = true
		}
	}
	s.True(sessionIssued)
}

// --- store ---

func (s *OAuthHandlerIntegrationSuite) TestLinkIntent_ExpiredIsNotUsable() {
	id, err := newOAuthLinkIntentID()
	s.Require().NoError(err)
	storeOAuthLinkIntent(id, oauthLinkIntent{
		userID:    1,
		provider:  "google",
		expiresAt: time.Now().Add(-time.Second),
	})

	_, ok := takeOAuthLinkIntent(id)
	s.False(ok)
}

func (s *OAuthHandlerIntegrationSuite) TestLinkIntentIDs_AreDistinct() {
	first, err := newOAuthLinkIntentID()
	s.Require().NoError(err)
	second, err := newOAuthLinkIntentID()
	s.Require().NoError(err)
	s.NotEqual(first, second)
	s.Len(first, 64)
}

// armLinkIntent stores an intent and returns the cookie a browser would carry
// back from the provider.
func (s *OAuthHandlerIntegrationSuite) armLinkIntent(userID uint, provider string) *http.Cookie {
	s.T().Helper()
	id, err := newOAuthLinkIntentID()
	s.Require().NoError(err)
	storeOAuthLinkIntent(id, oauthLinkIntent{
		userID:    userID,
		provider:  provider,
		expiresAt: time.Now().Add(oauthLinkIntentTTL),
	})
	return &http.Cookie{Name: oauthLinkIntentCookieName, Value: id}
}

func (s *OAuthHandlerIntegrationSuite) assertLinkRefusal(w *httptest.ResponseRecorder, wantMessage string) {
	s.T().Helper()
	s.Equal(http.StatusTemporaryRedirect, w.Code)
	location := w.Header().Get("Location")
	s.Require().True(strings.HasPrefix(location, linkSettingsPrefix), "got %s", location)

	parsed, err := url.Parse(location)
	s.Require().NoError(err)
	s.Equal(wantMessage, parsed.Query().Get("oauth_link_error"))
	s.Empty(parsed.Query().Get("oauth_link"))
}
