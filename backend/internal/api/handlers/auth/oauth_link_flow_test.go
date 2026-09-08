package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/markbates/goth"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/api/middleware"
	autherrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
)

// testFrontendOrigin is the origin the test config names as the frontend, and
// so the one a link start has to appear to come from.
const testFrontendOrigin = "http://localhost:3000"

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

// oauthLinkRequest builds a start request the way a real one arrives from
// Settings: a session, a session age recent enough to satisfy re-auth, and the
// one-time token Settings mints. Each is a separate gate, and the tests below
// that exercise one of them drop it explicitly.
func (s *OAuthHandlerIntegrationSuite) oauthLinkRequest(provider string, user *authm.User) (*httptest.ResponseRecorder, *http.Request) {
	s.T().Helper()
	w, req := s.oauthLinkRequestWithoutToken(provider, user)
	if user != nil {
		token, err := mintOAuthLinkToken(s.cfg.JWT.SecretKey, user.ID)
		s.Require().NoError(err)
		q := req.URL.Query()
		q.Set(oauthLinkTokenParam, token)
		req.URL.RawQuery = q.Encode()
	}
	return w, req
}

// oauthLinkRequestWithoutToken is the same request with no link token on it.
func (s *OAuthHandlerIntegrationSuite) oauthLinkRequestWithoutToken(provider string, user *authm.User) (*httptest.ResponseRecorder, *http.Request) {
	s.T().Helper()
	req := httptest.NewRequest("GET", "/auth/link/"+provider, nil)
	// A browser navigating from our own Settings page says so; the start
	// refuses a request that claims nothing.
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Origin", testFrontendOrigin)

	// The principal and the credential's age go in the way the JWT middleware
	// puts them there, so this exercises the same reads the handler makes.
	ctx := context.Background()
	if user != nil {
		ctx = testhelpers.CtxWithUser(user)
		ctx = context.WithValue(ctx, middleware.SessionAuthTimeContextKey, time.Now())
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
	w, req := s.oauthLinkRequest("google", user)
	handler.OAuthLinkHTTPHandler(w, req)

	cookie := linkIntentCookie(w)
	s.Require().NotNil(cookie, "the intent cookie is what tells the callback this is a link")
	s.True(cookie.HttpOnly)
	s.Equal(http.SameSiteLaxMode, cookie.SameSite,
		"Strict would be stripped on the provider's cross-site return")

	// The cookie names nothing on its own: the account lives in the store.
	s.NotContains(cookie.Value, "link-start@test.com")

	intent, found := peekOAuthLinkIntent(cookie.Value)
	s.Require().True(found)
	s.Equal(user.ID, intent.userID)
	s.Equal("google", intent.provider)
}

// The CSRF gate. /auth/link/{provider} is a cookie-authenticated GET and the
// auth cookie is SameSite=Lax, which a browser DOES send on a cross-site
// top-level navigation. Without the token, any page on the internet could push
// a signed-in user through the connect flow, and a user with a live provider
// session completes it with no interaction at all.
func (s *OAuthHandlerIntegrationSuite) TestLink_WithoutTokenRefused() {
	user := &authm.User{Email: strPtr("link-no-token@test.com"), IsActive: true, EmailVerified: true}
	s.Require().NoError(s.deps.DB.Create(user).Error)

	handler := s.newHandler(&mockOAuthCompleter{})
	w, req := s.oauthLinkRequestWithoutToken("google", user)
	handler.OAuthLinkHTTPHandler(w, req)

	s.assertLinkRefusal(w, oauthLinkErrorNotFromSettings)
	s.Nil(linkIntentCookie(w), "a refused start must arm nothing")
}

// The token is bound to the account, not merely to existing: one user's token
// must not start a link on another's session.
func (s *OAuthHandlerIntegrationSuite) TestLink_TokenFromAnotherAccountRefused() {
	owner := &authm.User{Email: strPtr("link-token-owner@test.com"), IsActive: true, EmailVerified: true}
	s.Require().NoError(s.deps.DB.Create(owner).Error)
	other := &authm.User{Email: strPtr("link-token-other@test.com"), IsActive: true, EmailVerified: true}
	s.Require().NoError(s.deps.DB.Create(other).Error)

	othersToken, err := mintOAuthLinkToken(s.cfg.JWT.SecretKey, other.ID)
	s.Require().NoError(err)

	handler := s.newHandler(&mockOAuthCompleter{})
	w, req := s.oauthLinkRequestWithoutToken("google", owner)
	q := req.URL.Query()
	q.Set(oauthLinkTokenParam, othersToken)
	req.URL.RawQuery = q.Encode()
	handler.OAuthLinkHTTPHandler(w, req)

	s.assertLinkRefusal(w, oauthLinkErrorNotFromSettings)
	s.Nil(linkIntentCookie(w))
}

// One use. A replayed start URL, which is the shape a shared or leaked link
// takes, arms nothing the second time.
func (s *OAuthHandlerIntegrationSuite) TestLink_TokenIsSingleUse() {
	user := &authm.User{Email: strPtr("link-token-once@test.com"), IsActive: true, EmailVerified: true}
	s.Require().NoError(s.deps.DB.Create(user).Error)

	handler := s.newHandler(&mockOAuthCompleter{})
	w, req := s.oauthLinkRequest("google", user)
	handler.OAuthLinkHTTPHandler(w, req)
	s.Require().NotNil(linkIntentCookie(w))

	replayW, replayReq := s.oauthLinkRequestWithoutToken("google", user)
	replayReq.URL.RawQuery = req.URL.RawQuery
	handler.OAuthLinkHTTPHandler(replayW, replayReq)

	s.assertLinkRefusal(replayW, oauthLinkErrorNotFromSettings)
	s.Nil(linkIntentCookie(replayW))
}

// Fetch metadata is set by the browser and cannot be forged by page script, so
// a declared cross-site navigation is refused before the token is even spent.
func (s *OAuthHandlerIntegrationSuite) TestLink_CrossSiteNavigationRefused() {
	user := &authm.User{Email: strPtr("link-cross-site@test.com"), IsActive: true, EmailVerified: true}
	s.Require().NoError(s.deps.DB.Create(user).Error)

	handler := s.newHandler(&mockOAuthCompleter{})
	w, req := s.oauthLinkRequest("google", user)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	// And an origin that is not ours: a cross-site deployment naming our own
	// frontend is the ordinary case on stage, and must still be admitted.
	req.Header.Set("Origin", "https://evil.example")
	handler.OAuthLinkHTTPHandler(w, req)

	s.assertLinkRefusal(w, oauthLinkErrorNotFromSettings)
	s.Nil(linkIntentCookie(w))
}

// The re-auth gate. Adding a way to sign in is not something an old cookie
// found in a shared browser may do on its own.
func (s *OAuthHandlerIntegrationSuite) TestLink_StaleSessionRefused() {
	hash := "$2a$not-a-real-hash"
	user := &authm.User{
		Email:        strPtr("link-stale-session@test.com"),
		IsActive:     true,
		PasswordHash: &hash,
	}
	s.Require().NoError(s.deps.DB.Create(user).Error)

	handler := s.newHandler(&mockOAuthCompleter{})
	w, req := s.oauthLinkRequestWithoutToken("google", user)
	token, err := mintOAuthLinkToken(s.cfg.JWT.SecretKey, user.ID)
	s.Require().NoError(err)
	q := req.URL.Query()
	q.Set(oauthLinkTokenParam, token)
	req.URL.RawQuery = q.Encode()
	// Older than recentSessionWindow: the account has a password, so the rule
	// asks for that rather than accepting the cookie.
	req = req.WithContext(context.WithValue(req.Context(),
		middleware.SessionAuthTimeContextKey, time.Now().Add(-2*time.Hour)))

	handler.OAuthLinkHTTPHandler(w, req)

	// Sent to sign in again, with a destination that returns here.
	s.Equal(http.StatusTemporaryRedirect, w.Code)
	location := w.Header().Get("Location")
	parsed, err := url.Parse(location)
	s.Require().NoError(err)
	s.Equal("/auth", parsed.Path)
	s.Contains(parsed.Query().Get("returnTo"), "tab=settings")
	s.Nil(linkIntentCookie(w))

	// The token is checked AFTER re-auth, so a refusal here leaves it usable
	// for the trip back.
	s.True(consumeOAuthLinkToken(s.cfg.JWT.SecretKey, token, user.ID),
		"a stale-session refusal must not burn the token")
}

// A session carrying no authentication time at all: an API-token principal, or
// a session minted before the auth_at claim existed. Absence is not evidence of
// freshness, so the gate refuses and the user signs in once more.
func (s *OAuthHandlerIntegrationSuite) TestLink_SessionWithNoAuthTimeRefused() {
	hash := "$2a$not-a-real-hash"
	user := &authm.User{
		Email:        strPtr("link-no-auth-time@test.com"),
		IsActive:     true,
		PasswordHash: &hash,
	}
	s.Require().NoError(s.deps.DB.Create(user).Error)

	handler := s.newHandler(&mockOAuthCompleter{})
	w, req := s.oauthLinkRequestWithoutToken("google", user)
	token, err := mintOAuthLinkToken(s.cfg.JWT.SecretKey, user.ID)
	s.Require().NoError(err)
	q := req.URL.Query()
	q.Set(oauthLinkTokenParam, token)
	req.URL.RawQuery = q.Encode()
	// The principal is there; the authentication time is not. oauthLinkRequest
	// puts one in, so this drops it explicitly.
	ctx := testhelpers.CtxWithUser(user)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("provider", "google")
	req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))

	handler.OAuthLinkHTTPHandler(w, req)

	s.Equal(http.StatusTemporaryRedirect, w.Code)
	parsed, err := url.Parse(w.Header().Get("Location"))
	s.Require().NoError(err)
	s.Equal("/auth", parsed.Path)
	s.Nil(linkIntentCookie(w))
}

