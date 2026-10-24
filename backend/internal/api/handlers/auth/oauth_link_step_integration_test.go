package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/markbates/goth"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	autherrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/auth"
)

// These run the Sign in tab's collision state over the real auth and user
// services, the real outbox and the suite's database: the callback holds the
// Google identity as a pending link, and the two proofs, a password sign-in
// and an emailed link, finish it. The routes are mounted on a huma API so the
// cookie parameters bind as they do in production.

const collisionPassword = "correct-horse-battery-staple"

// linkStepEnv is one browser's view of the collision state: the API, and the
// outbox whose sends it can read.
type linkStepEnv struct {
	api    humatest.TestAPI
	outbox *auth.AuthEmailOutbox

	mu   sync.Mutex
	sent []sentLink
}

type sentLink struct{ to, token string }

func (e *linkStepEnv) sends() []sentLink {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]sentLink(nil), e.sent...)
}

func (s *OAuthHandlerIntegrationSuite) linkEnv() *linkStepEnv {
	env := &linkStepEnv{}
	email := &testhelpers.MockEmailService{
		IsConfiguredFn: func() bool { return true },
		SendMagicLinkEmailFn: func(to, token string) error {
			env.mu.Lock()
			defer env.mu.Unlock()
			env.sent = append(env.sent, sentLink{to, token})
			return nil
		},
	}
	jwtSvc := s.jwtService()
	authSvc := auth.NewAuthService(s.deps.DB, s.cfg, s.deps.UserService)
	env.outbox = auth.NewAuthEmailOutbox(s.deps.DB, s.deps.UserService, jwtSvc, email)

	_, api := humatest.New(s.T(), huma.DefaultConfig("link-step", "1.0.0"))
	ah := NewAuthHandler(authSvc, jwtSvc, s.deps.UserService, email, env.outbox, nil, nil, s.cfg)
	huma.Post(api, "/auth/login", ah.LoginHandler)
	huma.Post(api, "/auth/magic-link/verify", ah.VerifyMagicLinkHandler)
	lh := NewOAuthLinkStepHandler(authSvc, email, env.outbox, s.cfg)
	huma.Get(api, OAuthPendingLinkRoute, lh.PendingLinkHandler)
	huma.Post(api, OAuthLinkByEmailRoute, lh.LinkByEmailHandler)
	env.api = api
	return env
}

// passwordAccount is an account on address with collisionPassword, verified
// or not.
func (s *OAuthHandlerIntegrationSuite) passwordAccount(address string, verified bool) *authm.User {
	s.T().Helper()
	hash, err := s.deps.UserService.HashPassword(collisionPassword)
	s.Require().NoError(err)
	user := &authm.User{Email: &address, PasswordHash: &hash, IsActive: true, EmailVerified: verified}
	s.Require().NoError(s.deps.DB.Create(user).Error)
	return user
}

// collisionIdentity is a Google identity for address that Google is the
// authority for: a Workspace account whose hosted domain is the address's.
func collisionIdentity(address, subject string) goth.User {
	return goth.User{
		Provider:  "google",
		UserID:    subject,
		Email:     address,
		Name:      "Alice Moreno",
		AvatarURL: "https://example.com/alice.png",
		RawData:   map[string]any{"verified_email": true, "hd": address[strings.LastIndexByte(address, '@')+1:]},
	}
}

// startLink runs the sign-in callback for identity and returns the pending
// link token the browser was handed.
func (s *OAuthHandlerIntegrationSuite) startLink(identity goth.User, cookies ...*http.Cookie) string {
	s.T().Helper()
	w, req := oauthCallbackRequest("google")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	s.newHandler(&mockOAuthCompleter{user: identity}).OAuthCallbackHTTPHandler(w, req)
	location, err := url.Parse(w.Header().Get("Location"))
	s.Require().NoError(err)
	s.Require().Equal("/auth", location.Path, w.Header().Get("Location"))
	s.Require().Equal(autherrors.CodeOAuthLinkPending, location.Query().Get("error_code"))
	pending := findCookie(w.Result(), oauthPendingLinkCookie.name)
	s.Require().NotNil(pending, "expected the pending link cookie")
	s.Equal(int(oauthPendingLinkCookie.lifetime/time.Second), pending.MaxAge)
	s.True(pending.HttpOnly)
	return pending.Value
}

func linkCookieHeader(token string) string {
	return "Cookie: " + oauthPendingLinkCookie.name + "=" + token
}

