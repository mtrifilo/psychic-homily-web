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

// parseLinkRedirect asserts the browser was sent back to the Settings tab and
// returns the query it carried. Parsed rather than prefix-matched: the query
// is encoded from a map, so parameter order is not part of the contract.
func (s *OAuthHandlerIntegrationSuite) parseLinkRedirect(location string) url.Values {
	s.T().Helper()
	parsed, err := url.Parse(location)
	s.Require().NoError(err)
	s.Equal("http://localhost:3000/profile", parsed.Scheme+"://"+parsed.Host+parsed.Path)
	s.Equal("settings", parsed.Query().Get("tab"))
	return parsed.Query()
}

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

	intent := takeOAuthLinkIntent(cookie.Value)
	s.Require().NotNil(intent)
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
	s.armLinkIntentOn(req, user.ID, "google")
	handler.OAuthCallbackHTTPHandler(w, req)

	s.Equal(http.StatusTemporaryRedirect, w.Code)
	query := s.parseLinkRedirect(w.Header().Get("Location"))
	s.Equal("connected", query.Get(oauthLinkResultParam))
	s.Empty(query.Get(oauthLinkErrorParam))

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
	cookie, state := s.armLinkIntent(user.ID, "google")

	w, req := oauthCallbackRequest("google")
	req.AddCookie(cookie)
	setCallbackState(req, state)
	handler.OAuthCallbackHTTPHandler(w, req)
	s.Contains(w.Header().Get("Location"), "oauth_link=connected")

	// Byte-identical replay: same cookie, same state.
	replayW, replayReq := oauthCallbackRequest("google")
	replayReq.AddCookie(cookie)
	setCallbackState(replayReq, state)
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

// An abandoned link leaves its cookie live for the whole TTL. The next
// callback on this browser is an ORDINARY SIGN-IN, carrying the state gothic
// minted for it, and it must not be diverted into the link path: on a shared
// machine that would attach the next person's provider identity to the account
// whose owner walked away.
func (s *OAuthHandlerIntegrationSuite) TestLinkCallback_StaleIntentDoesNotDivertADifferentHandshake() {
	abandoner := &authm.User{Email: strPtr("link-abandoner@test.com"), IsActive: true, EmailVerified: true}
	s.Require().NoError(s.deps.DB.Create(abandoner).Error)

	handler := s.newHandler(&mockOAuthCompleter{user: goth.User{
		Provider: "google",
		UserID:   "google-next-person-subject",
		Email:    "next.person@test.com",
		RawData:  map[string]any{"verified_email": true},
	}})

	// Armed and abandoned: the cookie survives, its state was never used.
	cookie, _ := s.armLinkIntent(abandoner.ID, "google")

	// A separate sign-in handshake comes back with its own state.
	w, req := oauthCallbackRequest("google")
	req.AddCookie(cookie)
	setCallbackState(req, "a-different-handshakes-state")
	handler.OAuthCallbackHTTPHandler(w, req)

	s.assertLinkRefusal(w, autherrors.ErrOAuthLinkExpired().UserMessage())

	var rows int64
	s.Require().NoError(s.deps.DB.Model(&authm.OAuthAccount{}).
		Where("user_id = ?", abandoner.ID).Count(&rows).Error)
	s.Equal(int64(0), rows, "a stale intent must not capture another handshake's identity")
}

// An intent authorizes one provider's handshake, not any handshake.
func (s *OAuthHandlerIntegrationSuite) TestLinkCallback_ProviderMismatchRefused() {
	user := &authm.User{Email: strPtr("link-mismatch@test.com"), IsActive: true, EmailVerified: true}
	s.Require().NoError(s.deps.DB.Create(user).Error)

	handler := s.newHandler(&mockOAuthCompleter{user: goth.User{
		Provider: "google", UserID: "google-mismatch-subject", Email: "link-mismatch@test.com",
	}})

	w, req := oauthCallbackRequest("google")
	s.armLinkIntentOn(req, user.ID, "github")
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
	s.armLinkIntentOn(req, claimant.ID, "google")
	handler.OAuthCallbackHTTPHandler(w, req)

	s.assertLinkRefusal(w, autherrors.ErrOAuthIdentityInUse("google").UserMessage())
	var rows int64
	s.Require().NoError(s.deps.DB.Model(&authm.OAuthAccount{}).
		Where("user_id = ?", claimant.ID).Count(&rows).Error)
	s.Equal(int64(0), rows)
}