func (s *OAuthHandlerIntegrationSuite) TestLink_UnknownProviderRefused() {
	user := &authm.User{Email: strPtr("link-bad-provider@test.com"), IsActive: true, EmailVerified: true}
	s.Require().NoError(s.deps.DB.Create(user).Error)

	handler := s.newHandler(&mockOAuthCompleter{})
	w, req := s.oauthLinkRequest("evilcorp", user)
	handler.OAuthLinkHTTPHandler(w, req)

	s.Equal(http.StatusBadRequest, w.Code)
	s.Nil(linkIntentCookie(w))
}

// The route runs behind the JWT middleware, but the handler must not depend on
// that for correctness: with no principal there is no account to link to.
func (s *OAuthHandlerIntegrationSuite) TestLink_NoSessionRefused() {
	handler := s.newHandler(&mockOAuthCompleter{})
	w, req := s.oauthLinkRequest("google", nil)
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
	s.addSessionCookie(req, user)
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
	s.addSessionCookie(req, user)
	handler.OAuthCallbackHTTPHandler(w, req)
	s.Contains(w.Header().Get("Location"), "oauth_link=connected")

	// The intent is spent, so nothing is armed for a second link.
	_, stillHeld := peekOAuthLinkIntent(cookie.Value)
	s.False(stillHeld, "a completed link must consume its intent")

	// Byte-identical replay: same cookie, same state. The intent is consumed,
	// so this is no longer a link. The identity the first pass attached now
	// resolves by provider_user_id, so the replay signs that account in rather
	// than attaching anything a second time.
	replayW, replayReq := oauthCallbackRequest("google")
	replayReq.AddCookie(cookie)
	setCallbackState(replayReq, state)
	s.addSessionCookie(replayReq, user)
	handler.OAuthCallbackHTTPHandler(replayW, replayReq)

	s.Equal("http://localhost:3000", replayW.Header().Get("Location"))

	var rows int64
	s.Require().NoError(s.deps.DB.Model(&authm.OAuthAccount{}).
		Where("user_id = ?", user.ID).Count(&rows).Error)
	s.Equal(int64(1), rows, "the replay must not attach a second identity")
}