func (s *OAuthHandlerIntegrationSuite) login(env *linkStepEnv, address, password string, headers ...any) *httptest.ResponseRecorder {
	return env.api.Post("/auth/login", append(headers, map[string]any{"email": address, "password": password})...)
}

// loginToConnect is the collision state's password proof, which says it is
// one (connect_pending_link).
func (s *OAuthHandlerIntegrationSuite) loginToConnect(env *linkStepEnv, address, password string, headers ...any) *httptest.ResponseRecorder {
	return env.api.Post("/auth/login", append(headers, map[string]any{
		"email": address, "password": password, "connect_pending_link": true,
	})...)
}

func (s *OAuthHandlerIntegrationSuite) verifyLink(env *linkStepEnv, token string) *httptest.ResponseRecorder {
	return env.api.Post("/auth/magic-link/verify", map[string]any{"token": token})
}

// requestLink posts the mailbox proof's send for token and runs the outbox,
// returning the emailed link token.
func (s *OAuthHandlerIntegrationSuite) requestLink(env *linkStepEnv, token string) string {
	s.T().Helper()
	body := s.decode(env.api.Post(OAuthLinkByEmailRoute, linkCookieHeader(token)))
	s.Require().Equal(true, body["success"], body)
	before := len(env.sends())
	env.outbox.RunNow(context.Background())
	sends := env.sends()
	s.Require().Len(sends, before+1, "expected the link to be sent")
	return sends[len(sends)-1].token
}

func (s *OAuthHandlerIntegrationSuite) googleIdentitiesOf(userID uint) []authm.OAuthAccount {
	var rows []authm.OAuthAccount
	s.Require().NoError(s.deps.DB.Where("user_id = ? AND provider = ?", userID, "google").Find(&rows).Error)
	return rows
}

func (s *OAuthHandlerIntegrationSuite) reload(userID uint) *authm.User {
	var user authm.User
	s.Require().NoError(s.deps.DB.First(&user, userID).Error)
	return &user
}

func (s *OAuthHandlerIntegrationSuite) sessionFrom(resp *httptest.ResponseRecorder) string {
	s.T().Helper()
	cookie := setCookies(resp)["auth_token"]
	s.Require().NotNil(cookie, "expected a session cookie")
	s.Require().NotEmpty(cookie.Value)
	return cookie.Value
}

func (s *OAuthHandlerIntegrationSuite) linkRows(userID uint) int64 {
	var rows int64
	s.Require().NoError(s.deps.DB.Model(&authm.OAuthPendingSignup{}).
		Where("purpose = ? AND user_id = ?", authm.OAuthPendingPurposeLink, userID).Count(&rows).Error)
	return rows
}

// --- The callback ---

func (s *OAuthHandlerIntegrationSuite) TestLinkStep_CallbackHoldsTheIdentityAndKeepsTheAddressOutOfTheURL() {
	account := s.passwordAccount("collide@test.com", true)
	w, req := oauthCallbackRequest("google")
	req.AddCookie(&http.Cookie{Name: oauthReturnToCookieName, Value: encodedReturnTo("/shows?city=phoenix")})
	s.newHandler(&mockOAuthCompleter{user: collisionIdentity("collide@test.com", "google-collide")}).OAuthCallbackHTTPHandler(w, req)

	location, err := url.Parse(w.Header().Get("Location"))
	s.Require().NoError(err)
	s.Equal(url.Values{
		"error_code": {autherrors.CodeOAuthLinkPending},
		"returnTo":   {"/shows?city=phoenix"},
	}, location.Query())
	s.NotContains(w.Header().Get("Location"), "collide")
	s.assertNoSessionCookie(w)
	s.Empty(s.googleIdentitiesOf(account.ID), "nothing is attached before a proof")
	s.Equal(int64(1), s.linkRows(account.ID))
}