// A callback with no intent is a sign-in, which is what keeps the two flows on
// one registered redirect URI without either changing the other. An address no
// account holds is the case where a sign-in still ends in a session.
func (s *OAuthHandlerIntegrationSuite) TestCallback_WithoutIntentStillSignsIn() {
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

// The other half of the no-intent case. An address an account already holds is
// refused, and refused as a SIGN-IN: the redirect goes to the auth page with
// the sign-in refusal's copy, not to the settings surface the link flow
// reports on.
func (s *OAuthHandlerIntegrationSuite) TestCallback_WithoutIntent_RefusesMatchingAddress() {
	existing := &authm.User{Email: strPtr("link-absent-intent-taken@test.com"), IsActive: true, EmailVerified: true}
	s.Require().NoError(s.deps.DB.Create(existing).Error)

	handler := s.newHandler(&mockOAuthCompleter{user: goth.User{
		Provider: "google",
		UserID:   "google-no-intent-taken-subject",
		Email:    "link-absent-intent-taken@test.com",
		RawData:  map[string]any{"verified_email": true},
	}})

	w, req := oauthCallbackRequest("google")
	s.addSignupConsentCookie(req)
	handler.OAuthCallbackHTTPHandler(w, req)

	s.Equal(http.StatusTemporaryRedirect, w.Code)
	location := w.Header().Get("Location")
	s.True(strings.HasPrefix(location, "http://localhost:3000/auth?error="),
		"expected the sign-in refusal redirect, got %s", location)

	parsed, err := url.Parse(location)
	s.Require().NoError(err)
	s.Equal(
		autherrors.ErrOAuthLinkRefused("link-absent-intent-taken@test.com").UserMessage(),
		parsed.Query().Get("error"),
	)

	for _, c := range w.Result().Cookies() {
		if c.Name == "auth_token" && c.Value != "" {
			s.Fail("a refused sign-in must not issue a session")
		}
	}

	var rows int64
	s.Require().NoError(s.deps.DB.Model(&authm.OAuthAccount{}).
		Where("user_id = ?", existing.ID).Count(&rows).Error)
	s.Equal(int64(0), rows)
}

// --- store ---

func (s *OAuthHandlerIntegrationSuite) TestLinkIntent_ExpiredIsNotUsable() {
	id, err := randomHexID(oauthLinkIntentIDBytes)
	s.Require().NoError(err)
	storeOAuthLinkIntent(id, oauthLinkIntent{
		userID:    1,
		provider:  "google",
		expiresAt: time.Now().Add(-time.Second),
	})

	s.Nil(takeOAuthLinkIntent(id))
}

// armLinkIntent stores an intent and returns the two things a browser carries
// back from the provider for it: the cookie, and the state the initiation put
// on the authorization URL.
func (s *OAuthHandlerIntegrationSuite) armLinkIntent(userID uint, provider string) (*http.Cookie, string) {
	s.T().Helper()
	id, err := randomHexID(oauthLinkIntentIDBytes)
	s.Require().NoError(err)
	s.Require().Len(id, 2*oauthLinkIntentIDBytes)
	state, err := randomHexID(oauthLinkStateBytes)
	s.Require().NoError(err)
	storeOAuthLinkIntent(id, oauthLinkIntent{
		userID:    userID,
		provider:  provider,
		state:     state,
		expiresAt: time.Now().Add(oauthLinkIntentTTL),
	})
	return &http.Cookie{Name: oauthLinkIntentCookieName, Value: id}, state
}

// armLinkIntentOn puts a pending link on req the way a real handshake would:
// the cookie the initiation set, and the state it sent to the provider.
func (s *OAuthHandlerIntegrationSuite) armLinkIntentOn(req *http.Request, userID uint, provider string) (*http.Cookie, string) {
	s.T().Helper()
	cookie, state := s.armLinkIntent(userID, provider)
	req.AddCookie(cookie)
	setCallbackState(req, state)
	return cookie, state
}

// setCallbackState puts the OAuth state the provider echoes back on a callback
// request. gothic reads it from the query; so does the link intent check.
func setCallbackState(req *http.Request, state string) {
	q := req.URL.Query()
	q.Set("state", state)
	req.URL.RawQuery = q.Encode()
}

func (s *OAuthHandlerIntegrationSuite) assertLinkRefusal(w *httptest.ResponseRecorder, wantMessage string) {
	s.T().Helper()
	s.Equal(http.StatusTemporaryRedirect, w.Code)
	query := s.parseLinkRedirect(w.Header().Get("Location"))
	s.Equal(wantMessage, query.Get(oauthLinkErrorParam))
	s.Empty(query.Get(oauthLinkResultParam))
}