// An intent cookie this process never held is a restart, or another replica.
// That is not evidence the reader wanted a link rather than a sign-in, so the
// callback is the sign-in it looks like and the stale cookie is cleared.
//
// Refusing here instead would strand an ordinary sign-in on a page it never
// asked for, every time a deploy rolled while anyone had a cookie in flight.
func (s *OAuthHandlerIntegrationSuite) TestCallback_UnknownIntentFallsThroughToSignIn() {
	handler := s.newHandler(&mockOAuthCompleter{user: goth.User{
		Provider: "google",
		UserID:   "google-unknown-intent-subject",
		Email:    "unknown.intent.newcomer@test.com",
		RawData:  map[string]any{"verified_email": true},
	}})

	w, req := oauthCallbackRequest("google")
	req.AddCookie(&http.Cookie{Name: oauthLinkIntentCookieName, Value: "an-id-nothing-armed"})
	s.addSignupConsentCookie(req)
	handler.OAuthCallbackHTTPHandler(w, req)

	s.Equal("http://localhost:3000", w.Header().Get("Location"))
	sessionIssued := false
	cookieCleared := false
	for _, c := range w.Result().Cookies() {
		if c.Name == "auth_token" && c.Value != "" {
			sessionIssued = true
		}
		if c.Name == oauthLinkIntentCookieName && c.MaxAge < 0 {
			cookieCleared = true
		}
	}
	s.True(sessionIssued, "an unknown intent must leave a real sign-in working")
	s.True(cookieCleared, "the stale cookie must not confuse the next callback")
}