// Every case the collision state does not serve keeps the plain refusal.
func (s *OAuthHandlerIntegrationSuite) TestLinkStep_CallbackKeepsTheRefusalOutsideItsCase() {
	s.Run("account already holds another Google identity", func() {
		defer s.TearDownTest()
		account := s.passwordAccount("has-google@test.com", true)
		s.Require().NoError(s.deps.DB.Create(&authm.OAuthAccount{UserID: account.ID, Provider: "google", ProviderUserID: "google-original"}).Error)
		w, req := oauthCallbackRequest("google")
		s.newHandler(&mockOAuthCompleter{user: collisionIdentity("has-google@test.com", "google-second")}).OAuthCallbackHTTPHandler(w, req)
		s.assertSignInRefusal(w, "has-google@test.com")
		s.Zero(s.linkRows(account.ID))
	})
	s.Run("deactivated account", func() {
		defer s.TearDownTest()
		account := s.passwordAccount("inactive@test.com", true)
		s.Require().NoError(s.deps.DB.Model(account).Update("is_active", false).Error)
		w, req := oauthCallbackRequest("google")
		s.newHandler(&mockOAuthCompleter{user: collisionIdentity("inactive@test.com", "google-inactive")}).OAuthCallbackHTTPHandler(w, req)
		s.assertSignInRefusal(w, "inactive@test.com")
		s.Zero(s.linkRows(account.ID))
	})
	// Google verified the address once but does not host it: whoever reads
	// that mailbox now may not be whoever holds the Google account, so the
	// emailed link must not be offered to join them.
	s.Run("provider vouches for an address it does not host", func() {
		defer s.TearDownTest()
		account := s.passwordAccount("isp-address@test.com", true)
		identity := collisionIdentity("isp-address@test.com", "google-isp")
		delete(identity.RawData, "hd")
		w, req := oauthCallbackRequest("google")
		s.newHandler(&mockOAuthCompleter{user: identity}).OAuthCallbackHTTPHandler(w, req)
		s.assertSignInRefusal(w, "isp-address@test.com")
		s.Zero(s.linkRows(account.ID))
	})
	s.Run("provider does not vouch for the address", func() {
		defer s.TearDownTest()
		account := s.passwordAccount("unvouched@test.com", true)
		identity := collisionIdentity("unvouched@test.com", "google-unvouched")
		identity.RawData = map[string]any{"verified_email": false}
		w, req := oauthCallbackRequest("google")
		s.newHandler(&mockOAuthCompleter{user: identity}).OAuthCallbackHTTPHandler(w, req)
		s.assertSignInRefusal(w, "unvouched@test.com")
		s.Zero(s.linkRows(account.ID))
	})
	s.Run("CLI sign-in", func() {
		defer s.TearDownTest()
		account := s.passwordAccount("cli@test.com", true)
		storeCLICallback("cli-link", "http://127.0.0.1:9999/cb")
		w, req := oauthCallbackRequest("google")
		req.AddCookie(&http.Cookie{Name: cliCallbackIDCookieName, Value: "cli-link"})
		s.newHandler(&mockOAuthCompleter{user: collisionIdentity("cli@test.com", "google-cli")}).OAuthCallbackHTTPHandler(w, req)
		s.True(strings.HasPrefix(w.Header().Get("Location"), "http://127.0.0.1:9999/cb?error="), w.Header().Get("Location"))
		s.Zero(s.linkRows(account.ID))
	})
}

// --- GET /auth/oauth/pending-link ---

func (s *OAuthHandlerIntegrationSuite) TestLinkStep_PendingReadShowsTheAddress() {
	s.passwordAccount("read-link@test.com", true)
	token := s.startLink(collisionIdentity("read-link@test.com", "google-read-link"))

	resp := s.linkEnv().api.Get(OAuthPendingLinkRoute, linkCookieHeader(token))
	body := s.decode(resp)
	s.Equal("pending", body["status"])
	s.Equal("read-link@test.com", body["email"])
	s.NotEmpty(body["expires_at"])
	s.Equal("no-store", resp.Header().Get("Cache-Control"))
}

// A tampered, foreign, expired, or sign-up token reads as expired and names
// no address; a pending link token does not read as a pending sign-up.
func (s *OAuthHandlerIntegrationSuite) TestLinkStep_PendingReadIsExpiredForAnythingButALiveToken() {
	account := s.passwordAccount("expired-link@test.com", true)
	token := s.startLink(collisionIdentity("expired-link@test.com", "google-expired-link"))
	signupToken := s.startStep(s.stepIdentity("not-a-link"))
	tampered := token[:len(token)-2] + "xx"
	otherBrowsers := strings.Repeat("ab", 32) + "." + strings.Split(token, ".")[1]

	env := s.linkEnv()
	for name, header := range map[string][]any{
		"no cookie":                 nil,
		"tampered signature":        {linkCookieHeader(tampered)},
		"id with another signature": {linkCookieHeader(otherBrowsers)},
		"a pending sign-up's token": {linkCookieHeader(signupToken)},
	} {
		s.Run(name, func() {
			body := s.decode(env.api.Get(OAuthPendingLinkRoute, header...))
			s.Equal("expired", body["status"])
			s.Nil(body["email"])
		})
	}
	signupBody := s.decode(s.stepAPI().Get(OAuthPendingSignupRoute, pendingCookieHeader(token)))
	s.Equal("expired", signupBody["status"], "a pending link is not a pending sign-up")

	s.Require().NoError(s.deps.DB.Exec(
		"UPDATE oauth_pending_signups SET expires_at = NOW() - INTERVAL '1 second' WHERE user_id = ?", account.ID).Error)
	s.Equal("expired", s.decode(env.api.Get(OAuthPendingLinkRoute, linkCookieHeader(token)))["status"])
}