// An intent that IS this handshake's but has run out of time is the one case
// that owes the reader an explanation, so it is the only one refused.
func (s *OAuthHandlerIntegrationSuite) TestLinkCallback_ExpiredIntentRefusedAsExpired() {
	user := &authm.User{Email: strPtr("link-timed-out@test.com"), IsActive: true, EmailVerified: true}
	s.Require().NoError(s.deps.DB.Create(user).Error)

	handler := s.newHandler(&mockOAuthCompleter{user: goth.User{
		Provider: "google", UserID: "google-timed-out-subject", Email: "link-timed-out@test.com",
	}})

	id, err := randomHexID(oauthLinkIntentIDBytes)
	s.Require().NoError(err)
	state, err := randomHexID(oauthLinkStateBytes)
	s.Require().NoError(err)
	storeOAuthLinkIntent(id, oauthLinkIntent{
		userID:    user.ID,
		provider:  "google",
		state:     state,
		expiresAt: time.Now().Add(-time.Second),
	})

	w, req := oauthCallbackRequest("google")
	req.AddCookie(&http.Cookie{Name: oauthLinkIntentCookieName, Value: id})
	setCallbackState(req, state)
	s.addSessionCookie(req, user)
	handler.OAuthCallbackHTTPHandler(w, req)

	s.assertLinkRefusal(w, autherrors.CodeOAuthLinkExpired)

	var rows int64
	s.Require().NoError(s.deps.DB.Model(&authm.OAuthAccount{}).
		Where("user_id = ?", user.ID).Count(&rows).Error)
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
	s.addSignupConsentCookie(req)
	handler.OAuthCallbackHTTPHandler(w, req)

	// Not diverted: this is the sign-in it actually was.
	s.Equal("http://localhost:3000", w.Header().Get("Location"))

	var rows int64
	s.Require().NoError(s.deps.DB.Model(&authm.OAuthAccount{}).
		Where("user_id = ?", abandoner.ID).Count(&rows).Error)
	s.Equal(int64(0), rows, "a stale intent must not capture another handshake's identity")

	// And the intent survives the look, so the abandoner can still finish the
	// link they started.
	_, stillHeld := peekOAuthLinkIntent(cookie.Value)
	s.True(stillHeld, "a mismatched callback must not burn the intent")
}

// An intent authorizes one provider's handshake, not any handshake. A github
// intent does not make a google callback a link.
func (s *OAuthHandlerIntegrationSuite) TestCallback_ProviderMismatchIsNotThisLink() {
	user := &authm.User{Email: strPtr("link-mismatch@test.com"), IsActive: true, EmailVerified: true}
	s.Require().NoError(s.deps.DB.Create(user).Error)

	handler := s.newHandler(&mockOAuthCompleter{user: goth.User{
		Provider: "google", UserID: "google-mismatch-subject", Email: "mismatch.newcomer@test.com",
	}})

	w, req := oauthCallbackRequest("google")
	s.armLinkIntentOn(req, user.ID, "github")
	s.addSignupConsentCookie(req)
	handler.OAuthCallbackHTTPHandler(w, req)

	s.Equal("http://localhost:3000", w.Header().Get("Location"))
	var rows int64
	s.Require().NoError(s.deps.DB.Model(&authm.OAuthAccount{}).
		Where("user_id = ?", user.ID).Count(&rows).Error)
	s.Equal(int64(0), rows, "a github intent must not absorb a google callback")
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
	s.addSessionCookie(req, claimant)
	handler.OAuthCallbackHTTPHandler(w, req)

	s.assertLinkRefusal(w, autherrors.CodeOAuthIdentityInUse)
	var rows int64
	s.Require().NoError(s.deps.DB.Model(&authm.OAuthAccount{}).
		Where("user_id = ?", claimant.ID).Count(&rows).Error)
	s.Equal(int64(0), rows)
}