// --- The password proof ---

func (s *OAuthHandlerIntegrationSuite) TestLinkStep_PasswordProofLinksAndSignsIn() {
	account := s.passwordAccount("pw-proof@test.com", true)
	token := s.startLink(collisionIdentity("pw-proof@test.com", "google-pw-proof"))
	env := s.linkEnv()

	resp := s.loginToConnect(env, "pw-proof@test.com", collisionPassword, linkCookieHeader(token))

	body := s.decode(resp)
	s.Equal(true, body["success"], body)
	s.sessionFrom(resp)
	cleared := setCookies(resp)[oauthPendingLinkCookie.name]
	s.Require().NotNil(cleared)
	s.Equal(-1, cleared.MaxAge)

	identities := s.googleIdentitiesOf(account.ID)
	s.Require().Len(identities, 1)
	s.Equal("google-pw-proof", identities[0].ProviderUserID)
	s.Zero(s.linkRows(account.ID), "the proof consumes the pending link")
	s.NotNil(s.reload(account.ID).PasswordHash, "the password proof keeps the password")

	// Replay: the spent cookie connects nothing. A sign-in that asks to
	// connect is refused; an ordinary one carrying it stands on its own.
	again := s.decode(s.loginToConnect(env, "pw-proof@test.com", collisionPassword, linkCookieHeader(token)))
	s.Equal(false, again["success"])
	s.Equal(autherrors.CodeOAuthLinkPendingExpired, again["error_code"])
	plain := s.decode(s.login(env, "pw-proof@test.com", collisionPassword, linkCookieHeader(token)))
	s.Equal(true, plain["success"])
	s.Len(s.googleIdentitiesOf(account.ID), 1)
}

// "Sign in and connect Google" never signs in without connecting: when the
// pending link is gone, or its cookie has lapsed, a correct password is
// refused with OAUTH_LINK_PENDING_EXPIRED and no session.
func (s *OAuthHandlerIntegrationSuite) TestLinkStep_ConnectingSignInRefusedWhenThePendingLinkIsGone() {
	account := s.passwordAccount("pw-gone@test.com", true)
	token := s.startLink(collisionIdentity("pw-gone@test.com", "google-pw-gone"))
	s.Require().NoError(s.deps.DB.Exec(
		"UPDATE oauth_pending_signups SET expires_at = NOW() - INTERVAL '1 second' WHERE user_id = ?", account.ID).Error)
	env := s.linkEnv()

	for name, headers := range map[string][]any{
		"expired pending link": {linkCookieHeader(token)},
		"cookie lapsed":        nil,
	} {
		s.Run(name, func() {
			resp := s.loginToConnect(env, "pw-gone@test.com", collisionPassword, headers...)
			body := s.decode(resp)
			s.Equal(false, body["success"])
			s.Equal(autherrors.CodeOAuthLinkPendingExpired, body["error_code"])
			s.Nil(setCookies(resp)["auth_token"])
		})
	}
	s.Empty(s.googleIdentitiesOf(account.ID))

	// A wrong password is refused as one, saying nothing about the link.
	wrong := s.decode(s.loginToConnect(env, "pw-gone@test.com", "not-the-password"))
	s.Equal(autherrors.CodeInvalidCredentials, wrong["error_code"])
}