// A callback with no intent is a sign-in, which is what keeps the two flows on
// one registered redirect URI without either changing the other. These two
// cover the sign-in branches that end in a session; the address that already
// belongs to an account is the same request refused, in
// TestCallback_VerifiedEmailMatch_RedirectsWithRefusalAndNoSession.

// An address no account holds is a signup, so the consent cookie is what lets
// this one through rather than decoration.
func (s *OAuthHandlerIntegrationSuite) TestCallback_WithoutIntent_SignsUpAnUnheldAddress() {
	handler := s.newHandler(&mockOAuthCompleter{user: goth.User{
		Provider: "google",
		UserID:   "google-no-intent-subject",
		Email:    "no.intent.newcomer@test.com",
		RawData:  map[string]any{"verified_email": true},
	}})

	w, req := oauthCallbackRequest("google")
	s.addSignupConsentCookie(req)
	handler.OAuthCallbackHTTPHandler(w, req)

	s.assertSignedIn(w)
}

// The returning user: the provider identity is already in oauth_accounts, so
// the callback resolves on it and never reaches the address comparison. This
// is the branch every OAuth sign-in takes after the first, and the one the
// e2e faux-provider fixture is seeded for.
func (s *OAuthHandlerIntegrationSuite) TestCallback_WithoutIntent_SignsInAKnownIdentity() {
	existing := &authm.User{Email: strPtr("link-known-identity@test.com"), IsActive: true, EmailVerified: true}
	s.Require().NoError(s.deps.DB.Create(existing).Error)
	s.Require().NoError(s.deps.DB.Create(&authm.OAuthAccount{
		UserID: existing.ID, Provider: "google", ProviderUserID: "google-known-identity-subject",
	}).Error)

	// A different address from the one the account holds, and no verification
	// claim: neither is consulted once the identity resolves.
	handler := s.newHandler(&mockOAuthCompleter{user: goth.User{
		Provider: "google",
		UserID:   "google-known-identity-subject",
		Email:    "link-known-identity-changed@test.com",
	}})

	w, req := oauthCallbackRequest("google")
	handler.OAuthCallbackHTTPHandler(w, req)

	s.assertSignedIn(w)

	var rows int64
	s.Require().NoError(s.deps.DB.Model(&authm.OAuthAccount{}).
		Where("user_id = ?", existing.ID).Count(&rows).Error)
	s.Equal(int64(1), rows, "a returning identity must not add a second row")
}

// assertSignedIn pins a completed sign-in: back to the frontend root, with a
// session.
func (s *OAuthHandlerIntegrationSuite) assertSignedIn(w *httptest.ResponseRecorder) {
	s.T().Helper()
	s.Equal("http://localhost:3000", w.Header().Get("Location"))
	for _, c := range w.Result().Cookies() {
		if c.Name == "auth_token" && c.Value != "" {
			return
		}
	}
	s.Fail("a completed sign-in must issue a session")
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

	_, found := peekOAuthLinkIntent(id)
	s.True(found, "the entry is still held; the CALLER decides it is past its TTL")
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

// assertLinkRefusal checks the refusal CODE, not prose. The redirect carries a
// code precisely so the settings page never renders a sentence taken from a
// URL, and asserting on prose here would let that regress unnoticed.
func (s *OAuthHandlerIntegrationSuite) assertLinkRefusal(w *httptest.ResponseRecorder, wantCode string) {
	s.T().Helper()
	s.Equal(http.StatusTemporaryRedirect, w.Code)
	query := s.parseLinkRedirect(w.Header().Get("Location"))
	s.Equal(wantCode, query.Get(oauthLinkErrorParam))
	s.NotContains(query.Get(oauthLinkErrorParam), " ", "the error parameter carries a code, never a sentence")
	s.Empty(query.Get(oauthLinkResultParam))
}