// "Use a different Google account" comes back through the callback with
// another identity; the browser stops holding the first one's pending link.
func (s *OAuthHandlerIntegrationSuite) TestLinkStep_AnotherGoogleSignInDropsThePendingLinkCookie() {
	s.passwordAccount("pw-switch@test.com", true)
	token := s.startLink(collisionIdentity("pw-switch@test.com", "google-pw-switch"))

	w, req := oauthCallbackRequest("google")
	req.AddCookie(&http.Cookie{Name: oauthPendingLinkCookie.name, Value: token})
	s.newHandler(&mockOAuthCompleter{user: s.stepIdentity("switch-new")}).OAuthCallbackHTTPHandler(w, req)

	s.Equal("http://localhost:3000"+oauthSignupStepPath, w.Header().Get("Location"))
	cleared := findCookie(w.Result(), oauthPendingLinkCookie.name)
	s.Require().NotNil(cleared, "the earlier pending link's cookie is cleared")
	s.Equal(-1, cleared.MaxAge)
}

func (s *OAuthHandlerIntegrationSuite) TestLinkStep_WrongPasswordLeavesThePendingLink() {
	account := s.passwordAccount("pw-wrong@test.com", true)
	token := s.startLink(collisionIdentity("pw-wrong@test.com", "google-pw-wrong"))

	body := s.decode(s.login(s.linkEnv(), "pw-wrong@test.com", "not-the-password", linkCookieHeader(token)))
	s.Equal(false, body["success"])
	s.Equal(int64(1), s.linkRows(account.ID))
	s.Empty(s.googleIdentitiesOf(account.ID))
}

// A sign-in to another account proves nothing about this pending link: it
// signs in, attaches nothing, and the pending link is discarded.
func (s *OAuthHandlerIntegrationSuite) TestLinkStep_SignInToAnotherAccountDiscardsThePendingLink() {
	colliding := s.passwordAccount("pw-colliding@test.com", true)
	other := s.passwordAccount("pw-other@test.com", true)
	token := s.startLink(collisionIdentity("pw-colliding@test.com", "google-pw-colliding"))

	resp := s.login(s.linkEnv(), "pw-other@test.com", collisionPassword, linkCookieHeader(token))

	s.Equal(true, s.decode(resp)["success"])
	s.sessionFrom(resp)
	s.Equal(-1, setCookies(resp)[oauthPendingLinkCookie.name].MaxAge)
	s.Empty(s.googleIdentitiesOf(other.ID))
	s.Empty(s.googleIdentitiesOf(colliding.ID))
	s.Zero(s.linkRows(colliding.ID))
}

// A sign-in whose pending link is gone signs in all the same.
func (s *OAuthHandlerIntegrationSuite) TestLinkStep_SignInWithAnExpiredPendingLinkSignsIn() {
	account := s.passwordAccount("pw-expired@test.com", true)
	token := s.startLink(collisionIdentity("pw-expired@test.com", "google-pw-expired"))
	s.Require().NoError(s.deps.DB.Exec(
		"UPDATE oauth_pending_signups SET expires_at = NOW() - INTERVAL '1 second' WHERE user_id = ?", account.ID).Error)

	resp := s.login(s.linkEnv(), "pw-expired@test.com", collisionPassword, linkCookieHeader(token))
	s.Equal(true, s.decode(resp)["success"])
	s.Empty(s.googleIdentitiesOf(account.ID))
}

// A sign-in without the cookie, from another browser, leaves the pending
// link for the browser that holds it.
func (s *OAuthHandlerIntegrationSuite) TestLinkStep_SignInWithoutTheCookieLeavesThePendingLink() {
	account := s.passwordAccount("pw-elsewhere@test.com", true)
	s.startLink(collisionIdentity("pw-elsewhere@test.com", "google-pw-elsewhere"))

	resp := s.login(s.linkEnv(), "pw-elsewhere@test.com", collisionPassword)
	s.Equal(true, s.decode(resp)["success"])
	s.Empty(s.googleIdentitiesOf(account.ID))
	s.Equal(int64(1), s.linkRows(account.ID))
}

// The identity was attached to another account between the callback and the
// proof: the sign-in is refused with that refusal and the pending link spent.
func (s *OAuthHandlerIntegrationSuite) TestLinkStep_PasswordProofRefusedWhenTheIdentityMovedElsewhere() {
	account := s.passwordAccount("pw-moved@test.com", true)
	elsewhere := s.passwordAccount("pw-elsewhere-owner@test.com", true)
	token := s.startLink(collisionIdentity("pw-moved@test.com", "google-pw-moved"))
	s.Require().NoError(s.deps.DB.Create(&authm.OAuthAccount{UserID: elsewhere.ID, Provider: "google", ProviderUserID: "google-pw-moved"}).Error)

	resp := s.login(s.linkEnv(), "pw-moved@test.com", collisionPassword, linkCookieHeader(token))
	body := s.decode(resp)
	s.Equal(false, body["success"])
	s.Equal(autherrors.CodeOAuthIdentityInUse, body["error_code"])
	s.Nil(setCookies(resp)["auth_token"])
	s.Zero(s.linkRows(account.ID))
}

// --- The mailbox proof ---

func (s *OAuthHandlerIntegrationSuite) TestLinkStep_MailboxProofLinksTurnsOffThePasswordAndRevokesSessions() {
	account := s.passwordAccount("mail-proof@test.com", true)
	env := s.linkEnv()
	oldSession := s.sessionFrom(s.login(env, "mail-proof@test.com", collisionPassword))

	w, req := oauthCallbackRequest("google")
	req.AddCookie(&http.Cookie{Name: oauthReturnToCookieName, Value: encodedReturnTo("/library")})
	s.newHandler(&mockOAuthCompleter{user: collisionIdentity("mail-proof@test.com", "google-mail-proof")}).OAuthCallbackHTTPHandler(w, req)
	token := findCookie(w.Result(), oauthPendingLinkCookie.name).Value

	linkToken := s.requestLink(env, token)
	s.Equal("mail-proof@test.com", env.sends()[0].to)

	resp := s.verifyLink(env, linkToken)
	body := s.decode(resp)
	s.Require().Equal(true, body["success"], body)
	s.Equal("/library", body["return_to"])
	newSession := s.sessionFrom(resp)

	s.Len(s.googleIdentitiesOf(account.ID), 1)
	after := s.reload(account.ID)
	s.Nil(after.PasswordHash, "the mailbox proof turns off the password")
	s.True(after.EmailVerified)
	s.NotNil(after.SessionsValidAfter)
	s.Zero(s.linkRows(account.ID))

	_, _, err := s.jwtService().ValidateSession(oldSession)
	s.Error(err, "a session from before the link is signed out")
	got, _, err := s.jwtService().ValidateSession(newSession)
	s.Require().NoError(err, "the link's own session works")
	s.Equal(account.ID, got.ID)

	refused := s.decode(s.login(env, "mail-proof@test.com", collisionPassword))
	s.Equal(false, refused["success"])
	s.Equal(autherrors.CodeInvalidCredentials, refused["error_code"])

	// Replay of the emailed link: the pending link is spent.
	replay := s.decode(s.verifyLink(env, linkToken))
	s.Equal(false, replay["success"])
	s.Equal("INVALID_TOKEN", replay["error_code"])
	_, _, err = s.jwtService().ValidateSession(newSession)
	s.NoError(err, "a replayed link revokes nothing further")
}

// The emailed link works from a browser that never held the cookie: the
// mailbox is the proof.
func (s *OAuthHandlerIntegrationSuite) TestLinkStep_MailboxProofWorksFromAnotherBrowser() {
	account := s.passwordAccount("mail-phone@test.com", true)
	token := s.startLink(collisionIdentity("mail-phone@test.com", "google-mail-phone"))
	linkToken := s.requestLink(s.linkEnv(), token)

	phone := s.linkEnv()
	s.Equal(true, s.decode(s.verifyLink(phone, linkToken))["success"])
	s.Len(s.googleIdentitiesOf(account.ID), 1)
}

func (s *OAuthHandlerIntegrationSuite) TestLinkStep_LinkByEmailRefusedForAGonePendingLink() {
	account := s.passwordAccount("mail-gone@test.com", true)
	token := s.startLink(collisionIdentity("mail-gone@test.com", "google-mail-gone"))
	s.Require().NoError(s.deps.DB.Exec(
		"UPDATE oauth_pending_signups SET expires_at = NOW() - INTERVAL '1 second' WHERE user_id = ?", account.ID).Error)

	env := s.linkEnv()
	resp := env.api.Post(OAuthLinkByEmailRoute, linkCookieHeader(token))
	s.requireRefusal(resp, autherrors.CodeOAuthLinkPendingExpired)
	s.Equal(-1, setCookies(resp)[oauthPendingLinkCookie.name].MaxAge)

	var queued int64
	s.Require().NoError(s.deps.DB.Model(&authm.AuthEmailOutboxItem{}).Where("user_id = ?", account.ID).Count(&queued).Error)
	s.Zero(queued)
	s.requireRefusal(env.api.Post(OAuthLinkByEmailRoute), autherrors.CodeOAuthLinkPendingExpired)
}

// An emailed link whose pending link expired after it was sent is refused,
// and changes nothing.
func (s *OAuthHandlerIntegrationSuite) TestLinkStep_MailboxProofRefusedOnceThePendingLinkExpires() {
	account := s.passwordAccount("mail-late@test.com", true)
	token := s.startLink(collisionIdentity("mail-late@test.com", "google-mail-late"))
	env := s.linkEnv()
	linkToken := s.requestLink(env, token)
	s.Require().NoError(s.deps.DB.Exec(
		"UPDATE oauth_pending_signups SET expires_at = NOW() - INTERVAL '1 second' WHERE user_id = ?", account.ID).Error)

	body := s.decode(s.verifyLink(env, linkToken))
	s.Equal(false, body["success"])
	s.Equal("INVALID_TOKEN", body["error_code"])
	s.Empty(s.googleIdentitiesOf(account.ID))
	s.NotNil(s.reload(account.ID).PasswordHash)
	s.Nil(s.reload(account.ID).SessionsValidAfter)
}

// The address moved between the send and the click: the link names a mailbox
// the account no longer has.
func (s *OAuthHandlerIntegrationSuite) TestLinkStep_MailboxProofRefusedWhenTheAddressMoved() {
	account := s.passwordAccount("mail-moved@test.com", true)
	token := s.startLink(collisionIdentity("mail-moved@test.com", "google-mail-moved"))
	env := s.linkEnv()
	linkToken := s.requestLink(env, token)
	s.Require().NoError(s.deps.DB.Model(account).Update("email", "mail-new@test.com").Error)

	body := s.decode(s.verifyLink(env, linkToken))
	s.Equal(false, body["success"])
	s.Empty(s.googleIdentitiesOf(account.ID))
	s.NotNil(s.reload(account.ID).PasswordHash)
}

// --- The pre-hijack case the design exists for ---

// An attacker registers a password account on the victim's address before
// the victim ever arrives. The victim signs in with Google, lands on the
// collision state, and takes the emailed link, because the mailbox is
// theirs. Afterwards Google is the victim's, the attacker's password is
// refused and the attacker's session is signed out.
func (s *OAuthHandlerIntegrationSuite) TestLinkStep_PreHijackedAccountIsReclaimedByTheMailbox() {
	squatted := s.passwordAccount("victim@test.com", false)
	attacker := s.linkEnv()
	attackerSession := s.sessionFrom(s.login(attacker, "victim@test.com", collisionPassword))

	victim := s.linkEnv()
	token := s.startLink(collisionIdentity("victim@test.com", "google-victim"))
	s.Equal("victim@test.com", s.decode(victim.api.Get(OAuthPendingLinkRoute, linkCookieHeader(token)))["email"])
	linkToken := s.requestLink(victim, token)

	// While the victim's link is pending, the attacker signs in with the
	// password from their own browser: that sign-in holds no pending link,
	// so it neither completes nor spends the victim's.
	midSession := s.sessionFrom(s.login(attacker, "victim@test.com", collisionPassword))
	s.Empty(s.googleIdentitiesOf(squatted.ID))
	s.Equal(int64(1), s.linkRows(squatted.ID))
	// Nor can the attacker turn it into the password proof without the
	// victim's cookie.
	s.Equal(autherrors.CodeOAuthLinkPendingExpired,
		s.decode(s.loginToConnect(attacker, "victim@test.com", collisionPassword))["error_code"])

	s.Equal(true, s.decode(s.verifyLink(victim, linkToken))["success"])

	identities := s.googleIdentitiesOf(squatted.ID)
	s.Require().Len(identities, 1)
	s.Equal("google-victim", identities[0].ProviderUserID)

	refused := s.decode(s.login(attacker, "victim@test.com", collisionPassword))
	s.Equal(false, refused["success"])
	s.Equal(autherrors.CodeInvalidCredentials, refused["error_code"])
	_, _, err := s.jwtService().ValidateSession(attackerSession)
	s.Error(err, "the attacker's session is signed out")
	_, _, err = s.jwtService().ValidateSession(midSession)
	s.Error(err, "so is the one minted while the link was pending")

	// The victim signs in with Google from now on.
	w, req := oauthCallbackRequest("google")
	s.newHandler(&mockOAuthCompleter{user: collisionIdentity("victim@test.com", "google-victim")}).OAuthCallbackHTTPHandler(w, req)
	s.NotNil(findCookie(w.Result(), "auth_token"), "Google signs the victim in")
}

// Whichever proof lands first consumes the pending link; the other is
// refused. Password first: the emailed link no longer completes anything.
func (s *OAuthHandlerIntegrationSuite) TestLinkStep_TwoProofRace_PasswordFirst() {
	account := s.passwordAccount("race-pw@test.com", true)
	token := s.startLink(collisionIdentity("race-pw@test.com", "google-race-pw"))
	env := s.linkEnv()
	linkToken := s.requestLink(env, token)

	s.Equal(true, s.decode(s.login(env, "race-pw@test.com", collisionPassword, linkCookieHeader(token)))["success"])
	s.Len(s.googleIdentitiesOf(account.ID), 1)

	body := s.decode(s.verifyLink(env, linkToken))
	s.Equal(false, body["success"])
	s.Equal("INVALID_TOKEN", body["error_code"])
	after := s.reload(account.ID)
	s.NotNil(after.PasswordHash, "the losing proof changes nothing")
	s.Nil(after.SessionsValidAfter)
}

// Mailbox first: the password no longer exists, so a password proof with the
// same cookie cannot complete the link, and attaches nothing a second time.
func (s *OAuthHandlerIntegrationSuite) TestLinkStep_TwoProofRace_MailboxFirst() {
	account := s.passwordAccount("race-mail@test.com", false)
	token := s.startLink(collisionIdentity("race-mail@test.com", "google-race-mail"))
	env := s.linkEnv()
	linkToken := s.requestLink(env, token)

	s.Equal(true, s.decode(s.verifyLink(env, linkToken))["success"])

	body := s.decode(s.login(env, "race-mail@test.com", collisionPassword, linkCookieHeader(token)))
	s.Equal(false, body["success"])
	s.Equal(autherrors.CodeInvalidCredentials, body["error_code"])
	s.Len(s.googleIdentitiesOf(account.ID), 1)
}

// The two proofs at once: exactly one acts.
func (s *OAuthHandlerIntegrationSuite) TestLinkStep_TwoProofRace_Concurrent() {
	account := s.passwordAccount("race-both@test.com", true)
	token := s.startLink(collisionIdentity("race-both@test.com", "google-race-both"))
	env := s.linkEnv()
	linkToken := s.requestLink(env, token)

	authSvc := auth.NewAuthService(s.deps.DB, s.cfg, s.deps.UserService)
	claims, err := s.jwtService().ValidateOAuthLinkToken(linkToken)
	s.Require().NoError(err)

	var wg sync.WaitGroup
	var passwordLinked bool
	var passwordErr, mailboxErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		passwordLinked, passwordErr = authSvc.LinkOAuthWithPassword(token, account)
	}()
	go func() {
		defer wg.Done()
		_, mailboxErr = authSvc.CompleteOAuthLinkByEmail(claims)
	}()
	wg.Wait()

	mailboxWon := mailboxErr == nil
	s.NotEqual(passwordLinked, mailboxWon, "exactly one proof acts (password: %v %v, mailbox: %v)", passwordLinked, passwordErr, mailboxErr)
	if mailboxWon {
		s.True(isLinkPendingExpired(passwordErr), "the password proof is refused: %v", passwordErr)
	} else {
		s.True(isLinkPendingExpired(mailboxErr), "the mailbox proof is refused: %v", mailboxErr)
	}
	s.Len(s.googleIdentitiesOf(account.ID), 1)
	s.Equal(mailboxWon, s.reload(account.ID).PasswordHash == nil)
}

// A passkey on the account is not a proof and is left as it is.
func (s *OAuthHandlerIntegrationSuite) TestLinkStep_MailboxProofLeavesPasskeysAlone() {
	account := s.passwordAccount("mail-passkey@test.com", true)
	s.Require().NoError(s.deps.DB.Create(&authm.WebAuthnCredential{
		UserID:       account.ID,
		CredentialID: []byte("credential-mail-passkey"),
		PublicKey:    []byte("public-key"),
		DisplayName:  "Laptop",
	}).Error)
	token := s.startLink(collisionIdentity("mail-passkey@test.com", "google-mail-passkey"))
	env := s.linkEnv()
	s.Equal(true, s.decode(s.verifyLink(env, s.requestLink(env, token)))["success"])

	var passkeys int64
	s.Require().NoError(s.deps.DB.Model(&authm.WebAuthnCredential{}).Where("user_id = ?", account.ID).Count(&passkeys).Error)
	s.Equal(int64(1), passkeys)
}
